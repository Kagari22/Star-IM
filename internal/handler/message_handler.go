package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/service"
)

type MessageHandler struct {
	messages *service.MessageService
}

func NewMessageHandler(messages *service.MessageService) *MessageHandler {
	return &MessageHandler{messages: messages}
}

// 获取与指定用户的聊天记录
func (h *MessageHandler) Messages(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	peerID, _ := strconv.ParseInt(r.URL.Query().Get("peer_id"), 10, 64) // 聊天对方的用户 ID
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after_id"), 10, 64) // 增量查询游标, 只查询 ID 比它大的消息
	limit, err := parseBoundedLimit(r.URL.Query().Get("limit"), 100) // 读取每次返回的最大消息数, 并限制上限为 100
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 调用 Service 层查询会话记录
	messages, err := h.messages.Conversation(r.Context(), claims.UserID, peerID, afterID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

// 拉取当前用户离线期间收到的消息
func (h *MessageHandler) Offline(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after_id"), 10, 64)
	limit, err := parseBoundedLimit(r.URL.Query().Get("limit"), 100)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 调用 Service 层, 以当前登录用户 ID 查询离线消息
	messages, err := h.messages.Offline(r.Context(), claims.UserID, afterID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

// 解析并限制接口的 limit 参数, 防止客户端一次请求过多数据
func parseBoundedLimit(raw string, maximum int) (int, error) {
	if raw == "" {
		return 50, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 || limit > maximum {
		return 0, fmt.Errorf("limit must be between 1 and %d", maximum)
	}
	return limit, nil
}
