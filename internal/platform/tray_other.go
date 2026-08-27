//go:build !windows && !darwin && !fpk

package platform

// Linux 等非 Windows/非 macOS 平台没有系统托盘概念，这里提供一个等价实现：
// 直接阻塞等待 Ctrl+C / 终止信号。自动打开浏览器、HTTP 服务逻辑与 Windows 完全一致。

import (
	"os"
	"os/signal"
	"syscall"
)

func runTray(url string) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}
