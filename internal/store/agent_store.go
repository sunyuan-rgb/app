package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// 申诉状态。
const (
	AppealOpen      int8 = 0 // 待处理
	AppealCompensed int8 = 1 // 已补发
	AppealRejected  int8 = 2 // 已驳回
)

// Appeal 申诉记录。
type Appeal struct {
	ID         int64
	UserID     int64
	ActivityID int64
	BizDate    string
	Content    string
	Status     int8
	Conclusion string
	OperatorID int64
	HandledAt  sql.NullTime
	CreatedAt  time.Time
}

// ListAppeals 按状态列出申诉；status 为 nil 时列出全部。
func ListAppeals(ctx context.Context, q Queryer, status *int8) ([]Appeal, error) {
	query := `SELECT id, user_id, activity_id, biz_date, content, status, conclusion, operator_id, handled_at, created_at FROM appeal`
	args := []any{}
	if status != nil {
		query += ` WHERE status = ?`
		args = append(args, *status)
	}
	query += ` ORDER BY id DESC`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Appeal
	for rows.Next() {
		var a Appeal
		var bizDate time.Time
		if err := rows.Scan(&a.ID, &a.UserID, &a.ActivityID, &bizDate, &a.Content,
			&a.Status, &a.Conclusion, &a.OperatorID, &a.HandledAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.BizDate = bizDate.Format("2006-01-02")
		list = append(list, a)
	}
	return list, rows.Err()
}

// GetAppealByID 按 ID 查询申诉，无则返回 nil。
func GetAppealByID(ctx context.Context, q Queryer, id int64) (*Appeal, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id, user_id, activity_id, biz_date, content, status, conclusion, operator_id, handled_at, created_at
		 FROM appeal WHERE id = ?`, id)
	var a Appeal
	var bizDate time.Time
	err := row.Scan(&a.ID, &a.UserID, &a.ActivityID, &bizDate, &a.Content,
		&a.Status, &a.Conclusion, &a.OperatorID, &a.HandledAt, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.BizDate = bizDate.Format("2006-01-02")
	return &a, nil
}

// ResolveAppeal 结案（仅允许对待处理申诉：WHERE status=0 乐观锁防并发重复处理）。
// 返回 false 表示申诉已被处理过。
func ResolveAppeal(ctx context.Context, q Queryer, id int64, status int8, conclusion string, operatorID int64) (bool, error) {
	res, err := q.ExecContext(ctx, `
		UPDATE appeal
		SET status = ?, conclusion = ?, operator_id = ?, handled_at = NOW(3)
		WHERE id = ? AND status = 0`,
		status, conclusion, operatorID, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// ListEventsByUserDate 查询用户某业务日的全部事件（含拒绝），供客服核查。
func ListEventsByUserDate(ctx context.Context, q Queryer, userID int64, bizDate string) ([]EventRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT event_id, article_id, occur_at, received_at, source, status, counted, reject_reason
		FROM reading_event WHERE user_id = ? AND biz_date = ? ORDER BY occur_at`,
		userID, bizDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []EventRow
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.EventID, &e.ArticleID, &e.OccurAt, &e.ReceivedAt,
			&e.Source, &e.Status, &e.Counted, &e.RejectReason); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// EventRow 事件核查行。
type EventRow struct {
	EventID      string
	ArticleID    int64
	OccurAt      time.Time
	ReceivedAt   time.Time
	Source       int8
	Status       int8
	Counted      bool
	RejectReason string
}

// GetRewardDetail 查询发奖记录（含 ID，用于告知流水）。
func GetRewardDetail(ctx context.Context, q Queryer, userID, activityID int64, bizDate string) (rewardID int64, points int, rewarded bool, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT id, points FROM reward WHERE user_id = ? AND activity_id = ? AND biz_date = ?`,
		userID, activityID, bizDate).Scan(&rewardID, &points)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return rewardID, points, true, nil
}
