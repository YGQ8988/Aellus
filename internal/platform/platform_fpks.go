//go:build fpk

package platform

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"aellus/internal/app"
)

// fpkPlatform 是飞牛 fnOS fpk 构建的 Platform 实现。
//
// fpk 是安装在 NAS 上的【后台服务】，没有桌面、没有托盘、没有系统通知、
// 没有原生文件夹选择器、也没有 macOS 开机自启。普通桌面构建（Mac / Windows）
// 通过 tray_*/notify_*/pickdir_*/app_agent_darwin.go/single_* 提供这些能力，
// 这些文件都带 !fpk 标签被本构建排除。本实现用等价但「空 / headless」的方式
// 填补 Platform 接口，使 main() 无需任何条件分支即可统一编译。
//
// 注意：fpk 构建固定 GOOS=linux（见 build-fnos.sh），不会在 macOS / Windows 下编译。
type fpkPlatform struct{}

// NewPlatform 由 main 调用，build-tag 选择返回桌面端或 fpk 端实现。
func NewPlatform() app.Platform { return fpkPlatform{} }

// RunTray 在 fpk（无桌面）下仅阻塞进程，保持 HTTP 服务常驻。
// 等价于「无托盘」：不创建任何 GUI 元素，等待 SIGINT/SIGTERM 后退出。
func (fpkPlatform) RunTray(url string) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}

// PostOpenNotification fpk 下无桌面通知，空实现。
func (fpkPlatform) PostOpenNotification(title, body, url string) {}

// EnforceSingleInstance fpk 由 fnOS / systemd 保证单实例，应用自身无需加锁，直接放行。
func (fpkPlatform) EnforceSingleInstance() bool { return true }

// PickDirSupported fpk 是 NAS 后台服务，不存在本机浏览器来触发目录选择。
func (fpkPlatform) PickDirSupported() bool { return false }

// PickFolderDialog fpk 不支持原生目录选择，返回空串占位。
func (fpkPlatform) PickFolderDialog() string { return "" }

// PersistSaveDirAllowed fpk 端【不允许】把保存目录持久化到本地配置文件。
// 文件落盘位置完全由飞牛授权决定（cmd/main 注入的 AELLUS_SAVE_DIR +
// 官方 API trim.file.getSharedAccessibleFolders 返回的授权目录树）；
// 改路径只影响当前运行实例，重启后回到飞牛注入值，避免本地文件污染授权语义。
func (fpkPlatform) PersistSaveDirAllowed() bool { return false }

// EnforceAuthBoundary fpk 端强制"保存目录必须落在飞牛授权目录树内"。
// 飞牛应用设置中授权的目录（经 TRIM_DATA_ACCESSIBLE_PATHS /
// TRIM_DATA_SHARE_PATHS / 官方后端 API 查询）才是应用可写的合法范围，
// web 只能在已授权目录（含其子树）内选，不能跳出授权边界、不能写任意路径。
func (fpkPlatform) EnforceAuthBoundary() bool { return true }

// OwnersBaseDir fpk 端：归属 manifest 集中存放到飞牛私有运行时数据目录，
// 不再散落在用户共享目录里生成 .aellus-owners 隐藏文件。
//
// 优先级：
//   1. AELLUS_OWNERS_DIR —— cmd/main 在 TRIM_PKGVAR 非空且以 /vol 开头时注入
//      （TRIM_PKGVAR = /vol[x]/@appdata/[appname]，官方「运行时动态数据、卸载保留」目录）
//   2. 回退 TRIM_PKGVAR/owners（若变量已设但未注入 AELLUS_OWNERS_DIR）
//   3. 最终兜底：应用可执行文件目录下的 .owners（防御式，避免 TRIM_PKGVAR 异常时误写系统根）
//
// 防御式校验：文档要求使用路径变量前必须校验非空且以 /vol 开头，否则不使用，
// 防止对系统根目录造成灾难性写入。
func (fpkPlatform) OwnersBaseDir(saveDir string) string {
	if d := os.Getenv("AELLUS_OWNERS_DIR"); d != "" {
		return d
	}
	if v := os.Getenv("TRIM_PKGVAR"); v != "" && strings.HasPrefix(v, "/vol") {
		return filepath.Join(v, "owners")
	}
	// 兜底：用可执行文件所在目录下的 .owners（与二进制同生命周期，不会污染共享目录）
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), ".owners")
	}
	return ".owners"
}

// LogsDir fpk 端：访问/操作日志存放到飞牛私有运行时数据目录，不写入应用安装目录。
// 优先级：AELLUS_LOGS_DIR（cmd/main 注入）> TRIM_PKGVAR/logs > 可执行文件同目录（旧行为兜底）。
func (fpkPlatform) LogsDir() string {
	if d := os.Getenv("AELLUS_LOGS_DIR"); d != "" {
		return d
	}
	if v := os.Getenv("TRIM_PKGVAR"); v != "" && strings.HasPrefix(v, "/vol") {
		return filepath.Join(v, "logs")
	}
	// 兜底：可执行文件同目录（与旧行为一致）
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}
