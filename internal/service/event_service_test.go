package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"reading-activity/internal/model"
	"reading-activity/internal/service"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

func setupEvent(t *testing.T) (*service.EventService, *model.Activity, *sql.DB) {
	t.Helper()
	db := testutil.Setup(t)
	a := seedActivity(t, db)
	return newEventService(db, baseNow), a, db
}

func TestEvent_NormalThresholdReward(t *testing.T) {
	svc, _, db := setupEvent(t)
	day := "2026-11-11"

	if res := handle(t, svc, "e-1", 1, 11, bj(10, 0)); res.Status != service.ResultAccepted {
		t.Fatalf("第1篇: %s", res.Status)
	}
	if res := handle(t, svc, "e-2", 1, 12, bj(10, 5)); res.Status != service.ResultAccepted {
		t.Fatalf("第2篇: %s", res.Status)
	}
	res := handle(t, svc, "e-3", 1, 13, bj(10, 10))
	if res.Status != service.ResultRewarded || res.RewardPoints != 30 {
		t.Fatalf("第3篇应发奖: %+v", res)
	}
	if res.BizDate != day {
		t.Fatalf("biz_date = %s, want %s", res.BizDate, day)
	}

	// 发奖记录、流水、余额强一致：余额 30 = 流水合计，reward 恰 1 条
	if n := count(t, db, "SELECT COUNT(*) FROM reward WHERE user_id=1"); n != 1 {
		t.Fatalf("reward 条数 = %d, want 1", n)
	}
	if b := getBalance(t, db, 1); b != 30 {
		t.Fatalf("balance = %d, want 30", b)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM point_txn WHERE user_id=1"); n != 1 {
		t.Fatalf("point_txn 条数 = %d, want 1", n)
	}
}

func TestEvent_DuplicateEventIdempotent(t *testing.T) {
	svc, _, db := setupEvent(t)
	in := service.EventInput{EventID: "e-1", UserID: 1, ArticleID: 11, OccurAt: bj(10, 0)}
	first, _ := svc.HandleEvent(context.Background(), in, model.SourceAuto)
	again, _ := svc.HandleEvent(context.Background(), in, model.SourceAuto)

	if first.Status != service.ResultAccepted || again.Status != service.ResultDupEvent {
		t.Fatalf("first=%s again=%s", first.Status, again.Status)
	}
	if again.BizDate != first.BizDate || again.Counted != first.Counted {
		t.Fatalf("幂等结果不一致: %+v vs %+v", first, again)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM reading_event WHERE event_id='e-1'"); n != 1 {
		t.Fatalf("事件应只落一条, got %d", n)
	}
}

func TestEvent_SameIdDifferentContentConflict(t *testing.T) {
	svc, _, db := setupEvent(t)
	handle(t, svc, "e-1", 1, 11, bj(10, 0))
	res := handle(t, svc, "e-1", 1, 99, bj(10, 0))

	if res.Status != service.ResultConflict {
		t.Fatalf("应判定冲突: %+v", res)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM event_conflict WHERE event_id='e-1'"); n != 1 {
		t.Fatalf("冲突应留痕, got %d", n)
	}
}

func TestEvent_DuplicateArticleNotCounted(t *testing.T) {
	svc, _, _ := setupEvent(t)
	handle(t, svc, "e-1", 1, 11, bj(10, 0))
	res := handle(t, svc, "e-2", 1, 11, bj(11, 0))

	if res.Status != service.ResultDupArticle || res.Counted {
		t.Fatalf("重复文章不应计数: %+v", res)
	}
	// 第 2 篇新文章到达门槛才发奖
	res = handle(t, svc, "e-3", 1, 12, bj(11, 5))
	if res.Status != service.ResultAccepted {
		t.Fatalf("第2篇不应发奖: %+v", res)
	}
	res = handle(t, svc, "e-4", 1, 13, bj(11, 10))
	if res.Status != service.ResultRewarded {
		t.Fatalf("第3篇应发奖: %+v", res)
	}
}

func TestEvent_RejectedEventsArePersisted(t *testing.T) {
	svc, _, db := setupEvent(t)

	cases := []struct {
		name   string
		occur  time.Time
		reason string
	}{
		{"未来事件", bj(23, 0).Add(time.Hour), "发生时间晚于接收时间"},
		{"超过24小时窗口", baseNow.Add(-25 * time.Hour), "超过 24 小时迟到窗口"},
		{"活动开始前", time.Date(2026, 11, 10, 23, 0, 0, 0, model.Beijing), "发生时间不在活动期间"},
		{"活动结束后", time.Date(2026, 11, 13, 0, 30, 0, 0, model.Beijing), "发生时间不在活动期间"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := handle(t, svc, "rej-"+c.name, 1, 11, c.occur)
			if res.Status != service.ResultRejected || res.Reason == "" {
				t.Fatalf("应拒绝: %+v", res)
			}
			if n := count(t, db, "SELECT COUNT(*) FROM reading_event WHERE status=2 AND reject_reason LIKE '%"+c.reason+"%'"); n == 0 {
				t.Fatalf("拒绝原因未留痕")
			}
		})
	}
}

func TestEvent_LateWindowBoundary(t *testing.T) {
	_, _, db := setupEvent(t)
	// 接收时间为 11-12 20:00：两个边界发生时间都落在活动期（11-11 19:59 / 20:01），仅窗口判定不同
	now := time.Date(2026, 11, 12, 20, 0, 0, 0, model.Beijing)
	svc := newEventService(db, now)

	// 23h59m：窗口内，受理
	res := handle(t, svc, "e-in", 1, 11, now.Add(-23*time.Hour-time.Minute))
	if res.Status != service.ResultAccepted {
		t.Fatalf("23h59m 应受理: %+v", res)
	}
	// 24h01m：窗口外，拒绝
	res = handle(t, svc, "e-out", 1, 12, now.Add(-24*time.Hour-time.Minute))
	if res.Status != service.ResultRejected {
		t.Fatalf("24h01m 应拒绝: %+v", res)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM reading_event WHERE status=1"); n != 1 {
		t.Fatalf("受理事件数 = %d, want 1", n)
	}
}

func TestEvent_LateEventAttributedToOccurDate(t *testing.T) {
	_, _, db := setupEvent(t)

	// 11-11 23:30 发生、11-12 10:00 送达：迟到但窗口内，归发生日 11-11
	now2 := time.Date(2026, 11, 12, 10, 0, 0, 0, model.Beijing)
	svc2 := newEventService(db, now2)
	res := handle(t, svc2, "e-late", 1, 11, time.Date(2026, 11, 11, 23, 30, 0, 0, model.Beijing))

	if res.Status != service.ResultAccepted || res.BizDate != "2026-11-11" {
		t.Fatalf("迟到事件应归发生日 11-11: %+v", res)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM daily_progress WHERE biz_date='2026-11-11' AND article_id=11"); n != 1 {
		t.Fatal("进度应落在发生日")
	}
}

func TestEvent_AfterActivityEndWithinWindow(t *testing.T) {
	db := testutil.Setup(t)
	seedActivity(t, db)
	// 活动结束后 10 小时送达：仍受理（窗口未关闭），归发生日
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, model.Beijing)
	svc := newEventService(db, now)
	res := handle(t, svc, "e-after", 1, 11, time.Date(2026, 11, 12, 23, 0, 0, 0, model.Beijing))

	if res.Status != service.ResultAccepted {
		t.Fatalf("结束后窗口内事件应受理: %+v", res)
	}
	if res.BizDate != "2026-11-12" {
		t.Fatalf("应归发生日 11-12: %+v", res)
	}
}

// 历史进度已达标但发奖缺失（历史故障/客服批量补入造成跳档）时，
// 后续任一新增都必须补触发发奖；已发过则由 reward 唯一键保证不重复。
func TestEvent_RewardWhenCountAboveThreshold(t *testing.T) {
	svc, activity, db := setupEvent(t)
	ctx := context.Background()
	for _, aid := range []int64{11, 12, 13} {
		if _, err := store.InsertProgress(ctx, db, 1, "2026-11-11", aid, "seed-"+itoa(aid)); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, db, "SELECT COUNT(*) FROM reward WHERE user_id=1"); n != 0 {
		t.Fatalf("铺垫应无发奖记录: %d", n)
	}

	// 第 4 篇到达：count=4 > 门槛 3，仍应补发
	res := handle(t, svc, "e-4th", 1, 14, bj(11, 0))
	if res.Status != service.ResultRewarded || res.RewardPoints != activity.RewardPoints {
		t.Fatalf("已达标但未发奖时应补发: %+v", res)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM reward WHERE user_id=1"); n != 1 {
		t.Fatalf("reward 条数 = %d, want 1", n)
	}
	if b := getBalance(t, db, 1); b != 30 {
		t.Fatalf("balance = %d, want 30", b)
	}

	// 第 5 篇到达：已发过，不重复发奖
	res = handle(t, svc, "e-5th", 1, 15, bj(11, 5))
	if res.Status != service.ResultAccepted {
		t.Fatalf("已发奖后新增不应再次发奖: %+v", res)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM reward WHERE user_id=1"); n != 1 {
		t.Fatalf("reward 条数 = %d, want 1（不得重复）", n)
	}
}

func TestEvent_NoPublishedActivity(t *testing.T) {
	db := testutil.Setup(t) // 不播种活动
	svc := newEventService(db, baseNow)
	res := handle(t, svc, "e-1", 1, 11, bj(10, 0))
	if res.Status != service.ResultRejected {
		t.Fatalf("无活动应拒绝: %+v", res)
	}
}
