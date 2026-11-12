package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"reading-activity/internal/model"
	"reading-activity/internal/service"
	"reading-activity/internal/testutil"
)

// 并发场景 1：N 个不同事件同时投递"第 3 篇文章"（同用户同日同文章），
// 文章去重唯一键必须保证只有一条计入、只发一次奖。
func TestConcurrent_ThirdArticleOnlyRewardedOnce(t *testing.T) {
	db := testutil.Setup(t)
	seedActivity(t, db)
	seedProgress(t, db, 1, "2026-11-11", 11, 12)

	svc := newEventService(db, baseNow)
	const n = 20
	var wg sync.WaitGroup
	results := make([]service.ResultStatus, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := svc.HandleEvent(context.Background(), service.EventInput{
				EventID:   fmt.Sprintf("race-%d", i),
				UserID:    1,
				ArticleID: 13, // 同一篇
				OccurAt:   bj(10, 0),
			}, model.SourceAuto)
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				return
			}
			results[i] = res.Status
		}(i)
	}
	wg.Wait()

	rewarded := 0
	for _, s := range results {
		if s == service.ResultRewarded {
			rewarded++
		}
	}
	if rewarded != 1 {
		t.Fatalf("应恰好 1 个 rewarded，实际 %d: %v", rewarded, results)
	}
	if b := getBalance(t, db, 1); b != 30 {
		t.Fatalf("balance = %d, want 30", b)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM reward"); n != 1 {
		t.Fatalf("reward 条数 = %d, want 1", n)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM point_txn"); n != 1 {
		t.Fatalf("point_txn 条数 = %d, want 1", n)
	}
}

// 并发场景 2：同一事件编号并发重复投递（消息队列重放典型场景），
// 只允许落一条事件记录，全部调用按首次结果幂等返回。
func TestConcurrent_SameEventIdOneRow(t *testing.T) {
	db := testutil.Setup(t)
	seedActivity(t, db)
	seedProgress(t, db, 2, "2026-11-11", 11, 12)

	svc := newEventService(db, baseNow)
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := svc.HandleEvent(context.Background(), service.EventInput{
				EventID: "race-same", UserID: 2, ArticleID: 13, OccurAt: bj(10, 0),
			}, model.SourceAuto)
			if err != nil {
				t.Errorf("HandleEvent: %v", err)
				return
			}
			if s := res.Status; s != service.ResultRewarded && s != service.ResultDupEvent {
				t.Errorf("非法状态: %s", s)
			}
		}()
	}
	wg.Wait()

	if n := count(t, db, "SELECT COUNT(*) FROM reading_event WHERE event_id='race-same'"); n != 1 {
		t.Fatalf("事件应只落一条, got %d", n)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM reward WHERE user_id=2"); n != 1 {
		t.Fatalf("reward 条数 = %d, want 1", n)
	}
	if b := getBalance(t, db, 2); b != 30 {
		t.Fatalf("balance = %d, want 30", b)
	}
}

// 并发场景 3（规则 7 核心防线）：自动入账与客服补发并发，
// 两条不同事件让用户恰好达到门槛，只允许发一次奖。
// 注：count==门槛 才尝试发奖，两条并发只可能有一个先把计数推到门槛，
// 该用例验证的是"补入路径与自动路径交织时余额与发奖记录仍然精确"。
func TestConcurrent_AutoAndAgentPaths(t *testing.T) {
	db := testutil.Setup(t)
	seedActivity(t, db)
	seedProgress(t, db, 3, "2026-11-11", 11, 12)

	agentSvc := service.NewAgentService(db, clockFixed(baseNow), newEventService(db, baseNow), nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		svc := newEventService(db, baseNow)
		if _, err := svc.HandleEvent(context.Background(), service.EventInput{
			EventID: "auto-3", UserID: 3, ArticleID: 13, OccurAt: bj(10, 0),
		}, model.SourceAuto); err != nil {
			t.Errorf("auto: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		appealID := insertAppealDirect(t, db, 3, 1, "2026-11-11")
		if _, err := agentSvc.CompensateAppeal(context.Background(), appealID, 6, "agent-3", 14, bj(10, 30)); err != nil {
			t.Errorf("compensate: %v", err)
		}
	}()
	wg.Wait()

	if n := count(t, db, "SELECT COUNT(*) FROM reward WHERE user_id=3"); n != 1 {
		t.Fatalf("reward 条数 = %d, want 1", n)
	}
	if b := getBalance(t, db, 3); b != 30 {
		t.Fatalf("balance = %d, want 30", b)
	}
	// 三条事件（2 预置 + 2 并发，其中文章去重后为 4 篇）：进度恰 4 篇
	if n := count(t, db, "SELECT COUNT(DISTINCT article_id) FROM daily_progress WHERE user_id=3"); n != 4 {
		t.Fatalf("去重文章数 = %d, want 4", n)
	}
}
