// Package migrations 内嵌数据库迁移脚本，供服务部署与集成测试共用，
// 保证测试与生产使用完全相同的表结构。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
