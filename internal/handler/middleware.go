package handler

import (
	"context"
	"net"
	"net/http"
	"time"

	"IM_Chat_System/internal/auth"
	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/ratelimit"
	"IM_Chat_System/internal/tokenblacklist"
)

type contextKey string

const claimsKey contextKey = "claims"
const tokenKey contextKey = "token"

/*
→ 读取 Authorization Header
→ 检查 Token 是否在 Redis 黑名单
→ 校验 JWT 签名和过期时间
→ 把用户信息写进 Request Context
→ 调用真实 Handler
*/
func WithAuth(secret string, blacklist tokenblacklist.Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.BearerToken(r.Header.Get("Authorization"))
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "missing token")
			return
		}
		if blacklist != nil {
			blocked, err := blacklist.Contains(r.Context(), token)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "check token blacklist failed")
				return
			}
			if blocked {
				httpx.WriteError(w, http.StatusUnauthorized, "token has been revoked")
				return
			}
		}

		claims, err := auth.ParseToken(secret, token)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		ctx = context.WithValue(ctx, tokenKey, token)
		next(w, r.WithContext(ctx))
	}
}

// 从请求的 Context 中取出已经解析好的 JWT 用户信息
func ClaimsFromContext(ctx context.Context) (auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(auth.Claims) // JWT 解析出的用户信息
	return claims, ok
}

// 从 Context 中取出原始 JWT 字符串
func TokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(tokenKey).(string) // 原始 JWT 字符串
	return token, ok
}

// scope: 限流范围; limit: 窗口内最多允许多少次; window: 时间窗口; keyFn: 从请求中提取限流维度
func WithRateLimit(store ratelimit.Store, scope string, limit int64, window time.Duration, keyFn func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			next(w, r)
			return
		}
		key := scope + ":" + keyFn(r) // 如 login:192.168.1.10
		allowed, err := store.Allow(r.Context(), key, limit, window)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "rate limit check failed")
			return
		}
		if !allowed {
			httpx.WriteError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next(w, r)
	}
}

func ClientIPKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
