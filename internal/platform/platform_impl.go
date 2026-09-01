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
func (platformImpl) PickDirSupported() bool              { return true }
func (platformImpl) PersistSaveDirAllowed() bool         { return true }
func (platformImpl) EnforceAuthBoundary() bool           { return false }

// OwnersBaseDir 桌面端：归属 manifest 集中存放到系统配置目录下的 owners/ 子目录，
// 不再散落在用户保存目录里（避免与上传文件混在一起被用户看到）。
// 与 aellus-settings.json 同级（系统配置目录/Aellus/owners/）。
func (platformImpl) OwnersBaseDir(saveDir string) string {
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		return filepath.Join(configDir, "Aellus", "owners")
	}
	// 回退：取不到系统配置目录，用保存目录下的 .owners 子目录（隐藏，不污染可见文件）
	return filepath.Join(saveDir, ".owners")
}

// LogsDir 桌面端：访问/操作日志集中存放到系统配置目录下的 logs/ 子目录，
// 不再散落在 .app 同级目录（拖到 /Applications 后不会在系统目录里生成日志）。
// 与 aellus-settings.json / owners 同级（系统配置目录/Aellus/logs/）。
func (platformImpl) LogsDir() string {
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		return filepath.Join(configDir, "Aellus", "logs")
	}
	// 回退：取不到系统配置目录，退回 .app / 可执行文件同级（旧行为）
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
