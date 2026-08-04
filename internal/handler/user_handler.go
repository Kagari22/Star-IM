package handler

import (
	"net/http"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/service"
)

type UserHandler struct {
	messages *service.MessageService
}

func NewUserHandler(messages *service.MessageService) *UserHandler {
	return &UserHandler{messages: messages}
}

// 获取当前登录用户个人信息
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	// 使用 JWT 中的 claims.UserID 查询当前用户信息
	user, err := h.messages.GetMe(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": user})
}

// 获取可聊天用户列表
func (h *UserHandler) Users(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	// 查询"除当前登录用户外的所有可聊天用户"
	users, err := h.messages.ListUsers(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": users})
}
