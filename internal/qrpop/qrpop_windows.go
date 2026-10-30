//go:build windows

package qrpop

import (
	"bytes"
	"image"
	_ "image/png" // 注册 PNG 解码器：二维码就是 PNG
	"runtime"
	"sync"
	"unsafe"
	"golang.org/x/sys/windows"
)

const (
	winPad = 10
	winW   = qrSize + winPad*2
	winH   = qrSize + winPad*2

	wmApp   = 0x8000 // WM_APP
	msgShow = wmApp + 1
	msgHide = wmApp + 2
	wmPaint = 0x000F

	wmSetCursor = 0x0020
	idcArrow    = 32512 // IDC_ARROW

	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExTransparent = 0x00000020

	swHide   = 0
	swShowNA = 8

	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	smCXScreen = 0
	smCYScreen = 1

	dibRGBColors = 0
	srcCopy      = 0x00CC0020

	className = "LanCoClipQrPopover"
)

var (
	k32              = windows.NewLazySystemDLL("Kernel32.dll")
	pGetModuleHandle = k32.NewProc("GetModuleHandleW")

	g32            = windows.NewLazySystemDLL("Gdi32.dll")
	pStretchDIBits = g32.NewProc("StretchDIBits")

	u32               = windows.NewLazySystemDLL("User32.dll")
	pCreateWindowEx   = u32.NewProc("CreateWindowExW")
	pDefWindowProc    = u32.NewProc("DefWindowProcW")
	pDispatchMessage  = u32.NewProc("DispatchMessageW")
	pGetCursorPos     = u32.NewProc("GetCursorPos")
	pGetDC            = u32.NewProc("GetDC")
	pGetMessage       = u32.NewProc("GetMessageW")
	pGetSystemMetrics = u32.NewProc("GetSystemMetrics")
	pPostMessage      = u32.NewProc("PostMessageW")
	pRegisterClass    = u32.NewProc("RegisterClassExW")
	pReleaseDC        = u32.NewProc("ReleaseDC")
	pSetWindowPos     = u32.NewProc("SetWindowPos")
	pShowWindow       = u32.NewProc("ShowWindow")
	pValidateRect     = u32.NewProc("ValidateRect")
	pLoadCursor       = u32.NewProc("LoadCursorW")
	pSetCursor        = u32.NewProc("SetCursor")
)

type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background windows.Handle
	MenuName, ClassName                *uint16
	IconSm                             windows.Handle
}

type msg struct {
	Hwnd     windows.Handle
	Message  uint32
	_        uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	_        uint32
	Pt       struct{ X, Y int32 }
	LPrivate uint32
	_        uint32
}

// BITMAPINFOHEADER; only the fields StretchDIBits needs are declared, the tail
// (ClrUsed/ClrImportant) is implied by the size we report.
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
}

var (
	popMu       sync.Mutex
	popHwnd     windows.Handle
	arrowCursor windows.Handle // IDC_ARROW，浮窗光标（避免回退成等待光标）

	// The bitmap to draw. Hover fires on every pointer move, but the QR itself
	// only changes when the address does, so the conversion is cached.
	bitsMu  sync.Mutex
	bits    []byte
	bitsW   int
	bitsH   int
	lastPNG []byte

	startOnce sync.Once
	started   = make(chan struct{})
)

// Supported reports whether this platform can show the floating window.
func Supported() bool { return true }

// showPNG renders the PNG into the popover. The actual UI work is posted to the
// window's own thread: the hover arrives while TrackPopupMenu is running, and
// doing window work inside the menu's message loop would deadlock the menu.
func showPNG(png []byte) error {
	if len(png) == 0 {
		return nil
	}
	bitsMu.Lock()
	cached := bytes.Equal(lastPNG, png) && len(bits) > 0
	bitsMu.Unlock()
	if !cached {
		img, _, err := image.Decode(bytes.NewReader(png))
		if err != nil {
			return err
		}
		b, w, h := toBGRA(img)
		bitsMu.Lock()
		bits, bitsW, bitsH, lastPNG = b, w, h, png
		bitsMu.Unlock()
	}
	ensureWindow()
	popMu.Lock()
	hwnd := popHwnd
	popMu.Unlock()
	if hwnd == 0 {
		return nil
	}
	pPostMessage.Call(uintptr(hwnd), uintptr(msgShow), 0, 0)
	return nil
}

// Hide closes the floating window.
func Hide() {
	popMu.Lock()
	hwnd := popHwnd
	popMu.Unlock()
	if hwnd == 0 {
		return
	}
	pPostMessage.Call(uintptr(hwnd), uintptr(msgHide), 0, 0)
}

// ensureWindow starts the UI thread that owns the popover window, waiting until
// the window exists (or has failed to) so the first hover is not swallowed.
func ensureWindow() {
	startOnce.Do(func() { go uiLoop() })
	<-started
}

// uiLoop owns the popover: it registers the class, creates the window and pumps
// its messages. It must stay on one OS thread for its whole life, hence
// LockOSThread.
func uiLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	inst, _, _ := pGetModuleHandle.Call(0)
	cls, err := windows.UTF16PtrFromString(className)
	if err != nil {
		close(started)
		return
	}
	title, err := windows.UTF16PtrFromString("")
	if err != nil {
		close(started)
		return
	}
	c, _, _ := pLoadCursor.Call(0, uintptr(idcArrow))
	arrowCursor = windows.Handle(c)
	wcex := wndClassEx{
		Style:      0x0001 | 0x0002, // CS_HREDRAW | CS_VREDRAW
		WndProc:    windows.NewCallback(wndProc),
		Instance:   windows.Handle(inst),
		Background: windows.Handle(6), // COLOR_WINDOW + 1
		Cursor:     arrowCursor,       // 普通箭头光标，避免回退成等待光标
		ClassName:  cls,
	}
	wcex.Size = uint32(unsafe.Sizeof(wcex))
	res, _, _ := pRegisterClass.Call(uintptr(unsafe.Pointer(&wcex)))
	if res == 0 {
		close(started)
		return
	}
	hw, _, _ := pCreateWindowEx.Call(
		uintptr(wsExTopmost|wsExToolWindow|wsExTransparent),
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsPopup),
		uintptr(0), uintptr(0), uintptr(winW), uintptr(winH),
		uintptr(0), uintptr(0), inst, uintptr(0),
	)
	if hw != 0 {
		popMu.Lock()
		popHwnd = windows.Handle(hw)
		popMu.Unlock()
	}
	close(started)
	if hw == 0 {
		return
	}

	var m msg
	for {
		ret, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			return
		}
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case msgShow:
		placeAndShow(hwnd)
		return 0
	case msgHide:
		pShowWindow.Call(uintptr(hwnd), uintptr(swHide))
		return 0
	case wmPaint:
		draw(hwnd)
		return 0
	case wmSetCursor:
		// 浮窗是只读展示窗口，鼠标移上去显示普通箭头，不要回退成等待光标（转圈圈）
		if arrowCursor != 0 {
			pSetCursor.Call(uintptr(arrowCursor))
		}
		return 1
	}
	ret, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), uintptr(wParam), uintptr(lParam))
	return ret
}

// placeAndShow moves the window next to the cursor and paints it. SW_SHOWNA
// shows without activating: a popover that steals focus would break typing in
// whatever app the user came from.
func placeAndShow(hwnd windows.Handle) {
	var pt struct{ X, Y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	sw, _, _ := pGetSystemMetrics.Call(smCXScreen)
	sh, _, _ := pGetSystemMetrics.Call(smCYScreen)

	x := pt.X + 18
	if int(x)+winW > int(sw) {
		x = pt.X - winW - 18
	}
	if x < 0 {
		x = 0
	}
	y := pt.Y - winH/2
	if y < 0 {
		y = 0
	}
	if int(y)+winH > int(sh) {
		y = int32(sh) - winH
	}
	if y < 0 {
		y = 0
	}
	// HWND_TOPMOST is (HWND)-1: the popup menu is topmost too, and re-ordering
	// ourselves on every show keeps us above it (the menu would otherwise cover
	// the QR, since the two overlap right where the cursor is).
	pSetWindowPos.Call(uintptr(hwnd), ^uintptr(0), uintptr(x), uintptr(y),
		uintptr(winW), uintptr(winH), uintptr(swpNoActivate|swpShowWindow))
	pShowWindow.Call(uintptr(hwnd), uintptr(swShowNA))
	draw(hwnd)
}

// draw blits the cached bitmap. It paints from an explicit GetDC (rather than
// BeginPaint) so no PAINTSTRUCT marshalling is needed; ValidateRect then clears
// the invalid region so the system stops asking for a repaint.
func draw(hwnd windows.Handle) {
	bitsMu.Lock()
	b, w, h := bits, bitsW, bitsH
	bitsMu.Unlock()
	if len(b) == 0 {
		return
	}
	hdc, _, _ := pGetDC.Call(uintptr(hwnd))
	if hdc == 0 {
		return
	}
	bmi := bitmapInfoHeader{
		Size:     40,
		Width:    int32(w),
		Height:   int32(h),
		Planes:   1,
		BitCount: 32,
	}
	pStretchDIBits.Call(
		hdc,
		uintptr(winPad), uintptr(winPad), uintptr(w), uintptr(h),
		uintptr(0), uintptr(0), uintptr(w), uintptr(h),
		uintptr(unsafe.Pointer(&b[0])),
		uintptr(unsafe.Pointer(&bmi)),
		uintptr(dibRGBColors), uintptr(srcCopy),
	)
	pReleaseDC.Call(uintptr(hwnd), hdc)
	pValidateRect.Call(uintptr(hwnd), 0)
}

// toBGRA converts an image to the 32bpp bottom-up BGRA layout StretchDIBits
// expects.
func toBGRA(img image.Image) ([]byte, int, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		// DIBs are bottom-up: the top scanline is the image's last row.
		sy := b.Max.Y - 1 - y
		row := out[y*w*4 : (y+1)*w*4]
		for x := 0; x < w; x++ {
			r, g, bb, _ := img.At(b.Min.X+x, sy).RGBA()
			o := x * 4
			row[o+0] = byte(bb >> 8)
			row[o+1] = byte(g >> 8)
			row[o+2] = byte(r >> 8)
			row[o+3] = 0xFF
		}
	}
	return out, w, h
}
