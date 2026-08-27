package app

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
)

// === 常量 ===
const (
	saveDirName   = "file-drops"                              // 文件保存目录名（实际路径在桌面上：~/Desktop/file-drops）
	DefaultPort   = 8000                                      // 默认端口；被占用时自动尝试 8001、8002……
	SettingsFile  = "aellus-settings.json"                    // 保存目录持久化配置文件名
	SaveDirName   = saveDirName                               // 导出别名（config.go 等同包文件直接用小写 saveDirName 即可）
	ownersFile    = ".aellus-owners"                          // 归属 manifest 文件名（旧版散落模式）
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

	absSaveDir string        // 保存目录的绝对路径，所有路径校验都以它为准
	saveDirMu  sync.RWMutex  // 保护 absSaveDir（HTTP 各请求在独立 goroutine 中读取）

	tmpl     *template.Template // 已解析的 HTML 模板（home/upload/browse/callback）
	staticFS embed.FS            // 静态资源（CSS/JS/图标），挂到 /static/

	accessLogPath    string     // 访问日志路径
	operationLogPath string     // 操作日志路径
	logMu            sync.Mutex // 日志并发追加写锁
	ownerMu          sync.Mutex // 归属 manifest 读改写锁
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

// Serve 注册路由并启动 HTTP 服务（在独立 goroutine 中运行，不阻塞调用方）。
// ln/port/ip 由 main 通过 listenWithFallback/getLANIP 取得。
func (a *App) Serve(ln net.Listener, port int) {
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	// /api/addr 返回局域网访问地址（供首页地址栏展示）。
	// 每次请求实时算 IP：电脑 IP 变了（切网络 / 重连路由）也能拿到最新值，
	// 不依赖启动时 GetLANIP 的快照。
	mux.HandleFunc("/api/addr", func(w http.ResponseWriter, r *http.Request) {
		curIP := GetLANIP()
		a.writeJSON(w, http.StatusOK, map[string]string{
			"ip":   curIP,
			"port": strconv.Itoa(port),
			"url":  "http://" + curIP + ":" + strconv.Itoa(port),
		})
	})
	go func() {
		log.Fatal(http.Serve(ln, a.withLog(withSecurityHeaders(mux))))
	}()
}

// writeJSON 统一的 JSON 响应 helper。
func (a *App) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// servePage 渲染一个静态 HTML 页面（home/upload/browse/callback）。
// 这些 HTML 里没有任何模板变量（动态数据全靠前端 JS fetch），直接原样输出。
func (a *App) servePage(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tmpl.ExecuteTemplate(w, name, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SaveDir 返回当前保存目录的绝对路径（供 main 打印启动信息）。
func (a *App) SaveDir() string { return a.getSaveDir() }

// LogOp 写一条操作日志（供 main 记录启动成功）。
func (a *App) LogOp(msg string) { a.logOp(msg) }
