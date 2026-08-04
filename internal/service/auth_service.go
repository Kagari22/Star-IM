package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"IM_Chat_System/internal/auth"
	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/repository"
)

type AuthService struct {
	users     repository.UserRepository
	tokenSpan TokenIssuer
}

type TokenIssuer interface {
	Generate(userID int64, username string) (string, error)
}

type JWTIssuer struct {
	Secret string
	TTL    int64
}

func (j JWTIssuer) Generate(userID int64, username string) (string, error) {
	return auth.GenerateToken(j.Secret, userID, username, time.Duration(j.TTL)*time.Hour)
}

func NewAuthService(users repository.UserRepository, secret string, ttlHours int64) *AuthService {
	return &AuthService{
		users:     users,
		tokenSpan: JWTIssuer{Secret: secret, TTL: ttlHours},
	}
}

func (s *AuthService) Register(ctx context.Context, username, password, nickname string) (model.User, error) {
	username = strings.TrimSpace(username)
	nickname = strings.TrimSpace(nickname)

	if username == "" || password == "" {
		return model.User{}, errors.New("username and password are required")
	}
	if len(password) < 6 {
		return model.User{}, errors.New("password must be at least 6 characters")
	}
	if nickname == "" {
		nickname = username
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return model.User{}, err
	}

	return s.users.Create(ctx, username, passwordHash, nickname)
}

func (s *AuthService) Login(ctx context.Context, username, password string) (string, model.User, error) {
	user, ok, err := s.users.GetByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return "", model.User{}, err
	}
	if !ok || !auth.CheckPassword(password, user.PasswordHash) {
		return "", model.User{}, errors.New("invalid username or password")
	}

	token, err := s.tokenSpan.Generate(user.ID, user.Username)
	if err != nil {
		return "", model.User{}, err
	}
	user.PasswordHash = ""
	return token, user, nil
}
