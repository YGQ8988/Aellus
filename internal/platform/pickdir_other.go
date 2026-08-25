//go:build !windows && !fpk

package platform

// 非 Windows 平台没有 SHBrowseForFolderW；空实现仅用于占位，
// main.go 的 pickFolderDialog 在非 Windows 分支不会调用它。
func pickFolderDialogWindows() string { return "" }
