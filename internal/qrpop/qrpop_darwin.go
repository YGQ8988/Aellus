//go:build darwin && cgo

package qrpop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework AppKit
void qrpopShow(const char *bytes, int length);
void qrpopHide(void);
*/
import "C"
import "unsafe"

// Supported reports whether this platform can show the floating window.
func Supported() bool { return true }

// showPNG hands the rendered QR to the Cocoa window. The real work (window
// creation, placement, ordering) happens on the main thread inside qrpopShow.
func showPNG(png []byte) error {
	if len(png) == 0 {
		return nil
	}
	C.qrpopShow((*C.char)(unsafe.Pointer(&png[0])), C.int(len(png)))
	return nil
}

// Hide closes the floating window.
func Hide() { C.qrpopHide() }
