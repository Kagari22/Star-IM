package handler

import (
	"net/http"
	"time"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/tokenblacklist"
)

type LogoutHandler struct {
	blacklist tokenblacklist.Store
}

func NewLogoutHandler(blacklist tokenblacklist.Store) *LogoutHandler {
	return &LogoutHandler{blacklist: blacklist}
}

/*
认证中间件验证 JWT
→ 从 Context 取出用户信息和原始 Token
→ 把 Token 写入 Redis 黑名单
→ TTL 设置为 Token 剩余有效期
→ 返回退出成功
*/
func (h *LogoutHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// 从请求 Context 中取出 JWT 解析后的身份信息
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	// 从 Context 中取出原始 JWT 字符串
	token, ok := TokenFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	if h.blacklist == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// 计算 Token 还剩多久过期
	ttl := time.Until(time.Unix(claims.Expires, 0))
	// 把 Token 写入 Redis 黑名单
	if err := h.blacklist.Blacklist(r.Context(), token, ttl); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "logout failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
