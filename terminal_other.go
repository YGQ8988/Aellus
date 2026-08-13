//go:build !darwin

// isTerminal 的非 macOS 实现：判断 stdin 是否为终端（交互模式）。
package main

import "os"

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
