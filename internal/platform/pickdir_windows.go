//go:build windows && !fpk

package platform

// Windows 原生“选择文件夹”对话框：直接调 shell32 的 SHBrowseForFolderW。
// 不用 powershell 等外部进程——那会闪控制台窗口，且"exe 拉起 powershell"
// 易被火绒等安全软件拦截。进程内弹出系统原生目录选择框，零外部进程、零闪窗。
//
// 前台问题：本进程是托盘后台程序，直接弹框会被 Windows 前台激活锁压到后台。
// 处理（三层保障）：
//  1. keybd_event 模拟 Alt 按下/抬起，解锁前台锁；
//  2. AttachThreadInput 绑定浏览器（前台进程）的线程，再 SetForegroundWindow
//     把本进程的隐藏 owner 窗口置前——系统会认为前台激活是"合法的"；
//  3. 对话框以隐藏窗口为 owner，z 序稳定保持在最前，且按 owner 位置居中。

import (
	"syscall"
	"unsafe"
)

var (
	procSHBrowseForFolder     = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW  = shell32.NewProc("SHGetPathFromIDListW")
	procKeybdEvent            = user32.NewProc("keybd_event")
	procGetForegroundWindow   = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcID = user32.NewProc("GetWindowThreadProcessId")
	procGetCurrentThreadId    = kernel32.NewProc("GetCurrentThreadId")
	procAttachThreadInput     = user32.NewProc("AttachThreadInput")
	procBringWindowToTop      = user32.NewProc("BringWindowToTop")
	procGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
)

const (
	VK_MENU               = 0x12 // Alt 键
	KEYEVENTF_EXTENDEDKEY = 0x0001
	KEYEVENTF_KEYUP       = 0x0002

	SWP_NOSIZE     = 0x0001
	SWP_NOZORDER   = 0x0004
	SWP_NOACTIVATE = 0x0010

	SM_CXSCREEN = 0
	SM_CYSCREEN = 1
)

// BROWSEINFOW（字段顺序/对齐需与 Windows 一致）
type BROWSEINFOW struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16 // 接收选中的显示名（至少 MAX_PATH）
	lpszTitle      *uint16 // 对话框标题
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

const (
	BIF_RETURNONLYFSDIRS = 0x00000001 // 只允许选择文件系统目录（禁止网络/虚拟项）
	maxPathLen           = 260        // MAX_PATH
)

// pickFolderDialogWindows 弹出 Windows 原生“浏览文件夹”对话框，返回选中的
// 绝对路径；用户取消时返回空串。
func pickFolderDialogWindows() string {
	// 建一个隐藏的顶层窗口作为对话框 owner（不可见，仅用于 z 序/居中/模态）
	hwndOwner := createPickOwnerWindow()
	defer func() {
		if hwndOwner != 0 {
			procDestroyWindow.Call(hwndOwner)
		}
	}()

	var displayBuf [maxPathLen]uint16
	bi := BROWSEINFOW{
		hwndOwner:      hwndOwner,
		pszDisplayName: &displayBuf[0],
		lpszTitle:      utf16Ptr("选择文件保存目录"),
		ulFlags:        BIF_RETURNONLYFSDIRS,
	}

	foregroundUnlock()      // 1) 模拟 Alt 解锁前台锁
	bringToFront(hwndOwner) // 2) AttachThreadInput 绑定前台线程并抢前台（关键）
	pidl, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "" // 用户取消
	}
	var pathBuf [maxPathLen]uint16
	ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&pathBuf[0])))
	if int(ok) == 0 {
		return ""
	}
	return syscall.UTF16ToString(pathBuf[:])
}

// foregroundUnlock 模拟一次 Alt 键按下/抬起，解除 Windows 的"前台激活锁"。
// 只产生一次按键事件，不输入任何实际内容。
func foregroundUnlock() {
	procKeybdEvent.Call(VK_MENU, 0, KEYEVENTF_EXTENDEDKEY, 0)
	procKeybdEvent.Call(VK_MENU, 0, KEYEVENTF_EXTENDEDKEY|KEYEVENTF_KEYUP, 0)
}

// bringToFront 把 hwnd 抢到前台：将本线程输入队列临时绑定到前台进程的线程
// （AttachThreadInput），系统即认为本窗口激活是合法的，再 SetForegroundWindow。
func bringToFront(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	fg, _, _ := procGetForegroundWindow.Call()
	if fg == 0 {
		procSetForegroundWindow.Call(hwnd)
		return
	}
	cur, _, _ := procGetCurrentThreadId.Call()
	fgThread, _, _ := procGetWindowThreadProcID.Call(fg, 0)
	procAttachThreadInput.Call(cur, fgThread, 1)
	procSetForegroundWindow.Call(hwnd)
	procBringWindowToTop.Call(hwnd)
	procAttachThreadInput.Call(cur, fgThread, 0)
}

// createPickOwnerWindow 创建一个隐藏的顶层窗口，作为文件夹对话框的 owner。
// 放在屏幕中央，让对话框以它为中心显示。
func createPickOwnerWindow() uintptr {
	hInstance, _, _ := procGetModuleHandle.Call(0)
	className := utf16Ptr("AellusPickClass")
	wndClass := WNDCLASSEX{
		cbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		lpfnWndProc:   syscall.NewCallback(pickWndProc),
		hInstance:     hInstance,
		lpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wndClass)))
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16Ptr("Aellus"))),
		0, // 样式 0（WS_OVERLAPPED，创建后不显示）
		0, 0, 0, 0,
		0, // 独立顶层窗口
		0, hInstance, 0,
	)
	if hwnd != 0 {
		sw, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
		sh, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
		procSetWindowPos.Call(hwnd, 0, sw>>1, sh>>1, 0, 0,
			SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
	}
	return hwnd
}

// pickWndProc 为 owner 窗口的窗口过程：不响应 WM_DESTROY（避免触发托盘
// wndProc 的 PostQuitMessage 干扰主循环）。
func pickWndProc(hwnd uintptr, msg uintptr, wparam, lparam uintptr) uintptr {
	if msg == WM_DESTROY {
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
