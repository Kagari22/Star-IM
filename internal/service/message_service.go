package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"IM_Chat_System/internal/ai"
	"IM_Chat_System/internal/auth"
	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/repository"
	"IM_Chat_System/internal/storage"
	"IM_Chat_System/internal/unread"
)

type MessageService struct {
	users    repository.UserRepository
	groups   repository.GroupRepository
	social   repository.SocialRepository
	messages repository.MessageRepository
	unread   unread.Store
	uploader storage.Uploader

	ai            ai.Chatter
	aiBotID       int64
	aiContextMsgs int
}

const recallWindow = 2 * time.Minute

// 构造 MessageService, 注入用户仓储、消息仓储、未读数存储和文件上传器
func NewMessageService(users repository.UserRepository, groups repository.GroupRepository, social repository.SocialRepository, messages repository.MessageRepository, unreadStore unread.Store, uploader storage.Uploader) *MessageService {
	if uploader == nil {
		uploader = storage.NoopUploader{}
	}
	return &MessageService{
		users:    users,
		groups:   groups,
		social:   social,
		messages: messages,
		unread:   unreadStore,
		uploader: uploader,
	}
}

// SetAI 配置 AI 机器人。chatter 为 nil 或 botID <= 0 时功能关闭。
func (s *MessageService) SetAI(chatter ai.Chatter, botID int64, contextMsgs int) {
	s.ai = chatter
	s.aiBotID = botID
	s.aiContextMsgs = contextMsgs
}

// 获取除当前用户外的用户列表, 并补充每个会话的未读消息数
// MySQL 查询联系人
// → 提取所有对方用户 ID
// → Redis 批量查询会话未读数
// → 写入 User.UnreadCount
func (s *MessageService) ListUsers(ctx context.Context, currentUserID int64) ([]model.User, error) {
	var users []model.User
	var err error
	if s.social != nil {
		users, err = s.social.ListFriends(ctx, currentUserID)
	} else {
		users, err = s.users.List(ctx, currentUserID)
	}
	if err != nil {
		return nil, err
	}
	if s.unread == nil || len(users) == 0 {
		for i := range users {
			s.enrichAvatarURL(ctx, &users[i])
		}
		return users, nil
	}

	peerIDs := make([]int64, 0, len(users))
	for _, user := range users {
		peerIDs = append(peerIDs, user.ID)
	}
	counts, err := s.unread.GetConversationCounts(ctx, currentUserID, peerIDs)
	if err != nil {
		return nil, err
	}
	for i := range users {
		users[i].UnreadCount = counts[users[i].ID]
		s.enrichAvatarURL(ctx, &users[i])
	}
	return users, nil
}

// 查询当前登录用户信息
func (s *MessageService) GetMe(ctx context.Context, userID int64) (model.User, error) {
	user, ok, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return model.User{}, err
	}
	if !ok {
		return model.User{}, errors.New("user not found")
	}
	user.PasswordHash = ""
	s.enrichAvatarURL(ctx, &user)
	return user, nil
}

// 保存文本消息并生成异步事件
// 去除消息首尾空格
// → 校验接收者和消息内容
// → 检查接收者是否存在
// → MySQL 事务保存消息和 Outbox 事件
// → Redis 增加接收者未读数
// → 返回消息
func (s *MessageService) SaveText(ctx context.Context, fromUserID, toUserID int64, content string) (model.Message, error) {
	return s.SaveTextReply(ctx, fromUserID, toUserID, content, nil)
}

func (s *MessageService) SaveTextReply(ctx context.Context, fromUserID, toUserID int64, content string, replyToID *int64) (model.Message, error) {
	content = strings.TrimSpace(content)
	if toUserID <= 0 || content == "" {
		return model.Message{}, errors.New("to and content are required")
	}

	if _, ok, err := s.users.GetByID(ctx, toUserID); err != nil {
		return model.Message{}, err
	} else if !ok {
		return model.Message{}, errors.New("receiver not found")
	}
	if s.social != nil {
		if toUserID == s.aiBotID {
			// 发给 AI 机器人时自动建立好友关系，覆盖功能启用前已注册的老用户。
			if err := s.social.EnsureFriendship(ctx, fromUserID, toUserID); err != nil {
				return model.Message{}, err
			}
		} else {
			friends, err := s.social.IsFriend(ctx, fromUserID, toUserID)
			if err != nil {
				return model.Message{}, err
			}
			if !friends {
				return model.Message{}, errors.New("you can only message an accepted friend")
			}
		}
	}

	message, err := s.messages.SaveAndEnqueue(ctx, model.Message{
		FromUserID:  fromUserID,
		ToUserID:    toUserID,
		ContentType: "text",
		Content:     content,
		ReplyToID:   replyToID,
	})
	if err != nil {
		return model.Message{}, err
	}
	if s.unread != nil {
		if _, err := s.unread.Increment(ctx, toUserID, fromUserID, message.ID); err != nil {
			log.Printf("increment unread for message %d: %v", message.ID, err)
		}
	}
	if s.ai != nil && toUserID == s.aiBotID {
		go s.replyFromAI(fromUserID, toUserID)
	}
	return message, nil
}

// replyFromAI 异步调用大模型为用户生成回复，并把回复作为机器人 → 用户的消息保存。
// 复用 SaveAndEnqueue，因此回复会走 Outbox → RabbitMQ → WebSocket 推送，同时更新未读数。
func (s *MessageService) replyFromAI(userID, botID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	history, err := s.messages.ListRecentConversation(ctx, userID, botID, s.aiContextMsgs)
	if err != nil {
		log.Printf("load ai context: %v", err)
		return
	}

	msgs := make([]ai.ChatMessage, 0, len(history))
	var lastUserMsg string
	for _, m := range history {
		switch {
		case m.FromUserID == userID:
			lastUserMsg = m.Content
			msgs = append(msgs, ai.ChatMessage{Role: "user", Content: m.Content})
		case m.FromUserID == botID:
			msgs = append(msgs, ai.ChatMessage{Role: "assistant", Content: m.Content})
		}
	}

	// 注入"今天"的具体日期，供模型搜索新闻时把日期写进搜索词、
	// 只采用当天结果，避免查到其他日期的旧新闻。
	msgs = append([]ai.ChatMessage{todayContextMessage()}, msgs...)

	// 能力询问（"你能联网搜索吗"等）模型回答不稳定，可能在随机时刻说自己"不支持"。
	// 这里在代码层直接给出确定性回答，不依赖模型的随机行为。
	if isCapabilityQuestion(lastUserMsg) {
		s.saveBotMessage(ctx, userID, botID, aiCapabilityAnswer)
		return
	}

	reply, err := s.generateAIReply(ctx, msgs)
	if err != nil {
		log.Printf("ai complete: %v", err)
		// 回显失败原因，避免静默无回复。
		s.saveBotMessage(ctx, userID, botID, "抱歉，AI 暂时无法回复："+err.Error())
		return
	}
	reply = cleanAIReply(reply)
	if reply == "" {
		return
	}

	s.saveBotMessage(ctx, userID, botID, reply)
}

// cleanAIReply 去掉 AI 回复里的 Markdown 强调星号（**加粗** / *斜体*），
// 避免聊天界面直接显示原始标记符号。
func cleanAIReply(reply string) string {
	reply = strings.ReplaceAll(reply, "**", "")
	reply = strings.ReplaceAll(reply, "*", "")
	return strings.TrimSpace(reply)
}

const aiCapabilityAnswer = "可以！我支持联网搜索，能帮你查新闻、最新资讯、实时信息等。你直接告诉我具体想查什么就行，比如「帮我查一下今天的科技新闻」。"

var aiWeekdayText = [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

// todayContextMessage 把"今天"的具体日期注入对话，让模型能据此按日期搜索新闻。
func todayContextMessage() ai.ChatMessage {
	now := time.Now()
	dateStr := now.Format("2006年1月2日")
	iso := now.Format("2006-01-02")
	return ai.ChatMessage{
		Role: "system",
		Content: fmt.Sprintf("今天是%s（%s）。凡涉及「今天」「当日」「最新」的新闻或事件，一律以今天 %s 为准；需要联网搜索时，把完整日期写进搜索词（如「%s 新闻」），并只采用当天发布的结果，忽略其他日期的旧新闻。",
			dateStr, aiWeekdayText[now.Weekday()], iso, iso),
	}
}

// capabilityAskPattern 匹配"能不能联网/支持搜索吗"这类能力询问。
// 限定为短消息、含能力询问词与能力词，避免误伤"帮我搜一下XX"等实际请求。
var capabilityAskPattern = regexp.MustCompile(`(?i)(能|可以|支持|会不会|有没有|是否).{0,8}(联网|上网|搜索|查新闻|查资讯|搜索功能)`)

func isCapabilityQuestion(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > 30 {
		return false
	}
	if !capabilityAskPattern.MatchString(s) {
		return false
	}
	// 仅拦截问句：以 吗/么/？/? 结尾，或整句很短。
	return strings.HasSuffix(s, "吗") || strings.HasSuffix(s, "么") ||
		strings.HasSuffix(s, "？") || strings.HasSuffix(s, "?") || len([]rune(s)) <= 10
}

// maxAIToolRounds 限制"模型请求工具 → 本地执行 → 回传结果"的最大轮数，防止死循环。
const maxAIToolRounds = 8

// generateAIReply 生成 AI 回复。若底层 Chatter 支持工具协议（ai.ToolChatter），
// 则循环执行工具调用直到模型给出文字回复；否则退化为普通 Complete。
// 工具调用的中间消息只存在于本次循环内，不会写入聊天历史。
func (s *MessageService) generateAIReply(ctx context.Context, msgs []ai.ChatMessage) (string, error) {
	toolChatter, ok := s.ai.(ai.ToolChatter)
	if !ok || len(ai.Registry) == 0 {
		return s.ai.Complete(ctx, msgs)
	}

	specs := ai.ToolSpecs(ai.Registry)
	for round := 0; round < maxAIToolRounds; round++ {
		resp, err := toolChatter.CompleteWithTools(ctx, msgs, specs)
		if err != nil {
			return "", err
		}
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// 模型的工具调用请求必须先回传，后续 role=tool 消息才能通过 ID 关联。
		msgs = append(msgs, resp)
		for _, call := range resp.ToolCalls {
			log.Printf("ai tool call: %s(%s)", call.Function.Name, call.Function.Arguments)
			msgs = append(msgs, ai.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Function.Name,
				Content:    ai.ExecTool(ctx, call),
			})
		}
	}
	return "", errors.New("ai exceeded max tool rounds")
}

// saveBotMessage 保存一条机器人 → 用户的消息并更新未读数。
func (s *MessageService) saveBotMessage(ctx context.Context, userID, botID int64, content string) {
	saved, err := s.messages.SaveAndEnqueue(ctx, model.Message{
		FromUserID:  botID,
		ToUserID:    userID,
		ContentType: "text",
		Content:     content,
	})
	if err != nil {
		log.Printf("save ai reply: %v", err)
		return
	}
	if s.unread != nil {
		if _, err := s.unread.Increment(ctx, userID, botID, saved.ID); err != nil {
			log.Printf("increment unread for ai reply %d: %v", saved.ID, err)
		}
	}
}

// SaveGroupText 会在保存一条群聊消息之前, 验证发送者是否属于该群
// 群消息的投递以及未读消息追踪会单独处理
// 因为一个群拥有多个接收者
func (s *MessageService) SaveGroupText(ctx context.Context, fromUserID, groupID int64, content string) (model.Message, error) {
	return s.SaveGroupTextReply(ctx, fromUserID, groupID, content, nil)
}

func (s *MessageService) SaveGroupTextReply(ctx context.Context, fromUserID, groupID int64, content string, replyToID *int64) (model.Message, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return model.Message{}, errors.New("content is required")
	}
	return s.saveGroupMessage(ctx, fromUserID, groupID, "text", content, replyToID)
}

// SaveGroupRedPacket 在群里发送一条红包消息。content 存 JSON（含红包 ID 与祝福语），前端据此渲染红包卡片。
func (s *MessageService) SaveGroupRedPacket(ctx context.Context, fromUserID, groupID, packetID int64, greeting string) (model.Message, error) {
	if packetID <= 0 {
		return model.Message{}, errors.New("packet_id is required")
	}
	content, err := json.Marshal(map[string]any{"id": packetID, "greeting": greeting})
	if err != nil {
		return model.Message{}, err
	}
	return s.saveGroupMessage(ctx, fromUserID, groupID, "red_packet", string(content), nil)
}

func (s *MessageService) saveGroupMessage(ctx context.Context, fromUserID, groupID int64, contentType, content string, replyToID *int64) (model.Message, error) {
	if groupID <= 0 {
		return model.Message{}, errors.New("group_id is required")
	}

	member, isMember, err := s.groups.IsMember(ctx, groupID, fromUserID)
	if err != nil {
		return model.Message{}, err
	}
	if !isMember {
		return model.Message{}, errors.New("you are not a member of this group")
	}
	if member.MutedUntil != nil && member.MutedUntil.After(time.Now()) {
		return model.Message{}, errors.New("you are muted in this group")
	}
	group, exists, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return model.Message{}, err
	}
	if !exists {
		return model.Message{}, errors.New("group not found")
	}
	if group.AllMuted && member.Role == model.GroupRoleMember {
		return model.Message{}, errors.New("all members are muted")
	}

	message, err := s.messages.SaveAndEnqueue(ctx, model.Message{
		FromUserID:  fromUserID,
		GroupID:     &groupID,
		ContentType: contentType,
		Content:     content,
		ReplyToID:   replyToID,
	})
	if err != nil {
		return model.Message{}, err
	}
	return message, nil
}

func (s *MessageService) Edit(ctx context.Context, userID, messageID int64, content string) (model.Message, error) {
	content = strings.TrimSpace(content)
	if userID <= 0 || messageID <= 0 || content == "" {
		return model.Message{}, errors.New("message_id and content are required")
	}
	if utf8.RuneCountInString(content) > 4000 {
		return model.Message{}, errors.New("message is too long")
	}
	return s.messages.Edit(ctx, messageID, userID, content)
}

func (s *MessageService) SetFavorite(ctx context.Context, userID, messageID int64, favorite bool) error {
	if userID <= 0 || messageID <= 0 {
		return errors.New("invalid message")
	}
	return s.messages.SetFavorite(ctx, userID, messageID, favorite)
}

// Favorite 返回当前用户在指定会话中收藏的消息。
func (s *MessageService) Favorite(ctx context.Context, userID int64, peerID, groupID *int64) ([]model.Message, error) {
	messages, err := s.messages.ListFavorite(ctx, userID, peerID, groupID)
	if err != nil {
		return nil, err
	}
	return s.EnrichMessages(ctx, messages)
}

func (s *MessageService) MarkRead(ctx context.Context, userID, peerID, groupID, messageID int64) error {
	if userID <= 0 || messageID <= 0 || (peerID <= 0 && groupID <= 0) {
		return errors.New("invalid read receipt")
	}
	if groupID > 0 {
		_, member, err := s.groups.IsMember(ctx, groupID, userID)
		if err != nil {
			return err
		}
		if !member {
			return errors.New("you are not a member of this group")
		}
	}
	return s.messages.MarkRead(ctx, userID, peerID, groupID, messageID)
}

// 保存图片或文件消息
// 校验接收者
// → 清理文件名
// → 推断文件 MIME 类型
// → 生成对象存储 Key
// → 上传 MinIO
// → 判断消息类型是 image 还是 file
// → MySQL 保存消息和 Outbox 事件
// → Redis 增加未读数
// → 生成短时预签名 URL
// → 返回媒体消息
func (s *MessageService) SaveMedia(ctx context.Context, fromUserID, toUserID int64, fileName string, size int64, contentType string, reader io.Reader) (model.Message, error) {
	if toUserID <= 0 {
		return model.Message{}, errors.New("to_user_id is required")
	}
	if _, ok, err := s.users.GetByID(ctx, toUserID); err != nil {
		return model.Message{}, err
	} else if !ok {
		return model.Message{}, errors.New("receiver not found")
	}

	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" {
		fileName = "file"
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(fileName))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	objectName := buildObjectName(fromUserID, toUserID, fileName)
	object, err := s.uploader.Upload(ctx, objectName, reader, size, contentType)
	if err != nil {
		return model.Message{}, err
	}

	messageType := "file"
	if strings.HasPrefix(contentType, "image/") {
		messageType = "image"
	}

	message, err := s.messages.SaveAndEnqueue(ctx, model.Message{
		FromUserID:  fromUserID,
		ToUserID:    toUserID,
		ContentType: messageType,
		Content:     fileName,
		ObjectKey:   object.Key,
		ObjectURL:   object.URL,
		FileName:    fileName,
		FileSize:    object.Size,
	})
	if err != nil {
		return model.Message{}, err
	}
	if s.unread != nil {
		if _, err := s.unread.Increment(ctx, toUserID, fromUserID, message.ID); err != nil {
			log.Printf("increment unread for message %d: %v", message.ID, err)
		}
	}
	enriched, err := s.EnrichMessage(ctx, message)
	if err != nil {
		log.Printf("presign media for message %d: %v", message.ID, err)
		return message, nil
	}
	return enriched, nil
}

// 获取当前用户与指定用户之间的聊天历史
// 校验 peerID
// → 规范化 limit
// → 查询双向聊天记录
// → 清除已读范围内的 Redis 未读数
// → 为媒体消息补充预签名 URL
// → 返回消息列表
func (s *MessageService) Conversation(ctx context.Context, userID, peerID, afterID int64, limit int) ([]model.Message, error) {
	if peerID <= 0 {
		return nil, errors.New("peer_id is required")
	}
	limit = normalizeLimit(limit, 100)
	messages, err := s.messages.ListConversation(ctx, userID, peerID, afterID, limit)
	if err != nil {
		return nil, err
	}
	if s.unread != nil {
		throughID := afterID
		// Opening a conversation from the beginning marks the whole conversation
		// as read. Using only the last message returned by the paged query can
		// leave stale Redis entries behind (for example when the query is empty or
		// there are more than `limit` messages), causing the badge to reappear on
		// the next users refresh.
		if afterID == 0 {
			throughID = int64(^uint64(0) >> 1)
		} else if len(messages) > 0 {
			throughID = messages[len(messages)-1].ID
		}
		if err := s.unread.ClearConversation(ctx, userID, peerID, throughID); err != nil {
			return nil, err
		}
	}
	return s.EnrichMessages(ctx, messages)
}

// 获取当前用户离线期间收到的消息
// 规范化 limit
// → 查询 to_user_id = 当前用户且 id > afterID 的消息
// → 补充媒体预签名 URL
// → 返回离线消息
func (s *MessageService) Offline(ctx context.Context, userID, afterID int64, limit int) ([]model.Message, error) {
	limit = normalizeLimit(limit, 100)
	messages, err := s.messages.ListOffline(ctx, userID, afterID, limit)
	if err != nil {
		return nil, err
	}
	return s.EnrichMessages(ctx, messages)
}

// 为单条图片或文件消息补充最新的 MinIO 预签名访问 URL
// 如果消息没有 ObjectKey, 说明不是媒体消息, 直接返回原消息
func (s *MessageService) EnrichMessage(ctx context.Context, message model.Message) (model.Message, error) {
	if message.ObjectKey == "" || s.uploader == nil {
		return message, nil
	}
	url, err := s.uploader.PresignGet(ctx, message.ObjectKey)
	if err != nil {
		return model.Message{}, err
	}
	message.ObjectURL = url
	return message, nil
}

// 批量调用 EnrichMessage, 为消息列表中的所有媒体消息生成访问 URL
// 搜索结果、聊天历史和离线消息都会使用该函数
func (s *MessageService) EnrichMessages(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	for i := range messages {
		message, err := s.EnrichMessage(ctx, messages[i])
		if err != nil {
			return nil, err
		}
		messages[i] = message
	}
	return messages, nil
}

func (s *MessageService) UpdateNickname(ctx context.Context, userID int64, nickname string) (model.User, error) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		return model.User{}, errors.New("nickname is required")
	}
	if utf8.RuneCountInString(nickname) > 64 {
		return model.User{}, errors.New("nickname must be at most 64 characters")
	}

	user, ok, err := s.users.UpdateNickname(ctx, userID, nickname)
	if err != nil {
		return model.User{}, err
	}
	if !ok {
		return model.User{}, errors.New("user not found")
	}
	user.PasswordHash = ""
	return user, nil
}

func (s *MessageService) enrichAvatarURL(ctx context.Context, user *model.User) {
	if user.AvatarKey == "" {
		return
	}
	url, err := s.uploader.PresignGet(ctx, user.AvatarKey)
	if err != nil {
		log.Printf("presign avatar %s: %v", user.AvatarKey, err)
		return
	}
	user.AvatarURL = url
}

func (s *MessageService) SaveAvatar(ctx context.Context, userID int64, reader io.Reader, size int64, contentType string) (model.User, error) {
	objectKey := "avatars/" + strconv.FormatInt(userID, 10)
	_, err := s.uploader.Upload(ctx, objectKey, reader, size, contentType)
	if err != nil {
		return model.User{}, err
	}
	if err := s.users.UpdateAvatarKey(ctx, userID, objectKey); err != nil {
		return model.User{}, err
	}
	user, ok, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return model.User{}, err
	}
	if !ok {
		return model.User{}, errors.New("user not found")
	}
	user.PasswordHash = ""
	s.enrichAvatarURL(ctx, &user)
	return user, nil
}

func (s *MessageService) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	user, ok, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("user not found")
	}
	if !auth.CheckPassword(oldPassword, user.PasswordHash) {
		return errors.New("old password is incorrect")
	}
	if len(newPassword) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, userID, newHash)
}

// 统一处理分页数量
func normalizeLimit(limit, maximum int) int {
	if limit <= 0 {
		return 50
	}
	if limit > maximum {
		return maximum
	}
	return limit
}

// 生成 MinIO 对象存储路径
func buildObjectName(fromUserID, toUserID int64, fileName string) string {
	safeName := strings.ReplaceAll(fileName, " ", "_")
	return "chat/" + strconv.FormatInt(fromUserID, 10) + "/" + strconv.FormatInt(toUserID, 10) + "/" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + safeName
}

// 只有群成员才可以查看群聊天记录
func (s *MessageService) GroupConversation(ctx context.Context, userID, groupID, afterID int64, limit int) ([]model.Message, error) {
	if groupID <= 0 {
		return nil, errors.New("group_id is required")
	}
	_, isMember, err := s.groups.IsMember(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errors.New("you are not a member of this group")
	}
	limit = normalizeLimit(limit, 100)
	messages, err := s.messages.ListGroupConversation(ctx, userID, groupID, afterID, limit)
	if err != nil {
		return nil, err
	}
	if len(messages) > 0 {
		_ = s.messages.MarkRead(ctx, userID, 0, groupID, messages[len(messages)-1].ID)
	}
	return s.EnrichMessages(ctx, messages)
}

// 查询某个群有哪些成员
func (s *MessageService) GroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	return s.groups.ListMembers(ctx, groupID)
}

// CanAccessGroup verifies group membership before a group-scoped query.
func (s *MessageService) CanAccessGroup(ctx context.Context, userID, groupID int64) (bool, error) {
	if groupID <= 0 {
		return false, errors.New("invalid group id")
	}
	_, isMember, err := s.groups.IsMember(ctx, groupID, userID)
	return isMember, err
}

func (s *MessageService) DeleteConversationMessages(ctx context.Context, userID int64, peerID, groupID *int64, messageIDs []int64, all bool) (int64, error) {
	if userID <= 0 {
		return 0, errors.New("invalid user")
	}
	if (peerID == nil) == (groupID == nil) {
		return 0, errors.New("exactly one conversation target is required")
	}
	if peerID != nil {
		if *peerID <= 0 || *peerID == userID {
			return 0, errors.New("invalid peer_id")
		}
		if s.social != nil {
			friends, err := s.social.IsFriend(ctx, userID, *peerID)
			if err != nil {
				return 0, err
			}
			if !friends {
				return 0, errors.New("you can only manage messages in an accepted friend conversation")
			}
		}
	}
	if groupID != nil {
		if *groupID <= 0 {
			return 0, errors.New("invalid group_id")
		}
		if _, member, err := s.groups.IsMember(ctx, *groupID, userID); err != nil {
			return 0, err
		} else if !member {
			return 0, errors.New("you are not a member of this group")
		}
	}
	for _, id := range messageIDs {
		if id <= 0 {
			return 0, errors.New("invalid message_id")
		}
	}
	return s.messages.DeleteConversationMessages(ctx, userID, peerID, groupID, messageIDs, all)
}

// Recall 将当前用户的一条消息标记为已撤回。
// Repository 负责执行原子性的“消息作者 + 时间窗口”检查，
// 并在同一个事务中写入 Outbox 事件；Service 负责输入参数校验和业务规则。
func (s *MessageService) Recall(ctx context.Context, userID, messageID int64) (model.Message, error) {
	if userID <= 0 {
		return model.Message{}, errors.New("invalid user")
	}
	if messageID <= 0 {
		return model.Message{}, errors.New("invalid message_id")
	}
	if s.messages == nil {
		return model.Message{}, errors.New("message repository is unavailable")
	}

	message, err := s.messages.Recall(ctx, messageID, userID, time.Now().Add(-recallWindow))
	if err != nil {
		return model.Message{}, err
	}

	// 被撤回的消息仍然保留在历史记录中，用于保证消息排序和审计；
	// 但它原本的媒体 URL 不能再通过 API 响应提供给用户使用。
	message.Content = ""
	message.ObjectURL = ""
	message.ObjectKey = ""
	message.FileName = ""
	message.FileSize = 0
	return message, nil
}
