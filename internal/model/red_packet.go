package model

import "time"

// 红包相关金额一律以「分」为单位（int64），避免浮点误差。

type RedPacket struct {
	ID              int64     `json:"id"`
	SenderID        int64     `json:"sender_id"`
	GroupID         *int64    `json:"group_id,omitempty"`
	ReceiverID      *int64    `json:"receiver_id,omitempty"`
	TotalAmount     int64     `json:"total_amount"`
	TotalCount      int       `json:"total_count"`
	RemainingAmount int64     `json:"remaining_amount"`
	RemainingCount  int       `json:"remaining_count"`
	Lucky           bool      `json:"lucky"`
	Greeting        string    `json:"greeting"`
	CreatedAt       time.Time `json:"created_at"`
}

// RedPacketReceipt 是一次抢红包的领取记录。
type RedPacketReceipt struct {
	ID        int64     `json:"id"`
	PacketID  int64     `json:"packet_id"`
	UserID    int64     `json:"user_id"`
	Amount    int64     `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

// RedeemCode 是兑换码。
type RedeemCode struct {
	Code      string     `json:"code"`
	Amount    int64      `json:"amount"`
	UsedBy    *int64     `json:"used_by,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
