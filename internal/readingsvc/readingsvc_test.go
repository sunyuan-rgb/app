package readingsvc_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/readingsvc"
	"reading-activity/internal/testutil"
)

// stubMain 模拟主服务事件接口，记录全部投递。
type stubMain struct {
	mu   sync.Mutex
	got  []map[string]any
	resp string
}

func (s *stubMain) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	s.mu.Lock()
	s.got = append(s.got, m)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(s.resp))
}

func (s *stubMain) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.got)
}

func setup(t *testing.T) (*readingsvc.Service, *stubMain, *sql.DB) {
	t.Helper()
	db := testutil.Setup(t)
	stub := &stubMain{resp: `{"status":"accepted"}`}
	srv := httptest.NewServer(http.HandlerFunc(stub.handler))
	t.Cleanup(srv.Close)
	svc := readingsvc.New(db, clock.Fixed{T: baseNow}, srv.URL, "test-token")
	return svc, stub, db
}

var baseNow = time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)

func countRows(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	return n
}

func TestRead_DeliversAndRecordsTrusted(t *testing.T) {
	svc, stub, db := setup(t)
	out, err := svc.Read(context.Background(), 1, 11, false)
	if err != nil {
		t.Fatal(err)
	}
	if out["delivered"] != true || out["biz_date"] != "2026-11-11" {
		t.Fatalf("阅读结果不符: %+v", out)
	}
	if stub.count() != 1 {
		t.Fatalf("应投递 1 次, got %d", stub.count())
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM trusted_record WHERE user_id=1 AND article_id=11"); n != 1 {
		t.Fatal("可信记录应落库")
	}
}

func TestRead_DropSkipsDelivery(t *testing.T) {
	svc, stub, db := setup(t)
	out, err := svc.Read(context.Background(), 1, 12, true)
	if err != nil {
		t.Fatal(err)
	}
	if out["delivered"] != false {
		t.Fatalf("drop 应 delivered=false: %+v", out)
	}
	if stub.count() != 0 {
		t.Fatalf("drop 不应投递, got %d", stub.count())
	}
	// 事实已发生：可信记录仍在（申诉补发的证据来源）
	if n := countRows(t, db, "SELECT COUNT(*) FROM trusted_record"); n != 1 {
		t.Fatal("可信记录应存在")
	}
}

func TestSimulate_Scenarios(t *testing.T) {
	cases := []struct {
		scenario   string
		deliveries int
	}{
		{"duplicate", 2}, // 同编号投递两次（幂等验证）
		{"conflict", 2},  // 同编号 + 换内容
		{"future", 1},
		{"too_late", 1},
		{"late_cross_day", 1},
		{"cross_day_normal", 1},
	}
	for _, c := range cases {
		t.Run(c.scenario, func(t *testing.T) {
			svc, stub, _ := setup(t)
			out, err := svc.Simulate(context.Background(), readingsvc.SimulateInput{
				Scenario: c.scenario, UserID: 3, ArticleID: 11,
			})
			if err != nil {
				t.Fatal(err)
			}
			if stub.count() != c.deliveries {
				t.Fatalf("%s 应投递 %d 次, got %d", c.scenario, c.deliveries, stub.count())
			}
			dels, _ := out["deliveries"].([]json.RawMessage)
			if len(dels) != c.deliveries {
				t.Fatalf("结果应含 %d 个投递响应: %d", c.deliveries, len(dels))
			}
		})
	}
}

func TestRead_DBError(t *testing.T) {
	db := testutil.NewErrDB()
	defer db.Close()
	svc := readingsvc.New(db, clock.Fixed{T: baseNow}, "http://localhost:8080", "tok")
	if _, err := svc.Read(context.Background(), 1, 11, false); err == nil {
		t.Fatal("数据库错误应返回 error")
	}
	if _, err := svc.TrustedRecords(context.Background(), 1, "2026-11-11"); err == nil {
		t.Fatal("数据库错误应返回 error")
	}
}

func TestDeliver_BuildRequestError(t *testing.T) {
	// 非法主服务 URL：构造请求失败，错误以合法 JSON 返回（不 panic、不中断流程）
	db := testutil.Setup(t)
	svc := readingsvc.New(db, clock.Fixed{T: baseNow}, "http://exa mple.com", "tok")
	out, err := svc.Simulate(context.Background(), readingsvc.SimulateInput{
		Scenario: "future", UserID: 3, ArticleID: 11,
	})
	if err != nil {
		t.Fatal(err)
	}
	dels, _ := out["deliveries"].([]json.RawMessage)
	if len(dels) != 1 {
		t.Fatalf("应有 1 个投递结果: %d", len(dels))
	}
	var m map[string]any
	if err := json.Unmarshal(dels[0], &m); err != nil || m["error"] == nil {
		t.Fatalf("构造失败应含 error: %v %v", m, err)
	}
}

func TestSimulate_DeliveryFailure(t *testing.T) {
	// 主服务不可达：deliver 失败不影响 Simulate 返回（错误信息内嵌于投递结果）
	db := testutil.Setup(t)
	svc := readingsvc.New(db, clock.Fixed{T: baseNow}, "http://127.0.0.1:1/unreachable", "tok")
	out, err := svc.Simulate(context.Background(), readingsvc.SimulateInput{
		Scenario: "future", UserID: 3, ArticleID: 11,
	})
	if err != nil {
		t.Fatal(err)
	}
	dels, _ := out["deliveries"].([]json.RawMessage)
	if len(dels) != 1 {
		t.Fatalf("应有 1 个投递结果: %d", len(dels))
	}
	var m map[string]any
	if err := json.Unmarshal(dels[0], &m); err != nil || m["error"] == nil {
		t.Fatalf("投递失败应含 error: %v %v", m, err)
	}
}

func TestSimulate_InvalidInput(t *testing.T) {
	svc, _, _ := setup(t)
	if _, err := svc.Simulate(context.Background(), readingsvc.SimulateInput{Scenario: "unknown"}); err == nil {
		t.Fatal("未知场景应报错")
	}
	if _, err := svc.Simulate(context.Background(), readingsvc.SimulateInput{Scenario: "custom"}); err == nil {
		t.Fatal("custom 缺 occur_at 应报错")
	}
	if _, err := svc.Simulate(context.Background(), readingsvc.SimulateInput{
		Scenario: "custom", OccurAt: "not-a-time"}); err == nil {
		t.Fatal("非法 occur_at 应报错")
	}
}

func TestTrustedRecords_Query(t *testing.T) {
	svc, _, db := setup(t)
	if _, err := svc.Read(context.Background(), 1, 11, true); err != nil {
		t.Fatal(err)
	}
	records, err := svc.TrustedRecords(context.Background(), 1, "2026-11-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ArticleID != 11 || records[0].BizDate != "2026-11-11" {
		t.Fatalf("可信查询不符: %+v", records)
	}
	// 空结果为空数组
	empty, err := svc.TrustedRecords(context.Background(), 2, "2026-11-11")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("空结果应为空数组: %v %v", empty, err)
	}
	_ = db
}
