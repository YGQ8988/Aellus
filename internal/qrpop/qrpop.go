// Package qrpop shows a QR code in a small borderless floating window, so the
// phone-scannable LAN address is one hover away from the tray menu.
//
// The window is a passive overlay: it ignores mouse events and never takes
// focus, otherwise it would steal the very hover that opened it.
package qrpop

import (
	"errors"
	"sync"
	qrcode "github.com/skip2/go-qrcode"
)

// qrSize is the edge length, in pixels, of the generated QR image. The window
// is this plus a small padding.
const qrSize = 256

// ErrNoURL is returned by Show when there is no address to encode yet (the
// server has not picked a port, or no LAN interface is up).
var ErrNoURL = errors.New("no URL to show")

var (
	cacheMu  sync.Mutex
	cacheURL string
	cachePNG []byte
)

// Show renders url as a QR code in the floating window, positioned next to the
// pointer, replacing whatever was shown before.
func Show(url string) error {
	if url == "" {
		return ErrNoURL
	}
	png, err := pngFor(url)
	if err != nil {
		return err
	}
	return showPNG(png)
}

// pngFor returns the PNG for url, reusing the previous render while the url is
// unchanged: hover fires on every pointer move, but the code only depends on
// the address, so re-encoding it each time is pure waste.
func pngFor(url string) ([]byte, error) {
	cacheMu.Lock()
	cachedURL, cachedPNG := cacheURL, cachePNG
	cacheMu.Unlock()
	if url == cachedURL && len(cachedPNG) > 0 {
		return cachedPNG, nil
	}
	png, err := qrcode.Encode(url, qrcode.Medium, qrSize)
	if err != nil {
		return nil, err
	}
	cacheMu.Lock()
	cacheURL, cachePNG = url, png
	cacheMu.Unlock()
	return png, nil
}
