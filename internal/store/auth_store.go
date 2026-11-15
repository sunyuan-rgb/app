package store

import (
	"context"
	"database/sql"
	"errors"
)

// GetAccountByUsername 按账号名取账号（登录校验用，含口令哈希与令牌）。
func GetAccountByUsername(ctx context.Context, q Queryer, username string) (*Account, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id, username, role, token, password_hash FROM user_account WHERE username = ?`, username)
	var a Account
	err := row.Scan(&a.ID, &a.Username, &a.Role, &a.Token, &a.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateAccount 创建账号（注册流：角色固定 user，令牌与口令哈希由服务层生成）。
func CreateAccount(ctx context.Context, q Queryer, username, token, passwordHash string) (*Account, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO user_account (username, role, token, password_hash) VALUES (?, 'user', ?, ?)`,
		username, token, passwordHash)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Account{ID: id, Username: username, Role: "user", Token: token, PasswordHash: sql.NullString{String: passwordHash, Valid: true}}, nil
}
