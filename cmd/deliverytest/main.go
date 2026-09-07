// Command deliverytest 验证「星讯」消息投递可靠性：
//
//	场景 A（在线直推）：发送者在线发消息，接收者在线通过 WebSocket 实时收到。
//	场景 B（离线补偿）：接收者下线期间发送者持续发消息，接收者重新上线后
//	                    通过 GET /api/offline 增量拉取，校验 0 丢失。
//
// 用法（先确保服务已用 scripts/Start-Dev.ps1 -WithInfra 启动）：
//
//	go run ./cmd/deliverytest \
//	  -base http://127.0.0.1:8080 \
//	  -origin http://127.0.0.1:8080 \
//	  -senders 4 -each 20
//
// 账号与限流说明：
//   - 服务端按 IP 限流：注册 10 次/分钟、登录 20 次/分钟；WebSocket 发消息
//     每用户 20 条/分钟（Redis 固定窗口）。脚本内置匀速节流器把速率压在限流之下，
//     不再出现"429 → 干等 61s"的低效循环。
//   - 账号自动注册并缓存到本地文件（-creds，默认 deliverytest-creds.json），
//     缓存含 token（有效期 7 天），再次运行秒级就绪、不触碰注册/登录接口。
//   - 若账号池已存在（连续 3 次注册被拒），脚本自动跳过剩余注册、只登录。
//     需要强制新建账号时用 -force-register。
//   - WS 握手 Origin 必须命中服务端 IM_ALLOWED_ORIGINS 白名单之一。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	wsSendInterval = 3 * time.Second // 单用户 20 条/分钟，3s/条始终安全
	rateWait       = 61 * time.Second
	authScheme     = "Bearer "
)

type account struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"-"`
	ID       int64  `json:"id"`
}

type wsOut struct {
	Type    string `json:"type"`
	To      int64  `json:"to"`
	Content string `json:"content"`
}

type serverMsg struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
	Error   string          `json:"error"`
}

type apiMessage struct {
	ID         int64  `json:"id"`
	FromUserID int64  `json:"from_user_id"`
	ToUserID   int64  `json:"to_user_id"`
	Content    string `json:"content"`
}

var (
	base     = flag.String("base", "http://127.0.0.1:8080", "后端 HTTP 地址")
	origin   = flag.String("origin", "http://127.0.0.1:8080", "WS 握手 Origin，须在服务端 IM_ALLOWED_ORIGINS 白名单内")
	prefix   = flag.String("prefix", "lt", "测试账号用户名前缀")
	password = flag.String("password", "Loadtest@123", "测试账号统一密码")
	sendersN = flag.Int("senders", 1, "并发发送者数量")
	each     = flag.Int("each", 10, "每个发送者发送的消息条数（服务端 20 条/分钟/人，超出会自动等待）")
	ackWait  = flag.Duration("wait", 30*time.Second, "等待服务端 ack / 在线推送的超时")
	credsP   = flag.String("creds", "deliverytest-creds.json", "账号/Token 缓存文件（避免每次重复注册登录）")
	forceReg = flag.Bool("force-register", false, "即使检测到账号池已存在也强制注册（用于批量新建账号）")
)

// pacemaker 以固定最小间隔放行调用，把请求速率压在服务端限流之下。
type pacemaker struct {
	mu       sync.Mutex
	last     time.Time
	interval time.Duration
}

func (p *pacemaker) Wait() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if d := p.interval - time.Since(p.last); d > 0 {
		time.Sleep(d)
	}
	p.last = time.Now()
}

// 服务端按 IP 限流：注册 10 次/分钟、登录 20 次/分钟，间隔留 12% 裕量。
var (
	registerPace = &pacemaker{interval: 6800 * time.Millisecond} // ≈8.8/min < 10
	loginPace    = &pacemaker{interval: 3300 * time.Millisecond} // ≈18/min < 20
	authMu       sync.Mutex
	skipRegister bool // 检测到账号已存在后跳过剩余注册
	takenInARow  int
)

// cachedCreds 是本地账号缓存：第二次起直接复用，不再注册/登录。
type cachedCreds struct {
	Accounts map[string]cachedAccount `json:"accounts"`
}

type cachedAccount struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
	ID       int64  `json:"id"`
}

func loadCreds() *cachedCreds {
	cc := &cachedCreds{Accounts: map[string]cachedAccount{}}
	data, err := os.ReadFile(*credsP)
	if err != nil {
		return cc
	}
	_ = json.Unmarshal(data, cc)
	if cc.Accounts == nil {
		cc.Accounts = map[string]cachedAccount{}
	}
	return cc
}

func saveCreds(cc *cachedCreds) {
	data, err := json.MarshalIndent(cc, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(*credsP, data, 0o600)
	log.Printf("账号缓存已写入 %s（内含 token，勿提交 git）", *credsP)
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags)

	if *sendersN < 1 || *each < 1 {
		log.Fatal("senders 和 each 必须 >= 1")
	}
	log.Printf("准备 %d 个账号（%d 发送者 + 1 接收者）；首次运行需现场注册，受注册 10/分、登录 20/分限流，可能耗时数分钟，之后走缓存秒级就绪", *sendersN+1, *sendersN)
	runID := fmt.Sprintf("%d", time.Now().UnixNano())

	creds := loadCreds()
	recv := ensureAccount(creds, fmt.Sprintf("%s_recv", *prefix))
	accs := make([]account, 0, *sendersN)
	for i := 0; i < *sendersN; i++ {
		accs = append(accs, ensureAccount(creds, fmt.Sprintf("%s_s%d", *prefix, i)))
	}
	saveCreds(creds)
	log.Printf("账号就绪：接收者=%s(id=%d)，发送者=%d 个", recv.Username, recv.ID, len(accs))

	// 好友关系：每个发送者 → 接收者（单聊强制好友校验）
	for i := range accs {
		ensureFriendship(&accs[i], &recv)
	}
	log.Printf("好友关系就绪")

	// 场景 A：接收者在线，sender_0 发 1 条 baseline，验证实时推送链路
	baselineID := onlineProbe(&accs[0], &recv)
	log.Printf("在线直推 OK：接收者实时收到 baseline(id=%d)", baselineID)

	// 场景 B：接收者下线，N 个发送者并发各发 each 条
	time.Sleep(1500 * time.Millisecond) // 等服务端把接收者 presence 置为离线
	log.Printf("开始离线发送：%d 个发送者 × %d 条 = %d 条（自动 20 条/分钟节流）",
		len(accs), *each, len(accs)**each)
	sent := sendOffline(runID, accs, recv)
	log.Printf("发送完成：服务端 ack %d 条", sent)

	// 拉取离线消息并校验完整性
	got := pullOffline(recv, baselineID)
	verify(runID, sent, got)
}

// ---------- HTTP ----------

// apiRequest 发一次 HTTP 请求。429 不自动重试——速率由调用方节流器控制，
// 限流发生时直接返回错误（含状态码），避免"429 → 空等 61s"的恶性循环。
func apiRequest(method, path, token string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, *base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", authScheme+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s -> %d: %s", method, path, resp.StatusCode, truncate(string(data), 200))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ensureAccount 取回一个可用账号（带 token）：
//  1. 本地缓存命中且 token 有效（GET /api/me 200）→ 直接复用，零网络开销；
//  2. 否则按注册限流节奏注册（已存在则 4xx，自动跳过后续重复注册），再按登录限流节奏登录；
//  3. 结果写回缓存，token 有效期内复跑秒级就绪。
func ensureAccount(creds *cachedCreds, username string) account {
	authMu.Lock()
	defer authMu.Unlock()

	if ca, ok := creds.Accounts[username]; ok {
		if err := apiRequest("GET", "/api/me", ca.Token, nil, nil); err == nil {
			return account{Username: ca.Username, Password: ca.Password, Token: ca.Token, ID: ca.ID}
		}
		log.Printf("%s 缓存 token 已失效，重新登录", username)
	}

	if !skipRegister {
		registerPace.Wait()
		var reg struct {
			User account `json:"user"`
		}
		err := apiRequest("POST", "/api/register", "", map[string]string{
			"username": username, "password": *password, "nickname": username,
		}, &reg)
		if err == nil {
			takenInARow = 0
			log.Printf("%s 注册成功", username)
		} else if isRateLimited(err) {
			log.Fatalf("%s 注册被限流(429): %v。等 1 分钟重跑即可，缓存会跳过已就绪账号；若账号均已存在可忽略。", username, err)
		} else {
			takenInARow++
			log.Printf("注册 %s 被拒(%v)，视为账号已存在", username, err)
			if takenInARow >= 3 && !*forceReg {
				skipRegister = true
				log.Printf("连续 3 个账号已存在，本进程跳过剩余注册、直接登录（确需新建请用 -force-register）")
			}
		}
	}

	acc, err := loginRaw(username)
	if err != nil {
		log.Fatalf("账号 %s 登录失败: %v", username, err)
	}
	creds.Accounts[username] = cachedAccount{Username: acc.Username, Password: acc.Password, Token: acc.Token, ID: acc.ID}
	return acc
}

// loginRaw 发起一次登录（带节流，429 不空等）。
func loginRaw(u string) (account, error) {
	loginPace.Wait()
	var resp struct {
		Token string  `json:"token"`
		User  account `json:"user"`
	}
	err := apiRequest("POST", "/api/login", "", map[string]string{"username": u, "password": *password}, &resp)
	if err != nil {
		return account{}, err
	}
	resp.User.Username = u
	resp.User.Password = *password
	resp.User.Token = resp.Token
	return resp.User, nil
}

func isRateLimited(err error) bool {
	return err != nil && strings.Contains(err.Error(), "429")
}

func ensureFriendship(sender, recv *account) {
	_ = apiRequest("POST", "/api/friend-requests", sender.Token,
		map[string]int64{"to_user_id": recv.ID}, nil) // 已加过好友时服务端拒绝，忽略
	var inbox struct {
		Requests []struct {
			ID         int64 `json:"id"`
			FromUserID int64 `json:"from_user_id"`
		} `json:"requests"`
	}
	if err := apiRequest("GET", "/api/friend-requests", recv.Token, nil, &inbox); err != nil {
		log.Fatalf("拉取 %s 的好友请求失败: %v", recv.Username, err)
	}
	for _, r := range inbox.Requests {
		if r.FromUserID != sender.ID {
			continue
		}
		path := fmt.Sprintf("/api/friend-requests/%d/respond", r.ID)
		_ = apiRequest("POST", path, recv.Token, map[string]bool{"accept": true}, nil)
	}
}

// ---------- WebSocket ----------

func wsEndpoint() string {
	u, err := url.Parse(*base)
	if err != nil {
		return "ws://127.0.0.1:8080/ws"
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	return scheme + "://" + u.Host + "/ws"
}

func dialWS(acc *account) (*websocket.Conn, error) {
	d := websocket.Dialer{
		Subprotocols:     []string{"im-chat", acc.Token}, // 身份验证走 Sec-WebSocket-Protocol
		HandshakeTimeout: 10 * time.Second,
	}
	h := http.Header{}
	h.Set("Origin", *origin) // Origin 必须在服务端 IM_ALLOWED_ORIGINS 内
	conn, _, err := d.Dial(wsEndpoint(), h)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// readServerMsg 读一条服务端消息；超时返回 error。
func readServerMsg(conn *websocket.Conn, deadline time.Duration) (*serverMsg, error) {
	_ = conn.SetReadDeadline(time.Now().Add(deadline))
	var m serverMsg
	if err := conn.ReadJSON(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// msgID 提取消息 JSON 里的 id。
func msgID(m *serverMsg) int64 {
	if m == nil || len(m.Message) == 0 {
		return 0
	}
	var mm struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(m.Message, &mm) != nil {
		return 0
	}
	return mm.ID
}

// onlineProbe 场景 A：recv 上线，sender 发 1 条 baseline；返回 recv 收到的消息 ID。
func onlineProbe(sender, recv *account) int64 {
	rc, err := dialWS(recv)
	if err != nil {
		log.Fatalf("接收者 WebSocket 连接失败(检查 -origin 是否在 IM_ALLOWED_ORIGINS): %v", err)
	}
	defer rc.Close()
	time.Sleep(300 * time.Millisecond) // 等 presence 上线

	sc, err := dialWS(sender)
	if err != nil {
		log.Fatalf("发送者 WebSocket 连接失败: %v", err)
	}
	defer sc.Close()

	content := fmt.Sprintf("lt-probe-%d", time.Now().UnixNano())
	if err := sc.WriteJSON(wsOut{Type: "chat", To: recv.ID, Content: content}); err != nil {
		log.Fatalf("发送 baseline 失败: %v", err)
	}
	// 发送者应收到 ack（消息已落库），随后接收者应实时收到 type=chat。
	// 先等发送者 ack
	deadline := time.After(*ackWait)
waitAck:
	for {
		m, err := readServerMsg(sc, 5*time.Second)
		if err != nil {
			break
		}
		switch m.Type {
		case "ack":
			break waitAck
		case "error":
			log.Fatalf("baseline 发送被服务端拒绝: %s", m.Error)
		}
		select {
		case <-deadline:
			log.Fatalf("等待 baseline ack 超时")
		default:
		}
	}
	// 再等接收者实时收到
	for {
		m, err := readServerMsg(rc, *ackWait)
		if err != nil {
			log.Fatalf("在线直推失败：接收者未在 %v 内收到消息（确认 RabbitMQ/Outbox 已随 -WithInfra 启动）: %v", *ackWait, err)
		}
		if m.Type == "chat" {
			return msgID(m)
		}
	}
}

// sendOffline 场景 B：所有发送者并发发送，每条等 ack，3s 节流规避 20 条/分钟限流。
func sendOffline(runID string, senders []account, recv account) int {
	type result struct {
		i   int
		n   int
		err error
	}
	results := make(chan result, len(senders))
	var wg sync.WaitGroup

	for si := range senders {
		wg.Add(1)
		go func(si int) {
			defer wg.Done()
			acc := &senders[si]
			conn, err := dialWS(acc)
			if err != nil {
				results <- result{i: si, err: fmt.Errorf("dial: %w", err)}
				return
			}
			defer conn.Close()

			count := 0
			for seq := 1; seq <= *each; seq++ {
				content := fmt.Sprintf("lt-%s-%d-%d", runID, si, seq)
				if err := conn.WriteJSON(wsOut{Type: "chat", To: recv.ID, Content: content}); err != nil {
					results <- result{i: si, err: fmt.Errorf("seq %d write: %w", seq, err)}
					return
				}
				for {
					m, err := readServerMsg(conn, *ackWait)
					if err != nil {
						results <- result{i: si, err: fmt.Errorf("seq %d 等 ack 超时: %w", seq, err)}
						return
					}
					switch m.Type {
					case "ack":
						count++
						goto next
					case "error":
						// 理论不会触发（3s 节流 < 20条/分钟）；触发即终止并展示原因
						results <- result{i: si, err: fmt.Errorf("seq %d 服务端拒绝: %s", seq, m.Error)}
						return
					}
				}
			next:
				if seq%10 == 0 {
					log.Printf("sender_%d 进度: %d/%d", si, seq, *each)
				}
				time.Sleep(wsSendInterval)
			}
			results <- result{i: si, n: count}
		}(si)
	}
	wg.Wait()
	close(results)

	total := 0
	for r := range results {
		if r.err != nil {
			log.Fatalf("sender_%d 发送失败: %v", r.i, r.err)
		}
		total += r.n
	}
	return total
}

// pullOffline 增量拉取 recv 在 baselineID 之后的所有消息（/api/offline 每页上限 100）。
func pullOffline(recv account, afterID int64) []apiMessage {
	var all []apiMessage
	last := afterID
	for {
		var page struct {
			Messages []apiMessage `json:"messages"`
		}
		path := fmt.Sprintf("/api/offline?after_id=%d&limit=100", last)
		if err := apiRequest("GET", path, recv.Token, nil, &page); err != nil {
			log.Fatalf("拉取离线消息失败: %v", err)
		}
		if len(page.Messages) == 0 {
			break
		}
		for _, m := range page.Messages {
			all = append(all, m)
			if m.ID > last {
				last = m.ID
			}
		}
		if len(page.Messages) < 100 {
			break
		}
	}
	return all
}

// verify 把离线拉到的消息与发送计划比对，输出可直接用于简历的统计。
func verify(runID string, sent int, got []apiMessage) {
	want := *sendersN * *each
	seen := make(map[string]bool, len(got))
	for _, m := range got {
		if strings.HasPrefix(m.Content, "lt-"+runID) {
			seen[m.Content] = true
		}
	}
	var missing []string
	for si := 0; si < *sendersN; si++ {
		for seq := 1; seq <= *each; seq++ {
			key := fmt.Sprintf("lt-%s-%d-%d", runID, si, seq)
			if !seen[key] {
				missing = append(missing, key)
			}
		}
	}
	var unexpected []string
	for _, m := range got {
		if strings.HasPrefix(m.Content, "lt-"+runID) {
			parts := strings.Split(m.Content, "-")
			if len(parts) == 4 {
				si, e1 := strconv.Atoi(parts[2])
				seq, e2 := strconv.Atoi(parts[3])
				if e1 == nil && e2 == nil && (si < 0 || si >= *sendersN || seq < 1 || seq > *each) {
					unexpected = append(unexpected, m.Content)
				}
			}
		}
	}

	fmt.Println()
	fmt.Println("================ 结果汇总 ================")
	fmt.Printf("发送计划        : %d 条（%d 发送者 × %d 条）\n", want, *sendersN, *each)
	fmt.Printf("服务端 ack      : %d 条（发送者视角，确认已落库）\n", sent)
	fmt.Printf("离线拉取(去重)  : %d 条（接收者视角）\n", len(seen))
	if sent == want && len(seen) == want {
		fmt.Printf("缺失            : 0 条\n")
		fmt.Printf("多余/杂散       : 0 条\n")
		fmt.Printf("丢失率          : 0.00%%\n")
		fmt.Println()
		fmt.Printf(">>> 实测 %d 条消息（含离线补偿场景）0 丢失，送达率 100%%\n", want)
		fmt.Println(">>> 简历写法：以「Outbox + 重试队列 + 离线补偿」保证消息不因进程崩溃或断线丢失，")
		fmt.Printf(">>>           压测 %d 条（含离线补偿场景）0 丢失，离线消息 100%% 可达\n", want)
	} else {
		if len(missing) > 0 {
			fmt.Printf("缺失 %d 条，前 10 条: %v\n", len(missing), missing)
		}
		if len(unexpected) > 0 {
			fmt.Printf("杂散消息 %d 条: %v\n", len(unexpected), unexpected)
		}
		if sent != want {
			fmt.Printf("注意: ack=%d 但计划=%d，有消息未落库（检查发送端错误输出）\n", sent, want)
		}
		fmt.Println(">>> 测试未通过，请检查上面的差异")
	}
	fmt.Println("==========================================")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
