package handler

import (
	"encoding/json"
	"net/http"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/service"
)

type AuthHandler struct {
	auth *service.AuthService
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

/*
接收浏览器发来的注册信息
→ 解析 JSON
→ 调用注册业务逻辑
→ 返回成功或失败结果
*/
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	user, err := h.auth.Register(r.Context(), req.Username, req.Password, req.Nickname)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"user": user})
}

/*
客户端发送用户名、密码
→ 解析 JSON
→ 调用 AuthService 登录
→ 验证用户名和密码
→ 生成 JWT
→ 返回 Token 和用户信息
*/
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	token, user, err := h.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}
