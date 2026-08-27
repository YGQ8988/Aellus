//go:build !darwin && !windows && !fpk

package platform

// 非 macOS / 非 Windows 平台不弹系统通知（保持原有行为：仅菜单栏/托盘点击才打开浏览器）。
func postOpenNotification(title, body, url string) {}
