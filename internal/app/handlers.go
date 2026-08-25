package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// === 路由处理 ===

// handleUpload POST /upload 接收 multipart/form-data。
//
// 注意：不能用 r.ParseMultipartForm。它底层 ReadForm 默认限制最多 1000 个 part，
// 而前端每个文件都发 files + rels 两个字段（N 文件 = 2N+1 parts），
// 文件夹文件数破千会触发 "multipart: message too large" -> 400。
// 改为手动 multipart.NewReader + NextPart 逐 part 解析，无 1000 parts 上限。
func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "multipart/form-data") {
		a.writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Message: "请求 Content-Type 不是 multipart/form-data"})
		return
	}
	_, params, perr := mime.ParseMediaType(ct)
	if perr != nil {
		a.writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Message: "请求 Content-Type 解析失败：" + perr.Error()})
		return
	}
	boundary := params["boundary"]
	if boundary == "" {
		a.writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Message: "multipart 缺少 boundary 参数"})
		return
	}

	mr := multipart.NewReader(r.Body, boundary)

	device := "default"
	upIP := clientIP(r)                    // 上传来源 IP（同一设备换任何浏览器都一致）
	upUA := deviceSigFromUA(r.UserAgent()) // 上传来源 UA 设备签名（IP 变化后的兜底）
	var rels []string                      // 所有文件的相对路径，按出现顺序收集
	var scratch bytes.Buffer
	buf := make([]byte, 1<<20) // 1MB 缓冲区，分块写入

	// pending 记录每个文件 part 落盘后的临时文件与原始文件名。
	// 由于前端 parts 顺序是 files[0], rels[0], files[1], rels[1]…，
	// rels[i] 在 files[i] 之后才出现，必须等整个 multipart 解析完、
	// rels 列表齐全后，才能正确配对，故先落临时文件、后处理。
	type pendingFile struct {
		tmpPath  string
		fileName string
	}
	var pending []pendingFile

	// cleanupPending 清理所有已落盘的临时文件，防止泄漏到系统临时目录。
	cleanupPending := func() {
		for _, pf := range pending {
			os.Remove(pf.tmpPath)
		}
	}

	for {
		part, nerr := mr.NextPart()
		if nerr == io.EOF {
			break
		}
		if nerr != nil {
			cleanupPending()
			a.writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Message: "请求解析失败（可能请求体过大或被截断）：" + nerr.Error()})
			return
		}

		formName := part.FormName()
		fileName := part.FileName()
		switch {
		case formName == "device":
			scratch.Reset()
			io.Copy(&scratch, part)
			device = sanitizeDevice(strings.TrimSpace(scratch.String()))
		case formName == "rels":
			scratch.Reset()
			io.Copy(&scratch, part)
			rels = append(rels, scratch.String())
		case formName == "files" && fileName != "":
			// 先把文件内容流式写入临时文件（不限制大小），之后再用 rels 配对重命名。
			tmp, terr := os.CreateTemp("", "aellus-upload-*")
			if terr != nil {
				part.Close()
				cleanupPending()
				a.writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false})
				return
			}
			if _, werr := io.CopyBuffer(tmp, part, buf); werr != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				part.Close()
				cleanupPending()
				a.writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false})
				return
			}
			tmp.Close()
			part.Close()
			pending = append(pending, pendingFile{tmpPath: tmp.Name(), fileName: fileName})
		default:
			// 非预期字段、或无文件名的 files 段：跳过并释放该 part。
			part.Close()
		}
	}

	if len(pending) == 0 {
		a.writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Message: "没有收到任何文件（files 为空）"})
		return
	}

	// rels 列表已齐全，现在按索引配对并落盘到设备目录。
	deviceDir := filepath.Join(a.getSaveDir(), device)
	if mkErr := os.MkdirAll(deviceDir, 0755); mkErr != nil {
		cleanupPending()
		a.writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false})
		return
	}
	// 记录设备目录归属（首个上传者为准，不覆盖）
	a.recordOwner(a.getSaveDir(), device, upIP, upUA)

	uploaded := []UploadFileResp{}
	for j, pf := range pending {
		// 优先用 rels 提供的相对路径(保留层级)；缺失时退回 filename(纯文件名)。
		rawName := pf.fileName
		if j < len(rels) && rels[j] != "" {
			rawName = rels[j]
		}
		dstPath, displayName, terr := resolveUploadTarget(deviceDir, rawName)
		if terr != nil {
			os.Remove(pf.tmpPath)
			cleanupPending()
			a.writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Message: "文件无法保存：" + rawName + "（" + terr.Error() + "）"})
			return
		}

		// 尝试原地 rename（同文件系统最快）；跨设备则回退到分块拷贝。
		if rerr := os.Rename(pf.tmpPath, dstPath); rerr != nil {
			src, oerr := os.Open(pf.tmpPath)
			if oerr != nil {
				cleanupPending()
				a.writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false})
				return
			}
			dst, cerr := os.Create(dstPath)
			if cerr != nil {
				src.Close()
				os.Remove(pf.tmpPath)
				cleanupPending()
				a.writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false})
				return
			}
			if _, werr := io.CopyBuffer(dst, src, buf); werr != nil {
				src.Close()
				dst.Close()
				os.Remove(pf.tmpPath)
				os.Remove(dstPath)
				cleanupPending()
				a.writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false})
				return
			}
			src.Close()
			dst.Close()
			os.Remove(pf.tmpPath)
		}

		// 记录本项归属（文件级）：文件记在其所在目录的 manifest；
		// 文件夹上传（rawName 含层级）时，逐级把新出现的文件夹也记为本次上传来源。
		a.recordOwner(filepath.Dir(dstPath), filepath.Base(dstPath), upIP, upUA)
		if rel := strings.ReplaceAll(rawName, "\\", "/"); strings.Contains(rel, "/") {
			segs := strings.Split(rel, "/")
			acc := deviceDir
			for _, seg := range segs[:len(segs)-1] {
				if seg == "" {
					continue
				}
				acc = filepath.Join(acc, seg)
				a.recordOwner(filepath.Dir(acc), filepath.Base(acc), upIP, upUA)
			}
		}

		info, _ := os.Stat(dstPath)
		size := int64(0)
		mtime := int64(0)
		if info != nil {
			size = info.Size()
			mtime = info.ModTime().Unix()
		}
		uploaded = append(uploaded, UploadFileResp{Name: displayName, Size: size, Mtime: mtime})
		a.logOp(fmt.Sprintf("上传成功 设备=%s 文件=%s 大小=%.2fMB", device, displayName, float64(size)/1048576.0))
	}

	a.writeJSON(w, http.StatusOK, UploadResp{OK: true, Files: uploaded, Dir: device})
}

// dirStats 递归统计目录：返回总大小（字节，含子目录内文件）、
// 文件总数（仅文件，不含目录本身）、最新修改时间（Unix 时间戳，含子目录与文件）。
// 若某路径无法读取则忽略该分支，不影响其余统计。纯函数。
func dirStats(path string) (size int64, count int, mtime int64) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, 0, 0
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mt := info.ModTime().Unix()
		if mt > mtime {
			mtime = mt
		}
		if e.IsDir() {
			s, c, m := dirStats(filepath.Join(path, e.Name()))
			size += s
			count += c
			if m > mtime {
				mtime = m
			}
		} else {
			size += info.Size()
			count++
		}
	}
	return
}

// handleDirs GET /api/dirs 列出所有设备目录及其文件数、总大小、最新修改时间。
func (a *App) handleDirs(w http.ResponseWriter, r *http.Request) {
	dirs := []DirInfo{} // 用空切片而非 nil，确保 JSON 输出为 [] 而非 null
	entries, err := os.ReadDir(a.getSaveDir())
	if err == nil {
		meIP, meUA := clientIP(r), deviceSigFromUA(r.UserAgent())
		for _, e := range entries {
			// 过滤：只要目录，且跳过以 "." 开头的隐藏目录
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			// 递归统计该目录的总大小、文件数、最新修改时间
			size, count, mtime := dirStats(filepath.Join(a.getSaveDir(), e.Name()))
			dirs = append(dirs, DirInfo{Name: e.Name(), Count: count, Size: size, Mtime: mtime,
				Deletable: deletable(a.ownerOf(a.getSaveDir(), e.Name()), meIP, meUA)})
		}
	}
	// 根目录（未命名设备）下直接存放的文件，也作为一个目录项展示；
	// 其内部 Name 为空字符串，前端显示为「未命名设备」，并可在根目录下列出。
	// 注意：这里只统计根目录的直接文件（不递归），因为子目录已作为单独卡片展示，避免重复计数。
	rootSize, rootCount, rootMtime := int64(0), int64(0), int64(0)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			rootSize += info.Size()
			rootCount++
			mt := info.ModTime().Unix()
			if mt > rootMtime {
				rootMtime = mt
			}
		}
	}
	if rootCount > 0 {
		dirs = append(dirs, DirInfo{Name: "", Count: int(rootCount), Size: rootSize, Mtime: rootMtime})
	}
	a.writeJSON(w, http.StatusOK, DirsResp{Dirs: dirs})
}

// handleFiles GET /api/files?dir=xxx 列出某目录下的文件与子目录。
// 文件夹排在最前，其余按修改时间倒序（最新的在最上面）。
func (a *App) handleFiles(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	dirAbs, err := a.resolveDir(dir)
	if err != nil {
		a.writeJSON(w, http.StatusBadRequest, FilesResp{Error: "目录不存在或非法"})
		return
	}

	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		a.writeJSON(w, http.StatusInternalServerError, FilesResp{Error: "读取失败"})
		return
	}

	meIP, meUA := clientIP(r), deviceSigFromUA(r.UserAgent())
	files := []FileInfo{} // 空切片，确保无内容时输出 [] 而非 null
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fi := FileInfo{
			Name:      e.Name(),
			Size:      info.Size(),
			Mtime:     info.ModTime().Unix(),
			IsDir:     e.IsDir(),
			Count:     0,
			Deletable: deletable(a.ownerOf(dirAbs, e.Name()), meIP, meUA),
		}
		// 文件夹：递归计算总大小、文件数、最新修改时间
		if e.IsDir() {
			s, c, m := dirStats(filepath.Join(dirAbs, e.Name()))
			fi.Size = s
			fi.Count = c
			fi.Mtime = m
		}
		files = append(files, fi)
	}
	// 文件夹排在最前，其余按修改时间倒序。
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Mtime > files[j].Mtime
	})

	a.writeJSON(w, http.StatusOK, FilesResp{Dir: dir, Files: files})
}

// handleDownload GET /api/download?dir=xxx&file=xxx[&inline=1]
// inline=1：浏览器内联预览（设 Content-Type，不设 Content-Disposition）
// 无 inline 或 inline=0：触发下载（设 Content-Disposition: attachment）
func (a *App) handleDownload(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	file := r.URL.Query().Get("file")

	dirAbs, err := a.resolveDir(dir)
	if err != nil {
		http.Error(w, "非法目录", http.StatusBadRequest)
		return
	}
	full, err := a.resolveFile(dirAbs, file)
	if err != nil {
		http.Error(w, "非法文件", http.StatusBadRequest)
		return
	}

	f, err := os.Open(full)
	if err != nil {
		http.Error(w, "打开失败", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "读取失败", http.StatusInternalServerError)
		return
	}
	// 目录不能作为单个文件下载：显式拒绝，避免 ServeContent 读目录返回 0 字节。
	// （目录打包走 /api/download-batch，删除目录走 /api/delete。）
	if info.IsDir() {
		http.Error(w, "不支持下载目录", http.StatusBadRequest)
		return
	}

	inline := r.URL.Query().Get("inline") == "1"
	ext := strings.ToLower(filepath.Ext(full))
	if inline && isInlineSafe(ext) {
		// 内联预览：仅允许安全类型（图片/视频/音频/PDF）直接显示，
		// HTML/SVG 等可执行类型不内联，防止存储型 XSS。
		ctype := mime.TypeByExtension(ext)
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("X-Content-Type-Options", "nosniff")
	} else {
		// 下载或危险类型：设置 Content-Disposition: attachment。
		// filename*=UTF-8'' 是 RFC 5987 标准，保证中文文件名在浏览器里不乱码。
		w.Header().Set("Content-Disposition",
			`attachment; filename="`+file+`"; filename*=UTF-8''`+url.QueryEscape(file))
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}

	// ServeContent 会自己处理 Range 请求、Content-Length、Last-Modified 等，
	// 它不会自动加 Content-Disposition，所以上面的设置能原样生效。
	http.ServeContent(w, r, file, info.ModTime(), f)
}

// handleBatchDownload POST /api/download-batch 把若干文件（或整目录）打包成 ZIP 下载。
func (a *App) handleBatchDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 限制请求体大小，防止恶意超大 JSON。
	var req BatchReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "请求体非法", http.StatusBadRequest)
		return
	}

	dirAbs, err := a.resolveDir(req.Dir)
	if err != nil {
		http.Error(w, "非法目录", http.StatusBadRequest)
		return
	}

	// 确定要打包的文件名列表（names 为相对 dirAbs 的路径，可含子目录层级）
	var names []string
	if len(req.Files) == 0 {
		// files 为空 -> 递归打包该目录（含子目录）全部非隐藏文件，保留层级结构
		err := filepath.WalkDir(dirAbs, func(p string, d os.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			rel, rerr := filepath.Rel(dirAbs, p)
			if rerr != nil {
				return rerr
			}
			names = append(names, rel)
			return nil
		})
		if err != nil {
			http.Error(w, "读取目录失败", http.StatusInternalServerError)
			return
		}
	} else {
		for _, f := range req.Files {
			if !isValidName(f) { // 逐个校验，防穿越
				http.Error(w, "非法文件名", http.StatusBadRequest)
				return
			}
			names = append(names, f)
		}
	}

	if len(names) == 0 {
		http.Error(w, "没有可下载的文件", http.StatusBadRequest)
		return
	}

	// 创建系统临时目录下的临时 ZIP 文件。
	tmp, err := os.CreateTemp("", "aellus-*.zip")
	if err != nil {
		http.Error(w, "创建临时文件失败", http.StatusInternalServerError)
		return
	}
	// 关键：响应结束后自动删除临时 ZIP（无论成功失败）。
	// defer 会在本函数返回（即 http.ServeFile 写完之后）才执行。
	defer os.Remove(tmp.Name())

	buf := make([]byte, 1<<20) // 1MB 缓冲
	zw := zip.NewWriter(tmp)
	for _, name := range names {
		full := filepath.Join(dirAbs, name)
		if !isInside(dirAbs, full) { // 再次校验，双保险
			continue
		}
		src, err := os.Open(full)
		if err != nil {
			continue
		}
		// 在 ZIP 里用原始文件名（不要用带 timestamp 的磁盘名，方便用户识别）
		zwEntry, err := zw.Create(name)
		if err != nil {
			src.Close()
			continue
		}
		_, _ = io.CopyBuffer(zwEntry, src, buf)
		src.Close()
	}
	zw.Close() // 必须 Close 才能写完 ZIP 中央目录
	tmp.Close()

	// 设置下载响应头（zip 文件名取 dir 最后一段，避免路径分隔符出现在文件名里）
	zipName := filepath.Base(req.Dir)
	if zipName == "" || zipName == "." {
		zipName = "files"
	}
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+zipName+`.zip"; filename*=UTF-8''`+url.QueryEscape(zipName+".zip"))
	w.Header().Set("Content-Type", "application/zip")

	// 把临时 ZIP 直接流式返回给浏览器
	http.ServeFile(w, r, tmp.Name())

	// 操作日志：目录名、文件数
	a.logOp(fmt.Sprintf("批量下载 目录=%s 文件数=%d", req.Dir, len(names)))
}

// handleDelete POST /api/delete 删除单个文件或目录（{dir, file}）。
// 复用 resolveDir/resolveFile 做安全校验（防路径穿越、隐藏文件、非法名）；
// 目录用 os.RemoveAll 递归删除，文件用 os.Remove。
func (a *App) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Dir  string `json:"dir"`
		File string `json:"file"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体非法"})
		return
	}
	dirAbs, err := a.resolveDir(req.Dir)
	if err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	full, err := a.resolveFile(dirAbs, req.File)
	if err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// 设备归属校验：目标项由其上传来源独占删除权（同 IP 或同 UA 签名可删；旧数据放行）。
	meIP, meUA := clientIP(r), deviceSigFromUA(r.UserAgent())
	if !deletable(a.ownerOf(dirAbs, req.File), meIP, meUA) {
		a.writeJSON(w, http.StatusForbidden, map[string]string{"error": "其他设备上传的内容，仅可下载不可删除"})
		return
	}
	// 用 RemoveAll 同时支持文件与目录（目录递归删除）。
	if err := os.RemoveAll(full); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "删除失败"})
		return
	}
	a.removeOwner(dirAbs, req.File) // 同步清理 manifest 中的归属记录
	a.logOp(fmt.Sprintf("删除 目录=%s 目标=%s", req.Dir, req.File))
	a.writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}
