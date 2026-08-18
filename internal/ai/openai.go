package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIClient 实现 OpenAI 兼容的 /chat/completions 协议。
// DeepSeek、通义千问、OpenAI 以及本地 Ollama 都支持该协议，只需替换 baseURL 与 model。
type OpenAIClient struct {
	baseURL      string
	apiKey       string
	model        string
	systemPrompt string
	http         *http.Client
	enableSearch bool
}

func NewOpenAIClient(baseURL, apiKey, model, systemPrompt string, timeout time.Duration) *OpenAIClient {
	return &OpenAIClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		model:        model,
		systemPrompt: systemPrompt,
		http:         &http.Client{Timeout: timeout},
	}
}

// SetSearch 开启或关闭联网搜索。开启后 Complete 走 DeepSeek 的 /responses
// 接口并携带 web_search 工具，由模型自主决定是否需要搜索。
func (c *OpenAIClient) SetSearch(enabled bool) {
	c.enableSearch = enabled
}

type chatCompletionRequest struct {
	Model      string        `json:"model"`
	Messages   []ChatMessage `json:"messages"`
	Stream     bool          `json:"stream"`
	Tools      []ToolSpec    `json:"tools,omitempty"`
	ToolChoice string        `json:"tool_choice,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAIClient) Complete(ctx context.Context, history []ChatMessage) (string, error) {
	if c.enableSearch {
		return c.completeWithSearch(ctx, history)
	}
	msg, err := c.completeMessage(ctx, history, nil)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return "", errors.New("chat completions returned empty content")
	}
	return content, nil
}

// responsesInputItem 是 DeepSeek /responses（Anthropic 风格）接口的 input 元素。
type responsesInputItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responsesRequest struct {
	Model        string               `json:"model"`
	Instructions string               `json:"instructions,omitempty"`
	Input        []responsesInputItem `json:"input"`
	Tools        []map[string]string  `json:"tools"`
	ToolChoice   string               `json:"tool_choice,omitempty"`
}

type responsesResponse struct {
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// completeWithSearch 走 DeepSeek 的 /responses 接口，携带 web_search 工具，
// 由模型自主决定是否联网搜索。system 提示词通过 instructions 字段传递，
// 多轮历史转换为 input 数组（只保留 user/assistant 文本消息）。
func (c *OpenAIClient) completeWithSearch(ctx context.Context, history []ChatMessage) (string, error) {
	input := make([]responsesInputItem, 0, len(history))
	for _, m := range history {
		switch m.Role {
		case "user", "assistant":
			if strings.TrimSpace(m.Content) != "" {
				input = append(input, responsesInputItem{Role: m.Role, Content: m.Content})
			}
		}
	}

	body, err := json.Marshal(responsesRequest{
		Model:        c.model,
		Instructions: c.systemPrompt,
		Input:        input,
		Tools:        []map[string]string{{"type": "web_search"}},
		ToolChoice:   "auto",
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("responses returned status %d: %s", resp.StatusCode, truncate(string(data), 300))
	}

	var parsed responsesResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil {
		return "", errors.New(parsed.Error.Message)
	}

	// 从 output 中取 type=message 的最终回答文本。
	for _, item := range parsed.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				text := strings.TrimSpace(content.Text)
				if text != "" {
					return text, nil
				}
			}
		}
	}
	return "", errors.New("responses returned no answer text")
}

// CompleteWithTools 在请求中附带工具清单。当模型决定调用工具时，
// 返回的 ChatMessage 含 ToolCalls（Content 可能为空），由调用方在本地
// 执行工具后以 role=tool 消息回传并再次调用，直到模型给出文字回复。
func (c *OpenAIClient) CompleteWithTools(ctx context.Context, history []ChatMessage, tools []ToolSpec) (ChatMessage, error) {
	return c.completeMessage(ctx, history, tools)
}

func (c *OpenAIClient) completeMessage(ctx context.Context, history []ChatMessage, tools []ToolSpec) (ChatMessage, error) {
	messages := make([]ChatMessage, 0, len(history)+1)
	if strings.TrimSpace(c.systemPrompt) != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: c.systemPrompt})
	}
	messages = append(messages, history...)

	request := chatCompletionRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	}
	if len(tools) > 0 {
		request.Tools = tools
		request.ToolChoice = "auto"
	}

	body, err := json.Marshal(request)
	if err != nil {
		return ChatMessage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return ChatMessage{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatMessage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ChatMessage{}, fmt.Errorf("chat completions returned status %d: %s", resp.StatusCode, truncate(string(data), 300))
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ChatMessage{}, err
	}
	if parsed.Error != nil {
		return ChatMessage{}, errors.New(parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return ChatMessage{}, errors.New("chat completions returned no choices")
	}

	msg := parsed.Choices[0].Message
	if strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
		return ChatMessage{}, errors.New("chat completions returned neither content nor tool calls")
	}
	return msg, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
