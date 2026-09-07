package repository

import (
	"context"
	"time"

	"IM_Chat_System/internal/model"
)

type UserRepository interface {
	Create(ctx context.Context, username, passwordHash, nickname string) (model.User, error)
	GetByID(ctx context.Context, id int64) (model.User, bool, error)
	GetByUsername(ctx context.Context, username string) (model.User, bool, error)
	List(ctx context.Context, excludeUserID int64) ([]model.User, error)
	SearchUsers(ctx context.Context, query string, excludeUserID int64, limit int) ([]model.User, error)
	UpdateNickname(ctx context.Context, id int64, nickname string) (model.User, bool, error)
	UpdateAvatarKey(ctx context.Context, id int64, avatarKey string) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
}

type MessageRepository interface {
	SaveAndEnqueue(ctx context.Context, message model.Message) (model.Message, error)
	ListConversation(ctx context.Context, userID, peerID, afterID int64, limit int) ([]model.Message, error)
	ListRecentConversation(ctx context.Context, userID, peerID int64, limit int) ([]model.Message, error)
	ListOffline(ctx context.Context, userID, afterID int64, limit int) ([]model.Message, error)
	ListGroupConversation(ctx context.Context, userID, groupID, afterID int64, limit int) ([]model.Message, error)
	DeleteConversationMessages(ctx context.Context, userID int64, peerID, groupID *int64, messageIDs []int64, all bool) (int64, error)
	Recall(ctx context.Context, messageID, requesterID int64, deadline time.Time) (model.Message, error)
	Edit(ctx context.Context, messageID, requesterID int64, content string) (model.Message, error)
	SetFavorite(ctx context.Context, userID, messageID int64, favorite bool) error
	ListFavorite(ctx context.Context, userID int64, peerID, groupID *int64) ([]model.Message, error)
	MarkRead(ctx context.Context, userID, peerID, groupID, messageID int64) error
}

type GroupRepository interface {
	Create(ctx context.Context, ownerID int64, name string, memberIDs []int64) (model.Group, error)
	ListForUser(ctx context.Context, userID int64) ([]model.Group, error)
	GetByID(ctx context.Context, groupID int64) (model.Group, bool, error)
	IsMember(ctx context.Context, groupID, userID int64) (model.GroupMember, bool, error)
	ListMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error)
	SearchGroups(ctx context.Context, query string, limit int) ([]model.Group, error)
	UpdateAvatarKey(ctx context.Context, groupID int64, avatarKey string) error
	AddMember(ctx context.Context, groupID, userID int64) error
	RemoveMember(ctx context.Context, groupID, userID int64) error
	Dissolve(ctx context.Context, groupID int64) error
	UpdateAnnouncement(ctx context.Context, groupID, requesterID int64, announcement string) (model.Group, error)
	SetAllMuted(ctx context.Context, groupID, requesterID int64, muted bool) (model.Group, error)
	SetMemberRole(ctx context.Context, groupID, userID int64, role model.GroupRole) error
	SetMemberMutedUntil(ctx context.Context, groupID, userID int64, until *time.Time) error
}

type SocialRepository interface {
	ListFriends(ctx context.Context, userID int64) ([]model.User, error)
	IsFriend(ctx context.Context, userID, peerID int64) (bool, error)
	HasPendingFriendRequest(ctx context.Context, fromUserID, toUserID int64) (bool, error)
	CreateFriendRequest(ctx context.Context, fromUserID, toUserID int64) (model.FriendRequest, error)
	ListFriendRequests(ctx context.Context, userID int64) ([]model.FriendRequest, error)
	RespondFriendRequest(ctx context.Context, requestID, userID int64, accept bool) error
	CreateGroupJoinRequest(ctx context.Context, userID, groupID int64) (model.GroupJoinRequest, error)
	HasPendingGroupJoinRequest(ctx context.Context, userID, groupID int64) (bool, error)
	ListGroupJoinRequests(ctx context.Context, ownerID int64) ([]model.GroupJoinRequest, error)
	RespondGroupJoinRequest(ctx context.Context, requestID, ownerID int64, accept bool) error
	RemoveFriend(ctx context.Context, userID, friendID int64) error
	EnsureFriendship(ctx context.Context, userA, userB int64) error
	EnsureFriendshipForAll(ctx context.Context, botID int64) error
}
