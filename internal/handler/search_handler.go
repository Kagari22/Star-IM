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

// SearchMessages returns messages visible to the current user. A search can
// target a private conversation (peer_id) or a group (group_id), but not both.
func (h *SearchHandler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	query := r.URL.Query().Get("q")
	if utf8.RuneCountInString(query) > 64 {
		httpx.WriteError(w, http.StatusBadRequest, "query must be at most 64 characters")
		return
	}
	peerID, _ := strconv.ParseInt(r.URL.Query().Get("peer_id"), 10, 64)
	groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
	if peerID > 0 && groupID > 0 {
		httpx.WriteError(w, http.StatusBadRequest, "peer_id and group_id cannot be used together")
		return
	}
	if groupID > 0 {
		allowed, err := h.messages.CanAccessGroup(r.Context(), claims.UserID, groupID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !allowed {
			httpx.WriteError(w, http.StatusForbidden, "you are not a member of this group")
			return
		}
	}

	limit, err := parseBoundedLimit(r.URL.Query().Get("limit"), 50)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	messages, err := h.indexer.SearchMessages(r.Context(), claims.UserID, query, peerID, groupID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	messages, err = h.messages.EnrichMessages(r.Context(), messages)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "media URL service unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages})
}
