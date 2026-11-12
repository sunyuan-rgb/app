package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/server/handler"
	"reading-activity/internal/service"
	"reading-activity/internal/testutil"
)

// 注入错误的服务 + 直接调用 handler：覆盖 HTTP 层数据库错误分支（500/400）。
func newHandlers() (*handler.EventHandler, *handler.UserHandler, *handler.AgentHandler, *handler.OpsHandler) {
	db := testutil.NewErrDB()
	clk := clock.Fixed{T: time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)}
	gin.SetMode(gin.TestMode)
	return handler.NewEventHandler(service.NewEventService(db, clk)),
		handler.NewUserHandler(service.NewUserService(db, clk)),
		handler.NewAgentHandler(service.NewAgentService(db, clk, service.NewEventService(db, clk), nil)),
		handler.NewOpsHandler(service.NewOpsService(db, clk))
}

func testCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

func TestHandler_DBErrorBranches(t *testing.T) {
	eventH, userH, agentH, opsH := newHandlers()

	// 事件接入：服务错误 → 500
	c, w := testCtx("POST", "/api/v1/events",
		`{"event_id":"e1","user_id":1,"article_id":1,"occur_at":"2026-11-11T20:00:00+08:00"}`)
	eventH.Create(c)
	if w.Code != 500 {
		t.Fatalf("event 500 expected, got %d", w.Code)
	}

	// 用户端：服务错误 → Activity 500 / Progress 400 / Points 500 / SubmitAppeal 400
	c, w = testCtx("GET", "/x", "")
	userH.Activity(c)
	if w.Code != 500 {
		t.Fatalf("activity 500 expected, got %d", w.Code)
	}
	c, w = testCtx("GET", "/x", "")
	userH.Progress(c)
	if w.Code != 400 {
		t.Fatalf("progress 400 expected, got %d", w.Code)
	}
	c, w = testCtx("GET", "/x", "")
	userH.Points(c)
	if w.Code != 500 {
		t.Fatalf("points 500 expected, got %d", w.Code)
	}
	c, w = testCtx("POST", "/x", `{"date":"2026-11-11","content":"内容"}`)
	userH.SubmitAppeal(c)
	if w.Code != 400 {
		t.Fatalf("appeal 400 expected, got %d", w.Code)
	}

	// 客服端：列表 500；驳回 / 补入数据库错误 → 400（mapAppealErr default 分支）
	c, w = testCtx("GET", "/x", "")
	agentH.ListAppeals(c)
	if w.Code != 500 {
		t.Fatalf("list 500 expected, got %d", w.Code)
	}
	c, w = testCtx("POST", "/x", `{"conclusion":"依据"}`)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	agentH.RejectAppeal(c)
	if w.Code != 400 {
		t.Fatalf("reject 400 expected, got %d", w.Code)
	}
	c, w = testCtx("POST", "/x",
		`{"event_id":"e9","article_id":1,"occur_at":"2026-11-11T20:00:00+08:00"}`)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	agentH.CompensateAppeal(c)
	if w.Code != 400 {
		t.Fatalf("compensate 400 expected, got %d", w.Code)
	}

	// 运营端：发布 400（default 分支）/ 汇总 400
	c, w = testCtx("POST", "/x",
		`{"name":"x","start_at":"2026-11-14T00:00:00+08:00","end_at":"2026-11-15T00:00:00+08:00","daily_threshold":3,"reward_points":30}`)
	opsH.PublishActivity(c)
	if w.Code != 400 {
		t.Fatalf("publish 400 expected, got %d", w.Code)
	}
	c, w = testCtx("GET", "/x", "")
	opsH.GetSummary(c)
	if w.Code != 400 {
		t.Fatalf("summary 400 expected, got %d", w.Code)
	}
}

func TestHandler_ParamErrorBranches(t *testing.T) {
	// 真实空库服务测参数分支（不触发数据库写入）
	db := testutil.Setup(t)
	clk := clock.Fixed{T: time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)}
	gin.SetMode(gin.TestMode)
	eventH := handler.NewEventHandler(service.NewEventService(db, clk))
	agentH := handler.NewAgentHandler(service.NewAgentService(db, clk, service.NewEventService(db, clk), nil))
	userH := handler.NewUserHandler(service.NewUserService(db, clk))

	// 事件缺字段 → 400
	c, w := testCtx("POST", "/x", `{"event_id":"e1"}`)
	eventH.Create(c)
	if w.Code != 400 {
		t.Fatalf("event bind 400 expected, got %d", w.Code)
	}

	// 申诉 ID 不合法 → 400（详情）
	c, w = testCtx("GET", "/x", "")
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	agentH.GetAppealDetail(c)
	if w.Code != 400 {
		t.Fatalf("bad appeal id 400 expected, got %d", w.Code)
	}
	// 申诉 ID 不合法 → 400（驳回）
	c, w = testCtx("POST", "/x", `{"conclusion":"x"}`)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	agentH.RejectAppeal(c)
	if w.Code != 400 {
		t.Fatalf("bad appeal id reject 400 expected, got %d", w.Code)
	}
	// 申诉 ID 不合法 → 400（补入）
	c, w = testCtx("POST", "/x",
		`{"event_id":"e9","article_id":1,"occur_at":"2026-11-11T20:00:00+08:00"}`)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	agentH.CompensateAppeal(c)
	if w.Code != 400 {
		t.Fatalf("bad appeal id compensate 400 expected, got %d", w.Code)
	}

	// 驳回缺 conclusion → 400（bind 分支）
	c, w = testCtx("POST", "/x", `{}`)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	agentH.RejectAppeal(c)
	if w.Code != 400 {
		t.Fatalf("reject bind 400 expected, got %d", w.Code)
	}
	// 补入缺 event_id → 400（bind 分支）
	c, w = testCtx("POST", "/x", `{}`)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	agentH.CompensateAppeal(c)
	if w.Code != 400 {
		t.Fatalf("compensate bind 400 expected, got %d", w.Code)
	}
	// 申诉缺内容 → 400（bind 分支）
	c, w = testCtx("POST", "/x", `{"date":"2026-11-11"}`)
	userH.SubmitAppeal(c)
	if w.Code != 400 {
		t.Fatalf("appeal bind 400 expected, got %d", w.Code)
	}
	// 发布活动坏 JSON → 400（bind 分支）
	_, _, agentH2, _ := newHandlers()
	opsH2 := handler.NewOpsHandler(service.NewOpsService(testutil.NewErrDB(),
		clock.Fixed{T: time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)}))
	c, w = testCtx("POST", "/x", `{"name":`)
	opsH2.PublishActivity(c)
	if w.Code != 400 {
		t.Fatalf("publish bind 400 expected, got %d", w.Code)
	}
	// 申诉列表 status 参数不合法 → 400
	c, w = testCtx("GET", "/x?status=abc", "")
	agentH2.ListAppeals(c)
	if w.Code != 400 {
		t.Fatalf("status bind 400 expected, got %d", w.Code)
	}
}
