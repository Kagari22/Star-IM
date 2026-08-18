package mysql

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"

	"IM_Chat_System/internal/model"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

const maxRandomIDRetries = 10

func random8DigitID() int64 {
	b := make([]byte, 4)
	_, _ = crand.Read(b)
	n := binary.BigEndian.Uint32(b)
	return 10000000 + int64(n%90000000)
}

func (r *UserRepository) Create(ctx context.Context, username, passwordHash, nickname string) (model.User, error) {
	for attempt := 0; attempt < maxRandomIDRetries; attempt++ {
		row := userRow{
			ID:           random8DigitID(),
			Username:     username,
			PasswordHash: passwordHash,
			Nickname:     nickname,
		}
		err := r.db.WithContext(ctx).Create(&row).Error
		if err == nil {
			user, ok, err := r.GetByID(ctx, row.ID)
			if err != nil {
				return model.User{}, err
			}
			if !ok {
				return model.User{}, errors.New("user inserted but not found")
			}
			return user, nil
		}
		if !errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.User{}, err
		}
	}
	return model.User{}, errors.New("failed to generate unique user id")
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (model.User, bool, error) {
	var row userRow
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, false, nil
	}
	if err != nil {
		return model.User{}, false, err
	}
	return row.model(), true, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (model.User, bool, error) {
	var row userRow
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, false, nil
	}
	if err != nil {
		return model.User{}, false, err
	}
	return row.model(), true, nil
}

func (r *UserRepository) List(ctx context.Context, excludeUserID int64) ([]model.User, error) {
	var rows []userRow
	if err := r.db.WithContext(ctx).
		Where("id <> ?", excludeUserID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
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

// 按用户名 / 昵称模糊匹配或按 ID 精确匹配
func (r *UserRepository) SearchUsers(ctx context.Context, query string, excludeUserID int64, limit int) ([]model.User, error) {
	var rows []userRow
	pattern := "%" + query + "%"
	err := r.db.WithContext(ctx).
		Where("id <> ?", excludeUserID).
		Where("username LIKE ? OR nickname LIKE ? OR CAST(id AS CHAR) = ?", pattern, pattern, query).
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
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

func (r *UserRepository) UpdateNickname(ctx context.Context, id int64, nickname string) (model.User, bool, error) {
	if err := r.db.WithContext(ctx).
		Model(&userRow{}).
		Where("id = ?", id).
		Update("nickname", nickname).Error; err != nil {
		return model.User{}, false, err
	}

	// 不要只依赖 RowsAffected 的结果, 而是重新查询一次: 
	// 因为当修改后的昵称和原昵称相同时, MySQL 可能会返回影响行数为 0
	return r.GetByID(ctx, id)
}

func (r *UserRepository) UpdateAvatarKey(ctx context.Context, id int64, avatarKey string) error {
	return r.db.WithContext(ctx).
		Model(&userRow{}).
		Where("id = ?", id).
		Update("avatar_key", avatarKey).Error
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	return r.db.WithContext(ctx).
		Model(&userRow{}).
		Where("id = ?", id).
		Update("password_hash", passwordHash).Error
}
