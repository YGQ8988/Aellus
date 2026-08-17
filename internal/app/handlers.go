// HTTP 路由与处理函数：页面、上传、列表、下载、批量打包、删除。
package app

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aellus/assets"
)

// tmpl 解析后的 HTML 模板集合（从 embed FS 加载）。
var tmpl *template.Template

// InitTemplates 从 embed FS 解析 templates/*.html。
// 模板内无变量，仅静态 HTML，ExecuteTemplate(nil) 即可。
func InitTemplates() error {
	sub, err := fs.Sub(assets.TemplatesFS, "templates")
	if err != nil {
		return err
	}
	t, err := template.ParseFS(sub, "*.html")
	if err != nil {
		return err
	}
	tmpl = t
	return nil
}

// mimeOverrides 覆盖前端关心的常见类型，避免依赖系统 mime.types（Windows 上可能不全）。
var mimeOverrides = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".heic": "image/heic",
	".svg":  "image/svg+xml",
	".mp4":  "video/mp4",
	".mov":  "video/quicktime",
	".m4v":  "video/x-m4v",
	".webm": "video/webm",
	".pdf":  "application/pdf",
	".txt":  "text/plain; charset=utf-8",
	".html": "text/html; charset=utf-8",
	".htm":  "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "application/javascript; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".zip":  "application/zip",
}

// contentTypeFor 推断文件的 Content-Type：先查内置表，再查系统 mime，最后回退 octet-stream。
func contentTypeFor(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ct, ok := mimeOverrides[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// writeJSON 写 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// disposition 构造 Content-Disposition 头，用 RFC 5987 编码文件名以支持中文。
func disposition(filename string, inline bool) string {
	encoded := url.PathEscape(filename)
	if inline {
		return fmt.Sprintf("inline; filename*=UTF-8''%s", encoded)
	}
	return fmt.Sprintf("attachment; filename*=UTF-8''%s", encoded)
}

// RegisterRoutes 注册全部路由，返回 mux。
func RegisterRoutes() http.Handler {
	mux := http.NewServeMux()

	// 静态资源：从 embed FS 提供 /static/*
	staticSub, _ := fs.Sub(assets.StaticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// favicon
	mux.HandleFunc("/favicon.ico", handleFavicon)

	// 页面与上传 API 共用 /upload（按方法分发）
	mux.HandleFunc("/upload", handleUpload)

	// 其余页面
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/browse", handleBrowsePage)

	// 读取 API
	mux.HandleFunc("/api/dirs", handleListDirs)
	mux.HandleFunc("/api/files", handleListFiles)
	mux.HandleFunc("/api/download", handleDownload)
	mux.HandleFunc("/api/download-batch", handleDownloadBatch)

	// 删除 API
	mux.HandleFunc("/api/delete", handleDelete)
	mux.HandleFunc("/api/deletedir", handleDeleteDir)

	// 存储目录 API（仅本机可访问，isLocalRequest 守卫）
	mux.HandleFunc("/api/savedir", handleGetSaveDir)
	mux.HandleFunc("/api/setsavedir", handleSetSaveDir)

	return mux
}

// ----------------------------------------------------------------------
// 页面
// ----------------------------------------------------------------------

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	_ = tmpl.ExecuteTemplate(w, "home.html", nil)
}

func handleBrowsePage(w http.ResponseWriter, r *http.Request) {
	_ = tmpl.ExecuteTemplate(w, "browse.html", map[string]bool{"isLocal": isLocalRequest(r)})
}

// isLocalRequest 判断请求是否来自本机：loopback（127.0.0.1/::1）或来源 IP 等于本机 LAN IP。
// 用于 browse 页：本机访问显示删除按钮，远程（手机等）访问隐藏。
func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	lanIP := GetLanIP()
	if lanIP == "" || lanIP == "<本机IP>" {
		return false
	}
	return host == lanIP
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	data, err := assets.StaticFS.ReadFile("static/favicon.svg")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// handleUpload 分发 GET（上传页）与 POST（上传文件 API）。
func handleUpload(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = tmpl.ExecuteTemplate(w, "upload.html", nil)
	case http.MethodPost:
		handleUploadFiles(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// ----------------------------------------------------------------------
// 上传 API：POST /upload  (multipart: files[] + device)
// ----------------------------------------------------------------------

func handleUploadFiles(w http.ResponseWriter, r *http.Request) {
	// ParseMultipartForm 把超过 32MB 的部分写临时文件，支持大文件上传。
	// 表单字段（device）进内存，文件字段按上述阈值落盘或留内存。
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}

	device := safeDevice(r.FormValue("device"))
	deviceDir := filepath.Join(SaveDir, device)
	if err := os.MkdirAll(deviceDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "创建目录失败"})
		return
	}

	fhs := r.MultipartForm.File["files"]
	results := make([]map[string]any, 0, len(fhs))
	for _, fh := range fhs {
		now := time.Now()
		// 时间戳：YYYYMMDD_HHMMSS + 毫秒3位
		ts := now.Format("20060102_150405") + fmt.Sprintf("%03d", now.Nanosecond()/1_000_000)
		raw := basename(fh.Filename)
		if raw == "" || raw == "." {
			raw = "file"
		}
		saveName := ts + "_" + raw
		dstPath := filepath.Join(deviceDir, saveName)

		src, err := fh.Open()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "打开上传文件失败"})
			return
		}
		dst, err := os.Create(dstPath)
		if err != nil {
			_ = src.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "创建文件失败"})
			return
		}
		size, err := io.Copy(dst, src)
		_ = src.Close()
		_ = dst.Close()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "写入文件失败"})
			return
		}

		results = append(results, map[string]any{"name": saveName, "size": size})
		logOp("✅ %s | %s | %.2fMB", device, saveName, float64(size)/1_048_576.0)
	}

	// dir 返回绝对路径，前端显示 "已保存到：{dir}"
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"files": results,
		"dir":   deviceDir,
	})
}

// ----------------------------------------------------------------------
// 读取 API
// ----------------------------------------------------------------------

// handleListDirs GET /api/dirs → {"dirs":[{"name","count"}]}
func handleListDirs(w http.ResponseWriter, r *http.Request) {
	_ = os.MkdirAll(SaveDir, 0o755)
	entries, err := os.ReadDir(SaveDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "读取目录失败"})
		return
	}
	type dirItem struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	dirs := make([]dirItem, 0)
	for _, e := range entries {
		if !e.IsDir() || isHidden(e.Name()) {
			continue
		}
		sub, err := os.ReadDir(filepath.Join(SaveDir, e.Name()))
		if err != nil {
			continue
		}
		count := 0
		for _, f := range sub {
			if !f.IsDir() && !isHidden(f.Name()) {
				count++
			}
		}
		dirs = append(dirs, dirItem{Name: e.Name(), Count: count})
	}
	// 按名字升序
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"dirs": dirs})
}

// handleListFiles GET /api/files?dir= → {"dir":"","files":[{"name","size","mtime"}]}
// mtime 为 Unix 秒（前端 ts*1000 还原），按 mtime 倒序。
func handleListFiles(w http.ResponseWriter, r *http.Request) {
	dirName := r.URL.Query().Get("dir")
	d, ok := safeSubpath(SaveDir, dirName)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录不存在或非法"})
		return
	}
	info, err := os.Stat(d)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录不存在或非法"})
		return
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录不存在或非法"})
		return
	}
	type fileItem struct {
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		Mtime int64  `json:"mtime"`
	}
	files := make([]fileItem, 0)
	for _, e := range entries {
		if e.IsDir() || isHidden(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileItem{Name: e.Name(), Size: fi.Size(), Mtime: fi.ModTime().Unix()})
	}
	// 按 mtime 倒序
	sort.Slice(files, func(i, j int) bool { return files[i].Mtime > files[j].Mtime })
	writeJSON(w, http.StatusOK, map[string]any{"dir": dirName, "files": files})
}

// handleDownload GET /api/download?dir=&file=&inline=
// inline=1 时内联预览（不设 attachment），否则附件下载。
func handleDownload(w http.ResponseWriter, r *http.Request) {
	dirName := r.URL.Query().Get("dir")
	fileName := r.URL.Query().Get("file")
	inline := r.URL.Query().Get("inline") == "1" || r.URL.Query().Get("inline") == "true"

	d, ok := safeSubpath(SaveDir, dirName)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录不存在或非法"})
		return
	}
	f, ok := safeSubpath(d, fileName)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "文件不存在或非法"})
		return
	}
	info, err := os.Stat(f)
	if err != nil || info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "文件不存在或非法"})
		return
	}

	w.Header().Set("Content-Type", contentTypeFor(f))
	if !inline {
		w.Header().Set("Content-Disposition", disposition(fileName, false))
	}
	// http.ServeFile 支持 Range / If-Modified-Since；Content-Type 已设则不覆盖。
	http.ServeFile(w, r, f)
}

// handleDownloadBatch POST /api/download-batch  JSON: {"dir":"","files":[""]}}
// files 为空则打包目录下全部非隐藏文件。返回 zip，filename = {dir}.zip。
func handleDownloadBatch(w http.ResponseWriter, r *http.Request) {
	var dirName string
	var selected []string

	// 兼容两种提交方式：
	//   1. JSON body {"dir":"","files":[""]}  —— fetch 提交
	//   2. form-urlencoded dir=&files=&files= —— 隐藏 form 提交，走浏览器原生下载（兼容 Alook 等安卓浏览器）
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var req struct {
			Dir   string   `json:"dir"`
			Files []string `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		dirName, selected = req.Dir, req.Files
	} else {
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		dirName = r.PostFormValue("dir")
		selected = r.PostForm["files"]
	}

	d, ok := safeSubpath(SaveDir, dirName)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录不存在或非法"})
		return
	}
	info, err := os.Stat(d)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录不存在或非法"})
		return
	}

	// 确定要打包的文件列表
	var toZip []string
	if len(selected) == 0 {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if e.IsDir() || isHidden(e.Name()) {
				continue
			}
			toZip = append(toZip, filepath.Join(d, e.Name()))
		}
	} else {
		for _, name := range selected {
			f, ok := safeSubpath(d, name)
			if !ok {
				continue
			}
			if st, err := os.Stat(f); err != nil || st.IsDir() {
				continue
			}
			toZip = append(toZip, f)
		}
	}
	if len(toZip) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "没有可下载的文件"})
		return
	}

	// 写入临时 zip 文件，响应结束后删除
	tmp, err := os.CreateTemp("", "aellus-*.zip")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "打包失败"})
		return
	}
	tmpPath := tmp.Name()

	zw := zip.NewWriter(tmp)
	for _, fp := range toZip {
		st, err := os.Stat(fp)
		if err != nil {
			continue
		}
		hdr, err := zip.FileInfoHeader(st)
		if err != nil {
			continue
		}
		hdr.Name = filepath.Base(fp)
		hdr.Method = zip.Deflate
		zf, err := zw.CreateHeader(hdr)
		if err != nil {
			continue
		}
		src, err := os.Open(fp)
		if err != nil {
			continue
		}
		_, _ = io.Copy(zf, src)
		_ = src.Close()
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "打包失败"})
		return
	}
	_ = tmp.Close()

	logOp("📦 打包下载 | %s | %d 个文件", dirName, len(toZip))

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", disposition(dirName+".zip", false))
	http.ServeFile(w, r, tmpPath)
	// ServeFile 返回后文件已发完，安全删除
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		log.Printf("warn: 删除临时 zip 失败: %v", err)
	}
}

// handleDelete POST /api/delete  JSON: {"dir":"","files":["a","b"]}}
// 逐个删除文件，返回成功/失败列表。复用 safeSubpath 防路径穿越。
func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method Not Allowed"})
		return
	}
	var req struct {
		Dir   string   `json:"dir"`
		Files []string `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	d, ok := safeSubpath(SaveDir, req.Dir)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录不存在或非法"})
		return
	}
	deleted := make([]string, 0, len(req.Files))
	failed := make([]map[string]any, 0)
	for _, name := range req.Files {
		f, ok := safeSubpath(d, name)
		if !ok {
			failed = append(failed, map[string]any{"name": name, "error": "非法路径"})
			continue
		}
		if err := os.Remove(f); err != nil {
			failed = append(failed, map[string]any{"name": name, "error": err.Error()})
			continue
		}
		deleted = append(deleted, name)
	}
	if len(deleted) > 0 {
		logOp("🗑️ 删除 | %s | %d 个文件: %s", req.Dir, len(deleted), strings.Join(deleted, ", "))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted, "failed": failed})
}

// handleDeleteDir POST /api/deletedir JSON: {"dirs":["a","b"]}
// 逐个删除整个目录（含其下所有文件），返回成功/失败列表。复用 safeSubpath 防路径穿越。
func handleDeleteDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method Not Allowed"})
		return
	}
	var req struct {
		Dirs []string `json:"dirs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	deleted := make([]string, 0, len(req.Dirs))
	failed := make([]map[string]any, 0)
	for _, name := range req.Dirs {
		d, ok := safeSubpath(SaveDir, name)
		if !ok {
			failed = append(failed, map[string]any{"name": name, "error": "非法路径"})
			continue
		}
		if err := os.RemoveAll(d); err != nil {
			failed = append(failed, map[string]any{"name": name, "error": err.Error()})
			continue
		}
		deleted = append(deleted, name)
	}
	if len(deleted) > 0 {
		logOp("🗑️ 删除目录 | %d 个: %s", len(deleted), strings.Join(deleted, ", "))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted, "failed": failed})
}

// ----------------------------------------------------------------------
// 存储目录 API（仅本机访问）
// ----------------------------------------------------------------------

// handleGetSaveDir GET /api/savedir → {"dir":"...","default":"..."}
// 仅本机可查；非本机返回 403，前端据此隐藏存储目录区块，只显示赞赏信息。
func handleGetSaveDir(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "仅本机可查看"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dir": SaveDir, "default": DefaultSaveDir()})
}

// handleSetSaveDir POST /api/setsavedir {"dir":"..."} → {"ok":true,"dir":"..."}
// 修改运行时存储目录。仅本机可改；先 MkdirAll 验证可写，再赋值全局 SaveDir。
//
// 并发说明：SaveDir 是全局 string，修改为低频手动操作（用户点「修改」），
// 读取为高频（每个请求）。此处直接赋值——string 赋值不会撕裂成乱码，
// 读到旧值或新值均为有效目录，实际安全。
func handleSetSaveDir(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "仅本机可修改"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method Not Allowed"})
		return
	}
	var req struct {
		Dir string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	dir := strings.TrimSpace(req.Dir)
	if dir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录不能为空"})
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "路径无效"})
		return
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "创建目录失败: " + err.Error()})
		return
	}
	SaveDir = abs
	logOp("📁 存储目录已修改 | %s", abs)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dir": abs})
}
