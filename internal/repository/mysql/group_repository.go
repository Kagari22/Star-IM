package mysql

import (
	"IM_Chat_System/internal/model"
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type GroupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

// 事务创建 chat_groups, 同时把群主和初始成员写入 group_members
func (r *GroupRepository) Create(ctx context.Context, ownerID int64, name string, memberIDs []int64) (model.Group, error) {
	var group groupRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		group = groupRow{
			Name:      name,
			OwnerID:   ownerID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		members := make([]groupMemberRow, 0, len(memberIDs)+1)
		members = append(members, groupMemberRow{
			GroupID:  group.ID,
			UserID:   ownerID,
			Role:     string(model.GroupRoleOwner),
			JoinedAt: now,
		})
		for _, userID := range memberIDs {
			members = append(members, groupMemberRow{
				GroupID:  group.ID,
				UserID:   userID,
				Role:     string(model.GroupRoleMember),
				JoinedAt: now,
			})
		}
		return tx.Create(&members).Error
	})
	if err != nil {
		return model.Group{}, err
	}
	return group.model(), nil
}

// 联查 chat_groups 与 group_members，返回"我加入的群"
func (r *GroupRepository) ListForUser(ctx context.Context, userID int64) ([]model.Group, error) {
	var rows []groupRow

	err := r.db.WithContext(ctx).Model(&groupRow{}).
		Joins("JOIN group_members ON group_members.group_id = chat_groups.id").
		Where("group_members.user_id = ?", userID).
		Order("chat_groups.updated_at DESC, chat_groups.id DESC").
		Find(&rows).Error

	if err != nil {
		return nil, err
	}

	groups := make([]model.Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, row.model())
	}
	return groups, nil
}

func (r *GroupRepository) GetByID(ctx context.Context, groupID int64) (model.Group, bool, error) {
	var row groupRow
	err := r.db.WithContext(ctx).First(&row, groupID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Group{}, false, nil
	}
	if err != nil {
		return model.Group{}, false, err
	}
	return row.model(), true, nil
}

// 从 group_members 查 (group_id, user_id) 是否存在
func (r *GroupRepository) IsMember(ctx context.Context, groupID, userID int64) (model.GroupMember, bool, error) {
	var row groupMemberRow
	err := r.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.GroupMember{}, false, nil
	}
	if err != nil {
		return model.GroupMember{}, false, err
	}
	return row.model(), true, nil
}

// 查询某个 group_id 下的成员列表
func (r *GroupRepository) ListMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	var rows []groupMemberRow

	if err := r.db.WithContext(ctx).Where("group_id = ?", groupID).
		Order("joined_at ASC, user_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	members := make([]model.GroupMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, row.model())
	}

	return members, nil
}

// 按群名模糊匹配或按 ID 精确匹配
func (r *GroupRepository) SearchGroups(ctx context.Context, query string, limit int) ([]model.Group, error) {
	var rows []groupRow
	pattern := "%" + query + "%"
	if err := r.db.WithContext(ctx).
		Where("name LIKE ? OR CAST(id AS CHAR) = ?", pattern, query).
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	groups := make([]model.Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, row.model())
	}
	return groups, nil
}

func (r *GroupRepository) UpdateAvatarKey(ctx context.Context, groupID int64, avatarKey string) error {
	return r.db.WithContext(ctx).
		Model(&groupRow{}).
		Where("id = ?", groupID).
		Update("avatar_key", avatarKey).Error
}

func (r *GroupRepository) AddMember(ctx context.Context, groupID, userID int64) error {
	member := groupMemberRow{
		GroupID:  groupID,
		UserID:   userID,
		Role:     string(model.GroupRoleMember),
		JoinedAt: time.Now(),
	}
	return r.db.WithContext(ctx).Create(&member).Error
}

func (r *GroupRepository) RemoveMember(ctx context.Context, groupID, userID int64) error {
	return r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ? AND role <> ?", groupID, userID, string(model.GroupRoleOwner)).
		Delete(&groupMemberRow{}).Error
}

// Dissolve removes all persistent data that belongs to a group. Messages must
// be removed before the group because messages.group_id has a foreign key, and
// outbox events must be removed before their messages for the same reason.
func (r *GroupRepository) Dissolve(ctx context.Context, groupID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM outbox_events WHERE message_id IN (SELECT id FROM messages WHERE group_id = ?)", groupID).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", groupID).Delete(&messageRow{}).Error; err != nil {
			return err
		}
		// New schemas cascade this row automatically, but deleting it explicitly
		// also supports databases created before the cascade was added.
		if err := tx.Exec("DELETE FROM group_join_requests WHERE group_id = ?", groupID).Error; err != nil {
			return err
		}
		return tx.Delete(&groupRow{}, groupID).Error
	})
}

func (r *GroupRepository) UpdateAnnouncement(ctx context.Context, groupID, requesterID int64, announcement string) (model.Group, error) {
	var row groupRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&groupRow{}).Where("id = ?", groupID).Update("announcement", announcement)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("group not found or permission denied")
		}
		return tx.First(&row, groupID).Error
	})
	if err != nil {
		return model.Group{}, err
	}
	return row.model(), nil
}

func (r *GroupRepository) SetAllMuted(ctx context.Context, groupID, requesterID int64, muted bool) (model.Group, error) {
	var row groupRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&groupRow{}).Where("id = ?", groupID).Update("all_muted", muted)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("group not found or permission denied")
		}
		return tx.First(&row, groupID).Error
	})
	if err != nil {
		return model.Group{}, err
	}
	return row.model(), nil
}

func (r *GroupRepository) SetMemberRole(ctx context.Context, groupID, userID int64, role model.GroupRole) error {
	return r.db.WithContext(ctx).Model(&groupMemberRow{}).Where("group_id = ? AND user_id = ? AND role <> ?", groupID, userID, string(model.GroupRoleOwner)).Update("role", string(role)).Error
}

func (r *GroupRepository) SetMemberMutedUntil(ctx context.Context, groupID, userID int64, until *time.Time) error {
	return r.db.WithContext(ctx).Model(&groupMemberRow{}).Where("group_id = ? AND user_id = ?", groupID, userID).Update("muted_until", until).Error
}
