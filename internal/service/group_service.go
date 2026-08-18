package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/repository"
	"IM_Chat_System/internal/storage"
)

type GroupService struct {
	groups   repository.GroupRepository
	users    repository.UserRepository
	uploader storage.Uploader
}

func NewGroupService(groups repository.GroupRepository, users repository.UserRepository, uploader storage.Uploader) *GroupService {
	if uploader == nil {
		uploader = storage.NoopUploader{}
	}
	return &GroupService{groups: groups, users: users, uploader: uploader}
}

const maxGroupMembers = 200

// 校验群名、成员 ID、用户是否存在、去重、人数上限; 再调用 Repository 建群
func (s *GroupService) Create(ctx context.Context, ownerID int64, name string, membersIDs []int64) (model.Group, error) {
	name = strings.TrimSpace(name)
	if ownerID <= 0 {
		return model.Group{}, errors.New("invalid owner")
	}
	if name == "" {
		return model.Group{}, errors.New("group name is required")
	}
	if utf8.RuneCountInString(name) > 64 {
		return model.Group{}, errors.New("group name must be at most 64 characters")
	}

	// 去重
	seen := make(map[int64]struct{})
	normalizeMemberIDs := make([]int64, 0, len(membersIDs))

	for _, userID := range membersIDs {
		if userID <= 0 {
			return model.Group{}, errors.New("member_id must be positive")
		}
		if userID == ownerID {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		if _, exists, err := s.users.GetByID(ctx, userID); err != nil {
			return model.Group{}, err
		} else if !exists {
			return model.Group{}, fmt.Errorf("member %d not found", userID)
		}
		seen[userID] = struct{}{}
		normalizeMemberIDs = append(normalizeMemberIDs, userID)
	}

	if len(normalizeMemberIDs)+1 > maxGroupMembers {
		return model.Group{}, fmt.Errorf("a group can have at most %d members", maxGroupMembers)
	}

	return s.groups.Create(ctx, ownerID, name, normalizeMemberIDs)
}

// 让当前用户查询自己加入的群
func (s *GroupService) ListMine(ctx context.Context, userID int64) ([]model.Group, error) {
	groups, err := s.groups.ListForUser(ctx, userID)
	if err != nil {
		return groups, err
	}
	for i := range groups {
		s.enrichAvatarURL(ctx, &groups[i])
	}
	return groups, nil
}

func (s *GroupService) enrichAvatarURL(ctx context.Context, group *model.Group) {
	if group.AvatarKey == "" {
		return
	}
	url, err := s.uploader.PresignGet(ctx, group.AvatarKey)
	if err != nil {
		log.Printf("presign group avatar %s: %v", group.AvatarKey, err)
		return
	}
	group.AvatarURL = url
}

func (s *GroupService) UploadAvatar(ctx context.Context, requesterID, groupID int64, reader io.Reader, size int64, contentType string) (model.Group, error) {
	group, ok, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return model.Group{}, err
	}
	if !ok {
		return model.Group{}, errors.New("group not found")
	}
	if group.OwnerID != requesterID {
		return model.Group{}, errors.New("only the group owner can set avatar")
	}
	objectKey := "groups/" + strconv.FormatInt(groupID, 10)
	if _, err := s.uploader.Upload(ctx, objectKey, reader, size, contentType); err != nil {
		return model.Group{}, err
	}
	if err := s.groups.UpdateAvatarKey(ctx, groupID, objectKey); err != nil {
		return model.Group{}, err
	}
	group.AvatarKey = objectKey
	s.enrichAvatarURL(ctx, &group)
	return group, nil
}

func (s *GroupService) InviteMember(ctx context.Context, requesterID, groupID, userID int64) error {
	group, ok, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("group not found")
	}
	if group.OwnerID != requesterID {
		return errors.New("only the group owner can invite members")
	}
	if userID <= 0 {
		return errors.New("invalid user id")
	}
	if _, ok, err := s.users.GetByID(ctx, userID); err != nil {
		return err
	} else if !ok {
		return errors.New("user not found")
	}
	if _, isMember, err := s.groups.IsMember(ctx, groupID, userID); err != nil {
		return err
	} else if isMember {
		return errors.New("user is already a member")
	}
	return s.groups.AddMember(ctx, groupID, userID)
}

func (s *GroupService) RemoveMember(ctx context.Context, requesterID, groupID, userID int64) error {
	group, ok, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("group not found")
	}
	if group.OwnerID != requesterID {
		return errors.New("only the group owner can remove members")
	}
	if userID == requesterID {
		return errors.New("cannot remove yourself as owner")
	}
	if _, isMember, err := s.groups.IsMember(ctx, groupID, userID); err != nil {
		return err
	} else if !isMember {
		return errors.New("user is not a member")
	}
	return s.groups.RemoveMember(ctx, groupID, userID)
}

func (s *GroupService) Dissolve(ctx context.Context, requesterID, groupID int64) error {
	group, ok, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("group not found")
	}
	if group.OwnerID != requesterID {
		return errors.New("only the group owner can dissolve the group")
	}
	return s.groups.Dissolve(ctx, groupID)
}

// 先确认请求者是群成员, 才允许查询成员列表
func (s *GroupService) Members(ctx context.Context, requesterID, groupID int64) ([]model.GroupMember, error) {
	if groupID <= 0 {
		return nil, errors.New("invalid group_id")
	}
	_, isMember, err := s.groups.IsMember(ctx, groupID, requesterID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errors.New("you are not a member of this group")
	}

	return s.groups.ListMembers(ctx, groupID)
}

func (s *GroupService) UpdateAnnouncement(ctx context.Context, requesterID, groupID int64, announcement string) (model.Group, error) {
	announcement = strings.TrimSpace(announcement)
	if len([]rune(announcement)) > 500 {
		return model.Group{}, errors.New("announcement must be at most 500 characters")
	}
	if err := s.requireManager(ctx, requesterID, groupID); err != nil {
		return model.Group{}, err
	}
	return s.groups.UpdateAnnouncement(ctx, groupID, requesterID, announcement)
}

func (s *GroupService) SetAllMuted(ctx context.Context, requesterID, groupID int64, muted bool) (model.Group, error) {
	if err := s.requireManager(ctx, requesterID, groupID); err != nil {
		return model.Group{}, err
	}
	return s.groups.SetAllMuted(ctx, groupID, requesterID, muted)
}

func (s *GroupService) SetMemberRole(ctx context.Context, requesterID, groupID, userID int64, role model.GroupRole) error {
	group, ok, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("group not found")
	}
	if group.OwnerID != requesterID {
		return errors.New("only the group owner can manage administrators")
	}
	if role != model.GroupRoleAdmin && role != model.GroupRoleMember {
		return errors.New("invalid group role")
	}
	if userID == requesterID {
		return errors.New("cannot change owner role")
	}
	return s.groups.SetMemberRole(ctx, groupID, userID, role)
}

func (s *GroupService) MuteMember(ctx context.Context, requesterID, groupID, userID int64, duration time.Duration) error {
	if duration < 0 || duration > 7*24*time.Hour {
		return errors.New("mute duration must be between 0 and 7 days")
	}
	if err := s.requireManager(ctx, requesterID, groupID); err != nil {
		return err
	}
	if userID == requesterID {
		return errors.New("cannot mute yourself")
	}
	var until *time.Time
	if duration > 0 {
		value := time.Now().Add(duration)
		until = &value
	}
	return s.groups.SetMemberMutedUntil(ctx, groupID, userID, until)
}

func (s *GroupService) Leave(ctx context.Context, userID, groupID int64) error {
	member, ok, err := s.groups.IsMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("you are not a member of this group")
	}
	if member.Role == model.GroupRoleOwner {
		return errors.New("the owner must transfer ownership or dissolve the group")
	}
	return s.groups.RemoveMember(ctx, groupID, userID)
}

func (s *GroupService) requireManager(ctx context.Context, userID, groupID int64) error {
	member, ok, err := s.groups.IsMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("you are not a member of this group")
	}
	if member.Role != model.GroupRoleOwner && member.Role != model.GroupRoleAdmin {
		return errors.New("group manager permission required")
	}
	return nil
}
