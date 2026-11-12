// Package web 内嵌页面资源。
package web

import "embed"

//go:embed *.html
var FS embed.FS
