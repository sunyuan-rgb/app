package service_test

import (
	"context"
	"testing"
	"time"

	"reading-activity/internal/service"
	"reading-activity/internal/testutil"
)

// 空库（无活动）：各视图的"无活动"分支。
func TestService_NoActivityBranches(t *testing.T) {
	db := testutil.Setup(t) // 不播种活动
	clk := clockFixed(baseNow)
	users := service.NewUserService(db, clk)
	ops := service.NewOpsService(db, clk)

	if v, err := users.GetActivity(context.Background()); err != nil || v != nil {
		t.Fatalf("无活动应返回 nil: %v %v", v, err)
	}
	if v, err := users.GetProgress(context.Background(), 1, "2026-11-11"); err != nil {
		t.Fatal(err)
	} else if v.Threshold != 0 || v.Reached || v.Rewarded {
		t.Fatalf("无活动进度不应有门槛: %+v", v)
	}
	if _, err := ops.GetSummary(context.Background()); err == nil {
		t.Fatal("无活动汇总应报错")
	}
}

// 发布活动：结束时间早于当前 → 拒绝。
func TestOps_PublishEndInPast(t *testing.T) {
	ops, _, _ := setupOps(t)
	if _, err := ops.PublishActivity(context.Background(), service.PublishInput{
		Name: "过期活动", StartAt: baseNow.Add(-3 * time.Hour), EndAt: baseNow.Add(-2 * time.Hour),
		DailyThreshold: 3, RewardPoints: 30,
	}); err == nil {
		t.Fatal("结束时间早于当前应报错")
	}
}

// 客服核查：可信查询失败时降级为说明文字，核查本身仍可完成。
func TestAgent_DetailTrustedQueryError(t *testing.T) {
	_, events, users, db := setupAgent(t)
	aid := seedAppealWithTwoArticles(t, events, users, 4)

	broken := service.NewAgentService(db, clockFixed(baseNow), events, &fakeReader{err: errTimeout{}})
	d, err := broken.GetAppealDetail(context.Background(), aid)
	if err != nil {
		t.Fatal(err)
	}
	if d.TrustedNote == "" || len(d.TrustedRecords) != 0 {
		t.Fatalf("可信查询失败应写入说明: %+v", d)
	}
}

type errTimeout struct{}

func (errTimeout) Error() string { return "dial timeout" }

// 可信查询客户端：非法 URL 与连接失败两个错误分支。
func TestHTTPReadingService_TransportErrors(t *testing.T) {
	badURL := service.NewHTTPReadingService("http://exa mple.com")
	if _, err := badURL.QueryRecords(context.Background(), 1, "2026-11-11"); err == nil {
		t.Fatal("非法 URL 应报错")
	}
	unreachable := service.NewHTTPReadingService("http://127.0.0.1:1")
	if _, err := unreachable.QueryRecords(context.Background(), 1, "2026-11-11"); err == nil {
		t.Fatal("连接失败应报错")
	}
}
