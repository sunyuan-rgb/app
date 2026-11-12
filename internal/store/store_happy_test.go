package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"reading-activity/internal/model"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

// 预置一份完整业务数据，逐个函数验证正常路径（store 包自覆盖率）。
func seedAccounts(t *testing.T, db *sql.DB) {
	t.Helper()
	for i := 1; i <= 5; i++ {
		if _, err := db.Exec(`INSERT INTO user_account (id, username, role, token) VALUES (?, ?, 'user', ?)`,
			i, "user"+itoa(int64(i)), "test-user-token-"+itoa(int64(i))); err != nil {
			t.Fatalf("seed account %d: %v", i, err)
		}
	}
}

func TestStore_HappyPaths(t *testing.T) {
	db := testutil.Setup(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)

	// 活动
	row := &store.ActivityRow{Name: "活动", StartAt: time.Date(2026, 11, 10, 16, 0, 0, 0, time.UTC),
		EndAt: time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC), DailyThreshold: 3, RewardPoints: 30}
	ok, err := store.CreateActivity(ctx, db, row)
	if err != nil || !ok || row.ID == 0 {
		t.Fatalf("CreateActivity: ok=%v id=%d err=%v", ok, row.ID, err)
	}
	// 重复发布被拒
	if ok, err := store.CreateActivity(ctx, db, &store.ActivityRow{Name: "x", StartAt: row.StartAt, EndAt: row.EndAt}); err != nil || ok {
		t.Fatalf("重复发布应返回 false: ok=%v err=%v", ok, err)
	}
	act, err := store.LatestPublishedActivity(ctx, db)
	if err != nil || act == nil || act.ID != row.ID || act.DailyThreshold != 3 {
		t.Fatalf("LatestPublishedActivity: %+v %v", act, err)
	}

	// 账号（申诉/运营汇总需要）
	seedAccounts(t, db)
	if acc, err := store.GetAccountByToken(ctx, db, "test-user-token-1"); err != nil || acc == nil || acc.ID != 1 || acc.Role != "user" {
		t.Fatalf("GetAccountByToken: %+v %v", acc, err)
	}
	if acc, err := store.GetAccountByToken(ctx, db, "no-such-token"); err != nil || acc != nil {
		t.Fatalf("未知令牌应返回 nil: %+v %v", acc, err)
	}

	// 事件 + 进度 + 发奖 + 流水 + 余额
	ev := &model.ReadingEvent{EventID: "e1", ActivityID: act.ID, UserID: 1, ArticleID: 11,
		OccurAt: now, ReceivedAt: now, BizDate: "2026-11-11", Source: model.SourceAuto, Status: model.EventAccepted}
	if ok, err := store.InsertEvent(ctx, db, ev); err != nil || !ok {
		t.Fatalf("InsertEvent: %v %v", ok, err)
	}
	if got, err := store.GetEventByID(ctx, db, "e1"); err != nil || got == nil || got.UserID != 1 || got.BizDate != "2026-11-11" {
		t.Fatalf("GetEventByID: %+v %v", got, err)
	}
	if got, err := store.GetEventByID(ctx, db, "nope"); err != nil || got != nil {
		t.Fatalf("不存在事件应返回 nil: %+v %v", got, err)
	}
	for i, aid := range []int64{11, 12, 13} {
		eid := "e" + itoa(int64(i+1))
		if ok, err := store.InsertProgress(ctx, db, 1, "2026-11-11", aid, eid); err != nil || !ok {
			t.Fatalf("InsertProgress: %v %v", ok, err)
		}
		if err := store.InsertPointTxn(ctx, db, 1, 0, "seed", 0); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := store.InsertProgress(ctx, db, 1, "2026-11-11", 11, "dup"); err != nil || ok {
		t.Fatalf("重复进度应返回 false: %v %v", ok, err)
	}
	if n, err := store.CountArticles(ctx, db, 1, "2026-11-11"); err != nil || n != 3 {
		t.Fatalf("CountArticles: %d %v", n, err)
	}
	if arts, err := store.ListProgressArticles(ctx, db, 1, "2026-11-11"); err != nil || len(arts) != 3 {
		t.Fatalf("ListProgressArticles: %v %v", arts, err)
	}
	if ok, err := store.InsertReward(ctx, db, 1, act.ID, "2026-11-11", 30); err != nil || !ok {
		t.Fatalf("InsertReward: %v %v", ok, err)
	}
	if ok, err := store.InsertReward(ctx, db, 1, act.ID, "2026-11-11", 30); err != nil || ok {
		t.Fatalf("重复发奖应返回 false: %v %v", ok, err)
	}
	rewardID, points, rewarded, err := store.GetRewardDetail(ctx, db, 1, act.ID, "2026-11-11")
	if err != nil || !rewarded || points != 30 || rewardID == 0 {
		t.Fatalf("GetRewardDetail: %d %d %v %v", rewardID, points, rewarded, err)
	}
	if p, ok, err := store.GetReward(ctx, db, 1, act.ID, "2026-11-11"); err != nil || !ok || p != 30 {
		t.Fatalf("GetReward: %d %v %v", p, ok, err)
	}
	if err := store.AddBalance(ctx, db, 1, 30); err != nil {
		t.Fatal(err)
	}
	if b, err := store.GetBalance(ctx, db, 1); err != nil || b != 30 {
		t.Fatalf("GetBalance: %d %v", b, err)
	}
	if b, err := store.GetBalance(ctx, db, 999); err != nil || b != 0 {
		t.Fatalf("无记录余额应为 0: %d %v", b, err)
	}
	if txns, err := store.ListPointTxn(ctx, db, 1, 50); err != nil || len(txns) != 3 {
		t.Fatalf("ListPointTxn: %d %v", len(txns), err)
	}

	// 申诉全流程
	ok, appealID, err := store.InsertAppeal(ctx, db, 1, act.ID, "2026-11-11", "奖励未到账")
	if err != nil || !ok || appealID == 0 {
		t.Fatalf("InsertAppeal: %v %d %v", ok, appealID, err)
	}
	if ok, _, err := store.InsertAppeal(ctx, db, 1, act.ID, "2026-11-11", "重复申诉"); err != nil || ok {
		t.Fatalf("同日重复申诉应返回 false: %v %v", ok, err)
	}
	if a, err := store.GetAppealByID(ctx, db, appealID); err != nil || a == nil || a.Status != store.AppealOpen {
		t.Fatalf("GetAppealByID: %+v %v", a, err)
	}
	open := store.AppealOpen
	if items, err := store.ListAppeals(ctx, db, &open); err != nil || len(items) != 1 {
		t.Fatalf("ListAppeals(open): %d %v", len(items), err)
	}
	if items, err := store.ListAppeals(ctx, db, nil); err != nil || len(items) != 1 {
		t.Fatalf("ListAppeals(all): %d %v", len(items), err)
	}
	if evs, err := store.ListEventsByUserDate(ctx, db, 1, "2026-11-11"); err != nil || len(evs) != 1 {
		t.Fatalf("ListEventsByUserDate: %d %v", len(evs), err)
	}
	if ok, err := store.ResolveAppeal(ctx, db, appealID, store.AppealRejected, "驳回：证据不足", 6); err != nil || !ok {
		t.Fatalf("ResolveAppeal: %v %v", ok, err)
	}
	if ok, err := store.ResolveAppeal(ctx, db, appealID, store.AppealRejected, "再次驳回", 6); err != nil || ok {
		t.Fatalf("重复结案应返回 false: %v %v", ok, err)
	}

	// 运营汇总
	sum, err := store.SummarizeDay(ctx, db, act.ID, "2026-11-11", 3)
	if err != nil || sum.ReachedUsers != 1 || sum.RewardedUsers != 1 || sum.TotalPoints != 30 {
		t.Fatalf("SummarizeDay: %+v %v", sum, err)
	}
	if rows, err := store.ListReachedNotRewarded(ctx, db, act.ID, "2026-11-12", 3); err != nil || len(rows) != 0 {
		t.Fatalf("ListReachedNotRewarded: %d %v", len(rows), err)
	}
	if rows, err := store.ListRewardedNotReached(ctx, db, act.ID, "2026-11-12", 3); err != nil || len(rows) != 0 {
		t.Fatalf("ListRewardedNotReached: %d %v", len(rows), err)
	}
	if rows, err := store.ListRewardsByDate(ctx, db, act.ID, "2026-11-11"); err != nil || len(rows) != 1 {
		t.Fatalf("ListRewardsByDate: %d %v", len(rows), err)
	}
	// 冲突留痕
	if err := store.InsertEventConflict(ctx, db, "e1", "{}", now); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
