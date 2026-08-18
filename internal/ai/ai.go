package ai

import "context"

// ChatMessage 是一条对话历史记录。Role 取值约定与 OpenAI 兼容接口一致：
// "system"、"user"、"assistant"、"tool"。
// 当 assistant 决定调用工具时 Content 可能为空，ToolCalls 携带调用请求；
// role 为 "tool" 的消息通过 ToolCallID 关联到某次调用，Content 为执行结果。
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall 表示模型发起的一次函数调用请求，Arguments 是 JSON 字符串。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// FunctionDef 描述一个暴露给模型的函数（OpenAI tools 协议的 function 部分），
// Parameters 为参数的 JSON Schema。
type FunctionDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ToolSpec 是随请求发给模型的工具定义。
type ToolSpec struct {
	Type     string      `json:"type"` // 固定为 "function"
	Function FunctionDef `json:"function"`
}

// Chatter 抽象大模型对话能力，便于在 OpenAI 兼容服务与本地 mock 之间切换。
type Chatter interface {
	// Complete 根据历史消息生成一条回复。
	Complete(ctx context.Context, history []ChatMessage) (string, error)
}

// ToolChatter 是 Chatter 的可选增强：把工具清单随请求发给模型，
// 并解析模型返回的 tool_calls。不支持工具协议的实现无需实现该接口，
// 调用方通过类型断言判断是否具备该能力。
type ToolChatter interface {
	CompleteWithTools(ctx context.Context, history []ChatMessage, tools []ToolSpec) (ChatMessage, error)
}
