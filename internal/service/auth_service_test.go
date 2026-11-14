package service

import (
	"context"
	"errors"
	"testing"

	"reading-activity/internal/clock"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

// TestAuthRegisterAndLogin 注册后可直接登录，令牌可用（真实认证流闭环）。
func TestAuthRegisterAndLogin(t *testing.T) {
	db := testutil.Setup(t)
	svc := NewAuthService(db, clock.System{})
	ctx := context.Background()

	acc, err := svc.Register(ctx, "demo_user", "pass123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if acc.Role != "user" || acc.Token == "" {
		t.Fatalf("unexpected account: %+v", acc)
	}

	// 注册返回的令牌应能通过 GetAccountByToken 解析
	got, err := store.GetAccountByToken(ctx, db, acc.Token)
	if err != nil || got == nil || got.Username != "demo_user" {
		t.Fatalf("token lookup: acc=%+v err=%v", got, err)
	}

	// 正确口令登录
	login, err := svc.Login(ctx, "demo_user", "pass123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if login.ID != acc.ID || login.Token != acc.Token {
		t.Fatalf("login mismatch: %+v vs %+v", login, acc)
	}
}

// TestLoginWrongPassword 口令错误返回 ErrInvalidCredentials。
func TestLoginWrongPassword(t *testing.T) {
	db := testutil.Setup(t)
	svc := NewAuthService(db, clock.System{})
	ctx := context.Background()

	if _, err := svc.Register(ctx, "demo_user", "pass123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := svc.Login(ctx, "demo_user", "wrong-pass")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	// 账号不存在同样返回 ErrInvalidCredentials（不泄露账号存在性）
	_, err = svc.Login(ctx, "ghost_user", "pass123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials for unknown user, got %v", err)
	}
}

// TestRegisterDuplicate 与校验：重名 409、弱口令/非法账号名 400。
func TestRegisterDuplicateAndValidation(t *testing.T) {
	db := testutil.Setup(t)
	svc := NewAuthService(db, clock.System{})
	ctx := context.Background()

	if _, err := svc.Register(ctx, "demo_user", "pass123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.Register(ctx, "demo_user", "other123"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("want ErrUsernameTaken, got %v", err)
	}
	if _, err := svc.Register(ctx, "demo_user2", "12345"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("want ErrWeakPassword, got %v", err)
	}
	if _, err := svc.Register(ctx, "ab", "pass123"); !errors.Is(err, ErrUsernameInvalid) {
		t.Fatalf("want ErrUsernameInvalid, got %v", err)
	}
	if _, err := svc.Register(ctx, "bad name!", "pass123"); !errors.Is(err, ErrUsernameInvalid) {
		t.Fatalf("want ErrUsernameInvalid for illegal chars, got %v", err)
	}
}

// TestAuthPresetLogin 迁移预置账号（005 默认口令）可登录。
func TestAuthPresetLogin(t *testing.T) {
	db := testutil.Setup(t)
	svc := NewAuthService(db, clock.System{})
	ctx := context.Background()

	// testutil.Setup 会清空全部数据；按 005 的生产种子口径重新插入预置账号
	if _, err := db.ExecContext(ctx,
		"INSERT INTO user_account (username, role, token, password_hash) VALUES ('user1','user','user-token-1', SHA2(CONCAT('user1', ':', '123456'), 256))"); err != nil {
		t.Fatalf("seed preset: %v", err)
	}
	acc, err := svc.Login(ctx, "user1", "123456")
	if err != nil || acc.Role != "user" {
		t.Fatalf("preset login: acc=%+v err=%v", acc, err)
	}
}
