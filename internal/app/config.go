// 配置：保存目录、监听地址、端口、日志路径。
package app

import (
	"os"
	"path/filepath"
)

// 默认监听地址与端口。
const (
	DefaultHost = "0.0.0.0"
	DefaultPort = 8000
)

// 全局配置，运行时可覆盖（main 中根据参数 / 交互输入更新）。
var (
	SaveDir   string // 文件保存根目录（绝对路径）
	Host      = DefaultHost
	Port      = DefaultPort
	AccessLog string // 访问日志文件路径
	OpLog     string // 操作日志文件路径
	ExeDir    string // 可执行文件所在目录（日志写这里）
)

// InitConfig 在 main 启动早期初始化路径配置。
//
// 日志写在 exe 所在目录而非工作目录，这样双击运行时日志与程序在一起。
func InitConfig() {
	if exe, err := os.Executable(); err == nil {
		ExeDir = filepath.Dir(exe)
	} else {
		ExeDir, _ = os.Getwd()
	}
	AccessLog = filepath.Join(ExeDir, "access.log")
	OpLog = filepath.Join(ExeDir, "operation.log")

	// 默认保存目录：~/Desktop/aellus-drops
	home, _ := os.UserHomeDir()
	SaveDir = filepath.Join(home, "Desktop", "aellus-drops")
}
