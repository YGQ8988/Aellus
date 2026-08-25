//go:build windows && !fpk

package platform

// 系统托盘实现：纯 Go 标准库 syscall 直接调用 Windows API，不引入任何第三方包。
// 功能：任务栏托盘图标 + 右键菜单（打开浏览器 / 退出）+ 双击图标打开浏览器。

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

//go:embed favicon.ico
var faviconICO []byte

// ---- Windows API（全部来自标准库 syscall，加载系统 DLL）----
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenuEx    = user32.NewProc("TrackPopupMenuEx")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procCreateIconFromRes   = user32.NewProc("CreateIconFromResourceEx")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procGetModuleFileName   = kernel32.NewProc("GetModuleFileNameW")
	shellNotifyIcon         = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconEx       = shell32.NewProc("ExtractIconExW")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procGetLastError        = kernel32.NewProc("GetLastError")
)

const (
	WM_TRAYICON       = 0x8000 + 1 // 自定义：托盘图标回调消息
	WM_AELLUS_ALREADY = 0x8000 + 2 // 自定义：另一实例被重复启动（single_windows.go 发来）
	WM_RBUTTONUP      = 0x0205
	WM_LBUTTONDBLCLK  = 0x0203
	WM_DESTROY        = 0x0002
	WM_COMMAND        = 0x0111

	NIM_ADD     = 0x00000000
	NIM_DELETE  = 0x00000002
	NIF_MESSAGE = 0x1
	NIF_ICON    = 0x2
	NIF_TIP     = 0x4

	MF_STRING       = 0x0000
	TPM_RIGHTBUTTON = 0x0002

	IDI_APPLICATION = 0x7F00 // 系统默认应用图标（兜底用）

	CW_USEDEFAULT = 0x80000000
	HWND_MESSAGE  = ^uintptr(2) // 消息专用窗口（不可见，仅用来收消息）
)

const (
	trayIconID = 1
	menuOpenID = 1001
	menuQuitID = 1002
)

var (
	gURL     string
	gIcoData []byte
)

// ---- Win32 结构体（字段顺序/对齐需与 Windows 一致）----
type WNDCLASSEX struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type MSG struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      POINT
}

type POINT struct {
	x int32
	y int32
}

// NOTIFYICONDATAW（Vista+ 布局，含 guidItem / hBalloonIcon）
type NOTIFYICONDATA struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func copyTip(dst *[128]uint16, s string) {
	u := utf16.Encode([]rune(s))
	for i := 0; i < len(u) && i < 127; i++ {
		dst[i] = u[i]
	}
}

// trayDebug 写一行诊断信息到 aellus_tray.log（位于工作目录）。
// 双击 exe 是窗口程序，没有控制台，用它来排查“托盘图标出不来”之类的问题。
func trayDebug(format string, a ...interface{}) {
	f, err := os.OpenFile("aellus_tray.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format+"\n", a...)
}

// loadIconFromICO 解析嵌入的 .ico 字节，取最大那张图，创建 HICON（兜底方案）。
func loadIconFromICO(data []byte) uintptr {
	if len(data) < 6 {
		return 0
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 {
		return 0
	}
	bestOff := -1
	bestSize := uint32(0)
	p := 6
	for i := 0; i < count; i++ {
		if p+16 > len(data) {
			break
		}
		bytesInRes := binary.LittleEndian.Uint32(data[p+8 : p+12])
		imageOffset := binary.LittleEndian.Uint32(data[p+12 : p+16])
		if bytesInRes > bestSize {
			bestSize = bytesInRes
			bestOff = int(imageOffset)
		}
		p += 16
	}
	if bestOff < 0 || bestOff+int(bestSize) > len(data) {
		return 0
	}
	res := data[bestOff : bestOff+int(bestSize)]
	r, _, _ := procCreateIconFromRes.Call(
		uintptr(unsafe.Pointer(&res[0])),
		uintptr(bestSize),
		1, // fIcon = TRUE（不是光标）
		0x00030000,
		0, 0,
		0, // LR_DEFAULTCOLOR
	)
	return r
}

// loadTrayIcon 返回托盘要用的 HICON，按可靠性从高到低尝试：
//  1. ExtractIconExW 从 exe 自身资源取图标（与资源管理器里显示的 exe 图标同源，最可靠）
//  2. 解析嵌入的 static/favicon.ico
//  3. 系统默认应用图标兜底，保证托盘至少有图标出现
func loadTrayIcon() uintptr {
	// —— 方案 1：从 exe 自身资源取图标 ——
	var pathBuf [32768]uint16
	procGetModuleFileName.Call(0, uintptr(unsafe.Pointer(&pathBuf[0])), uintptr(len(pathBuf)))
	var hLarge uintptr
	ret, _, _ := procExtractIconEx.Call(
		uintptr(unsafe.Pointer(&pathBuf[0])),
		0,                                // 第 0 个图标组
		uintptr(unsafe.Pointer(&hLarge)), // 大图标（托盘用这个）
		0,                                // 不需要小图标
		1,                                // 取 1 个
	)
	if int(ret) > 0 && hLarge != 0 {
		trayDebug("loadTrayIcon: 方案1 ExtractIconEx 成功 hIcon=%d", hLarge)
		return hLarge
	}
	trayDebug("loadTrayIcon: 方案1 ExtractIconEx 失败 ret=%d hLarge=%d", int(ret), hLarge)

	// —— 方案 2：解析嵌入的 favicon.ico ——
	if len(gIcoData) > 6 {
		if h := loadIconFromICO(gIcoData); h != 0 {
			trayDebug("loadTrayIcon: 方案2 ICO解析 成功 hIcon=%d", h)
			return h
		}
		trayDebug("loadTrayIcon: 方案2 ICO解析 失败")
	}

	// —— 方案 3：兜底默认图标 ——
	h, _, _ := procLoadIcon.Call(0, uintptr(IDI_APPLICATION))
	trayDebug("loadTrayIcon: 方案3 默认图标 hIcon=%d", h)
	return h
}

func addTrayIcon(hwnd uintptr) {
	hIcon := loadTrayIcon()
	if hIcon == 0 {
		trayDebug("addTrayIcon: hIcon 为 0，放弃添加托盘图标")
		return
	}
	var nid NOTIFYICONDATA
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = trayIconID
	nid.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.uCallbackMessage = WM_TRAYICON
	nid.hIcon = hIcon
	copyTip(&nid.szTip, "Aellus 已启动")
	r, _, err := shellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	var le uintptr
	if ee, ok := err.(syscall.Errno); ok {
		le = uintptr(ee)
	}
	trayDebug("addTrayIcon: hIcon=%d NIM_ADD ret=%d lastErr=%d", hIcon, int(r), le)
	// 供通知模块复用（气球通知需要同一个 hWnd/uID/hIcon）
	trayNotifyHWND = hwnd
	trayNotifyIcon = hIcon
}

func removeTrayIcon(hwnd uintptr) {
	var nid NOTIFYICONDATA
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = trayIconID
	shellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
}

func showMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	procAppendMenu.Call(hMenu, MF_STRING, menuOpenID, uintptr(unsafe.Pointer(utf16Ptr("打开浏览器"))))
	procAppendMenu.Call(hMenu, MF_STRING, menuQuitID, uintptr(unsafe.Pointer(utf16Ptr("退出"))))
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(hwnd)
	// 在鼠标位置弹出菜单，等待用户选择（选择后触发 WM_COMMAND）
	procTrackPopupMenuEx.Call(hMenu, TPM_RIGHTBUTTON, uintptr(pt.x), uintptr(pt.y), hwnd, 0)
	procDestroyMenu.Call(hMenu)
}

// wndProc 是窗口过程，处理托盘图标的各类消息。
// 注意：NewCallback 要求所有参数与返回值都是 uintptr，不能用 uint32，否则 amd64 上
// wparam/lparam 会被读错，导致菜单点击失效。
func wndProc(hwnd uintptr, msg uintptr, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_DESTROY:
		removeTrayIcon(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	case WM_TRAYICON:
		evt := uint32(lparam & 0xffff)
		switch evt {
		case WM_RBUTTONUP:
			showMenu(hwnd) // 右键 -> 弹出菜单
		case WM_LBUTTONDBLCLK:
			openBrowser(gURL) // 双击 -> 打开浏览器
		case NIN_BALLOONUSERCLICK:
			onBalloonClick() // 点击系统通知 -> 打开浏览器
		}
		return 0
	case WM_AELLUS_ALREADY:
		// 另一个实例被重复启动：由本实例弹「已在运行」通知（点击可打开浏览器）
		trayDebug("收到重复启动消息，弹出已在运行通知")
		postOpenNotification("Aellus 已在运行", "点击打开文件传输页", gURL)
		return 0
	case WM_COMMAND:
		id := uint32(wparam & 0xffff)
		switch id {
		case menuQuitID:
			removeTrayIcon(hwnd)
			procDestroyWindow.Call(hwnd) // 触发 WM_DESTROY -> 退出消息循环
		case menuOpenID:
			openBrowser(gURL)
		}
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

// runTray 注册并创建一个"消息专用窗口"，挂上托盘图标，然后进入消息循环阻塞。
// 用户点"退出"时窗口被销毁 -> WM_DESTROY -> PostQuitMessage -> 循环结束 -> 进程退出。
func runTray(url string) {
	gURL = url
	gIcoData = faviconICO
	trayDebug("runTray: gIcoLen=%d", len(gIcoData))

	hInstance, _, _ := procGetModuleHandle.Call(0)
	className := utf16Ptr("AellusTrayClass")
	wndClass := WNDCLASSEX{
		cbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hInstance,
		lpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wndClass)))
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		0,
		CW_USEDEFAULT, CW_USEDEFAULT, CW_USEDEFAULT, CW_USEDEFAULT,
		HWND_MESSAGE,
		0,
		hInstance,
		0,
	)
	trayDebug("runTray: hInstance=%d hwnd=%d", hInstance, hwnd)
	if hwnd == 0 {
		select {} // 极端情况：窗口创建失败则永久阻塞，保持进程存活
	}
	// 注册并添加托盘图标
	addTrayIcon(hwnd)
	// 托盘就绪后补发启动通知（postOpenNotification 可能已在 runTray 之前被调用）
	flushPendingBalloon()

	var msg MSG
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int(ret) == 0 { // 收到 WM_QUIT
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
