//go:build windows && !fpk

package platform

// 单实例：重复打开 exe 时不再启动第二个进程。
// 用命名互斥体（CreateMutexW）判定是否已有实例在运行：
//   - 已有实例 -> 向它的托盘窗口发 WM_AELLUS_ALREADY 消息，由它弹「已在运行」通知
//     （点击可打开浏览器）；找不到窗口时本进程临时建托盘图标自弹一条兜底通知，然后退出。
//   - 本实例是第一个 -> 持有互斥体句柄直到进程结束（进程退出时系统自动释放）。

import (
	"syscall"
	"time"
	"unsafe"
)

const (
	singleInstanceMutexName = `Local\Aellus.SingleInstance`
	ERROR_ALREADY_EXISTS    = 183
)

var (
	procCreateMutex = kernel32.NewProc("CreateMutexW")
	procCloseHandle = kernel32.NewProc("CloseHandle")
	procFindWindow  = user32.NewProc("FindWindowW")
	procSendMessage = user32.NewProc("SendMessageW")
)

// enforceSingleInstance 单实例检查。返回 false 表示已有实例在运行
// （已发出「已在运行」通知），调用方应立即 os.Exit(0)，不要再启动服务。
func enforceSingleInstance() bool {
	h, _, err := procCreateMutex.Call(
		0, // 无安全属性（当前用户可用）
		0, // 不立即请求所有权
		uintptr(unsafe.Pointer(utf16Ptr(singleInstanceMutexName))),
	)
	if h == 0 {
		// 创建失败（权限等罕见情况）：放行，不阻塞启动
		return true
	}
	var le uintptr
	if ee, ok := err.(syscall.Errno); ok {
		le = uintptr(ee)
	}
	if le == ERROR_ALREADY_EXISTS {
		// 已有实例在运行：通知用户后退出
		procCloseHandle.Call(h)
		notifyAlreadyRunning()
		return false
	}
	return true
}

// notifyAlreadyRunning 让用户知道程序已在运行：优先由已有实例弹通知（可点击打开浏览器），
// 找不到它的窗口时自己弹一条兜底气球通知。
func notifyAlreadyRunning() {
	hwnd, _, _ := procFindWindow.Call(
		uintptr(unsafe.Pointer(utf16Ptr("AellusTrayClass"))), 0,
	)
	if hwnd != 0 {
		procSendMessage.Call(hwnd, WM_AELLUS_ALREADY, 0, 0)
		return
	}
	showSelfBalloon("Aellus 已在运行", "程序已在运行，无需重复启动")
}

// showSelfBalloon 兜底：本进程临时建一个消息窗口 + 托盘图标，弹一条气球通知，
// 约 4 秒后自动清理退出（不点击也能看到）。
func showSelfBalloon(title, body string) {
	hInstance, _, _ := procGetModuleHandle.Call(0)
	className := utf16Ptr("AellusNotifyClass")
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
		0, 0,
		CW_USEDEFAULT, CW_USEDEFAULT, CW_USEDEFAULT, CW_USEDEFAULT,
		HWND_MESSAGE,
		0, hInstance, 0,
	)
	if hwnd == 0 {
		return
	}
	hIcon := loadTrayIcon()
	if hIcon == 0 {
		return
	}
	var nid NOTIFYICONDATA
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = trayIconID
	nid.uFlags = NIF_INFO | NIF_ICON | NIF_MESSAGE
	nid.uCallbackMessage = WM_TRAYICON
	nid.hIcon = hIcon
	nid.dwInfoFlags = NIIF_INFO
	copyInfo(&nid.szInfo, body)
	copyInfoTitle(&nid.szInfoTitle, title)
	shellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))

	// 4 秒后退出消息循环（气球默认展示几秒，够用户看到）
	go func() {
		time.Sleep(4 * time.Second)
		procPostQuitMessage.Call(0)
	}()

	var msg MSG
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int(ret) == 0 { // 收到 WM_QUIT
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}

	// 清理：移除临时托盘图标、销毁窗口
	var del NOTIFYICONDATA
	del.cbSize = uint32(unsafe.Sizeof(del))
	del.hWnd = hwnd
	del.uID = trayIconID
	shellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&del)))
	procDestroyWindow.Call(hwnd)
}
