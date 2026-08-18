package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/service"
)

type SocialHandler struct{ social *service.SocialService }

func NewSocialHandler(social *service.SocialService) *SocialHandler {
	return &SocialHandler{social: social}
}

func (h *SocialHandler) SearchUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	user, err := h.social.SearchUser(r.Context(), claims.UserID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *SocialHandler) SearchGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := ClaimsFromContext(r.Context()); !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	group, err := h.social.SearchGroup(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"group": group})
}

// 按名字/昵称/ID 搜索用户或群聊
func (h *SocialHandler) SearchDirectory(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	searchType := r.URL.Query().Get("type")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if searchType != "user" && searchType != "group" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid type, must be user or group")
		return
	}
	if query == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing query q")
		return
	}
	limit, err := parseBoundedLimit(r.URL.Query().Get("limit"), 20)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if searchType == "user" {
		users, err := h.social.SearchUsersByKeyword(r.Context(), claims.UserID, query, limit)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"type": "user", "users": users})
		return
	}
	groups, err := h.social.SearchGroupsByKeyword(r.Context(), query, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"type": "group", "groups": groups})
}

type friendRequestInput struct {
	ToUserID int64 `json:"to_user_id"`
}

func (h *SocialHandler) CreateFriendRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input friendRequestInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	request, err := h.social.SendFriendRequest(r.Context(), claims.UserID, input.ToUserID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"request": request})
}

func (h *SocialHandler) FriendRequests(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	requests, err := h.social.FriendRequests(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"requests": requests})
}

type respondRequestInput struct {
	Accept bool `json:"accept"`
}

func (h *SocialHandler) RespondFriendRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request_id")
		return
	}
	var input respondRequestInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.social.RespondFriendRequest(r.Context(), id, claims.UserID, input.Accept); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

type groupJoinRequestInput struct {
	GroupID int64 `json:"group_id"`
}

func (h *SocialHandler) CreateGroupJoinRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input groupJoinRequestInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	request, err := h.social.RequestGroupJoin(r.Context(), claims.UserID, input.GroupID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"request": request})
}

func (h *SocialHandler) GroupJoinRequests(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	requests, err := h.social.GroupJoinRequests(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"requests": requests})
}

func (h *SocialHandler) RespondGroupJoinRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request_id")
		return
	}
	var input respondRequestInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.social.RespondGroupJoinRequest(r.Context(), id, claims.UserID, input.Accept); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *SocialHandler) RemoveFriend(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	friendID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid friend_id")
		return
	}
	if err := h.social.RemoveFriend(r.Context(), claims.UserID, friendID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
