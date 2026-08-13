// Package assets 用 //go:embed 把前端模板与静态资源编译进二进制，
// 使最终产物为真正的单文件，分发时无需携带 templates/ static/ 目录。
package assets

import "embed"

// TemplatesFS 嵌入 templates/ 下所有 HTML 模板。
//
//go:embed templates/*
var TemplatesFS embed.FS

// StaticFS 嵌入 static/ 下所有 CSS / JS / 图标。
//
//go:embed static/*
var StaticFS embed.FS
