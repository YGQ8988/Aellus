package app

import "net/http"

// registerRoutes 把所有路由挂到 mux 上。
// 页面 <base> 由 withPrefix 按请求路径写入上下文，servePage 读取（见 pageBase）。
func (a *App) registerRoutes(mux *http.ServeMux) {
	// 页面（HTML 里的 static/xxx 等相对路径由下面的静态处理器负责，无需改动）
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.servePage(w, r, "home.html")
	})
	mux.HandleFunc("/browse", func(w http.ResponseWriter, r *http.Request) {
		a.servePage(w, r, "browse.html")
	})
	mux.HandleFunc("/upload", a.requireTrustedClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			a.handleUpload(w, r)
			return
		}
		a.servePage(w, r, "upload.html")
	}))

	// 静态资源：把 embed 进来的 static/ 目录挂到 /static/ 路由。
	// 因为 embedded 文件名就是 static/css/common.css 这种，URL /static/css/common.css 能直接对应上。
	// 包一层 noCache：开发期前端常改 CSS，禁用缓存保证每次都拿到最新（避免"改了没变化"）。
	mux.Handle("/static/", noCache(http.FileServer(http.FS(a.staticFS))))

	// 浏览器可能额外请求 /favicon.ico，重定向到我们的 svg 图标（不记录日志）。
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		// 用相对路径重定向：经飞牛网关访问时（/app/<appname>/favicon.ico）也能落到
		// 正确的 /app/<appname>/static/... 下，裸端口直连时与原来的绝对路径等价。
		http.Redirect(w, r, "static/img/favicon.svg", http.StatusFound)
	})

	// API
	// 读取类接口一律包 noCache：列表、设置、文件内容会随时变化，被浏览器或中间
	// 代理缓存后会读到过期数据（下载的内容还可能被留在代理/磁盘缓存里）。
	// /api/thumb 例外：它自己用 Last-Modified + must-revalidate 走 304，
	// 命中缓存是期望行为（列表页大量缩略图全量重传代价很高）。
	mux.Handle("/api/dirs", noCache(http.HandlerFunc(a.handleDirs)))
	mux.Handle("/api/files", noCache(http.HandlerFunc(a.handleFiles)))
	mux.HandleFunc("/api/thumb", a.handleThumb)
	mux.Handle("/api/download", noCache(http.HandlerFunc(a.handleDownload)))
	// 会改变服务端状态的接口统一包一层 CSRF 校验（见 middleware.go requireTrustedClient）：
	// 只放行携带前端自定义头 X-Aellus-Client 的请求——浏览器跨站时该头会触发预检，
	// 而本服务不返回任何 CORS 许可，预检必然失败，请求根本发不出去。
	mux.HandleFunc("/api/download-batch", a.requireTrustedClient(a.handleBatchDownload))
	mux.HandleFunc("/api/delete", a.requireTrustedClient(a.handleDelete))
	// 设置：读取无副作用（不校验），修改保存路径要校验
	mux.Handle("/api/settings", noCache(http.HandlerFunc(a.handleSettings)))
	mux.Handle("/api/authpaths", noCache(http.HandlerFunc(a.handleAuthPaths)))
	mux.Handle("/api/listdir", noCache(http.HandlerFunc(a.handleListDir)))
	mux.HandleFunc("/api/set-savedir", a.requireTrustedClient(a.handleSetSaveDir))
	mux.HandleFunc("/api/pick-dir", a.requireTrustedClient(a.handlePickDir))
}
