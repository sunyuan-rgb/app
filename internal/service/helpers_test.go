package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/service"
	"reading-activity/internal/store"
)

// 演示基准时间：北京时间 2026-11-11 21:00。
var baseNow = time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)

// seedActivity 发布一个测试活动：11-11 00:00 ~ 11-13 00:00（北京），门槛 3 篇，奖 30 分。
func seedActivity(t *testing.T, db *sql.DB) *model.Activity {
	t.Helper()
	a := &store.ActivityRow{
		Name:           "测试活动",
		StartAt:        time.Date(2026, 11, 11, 0, 0, 0, 0, model.Beijing),
		EndAt:          time.Date(2026, 11, 13, 0, 0, 0, 0, model.Beijing),
		DailyThreshold: 3,
		RewardPoints:   30,
	}
	ok, err := store.CreateActivity(context.Background(), db, a)
	if err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	if !ok {
		t.Fatal("seed activity: 已存在已发布活动")
	}
	return &model.Activity{
		ID: a.ID, Name: a.Name, StartAt: a.StartAt, EndAt: a.EndAt,
		DailyThreshold: a.DailyThreshold, RewardPoints: a.RewardPoints,
	}
}

// handle 提交一条事件的便捷封装。
func handle(t *testing.T, svc *service.EventService, eventID string, userID, articleID int64, occurAt time.Time) *service.HandleResult {
	t.Helper()
	res, err := svc.HandleEvent(context.Background(), service.EventInput{
		EventID: eventID, UserID: userID, ArticleID: articleID, OccurAt: occurAt,
	}, model.SourceAuto)
	if err != nil {
		t.Fatalf("HandleEvent %s: %v", eventID, err)
	}
	return res
}

func newEventService(db *sql.DB, now time.Time) *service.EventService {
	return service.NewEventService(db, clock.Fixed{T: now})
}

// bj 北京时间 2026-11-11 h:m（活动首日）。
func bj(h, m int) time.Time {
	return time.Date(2026, 11, 11, h, m, 0, 0, model.Beijing)
}

// count 执行返回单值的查询。
func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	return n
}

// getBalance 查询用户余额（无记录为 0）。
func getBalance(t *testing.T, db *sql.DB, userID int64) int {
	t.Helper()
	var balance int
	if err := db.QueryRow(`SELECT balance FROM user_balance WHERE user_id = ?`, userID).Scan(&balance); err != nil {
		return 0
	}
	return balance
}

// seedProgress 直接预置用户当日已计文章（并发测试前铺垫）。
func seedProgress(t *testing.T, db *sql.DB, userID int64, bizDate string, articleIDs ...int64) {
	t.Helper()
	for _, aid := range articleIDs {
		ok, err := store.InsertProgress(context.Background(), db, userID, bizDate, aid, "seed-"+bizDate+"-"+itoa(aid))
		if err != nil {
			t.Fatalf("seed progress: %v", err)
		}
		if !ok {
			t.Fatalf("seed progress: article %d 已存在", aid)
		}
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// clockFixed 便捷构造固定时钟。
func clockFixed(now time.Time) clock.Fixed {
	return clock.Fixed{T: now}
}

// seedAccounts 预置 5 个用户账号（运营汇总按 role='user' 统计，测试需还原生产账号表）。
func seedAccounts(t *testing.T, db *sql.DB) {
	t.Helper()
	for i := 1; i <= 5; i++ {
		if _, err := db.Exec(`INSERT INTO user_account (id, username, role, token) VALUES (?, ?, 'user', ?)`,
			i, "user"+itoa(int64(i)), "test-user-token-"+itoa(int64(i))); err != nil {
			t.Fatalf("seed account %d: %v", i, err)
		}
	}
}

// insertAppealDirect 直插一条待处理申诉（并发测试铺垫）。
func insertAppealDirect(t *testing.T, db *sql.DB, userID, activityID int64, bizDate string) int64 {
	t.Helper()
	ok, id, err := store.InsertAppeal(context.Background(), db, userID, activityID, bizDate, "测试申诉")
	if err != nil || !ok {
		t.Fatalf("insert appeal: ok=%v err=%v", ok, err)
	}
	return id
}
