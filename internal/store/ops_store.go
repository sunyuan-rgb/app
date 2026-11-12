package store

import (
	"context"
	"database/sql"
	"time"
)

// CreateActivity 创建并发布活动（题面为单一固定活动：若已有已发布活动则拒绝）。
// 返回 false 表示已存在已发布活动。
func CreateActivity(ctx context.Context, q Queryer, a *ActivityRow) (bool, error) {
	var n int
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM activity WHERE status = 1`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	res, err := q.ExecContext(ctx, `
		INSERT INTO activity (name, start_at, end_at, daily_threshold, reward_points, status)
		VALUES (?, ?, ?, ?, ?, 1)`,
		a.Name, a.StartAt.UTC(), a.EndAt.UTC(), a.DailyThreshold, a.RewardPoints)
	if err != nil {
		if isDuplicateErr(err) {
			return false, nil
		}
		return false, err
	}
	id, _ := res.LastInsertId()
	a.ID = id
	return true, nil
}

// ActivityRow 活动行（创建输入）。
type ActivityRow struct {
	ID             int64
	Name           string
	StartAt        time.Time
	EndAt          time.Time
	DailyThreshold int
	RewardPoints   int
	Status         int8
}

// DaySummary 单个业务日的结算核查汇总。
type DaySummary struct {
	BizDate            string `json:"biz_date"`
	ReachedUsers       int    `json:"reached_users"`       // 达标人数（去重后有效文章数 ≥ 门槛）
	RewardedUsers      int    `json:"rewarded_users"`      // 发奖人数
	TotalPoints        int    `json:"total_points"`        // 积分总额
	AppealsOpen        int    `json:"appeals_open"`        // 待处理申诉
	AppealsCompensated int    `json:"appeals_compensated"` // 已补发
	AppealsRejected    int    `json:"appeals_rejected"`    // 已驳回
}

// SummarizeDay 汇总某业务日：达标人数、发奖人数、积分总额、申诉结果。
// 达标人数基于去重后的有效文章集合（不按事件条数）。
func SummarizeDay(ctx context.Context, q Queryer, activityID int64, bizDate string, threshold int) (*DaySummary, error) {
	s := &DaySummary{BizDate: bizDate}
	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT user_id FROM daily_progress
			WHERE biz_date = ? AND user_id IN (SELECT id FROM user_account WHERE role = 'user')
			GROUP BY user_id HAVING COUNT(DISTINCT article_id) >= ?
		) t`, bizDate, threshold).Scan(&s.ReachedUsers); err != nil {
		return nil, err
	}
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*), IFNULL(SUM(points), 0) FROM reward WHERE activity_id = ? AND biz_date = ?`,
		activityID, bizDate).Scan(&s.RewardedUsers, &s.TotalPoints); err != nil {
		return nil, err
	}
	if err := q.QueryRowContext(ctx, `
		SELECT
			IFNULL(SUM(CASE WHEN status = 0 THEN 1 ELSE 0 END), 0),
			IFNULL(SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END), 0),
			IFNULL(SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END), 0)
		FROM appeal WHERE biz_date = ?`, bizDate).
		Scan(&s.AppealsOpen, &s.AppealsCompensated, &s.AppealsRejected); err != nil {
		return nil, err
	}
	return s, nil
}

// UserDayRow 不一致明细行。
type UserDayRow struct {
	UserID           int64 `json:"user_id"`
	DistinctArticles int   `json:"distinct_articles"`
}

// ListReachedNotRewarded 达标但未发奖的用户（不一致明细）。
func ListReachedNotRewarded(ctx context.Context, q Queryer, activityID int64, bizDate string, threshold int) ([]UserDayRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT p.user_id, COUNT(DISTINCT p.article_id)
		FROM daily_progress p
		WHERE p.biz_date = ? AND p.user_id NOT IN (
			SELECT user_id FROM reward WHERE activity_id = ? AND biz_date = ?
		)
		GROUP BY p.user_id HAVING COUNT(DISTINCT p.article_id) >= ?`,
		bizDate, activityID, bizDate, threshold)
	return scanUserDayRows(rows, err)
}

// ListRewardedNotReached 发奖但未达标的用户（不一致明细）。
func ListRewardedNotReached(ctx context.Context, q Queryer, activityID int64, bizDate string, threshold int) ([]UserDayRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT r.user_id, IFNULL(p.cnt, 0)
		FROM reward r
		LEFT JOIN (
			SELECT user_id, COUNT(DISTINCT article_id) AS cnt
			FROM daily_progress WHERE biz_date = ? GROUP BY user_id
		) p ON p.user_id = r.user_id
		WHERE r.activity_id = ? AND r.biz_date = ?
		  AND IFNULL(p.cnt, 0) < ?`,
		bizDate, activityID, bizDate, threshold)
	return scanUserDayRows(rows, err)
}

// ListRewardsByDate 某业务日全部发奖明细（可追溯）。
func ListRewardsByDate(ctx context.Context, q Queryer, activityID int64, bizDate string) ([]RewardRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, user_id, points, created_at FROM reward
		WHERE activity_id = ? AND biz_date = ? ORDER BY id`, activityID, bizDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []RewardRow
	for rows.Next() {
		var r RewardRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.Points, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// RewardRow 发奖明细行。
type RewardRow struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Points    int       `json:"points"`
	CreatedAt time.Time `json:"created_at"`
}

func scanUserDayRows(rows *sql.Rows, err error) ([]UserDayRow, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []UserDayRow
	for rows.Next() {
		var r UserDayRow
		if err := rows.Scan(&r.UserID, &r.DistinctArticles); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
