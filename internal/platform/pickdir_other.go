//go:build !windows && !fpk

package platform

// 非 Windows 平台没有 SHBrowseForFolderW；空实现仅用于占位——
// 实际调用点在 platform_impl.go 的 PickFolderDialog，仅 Windows 分支会走到它。
func pickFolderDialogWindows() string { return "" }
