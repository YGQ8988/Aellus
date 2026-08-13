//go:build darwin

// macOS 通知：双击 .app 启动时终端输出不可见，用系统通知（toast）告知访问地址。
package platform

import (
	"fmt"
	"os/exec"
)

// NotifyStartup 在 macOS 通知中心展示文件存储目录与访问地址。
func NotifyStartup(saveDir, accessURL string) {
	msg := fmt.Sprintf("文件存储目录: %s\n访问地址: %s", saveDir, accessURL)
	// display notification 的 message 用 %q 转义，避免路径/URL 中的特殊字符注入
	script := fmt.Sprintf(`display notification %q with title "Aellus 已启动"`, msg)
	_ = exec.Command("osascript", "-e", script).Run()
}
