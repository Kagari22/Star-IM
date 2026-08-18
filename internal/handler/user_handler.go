package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/service"
)

type UserHandler struct {
	messages *service.MessageService
}

type updateNicknameRequest struct {
	Nickname string `json:"nickname"`
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

func (h *UserHandler) UpdateNickname(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var req updateNicknameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	user, err := h.messages.UpdateNickname(r.Context(), claims.UserID, req.Nickname)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any {
		"user": user,
	})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *UserHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "avatar file is required")
		return
	}
	defer file.Close()

	if header.Size <= 0 || header.Size > 2*1024*1024 {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "avatar must be under 2 MB")
		return
	}

	peek := make([]byte, 512)
	n, _ := io.ReadFull(file, peek)
	contentType := http.DetectContentType(peek[:n])
	if !strings.HasPrefix(contentType, "image/") {
		httpx.WriteError(w, http.StatusUnsupportedMediaType, "only image files allowed")
		return
	}
	reader := io.MultiReader(bytes.NewReader(peek[:n]), file)

	user, err := h.messages.SaveAvatar(r.Context(), claims.UserID, reader, header.Size, contentType)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := h.messages.ChangePassword(r.Context(), claims.UserID, req.OldPassword, req.NewPassword); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
