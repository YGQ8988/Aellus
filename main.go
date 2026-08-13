// Aellus 局域网文件互传服务 — 入口。
// 启动: ./aellus [--dir <保存目录>] [--port <端口>]
// 访问: 浏览器打开 http://<本机IP>:8000
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// version 由 build.sh 通过 -ldflags "-X main.version=..." 注入，默认 dev。
var version = "dev"

func main() {
	initConfig()

	var dirFlag string
	var portFlag int
	flag.StringVar(&dirFlag, "dir", "", "文件保存根目录（不填则交互式输入）")
	flag.IntVar(&portFlag, "port", DefaultPort, "服务端口（默认 8000）")
	flag.Parse()

	// 确定保存目录：命令行参数 > 交互式输入 > 默认桌面/aellus-drops
	// 与原 Python 版三段逻辑一致
	var saveDir string
	switch {
	case dirFlag != "":
		saveDir = dirFlag
	case isTerminal():
		home, _ := os.UserHomeDir()
		defaultDir := filepath.Join(home, "Desktop", "aellus-drops")
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════╗")
		fmt.Println("  ║          Aellus 文件互传             ║")
		fmt.Printf("  ║          版本: %-20s║\n", version)
		fmt.Println("  ╚══════════════════════════════════════╝")
		fmt.Println()
		fmt.Printf("  📁 文件保存目录（回车默认 %s）: ", defaultDir)
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			saveDir = line
		} else {
			saveDir = defaultDir
		}
	default:
		// 非交互模式（后台 / 管道启动）：用默认目录
		saveDir = SaveDir
	}

	// 转绝对路径并创建
	abs, err := filepath.Abs(saveDir)
	if err == nil {
		saveDir = abs
	}
	SaveDir = saveDir
	if err := os.MkdirAll(SaveDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "创建保存目录失败: %v\n", err)
		os.Exit(1)
	}
	Port = portFlag

	// 初始化日志与模板
	initLoggers()
	if err := initTemplates(); err != nil {
		fmt.Fprintf(os.Stderr, "模板初始化失败: %v\n", err)
		os.Exit(1)
	}

	ip := getLanIP()
	fmt.Println()
	fmt.Printf("  📁 保存目录: %s\n", SaveDir)
	fmt.Printf("  🌐 访问地址: http://%s:%d\n", ip, Port)
	fmt.Println("     (同局域网内，浏览器打开上面地址)")
	fmt.Println("  🚀 启动中... 按 Ctrl+C 停止")
	fmt.Println()

	handler := accessLogMiddleware(registerRoutes())
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", Host, Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second, // 防止慢速攻击
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

// isTerminal 判断 stdin 是否为终端（交互模式）。
// 对应原 Python 版 sys.stdin.isatty()。
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
