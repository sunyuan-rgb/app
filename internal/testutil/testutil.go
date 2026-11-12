// Package testutil 集成测试基建：为每个测试二进制提供全新结构的独立测试库
// （库名按测试包区分，避免 go test ./... 并行时互相 DROP/TRUNCATE），
// 表结构与生产迁移完全一致。连接串由环境变量 TEST_DSN 提供，不含任何口令。
package testutil

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"reading-activity/migrations"
)

// dsnParams 迁移脚本含多条语句，须开启 multiStatements。
const dsnParams = "multiStatements=true&parseTime=true&loc=UTC"

// rootDSN 不带库名的基础 DSN（以 / 结尾）。口令一律通过 TEST_DSN 注入，不落仓库。
func rootDSN() string {
	if v := os.Getenv("TEST_DSN"); v != "" {
		return v
	}
	// 本地默认：root 无口令（仅适用于未设置口令的开发库）
	return "root:@tcp(127.0.0.1:3306)/"
}

// testDBName 按测试二进制名派生独立库名（如 service.test → reading_activity_test_service），
// 保证不同包的测试可并行执行而不互相干扰。
func testDBName() string {
	base := filepath.Base(os.Args[0])
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".test")
	if base == "" || base == "." {
		base = "default"
	}
	return "reading_activity_test_" + sanitize(base)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func dbDSN() string {
	return rootDSN() + testDBName() + "?" + dsnParams
}

// Setup 创建全新测试库并执行全部迁移，返回连接。
func Setup(t *testing.T) *sql.DB {
	t.Helper()
	root, err := sql.Open("mysql", rootDSN())
	if err != nil {
		t.Fatalf("open root dsn: %v", err)
	}
	defer root.Close()
	ctx := context.Background()
	dbName := testDBName()
	if _, err := root.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+dbName+" DEFAULT CHARSET utf8mb4"); err != nil {
		// 账号可能仅有该库的权限而无 CREATE DATABASE 权限：库已存在则继续
		if err := root.PingContext(ctx); err != nil {
			t.Fatalf("create test database: %v", err)
		}
		var n int
		if err := root.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", dbName).Scan(&n); err != nil || n == 0 {
			t.Fatalf("create test database: %v（且库不存在）", err)
		}
	}
	db, err := sql.Open("mysql", dbDSN())
	if err != nil {
		t.Fatalf("open test dsn: %v", err)
	}
	// 重建全部表，保证用例隔离
	if err := dropAllTables(ctx, db); err != nil {
		t.Fatalf("drop tables: %v", err)
	}
	for _, name := range migrationNames() {
		content, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	// 迁移含种子数据（002）：清空全部业务数据，只保留表结构，测试自行播种
	if err := truncateAllTables(ctx, db); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func truncateAllTables(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return err
	}
	defer db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=1")
	for _, name := range tables {
		if _, err := db.ExecContext(ctx, "TRUNCATE TABLE `"+name+"`"); err != nil {
			return err
		}
	}
	return nil
}

func migrationNames() []string {
	entries, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		panic(err)
	}
	sort.Strings(entries)
	return entries
}

func dropAllTables(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return err
	}
	defer db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=1")
	for _, name := range tables {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS `"+name+"`"); err != nil {
			return err
		}
	}
	return nil
}
