package app

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// === 常量 ===
const (
	saveDirName   = "file-drops"                              // 文件保存目录名（实际路径在桌面上：~/Desktop/file-drops）
	DefaultPort   = 8000                                      // 默认端口；被占用时自动尝试 8001、8002……
	SettingsFile  = "aellus-settings.json"                    // 保存目录持久化配置文件名
	SaveDirName   = saveDirName                               // 导出别名（config.go 等同包文件直接用小写 saveDirName 即可）
	trimAPISocket = "/var/run/trim_open_gateway_apiscope.socket" // 飞牛开放 API 后端网关 Unix Socket
)

// Options 是构造 App 的参数（embed 资源由 main 包持有并传入，因 //go:embed 不支持 ../ 跨目录）。
type Options struct {
	TemplatesFS embed.FS // templates/* 嵌入
	StaticFS    embed.FS // static/* 嵌入
	BaseDir     string   // 日志根目录（与可执行文件/ .app 同级）
	SaveDir     string   // 初始保存目录（main 已完成解析、mkdir 与回退）
}

// App 持有 HTTP 服务的全部运行时状态。
//
// 所有原本散落在 package main 的全局变量（absSaveDir / tmpl / logPath / 各类 mutex）
// 都收敛到这里，由 App 方法并发安全地访问。平台差异通过 Platform 接口注入。
type App struct {
	platform Platform

	// mountPrefix 是飞牛门户对外路径前缀（如 /app/aellus），由启动脚本经
	// FNNAS_GATEWAY_PREFIX 注入；空串表示无前缀（桌面端）。
	// 用于把 /app/<appname>/... 归一化成内部路径，并据此决定页面 <base>。
	mountPrefix string

	absSaveDir string        // 保存目录的绝对路径，所有路径校验都以它为准
	saveDirMu  sync.RWMutex  // 保护 absSaveDir（HTTP 各请求在独立 goroutine 中读取）

	tmpl     *template.Template // 已解析的 HTML 模板（home/upload/browse/callback）
	staticFS embed.FS            // 静态资源（CSS/JS/图标），挂到 /static/

	accessLogPath    string     // 访问日志路径
	operationLogPath string     // 操作日志路径
	logMu            sync.Mutex // 日志并发追加写锁
	ownerMu          sync.Mutex // 设备名映射（devices.json）读改写锁
}

// New 构造 App：注入平台实现与 embed 资源，解析模板，记录日志路径。
// 调用方（main）需在此之前完成保存目录的解析、创建与回退。
func New(p Platform, opts Options) *App {
	a := &App{
		platform:         p,
		staticFS:         opts.StaticFS,
		accessLogPath:    filepath.Join(opts.BaseDir, "access.log"),
		operationLogPath: filepath.Join(opts.BaseDir, "operation.log"),
	}
	a.setSaveDir(opts.SaveDir)
	a.tmpl = template.Must(template.ParseFS(opts.TemplatesFS, "templates/*.html"))
	return a
}

// getSaveDir / setSaveDir：并发安全地读写 absSaveDir。
func (a *App) getSaveDir() string {
	a.saveDirMu.RLock()
	defer a.saveDirMu.RUnlock()
	return a.absSaveDir
}
func (a *App) setSaveDir(d string) {
	a.saveDirMu.Lock()
	defer a.saveDirMu.Unlock()
	a.absSaveDir = d
}

// Serve 启动 HTTP 服务（在独立 goroutine 中运行，不阻塞调用方）。
//
// 飞牛 fnOS 下同时开启两个入口，共用同一套路由：
//  1. 裸 TCP 端口（局域网设备直连 IP:端口）：剥离客户端伪造的 X-Trim-* 头，
//     只保留浏览、上传、下载（免登录直传的设计目的）；
//  2. 飞牛统一网关 Unix Socket（应用中心 / 桌面门户内打开）：网关先校验飞牛登录态，
//     再转发并注入可信头 X-Trim-Userid，删除 / 修改保存目录据此放行。
//
// 两个入口都会归一化飞牛门户的 /app/<appname> 路径前缀（见 withPrefix）：
// 门户既可能经统一网关 Socket 访问，也可能以「端口 + 路径」方式访问，
// 归一化后页面 <base> 与内部路由在两种方式下都正确。
//
// 非飞牛平台（桌面端）不注入 FNNAS_GATEWAY_SOCKET / PREFIX，仅开启裸端口。
// ln/port 由 main 通过 ListenWithFallback / ListenStrict 取得。
func (a *App) Serve(ln net.Listener, port int) {
	// 门户路径前缀由启动脚本注入（如 /app/aellus）；未注入时为无前缀模式。
	a.mountPrefix = strings.TrimSuffix(strings.TrimSpace(os.Getenv("FNNAS_GATEWAY_PREFIX")), "/")

	// 裸端口：先剥离伪造的 X-Trim-*，再归一化前缀，最后进入路由。
	raw := a.withLog(withSecurityHeaders(a.stripTrimHeaders(a.withPrefix(a.buildMux(port)))))
	go func() {
		log.Fatal(http.Serve(ln, raw))
	}()

	// 飞牛统一网关：仅当启动脚本注入 Socket 路径时启用（见 fnos/cmd/main）。
	if sock := strings.TrimSpace(os.Getenv("FNNAS_GATEWAY_SOCKET")); sock != "" {
		go a.serveGateway(sock, port)
	}
}

// buildMux 构造全部路由（页面 + 静态 + API + /api/addr），裸端口与网关共用。
// 页面 <base> 由 withPrefix 按请求路径写入上下文（见 pageBase / servePage）。
func (a *App) buildMux(port int) *http.ServeMux {
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	// /api/addr 返回局域网访问地址（供首页地址栏 / 二维码展示）。
	// 每次请求实时算 IP：电脑 IP 变了（切网络 / 重连路由）也能拿到最新值，
	// 不依赖启动时 GetLANIP 的快照。
	// 注意：始终返回「裸端口直连」地址——门户内展示的二维码要给局域网设备扫，
	// 必须指向 IP:端口（免登录上传），不能返回网关地址（网关要求飞牛登录态）。
	mux.HandleFunc("/api/addr", func(w http.ResponseWriter, r *http.Request) {
		curIP := GetLANIP()
		clientIP := ""
		if ip := remoteIP(r); ip != nil {
			clientIP = ip.String()
		}
		platform := "桌面端（macOS / Windows / Linux）"
		if a.platform.EnforceAuthBoundary() {
			platform = "飞牛 fnOS"
		}
		a.writeJSON(w, http.StatusOK, map[string]string{
			"ip":       curIP,
			"port":     strconv.Itoa(port),
			"url":      "http://" + curIP + ":" + strconv.Itoa(port),
			"clientIP": clientIP,
			"platform": platform,
		})
	})
	return mux
}

// serveGateway 在飞牛统一网关的 Unix Socket 上提供与裸端口相同的服务。
// socketPath 由启动脚本注入（${TRIM_APPDEST}/aellus.sock，与 app/ui/config 的
// gatewaySocket 对应）。该入口的请求已经过网关登录态校验并带可信头 X-Trim-Userid，
// 故不剥离 X-Trim-*；路径前缀归一化与页面 <base> 由 withPrefix 统一处理。
func (a *App) serveGateway(socketPath string, port int) {
	// 上次异常退出可能残留旧 Socket，先删除再绑定，否则 bind 直接失败。
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Printf("[gateway] Unix Socket 监听失败，门户内删除/设置将不可用: %v", err)
		return
	}
	// Socket 位于应用 target 目录（普通用户无法进入该目录），放开文件权限不影响安全，
	// 同时兼容飞牛网关以其它用户/用户组连接。
	_ = os.Chmod(socketPath, 0666)
	log.Printf("[gateway] 已接入飞牛统一网关（Socket=%s Prefix=%s）", socketPath, a.mountPrefix)
	log.Fatal(http.Serve(ln, a.withLog(withSecurityHeaders(a.withPrefix(a.buildMux(port))))))
}

// ctxKey 请求上下文键类型（避免与其它包/中间件的键冲突）。
type ctxKey int

// pageBaseCtxKey 上下文键：当前请求的页面基准路径（供模板 <base> 注入）。
const pageBaseCtxKey ctxKey = 0

// withPrefix 统一归一化飞牛门户的 /app/<appname> 路径前缀。
// 门户既可能经统一网关 Socket 访问，也可能以「端口 + 路径」方式访问
// （http://NAS-IP:8000/app/aellus），这里对两种情形一致处理：
//   - 路径正好等于前缀（无尾斜杠）：302 补上尾斜杠，否则页面内相对路径
//     （static/... 、api/...）会解析到上一层目录，导致样式/脚本全部 404；
//   - 路径带前缀：剥离前缀，使内部路由与裸端口完全一致；
//   - 同时把前缀记为页面基准路径（<base>），供模板注入。
//
// 局域网直连（/、/browse）不带前缀 → 基准路径为 "/"，行为与改造前完全一致。
func (a *App) withPrefix(next http.Handler) http.Handler {
	prefix := a.mountPrefix
	if prefix == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/"
		p := r.URL.Path
		// 大小写不敏感：飞牛可能按 manifest 的 appname 原样（如 /app/Aellus）路由，
		// 而启动脚本注入的是 /app/aellus。
		if strings.EqualFold(p, prefix) {
			http.Redirect(w, r, p+"/", http.StatusFound)
			return
		}
		if len(p) > len(prefix) && p[len(prefix)] == '/' && strings.EqualFold(p[:len(prefix)], prefix) {
			base = p[:len(prefix)] + "/"
			r.URL.Path = p[len(prefix):]
			if len(r.URL.RawPath) > len(prefix) {
				r.URL.RawPath = r.URL.RawPath[len(prefix):]
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), pageBaseCtxKey, base)))
	})
}

// pageBase 取当前请求的页面基准路径（由 withPrefix 写入），无前缀时为 "/"。
func pageBase(r *http.Request) string {
	if v, ok := r.Context().Value(pageBaseCtxKey).(string); ok && v != "" {
		return v
	}
	return "/"
}

// stripTrimHeaders 剥离客户端伪造的 X-Trim-* 系列头（仅用于裸 TCP 端口）。
// 飞牛网关注入的可信身份头只会出现在经网关 Socket 转发的请求上；裸端口不经过网关，
// 任何 X-Trim-* 都是客户端伪造，必须清除，避免局域网设备冒充「已登录门户」越权删除。
func (a *App) stripTrimHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k := range r.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-trim-") {
				r.Header.Del(k)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON 统一的 JSON 响应 helper。
func (a *App) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// servePage 渲染一个静态 HTML 页面（home/upload/browse/callback）。
// 页面本身没有服务端数据（动态内容全靠前端 JS fetch），唯一模板变量是 Base——
// 当前请求的页面基准路径（局域网直连 "/"，门户内 "/app/<appname>/"），写入 <base>，
// 保证 static/... 、api/... 等相对路径在两种访问方式下都解析正确。
func (a *App) servePage(w http.ResponseWriter, r *http.Request, name string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tmpl.ExecuteTemplate(w, name, map[string]string{"Base": pageBase(r)}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SaveDir 返回当前保存目录的绝对路径（供 main 打印启动信息）。
func (a *App) SaveDir() string { return a.getSaveDir() }

// LogOp 写一条操作日志（供 main 记录启动成功）。
func (a *App) LogOp(msg string) { a.logOp(msg) }
