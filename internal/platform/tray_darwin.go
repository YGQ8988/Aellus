//go:build darwin && !fpk

package platform

/*
#cgo LDFLAGS: -framework Cocoa -framework AppKit
// Declaration only — implementation is in forceSetTemplateIcon.m (compiled as ObjC)
void forceSetTemplateIcon(const char *bytes, int length);
void setupQRMenuHover(void);
*/
import "C"
import (
	_ "embed"
	"strings"
	"time"
	"unsafe"
	"github.com/getlantern/systray"

	"aellus/internal/qrpop"
)

//go:embed menuicon.png
var menuIconPNG []byte

// gLanURL 是局域网访问地址（手机扫码用），来自首页中间 lanUrl。
var gLanURL string

//export goQRHover
// goQRHover 由 forceSetTemplateIcon.m 给 systray 的 NSMenu 挂的 delegate 调用：
// 当菜单里被高亮（hover）到「访问二维码」项时 hovered=1 弹二维码，移到别处或菜单收起时
// hovered=0 收起。这正是 LanCoClip 用 HoveredCh 实现的效果——菜单项级 hover。
// 此函数被 ObjC 在主线程调用；qrpop.Show 内部会再 dispatch 到主线程，安全。
func goQRHover(hovered C.int) {
	if hovered == 1 {
		_ = qrpop.Show(gLanURL)
	} else {
		qrpop.Hide()
	}
}

// runTray provides the macOS menu bar icon + right-click menu (open browser / QR / quit) via systray.
// systray uses cgo(Cocoa) on darwin, so this file can only be compiled on macOS (needs Xcode CLI tools).
// withQRParam 追加 ?qr=1 查询参数（若 URL 已带查询则用 &），让首页打开后自动弹出二维码。
func withQRParam(u string) string {
	if u == "" {
		return u
	}
	if strings.Contains(u, "?") {
		return u + "&qr=1"
	}
	return u + "?qr=1"
}

func runTray(url, lanURL string) {
	gLanURL = lanURL
	setAgentMode()
	systray.Run(func() {
		systray.SetTemplateIcon(menuIconPNG, menuIconPNG)
		systray.SetTitle("")
		systray.SetTooltip("Aellus")

		// Fallback: directly set NSStatusItem.button.image via Cocoa API.
		// Fixes missing icon on macOS 26 (Tahoe) where systray's internal path fails.
		// Retry after a delay — macOS 26 has a known backing-window/status-item
		// teardown race; re-setting the image on the main thread often recovers it.
		if len(menuIconPNG) > 0 {
			cptr := (*C.char)(unsafe.Pointer(&menuIconPNG[0]))
			C.forceSetTemplateIcon(cptr, C.int(len(menuIconPNG)))
			go func() {
				for _, d := range []time.Duration{300 * time.Millisecond, 1200 * time.Millisecond} {
					time.Sleep(d)
					C.forceSetTemplateIcon(cptr, C.int(len(menuIconPNG)))
				}
			}()
		}

	mOpen := systray.AddMenuItem("打开浏览器", "在默认浏览器中打开 Aellus")
	mQR := systray.AddMenuItem("访问二维码", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 Aellus")

		// 给 systray 的 NSMenu 挂 delegate：菜单打开后，鼠标 hover 到「访问二维码」项即
		// 弹出二维码浮窗（menu:willHighlightItem:），移到别处或菜单收起即收起。点击该项
		// 仍走 systray 原本的 menuHandler（打开浏览器），不受 delegate 影响。
		C.setupQRMenuHover()

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser(url)
				case <-mQR.ClickedCh:
					// 点击「访问二维码」：打开本机页面并带 ?qr=1，页面会自动弹出二维码
					// （与 LanCoClip 一致）。悬浮二维码仍是 hover 专属。
					openBrowser(withQRParam(url))
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}
