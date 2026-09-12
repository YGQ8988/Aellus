package app

import (
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// === 文件内容输出头：全项目唯一判定出口 ===
//
// 上传进来的文件一律视为不可信。凡是将文件内容回给浏览器的接口（下载 / 缩略图），
// 都必须经过这里的判定，不允许各自再写一套 Content-Type 逻辑。
//
// 历史教训：下载接口做了内联白名单（isInlineSafe），而缩略图的回退分支另写了一段
// mime.TypeByExtension，把上传的 .html / .svg 当页面内联返回，形成存储型 XSS。
// 收敛到本文件后，「同类问题可能漏在多处」变成「只可能漏在这一个出口」。
//
// 判定规则：只有白名单内、且解析出的 MIME 不是脚本类的类型才允许浏览器内联渲染；
// 其余（HTML / SVG / XML / 未知类型）一律 application/octet-stream + attachment。

// fileOutputMode 是文件内容的输出模式。
type fileOutputMode int

const (
	// outputDownload 按附件输出（强制下载，最安全）。
	outputDownload fileOutputMode = iota
	// outputInlineMedia 允许内联的媒体类型：图片 / 音视频 / PDF（下载接口 inline=1 预览用）。
	outputInlineMedia
	// outputInlineImage 只允许图片内联（缩略图接口用：前端以 <img> 引用，必须能直接显示）。
	outputInlineImage
)

// setFileOutputHeaders 按 mode 设置文件内容的响应头（Content-Type / Content-Disposition / nosniff）。
func setFileOutputHeaders(w http.ResponseWriter, name string, mode fileOutputMode) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if ctype := inlineContentType(filepath.Ext(name), mode); ctype != "" {
		w.Header().Set("Content-Type", ctype)
		return
	}
	// 非白名单类型：强制附件下载，且不给可执行的 Content-Type（避免浏览器按扩展名渲染）。
	w.Header().Set("Content-Type", "application/octet-stream")
	// filename*=UTF-8'' 是 RFC 5987 标准，保证中文文件名不乱码；
	// legacy 的 filename="..." 需去掉引号与换行，防响应头内容注入。
	// 用 PathEscape 而不是 QueryEscape：后者会把空格编码成 "+"，在 RFC 5987 里
	// 会被浏览器当成字面加号（文件名多一个 +），PathEscape 才是 %20。
	safe := strings.NewReplacer(`"`, "_", "\r", "", "\n", "").Replace(name)
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+safe+`"; filename*=UTF-8''`+url.PathEscape(name))
}

// inlineContentType 返回允许内联使用的 Content-Type；不允许内联时返回空串。
func inlineContentType(ext string, mode fileOutputMode) string {
	switch mode {
	case outputInlineImage:
		// 缩略图：仅图片，且显式排除 svg（可携带脚本）。
		if !isInlineImageExt(ext) {
			return ""
		}
	case outputInlineMedia:
		// 下载预览：图片 / 音视频 / PDF（isInlineSafe 已排除 HTML/SVG/XML）。
		if !isInlineSafe(ext) {
			return ""
		}
	default:
		return ""
	}
	ctype := mime.TypeByExtension(ext)
	if ctype == "" {
		return ""
	}
	// 纵深防御：即使扩展名在白名单里，只要解析出的 MIME 是脚本类就绝不内联。
	if isScriptableMIME(ctype) {
		return ""
	}
	// 缩略图模式必须真的是图片类型（防止 mime 表把某扩展名映射成别的类型）。
	if mode == outputInlineImage && !strings.HasPrefix(strings.ToLower(ctype), "image/") {
		return ""
	}
	return ctype
}

// isInlineImageExt 判断是否为可安全内联的图片扩展名。
// 显式排除 .svg / .svgz（SVG 可携带脚本，内联即 XSS）。
// 说明：webp / bmp / ico / heic 等标准库不解码，走缩略图回退分支原样返回，
// 它们仍属图片类型，必须允许内联，否则列表缩略图无法显示。
func isInlineImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".jfif", ".png", ".gif", ".webp", ".bmp", ".ico",
		".heic", ".heif", ".avif", ".tif", ".tiff":
		return true
	}
	return false
}

// isScriptableMIME 判断 MIME 是否为「可在浏览器里执行脚本 / 渲染标记」的类型。
// 用于纵深防御：任何一处白名单判断疏漏，只要 MIME 落到这里就不会被内联。
func isScriptableMIME(ctype string) bool {
	c := strings.ToLower(strings.TrimSpace(strings.Split(ctype, ";")[0]))
	switch c {
	case "text/html", "application/xhtml+xml", "text/xml", "application/xml",
		"image/svg+xml", "text/xsl", "application/xslt+xml", "application/xml-dtd",
		"application/javascript", "text/javascript", "application/ecmascript",
		"text/vtt", "application/x-shockwave-flash", "text/cache-manifest":
		return true
	}
	// 形如 application/xxx+xml 的类型同样可携带脚本（XHTML 等）。
	return strings.HasSuffix(c, "+xml")
}
