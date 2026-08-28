package service

import (
	"context"
	"errors"

	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/repository"
)

// RedPacketService 提供余额、兑换码与抢红包的业务能力。
type RedPacketService struct {
	red    repository.RedPacketRepository
	groups repository.GroupRepository
}

func NewRedPacketService(red repository.RedPacketRepository, groups repository.GroupRepository) *RedPacketService {
	return &RedPacketService{red: red, groups: groups}
}

// Balance 返回用户余额（分）。
func (s *RedPacketService) Balance(ctx context.Context, userID int64) (int64, error) {
	return s.red.GetBalance(ctx, userID)
}

// Redeem 兑换一个兑换码，返回兑换到的金额（分）。
func (s *RedPacketService) Redeem(ctx context.Context, userID int64, code string) (int64, error) {
	if code == "" {
		return 0, errors.New("兑换码不能为空")
	}
	return s.red.RedeemCode(ctx, userID, code)
}

// SendGroupPacket 在群内发红包。amount 为总金额（分），count 为红包个数。
func (s *RedPacketService) SendGroupPacket(ctx context.Context, userID, groupID int64, amount int64, count int, lucky bool, greeting string) (model.RedPacket, error) {
	if amount <= 0 || count <= 0 {
		return model.RedPacket{}, errors.New("金额和个数必须大于 0")
	}
	if int64(count) > amount {
		return model.RedPacket{}, errors.New("红包金额至少每个 1 分")
	}
	if _, ok, err := s.groups.IsMember(ctx, groupID, userID); err != nil {
		return model.RedPacket{}, err
	} else if !ok {
		return model.RedPacket{}, errors.New("你不是该群成员")
	}
	return s.red.CreatePacket(ctx, userID, &groupID, nil, amount, count, lucky, greeting)
}

// Grab 抢红包。返回本次抢到的金额（分）。
func (s *RedPacketService) Grab(ctx context.Context, userID, packetID int64) (model.RedPacketReceipt, error) {
	packet, ok, err := s.red.GetPacket(ctx, packetID)
	if err != nil {
		return model.RedPacketReceipt{}, err
	}
	if !ok {
		return model.RedPacketReceipt{}, repository.ErrPacketNotFound
	}
	// 群红包需校验成员身份。
	if packet.GroupID != nil {
		if _, ok, err := s.groups.IsMember(ctx, *packet.GroupID, userID); err != nil {
			return model.RedPacketReceipt{}, err
		} else if !ok {
			return model.RedPacketReceipt{}, errors.New("你不是该群成员")
		}
	}
	return s.red.Grab(ctx, packetID, userID)
}

// Detail 返回红包及其领取记录。
func (s *RedPacketService) Detail(ctx context.Context, packetID int64) (model.RedPacket, []model.RedPacketReceipt, error) {
	packet, ok, err := s.red.GetPacket(ctx, packetID)
	if err != nil {
		return model.RedPacket{}, nil, err
	}
	if !ok {
		return model.RedPacket{}, nil, repository.ErrPacketNotFound
	}
	receipts, err := s.red.ListReceipts(ctx, packetID)
	if err != nil {
		return model.RedPacket{}, nil, err
	}
	return packet, receipts, nil
}
