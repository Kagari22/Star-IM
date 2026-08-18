package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/service"
)

type GroupHandler struct {
	groups   *service.GroupService
	messages *service.MessageService
}

// 创建 HTTP Handler，并注入 GroupService
func NewGroupHandler(groups *service.GroupService, messages *service.MessageService) *GroupHandler {
	return &GroupHandler{groups: groups, messages: messages}
}

type createGroupRequest struct {
	Name      string  `json:"name"`
	MemberIDs []int64 `json:"member_ids"`
}

// 处理 POST /api/groups: 解析 JSON、读取当前登录用户、调用 Service 建群
func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var input createGroupRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	group, err := h.groups.Create(r.Context(), claims.UserID, input.Name, input.MemberIDs)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"group": group,
	})
}

// 处理 GET /api/groups: 返回当前用户加入的群
func (h *GroupHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	groups, err := h.groups.ListMine(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"groups": groups,
	})
}

// 处理 GET /api/groups/:id/members: 读取群 ID、校验登录状态、返回成员
func (h *GroupHandler) Members(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}

	members, err := h.groups.Members(r.Context(), claims.UserID, groupID)
	if err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"members": members,
	})
}

// 处理客户端获取某个群聊历史消息的请求
func (h *GroupHandler) Messages(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after_id"), 10, 64)
	limit, err := parseBoundedLimit(r.URL.Query().Get("limit"), 100)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	messages, err := h.messages.GroupConversation(r.Context(), claims.UserID, groupID, afterID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"messages": messages,
	})
}

type sendGroupMessageRequest struct {
	Content string `json:"content"`
}

// 群消息 HTTP 接入层, 负责鉴权、参数解析和响应
func (h *GroupHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}

	var input sendGroupMessageRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	message, err := h.messages.SaveGroupText(r.Context(), claims.UserID, groupID, input.Content)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"message": message,
	})
}

func (h *GroupHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
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

	group, err := h.groups.UploadAvatar(r.Context(), claims.UserID, groupID, reader, header.Size, contentType)
	if err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"group": group})
}

type addMemberRequest struct {
	UserID int64 `json:"user_id"`
}

func (h *GroupHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}

	var input addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.groups.InviteMember(r.Context(), claims.UserID, groupID, input.UserID); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *GroupHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	if err := h.groups.RemoveMember(r.Context(), claims.UserID, groupID, userID); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *GroupHandler) Dissolve(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	if err := h.groups.Dissolve(r.Context(), claims.UserID, groupID); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

type announcementRequest struct {
	Announcement string `json:"announcement"`
}

func (h *GroupHandler) UpdateAnnouncement(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	var input announcementRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	group, err := h.groups.UpdateAnnouncement(r.Context(), claims.UserID, groupID, input.Announcement)
	if err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"group": group})
}

type allMutedRequest struct {
	Muted bool `json:"muted"`
}

func (h *GroupHandler) SetAllMuted(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	var input allMutedRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	group, err := h.groups.SetAllMuted(r.Context(), claims.UserID, groupID, input.Muted)
	if err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"group": group})
}

type memberRoleRequest struct {
	Role string `json:"role"`
}

func (h *GroupHandler) SetMemberRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	var input memberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.groups.SetMemberRole(r.Context(), claims.UserID, groupID, userID, model.GroupRole(input.Role)); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

type memberMuteRequest struct {
	Minutes int64 `json:"minutes"`
}

func (h *GroupHandler) MuteMember(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	var input memberMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.groups.MuteMember(r.Context(), claims.UserID, groupID, userID, time.Duration(input.Minutes)*time.Minute); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *GroupHandler) Leave(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid group_id")
		return
	}
	if err := h.groups.Leave(r.Context(), claims.UserID, groupID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
