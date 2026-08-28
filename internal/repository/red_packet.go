package repository

import (
	"context"
	"errors"

	"IM_Chat_System/internal/model"
)

// 红包相关的领域错误，service 层据此区分业务失败原因。
var (
	ErrRedeemCodeInvalid   = errors.New("兑换码无效或已被使用")
	ErrInsufficientBalance = errors.New("余额不足")
	ErrPacketNotFound      = errors.New("红包不存在")
	ErrPacketEmpty         = errors.New("红包已被抢完")
	ErrAlreadyGrabbed      = errors.New("你已经抢过这个红包")
)

type RedPacketRepository interface {
	// GetBalance 返回用户余额（分）。
	GetBalance(ctx context.Context, userID int64) (int64, error)
	// RedeemCode 原子地兑换一个兑换码，把金额加到用户余额，返回兑换到的金额（分）。
	RedeemCode(ctx context.Context, userID int64, code string) (int64, error)
	// CreatePacket 原子地扣除发送者余额并创建红包。
	CreatePacket(ctx context.Context, senderID int64, groupID, receiverID *int64, totalAmount int64, count int, lucky bool, greeting string) (model.RedPacket, error)
	// GetPacket 查询红包。
	GetPacket(ctx context.Context, packetID int64) (model.RedPacket, bool, error)
	// Grab 抢红包：行锁 + 随机金额 + 唯一约束防重复，返回本次领取记录。
	Grab(ctx context.Context, packetID, userID int64) (model.RedPacketReceipt, error)
	// ListReceipts 列出某红包的所有领取记录。
	ListReceipts(ctx context.Context, packetID int64) ([]model.RedPacketReceipt, error)
}
