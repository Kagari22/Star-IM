package handler

import (
	"context"
	"encoding/json"
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

	peerID, _ := strconv.ParseInt(r.URL.Query().Get("peer_id"), 10, 64)   // 聊天对方的用户 ID
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after_id"), 10, 64) // 增量查询游标, 只查询 ID 比它大的消息
	limit, err := parseBoundedLimit(r.URL.Query().Get("limit"), 100)      // 读取每次返回的最大消息数, 并限制上限为 100
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

type deleteMessagesRequest struct {
	PeerID     *int64  `json:"peer_id,omitempty"`
	GroupID    *int64  `json:"group_id,omitempty"`
	MessageIDs []int64 `json:"message_ids,omitempty"`
	All        bool    `json:"all"`
}

func (h *MessageHandler) DeleteConversationMessages(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input deleteMessagesRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !input.All && len(input.MessageIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "message_ids are required unless all is true")
		return
	}
	deleted, err := h.messages.DeleteConversationMessages(r.Context(), claims.UserID, input.PeerID, input.GroupID, input.MessageIDs, input.All)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

// Recall marks a message sent by the current user as recalled.
func (h *MessageHandler) Recall(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	messageID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || messageID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	message, err := h.messages.Recall(r.Context(), claims.UserID, messageID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"message": message})
}

type editMessageRequest struct {
	Content string `json:"content"`
}

func (h *MessageHandler) Edit(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	messageID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || messageID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	var input editMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	message, err := h.messages.Edit(r.Context(), claims.UserID, messageID, input.Content)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"message": message})
}

type messageFlagRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *MessageHandler) SetFavorite(w http.ResponseWriter, r *http.Request) {
	h.setFlag(w, r, h.messages.SetFavorite)
}

// GET /api/messages/favorites?peer_id= 或 ?group_id= 返回当前会话收藏的消息。
func (h *MessageHandler) Favorite(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var peerID, groupID *int64
	if raw := r.URL.Query().Get("peer_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid peer_id")
			return
		}
		peerID = &id
	}
	if raw := r.URL.Query().Get("group_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
			return
		}
		groupID = &id
	}
	messages, err := h.messages.Favorite(r.Context(), claims.UserID, peerID, groupID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (h *MessageHandler) setFlag(w http.ResponseWriter, r *http.Request, update func(context.Context, int64, int64, bool) error) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	messageID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || messageID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	var input messageFlagRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := update(r.Context(), claims.UserID, messageID, input.Enabled); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "enabled": input.Enabled})
}

type readReceiptRequest struct {
	PeerID    int64 `json:"peer_id"`
	GroupID   int64 `json:"group_id"`
	MessageID int64 `json:"message_id"`
}

func (h *MessageHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input readReceiptRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.messages.MarkRead(r.Context(), claims.UserID, input.PeerID, input.GroupID, input.MessageID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
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
