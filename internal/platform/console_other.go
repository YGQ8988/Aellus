//go:build !windows

// 非 Windows 平台：终端默认 UTF-8，无需任何处理。
package platform

func InitConsoleUTF8() {}
