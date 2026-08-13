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
	initConsoleUTF8() // Windows 下将控制台切到 UTF-8，避免中文/emoji 乱码（其他平台空操作）
	initConfig()

	var dirFlag string
	var portFlag int
	flag.StringVar(&dirFlag, "dir", "", flagDirUsage)
	flag.IntVar(&portFlag, "port", DefaultPort, flagPortUsage)
	flag.Parse()

	// 确定保存目录：命令行参数 > 交互式输入 > 默认桌面/aellus-drops
	var saveDir string
	switch {
	case dirFlag != "":
		saveDir = dirFlag
	case isTerminal():
		home, _ := os.UserHomeDir()
		defaultDir := filepath.Join(home, "Desktop", "aellus-drops")
		fmt.Println()
		fmt.Println(bannerTop)
		fmt.Println(bannerTitle)
		fmt.Printf(bannerVerFmt, version)
		fmt.Println(bannerBottom)
		fmt.Println()
		fmt.Printf(promptSaveDir, defaultDir)
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
		fmt.Fprintf(os.Stderr, msgMkdirFail, err)
		os.Exit(1)
	}
	Port = portFlag

	// 初始化日志与模板
	initLoggers()
	if err := initTemplates(); err != nil {
		fmt.Fprintf(os.Stderr, msgTmplFail, err)
		os.Exit(1)
	}

	ip := getLanIP()
	fmt.Println()
	fmt.Printf(msgSaveDir, SaveDir)
	fmt.Printf(msgAccessURL, ip, Port)
	fmt.Println(msgLanHint)
	fmt.Println(msgStarting)
	fmt.Println()

	handler := accessLogMiddleware(registerRoutes())
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", Host, Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second, // 防止慢速攻击
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, msgServerFail, err)
		os.Exit(1)
	}
}

// isTerminal 判断 stdin 是否为终端（交互模式）。
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
