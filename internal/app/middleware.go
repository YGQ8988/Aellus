package app

import (
	"net/http"
	"strconv"
	"strings"
)

// === CSRF 防护 ===

// clientHeaderName 是前端所有请求都会携带的自定义头（见 static/js/ui.js 与 static/js/upload.js）。
//
// 自定义头属于「非简单请求头」：浏览器对跨站请求会先发 OPTIONS 预检，而本服务不返回任何
// CORS 许可头（预检必然失败），因此任意第三方网页都无法让受害者浏览器发出带该头的请求。
// 这样「局域网免登录上传 / 浏览 / 下载」的能力完全不变，同时把「借用户浏览器删文件、
// 改保存目录、上传文件」这条路径关掉。命令行 / 脚本客户端自行加上该头即可继续调用。
const clientHeaderName = "X-Aellus-Client"

// isTrustedStateChange 判断一个「会改变服务端状态」的请求是否可信（防 CSRF）。
func isTrustedStateChange(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get(clientHeaderName)) == "" {
		return false
	}
	// 纵深防御：浏览器明确声明「跨站」时直接拒绝。万一前置代理（如飞牛网关）放开了
	// CORS 许可，自定义头这一层就不再可靠，这里作为第二道闸。
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
		return false
	}
	return true
}

// requireTrustedClient 包装「会改变状态」的接口：仅在 POST 时校验（GET 是页面与只读接口，
// 不受影响，避免影响首页 / 上传页的正常渲染）。
func (a *App) requireTrustedClient(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && !isTrustedStateChange(r) {
			a.writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "缺少客户端标识头（" + clientHeaderName + "），跨站请求已被拒绝",
			})
			return
		}
		next(w, r)
	}
}

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

// withLog 统一访问日志中间件，包装整个路由（用 App.logAccess 记录）。
func (a *App) withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: 0}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		// 只记 TCP 对端地址：X-Forwarded-For 等请求头可被局域网设备任意伪造，
		// 用它记日志会污染审计记录（能把操作栽赃到别的 IP 上）。
		clientIP := ""
		if ip := remoteIP(r); ip != nil {
			clientIP = ip.String()
		}
		a.logAccess(clientIP, r.Method, r.URL.Path, strconv.Itoa(status), deviceID(r))
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

// frameAncestorsAny 允许任意来源 iframe 嵌入。
// 用于裸端口（局域网直连）入口：该入口没有删除 / 管理权限，且需兼容飞牛门户以
// 「端口 + 路径」方式嵌入应用。
const frameAncestorsAny = "*"

// frameAncestorsSelf 只允许同源 iframe 嵌入。
// 用于飞牛统一网关入口：门户页面与应用的源一致（同一个 host:port），因此门户内嵌不受影响；
// 而第三方站点无法再 iframe 嵌入这个【具备管理权限】的页面 → 防点击劫持。
const frameAncestorsSelf = "'self'"

// withSecurityHeaders 为所有响应添加安全响应头，防止 MIME 嗅探、点击劫持等。
// frameAncestors 由调用方按监听入口决定（见上面两个常量）。
// 不依赖 App 状态，保持为普通函数。
func withSecurityHeaders(next http.Handler, frameAncestors string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		// CSP 做纵深防御：默认全禁，仅放开同源资源与必需的内联脚本/样式，
		// 阻断外域脚本 / 图片 / 字体加载（缓解 XSS 影响面），同时保留门户 iframe 嵌入能力。
		h.Set("Content-Security-Policy",
			"default-src 'none'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"media-src 'self' blob:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"frame-ancestors "+frameAncestors+"; "+
				"base-uri 'self'; "+
				"form-action 'self'")
		// 老浏览器不识别 CSP 的 frame-ancestors，补 X-Frame-Options 兜底。
		// 只在收紧为 'self' 时下发：飞牛门户需要跨域 iframe 嵌入，
		// 那里一旦带上这个头，老浏览器里的门户页面就打不开了。
		if frameAncestors == frameAncestorsSelf {
			h.Set("X-Frame-Options", "SAMEORIGIN")
		}
		next.ServeHTTP(w, r)
	})
}
