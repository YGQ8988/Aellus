package app

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
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
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".ico",
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
		if n, e := strconv.Atoi(q); e == nil && n > 0 && n <= 2000 {
			width = n
		}
	}
	ext := strings.ToLower(filepath.Ext(full))

	// 不支持的类型（webp/heic/bmp 等）或解码失败：回退为原文件流式返回
	serveOriginal := func() {
		ctype := mime.TypeByExtension(ext)
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
		w.Header().Set("Cache-Control", "public, max-age=300")
		f.Seek(0, io.SeekStart)
		http.ServeContent(w, r, file, info.ModTime(), f)
	}
	if !isDecodeableImage(ext) {
		serveOriginal()
		return
	}
	// 先解码头部获取尺寸，超大图片直接回退原文件，避免全图解码 OOM
	cfg, _, cerr := image.DecodeConfig(f)
	if cerr != nil {
		serveOriginal()
		return
	}
	const maxPixels = 40_000_000 // 4000 万像素，约 160MB RGBA
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		serveOriginal()
		return
	}
	f.Seek(0, io.SeekStart)
	img, format, derr := image.Decode(f)
	if derr != nil {
		serveOriginal()
		return
	}

	sw, sh := img.Bounds().Dx(), img.Bounds().Dy()
	if sw == 0 || sh == 0 {
		serveOriginal()
		return
	}
	dw := width
	dh := dw * sh / sw
	if dh <= 0 {
		dh = 1
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
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// 用 ServeContent 输出：自动处理 If-Modified-Since(304) 与 Range，文件不变时零流量重传
	http.ServeContent(w, r, file, info.ModTime(), bytes.NewReader(buf.Bytes()))
}
