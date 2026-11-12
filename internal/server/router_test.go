package server_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/server"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

// 北京时间 2026-11-11 21:00（活动首日晚间，迟到窗口内）。
var baseNow = time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)

const eventsToken = "test-events-token"

func newRouter(t *testing.T) (*gin.Engine, *sql.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.Setup(t)
	seedActivity(t, db)
	seedAccounts(t, db)
	return server.New(server.Deps{DB: db, Clk: clock.Fixed{T: baseNow}, EventsToken: eventsToken}), db
}

func seedActivity(t *testing.T, db *sql.DB) {
	t.Helper()
	ok, err := store.CreateActivity(t.Context(), db, &store.ActivityRow{
		Name:           "测试活动",
		StartAt:        time.Date(2026, 11, 11, 0, 0, 0, 0, model.Beijing),
		EndAt:          time.Date(2026, 11, 13, 0, 0, 0, 0, model.Beijing),
		DailyThreshold: 3,
		RewardPoints:   30,
	})
	if err != nil || !ok {
		t.Fatalf("seed activity: ok=%v err=%v", ok, err)
	}
}

func seedAccounts(t *testing.T, db *sql.DB) {
	t.Helper()
	rows := []struct {
		id   int
		name string
		role string
	}{
		{1, "user1", "user"}, {2, "user2", "user"}, {3, "user3", "user"},
		{4, "user4", "user"}, {5, "user5", "user"}, {6, "agent1", "agent"}, {7, "ops1", "ops"},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO user_account (id, username, role, token) VALUES (?, ?, ?, ?)`,
			r.id, r.name, r.role, r.name+"-token"); err != nil {
			t.Fatalf("seed account: %v", err)
		}
	}
}

func doJSON(r *gin.Engine, method, path, token string, header string, body any) (*httptest.ResponseRecorder, map[string]any) {
	var req *http.Request
	switch b := body.(type) {
	case nil:
		req, _ = http.NewRequest(method, path, nil)
	case string:
		req, _ = http.NewRequest(method, path, bytes.NewBufferString(b))
	default:
		data, _ := json.Marshal(body)
		req, _ = http.NewRequest(method, path, bytes.NewReader(data))
	}
	switch header {
	case "source":
		req.Header.Set("X-Source-Token", token)
	case "auth":
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestHealthz(t *testing.T) {
	r, _ := newRouter(t)
	w, _ := doJSON(r, "GET", "/healthz", "", "", nil)
	if w.Code != 200 {
		t.Fatalf("healthz = %d", w.Code)
	}
}

func TestPagesRendered(t *testing.T) {
	r, _ := newRouter(t)
	for _, p := range []string{"/", "/user", "/agent", "/ops"} {
		w, _ := doJSON(r, "GET", p, "", "", nil)
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("page %s => %d", p, w.Code)
		}
	}
}

func TestEventsAuth(t *testing.T) {
	r, _ := newRouter(t)
	body := fmt.Sprintf(`{"event_id":"e1","user_id":1,"article_id":11,"occur_at":"%s"}`,
		time.Date(2026, 11, 11, 10, 0, 0, 0, model.Beijing).Format(time.RFC3339))

	w, _ := doJSON(r, "POST", "/api/v1/events", "", "", body)
	if w.Code != 401 {
		t.Fatalf("无令牌 => %d", w.Code)
	}
	w, _ = doJSON(r, "POST", "/api/v1/events", "wrong-token", "source", body)
	if w.Code != 401 {
		t.Fatalf("错令牌 => %d", w.Code)
	}
	w, out := doJSON(r, "POST", "/api/v1/events", eventsToken, "source", body)
	if w.Code != 200 || out["status"] != "accepted" {
		t.Fatalf("受理应 200: %d %v", w.Code, out)
	}
	w, _ = doJSON(r, "POST", "/api/v1/events", eventsToken, "source", "{bad json")
	if w.Code != 400 {
		t.Fatalf("坏 JSON => %d", w.Code)
	}
}

func TestUserEndpointsEndToEnd(t *testing.T) {
	r, _ := newRouter(t)
	// 3 篇文章触发发奖
	for i, aid := range []int{11, 12, 13} {
		body := fmt.Sprintf(`{"event_id":"u1-%d","user_id":1,"article_id":%d,"occur_at":"%s"}`,
			i+1, aid, time.Date(2026, 11, 11, 9, i, 0, 0, model.Beijing).Format(time.RFC3339))
		w, out := doJSON(r, "POST", "/api/v1/events", eventsToken, "source", body)
		if w.Code != 200 {
			t.Fatalf("事件 %d => %d", aid, w.Code)
		}
		if i == 2 && out["status"] != "rewarded" {
			t.Fatalf("第 3 篇应发奖: %v", out["status"])
		}
	}

	// 鉴权：无令牌 401 / 客服令牌访问用户端 403
	w, _ := doJSON(r, "GET", "/api/v1/users/points", "", "", nil)
	if w.Code != 401 {
		t.Fatalf("无令牌 => %d", w.Code)
	}
	w, _ = doJSON(r, "GET", "/api/v1/users/points", "agent1-token", "auth", nil)
	if w.Code != 403 {
		t.Fatalf("越权 => %d", w.Code)
	}

	// 活动视图（嵌套在 activity 键下）
	w, out := doJSON(r, "GET", "/api/v1/users/activity", "user1-token", "auth", nil)
	if w.Code != 200 {
		t.Fatalf("activity => %d", w.Code)
	}
	act, _ := out["activity"].(map[string]any)
	if act == nil || act["daily_threshold"] != float64(3) || act["end_at"] != "2026-11-13T00:00:00+08:00" {
		t.Fatalf("activity: %v", out)
	}
	// 进度
	w, out = doJSON(r, "GET", "/api/v1/users/progress", "user1-token", "auth", nil)
	if w.Code != 200 || out["distinct_count"] != float64(3) || out["rewarded"] != true {
		t.Fatalf("progress: %d %v", w.Code, out)
	}
	// 积分
	w, out = doJSON(r, "GET", "/api/v1/users/points", "user1-token", "auth", nil)
	if w.Code != 200 || out["balance"] != float64(30) {
		t.Fatalf("points: %d %v", w.Code, out)
	}

	// 申诉：成功 + 重复 409
	w, _ = doJSON(r, "POST", "/api/v1/users/appeals", "user2-token", "auth",
		map[string]string{"date": "2026-11-11", "content": "第三篇奖励未到账"})
	if w.Code != 200 {
		t.Fatalf("申诉提交 => %d", w.Code)
	}
	w, _ = doJSON(r, "POST", "/api/v1/users/appeals", "user2-token", "auth",
		map[string]string{"date": "2026-11-11", "content": "重复申诉"})
	if w.Code != 409 {
		t.Fatalf("重复申诉 => %d", w.Code)
	}
}

func TestAgentEndpointsEndToEnd(t *testing.T) {
	r, db := newRouter(t)
	// user4 送达 2 篇，提交申诉
	for i, aid := range []int{21, 22} {
		body := fmt.Sprintf(`{"event_id":"u4-%d","user_id":4,"article_id":%d,"occur_at":"%s"}`,
			i+1, aid, time.Date(2026, 11, 11, 9, 0, 0, 0, model.Beijing).Format(time.RFC3339))
		if w, _ := doJSON(r, "POST", "/api/v1/events", eventsToken, "source", body); w.Code != 200 {
			t.Fatalf("事件 => %d", w.Code)
		}
	}
	if _, err := db.Exec(`INSERT INTO appeal (user_id, activity_id, biz_date, content) VALUES (4, 1, '2026-11-11', '第三篇未到账')`); err != nil {
		t.Fatal(err)
	}

	// 客服鉴权：用户令牌访问客服端 403
	w, _ := doJSON(r, "GET", "/api/v1/agent/appeals", "user1-token", "auth", nil)
	if w.Code != 403 {
		t.Fatalf("越权 => %d", w.Code)
	}

	// 列表与详情
	w, out := doJSON(r, "GET", "/api/v1/agent/appeals?status=0", "agent1-token", "auth", nil)
	if w.Code != 200 {
		t.Fatalf("列表 => %d", w.Code)
	}
	items, _ := out["appeals"].([]any)
	if len(items) != 1 {
		t.Fatalf("应有 1 条待处理申诉: %v", out)
	}
	id := int64(items[0].(map[string]any)["id"].(float64))

	w, out = doJSON(r, "GET", fmt.Sprintf("/api/v1/agent/appeals/%d", id), "agent1-token", "auth", nil)
	if w.Code != 200 {
		t.Fatalf("详情 => %d", w.Code)
	}
	w, _ = doJSON(r, "GET", "/api/v1/agent/appeals/9999", "agent1-token", "auth", nil)
	if w.Code != 404 {
		t.Fatalf("不存在申诉 => %d", w.Code)
	}

	// 补入第三篇 → 发奖结案
	w, out = doJSON(r, "POST", fmt.Sprintf("/api/v1/agent/appeals/%d/compensate", id), "agent1-token", "auth",
		map[string]any{"event_id": "u4-a3", "article_id": 23,
			"occur_at": time.Date(2026, 11, 11, 9, 15, 0, 0, model.Beijing).Format(time.RFC3339)})
	if w.Code != 200 || out["resolved"] != true {
		t.Fatalf("补入应发奖结案: %d %v", w.Code, out)
	}
	// 已结申诉再操作 → 409
	w, _ = doJSON(r, "POST", fmt.Sprintf("/api/v1/agent/appeals/%d/compensate", id), "agent1-token", "auth",
		map[string]any{"event_id": "u4-x", "article_id": 24,
			"occur_at": time.Date(2026, 11, 11, 9, 20, 0, 0, model.Beijing).Format(time.RFC3339)})
	if w.Code != 409 {
		t.Fatalf("重复补入 => %d", w.Code)
	}
}

func TestAgentRejectFlow(t *testing.T) {
	r, db := newRouter(t)
	if _, err := db.Exec(`INSERT INTO appeal (user_id, activity_id, biz_date, content) VALUES (5, 1, '2026-11-11', '奖励未到账')`); err != nil {
		t.Fatal(err)
	}
	w, out := doJSON(r, "GET", "/api/v1/agent/appeals?status=0", "agent1-token", "auth", nil)
	if w.Code != 200 {
		t.Fatalf("列表 => %d", w.Code)
	}
	items, _ := out["appeals"].([]any)
	id := int64(items[0].(map[string]any)["id"].(float64))

	// 空依据 → 400
	w, _ = doJSON(r, "POST", fmt.Sprintf("/api/v1/agent/appeals/%d/reject", id), "agent1-token", "auth",
		map[string]string{"conclusion": "   "})
	if w.Code != 400 {
		t.Fatalf("空依据 => %d", w.Code)
	}
	// 正常驳回 → 200
	w, _ = doJSON(r, "POST", fmt.Sprintf("/api/v1/agent/appeals/%d/reject", id), "agent1-token", "auth",
		map[string]string{"conclusion": "证据不足，当日仅2篇"})
	if w.Code != 200 {
		t.Fatalf("驳回 => %d %s", w.Code, w.Body.String())
	}
	// 重复驳回 → 409
	w, _ = doJSON(r, "POST", fmt.Sprintf("/api/v1/agent/appeals/%d/reject", id), "agent1-token", "auth",
		map[string]string{"conclusion": "再来一次"})
	if w.Code != 409 {
		t.Fatalf("重复驳回 => %d", w.Code)
	}
	// 非法 ID → 404
	w, _ = doJSON(r, "POST", "/api/v1/agent/appeals/9999/reject", "agent1-token", "auth",
		map[string]string{"conclusion": "x"})
	if w.Code != 404 {
		t.Fatalf("不存在申诉 => %d", w.Code)
	}
}

func TestEmptyDBErrorBranches(t *testing.T) {
	// 空库（无已发布活动）：验证接口错误分支
	gin.SetMode(gin.TestMode)
	db := testutil.Setup(t)
	seedAccounts(t, db)
	r := server.New(server.Deps{DB: db, Clk: clock.Fixed{T: baseNow}, EventsToken: eventsToken})

	// 无活动：活动视图为 null（200），汇总返回错误（400）
	w, out := doJSON(r, "GET", "/api/v1/users/activity", "user1-token", "auth", nil)
	if w.Code != 200 || out["activity"] != nil {
		t.Fatalf("无活动 activity => %d %v", w.Code, out)
	}
	w, _ = doJSON(r, "GET", "/api/v1/ops/summary", "ops1-token", "auth", nil)
	if w.Code == 200 {
		t.Fatalf("无活动汇总不应成功: %d", w.Code)
	}
}

func TestOpsEndpoints(t *testing.T) {
	r, _ := newRouter(t)

	w, _ := doJSON(r, "GET", "/api/v1/ops/summary", "user1-token", "auth", nil)
	if w.Code != 403 {
		t.Fatalf("越权 => %d", w.Code)
	}
	w, _ = doJSON(r, "GET", "/api/v1/ops/summary", "ops1-token", "auth", nil)
	if w.Code != 200 || w.Body.Len() == 0 {
		t.Fatalf("汇总 => %d", w.Code)
	}

	// 重复发布（种子活动已存在）→ 409
	w, _ = doJSON(r, "POST", "/api/v1/ops/activity", "ops1-token", "auth", map[string]any{
		"name": "x", "start_at": "2026-12-01T00:00:00+08:00", "end_at": "2026-12-02T00:00:00+08:00",
		"daily_threshold": 3, "reward_points": 30})
	if w.Code != 409 {
		t.Fatalf("重复发布 => %d", w.Code)
	}
}
