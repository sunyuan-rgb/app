// Package web 内嵌模拟阅读服务页面。
package web

import "embed"

//go:embed *.html
var FS embed.FS
