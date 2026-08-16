//go:build darwin

// macOS 通知：双击 .app 启动时终端输出不可见，用系统通知（toast）告知访问地址。
package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"aellus/assets"
)

// NotifyStartup 在 macOS 通知中心展示文件存储目录与访问地址。
//
// 命令行二进制无 app bundle，UNUserNotificationCenter 会因 bundleProxy 为 nil 崩溃，
// 故用 osascript -l JavaScript（JXA）调 NSUserNotificationCenter（旧 API，无 bundle 限制），
// 并把内嵌的 favicon 渲染成临时 png 作为 contentImage，使通知正文显示项目图标。
// 任一步失败则降级为普通 display notification（图标为脚本编辑器，但通知不丢）。
func NotifyStartup(saveDir, accessURL string) {
	msg := fmt.Sprintf("文件存储目录: %s\n访问地址: %s", saveDir, accessURL)

	if pngPath, ok := renderIconPNG(); ok {
		defer os.Remove(pngPath)
		if deliverWithContentImage(msg, pngPath) {
			return
		}
	}

	// 降级：普通通知（无自定义图标）
	script := fmt.Sprintf(`display notification %q with title "Aellus 已启动"`, msg)
	_ = exec.Command("osascript", "-e", script).Run()
}

// renderIconPNG 从 embed FS 取 favicon.svg，经 sips 转为临时 png，返回路径。
func renderIconPNG() (string, bool) {
	svgData, err := assets.StaticFS.ReadFile("static/favicon.svg")
	if err != nil || len(svgData) == 0 {
		return "", false
	}
	tmpDir := os.TempDir()
	svgPath := filepath.Join(tmpDir, "aellus-notify-icon.svg")
	pngPath := filepath.Join(tmpDir, "aellus-notify-icon.png")
	defer os.Remove(svgPath)

	if err := os.WriteFile(svgPath, svgData, 0o644); err != nil {
		return "", false
	}
	// sips 把 svg 转 png（macOS 自带，build-mac-app.sh 亦用此）
	if err := exec.Command("sips", "-s", "format", "png", svgPath, "--out", pngPath).Run(); err != nil {
		return "", false
	}
	return pngPath, true
}

// deliverWithContentImage 用 JXA 发 NSUserNotification，contentImage 设为指定 png。
func deliverWithContentImage(msg, pngPath string) bool {
	// JXA 脚本：调 Cocoa 旧通知 API，设 contentImage 为临时 png。
	// 用 %q 转义避免消息/路径中的特殊字符注入。
	script := fmt.Sprintf(`
ObjC.import("Cocoa");
var img = $.NSImage.alloc.initByReferencingFile(%q);
var notif = $.NSUserNotification.alloc.init;
notif.title = "Aellus 已启动";
notif.informativeText = %q;
notif.contentImage = img;
notif.identifier = "aellus-startup";
$.NSUserNotificationCenter.defaultUserNotificationCenter.deliverNotification(notif);
`, pngPath, msg)
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", script)
	if err := cmd.Run(); err != nil {
		return false
	}
	// deliverNotification 异步投递，给系统一点时间真正展示，再让调用方清理临时 png
	time.Sleep(300 * time.Millisecond)
	return true
}
