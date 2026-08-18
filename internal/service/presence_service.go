package service

import (
	"context"
	"IM_Chat_System/internal/presence"
)

type PresenceService struct {
	store presence.Store
}

func NewPresenceService(store presence.Store) *PresenceService {
	return &PresenceService{store: store}
}

func (s *PresenceService) IsOnline(ctx context.Context, userID int64) (bool, error) {
	_, online, err := s.store.GetOnlineNode(ctx, userID)
	return online, err
}