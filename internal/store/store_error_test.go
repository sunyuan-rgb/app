package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"reading-activity/internal/model"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

// TestStoreErrorBranches 所有数据访问函数在数据库错误时必须返回错误、不得 panic。
// 使用注入错误的 *sql.DB 确定性覆盖每个函数的错误分支。
func TestStoreErrorBranches(t *testing.T) {
	db := testutil.NewErrDB()
	defer db.Close()
	ctx := context.Background()
	now := time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)

	ev := &model.ReadingEvent{EventID: "e1", UserID: 1, ArticleID: 1, OccurAt: now, ReceivedAt: now, BizDate: "2026-11-11"}

	if _, err := store.InsertEvent(ctx, db, ev); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("InsertEvent: %v", err)
	}
	if _, err := store.GetEventByID(ctx, db, "e1"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetEventByID: %v", err)
	}
	if err := store.InsertEventConflict(ctx, db, "e1", "{}", now); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("InsertEventConflict: %v", err)
	}
	if _, err := store.InsertProgress(ctx, db, 1, "2026-11-11", 1, "e1"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("InsertProgress: %v", err)
	}
	if _, err := store.CountArticles(ctx, db, 1, "2026-11-11"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("CountArticles: %v", err)
	}
	if _, err := store.InsertReward(ctx, db, 1, 1, "2026-11-11", 30); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("InsertReward: %v", err)
	}
	if err := store.InsertPointTxn(ctx, db, 1, 30, "reward", 1); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("InsertPointTxn: %v", err)
	}
	if err := store.AddBalance(ctx, db, 1, 30); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("AddBalance: %v", err)
	}
	if _, err := store.LatestPublishedActivity(ctx, db); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("LatestPublishedActivity: %v", err)
	}
	if _, _, err := store.GetReward(ctx, db, 1, 1, "2026-11-11"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetReward: %v", err)
	}
	if _, _, _, err := store.GetRewardDetail(ctx, db, 1, 1, "2026-11-11"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetRewardDetail: %v", err)
	}
	if _, err := store.ListProgressArticles(ctx, db, 1, "2026-11-11"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ListProgressArticles: %v", err)
	}
	if _, err := store.GetBalance(ctx, db, 1); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetBalance: %v", err)
	}
	if _, err := store.ListPointTxn(ctx, db, 1, 50); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ListPointTxn: %v", err)
	}
	if _, err := store.CreateActivity(ctx, db, &store.ActivityRow{
		Name: "x", StartAt: now, EndAt: now.Add(time.Hour), DailyThreshold: 3, RewardPoints: 30,
	}); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("CreateActivity: %v", err)
	}
	if _, _, err := store.InsertAppeal(ctx, db, 1, 1, "2026-11-11", "x"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("InsertAppeal: %v", err)
	}
	if _, err := store.GetAppealByID(ctx, db, 1); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetAppealByID: %v", err)
	}
	if _, err := store.ListAppeals(ctx, db, nil); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ListAppeals: %v", err)
	}
	if _, err := store.ResolveAppeal(ctx, db, 1, store.AppealRejected, "x", 6); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ResolveAppeal: %v", err)
	}
	if _, err := store.ListEventsByUserDate(ctx, db, 1, "2026-11-11"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ListEventsByUserDate: %v", err)
	}
	if _, err := store.SummarizeDay(ctx, db, 1, "2026-11-11", 3); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("SummarizeDay: %v", err)
	}
	if _, err := store.ListReachedNotRewarded(ctx, db, 1, "2026-11-11", 3); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ListReachedNotRewarded: %v", err)
	}
	if _, err := store.ListRewardedNotReached(ctx, db, 1, "2026-11-11", 3); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ListRewardedNotReached: %v", err)
	}
	if _, err := store.ListRewardsByDate(ctx, db, 1, "2026-11-11"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ListRewardsByDate: %v", err)
	}
}
