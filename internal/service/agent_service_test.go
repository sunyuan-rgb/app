package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"reading-activity/internal/model"
	"reading-activity/internal/service"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

// fakeReader 模拟阅读服务可信查询客户端。
type fakeReader struct {
	records []service.ReadingRecord
	err     error
}

func (f *fakeReader) QueryRecords(ctx context.Context, userID int64, bizDate string) ([]service.ReadingRecord, error) {
	return f.records, f.err
}

func setupAgent(t *testing.T) (*service.AgentService, *service.EventService, *service.UserService, *sql.DB) {
	t.Helper()
	db := testutil.Setup(t)
	seedActivity(t, db)
	events := newEventService(db, baseNow)
	users := service.NewUserService(db, clockFixed(baseNow))
	agent := service.NewAgentService(db, clockFixed(baseNow), events, nil)
	return agent, events, users, db
}

// 铺垫：用户已有 2 篇有效文章并提交一条申诉，返回申诉 ID。
func seedAppealWithTwoArticles(t *testing.T, events *service.EventService, users *service.UserService, userID int64) int64 {
	t.Helper()
	ctx := context.Background()
	for i, aid := range []int64{21, 22} {
		if _, err := events.HandleEvent(ctx, service.EventInput{
			EventID: itoa(userID) + "-a" + itoa(int64(i+1)), UserID: userID, ArticleID: aid, OccurAt: bj(9, 0),
		}, model.SourceAuto); err != nil {
			t.Fatal(err)
		}
	}
	v, err := users.SubmitAppeal(ctx, userID, "2026-11-11", "第三篇已读完但奖励未到账")
	if err != nil {
		t.Fatal(err)
	}
	return v.ID
}

func TestAgent_ListAndDetail(t *testing.T) {
	agent, events, users, _ := setupAgent(t)
	aid := seedAppealWithTwoArticles(t, events, users, 4)

	items, err := agent.ListAppeals(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != aid || items[0].Status != store.AppealOpen {
		t.Fatalf("申诉列表不符: %+v", items)
	}
	// 按状态过滤：待处理过滤命中，已结过滤为空
	if items, _ := agent.ListAppeals(context.Background(), ptrInt8(store.AppealOpen)); len(items) != 1 {
		t.Fatalf("open 过滤应命中")
	}
	if items, _ := agent.ListAppeals(context.Background(), ptrInt8(store.AppealCompensed)); len(items) != 0 {
		t.Fatalf("compensated 过滤应为空")
	}

	d, err := agent.GetAppealDetail(context.Background(), aid)
	if err != nil {
		t.Fatal(err)
	}
	if d.Appeal.ID != aid || len(d.AcceptedEvents) != 2 {
		t.Fatalf("核查详情不符: appeal=%+v events=%d", d.Appeal, len(d.AcceptedEvents))
	}
	if d.Progress == nil || d.Progress.DistinctCount != 2 || d.Progress.Reached {
		t.Fatalf("进度证据不符: %+v", d.Progress)
	}
	if d.TrustedNote == "" {
		t.Fatal("未配置可信查询时应注明")
	}
	if _, err := agent.GetAppealDetail(context.Background(), 9999); !errors.Is(err, service.ErrAppealNotFound) {
		t.Fatalf("不存在申诉应 ErrAppealNotFound: %v", err)
	}
}

func TestAgent_DetailWithTrustedReader(t *testing.T) {
	agent, events, users, db := setupAgent(t)
	aid := seedAppealWithTwoArticles(t, events, users, 4)

	// 换一个带可信查询的客服服务实例
	withReader := service.NewAgentService(db, clockFixed(baseNow), events, &fakeReader{
		records: []service.ReadingRecord{{EventID: "rs-1", ArticleID: 23}},
	})
	d, err := withReader.GetAppealDetail(context.Background(), aid)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.TrustedRecords) != 1 || d.TrustedRecords[0].ArticleID != 23 {
		t.Fatalf("可信记录应展示: %+v", d.TrustedRecords)
	}
	_ = agent
}

func TestAgent_CompensateRewardsAndResolves(t *testing.T) {
	agent, events, users, db := setupAgent(t)
	aid := seedAppealWithTwoArticles(t, events, users, 4)

	res, err := agent.CompensateAppeal(context.Background(), aid, 6, "agent-u4-1", 23, bj(9, 15))
	if err != nil {
		t.Fatal(err)
	}
	if res.HandleResult.Status != service.ResultRewarded || !res.Resolved || res.RewardID == 0 {
		t.Fatalf("补入应触发发奖并结案: %+v", res)
	}
	if b := getBalance(t, db, 4); b != 30 {
		t.Fatalf("balance = %d", b)
	}
	appeal, _ := store.GetAppealByID(context.Background(), db, aid)
	if appeal.Status != store.AppealCompensed || appeal.OperatorID != 6 {
		t.Fatalf("结案留痕不符: %+v", appeal)
	}
	// 事件来源应为客服补入
	if n := count(t, db, "SELECT COUNT(*) FROM reading_event WHERE event_id='agent-u4-1' AND source=2"); n != 1 {
		t.Fatal("补入事件 source 应为客服")
	}
	// 已结申诉再补入 → 409
	if _, err := agent.CompensateAppeal(context.Background(), aid, 6, "agent-u4-2", 24, bj(9, 20)); !errors.Is(err, service.ErrAppealProcessed) {
		t.Fatalf("已结申诉应 ErrAppealProcessed: %v", err)
	}
	// 自动路径重送同编号事件 → 幂等，不重复发奖
	res2, err := events.HandleEvent(context.Background(), service.EventInput{
		EventID: "agent-u4-1", UserID: 4, ArticleID: 23, OccurAt: bj(9, 15),
	}, model.SourceAuto)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Status != service.ResultDupEvent {
		t.Fatalf("重送应幂等: %s", res2.Status)
	}
	if b := getBalance(t, db, 4); b != 30 {
		t.Fatalf("余额不应变化: %d", b)
	}
}

func TestAgent_CompensateValidation(t *testing.T) {
	agent, events, users, _ := setupAgent(t)
	aid := seedAppealWithTwoArticles(t, events, users, 4)
	ctx := context.Background()

	if _, err := agent.CompensateAppeal(ctx, aid, 6, "  ", 23, bj(9, 15)); err == nil {
		t.Fatal("缺 event_id 应报错")
	}
	if _, err := agent.CompensateAppeal(ctx, aid, 6, "agent-x", 0, bj(9, 15)); err == nil {
		t.Fatal("article_id 非法应报错")
	}
	if _, err := agent.CompensateAppeal(ctx, 9999, 6, "agent-x", 23, bj(9, 15)); !errors.Is(err, service.ErrAppealNotFound) {
		t.Fatal("不存在申诉应 ErrAppealNotFound")
	}
	// 补入超窗事件：走统一校验被拒绝，申诉不结案
	res, err := agent.CompensateAppeal(ctx, aid, 6, "agent-late", 23, baseNow.Add(-25*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if res.HandleResult.Status != service.ResultRejected || res.Resolved {
		t.Fatalf("超窗补入应拒绝且不结案: %+v", res)
	}
}

func TestAgent_RejectFlow(t *testing.T) {
	agent, events, users, db := setupAgent(t)
	aid := seedAppealWithTwoArticles(t, events, users, 5)
	ctx := context.Background()

	if err := agent.RejectAppeal(ctx, aid, 6, "   "); err == nil {
		t.Fatal("空依据应报错")
	}
	if err := agent.RejectAppeal(ctx, aid, 6, "当日仅计入2篇，未达门槛"); err != nil {
		t.Fatal(err)
	}
	appeal, _ := store.GetAppealByID(ctx, db, aid)
	if appeal.Status != store.AppealRejected || appeal.OperatorID != 6 {
		t.Fatalf("驳回留痕不符: %+v", appeal)
	}
	// 重复驳回 → 已处理
	if err := agent.RejectAppeal(ctx, aid, 6, "再来一次"); !errors.Is(err, service.ErrAppealProcessed) {
		t.Fatalf("重复驳回应 ErrAppealProcessed: %v", err)
	}
	// 驳回后积分不变
	if b := getBalance(t, db, 5); b != 0 {
		t.Fatalf("驳回不应发奖: %d", b)
	}
	_ = events
}

func TestAgent_CompensateDuplicateArticleDoesNotResolve(t *testing.T) {
	agent, events, users, _ := setupAgent(t)
	aid := seedAppealWithTwoArticles(t, events, users, 4)
	// 补入一篇当日已计文章 → dup_article，申诉保持待处理
	res, err := agent.CompensateAppeal(context.Background(), aid, 6, "agent-dup", 21, bj(9, 30))
	if err != nil {
		t.Fatal(err)
	}
	if res.HandleResult.Status != service.ResultDupArticle || res.Resolved {
		t.Fatalf("重复文章补入不应结案: %+v", res)
	}
}

func ptrInt8(v int8) *int8 { return &v }
