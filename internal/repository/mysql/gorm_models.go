package mysql

import (
	"time"

	"IM_Chat_System/internal/model"
)

// 持久化层的行(数据库记录结构)只包含真实存在于数据库中的字段
// 像 User.UnreadCount 这种仅用于 API 返回的字段, 不能包含在 GORM 查询中

// 数据库映射结构体, 表示 users 表的一行数据
type userRow struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement:false"`
	Username     string    `gorm:"column:username"`
	PasswordHash string    `gorm:"column:password_hash"`
	Nickname     string    `gorm:"column:nickname"`
	AvatarKey    string    `gorm:"column:avatar_key"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

type groupRow struct {
	ID           int64     `gorm:"column:id;primaryKey"`
	Name         string    `gorm:"column:name"`
	OwnerID      int64     `gorm:"column:owner_id"`
	AvatarKey    string    `gorm:"column:avatar_key"`
	AvatarURL    string    `gorm:"column:avatar_url"`
	Announcement string    `gorm:"column:announcement"`
	AllMuted     bool      `gorm:"column:all_muted"`
	InviteCode   string    `gorm:"column:invite_code"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (groupRow) TableName() string { return "chat_groups" }

func (r groupRow) model() model.Group {
	return model.Group{
		ID:           r.ID,
		Name:         r.Name,
		OwnerID:      r.OwnerID,
		AvatarKey:    r.AvatarKey,
		AvatarURL:    r.AvatarURL,
		Announcement: r.Announcement,
		AllMuted:     r.AllMuted,
		InviteCode:   r.InviteCode,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

func (r groupMemberRow) model() model.GroupMember {
	return model.GroupMember{
		GroupID:           r.GroupID,
		UserID:            r.UserID,
		Role:              model.GroupRole(r.Role),
		JoinedAt:          r.JoinedAt,
		LastReadMessageID: r.LastReadMessageID,
		MutedUntil:        r.MutedUntil,
	}
}

type groupMemberRow struct {
	GroupID           int64      `gorm:"column:group_id;primaryKey"`
	UserID            int64      `gorm:"column:user_id;primaryKey"`
	Role              string     `gorm:"column:role"`
	JoinedAt          time.Time  `gorm:"column:joined_at"`
	LastReadMessageID int64      `gorm:"column:last_read_message_id"`
	MutedUntil        *time.Time `gorm:"column:muted_until"`
}

func (groupMemberRow) TableName() string { return "group_members" }

func (userRow) TableName() string { return "users" }

// 把数据库对象转换成业务对象
func (r userRow) model() model.User {
	return model.User{
		ID:           r.ID,
		Username:     r.Username,
		PasswordHash: r.PasswordHash,
		Nickname:     r.Nickname,
		AvatarKey:    r.AvatarKey,
		CreatedAt:    r.CreatedAt,
	}
}

// 表示 messages 表的一行数据
type messageRow struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	FromUserID   int64      `gorm:"column:from_user_id"`
	ToUserID     *int64     `gorm:"column:to_user_id"`
	GroupID      *int64     `gorm:"column:group_id"`
	ContentType  string     `gorm:"column:content_type"`
	Content      string     `gorm:"column:content"`
	ObjectKey    string     `gorm:"column:object_key"`
	ObjectURL    string     `gorm:"column:object_url"`
	FileName     string     `gorm:"column:file_name"`
	FileSize     int64      `gorm:"column:file_size"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	RecalledAt   *time.Time `gorm:"column:recalled_at"`
	RecalledBy   *int64     `gorm:"column:recalled_by"`
	RecallReason string     `gorm:"column:recall_reason"`
	ReplyToID    *int64     `gorm:"column:reply_to_id"`
	EditedAt     *time.Time `gorm:"column:edited_at"`
}

func (messageRow) TableName() string { return "messages" }

func (r messageRow) model() model.Message {
	var toUserID int64
	if r.ToUserID != nil {
		toUserID = *r.ToUserID
	}

	message := model.Message{
		ID:           r.ID,
		FromUserID:   r.FromUserID,
		ToUserID:     toUserID,
		GroupID:      r.GroupID,
		ContentType:  r.ContentType,
		Content:      r.Content,
		ObjectKey:    r.ObjectKey,
		ObjectURL:    r.ObjectURL,
		FileName:     r.FileName,
		FileSize:     r.FileSize,
		CreatedAt:    r.CreatedAt,
		RecalledAt:   r.RecalledAt,
		RecallReason: r.RecallReason,
		ReplyToID:    r.ReplyToID,
		EditedAt:     r.EditedAt,
	}
	if r.RecalledBy != nil {
		message.RecalledBy = *r.RecalledBy
	}
	return message
}
