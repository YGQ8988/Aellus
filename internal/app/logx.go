package app

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// === 日志（App 方法，使用 App 持有的日志路径与锁） ===

// maxLogBytes 单个日志文件的大小上限（超过即轮转）。
// 访问日志会记录每次非静态请求，长期运行必须限制体积，否则会把磁盘写满
// （飞牛端日志落在持久卷 TRIM_PKGVAR 下）。
const maxLogBytes = 5 << 20 // 5MB

// maxLogValueRunes 单条日志内容的字符数上限（防超长输入灌满日志文件）。
const maxLogValueRunes = 512

// sanitizeLogValue 清洗写入日志的内容：控制字符（换行 / 制表等）替换为空格并限长。
//
// 请求路径、文件名、目录名、Deviceid 都来自客户端，若原样写进日志，攻击者可以用
// 换行伪造出整条假的审计记录（日志注入），也可以用超长值把日志刷满。
// 这里是所有日志的唯一出口（writeLog），因此在出口统一清洗。
func sanitizeLogValue(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if rs := []rune(cleaned); len(rs) > maxLogValueRunes {
		return string(rs[:maxLogValueRunes]) + "…(截断)"
	}
	return cleaned
}

// writeLog 日志文件是并发追加写入的，用 App.logMu 保证不互相穿插。
func (a *App) writeLog(path, msg string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	// 简单轮转：超过上限就把当前文件改名为 <name>.old（只保留一份历史）。
	// 并发安全由 logMu 保证，不会出现两个请求同时轮转。
	if fi, err := os.Stat(path); err == nil && fi.Size() >= maxLogBytes {
		old := path + ".old"
		_ = os.Remove(old) // Windows 下 Rename 不能覆盖已存在的目标文件
		_ = os.Rename(path, old)
	}
	// 0600：日志里有完整的文件名清单与访问者 IP，不应被本机其它用户/应用读取。
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(ts + "  " + sanitizeLogValue(msg) + "\n")
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
