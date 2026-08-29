//go:build darwin && !fpk

package platform

/*
#cgo darwin LDFLAGS: -framework Foundation -framework UserNotifications -framework AppKit
#include <stdlib.h>
// Declaration only — implementation is in notify_darwin.m (compiled as ObjC)
void postNotify(const char* title, const char* body, const char* url);
*/
import "C"
import (
	"os"
	"unsafe"
)

//export aellusOpenBrowser
// 由 notify_darwin.m 的 UNUserNotificationCenter 点击回调调用，打开默认浏览器。
func aellusOpenBrowser(url *C.char) {
	openBrowser(C.GoString(url))
}

//export aellusQuit
// 由 notify_darwin.m 的 fallbackAlert「退出」按钮调用，退出进程。
func aellusQuit() {
	os.Exit(0)
}

// postOpenNotification 在 macOS 上弹一条系统通知；点击通知会用默认浏览器打开 url。
// 非 macOS 平台由 notify_other.go 提供空实现。不点击则什么都不会发生。
func postOpenNotification(title, body, url string) {
	ct := C.CString(title)
	cb := C.CString(body)
	cu := C.CString(url)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cb))
	defer C.free(unsafe.Pointer(cu))
	C.postNotify(ct, cb, cu)
}
