// 日志：访问日志中间件 + 操作日志。
// 对应原 Python 版 server.py 中的 access_log_middleware 与 _op_logger。
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	accessLogger *log.Logger // 访问日志（access.log）
	opLogger     *log.Logger // 操作日志（operation.log）
)

// 日期格式与原 Python 版 ACCESS_LOG_DATEFMT 一致。
const logDateFmt = "2006-01-02 15:04:05"

// initLoggers 打开日志文件（追加写），初始化两个 logger。
//
// logger 的 flags=0 表示不自动加时间前缀，时间由我们在消息里手动拼，
// 以精确复刻原版 "%(asctime)s  %(message)s" 的格式（时间 + 两个空格 + 消息）。
func initLoggers() {
	accessLogger = newFileLogger(AccessLog)
	opLogger = newFileLogger(OpLog)
}

func newFileLogger(path string) *log.Logger {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// 日志文件打不开时回退到标准错误，绝不让服务因日志初始化失败而退出
		return log.New(os.Stderr, "", 0)
	}
	return log.New(f, "", 0)
}

// logAccess 写一条访问日志，格式与原版 ACCESS_LOG_TEMPLATE 完全一致。
func logAccess(ip, method, path, ua string, status int) {
	if accessLogger == nil {
		return
	}
	if ua == "" {
		ua = "-"
	}
	ts := time.Now().Format(logDateFmt)
	accessLogger.Printf("%s  访问来源IP: %s  请求方式: %s  请求URL路径: %s  响应状态: %d  浏览器UA: %q",
		ts, ip, method, path, status, ua)
}

// logOp 写一条操作日志（上传成功 / 批量打包下载）。
func logOp(format string, args ...any) {
	if opLogger == nil {
		return
	}
	ts := time.Now().Format(logDateFmt)
	opLogger.Printf("%s  %s", ts, fmt.Sprintf(format, args...))
}

// clientIP 取真实来源 IP：优先 X-Forwarded-For 首段（反代场景），否则直连 IP。
// 对应原 Python 版 server._client_ip。
func clientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// statusRecorder 包装 ResponseWriter 以捕获响应状态码，供访问日志记录。
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

// accessLogMiddleware 记录每个非静态资源请求的访问日志。
// 静态资源（/static/）不记录，避免每次页面加载刷屏——与原版一致。
func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			logAccess(clientIP(r), r.Method, r.URL.Path, r.UserAgent(), rec.status)
		}
	})
}
