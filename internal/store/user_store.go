package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Account 预置账号。
type Account struct {
	ID       int64
	Username string
	Role     string
}

// GetAccountByToken 按访问令牌解析账号身份。
func GetAccountByToken(ctx context.Context, q Queryer, token string) (*Account, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id, username, role FROM user_account WHERE token = ?`, token)
	var a Account
	err := row.Scan(&a.ID, &a.Username, &a.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListProgressArticles 返回用户某业务日已计入的不同文章列表（保持计入顺序）。
func ListProgressArticles(ctx context.Context, q Queryer, userID int64, bizDate string) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT article_id FROM daily_progress WHERE user_id = ? AND biz_date = ? ORDER BY id`,
		userID, bizDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var articles []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		articles = append(articles, id)
	}
	return articles, rows.Err()
}

// GetBalance 查询余额，无记录返回 0。
func GetBalance(ctx context.Context, q Queryer, userID int64) (int, error) {
	var balance int
	err := q.QueryRowContext(ctx,
		`SELECT balance FROM user_balance WHERE user_id = ?`, userID).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return balance, err
}

// PointTxn 积分流水。
type PointTxn struct {
	ID      int64     `json:"id"`
	UserID  int64     `json:"user_id"`
	Points  int       `json:"points"`
	RefType string    `json:"ref_type"`
	RefID   int64     `json:"ref_id"`
	Created time.Time `json:"created_at"`
}

// ListPointTxn 按时间倒序列出积分流水。
func ListPointTxn(ctx context.Context, q Queryer, userID int64, limit int) ([]PointTxn, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, user_id, points, ref_type, ref_id, created_at
		FROM point_txn WHERE user_id = ? ORDER BY id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PointTxn
	for rows.Next() {
		var t PointTxn
		if err := rows.Scan(&t.ID, &t.UserID, &t.Points, &t.RefType, &t.RefID, &t.Created); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// GetReward 查询用户某活动某业务日的发奖记录，无则返回 nil。
func GetReward(ctx context.Context, q Queryer, userID, activityID int64, bizDate string) (points int, rewarded bool, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT points FROM reward WHERE user_id = ? AND activity_id = ? AND biz_date = ?`,
		userID, activityID, bizDate).Scan(&points)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return points, true, nil
}

// InsertAppeal 提交申诉。返回 (是否成功, 新申诉 ID)。false 表示该用户当天已有未结申诉（唯一键约束）。
func InsertAppeal(ctx context.Context, q Queryer, userID, activityID int64, bizDate, content string) (bool, int64, error) {
	res, err := q.ExecContext(ctx, `
		INSERT INTO appeal (user_id, activity_id, biz_date, content) VALUES (?, ?, ?, ?)`,
		userID, activityID, bizDate, content)
	if err != nil {
		if isDuplicateErr(err) {
			return false, 0, nil
		}
		return false, 0, err
	}
	id, _ := res.LastInsertId()
	return true, id, nil
}
