package testutil

import (
	"context"
	"testing"
)

// TestErrDBInternals 覆盖注入错误驱动的各接口实现（错误分支测试基建自身）。
func TestErrDBInternals(t *testing.T) {
	conn, err := (errDriver{}).Open("dsn")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Prepare("SELECT 1"); err == nil {
		t.Fatal("Prepare 应注入错误")
	}
	if _, err := conn.Begin(); err == nil {
		t.Fatal("Begin 应注入错误")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := (errConn{}).QueryContext(context.Background(), "q", nil); err == nil {
		t.Fatal("QueryContext 应注入错误")
	}
	if _, err := (errConn{}).ExecContext(context.Background(), "q", nil); err == nil {
		t.Fatal("ExecContext 应注入错误")
	}
	if (errConnector{}).Driver() == nil {
		t.Fatal("Driver 不应为 nil")
	}
	db := NewErrDB()
	defer db.Close()
	if db == nil {
		t.Fatal("NewErrDB 不应返回 nil")
	}
}
