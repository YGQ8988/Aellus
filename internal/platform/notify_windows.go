//go:build windows && !fpk

package platform

// Windows 系统通知：复用托盘图标，用 Shell_NotifyIcon 的气球提示（NIF_INFO）实现。
// 与 macOS 行为对齐：启动时弹一条「Aellus 已就绪」，点击通知用默认浏览器打开传输页。
// 纯标准库 syscall，不引入第三方包（与 tray_windows.go 风格一致）。

import (
	"unicode/utf16"
	"unsafe"
)

// 通知内容（启动期由 postOpenNotification 写入，runTray 托盘就绪后补发）
type balloonInfo struct {
	title string
	body  string
	url   string
}

var (
	balloonPending *balloonInfo
	// 托盘就绪后由 tray_windows.go 写入（addTrayIcon 成功时）
	trayNotifyHWND uintptr
	trayNotifyIcon uintptr
)

const (
	NIF_INFO             = 0x10
	NIM_MODIFY           = 0x1
	NIIF_INFO            = 0x1
	NIN_BALLOONUSERCLICK = 0x0405 // WM_USER + 5：气球通知被点击
)

// postOpenNotification 保存通知并尝试立即弹出；托盘未就绪则等 runTray 初始化后补发。
func postOpenNotification(title, body, url string) {
	balloonPending = &balloonInfo{title: title, body: body, url: url}
	if trayNotifyHWND != 0 {
		showTrayBalloon()
	}
}

// flushPendingBalloon 由 runTray 在托盘图标创建成功后调用，补发启动通知。
func flushPendingBalloon() {
	if balloonPending != nil && trayNotifyHWND != 0 {
		showTrayBalloon()
	}
}

// showTrayBalloon 通过 NIM_MODIFY + NIF_INFO 在托盘图标上弹一条系统通知。
func showTrayBalloon() {
	if balloonPending == nil || trayNotifyHWND == 0 || trayNotifyIcon == 0 {
		return
	}
	var nid NOTIFYICONDATA
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = trayNotifyHWND
	nid.uID = trayIconID
	nid.uFlags = NIF_INFO | NIF_ICON | NIF_MESSAGE
	nid.uCallbackMessage = WM_TRAYICON
	nid.hIcon = trayNotifyIcon
	nid.dwInfoFlags = NIIF_INFO
	copyInfo(&nid.szInfo, balloonPending.body)
	copyInfoTitle(&nid.szInfoTitle, balloonPending.title)
	shellNotifyIcon.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

// onBalloonClick 由 tray_windows.go 在收到 NIN_BALLOONUSERCLICK 时调用：打开浏览器。
func onBalloonClick() {
	if balloonPending != nil && balloonPending.url != "" {
		openBrowser(balloonPending.url)
	}
}

func copyInfo(dst *[256]uint16, s string) {
	u := utf16.Encode([]rune(s))
	for i := 0; i < len(u) && i < len(dst)-1; i++ {
		dst[i] = u[i]
	}
}

func copyInfoTitle(dst *[64]uint16, s string) {
	u := utf16.Encode([]rune(s))
	for i := 0; i < len(u) && i < len(dst)-1; i++ {
		dst[i] = u[i]
	}
}
