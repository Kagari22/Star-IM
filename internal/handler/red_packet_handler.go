package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"IM_Chat_System/internal/httpx"
	"IM_Chat_System/internal/service"
)

type RedPacketHandler struct {
	red      *service.RedPacketService
	messages *service.MessageService
}

func NewRedPacketHandler(red *service.RedPacketService, messages *service.MessageService) *RedPacketHandler {
	return &RedPacketHandler{red: red, messages: messages}
}

// GET /api/balance：返回当前用户余额（分）。
func (h *RedPacketHandler) Balance(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	balance, err := h.red.Balance(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"balance": balance})
}

// 兑换码充值接口
type redeemRequest struct {
	Code string `json:"code"`
}

// POST /api/redeem：兑换码兑换，返回兑换金额。
func (h *RedPacketHandler) Redeem(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input redeemRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	amount, err := h.red.Redeem(r.Context(), claims.UserID, input.Code)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"amount": amount})
}

type sendRedPacketRequest struct {
	Amount   int64  `json:"amount"` // 总金额，分
	Count    int    `json:"count"`
	Lucky    bool   `json:"lucky"`
	Greeting string `json:"greeting"`
}

// POST /api/groups/:id/red-packets：在群内发红包。
func (h *RedPacketHandler) SendGroupPacket(w http.ResponseWriter, r *http.Request) {
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
	var input sendRedPacketRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	packet, err := h.red.SendGroupPacket(r.Context(), claims.UserID, groupID, input.Amount, input.Count, input.Lucky, input.Greeting)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 把红包作为一条群消息推送出去，让群成员实时看到红包卡片。
	if _, err := h.messages.SaveGroupRedPacket(r.Context(), claims.UserID, groupID, packet.ID, packet.Greeting); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"packet": packet})
}

// POST /api/red-packets/:id/grab：抢红包。
func (h *RedPacketHandler) Grab(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	packetID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || packetID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid packet_id")
		return
	}
	receipt, err := h.red.Grab(r.Context(), claims.UserID, packetID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"receipt": receipt})
}

// GET /api/red-packets/:id：红包详情（含领取记录）。
func (h *RedPacketHandler) Detail(w http.ResponseWriter, r *http.Request) {
	packetID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || packetID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid packet_id")
		return
	}
	packet, receipts, err := h.red.Detail(r.Context(), packetID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"packet":   packet,
		"receipts": receipts,
	})
}
