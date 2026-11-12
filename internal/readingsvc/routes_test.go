package readingsvc_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/clock"
	"reading-activity/internal/readingsvc"
	"reading-activity/internal/testutil"
)

func newRouteServer(t *testing.T) (*gin.Engine, *stubMain) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.Setup(t)
	stub := &stubMain{resp: `{"status":"accepted"}`}
	srv := httptest.NewServer(http.HandlerFunc(stub.handler))
	t.Cleanup(srv.Close)
	svc := readingsvc.New(db, clock.Fixed{T: baseNow}, srv.URL, "test-token")
	r := gin.New()
	svc.RegisterRoutes(r, clock.Fixed{T: baseNow})
	return r, stub
}

func postJSON(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRoutes_ReadEndpoint(t *testing.T) {
	r, stub := newRouteServer(t)

	w := postJSON(t, r, "/read", map[string]any{"user_id": 1, "article_id": 11, "drop": true})
	if w.Code != 200 {
		t.Fatalf("/read => %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["delivered"] != false {
		t.Fatalf("drop=true 应 delivered=false: %v", out)
	}
	if stub.count() != 0 {
		t.Fatal("drop=true 不应投递")
	}

	// 缺字段 → 400
	w = postJSON(t, r, "/read", map[string]any{"user_id": 1})
	if w.Code != 400 {
		t.Fatalf("缺 article_id => %d", w.Code)
	}
	// 非 JSON → 400
	req := httptest.NewRequest(http.MethodPost, "/read", bytes.NewBufferString("not-json"))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != 400 {
		t.Fatalf("非 JSON => %d", w2.Code)
	}
}

func TestRoutes_SimulateEndpoint(t *testing.T) {
	r, stub := newRouteServer(t)

	w := postJSON(t, r, "/simulate", map[string]any{"scenario": "duplicate", "user_id": 1, "article_id": 11})
	if w.Code != 200 {
		t.Fatalf("/simulate => %d %s", w.Code, w.Body.String())
	}
	if stub.count() != 2 {
		t.Fatalf("duplicate 应投递 2 次, got %d", stub.count())
	}

	// 未知场景 → 400
	w = postJSON(t, r, "/simulate", map[string]any{"scenario": "nope", "user_id": 1, "article_id": 1})
	if w.Code != 400 {
		t.Fatalf("未知场景 => %d", w.Code)
	}
}

func TestRoutes_TrustedRecordsEndpoint(t *testing.T) {
	r, _ := newRouteServer(t)

	// 非法 user_id → 400
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/trusted/records?user_id=abc", nil))
	if w.Code != 400 {
		t.Fatalf("非法 user_id => %d", w.Code)
	}
	// 空记录
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/trusted/records?user_id=1&biz_date=2026-11-11", nil))
	if w.Code != 200 {
		t.Fatalf("查询 => %d", w.Code)
	}
	var out struct {
		Records []map[string]any `json:"records"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Records == nil || len(out.Records) != 0 {
		t.Fatalf("应为空数组: %v", out.Records)
	}
	// 页面与 healthz
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != 200 || w.Body.Len() == 0 {
		t.Fatalf("页面 => %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("healthz => %d", w.Code)
	}
}
