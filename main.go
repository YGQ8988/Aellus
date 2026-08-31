// Aellus —— Go 单文件版。Windows 托盘用纯标准库 syscall（零依赖）；
// macOS 菜单栏用 systray 库（走 cgo/Cocoa，需在 Mac 本机编译）。前端 templates/、static/ 经 //go:embed 编入 exe。
//
// 编译成一个可执行文件，运行时不需要任何外部文件。
// 业务逻辑在 internal/app/，平台差异通过 Platform 接口注入（见 internal/platform/）。

package main

import (
	"embed"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	"aellus/internal/app"
	"aellus/internal/platform"
)

// === 版本 ===
// 由构建脚本通过 -ldflags "-X main.Version=x.y.z" 注入。
var Version = "1.0.0"

// === 启动语言 ===
// Linux 终端字体/locale 差异大，中文易显示成黑方块（字体缺中文字形，程序无法替终端装字体），
// 默认英文彻底规避；macOS/Windows 图形终端字体齐全，默认中文。AELLUS_LANG=en|zh 可强制覆盖。
var langEN = func() bool {
	switch strings.ToLower(os.Getenv("AELLUS_LANG")) {
	case "en":
		return true
	case "zh":
		return false
	}
	return runtime.GOOS == "linux"
}()

// tr 按启动语言返回中文或英文文案，用于控制台输出，避免 Linux 终端中文黑方块。
func tr(zh, en string) string {
	if langEN {
		return en
	}
	return zh
}

// === 嵌入资源 ===
// //go:embed 把整个 templates/ 和 static/ 目录在编译期塞进可执行文件，运行时不需要这些文件存在。
//
//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

func main() {
	// 平台实现由 build-tag 选择：platformImpl(!fpk 桌面) / fpkPlatform(fpk 飞牛 NAS)
	p := platform.NewPlatform()

	// 0) 单实例：已有实例在运行时退出，不再启动第二个进程
	if !p.EnforceSingleInstance() {
		os.Exit(0)
	}

	// 1) 数据目录解析
	// 日志目录：与 settings/owners 统一到系统配置目录，拖到 /Applications 不再污染系统目录。
	baseDir := p.LogsDir()
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		// 系统配置目录不可写（罕见），回退到 .app / 可执行文件同级（旧行为）
		baseDir = app.ResolveBaseDir()
	}
	defaultSave := app.ResolveSaveDir() // 上传文件保存目录（桌面 file-drops，跨机器默认可写）
	saveParent := defaultSave
	// 飞牛 fnOS：cmd/main 通过 AELLUS_SAVE_DIR 注入共享目录，把它当"配置"而非"默认"。
	if d := os.Getenv("AELLUS_SAVE_DIR"); d != "" {
		saveParent = d
	}
	// 桌面端才允许持久化用户选择的保存目录（fpk 端不读本地配置，路径完全由飞牛授权决定）
	if p.PersistSaveDirAllowed() {
		// 一次性迁移：把旧版残留在二进制同级的 aellus-settings.json 搬到系统配置目录
		app.MigrateLegacySettings()
		if cfgDir := app.LoadSaveDirConfig(); cfgDir != "" {
			saveParent = cfgDir
		}
	}

	// 2) 保存目录转成绝对路径，并确保目录存在
	if err := os.MkdirAll(saveParent, 0755); err != nil {
		if saveParent != defaultSave {
			// 配置里指定的保存目录不可用（常见于 app 被分发到他人机器、
			// 原路径属于别的用户而无权限），回退到当前用户默认可写目录，避免直接崩溃。
			log.Printf(tr("警告：配置的保存目录 %q 不可用（%v），已回退到默认目录 %s",
				"warning: configured save dir %q unavailable (%v), fell back to default %s"),
				saveParent, err, defaultSave)
			saveParent = defaultSave
			if mkErr := os.MkdirAll(saveParent, 0755); mkErr != nil {
				log.Fatal(tr("创建默认保存目录失败: ", "failed to create default save dir: ") + mkErr.Error())
			}
			// 把回退后的默认目录写回配置，避免下次再尝试坏路径
			_ = app.SaveSaveDirConfig(saveParent)
		} else {
			log.Fatal(tr("创建保存目录失败: ", "failed to create save dir: ") + err.Error())
		}
	}

	// 3) 构造 App（解析模板、记录日志路径、设置保存目录）
	a := app.New(p, app.Options{
		TemplatesFS: templatesFS,
		StaticFS:    staticFS,
		BaseDir:     baseDir,
		SaveDir:     saveParent,
	})

	// 3.5) 桌面端：一次性迁移旧版散落在保存目录里的归属 manifest 到集中目录
	//      （旧版把 <sha1>.json 直接写在 saveDir 里，与上传文件混在一起；现在集中到系统配置目录）
	if p.PersistSaveDirAllowed() {
		app.MigrateLegacyOwners(saveParent, p.OwnersBaseDir(saveParent))
	}

	// 4) 局域网 IP + 端口（被占用自动 +1）
	//    端口优先级：AELLUS_PORT 环境变量（飞牛 cmd/main 注入）> DefaultPort
	ip := app.GetLANIP()
	listenPort := app.DefaultPort
	if ep := os.Getenv("AELLUS_PORT"); ep != "" {
		if n, err := strconv.Atoi(ep); err == nil && n > 0 && n < 65536 {
			listenPort = n
		}
	}
	ln, port := func() (net.Listener, int) {
		if os.Getenv("AELLUS_STRICT_PORT") == "1" {
			// 飞牛等平台环境：平台已管理端口（manifest service_port + 向导选择），
			// 严格监听声明端口，占用即 Fatal 退出，避免静默换端口导致与 service_port 错位。
			return app.ListenStrict(listenPort)
		}
		// 桌面端：被占用自动 +1 兜底
		return app.ListenWithFallback(listenPort)
	}()

	// 5) 启动 HTTP 服务（在独立 goroutine 中运行，不阻塞）
	a.Serve(ln, port)

	// 6) 打印启动信息
	desktopURL := "http://localhost:" + strconv.Itoa(port)
	fmt.Println(tr("Aellus 已启动 (Go 单文件版)", "Aellus started (Go single-binary)"))
	fmt.Println(tr("保存目录：", "Save dir: ") + a.SaveDir())
	fmt.Println(tr("本机局域网 IP：", "LAN IP:  ") + ip)
	fmt.Println(tr("访问地址：", "Local:   ") + "http://localhost:" + strconv.Itoa(port))
	fmt.Println(tr("手机访问：", "Mobile:  ") + "http://" + ip + ":" + strconv.Itoa(port))
	fmt.Println(tr("按 Ctrl+C 停止", "Press Ctrl+C to stop"))

	// 7) 操作日志 + 通知 + 托盘
	a.LogOp("启动成功 访问地址=" + desktopURL)
	// 无头模式（CI / 终端常驻 / 调试）：跳过菜单栏 GUI，仅常驻 HTTP 服务。
	if os.Getenv("AELLUS_HEADLESS") == "1" {
		log.Println(tr("[headless] 已跳过菜单栏 GUI，仅运行 HTTP 服务于",
			"[headless] skipped tray GUI, HTTP server at"), desktopURL)
		select {} // 阻塞，保持进程存活
	}
	// 启动后弹系统通知（macOS/Windows）；点击通知才打开浏览器，不点击不跳转。
	p.PostOpenNotification("Aellus 已就绪", "点击打开文件传输页", desktopURL)
	p.RunTray(desktopURL)
}
