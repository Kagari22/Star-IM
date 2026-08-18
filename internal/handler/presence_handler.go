package handler

import (
    "net/http"
    "strconv"

    "IM_Chat_System/internal/httpx"
    "IM_Chat_System/internal/service"
)

type PresenceHandler struct {
	presence *service.PresenceService
}

func NewPresenceHandler(presence *service.PresenceService) *PresenceHandler {
	return &PresenceHandler{presence: presence}
}

func (h *PresenceHandler) Status(w http.ResponseWriter, r *http.Request) {
	rawID := r.URL.Query().Get("user_id")
	userID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || userID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	online, err := h.presence.IsOnline(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "query presence failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any {
		"user_id": userID,
		"online": online,
	})
}