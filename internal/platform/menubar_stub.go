//go:build !darwin || !cgo

// 状态栏常驻的占位实现：非 macOS 或非 cgo 编译时禁用。
// 命令行版本（交叉编译 CGO_ENABLED=0）保持原行为，不启用状态栏。
package platform

func MenuBarEnabled() bool { return false }

func RunMenuBar(saveDir, accessURL string) {}
