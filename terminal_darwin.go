//go:build darwin

// isTerminal 的 macOS 实现：用 ioctl(TIOCGETA) 判断 stdin 是否为真正的 TTY。
//
// 双击 .app 启动时 stdin 是 /dev/null——它是字符设备(char device)但不是 TTY，
// 用 os.ModeCharDevice 判断会误判为终端，导致弹通知等逻辑失效。
// ioctl(TIOCGETA) 只对真正的 TTY 成功，/dev/null 会返回错误，判断准确。
package main

import (
	"os"
	"syscall"
	"unsafe"
)

func isTerminal() bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		os.Stdin.Fd(),
		uintptr(syscall.TIOCGETA),
		uintptr(unsafe.Pointer(&termios)),
		0, 0, 0,
	)
	return errno == 0
}
