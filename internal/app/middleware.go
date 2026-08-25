package app

import (
	"net/http"
	"strconv"
)

// === 中间件 ===

// statusRecorder 包装 ResponseWriter，记录实际写出的 HTTP 状态码。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Flush 透传到底层 ResponseWriter，使 SSE（http.Flusher）在 withLog 包装下仍可用。
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// withLog 统一访问日志中间件，包装整个路由（用 App.logAccess 记录）。
func (a *App) withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: 0}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		a.logAccess(realIP(r), r.Method, r.URL.Path, strconv.Itoa(status), r.UserAgent())
	})
}

// noCache 包装一个 http.Handler，强制浏览器不缓存响应（开发期前端常改 CSS/JS）。
// 不依赖 App 状态，保持为普通函数。
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

// withSecurityHeaders 为所有响应添加安全响应头，防止 MIME 嗅探、点击劫持等。
// 不依赖 App 状态，保持为普通函数。
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// 注意：不设置 X-Frame-Options / CSP frame-ancestors。
		// 飞牛 fnOS 门户以 iframe 方式嵌入应用（iframe 入口 + micro_app），
		// 设置 DENY 会导致门户内"拒绝访问"（此前踩坑，已移除）。
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
