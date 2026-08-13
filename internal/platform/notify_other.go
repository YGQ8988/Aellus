//go:build !darwin

// 非 macOS 平台：无需通知。
package platform

func NotifyStartup(saveDir, accessURL string) {}
