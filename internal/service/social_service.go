package service

import (
	"context"
	"errors"

	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/repository"
)

type SocialService struct {
	users  repository.UserRepository
	groups repository.GroupRepository
	social repository.SocialRepository
}

func NewSocialService(users repository.UserRepository, groups repository.GroupRepository, social repository.SocialRepository) *SocialService {
	return &SocialService{users: users, groups: groups, social: social}
}

func (s *SocialService) ListFriends(ctx context.Context, userID int64) ([]model.User, error) {
	return s.social.ListFriends(ctx, userID)
}

func (s *SocialService) SearchUser(ctx context.Context, userID, targetID int64) (model.User, error) {
	if targetID <= 0 || targetID == userID {
		return model.User{}, errors.New("invalid user id")
	}
	user, ok, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return model.User{}, err
	}
	if !ok {
		return model.User{}, errors.New("user not found")
	}
	user.PasswordHash = ""
	return user, nil
}

func (s *SocialService) SearchUsersByKeyword(ctx context.Context, userID int64, query string, limit int) ([]model.User, error) {
	users, err := s.users.SearchUsers(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	for i := range users {
		users[i].PasswordHash = ""
	}
	return users, nil
}

func (s *SocialService) SearchGroupsByKeyword(ctx context.Context, query string, limit int) ([]model.Group, error) {
	return s.groups.SearchGroups(ctx, query, limit)
}

func (s *SocialService) SearchGroup(ctx context.Context, groupID int64) (model.Group, error) {
	if groupID <= 0 {
		return model.Group{}, errors.New("invalid group id")
	}
	group, ok, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return model.Group{}, err
	}
	if !ok {
		return model.Group{}, errors.New("group not found")
	}
	return group, nil
}

func (s *SocialService) SendFriendRequest(ctx context.Context, fromUserID, toUserID int64) (model.FriendRequest, error) {
	if toUserID <= 0 || fromUserID == toUserID {
		return model.FriendRequest{}, errors.New("invalid friend id")
	}
	if _, ok, err := s.users.GetByID(ctx, toUserID); err != nil {
		return model.FriendRequest{}, err
	} else if !ok {
		return model.FriendRequest{}, errors.New("user not found")
	}
	friends, err := s.social.IsFriend(ctx, fromUserID, toUserID)
	if err != nil {
		return model.FriendRequest{}, err
	}
	if friends {
		return model.FriendRequest{}, errors.New("already friends")
	}
	pending, err := s.social.HasPendingFriendRequest(ctx, fromUserID, toUserID)
	if err != nil {
		return model.FriendRequest{}, err
	}
	if pending {
		return model.FriendRequest{}, errors.New("friend request already pending")
	}
	return s.social.CreateFriendRequest(ctx, fromUserID, toUserID)
}

func (s *SocialService) FriendRequests(ctx context.Context, userID int64) ([]model.FriendRequest, error) {
	return s.social.ListFriendRequests(ctx, userID)
}

func (s *SocialService) RespondFriendRequest(ctx context.Context, requestID, userID int64, accept bool) error {
	if requestID <= 0 {
		return errors.New("invalid request id")
	}
	return s.social.RespondFriendRequest(ctx, requestID, userID, accept)
}

func (s *SocialService) RequestGroupJoin(ctx context.Context, userID, groupID int64) (model.GroupJoinRequest, error) {
	group, ok, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return model.GroupJoinRequest{}, err
	}
	if !ok {
		return model.GroupJoinRequest{}, errors.New("group not found")
	}
	if _, member, err := s.groups.IsMember(ctx, groupID, userID); err != nil {
		return model.GroupJoinRequest{}, err
	} else if member {
		return model.GroupJoinRequest{}, errors.New("already a group member")
	}
	pending, err := s.social.HasPendingGroupJoinRequest(ctx, userID, groupID)
	if err != nil {
		return model.GroupJoinRequest{}, err
	}
	if pending {
		return model.GroupJoinRequest{}, errors.New("group join request already pending")
	}
	return s.social.CreateGroupJoinRequest(ctx, userID, group.ID)
}

func (s *SocialService) GroupJoinRequests(ctx context.Context, ownerID int64) ([]model.GroupJoinRequest, error) {
	return s.social.ListGroupJoinRequests(ctx, ownerID)
}

func (s *SocialService) RespondGroupJoinRequest(ctx context.Context, requestID, ownerID int64, accept bool) error {
	if requestID <= 0 {
		return errors.New("invalid request id")
	}
	return s.social.RespondGroupJoinRequest(ctx, requestID, ownerID, accept)
}

func (s *SocialService) RemoveFriend(ctx context.Context, userID, friendID int64) error {
	if friendID <= 0 {
		return errors.New("invalid friend id")
	}
	friends, err := s.social.IsFriend(ctx, userID, friendID)
	if err != nil {
		return err
	}
	if !friends {
		return errors.New("not your friend")
	}
	return s.social.RemoveFriend(ctx, userID, friendID)
}
