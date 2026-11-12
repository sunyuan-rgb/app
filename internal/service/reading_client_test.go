package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"reading-activity/internal/service"
)

func TestHTTPReadingService_QueryRecords(t *testing.T) {
	// 正常返回
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/trusted/records" {
			t.Errorf("路径不符: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"records":[{"event_id":"e1","user_id":1,"article_id":11,"occur_at":"2026-11-11T09:00:00+08:00"}]}`))
	}))
	defer stub.Close()

	cli := service.NewHTTPReadingService(stub.URL)
	records, err := cli.QueryRecords(context.Background(), 1, "2026-11-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ArticleID != 11 {
		t.Fatalf("解析不符: %+v", records)
	}

	// 非 200 响应
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()
	if _, err := service.NewHTTPReadingService(bad.URL).QueryRecords(context.Background(), 1, "2026-11-11"); err == nil {
		t.Fatal("非 200 应报错")
	}

	// 非法 JSON
	junk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer junk.Close()
	if _, err := service.NewHTTPReadingService(junk.URL).QueryRecords(context.Background(), 1, "2026-11-11"); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
