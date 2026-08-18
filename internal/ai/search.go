package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ---- 联网搜索工具：数据源为 Bing（cn.bing.com），免 API key ----
// 注：本机网络环境下 DuckDuckGo 不可达、百度强制验证码，Bing 实测可稳定返回
// 服务端渲染的结果页，因此选定 Bing 作为唯一搜索后端。

var searchHTTP = &http.Client{Timeout: 12 * time.Second}

var webSearchTool = Tool{
	Spec: ToolSpec{
		Type: "function",
		Function: FunctionDef{
			Name:        "web_search",
			Description: "联网搜索最新信息。当用户询问最新新闻、时事、你训练数据之后发生的事件、不确定的实时信息，或明确要求搜索/查资料时，必须调用本工具，不要凭记忆编造。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "搜索关键词，中英文均可，例如：DeepSeek 最新模型、上海 樱花展 2026",
					},
				},
				"required": []string{"query"},
			},
		},
	},
	Exec: execWebSearch,
}

// SearchResult 是返回给模型的一条搜索结果。
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

func execWebSearch(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("arguments is not valid json: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", errors.New("query is required")
	}

	results, err := searchBing(ctx, query)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", fmt.Errorf("搜索 %q 没有返回任何结果，建议换个关键词", query)
	}

	data, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func searchBing(ctx context.Context, query string) ([]SearchResult, error) {
	// mkt=zh-CN 指定中文市场，避免默认被导向香港区域（繁体新闻站）；
	// setlang=zh-hans 设置界面语言。
	reqURL := "https://www.bing.com/search?q=" + url.QueryEscape(query) + "&mkt=zh-CN&setlang=zh-hans"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	// 伪装成常见浏览器 UA，否则 Bing 可能返回精简页或跳转协议页。
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := searchHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request bing: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bing returned status %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return parseBingResults(string(data), 5), nil
}

var (
	bingBlockStart = regexp.MustCompile(`<li class="b_algo"`)
	bingTitleLink  = regexp.MustCompile(`(?s)<h2[^>]*>\s*<a[^>]*href="(https?://[^"]+)"[^>]*>(.*?)</a>`)
	bingSnippet    = regexp.MustCompile(`(?s)class="b_caption"[^>]*>.*?<p[^>]*>(.*?)</p>`)
	anyTag         = regexp.MustCompile(`<[^>]+>`)
	whitespace     = regexp.MustCompile(`[\s\x{00a0}\x{2002}\x{2003}]+`)
)

// parseBingResults 从 Bing 结果页 HTML 中提取自然结果。
// 每个结果是一个 <li class="b_algo"> 块，标题与链接在块内第一个 <h2><a>，
// 摘要在 class="b_caption" 的段落里；按下一个块起点切分，避免嵌套标签截断。
func parseBingResults(pageHTML string, limit int) []SearchResult {
	locs := bingBlockStart.FindAllStringIndex(pageHTML, -1)
	results := make([]SearchResult, 0, limit)
	for i, loc := range locs {
		if len(results) >= limit {
			break
		}
		end := len(pageHTML)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := pageHTML[loc[0]:end]

		link := bingTitleLink.FindStringSubmatch(block)
		if link == nil {
			continue
		}
		result := SearchResult{
			URL:   unwrapBingRedirect(link[1]),
			Title: cleanHTMLText(link[2]),
		}
		if snippet := bingSnippet.FindStringSubmatch(block); snippet != nil {
			result.Snippet = truncateRunes(cleanHTMLText(snippet[1]), 220)
		}
		results = append(results, result)
	}
	return results
}

// unwrapBingRedirect 还原 Bing 的 /ck/a?...&u=a1<base64url> 跳转链接。
func unwrapBingRedirect(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if !strings.HasSuffix(parsed.Host, "bing.com") || parsed.Path != "/ck/a" {
		return rawURL
	}
	u := parsed.Query().Get("u")
	if !strings.HasPrefix(u, "a1") {
		return rawURL
	}
	decoded, err := base64.RawURLEncoding.DecodeString(u[2:])
	if err != nil {
		return rawURL
	}
	return string(decoded)
}

// cleanHTMLText 去除内联标签、还原 HTML 实体并压缩空白。
// 标签直接移除不加空格：中文标题（如 <strong>上海</strong>天气）不希望被插入空格，
// 英文文本自身已有空位，实体（&ensp;&nbsp; 等）解码后再统一压缩空白。
func cleanHTMLText(s string) string {
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = whitespace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
