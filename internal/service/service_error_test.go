package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/service"
	"reading-activity/internal/testutil"
)

// TestServiceErrorBranches 数据库错误必须向上传播且不得 panic。
// 使用注入错误的 *sql.DB 确定性覆盖服务层错误分支。
func TestServiceErrorBranches(t *testing.T) {
	db := testutil.NewErrDB()
	defer db.Close()
	clk := clock.Fixed{T: baseNow}
	ctx := context.Background()

	events := service.NewEventService(db, clk)
	in := service.EventInput{EventID: "e1", UserID: 1, ArticleID: 1, OccurAt: baseNow.Add(-time.Hour)}

	// 事件接入：事务前校验通过后，活动查询出错
	if _, err := events.HandleEvent(ctx, in, model.SourceAuto); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("HandleEvent: %v", err)
	}
	// 拒绝留痕路径中查询活动出错
	if _, err := events.HandleEvent(ctx, service.EventInput{
		EventID: "e2", UserID: 1, ArticleID: 1, OccurAt: baseNow.Add(2 * time.Hour), // 未来事件 → reject
	}, model.SourceAuto); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("HandleEvent(reject): %v", err)
	}

	users := service.NewUserService(db, clk)
	if _, err := users.GetActivity(ctx); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetActivity: %v", err)
	}
	if _, err := users.GetProgress(ctx, 1, "2026-11-11"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetProgress: %v", err)
	}
	if _, err := users.GetPoints(ctx, 1); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetPoints: %v", err)
	}
	if _, err := users.SubmitAppeal(ctx, 1, "2026-11-11", "内容"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("SubmitAppeal: %v", err)
	}

	agent := service.NewAgentService(db, clk, events, nil)
	if _, err := agent.ListAppeals(ctx, nil); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("ListAppeals: %v", err)
	}
	if _, err := agent.GetAppealDetail(ctx, 1); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetAppealDetail: %v", err)
	}
	if err := agent.RejectAppeal(ctx, 1, 6, "依据"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("RejectAppeal: %v", err)
	}
	if _, err := agent.CompensateAppeal(ctx, 1, 6, "e9", 1, baseNow.Add(-time.Hour)); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("CompensateAppeal: %v", err)
	}

	ops := service.NewOpsService(db, clk)
	if _, err := ops.GetSummary(ctx); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("GetSummary: %v", err)
	}
	if _, err := ops.PublishActivity(ctx, service.PublishInput{
		Name: "x", StartAt: baseNow.Add(time.Hour), EndAt: baseNow.Add(2 * time.Hour),
		DailyThreshold: 3, RewardPoints: 30,
	}); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("PublishActivity: %v", err)
	}
}
