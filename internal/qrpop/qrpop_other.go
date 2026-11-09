//go:build !(darwin && cgo) && !windows

package qrpop

// Supported reports whether this platform can show the floating window. Native
// tray menus exist only on macOS and Windows, so nowhere else is there a hover
// to trigger it.
func Supported() bool { return false }

func showPNG(png []byte) error { return ErrNoURL }

// Hide closes the floating window.
func Hide() {}
