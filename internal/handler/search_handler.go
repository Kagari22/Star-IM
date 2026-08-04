package handler

import (
	"net/http"
	"strconv"
	"unicode/utf8"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/search"
	"IM_Chat_System/internal/service"
)

type SearchHandler struct {
	indexer  search.Indexer
	messages *service.MessageService
}

func NewSearchHandler(indexer search.Indexer, messages *service.MessageService) *SearchHandler {
	return &SearchHandler{indexer: indexer, messages: messages}
}

// 搜索当前用户有权限查看的聊天消息
func (h *SearchHandler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	query := r.URL.Query().Get("q")
	if utf8.RuneCountInString(query) > 64 { // 限制搜索关键词最多 64 个 Unicode 字符
		httpx.WriteError(w, http.StatusBadRequest, "query must be at most 64 characters")
		return
	}
	peerID, _ := strconv.ParseInt(r.URL.Query().Get("peer_id"), 10, 64) // 只搜索当前用户与 peerID 对应的用户的会话
	limit, err := parseBoundedLimit(r.URL.Query().Get("limit"), 50) // 解析结果数量, 默认 50、最多 50, 避免一次返回大量搜索结果
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	/*
	调用 Elasticsearch 搜索当前用户有权限查看的消息
	-> claims.UserID: 当前登录用户, 用于权限过滤
	-> query: 关键词
	-> peerID: 可选的会话限定
	-> limit: 结构数量上限
	*/
	messages, err := h.indexer.SearchMessages(r.Context(), claims.UserID, query, peerID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	messages, err = h.messages.EnrichMessages(r.Context(), messages) // 给搜索结果中的媒体消息补全可访问的文件 URL
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "media URL service unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages})
}
