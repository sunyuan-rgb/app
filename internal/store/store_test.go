package store_test

import (
	"context"
	"testing"
	"time"

	"reading-activity/internal/model"
	"reading-activity/internal/store"
	"reading-activity/internal/testutil"
)

func TestOpen_InvalidDSN(t *testing.T) {
	if _, err := store.Open("this-is-not-a-dsn\x00"); err == nil {
		t.Fatal("非法 DSN 应报错")
	}
}

func TestOpen_ValidDSN(t *testing.T) {
	db := testutil.Setup(t) // 确保测试库存在，并取得库名
	_ = db
	dsn := testutil.RootDSN()
	opened, err := store.Open(dsn + testutil.DBName() + "?parseTime=true&loc=UTC")
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if err := opened.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestQueryer_CompatWithTx(t *testing.T) {
	// Queryer 接口同时兼容 *sql.DB 与 *sql.Tx（事务内外复用）
	db := testutil.Setup(t)
	if _, err := db.ExecContext(context.Background(), "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	var q store.Queryer = db
	if q == nil {
		t.Fatal("Queryer 应兼容 *sql.DB")
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var q2 store.Queryer = tx
	if _, err := q2.ExecContext(context.Background(), "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertEvent_CountedTrue(t *testing.T) {
	// Counted=true 路径（boolToInt(true) 分支）
	db := testutil.Setup(t)
	now := time.Date(2026, 11, 11, 21, 0, 0, 0, model.Beijing)
	ok, err := store.InsertEvent(context.Background(), db, &model.ReadingEvent{
		EventID: "e-counted", UserID: 1, ArticleID: 1,
		OccurAt: now, ReceivedAt: now, BizDate: "2026-11-11", Counted: true,
	})
	if err != nil || !ok {
		t.Fatalf("insert: ok=%v err=%v", ok, err)
	}
	var counted int
	if err := db.QueryRow(`SELECT counted FROM reading_event WHERE event_id = 'e-counted'`).Scan(&counted); err != nil || counted != 1 {
		t.Fatalf("counted 应为 1: %d %v", counted, err)
	}
}
