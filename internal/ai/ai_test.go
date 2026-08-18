package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompleteWithToolsParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Tools) == 0 || req.ToolChoice != "auto" {
			t.Errorf("expected tools and tool_choice=auto, got %+v", req)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {"name": "get_weather", "arguments": "{\"city\":\"上海\"}"}
					}]
				}
			}]
		}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key", "test-model", "system prompt", 5*time.Second)
	resp, err := client.CompleteWithTools(context.Background(), []ChatMessage{{Role: "user", Content: "上海天气怎么样"}}, ToolSpecs(Registry))
	if err != nil {
		t.Fatalf("CompleteWithTools: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	call := resp.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "get_weather" {
		t.Errorf("unexpected tool call: %+v", call)
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		t.Fatalf("arguments not valid json: %v", err)
	}
	if args["city"] != "上海" {
		t.Errorf("expected city 上海, got %q", args["city"])
	}
}

func TestCompleteWithoutToolsOmitsToolFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Tools != nil || req.ToolChoice != "" {
			t.Errorf("tool fields should be omitted, got %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"你好"}}]}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "", "test-model", "", 5*time.Second)
	reply, err := client.Complete(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply != "你好" {
		t.Errorf("expected 你好, got %q", reply)
	}
}

func TestExecToolUnknownTool(t *testing.T) {
	result := ExecTool(context.Background(), ToolCall{
		ID:       "call_1",
		Function: ToolFunction{Name: "no_such_tool", Arguments: "{}"},
	})
	if !strings.Contains(result, "unknown tool") {
		t.Errorf("expected unknown tool error, got %s", result)
	}
}

func TestDescribeWeatherCode(t *testing.T) {
	if got := describeWeatherCode(0); got != "晴" {
		t.Errorf("code 0 = %q, want 晴", got)
	}
	if got := describeWeatherCode(95); got != "雷暴" {
		t.Errorf("code 95 = %q, want 雷暴", got)
	}
	if got := describeWeatherCode(9999); !strings.Contains(got, "9999") {
		t.Errorf("unknown code should mention the code, got %q", got)
	}
}

func TestBuildTodayInfo(t *testing.T) {
	// 2026-08-16 是星期日
	info := buildTodayInfo(time.Date(2026, 8, 16, 20, 30, 0, 0, time.Local))
	for _, want := range []string{`"date":"2026-08-16"`, `"weekday":"星期日"`, `"time":"20:30"`, `"year":2026`} {
		if !strings.Contains(info, want) {
			t.Errorf("today info %s missing %s", info, want)
		}
	}
}

func TestExecToolGetToday(t *testing.T) {
	result := ExecTool(context.Background(), ToolCall{
		ID:       "call_1",
		Function: ToolFunction{Name: "get_today", Arguments: "{}"},
	})
	if !strings.Contains(result, `"date"`) {
		t.Errorf("expected date field, got %s", result)
	}
}

func TestGeoLocationDisplayName(t *testing.T) {
	g := geoLocation{Name: "上海", Admin1: "上海市", Country: "中国"}
	if got := g.DisplayName(); got != "上海, 上海市, 中国" {
		t.Errorf("DisplayName = %q", got)
	}
	g = geoLocation{Name: "Tokyo", Country: "Japan"}
	if got := g.DisplayName(); got != "Tokyo, Japan" {
		t.Errorf("DisplayName = %q", got)
	}
}
