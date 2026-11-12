package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"reading-activity/internal/model"
	"reading-activity/internal/service"
	"reading-activity/internal/testutil"
)

func setupOps(t *testing.T) (*service.OpsService, *service.EventService, *sql.DB) {
	t.Helper()
	db := testutil.Setup(t)
	seedActivity(t, db)
	seedAccounts(t, db)
	return service.NewOpsService(db, clockFixed(baseNow)), newEventService(db, baseNow), db
}

func TestOps_PublishValidation(t *testing.T) {
	db := testutil.Setup(t)
	ops := service.NewOpsService(db, clockFixed(baseNow))
	ctx := context.Background()

	in := service.PublishInput{
		Name: "新活动", StartAt: bj(0, 0).AddDate(0, 1, 0), EndAt: bj(0, 0).AddDate(0, 1, 1),
		DailyThreshold: 3, RewardPoints: 30,
	}
	// 合法发布
	v, err := ops.PublishActivity(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID == 0 {
		t.Fatalf("应返回活动 ID: %+v", v)
	}
	// 重复发布 → ErrActivityExists
	if _, err := ops.PublishActivity(ctx, in); !errors.Is(err, service.ErrActivityExists) {
		t.Fatalf("重复发布应 ErrActivityExists: %v", err)
	}
	// 参数校验
	bad := in
	bad.Name = " "
	if _, err := ops.PublishActivity(ctx, bad); !errors.Is(err, service.ErrActivityInput) {
		t.Fatal("空名称应 ErrActivityInput")
	}
	bad = in
	bad.DailyThreshold = 0
	if _, err := ops.PublishActivity(ctx, bad); !errors.Is(err, service.ErrActivityInput) {
		t.Fatal("门槛 0 应 ErrActivityInput")
	}
	bad = in
	bad.EndAt = in.StartAt
	if _, err := ops.PublishActivity(ctx, bad); err == nil {
		t.Fatal("结束不晚于开始应报错")
	}
	bad = in
	bad.EndAt = baseNow.Add(-time.Hour) // 结束早于当前时间
	if _, err := ops.PublishActivity(ctx, bad); err == nil {
		t.Fatal("结束早于当前时间应报错")
	}
}

func TestOps_SummaryConsistency(t *testing.T) {
	ops, events, db := setupOps(t)
	ctx := context.Background()

	// day1: user1、user2 达标发奖
	for _, u := range []int64{1, 2} {
		for i, aid := range []int64{11, 12, 13} {
			if _, err := events.HandleEvent(ctx, service.EventInput{
				EventID: "s-" + itoa(u) + "-" + itoa(int64(i+1)), UserID: u, ArticleID: aid, OccurAt: bj(9, 0),
			}, model.SourceAuto); err != nil {
				t.Fatal(err)
			}
		}
	}
	// day2（业务日 11-12）: user3 达标发奖
	now2 := time.Date(2026, 11, 12, 20, 0, 0, 0, model.Beijing)
	events2 := newEventService(db, now2)
	for i, aid := range []int64{31, 32, 33} {
		if _, err := events2.HandleEvent(ctx, service.EventInput{
			EventID: "s3-" + itoa(int64(i+1)), UserID: 3, ArticleID: aid, OccurAt: now2.Add(-time.Hour),
		}, model.SourceAuto); err != nil {
			t.Fatal(err)
		}
	}

	v, err := ops.GetSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Days) != 2 {
		t.Fatalf("应有 2 个业务日（11-11 / 11-12）: %d", len(v.Days))
	}
	day1, day2 := v.Days[0], v.Days[1]
	if day1.BizDate != "2026-11-11" || day2.BizDate != "2026-11-12" {
		t.Fatalf("业务日不符: %s / %s", day1.BizDate, day2.BizDate)
	}
	if day1.ReachedUsers != 2 || day1.RewardedUsers != 2 || day1.TotalPoints != 60 {
		t.Fatalf("day1 汇总不符: %+v", day1)
	}
	if day2.ReachedUsers != 1 || day2.RewardedUsers != 1 || day2.TotalPoints != 30 {
		t.Fatalf("day2 汇总不符: %+v", day2)
	}
	if len(v.Inconsistencies) != 0 {
		t.Fatalf("不应有不一致项: %+v", v.Inconsistencies)
	}
	if len(v.Rewards) != 3 {
		t.Fatalf("发奖明细应 3 条: %d", len(v.Rewards))
	}
	if v.WindowClosed {
		t.Fatal("结束+24h 未到，窗口未关闭")
	}
}

func TestOps_SummaryDetectsInconsistency(t *testing.T) {
	ops, events, db := setupOps(t)
	ctx := context.Background()

	// user1 正常达标发奖
	for i, aid := range []int64{11, 12, 13} {
		if _, err := events.HandleEvent(ctx, service.EventInput{
			EventID: "inc-" + itoa(int64(i+1)), UserID: 1, ArticleID: aid, OccurAt: bj(9, 0),
		}, model.SourceAuto); err != nil {
			t.Fatal(err)
		}
	}
	// 人为制造"发奖未达标"脏数据（模拟故障）：user2 直插发奖三件套
	if _, err := db.Exec(`INSERT INTO reward (user_id, activity_id, biz_date, points) VALUES (2, 1, '2026-11-11', 30)`); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO point_txn (user_id, type, points, ref_type, ref_id) VALUES (2, 1, 30, 'fault', 1)`)
	db.Exec(`INSERT INTO user_balance (user_id, balance) VALUES (2, 30)`)

	v, err := ops.GetSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, inc := range v.Inconsistencies {
		if inc.Type == "rewarded_not_reached" && inc.BizDate == "2026-11-11" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应检出 rewarded_not_reached: %+v", v.Inconsistencies)
	}
	if v.Days[0].ReachedUsers != 1 || v.Days[0].RewardedUsers != 2 {
		t.Fatalf("汇总应反映不一致: %+v", v.Days[0])
	}
}

func TestOps_SummaryWindowClosed(t *testing.T) {
	db := testutil.Setup(t)
	seedActivity(t, db)
	// 时钟推到结束+24h 之后：窗口关闭
	later := time.Date(2026, 11, 14, 0, 1, 0, 0, model.Beijing)
	ops := service.NewOpsService(db, clockFixed(later))
	v, err := ops.GetSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !v.WindowClosed {
		t.Fatal("结束+24h 后窗口应关闭")
	}
}

func TestOps_SummaryNoActivity(t *testing.T) {
	db := testutil.Setup(t)
	ops := service.NewOpsService(db, clockFixed(baseNow))
	if _, err := ops.GetSummary(context.Background()); err == nil {
		t.Fatal("无活动应报错")
	}
}

var _ = model.SourceAuto
