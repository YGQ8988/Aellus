package app

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// === 缩略图 ===

// isDecodeableImage 判断扩展名是否为标准库可解码的图片（jpeg/png/gif）。
// webp/heic/bmp 等标准库解不了，这类直接回退为原文件返回。纯函数。
func isDecodeableImage(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif":
		return true
	}
	return false
}

// isInlineSafe 判断扩展名是否允许浏览器内联预览（inline=1）。
// HTML/SVG/XML 等可携带脚本的类型不在此列，防止存储型 XSS。纯函数。
func isInlineSafe(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".ico", ".avif", ".heic",
		".mp4", ".webm", ".mov", ".avi", ".mkv", ".m4v",
		".mp3", ".wav", ".ogg", ".flac", ".aac", ".m4a",
		".pdf":
		return true
	}
	return false
}

// resizeBox 用“区域均值”对图片做缩小（box downsampling），输出质量为缩略图够用、
// 不产生明显锯齿，且纯标准库实现、零依赖。sw*sh 再大也只是 O(像素数) 的一次遍历。纯函数。
func resizeBox(src image.Image, dw, dh int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	if sw == 0 || sh == 0 || dw == 0 || dh == 0 {
		return dst
	}
	fx := float64(sw) / float64(dw)
	fy := float64(sh) / float64(dh)
	for y := 0; y < dh; y++ {
		y0 := int(float64(y) * fy)
		y1 := int(float64(y+1) * fy)
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if y1 > sh {
			y1 = sh
		}
		for x := 0; x < dw; x++ {
			x0 := int(float64(x) * fx)
			x1 := int(float64(x+1) * fx)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if x1 > sw {
				x1 = sw
			}
			var r, g, b, a uint64
			var n int
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					pr, pg, pb, pa := src.At(sb.Min.X+sx, sb.Min.Y+sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					b += uint64(pb)
					a += uint64(pa)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8((r / uint64(n)) >> 8),
				G: uint8((g / uint64(n)) >> 8),
				B: uint8((b / uint64(n)) >> 8),
				A: uint8((a / uint64(n)) >> 8),
			})
		}
	}
	return dst
}

// 缩略图尺寸上限（防「放大式」内存耗尽）：
//   - maxThumbWidth  ：客户端可指定的宽度上限，也是缩略图的目标宽度；
//   - maxThumbHeight ：输出高度上限，超过则不做缩略、直接返回原文件。
//
// 配合 handleThumb 里「只缩小、不放大」的规则，输出像素数一定不超过源图像素数
// （源图已由 maxPixels 限制在 4000 万像素内），内存因此有界。
const (
	maxThumbWidth  = 800
	maxThumbHeight = 8000
)

// handleThumb GET /api/thumb?dir=xxx&file=xxx[&w=240]
// 列表缩略图：仅对 jpeg/png/gif 做服务端缩放（标准库），其它格式原样返回原文件。
// 用 Last-Modified + 304 缓存，文件不变时重复访问秒开、零流量。
func (a *App) handleThumb(w http.ResponseWriter, r *http.Request) {
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
	// 目录没有缩略图：显式拒绝，避免回退原文件时读目录返回 0 字节。
	if info.IsDir() {
		http.Error(w, "不支持目录", http.StatusBadRequest)
		return
	}

	// 304：客户端带 If-Modified-Since 且文件未变 -> 直接 304，不重传
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		if t, e := http.ParseTime(ims); e == nil && !info.ModTime().After(t) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	width := 240
	if q := r.URL.Query().Get("w"); q != "" {
		// 客户端可指定宽度，但必须封顶：该值直接决定缩略图要分配多少内存。
		if n, e := strconv.Atoi(q); e == nil && n > 0 && n <= maxThumbWidth {
			width = n
		}
	}
	ext := strings.ToLower(filepath.Ext(full))
	const maxPixels = 40_000_000 // 4000 万像素，约 160MB RGBA：超大图回退原文件，避免全图解码 OOM
	// icns 专用像素预算：远小于通用图片。icns 分支要先整幅解码、再做两遍逐像素
	// 扫描（透明边 / 纯色背景）、再整幅 PNG 重编码，最后还要再解码一次——
	// 4000 万像素下单请求峰值可达数百 MB。缩略图目标宽度仅 ≤800，400 万像素足够。
	const icnsMaxPixels = 4_000_000
	// 扫描 GIF 帧数时最多读取的字节数（防止为数帧而把超大 GIF 整个读进内存）。
	const gifScanMaxBytes = 32 << 20

	// 不支持的类型（webp/heic/bmp 等）或解码失败：回退为原文件流式返回。
	// 输出头必须走 setFileOutputHeaders 统一判定（图片内联、其余一律附件下载）：
	// 这里曾自行用 mime.TypeByExtension 决定 Content-Type，把上传的 .html / .svg
	// 当页面内联返回，形成存储型 XSS（脚本在应用 / 门户同源下执行）。
	serveOriginal := func() {
		setFileOutputHeaders(w, file, outputInlineImage)
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
		// 短缓存 + 每次重验证：文件内容变了（mtime 变）立刻反映，未变则 304 零流量。
		// 不能再用长 max-age 强缓存——服务端代码升级后浏览器会一直持旧缩略图。
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
		f.Seek(0, io.SeekStart)
		http.ServeContent(w, r, file, info.ModTime(), f)
	}
	// icns（Apple 图标容器）：浏览器无法直接显示，解析提取「够用且最小」的可解码
	// 图像块（内嵌 PNG/JPEG）交给下方通用流程转 PNG；解析失败回退原文件（下载路径不受影响）。
	var icnsPNG []byte // 非空表示本次为 icns 且已成功提取图像块
	decodeSource := io.ReadSeeker(f)
	decodeable := isDecodeableImage(ext)
	if ext == ".icns" {
		raw, rerr := io.ReadAll(io.LimitReader(f, icnsMaxSize))
		if rerr == nil {
			if imgBytes, ierr := extractIcnsImage(raw, width); ierr == nil {
				// 裁剪图标四周透明边距，让内容占满缩略图（真实 macOS 图标画布常带安全边距）
				// 用 icns 专用像素预算：该流程的解码/扫描/重编码放大倍数远高于普通图片。
				if trimmed := tryTrimIcnsImage(imgBytes, icnsMaxPixels); trimmed != nil {
					icnsPNG = trimmed
				} else {
					icnsPNG = imgBytes
				}
				decodeSource = bytes.NewReader(icnsPNG)
				decodeable = true
			}
		}
	}
	// GIF 解压炸弹防护：标准库解码 GIF 会把【全部帧】都解出来（每帧按画布大小分配
	// 内存），而缩略图只需要第一帧——多帧 GIF 因此能把几 MB 的文件放大成数 GB。
	// 这里只扫描块结构，把「文件头 + 第一帧 + 结束符」截成单帧 GIF 再交给标准库，
	// 于是只解一帧：内存恒等于单帧面积，下面的 maxPixels 检查也就重新有效了。
	if ext == ".gif" && decodeable {
		f.Seek(0, io.SeekStart)
		raw, rerr := io.ReadAll(io.LimitReader(f, gifScanMaxBytes))
		f.Seek(0, io.SeekStart)
		one, ok := gifFirstFrame(raw)
		if rerr != nil || !ok {
			// 读不出或结构异常：不要转而解码原文件（那会解全部帧），直接回退。
			serveOriginal()
			return
		}
		decodeSource = bytes.NewReader(one)
	}
	if !decodeable {
		serveOriginal()
		return
	}
	// 先解码头部获取尺寸
	cfg, _, cerr := image.DecodeConfig(decodeSource)
	if cerr != nil {
		serveOriginal()
		return
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		serveOriginal()
		return
	}
	decodeSource.Seek(0, io.SeekStart)
	img, format, derr := image.Decode(decodeSource)
	if derr != nil {
		serveOriginal()
		return
	}

	sw, sh := img.Bounds().Dx(), img.Bounds().Dy()
	if sw == 0 || sh == 0 {
		serveOriginal()
		return
	}
	// 只缩小、不放大：源图宽度已经不超过目标宽度时直接返回原文件。
	// 否则 1×100 这种极端长宽比的图会被放大成 240×24000（57600 倍像素），
	// 一条免登录请求就能分配几百 MB ~ 几十 GB 内存，把进程撑死（DoS）。
	// icns 例外：其原文件浏览器无法显示，必须输出提取的 PNG（即使不缩放）。
	if sw <= width {
		if icnsPNG != nil {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
			http.ServeContent(w, r, file, info.ModTime(), bytes.NewReader(icnsPNG))
			return
		}
		serveOriginal()
		return
	}
	dw := width
	dh := dw * sh / sw
	if dh <= 0 {
		dh = 1
	}
	// 高度兜底：极端长宽比（如 800×40000）缩小后依然很高，直接返回原文件，
	// 避免大块内存分配。此时输出像素仍受源图像素数约束。
	if dh > maxThumbHeight {
		serveOriginal()
		return
	}
	thumb := resizeBox(img, dw, dh)

	var buf bytes.Buffer
	switch format {
	case "png", "gif":
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(&buf, thumb)
	default: // jpeg
		w.Header().Set("Content-Type", "image/jpeg")
		_ = jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 82})
	}
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	// 用 ServeContent 输出：自动处理 If-Modified-Since(304) 与 Range，文件不变时零流量重传
	http.ServeContent(w, r, file, info.ModTime(), bytes.NewReader(buf.Bytes()))
}

// gifFirstFrame 截取 GIF 的第一帧，返回【只含这一帧】的 GIF 字节流，全程不解码像素。
//
// 为什么需要它：标准库解码 GIF 时会把全部帧都解出来（每帧按画布大小分配内存），
// 缩略图却只要第一帧——多帧 GIF 因此能把几 MB 的文件放大成数 GB（解压炸弹）。
// 这里只做块结构扫描，把「文件头 + 第一帧 + 结束符」拼成单帧 GIF 交给标准库，
// 解码就只会发生一次、只占一帧内存。无需任何第三方库。
//
// 保留第一帧之前最近的 Graphic Control Extension（0xF9）：透明色索引与帧延迟在
// 里面，丢掉它第一帧的透明会变成黑/白底。
// 返回 ok=false 表示数据不符合 GIF 结构或没有图像帧——调用方应回退，
// 不要转而解码原文件（那正是要避免的全帧解码）。
func gifFirstFrame(data []byte) ([]byte, bool) {
	// Header(6) + Logical Screen Descriptor(7) 之后才是数据块
	if len(data) < 13 || string(data[:3]) != "GIF" {
		return nil, false
	}
	packed := data[10] // Logical Screen Descriptor 的 packed 字段
	pos := 13
	if packed&0x80 != 0 { // 有全局颜色表：3 字节/色，色数 = 2^(size+1)
		pos += 3 * (1 << ((packed & 0x07) + 1))
	}
	if pos > len(data) {
		return nil, false
	}
	headEnd := pos
	var gceStart, gceEnd int // 最近一个图形控制扩展（透明色/延迟）
	frameStart, frameEnd := -1, -1
	for frameEnd < 0 && pos < len(data) {
		switch data[pos] {
		case 0x21: // Extension
			if pos+1 >= len(data) {
				return nil, false
			}
			label := data[pos+1]
			start := pos
			next, ok := skipGIFSubBlocks(data, pos+2)
			if !ok {
				return nil, false
			}
			if label == 0xF9 { // Graphic Control Extension：需随第一帧一起保留
				gceStart, gceEnd = start, next
			}
			pos = next
		case 0x2C: // Image Descriptor：第一帧
			frameStart = pos
			if pos+10 > len(data) {
				return nil, false
			}
			ip := data[pos+9] // 该帧 packed：局部颜色表标志 + 大小
			pos += 10
			if ip&0x80 != 0 {
				pos += 3 * (1 << ((ip & 0x07) + 1))
			}
			pos++ // LZW minimum code size
			next, ok := skipGIFSubBlocks(data, pos)
			if !ok {
				return nil, false
			}
			frameEnd = next
		case 0x3B: // Trailer：没有任何图像帧
			return nil, false
		default: // 非预期字节：结构异常
			return nil, false
		}
	}
	if frameStart < 0 || frameEnd <= frameStart {
		return nil, false
	}
	out := make([]byte, 0, headEnd+(gceEnd-gceStart)+(frameEnd-frameStart)+1)
	out = append(out, data[:headEnd]...)
	if gceEnd > gceStart {
		out = append(out, data[gceStart:gceEnd]...)
	}
	out = append(out, data[frameStart:frameEnd]...)
	out = append(out, 0x3B) // Trailer：告诉解码器文件到此结束
	return out, true
}

// skipGIFSubBlocks 跳过 GIF 的子块序列（长度字节 + 数据，直到长度为 0 的终止块）。
func skipGIFSubBlocks(data []byte, pos int) (int, bool) {
	for pos < len(data) {
		n := int(data[pos])
		pos++
		if n == 0 {
			return pos, true
		}
		pos += n
		if pos > len(data) {
			return pos, false
		}
	}
	return pos, false
}
