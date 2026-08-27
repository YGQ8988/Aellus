//go:build darwin && !fpk

package platform

/*
#cgo LDFLAGS: -framework Cocoa -framework AppKit
// Declaration only — implementation is in forceSetTemplateIcon.m (compiled as ObjC)
void forceSetTemplateIcon(const char *bytes, int length);
*/
import "C"
import (
	_ "embed"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
)

//go:embed menuicon.png
var menuIconPNG []byte

// runTray provides the macOS menu bar icon + right-click menu (open browser / quit) via systray.
// systray uses cgo(Cocoa) on darwin, so this file can only be compiled on macOS (needs Xcode CLI tools).
func runTray(url string) {
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
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 Aellus")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser(url)
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}
