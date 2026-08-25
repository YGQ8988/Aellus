package app

import "net/http"

// registerRoutes 把所有路由挂到 mux 上。
func (a *App) registerRoutes(mux *http.ServeMux) {
	// 页面（HTML 里写死的 /static/xxx 路径由下面的静态处理器负责，无需改动）
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.servePage(w, "home.html")
	})
	mux.HandleFunc("/browse", func(w http.ResponseWriter, r *http.Request) {
		a.servePage(w, "browse.html")
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			a.handleUpload(w, r)
			return
		}
		a.servePage(w, "upload.html")
	})

	// 静态资源：把 embed 进来的 static/ 目录挂到 /static/ 路由。
	// 因为 embedded 文件名就是 static/css/common.css 这种，URL /static/css/common.css 能直接对应上。
	// 包一层 noCache：开发期前端常改 CSS，禁用缓存保证每次都拿到最新（避免"改了没变化"）。
	mux.Handle("/static/", noCache(http.FileServer(http.FS(a.staticFS))))

	// 浏览器可能额外请求 /favicon.ico，重定向到我们的 svg 图标（不记录日志）。
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/img/favicon.svg", http.StatusFound)
	})

	// API
	mux.HandleFunc("/api/dirs", a.handleDirs)
	mux.HandleFunc("/api/files", a.handleFiles)
	mux.HandleFunc("/api/thumb", a.handleThumb)
	mux.HandleFunc("/api/download", a.handleDownload)
	mux.HandleFunc("/api/download-batch", a.handleBatchDownload)
	mux.HandleFunc("/api/delete", a.handleDelete)
	// 设置：读取/修改文件保存路径
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/authpaths", a.handleAuthPaths)
	mux.HandleFunc("/api/listdir", a.handleListDir)
	mux.HandleFunc("/api/set-savedir", a.handleSetSaveDir)
	mux.HandleFunc("/api/pick-dir", a.handlePickDir)
	// 飞牛开放 API 授权路由回调页（openAppAuth 的 redirectUri 指向本页）
	mux.HandleFunc("/callback.html", func(w http.ResponseWriter, r *http.Request) {
		a.servePage(w, "callback.html")
	})
}
