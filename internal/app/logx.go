package app

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// === 日志（App 方法，使用 App 持有的日志路径与锁） ===

// writeLog 日志文件是并发追加写入的，用 App.logMu 保证不互相穿插。
func (a *App) writeLog(path, msg string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(ts + "  " + msg + "\n")
}

// logAccess 访问日志：记录 IP、Method、Path、Status、设备 ID。
// /static/* 和 /favicon.ico 这种刷屏请求不记录。
func (a *App) logAccess(ip, method, path, status, devID string) {
	if strings.HasPrefix(path, "/static/") || path == "/favicon.ico" {
		return
	}
	a.writeLog(a.accessLogPath, fmt.Sprintf("%s %s %s %s %s", ip, method, path, status, devID))
}

// logOp 操作日志：记录上传成功、批量下载等。
func (a *App) logOp(msg string) {
	a.writeLog(a.operationLogPath, msg)
}
