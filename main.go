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

	"aellus/internal/app"
	"aellus/internal/platform"
)

// version 由 build.sh 通过 -ldflags "-X main.version=..." 注入，默认 dev。
var version = "dev"

func main() {
	platform.InitConsoleUTF8() // Windows 下将控制台切到 UTF-8 + 中文字体（其他平台空操作）
	app.InitConfig()

	var dirFlag string
	var portFlag int
	flag.StringVar(&dirFlag, "dir", "", platform.FlagDirUsage)
	flag.IntVar(&portFlag, "port", app.DefaultPort, platform.FlagPortUsage)
	flag.Parse()

	// 是否交互式终端（决定是否提示输入目录、是否弹通知）
	interactive := platform.IsTerminal()

	// 确定保存目录：命令行参数 > 交互式输入 > 默认桌面/aellus-drops
	var saveDir string
	switch {
	case dirFlag != "":
		saveDir = dirFlag
	case interactive:
		home, _ := os.UserHomeDir()
		defaultDir := filepath.Join(home, "Desktop", "aellus-drops")
		fmt.Println()
		fmt.Println(platform.BannerTop)
		fmt.Println(platform.BannerTitle)
		fmt.Printf(platform.BannerVerFmt, version)
		fmt.Println(platform.BannerBottom)
		fmt.Println()
		fmt.Printf(platform.PromptSaveDir, defaultDir)
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
		saveDir = app.SaveDir
	}

	// 转绝对路径并创建
	abs, err := filepath.Abs(saveDir)
	if err == nil {
		saveDir = abs
	}
	app.SaveDir = saveDir
	if err := os.MkdirAll(app.SaveDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, platform.MsgMkdirFail, err)
		os.Exit(1)
	}
	app.Port = portFlag

	// 初始化日志与模板
	app.InitLoggers()
	if err := app.InitTemplates(); err != nil {
		fmt.Fprintf(os.Stderr, platform.MsgTmplFail, err)
		os.Exit(1)
	}

	ip := app.GetLanIP()
	fmt.Println()
	fmt.Printf(platform.MsgSaveDir, app.SaveDir)
	fmt.Printf(platform.MsgAccessURL, ip, app.Port)
	fmt.Println(platform.MsgLanHint)
	fmt.Println(platform.MsgStarting)
	fmt.Println()

	accessURL := fmt.Sprintf("http://%s:%d", ip, app.Port)

	handler := app.AccessLogMiddleware(app.RegisterRoutes())
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", app.Host, app.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second, // 防止慢速攻击
	}

	// macOS 双击 .app 启动（非终端）：菜单栏常驻模式，HTTP 服务放后台 goroutine
	if !interactive && platform.MenuBarEnabled() {
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, platform.MsgServerFail, err)
			}
		}()
		platform.RunMenuBar(app.SaveDir, accessURL) // 发启动通知 + 状态栏常驻，阻塞直到点击「退出」
		return
	}

	// 非终端但无状态栏（如交叉编译的命令行版）：弹通知告知访问地址
	if !interactive {
		platform.NotifyStartup(app.SaveDir, accessURL)
	}

	// 常规模式：阻塞在 HTTP 服务器
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, platform.MsgServerFail, err)
		os.Exit(1)
	}
}
