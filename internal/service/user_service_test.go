package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"reading-activity/internal/model"
	"reading-activity/internal/service"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

func setupUser(t *testing.T) (*service.UserService, *service.EventService, *sql.DB) {
	t.Helper()
	db := testutil.Setup(t)
	seedActivity(t, db)
	return service.NewUserService(db, clockFixed(baseNow)), newEventService(db, baseNow), db
}

func TestUser_GetActivityView(t *testing.T) {
	svc, _, db := setupUser(t)
	v, err := svc.GetActivity(context.Background())
	if err != nil || v == nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if v.DailyThreshold != 3 || v.RewardPoints != 30 || v.Name != "测试活动" {
		t.Fatalf("活动视图不符: %+v", v)
	}
	if v.StartAt[:10] != "2026-11-11" || v.EndAt[:10] != "2026-11-13" {
		t.Fatalf("时间应为北京时间: %s ~ %s", v.StartAt, v.EndAt)
	}
	_ = db
}

func TestUser_ProgressAndPoints(t *testing.T) {
	svc, events, _ := setupUser(t)
	ctx := context.Background()
	for _, aid := range []int64{11, 12, 13} {
		if _, err := events.HandleEvent(ctx, service.EventInput{
			EventID: "up-" + itoa(aid), UserID: 1, ArticleID: aid, OccurAt: bj(9, 0),
		}, model.SourceAuto); err != nil {
			t.Fatal(err)
		}
	}

	p, err := svc.GetProgress(ctx, 1, "2026-11-11")
	if err != nil {
		t.Fatal(err)
	}
	if p.DistinctCount != 3 || !p.Reached || !p.Rewarded || p.RewardPoints != 30 {
		t.Fatalf("进度不符: %+v", p)
	}

	// 空进度日：文章列表必须是空数组而非 null
	p0, err := svc.GetProgress(ctx, 2, "2026-11-11")
	if err != nil {
		t.Fatal(err)
	}
	if p0.Articles == nil || len(p0.Articles) != 0 {
		t.Fatalf("空进度 Articles 应为空数组: %+v", p0)
	}

	pts, err := svc.GetPoints(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pts.Balance != 30 || len(pts.Txns) != 1 {
		t.Fatalf("积分不符: %+v", pts)
	}
}

func TestUser_ProgressInvalidDate(t *testing.T) {
	svc, _, _ := setupUser(t)
	if _, err := svc.GetProgress(context.Background(), 1, "2026/11/11"); err == nil {
		t.Fatal("非法日期应报错")
	}
}

func TestUser_SubmitAppeal(t *testing.T) {
	svc, _, db := setupUser(t)
	ctx := context.Background()

	v, err := svc.SubmitAppeal(ctx, 1, "2026-11-11", "  第三篇奖励未到账  ")
	if err != nil {
		t.Fatal(err)
	}
	if v.ID == 0 || v.Status != "待处理" {
		t.Fatalf("申诉视图不符: %+v", v)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM appeal WHERE user_id=1"); n != 1 {
		t.Fatalf("申诉条数 = %d", n)
	}

	// 同日第二条未结申诉 → 唯一键拒绝
	if _, err := svc.SubmitAppeal(ctx, 1, "2026-11-11", "再来一条"); !errors.Is(err, service.ErrAppealExists) {
		t.Fatalf("应返回 ErrAppealExists: %v", err)
	}

	// 内容校验
	if _, err := svc.SubmitAppeal(ctx, 1, "2026-11-12", "   "); err == nil {
		t.Fatal("空内容应报错")
	}
	if _, err := svc.SubmitAppeal(ctx, 1, "2026-11-12", string(make([]byte, 2000))); err == nil {
		t.Fatal("超长内容应报错")
	}
	// 活动期间外的业务日
	if _, err := svc.SubmitAppeal(ctx, 1, "2026-11-01", "活动开始前"); err == nil {
		t.Fatal("活动期外申诉应报错")
	}
	// 非法日期
	if _, err := svc.SubmitAppeal(ctx, 1, "bad-date", "x"); err == nil {
		t.Fatal("非法日期应报错")
	}
}

func TestUser_AppealDateOutsideActivity(t *testing.T) {
	svc, _, _ := setupUser(t)
	// 11-13 含活动结束时刻（11-13 00:00:00+08），属合法业务日；11-14 才算期外
	if _, err := svc.SubmitAppeal(context.Background(), 1, "2026-11-14", "结束日之后"); err == nil {
		t.Fatal("活动期外申诉应报错")
	}
	if _, err := svc.SubmitAppeal(context.Background(), 1, "2026-11-13", "结束时刻当日"); err != nil {
		t.Fatalf("结束日申诉应成功: %v", err)
	}
}

func TestUser_AppealDateInsideActivityEndDay(t *testing.T) {
	svc, _, _ := setupUser(t)
	// 活动期最后一天（11-12）可以申诉
	if _, err := svc.SubmitAppeal(context.Background(), 1, "2026-11-12", "结束日申诉"); err != nil {
		t.Fatalf("活动期内申诉应成功: %v", err)
	}
}

var _ = store.AppealOpen
