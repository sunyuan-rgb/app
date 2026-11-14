// Package service - 认证流：注册/登录（密码哈希校验 + 会话令牌下发）。
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"

	"reading-activity/internal/clock"
	"reading-activity/internal/store"
)

// 认证错误（handler 层映射为 401/409/400）。
var (
	ErrInvalidCredentials = errors.New("账号名或密码错误")
	ErrUsernameTaken      = errors.New("账号名已存在")
	ErrWeakPassword       = errors.New("密码长度须为 6~64 位")
	ErrUsernameInvalid    = errors.New("账号名须为 3~32 位字母/数字/下划线")
)

// AuthService 认证服务。
type AuthService struct {
	DB  *sql.DB
	Clk clock.Clock
}

// NewAuthService 创建认证服务。
func NewAuthService(db *sql.DB, clk clock.Clock) *AuthService {
	return &AuthService{DB: db, Clk: clk}
}

// Login 校验用户名口令，成功返回账号（含会话令牌）。
func (s *AuthService) Login(ctx context.Context, username, password string) (*store.Account, error) {
	acc, err := store.GetAccountByUsername(ctx, s.DB, username)
	if err != nil {
		return nil, err
	}
	if acc == nil || !acc.PasswordHash.Valid || acc.PasswordHash.String != hashPassword(username, password) {
		return nil, ErrInvalidCredentials
	}
	return acc, nil
}

// Register 创建用户账号并下发会话令牌（角色固定 user，余额由流水合计推导）。
func (s *AuthService) Register(ctx context.Context, username, password string) (*store.Account, error) {
	if !validUsername(username) {
		return nil, ErrUsernameInvalid
	}
	if len(password) < 6 || len(password) > 64 {
		return nil, ErrWeakPassword
	}
	if existing, err := store.GetAccountByUsername(ctx, s.DB, username); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrUsernameTaken
	}
	token, err := newSessionToken()
	if err != nil {
		return nil, err
	}
	return store.CreateAccount(ctx, s.DB, username, token, hashPassword(username, password))
}

// hashPassword 口令哈希：SHA2(username:password)，与 migrations/005_auth.sql 预置口径一致。
func hashPassword(username, password string) string {
	sum := sha256.Sum256([]byte(username + ":" + password))
	return hex.EncodeToString(sum[:])
}

// newSessionToken 生成 32 字节随机会话令牌（64 位十六进制）。
func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func validUsername(u string) bool {
	if len(u) < 3 || len(u) > 32 {
		return false
	}
	for _, r := range u {
		ok := r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			return false
		}
	}
	return true
}
