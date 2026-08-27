//go:build !windows && !fpk

package platform

// 非 Windows 单实例：用文件锁（flock）保证同一用户只有一个实例。
// flock 锁在进程退出/崩溃时由系统自动释放，不会留下失效锁。
// 重复启动时：macOS 弹系统通知告知，Linux 静默退出。

import (
	"os"
	"path/filepath"
	"syscall"
)

var gLockFile *os.File

// enforceSingleInstance 单实例检查。返回 false 表示已有实例在运行，调用方应立即退出。
func enforceSingleInstance() bool {
	dir, err := os.UserConfigDir()
	if err != nil {
		return true
	}
	appDir := filepath.Join(dir, "aellus")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return true
	}
	f, err := os.OpenFile(filepath.Join(appDir, "app.lock"), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return true // 无法创建锁文件：放行，不阻塞启动
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		// 锁被占用 -> 已有实例
		f.Close()
		postOpenNotification("Aellus 已在运行", "程序已在运行，无需重复启动", "")
		return false
	}
	gLockFile = f // 持有锁到进程退出（自动解锁）
	return true
}
