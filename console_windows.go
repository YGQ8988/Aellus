//go:build windows

// Windows 控制台初始化：解决中文显示为方框的问题。
//
// 根因：双击 exe 时 conhost 控制台窗口使用默认字体(Consolas/点阵字体)，
// 这些字体不含中文字形，导致中文显示为方框(□)。Go 本身用 WriteConsoleW
// 输出 Unicode，编码一直正确，问题在字体而非编码。
//
// 方案：调用 SetCurrentConsoleFontEx 把控制台字体切换为支持中文的等宽字体
// 新宋体(NSimSun)。同时设置代码页为 UTF-8 作为兜底。零第三方依赖。
package main

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// consoleFontInfoEx 对应 Windows CONSOLE_FONT_INFOEX 结构体。
type consoleFontInfoEx struct {
	cbSize      uint32   // ULONG
	nFont       uint32   // DWORD
	dwFontSizeX int16    // COORD.X (SHORT)
	dwFontSizeY int16    // COORD.Y (SHORT)
	fontFamily  uint32   // UINT
	fontWeight  uint32   // UINT
	faceName    [32]uint16 // WCHAR[LF_FACESIZE]
}

func initConsoleUTF8() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")

	// 1. 代码页设为 UTF-8（对输入中文目录名有帮助，无害）
	kernel32.NewProc("SetConsoleOutputCP").Call(65001)
	kernel32.NewProc("SetConsoleCP").Call(65001)

	// 2. 切换控制台字体为支持中文的等宽字体，解决中文方框问题。
	//    依次尝试多个候选字体名，直到成功。
	setFont := kernel32.NewProc("SetCurrentConsoleFontEx")
	for _, name := range []string{"新宋体", "NSimSun", "SimSun"} {
		var font consoleFontInfoEx
		font.cbSize = uint32(unsafe.Sizeof(font))
		font.dwFontSizeY = 16 // 字号（像素高），X=0 让系统自动确定宽度
		font.fontWeight = 400 // FW_NORMAL
		face := utf16.Encode([]rune(name))
		if len(face) >= len(font.faceName) {
			face = face[:len(font.faceName)-1]
		}
		copy(font.faceName[:], face)
		r, _, _ := setFont.Call(uintptr(syscall.Stdout), 0, uintptr(unsafe.Pointer(&font)))
		if r != 0 { // 成功
			break
		}
	}
}
