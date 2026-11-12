package handler_test

import (
	"context"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/server/handler"
	"reading-activity/internal/service"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

// 真实测试库 + 直接调用 handler：覆盖 HTTP 层正常路径（状态码与响应体）。
func TestHandler_HappyPaths(t *testing.T) {
	db := testutil.Setup(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)

	// 活动（发布状态）
	if _, err := db.Exec(`INSERT INTO activity (name, start_at, end_at, daily_threshold, reward_points, status)
		VALUES ('每日阅读三篇文章得积分', '2026-11-10 16:00:00', '2026-11-12 16:00:00', 3, 30, 1)`); err != nil {
		t.Fatal(err)
	}
	// 账号
	for i := 1; i <= 6; i++ {
		role := "user"
		if i == 6 {
			role = "agent"
		}
		if _, err := db.Exec(`INSERT INTO user_account (id, username, role, token) VALUES (?, ?, ?, ?)`,
			i, "acct"+string(rune('0'+i)), role, "tok-"+string(rune('0'+i))); err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	clk := clock.Fixed{T: now}
	events := service.NewEventService(db, clk)
	users := service.NewUserService(db, clk)
	agent := service.NewAgentService(db, clk, events, nil)
	ops := service.NewOpsService(db, clk)
	eventH := handler.NewEventHandler(events)
	userH := handler.NewUserHandler(users)
	agentH := handler.NewAgentHandler(agent)
	opsH := handler.NewOpsHandler(ops)

	// 事件接入 200
	c, w := testCtx("POST", "/x",
		`{"event_id":"h-1","user_id":1,"article_id":11,"occur_at":"2026-11-11T20:00:00+08:00"}`)
	eventH.Create(c)
	if w.Code != 200 {
		t.Fatalf("event => %d %s", w.Code, w.Body.String())
	}

	// 活动 200
	c, w = testCtx("GET", "/x", "")
	userH.Activity(c)
	if w.Code != 200 {
		t.Fatalf("activity => %d", w.Code)
	}
	// 进度 200（指定日期）
	c, w = testCtx("GET", "/x?date=2026-11-11", "")
	c.Set("uid", int64(1))
	userH.Progress(c)
	if w.Code != 200 {
		t.Fatalf("progress => %d %s", w.Code, w.Body.String())
	}
	// 积分 200
	c, w = testCtx("GET", "/x", "")
	c.Set("uid", int64(1))
	userH.Points(c)
	if w.Code != 200 {
		t.Fatalf("points => %d", w.Code)
	}
	// 提交申诉 200/201
	c, w = testCtx("POST", "/x", `{"date":"2026-11-11","content":"第三篇未计"}`)
	c.Set("uid", int64(1))
	userH.SubmitAppeal(c)
	if w.Code < 200 || w.Code >= 300 {
		t.Fatalf("appeal => %d %s", w.Code, w.Body.String())
	}

	// 客服列表（带状态过滤）200
	c, w = testCtx("GET", "/x?status=0", "")
	agentH.ListAppeals(c)
	if w.Code != 200 {
		t.Fatalf("list => %d", w.Code)
	}
	// 详情 200
	items, err := store.ListAppeals(ctx, db, nil)
	if err != nil || len(items) == 0 {
		t.Fatalf("seed appeal: %v", err)
	}
	aid := items[0].ID
	c, w = testCtx("GET", "/x", "")
	c.Params = gin.Params{{Key: "id", Value: itoa64(aid)}}
	agentH.GetAppealDetail(c)
	if w.Code != 200 {
		t.Fatalf("detail => %d %s", w.Code, w.Body.String())
	}
	// 补入 200（article 12 触发第 3 篇？用户仅 1 篇，不触发发奖）
	c, w = testCtx("POST", "/x",
		`{"event_id":"h-2","article_id":12,"occur_at":"2026-11-11T20:10:00+08:00"}`)
	c.Params = gin.Params{{Key: "id", Value: itoa64(aid)}}
	c.Set("uid", int64(6))
	agentH.CompensateAppeal(c)
	if w.Code != 200 {
		t.Fatalf("compensate => %d %s", w.Code, w.Body.String())
	}
	// 驳回另一条申诉 200
	if _, id, err := store.InsertAppeal(ctx, db, 2, 1, "2026-11-11", "无奖励"); err != nil || id == 0 {
		t.Fatalf("seed appeal2: %v", err)
	} else {
		c, w = testCtx("POST", "/x", `{"conclusion":"当日仅1篇，未达门槛"}`)
		c.Params = gin.Params{{Key: "id", Value: itoa64(id)}}
		c.Set("uid", int64(6))
		agentH.RejectAppeal(c)
		if w.Code != 200 {
			t.Fatalf("reject => %d %s", w.Code, w.Body.String())
		}
	}

	// 运营汇总 200
	c, w = testCtx("GET", "/x", "")
	opsH.GetSummary(c)
	if w.Code != 200 {
		t.Fatalf("summary => %d %s", w.Code, w.Body.String())
	}
	// 运营发布：已有已发布活动 → 409；无活动的空库 → 200
	c, w = testCtx("POST", "/x",
		`{"name":"新活动","start_at":"2026-11-14T00:00:00+08:00","end_at":"2026-11-16T00:00:00+08:00","daily_threshold":3,"reward_points":30}`)
	opsH.PublishActivity(c)
	if w.Code != 409 {
		t.Fatalf("重复发布 => %d", w.Code)
	}
	fresh := testutil.Setup(t)
	opsH2 := handler.NewOpsHandler(service.NewOpsService(fresh, clk))
	c, w = testCtx("POST", "/x",
		`{"name":"新活动","start_at":"2026-11-14T00:00:00+08:00","end_at":"2026-11-16T00:00:00+08:00","daily_threshold":3,"reward_points":30}`)
	opsH2.PublishActivity(c)
	if w.Code != 200 {
		t.Fatalf("发布 => %d %s", w.Code, w.Body.String())
	}
}

func itoa64(n int64) string {
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
