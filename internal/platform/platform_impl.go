//go:build !fpk

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"aellus/internal/app"
)

// platformImpl 是桌面端（macOS / Windows）的 Platform 实现。
//
// 所有平台差异能力由同包的 build-tag 文件提供为包级函数：
//   - runTray / setAgentMode / flushPendingBalloon  → tray_*.go / app_agent_darwin.go / notify_windows.go
//   - postOpenNotification                          → notify_*.go
//   - enforceSingleInstance                         → single_*.go
//   - pickFolderDialogWindows                       → pickdir_*.go
// 本文件仅做 Platform 接口适配，不引入新的平台逻辑。
type platformImpl struct{}

// NewPlatform 由 main 调用，build-tag 选择返回桌面端或 fpk 端实现。
func NewPlatform() app.Platform { return platformImpl{} }

// openBrowser 用系统默认浏览器打开指定 URL（跨平台）。
// 由 tray_*.go / notify_*.go 在用户点击菜单项或通知时调用。
func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// rundll32 调系统的 URL 协议处理器，最稳，不弹黑窗口
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	_ = cmd.Start()
}

func (platformImpl) RunTray(url string)                  { runTray(url) }
func (platformImpl) PostOpenNotification(t, b, u string) { postOpenNotification(t, b, u) }
func (platformImpl) EnforceSingleInstance() bool         { return enforceSingleInstance() }
func (platformImpl) EnforceAuthBoundary() bool           { return false }

// PickDirSupported 桌面端支持系统原生目录选择。
// OpenWrt 例外：它是无桌面的路由器后台服务（procd 守护进程），没有原生对话框可弹，
// 保存目录由 UCI（/etc/config/aellus 的 save_dir）在 LuCI 里配置，故返回 false。
func (platformImpl) PickDirSupported() bool {
	return !app.IsOpenWrt()
}

// PersistSaveDirAllowed 桌面端允许把保存目录持久化到 aellus-settings.json。
//
// OpenWrt 例外（返回 false）：那里保存目录的唯一真源是 UCI —— LuCI「服务 → Aellus」
// 的 save_dir，由 init 脚本注入 AELLUS_SAVE_DIR。若再持久化一份到 JSON，重启后 JSON
// 会反过来覆盖 UCI，出现「LuCI 里显示 A、实际却用 B」的错位，故禁止持久化。
// 网页里改保存目录仍然立即生效（进程内切换），只是重启后回到 UCI 配置值。
func (platformImpl) PersistSaveDirAllowed() bool {
	return !app.IsOpenWrt()
}

// ConfigBaseDir 桌面端：配置数据统一存放到系统配置目录下的 Aellus/ 子目录，
// 与 aellus-settings.json 同级，删除/重装程序不丢失。
func (platformImpl) ConfigBaseDir(saveDir string) string {
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		return filepath.Join(configDir, "Aellus")
	}
	// 回退：取不到系统配置目录，用保存目录
	return saveDir
}

// LogsDir 桌面端：访问/操作日志集中存放到系统配置目录下的 logs/ 子目录，
// 不再散落在 .app 同级目录（拖到 /Applications 后不会在系统目录里生成日志）。
// 与 aellus-settings.json 同级（系统配置目录/Aellus/logs/）。
//
// AELLUS_LOGS_DIR 可覆盖（与 fpk 端一致）：OpenWrt 的 init 脚本用它把日志指到
// tmpfs，避免访问日志持续写闪存（路由器 NAND 写入寿命有限）。
func (platformImpl) LogsDir() string {
	if d := os.Getenv("AELLUS_LOGS_DIR"); d != "" {
		return d
	}
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		return filepath.Join(configDir, "Aellus", "logs")
	}
	// 回退：取不到系统配置目录，退回 .app / 可执行文件同级
	return app.ResolveBaseDir()
}

// PickFolderDialog 弹出系统原生"选择文件夹"对话框，返回选中的绝对路径；用户取消返回空串。
//   - Windows：进程内调用 SHBrowseForFolderW（pickdir_windows.go），不启动外部进程
//   - macOS：osascript choose folder（原生对话框）
//   - Linux 等无桌面选择器：osascript 不存在会失败返回空（前端按取消处理）
func (platformImpl) PickFolderDialog() string {
	switch runtime.GOOS {
	case "windows":
		return pickFolderDialogWindows()
	default:
		out, err := exec.Command("osascript", "-e", "POSIX path of (choose folder)").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
}
