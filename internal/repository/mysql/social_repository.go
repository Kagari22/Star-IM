package mysql

import (
	"context"
	"errors"
	"time"

	"IM_Chat_System/internal/model"
	"gorm.io/gorm"
)

type SocialRepository struct {
	db *gorm.DB
}

func NewSocialRepository(db *gorm.DB) *SocialRepository { return &SocialRepository{db: db} }

// 表示 friendships 表的一行数据
type friendshipRow struct {
	UserID    int64     `gorm:"column:user_id;primaryKey"`
	FriendID  int64     `gorm:"column:friend_id;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (friendshipRow) TableName() string { return "friendships" }

type friendRequestRow struct {
	ID          int64      `gorm:"column:id;primaryKey"`
	FromUserID  int64      `gorm:"column:from_user_id"`
	ToUserID    int64      `gorm:"column:to_user_id"`
	Status      string     `gorm:"column:status"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	RespondedAt *time.Time `gorm:"column:responded_at"`
}

func (friendRequestRow) TableName() string { return "friend_requests" }

func (r friendRequestRow) model() model.FriendRequest {
	return model.FriendRequest{ID: r.ID, FromUserID: r.FromUserID, ToUserID: r.ToUserID, Status: model.RequestStatus(r.Status), CreatedAt: r.CreatedAt, RespondedAt: r.RespondedAt}
}

type groupJoinRequestRow struct {
	ID          int64      `gorm:"column:id;primaryKey"`
	GroupID     int64      `gorm:"column:group_id"`
	UserID      int64      `gorm:"column:user_id"`
	Status      string     `gorm:"column:status"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	RespondedAt *time.Time `gorm:"column:responded_at"`
}

func (groupJoinRequestRow) TableName() string { return "group_join_requests" }

func (r groupJoinRequestRow) model() model.GroupJoinRequest {
	return model.GroupJoinRequest{ID: r.ID, GroupID: r.GroupID, UserID: r.UserID, Status: model.RequestStatus(r.Status), CreatedAt: r.CreatedAt, RespondedAt: r.RespondedAt}
}

func (r *SocialRepository) ListFriends(ctx context.Context, userID int64) ([]model.User, error) {
	var rows []userRow
	if err := r.db.WithContext(ctx).Model(&userRow{}).
		Joins("JOIN friendships ON friendships.friend_id = users.id").
		Where("friendships.user_id = ?", userID).
		Order("users.id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	users := make([]model.User, 0, len(rows))
	for _, row := range rows {
		user := row.model()
		user.PasswordHash = ""
		users = append(users, user)
	}
	return users, nil
}

func (r *SocialRepository) IsFriend(ctx context.Context, userID, peerID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("friendships").Where("user_id = ? AND friend_id = ?", userID, peerID).Count(&count).Error
	return count > 0, err
}

func (r *SocialRepository) HasPendingFriendRequest(ctx context.Context, fromUserID, toUserID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("friend_requests").Where("from_user_id = ? AND to_user_id = ? AND status = ?", fromUserID, toUserID, string(model.RequestPending)).Count(&count).Error
	return count > 0, err
}

func (r *SocialRepository) CreateFriendRequest(ctx context.Context, fromUserID, toUserID int64) (model.FriendRequest, error) {
	row := friendRequestRow{FromUserID: fromUserID, ToUserID: toUserID, Status: string(model.RequestPending)}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return model.FriendRequest{}, err
	}
	return row.model(), nil
}

func (r *SocialRepository) ListFriendRequests(ctx context.Context, userID int64) ([]model.FriendRequest, error) {
	var rows []friendRequestRow
	if err := r.db.WithContext(ctx).Where("to_user_id = ? AND status = ?", userID, string(model.RequestPending)).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]model.FriendRequest, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.model())
	}
	if len(result) == 0 {
		return result, nil
	}

	fromIDs := make([]int64, 0, len(result))
	for _, req := range result {
		fromIDs = append(fromIDs, req.FromUserID)
	}
	var userRows []userRow
	if err := r.db.WithContext(ctx).Where("id IN ?", fromIDs).Find(&userRows).Error; err != nil {
		return nil, err
	}
	nameByID := make(map[int64]userRow, len(userRows))
	for _, u := range userRows {
		nameByID[u.ID] = u
	}
	for i := range result {
		if from, ok := nameByID[result[i].FromUserID]; ok {
			result[i].FromUsername = from.Username
			result[i].FromNickname = from.Nickname
		}
	}
	return result, nil
}

func (r *SocialRepository) RespondFriendRequest(ctx context.Context, requestID, userID int64, accept bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var request friendRequestRow
		if err := tx.Where("id = ? AND to_user_id = ? AND status = ?", requestID, userID, string(model.RequestPending)).First(&request).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("friend request not found")
			}
			return err
		}
		now := time.Now()
		status := model.RequestRejected
		if accept {
			status = model.RequestAccepted
			for _, pair := range [][2]int64{{request.FromUserID, request.ToUserID}, {request.ToUserID, request.FromUserID}} {
				friend := friendshipRow{UserID: pair[0], FriendID: pair[1], CreatedAt: now}
				if err := tx.Where("user_id = ? AND friend_id = ?", pair[0], pair[1]).FirstOrCreate(&friend).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&friendRequestRow{}).Where("id = ?", requestID).Updates(map[string]any{"status": string(status), "responded_at": now}).Error
	})
}

func (r *SocialRepository) CreateGroupJoinRequest(ctx context.Context, userID, groupID int64) (model.GroupJoinRequest, error) {
	row := groupJoinRequestRow{GroupID: groupID, UserID: userID, Status: string(model.RequestPending)}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return model.GroupJoinRequest{}, err
	}
	return row.model(), nil
}

func (r *SocialRepository) HasPendingGroupJoinRequest(ctx context.Context, userID, groupID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("group_join_requests").Where("user_id = ? AND group_id = ? AND status = ?", userID, groupID, string(model.RequestPending)).Count(&count).Error
	return count > 0, err
}

func (r *SocialRepository) ListGroupJoinRequests(ctx context.Context, ownerID int64) ([]model.GroupJoinRequest, error) {
	var rows []groupJoinRequestRow
	if err := r.db.WithContext(ctx).Table("group_join_requests").Joins("JOIN chat_groups ON chat_groups.id = group_join_requests.group_id").Where("chat_groups.owner_id = ? AND group_join_requests.status = ?", ownerID, string(model.RequestPending)).Order("group_join_requests.created_at ASC, group_join_requests.id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]model.GroupJoinRequest, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.model())
	}
	if len(result) == 0 {
		return result, nil
	}

	groupIDs := make([]int64, 0, len(result))
	userIDs := make([]int64, 0, len(result))
	for _, req := range result {
		groupIDs = append(groupIDs, req.GroupID)
		userIDs = append(userIDs, req.UserID)
	}

	var groupRows []groupRow
	if err := r.db.WithContext(ctx).Where("id IN ?", groupIDs).Find(&groupRows).Error; err != nil {
		return nil, err
	}
	groupNameByID := make(map[int64]string, len(groupRows))
	for _, g := range groupRows {
		groupNameByID[g.ID] = g.Name
	}

	var userRows []userRow
	if err := r.db.WithContext(ctx).Where("id IN ?", userIDs).Find(&userRows).Error; err != nil {
		return nil, err
	}
	nameByID := make(map[int64]userRow, len(userRows))
	for _, u := range userRows {
		nameByID[u.ID] = u
	}

	for i := range result {
		result[i].GroupName = groupNameByID[result[i].GroupID]
		if applicant, ok := nameByID[result[i].UserID]; ok {
			result[i].Username = applicant.Username
			result[i].Nickname = applicant.Nickname
		}
	}
	return result, nil
}

func (r *SocialRepository) RespondGroupJoinRequest(ctx context.Context, requestID, ownerID int64, accept bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var request groupJoinRequestRow
		if err := tx.Table("group_join_requests").Joins("JOIN chat_groups ON chat_groups.id = group_join_requests.group_id").Where("group_join_requests.id = ? AND chat_groups.owner_id = ? AND group_join_requests.status = ?", requestID, ownerID, string(model.RequestPending)).First(&request).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("group join request not found")
			}
			return err
		}
		now := time.Now()
		status := model.RequestRejected
		if accept {
			status = model.RequestAccepted
			member := groupMemberRow{GroupID: request.GroupID, UserID: request.UserID, Role: string(model.GroupRoleMember), JoinedAt: now}
			if err := tx.Where("group_id = ? AND user_id = ?", request.GroupID, request.UserID).FirstOrCreate(&member).Error; err != nil {
				return err
			}
		}
		return tx.Model(&groupJoinRequestRow{}).Where("id = ?", requestID).Updates(map[string]any{"status": string(status), "responded_at": now}).Error
	})
}

func (r *SocialRepository) RemoveFriend(ctx context.Context, userID, friendID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND friend_id = ?", userID, friendID).Delete(&friendshipRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND friend_id = ?", friendID, userID).Delete(&friendshipRow{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// EnsureFriendship 建立两个用户之间的双向好友关系，幂等，用于把 AI 机器人自动添加为新用户的好友。
func (r *SocialRepository) EnsureFriendship(ctx context.Context, userA, userB int64) error {
	if userA <= 0 || userB <= 0 || userA == userB {
		return errors.New("invalid friendship pair")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, pair := range [][2]int64{{userA, userB}, {userB, userA}} {
			friend := friendshipRow{UserID: pair[0], FriendID: pair[1], CreatedAt: time.Now()}
			if err := tx.Where("user_id = ? AND friend_id = ?", pair[0], pair[1]).FirstOrCreate(&friend).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// EnsureFriendshipForAll 把机器人加入所有已有用户的双向好友列表，幂等。
// 用于覆盖在 AI 功能启用之前就已注册的老账号。
func (r *SocialRepository) EnsureFriendshipForAll(ctx context.Context, botID int64) error {
	if botID <= 0 {
		return errors.New("invalid bot id")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT IGNORE INTO friendships (user_id, friend_id) SELECT id, ? FROM users WHERE id <> ?", botID, botID).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT IGNORE INTO friendships (user_id, friend_id) SELECT ?, id FROM users WHERE id <> ?", botID, botID).Error; err != nil {
			return err
		}
		return nil
	})
}
