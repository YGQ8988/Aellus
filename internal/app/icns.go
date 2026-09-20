package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // icns 部分版本内嵌 JPEG
	"image/png"    // 注册 PNG 解码器 + 裁剪后重编码
)

// === icns（Apple 图标容器）预览支持 ===
//
// icns 不是浏览器可显示的图像格式，而是一个容器：内部按 4 字节类型 + 4 字节长度
// （大端）排列若干图像块，现代格式内嵌 PNG（icp4~icpA / ic07~ic14），
// 老格式（ic04/ic05 等）为 JPEG 2000 / ARGB 原始像素，标准库无法解码。
// 预览策略：解析容器，用 image.DecodeConfig 逐个尝试解码图像块，
// 选出像素面积最大且可解码的一块原始字节返回（交给 handleThumb 的
// 通用解码/缩放流程输出 PNG）。

// icnsMaxSize：单个 icns 文件读取上限（含 1024px 图标的常见文件约 1~2MB，
// 20MB 上限足够容纳极端场景，同时防止畸形文件把内存拉满）。
const icnsMaxSize = 20 << 20

// minVisibleAlpha：视为"可见内容"的最低 alpha（RGBA() 16bit 值，255 → 65535）。
// 图标画布边缘常见抗锯齿残影（alpha≈1/255），若按 a>0 判定会被当成内容，
// 导致透明边距裁剪失效；取 4/255 之上才算可见内容。
const minVisibleAlpha = 1024

// icnsImageChunks：现代 icns 内嵌 PNG/JPEG 的图像块类型。
// icp4~icpA = 16/32/64/128/256/512/1024 点单倍图；ic07~ic10 = 128/256/512/1024；
// ic11~ic14 = 32/64/256/512 点 @2x 图。ic04/ic05 等（JP2/ARGB）不可解码，不列。
func isIcnsImageChunk(typ string) bool {
	switch typ {
	case "icp4", "icp5", "icp6", "icp7", "icp8", "icp9", "icpA",
		"ic07", "ic08", "ic09", "ic10",
		"ic11", "ic12", "ic13", "ic14":
		return true
	}
	return false
}

// extractIcnsImage 解析 icns 容器，返回最大的、可解码的图像块原始字节。
// 解析失败 / 容器内没有可解码块时返回错误（调用方回退为原文件返回）。
func extractIcnsImage(data []byte) ([]byte, error) {
	if len(data) < 8 || string(data[:4]) != "icns" {
		return nil, fmt.Errorf("不是 icns 容器")
	}
	var best []byte
	bestArea := 0
	off := 8 // 跳过 magic + 总长度
	for off+8 <= len(data) {
		size := int(binary.BigEndian.Uint32(data[off+4 : off+8]))
		if size < 8 {
			return nil, fmt.Errorf("图像块长度非法")
		}
		if off+size > len(data) {
			break // 文件被截断：忽略剩余块，用已找到的最大块
		}
		typ := string(data[off : off+4])
		body := data[off+8 : off+size]
		if isIcnsImageChunk(typ) {
			if cfg, _, err := image.DecodeConfig(bytes.NewReader(body)); err == nil &&
				cfg.Width > 0 && cfg.Height > 0 {
				if area := cfg.Width * cfg.Height; area > bestArea {
					bestArea = area
					best = append(best[:0], body...) // 复用底层数组，避免大块反复分配
				}
			}
		}
		off += size
	}
	if best == nil {
		return nil, fmt.Errorf("没有可解码的图像块")
	}
	return best, nil
}

// trimTransparent 裁剪图片四周的透明边距，返回新图；全透明或无需裁剪时返回 nil。
// icns 图标画布常带设计安全边距（内容只占画布一部分，四周透明/近透明），
// 直接显示时缩略图里图标"撑不满"，先裁剪掉边距让内容占满。
// alpha 阈值用于忽略边缘抗锯齿残影（alpha≈1/255 的像素仍应视为透明边）。
func trimTransparent(img image.Image) *image.RGBA {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > minVisibleAlpha {
				found = true
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if !found {
		return nil
	}
	if minX == b.Min.X && minY == b.Min.Y && maxX == b.Max.X-1 && maxY == b.Max.Y-1 {
		return nil // 内容已占满画布，无需裁剪
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxX-minX+1, maxY-minY+1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dst.Set(x-minX, y-minY, img.At(x, y))
		}
	}
	return dst
}

// trimUniformBackground 裁剪纯色背景边距：图标内容被四周统一背景色（白底/灰底等）
// 包裹、但内容未触及画布边缘时，把"颜色明显不同于背景色"的像素边界作为内容边界裁剪，
// 让图标主体尽量撑满。四边采样颜色不一致（全幅图 / 多色边缘）或边缘透明时返回 nil。
// 用在透明边裁剪之后：圆角图标（四角透明但内容触及四边）透明裁剪无效，
// 若其画布是纯色背景（如白底图标），这里能把内容放大到占满。
func trimUniformBackground(img image.Image) *image.RGBA {
	b := img.Bounds()
	if b.Dx() < 24 || b.Dy() < 24 {
		return nil
	}
	// 四边中点采样（避开四角透明圆角区域；取 y=2/3 深一点避免抗锯齿边缘）
	cx, cy := b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2
	samples := []image.Point{
		{X: cx, Y: b.Min.Y + 2},
		{X: cx, Y: b.Max.Y - 3},
		{X: b.Min.X + 2, Y: cy},
		{X: b.Max.X - 3, Y: cy},
	}
	colors := make([]color.Color, 0, 4)
	for _, p := range samples {
		_, _, _, a := img.At(p.X, p.Y).RGBA()
		if a < minVisibleAlpha {
			return nil // 边缘透明/近透明：交给透明边裁剪
		}
		colors = append(colors, img.At(p.X, p.Y))
	}
	// 背景色取第一个采样色；用 16bit 通道差的绝对值之和判断接近程度
	br, bg, bb, _ := colors[0].RGBA()
	closeTo := func(c color.Color) bool {
		r, g, bl, _ := c.RGBA()
		d := absInt(int(r)-int(br)) + absInt(int(g)-int(bg)) + absInt(int(bl)-int(bb))
		return d < 96 // 每通道 16bit；约 RGB8 每通道 ±12 以内视为同一背景色
	}
	for _, c := range colors[1:] {
		if !closeTo(c) {
			return nil // 四边颜色不一致 → 非纯色背景（全幅图），不裁剪
		}
	}
	// 找内容边界：颜色与背景色差异超过容差（内容内部与背景同色的孔洞不影响边界）
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if !closeTo(img.At(x, y)) {
				found = true
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if !found || (minX == b.Min.X && minY == b.Min.Y && maxX == b.Max.X-1 && maxY == b.Max.Y-1) {
		return nil // 无内容 / 内容已触及边缘
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxX-minX+1, maxY-minY+1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dst.Set(x-minX, y-minY, img.At(x, y))
		}
	}
	return dst
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// tryTrimIcnsImage 尝试裁剪 icns 提取块的边距（透明边 → 纯色背景边）并重新编码为 PNG。
// 解码失败 / 尺寸超限 / 无需裁剪时返回 nil，调用方应保持原块（后续流程
// 会基于原块尺寸走通用的 maxPixels 检查与缩放）。
func tryTrimIcnsImage(imgBytes []byte, maxPixels int) []byte {
	if maxPixels <= 0 {
		maxPixels = 40_000_000
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil
	}
	// 第一层：透明边裁剪（内容四周透明留白）
	if trimmed := trimTransparent(img); trimmed != nil {
		img = trimmed
	}
	// 第二层：纯色背景裁剪（白底/灰底图标，透明裁剪无效的场景）
	if trimmed := trimUniformBackground(img); trimmed != nil {
		img = trimmed
	}
	var buf bytes.Buffer
	if png.Encode(&buf, img) != nil {
		return nil
	}
	return buf.Bytes()
}
