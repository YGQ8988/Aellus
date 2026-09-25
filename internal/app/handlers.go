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
	"time"
)

// === 路由处理 ===

// uploadTempDirName 临时文件目录名（位于保存目录内，以 "." 开头，因此不会出现在列表与打包结果里）。
// 上传的临时文件与批量下载的临时 ZIP 都放在这里：与保存目录同盘（rename 快、空间算对盘），
// 不用系统临时目录（NAS 上常是 tmpfs / 小根分区，写满会引发系统级故障）。
const uploadTempDirName = ".aellus-tmp"

// 上传表单里的文本字段上限：局域网内任何人可上传（设计如此），
// 但文本字段如果不限长，就能用很小的请求撑爆服务端内存
// （实测：20MB 的 rels 字段 → 服务端累计分配 325MB）。
const (
	maxDeviceFieldBytes = 4 << 10 // device 字段（设备名）
	maxRelPathBytes     = 8 << 10 // 单个 rels 字段（相对路径，Linux PATH_MAX 为 4096）
	maxRelEntries       = 20000   // rels 条数上限
	maxBatchFiles       = 10000   // 批量下载一次可指定的文件条数上限（防重复放大）
)

// uploadTempDir 返回临时文件目录：保存目录内的隐藏子目录 <saveDir>/.aellus-tmp。
// 上传的临时文件与批量下载的临时 ZIP 都落在这里。
//
// 为什么不用系统临时目录（os.TempDir / /tmp）：
//   - 空间算错盘：占用的是系统盘/根分区（NAS 上常是 tmpfs 或很小的小根分区），
//     大文件传输会把它写满，导致系统级故障；保存目录那块的卷才是用户预期消耗空间的地方；
//   - 必然多写一遍：临时目录与保存目录跨文件系统时 os.Rename 必然失败（EXDEV），
//     只能整份复制 → 峰值磁盘占用 2× 文件大小、耗时翻倍。
//
// 该目录以 "." 开头：列表接口（/api/dirs、/api/files）过滤它，批量下载（WalkDir）
// 整棵跳过（见 handleBatchDownload），也无法通过 API 下载 / 删除其中的文件。
//
// 返回空串表示保存目录不可用或该子目录是软链等异常。
// 调用方【必须中止操作并返回错误】，不要回退系统临时目录——那正是上面要避免的
// 情形（NAS 上 tmpfs / 小根分区被写满会引发系统级故障）；而且此时文件最终也无法
// 落盘到保存目录，与其白白传完再失败，不如一开始就拒绝。
func (a *App) uploadTempDir() string {
	dir := filepath.Join(a.getSaveDir(), uploadTempDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ""
	}
	// 防软链：该目录若被替换成指向外部的软链，临时文件就会写到保存目录之外。
	if !realInside(a.getSaveDir(), dir) {
		return ""
	}
	return dir
}

// cleanStaleUploadTemps 清理临时目录里的残留文件（上传/打包过程中进程被强杀，来不及删除的）。
// 只删超过 6 小时的，避免误删正在传输的文件；只处理该目录下的普通文件。best-effort。
func (a *App) cleanStaleUploadTemps(dir string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-6 * time.Hour)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// 只清理上传临时文件：批量下载的 ZIP 也放在这个目录里，而打包大目录可能持续
		// 很久（低速写入时 mtime 长时间不更新），按 6 小时一刀切会删掉还在进行中的
		// 打包产物，导致下载拿到半截文件。
		if !strings.HasPrefix(e.Name(), "aellus-upload-") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

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
	upDevID := deviceID(r) // 设备 ID（供设备名映射记录）
	// 临时文件落地点（保存目录内隐藏子目录，失败时回退系统临时目录），并顺手清理上次残留。
	tmpDir := a.uploadTempDir()
	if tmpDir == "" {
		// 保存目录不可用：不回退系统临时目录（见 uploadTempDir 注释），直接拒绝，
		// 否则大文件会把 NAS 的系统盘/根分区写满。
		a.writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false, Message: "保存目录不可用，无法接收上传"})
		return
	}
	a.cleanStaleUploadTemps(tmpDir)
	var rels []string // 所有文件的相对路径，按出现顺序收集
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

	// cleanupPending 清理所有已落盘的临时文件，避免上传失败时残留占用保存目录空间。
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
			// 限长读取：超长则整个字段丢弃（设备名保持 default），避免被用来撑内存。
			if n, _ := io.CopyN(&scratch, part, maxDeviceFieldBytes+1); n <= maxDeviceFieldBytes {
				device = sanitizeDevice(strings.TrimSpace(scratch.String()))
			}
			part.Close()
		case formName == "rels":
			if len(rels) >= maxRelEntries {
				part.Close()
				continue
			}
			scratch.Reset()
			// 限长读取：超长则记为无效（该文件回退用 multipart 文件名），
			// 而不是把截断后的路径当文件名用。
			n, _ := io.CopyN(&scratch, part, maxRelPathBytes+1)
			part.Close()
			if n > maxRelPathBytes {
				rels = append(rels, "")
				continue
			}
			rels = append(rels, scratch.String())
		case formName == "files" && fileName != "":
			// 先把文件内容流式写入临时文件（内容不限大小），之后再用 rels 配对重命名。
			// 临时文件放保存目录内的隐藏子目录（同盘 rename、空间算对盘），不可用时回退系统临时目录。
			tmp, terr := os.CreateTemp(tmpDir, "aellus-upload-*")
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
	// 防软链逃逸：设备目录可能是授权目录内被替换成指向外部的软链（能写共享目录的人
	// 可以创建），MkdirAll 与后续写入都会顺着软链落到授权目录之外 → 先做真实路径校验。
	if !realInside(a.getSaveDir(), deviceDir) {
		cleanupPending()
		a.writeJSON(w, http.StatusForbidden, UploadResp{OK: false, Message: "目标目录不在授权范围内"})
		return
	}
	if mkErr := os.MkdirAll(deviceDir, 0755); mkErr != nil {
		cleanupPending()
		a.writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false})
		return
	}
	// 记录设备名映射（供下次上传页自动填充，换浏览器/清 localStorage 也能取回）
	a.recordDeviceName(upDevID, device)

	uploaded := []UploadFileResp{}
	for j, pf := range pending {
		// 优先用 rels 提供的相对路径(保留层级)；缺失时退回 filename(纯文件名)。
		rawName := pf.fileName
		if j < len(rels) && rels[j] != "" {
			rawName = rels[j]
		}
		dstPath, displayName, terr := resolveUploadTarget(a.getSaveDir(), deviceDir, rawName)
		if terr != nil {
			os.Remove(pf.tmpPath)
			cleanupPending()
			// 不回显 terr 原文（含内部路径），只说明是名称/路径不合法。
			a.writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Message: "文件无法保存：" + rawName + "（名称或路径不合法）"})
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
			// 回退分支的 os.Create 会跟随软链：realInside 校验与真正写入之间有窗口，
			// 目标若在此期间被换成软链，就会写到授权目录之外。用 Lstat 复核一次——
			// 它不跟随链接，能认出"这一层就是软链"（os.Rename 本就不跟随，故仅此分支需要）。
			if fi, lerr := os.Lstat(dstPath); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
				src.Close()
				os.Remove(pf.tmpPath)
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
		for _, e := range entries {
			// 过滤：只要目录，且跳过以 "." 开头的隐藏目录
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			// 递归统计该目录的总大小、文件数、最新修改时间
			size, count, mtime := dirStats(filepath.Join(a.getSaveDir(), e.Name()))
			dirs = append(dirs, DirInfo{Name: e.Name(), Count: count, Size: size, Mtime: mtime})
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
	part, total, page, size := pageScope(r, dirs)
	a.writeJSON(w, http.StatusOK, DirsResp{Dirs: part, CanDelete: a.canManage(r), Total: total, Page: page, Size: size})
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
			Name:  e.Name(),
			Size:  info.Size(),
			Mtime: info.ModTime().Unix(),
			IsDir: e.IsDir(),
			Count: 0,
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
	// 文件夹排在最前，其余按修改时间倒序；mtime 相同时按名字排序（保证全序稳定，
	// 分页切分跨页不重不漏——同秒写入的多个文件否则顺序不确定）。
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		if files[i].Mtime != files[j].Mtime {
			return files[i].Mtime > files[j].Mtime
		}
		return files[i].Name < files[j].Name
	})

	part, total, page, size := pageScope(r, files)
	a.writeJSON(w, http.StatusOK, FilesResp{Dir: dir, Files: part, CanDelete: a.canManage(r), Total: total, Page: page, Size: size})
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

	// 输出头统一走 setFileOutputHeaders（见 fileout.go）：inline=1 且扩展名在
	// 内联白名单内才允许浏览器直接渲染，其余（含 HTML/SVG/XML）一律按附件下载，
	// 防止上传的文件在应用同源下执行脚本（存储型 XSS）。
	mode := outputDownload
	if r.URL.Query().Get("inline") == "1" {
		mode = outputInlineMedia
	}
	// attachment 模式用「显示名」（去掉上传时拼接的时间戳前缀）作为下载文件名，
	// 与页面展示一致；手机扫码直接访问下载 URL 时的文件名也由这里决定。
	outName := file
	if mode == outputDownload {
		outName = stripUploadPrefix(file)
	}
	setFileOutputHeaders(w, outName, mode)

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
				// 隐藏目录必须整棵跳过（SkipDir）：只 return nil 仅表示"不把目录本身加入列表"，
				// WalkDir 仍会进入它，把里面的普通文件打包出去（实测会泄露 .aellus-tmp 里的
				// 上传临时文件、以及其它隐藏目录的内容）。dirAbs 自身可能以 "." 开头，不能跳。
				if p != dirAbs && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
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
		// 条数上限 + 去重：请求体虽只限 1MB，却能列出数十万条且允许重复，
		// 每个条目都会重新 os.Open + 压缩一遍——1MB 请求可放大成数十 TB 写入
		// （临时 ZIP 落在保存卷上）。这是免登录接口，必须堵住。
		if len(req.Files) > maxBatchFiles {
			http.Error(w, "一次打包的文件过多", http.StatusBadRequest)
			return
		}
		seenReq := map[string]bool{}
		for _, f := range req.Files {
			if !isValidName(f) { // 逐个校验，防穿越
				http.Error(w, "非法文件名", http.StatusBadRequest)
				return
			}
			if seenReq[f] {
				continue
			}
			seenReq[f] = true
			names = append(names, f)
		}
	}

	if len(names) == 0 {
		http.Error(w, "没有可下载的文件", http.StatusBadRequest)
		return
	}

	// 创建临时 ZIP 文件：与上传临时文件同放保存目录内的隐藏子目录（同盘、空间算对盘）。
	// 不用系统临时目录——NAS 上它常是 tmpfs / 很小的小根分区，打包大目录会把它写满，
	// 造成系统级故障。uploadTempDir 返回空串时 CreateTemp 自动回退系统临时目录，保证可用。
	tmpDir := a.uploadTempDir()
	if tmpDir == "" {
		http.Error(w, "保存目录不可用，无法打包", http.StatusInternalServerError)
		return
	}
	tmp, err := os.CreateTemp(tmpDir, "aellus-download-*.zip")
	if err != nil {
		http.Error(w, "创建临时文件失败", http.StatusInternalServerError)
		return
	}
	// 关键：响应结束后自动删除临时 ZIP（无论成功失败）。
	// defer 会在本函数返回（即 http.ServeFile 写完之后）才执行。
	defer os.Remove(tmp.Name())

	buf := make([]byte, 1<<20) // 1MB 缓冲
	zw := zip.NewWriter(tmp)
	// seen 记录已写入 ZIP 的条目名（去时间戳前缀后），用于给重名条目加序号。
	seen := map[string]int{}
	for _, name := range names {
		full := filepath.Join(dirAbs, name)
		// 双重校验：isInside 防路径穿越（仅字符串判断），realInside 解析符号链接防逃逸。
		// 递归打包时 WalkDir 不跟随软链，但下方 os.Open 会跟随；若软链指向授权目录外，
		// isInside 字符串判断会放行，故需 realInside 兜底（与单文件下载 resolveFile 一致）。
		if !isInside(dirAbs, full) || !realInside(dirAbs, full) {
			continue
		}
		src, err := os.Open(full)
		if err != nil {
			continue
		}
		// 在 ZIP 里用原始文件名（不要用带 timestamp 的磁盘名，方便用户识别）——
		// 与页面列表展示、单文件下载保持一致（见 stripUploadPrefix）。
		// 去前缀后可能重名（同名文件被多次上传），重名时加序号：ZIP 允许同名条目，
		// 但解压时后者会覆盖前者，等于静默丢文件。
		entryName := stripUploadPrefix(name)
		if entryName == "" {
			// 极端情况：文件名恰好等于纯时间戳前缀（去完前缀为空）→ 回退用原名，
			// 避免写入空名条目（解压时会产生无名文件）。
			entryName = name
		}
		if n, dup := seen[entryName]; dup {
			ext := filepath.Ext(entryName)
			base := strings.TrimSuffix(entryName, ext)
			entryName = fmt.Sprintf("%s(%d)%s", base, n+1, ext)
		}
		seen[stripUploadPrefix(name)]++
		zwEntry, err := zw.Create(entryName)
		if err != nil {
			src.Close()
			continue
		}
		if _, werr := io.CopyBuffer(zwEntry, src, buf); werr != nil {
			// 写入失败（多为保存目录空间不足）：记一条日志便于排查，其余文件继续打包。
			a.logOp(fmt.Sprintf("批量下载 写入失败 文件=%s 错误=%v", name, werr))
		}
		src.Close()
	}
	zw.Close() // 必须 Close 才能写完 ZIP 中央目录
	tmp.Close()

	// 设置下载响应头（zip 文件名取 dir 最后一段，避免路径分隔符出现在文件名里）
	zipName := filepath.Base(req.Dir)
	if zipName == "" || zipName == "." {
		zipName = "files"
	}
	safeZip := strings.NewReplacer(`"`, "_", "\r", "", "\n", "").Replace(zipName)
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+safeZip+`.zip"; filename*=UTF-8''`+url.PathEscape(zipName+".zip"))
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
		// 不回显 err：解析错误里带有完整路径，交给客户端等于泄露目录结构。
		a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目录不存在或非法"})
		return
	}
	full, err := a.resolveFile(dirAbs, req.File)
	if err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件不存在或非法"})
		return
	}
	// 删除权限：由 canManage 判定——
	//   - 飞牛应用且已接入统一网关：请求须携带网关可信注入头 X-Trim-Userid（仅门户内已登录用户才有），否则拒绝；
	//   - 飞牛应用未接入网关 / 非飞牛应用：仅本机（来源 IP 等于服务 IP）可删。
	// 裸 TCP 端口（局域网直连）的请求在到达此处前已被剥离伪造的 X-Trim-* 头，无法越权。
	if !a.canManage(r) {
		a.writeJSON(w, http.StatusForbidden, map[string]string{"error": "无删除权限"})
		return
	}
	// 用 RemoveAll 同时支持文件与目录（目录递归删除）。
	if err := os.RemoveAll(full); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "删除失败"})
		return
	}
	a.logOp(fmt.Sprintf("删除 目录=%s 目标=%s", req.Dir, req.File))
	a.writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}
