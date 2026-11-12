package testutil

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
)

// ErrInjected 错误分支测试用的注入错误。
var ErrInjected = errors.New("注入的数据库错误")

// NewErrDB 返回一个所有操作都返回 ErrInjected 的 *sql.DB，
// 用于以确定性的方式覆盖 store/service/handler 的数据库错误分支（无需 mock 三方依赖）。
func NewErrDB() *sql.DB {
	return sql.OpenDB(errConnector{})
}

type errConnector struct{}

func (errConnector) Connect(context.Context) (driver.Conn, error) { return errConn{}, nil }
func (errConnector) Driver() driver.Driver                        { return errDriver{} }

type errDriver struct{}

func (errDriver) Open(string) (driver.Conn, error) { return errConn{}, nil }

type errConn struct{}

func (errConn) Prepare(string) (driver.Stmt, error) { return nil, ErrInjected }
func (errConn) Close() error                        { return nil }
func (errConn) Begin() (driver.Tx, error)           { return nil, ErrInjected }

// QueryContext / ExecContext：database/sql 优先走快速路径，直接注入错误。
func (errConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, ErrInjected
}

func (errConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, ErrInjected
}
