//go:build !windows

package app

import "syscall"

// availableBytes 返回给定路径所在文件系统的可用字节数。
// 用于批量下载前判断"卷还装得下临时 ZIP"，避免把卷写满。
//
// 实现说明：syscall.Statfs 在 unix 平台（linux/darwin/bsd）可用，故用 !windows 标签；
// Windows 侧见 availableBytes_windows.go（返回未知，不做限制）。
func availableBytes(path string) (uint64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	// Bavail：非特权用户可用的块数；Bsize：每块字节数
	return uint64(st.Bavail) * uint64(st.Bsize), true
}