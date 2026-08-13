// 路径安全：设备名过滤 + 子路径穿越校验。
package app

import (
	"path/filepath"
	"strings"
	"unicode"
)

// safeDevice 过滤设备名：只保留字母、数字、中文及 - _，其余剔除；结果为空则回退 "default"。
//
// 用 unicode.IsLetter || unicode.IsDigit 匹配字母数字语义（含中文）。
func safeDevice(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

// safeSubpath 校验 name 合法并返回其在 base 下的绝对路径，非法返回 ("", false)。
//
// 校验规则：
//  1. name 非空、不以 '.' 开头（隐藏文件）、不含 '/' 或 '\'（禁止路径分隔注入）
//  2. resolve 后必须仍在 SaveDir 全局根之内（防止 base 本身被构造恶意路径后越界）
//
// 注意：第 2 步判断的是 SaveDir 而非 base——
// 即使 base 是子目录也校验全局根，防止越界。
func safeSubpath(base, name string) (string, bool) {
	if name == "" ||
		strings.HasPrefix(name, ".") ||
		strings.Contains(name, "/") ||
		strings.Contains(name, "\\") {
		return "", false
	}
	target, err := filepath.Abs(filepath.Join(base, name))
	if err != nil {
		return "", false
	}
	absRoot, err := filepath.Abs(SaveDir)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return target, true
}

// isHidden 判断文件名是否以 '.' 开头（隐藏文件，不展示不可访问）。
func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

// basename 取文件名的最后一段，同时识别 '/' 与 '\'，跨平台一致。
//
// 浏览器上传的 filename 一般已是纯名，但此处做防御性处理。
func basename(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}
