package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"

	"reading-activity/internal/model"
)

// InsertEvent 写入阅读事件。返回 false 表示 event_id 已存在（幂等入口）。
func InsertEvent(ctx context.Context, q Queryer, e *model.ReadingEvent) (bool, error) {
	res, err := q.ExecContext(ctx, `
		INSERT INTO reading_event
			(event_id, activity_id, user_id, article_id, occur_at, received_at, biz_date, source, status, counted, reject_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EventID, e.ActivityID, e.UserID, e.ArticleID, e.OccurAt.UTC(), e.ReceivedAt.UTC(), e.BizDate,
		e.Source, e.Status, boolToInt(e.Counted), e.RejectReason)
	if err != nil {
		if isDuplicateErr(err) {
			return false, nil
		}
		return false, err
	}
	id, _ := res.LastInsertId()
	e.ID = id
	return true, nil
}

// GetEventByID 按全局事件编号查询。
func GetEventByID(ctx context.Context, q Queryer, eventID string) (*model.ReadingEvent, error) {
	row := q.QueryRowContext(ctx, `
		SELECT id, event_id, activity_id, user_id, article_id, occur_at, received_at, biz_date, source, status, counted, reject_reason
		FROM reading_event WHERE event_id = ?`, eventID)
	var e model.ReadingEvent
	var occur, received, bizDate time.Time
	var source, status int8
	var counted int8
	err := row.Scan(&e.ID, &e.EventID, &e.ActivityID, &e.UserID, &e.ArticleID,
		&occur, &received, &bizDate, &source, &status, &counted, &e.RejectReason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.OccurAt, e.ReceivedAt = occur, received
	e.BizDate = bizDate.Format("2006-01-02")
	e.Source, e.Status = model.EventSource(source), model.EventStatus(status)
	e.Counted = counted == 1
	return &e, nil
}

// InsertEventConflict 记录"同事件编号更换内容"的冲突请求。
func InsertEventConflict(ctx context.Context, q Queryer, eventID, payload string, receivedAt time.Time) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO event_conflict (event_id, payload, received_at) VALUES (?, ?, ?)`,
		eventID, payload, receivedAt.UTC())
	return err
}

// InsertProgress 写入每日进度。返回 false 表示该文章当日已计（唯一键去重）。
func InsertProgress(ctx context.Context, q Queryer, userID int64, bizDate string, articleID int64, eventID string) (bool, error) {
	res, err := q.ExecContext(ctx, `
		INSERT INTO daily_progress (user_id, biz_date, article_id, event_id) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE event_id = event_id`,
		userID, bizDate, articleID, eventID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// CountArticles 统计用户当日已计的不同文章数。
func CountArticles(ctx context.Context, q Queryer, userID int64, bizDate string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT article_id) FROM daily_progress WHERE user_id = ? AND biz_date = ?`,
		userID, bizDate).Scan(&n)
	return n, err
}

// InsertReward 写入发奖记录。返回 false 表示当日已发过（唯一键幂等，防并发重复发奖）。
func InsertReward(ctx context.Context, q Queryer, userID, activityID int64, bizDate string, points int) (bool, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO reward (user_id, activity_id, biz_date, points) VALUES (?, ?, ?, ?)`,
		userID, activityID, bizDate, points)
	if err != nil {
		if isDuplicateErr(err) {
			return false, nil
		}
		return false, err
	}
	_, _ = res.LastInsertId()
	return true, nil
}

// InsertPointTxn 写入积分流水。
func InsertPointTxn(ctx context.Context, q Queryer, userID int64, points int, refType string, refID int64) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO point_txn (user_id, type, points, ref_type, ref_id) VALUES (?, 1, ?, ?, ?)`,
		userID, points, refType, refID)
	return err
}

// AddBalance 累加积分余额（余额不存在则初始化）。必须与流水同一事务调用。
func AddBalance(ctx context.Context, q Queryer, userID int64, points int) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO user_balance (user_id, balance) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE balance = balance + VALUES(balance)`,
		userID, points)
	return err
}

// LatestPublishedActivity 查询当前已发布活动（题面为单一固定活动）。
func LatestPublishedActivity(ctx context.Context, q Queryer) (*model.Activity, error) {
	row := q.QueryRowContext(ctx, `
		SELECT id, name, start_at, end_at, daily_threshold, reward_points, status
		FROM activity WHERE status = ? ORDER BY id DESC LIMIT 1`, int16(model.ActivityPublished))
	var a model.Activity
	var start, end time.Time
	err := row.Scan(&a.ID, &a.Name, &start, &end, &a.DailyThreshold, &a.RewardPoints, &a.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.StartAt, a.EndAt = start, end
	return &a, nil
}

func isDuplicateErr(err error) bool {
	// MySQL 错误码 1062 = duplicate entry
	const errDupEntry = 1062
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == errDupEntry
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
