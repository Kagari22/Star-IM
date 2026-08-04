package mysql

import (
	"context"
	"database/sql"
	"errors"

	"IM_Chat_System/internal/model"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// 向 MySQL 创建新用户, 并返回数据库中完整的用户对象
func (r *UserRepository) Create(ctx context.Context, username, passwordHash, nickname string) (model.User, error) {
	result, err := r.db.ExecContext(
		ctx,
		`INSERT INTO users (username, password_hash, nickname) VALUES (?, ?, ?)`,
		username,
		passwordHash,
		nickname,
	)
	if err != nil {
		return model.User{}, err
	}

	// 获取数据库自动生成的用户 ID
	id, err := result.LastInsertId()
	if err != nil {
		return model.User{}, err
	}

	// 根据刚插入的 ID 重新查询完整用户信息, 获取数据库生成的字段
	user, ok, err := r.GetByID(ctx, id)
	if err != nil {
		return model.User{}, err
	}
	if !ok {
		return model.User{}, errors.New("user inserted but not found")
	}
	return user, nil
}

// 根据用户 ID 查询用户信息, 并区分"用户不存在"和"数据库查询失败"
func (r *UserRepository) GetByID(ctx context.Context, id int64) (model.User, bool, error) {
	var user model.User
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, nickname, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Nickname, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, false, nil
	}
	if err != nil {
		return model.User{}, false, err
	}
	return user, true, nil
}

// 根据用户名查询用户, 主要服务于登录和用户名唯一性校验
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (model.User, bool, error) {
	var user model.User
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, nickname, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Nickname, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, false, nil
	}
	if err != nil {
		return model.User{}, false, err
	}
	return user, true, nil
}

// 用于查询用户列表, 通常用于展示聊天成员列表, 并排除当前登录用户
func (r *UserRepository) List(ctx context.Context, excludeUserID int64) ([]model.User, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, username, password_hash, nickname, created_at FROM users WHERE id <> ? ORDER BY id ASC`,
		excludeUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var user model.User
		if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Nickname, &user.CreatedAt); err != nil {
			return nil, err
		}
		user.PasswordHash = ""
		users = append(users, user)
	}
	return users, rows.Err()
}
