//go:build windows

package app

// availableBytes Windows 下返回「未知」：标准库 syscall 没有 Statfs，
// 获取可用空间需要 WinAPI（GetDiskFreeSpaceEx）。返回 ok=false 表示
// 「拿不到剩余空间」，调用方应跳过基于空间的限制，而不是当成 0。
//
// 因此 Windows 桌面端不受批量打包空间限制影响（该端本机使用，
// 且 zip 落在保存目录内，磁盘写满是本机用户自己能看到的）。
func availableBytes(path string) (uint64, bool) {
	return 0, false
}