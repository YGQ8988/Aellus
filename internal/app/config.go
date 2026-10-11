package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// === 配置与路径纯函数 ===
//
// 这些函数不依赖 App 状态，仅基于环境/可执行文件位置计算路径，
// 供 main 包在启动阶段调用，结果注入 App。

// ResolveBaseDir 返回“日志根目录”：日志放在这里（与可执行文件/ .app 同级，避免污染桌面）。
// 关键：必须基于可执行文件自身位置，不能依赖 cwd——
// 双击 .app 启动时系统把 cwd 设为 “/”，用相对路径会尝试写根目录而失败退出。
//   - 普通二进制：放在可执行文件同目录。
//   - .app 包（.../Aellus.app/Contents/MacOS/二进制）：放在 .app 的同级目录，
//     即用户能看到 Aellus.app 和日志文件并列，方便查找，也不污染 app 包内部。
func ResolveBaseDir() string {
	exe, err := os.Executable()
	if err != nil {
		if wd, werr := os.Getwd(); werr == nil {
			return wd
		}
		return "."
	}
	exeDir := filepath.Dir(exe)
	// 形如 .../Aellus.app/Contents/MacOS
	if contents := filepath.Dir(exeDir); filepath.Base(contents) == "Contents" {
		appDir := filepath.Dir(contents) // .../Aellus.app
		return filepath.Dir(appDir)      // .app 的同级目录
	}
	return exeDir
}

// resolveSaveDir 返回“上传文件保存目录”：放在用户桌面下的 file-drops，
// 这样收到的文件直接在 Finder / 资源管理器桌面可见，符合用户直觉。
// 跨平台：macOS 与 Windows 的桌面都在 家目录/Desktop 下。
// 取不到家目录时回退到可执行文件同目录（不会崩）。
func resolveSaveDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Desktop", saveDirName)
	}
	return filepath.Join(ResolveBaseDir(), saveDirName)
}

// ResolveSaveDir 是 resolveSaveDir 的导出版本，供 main 包启动阶段调用。
func ResolveSaveDir() string {
	return resolveSaveDir()
}

// bootDefaultSaveDir 返回「恢复默认」应当回落到的保存目录：
// 飞牛 fnOS 下是 cmd/main 通过 AELLUS_SAVE_DIR 注入的授权/共享目录（如 /vol1/@appcenter/Aellus/file-drops），
// 其余平台是桌面 file-drops。这才是用户语境里的"默认"，而不是 resolveSaveDir()。
// 注意：用户持久化配置（aellus-settings.json，位于系统配置目录）覆盖此值，
// 调用 LoadSaveDirConfig 的代码负责链式优先级。
func bootDefaultSaveDir() string {
	if d := os.Getenv("AELLUS_SAVE_DIR"); d != "" {
		return d
	}
	return resolveSaveDir()
}

// === 保存目录配置持久化 ===
// 用户可在首页设置里修改文件保存路径，写入系统配置目录下的 aellus-settings.json，重启后自动生效。
// 配置目录由 os.UserConfigDir() 决定：
//   - macOS: ~/Library/Application Support/Aellus/aellus-settings.json
//   - Windows: %APPDATA%\Aellus\aellus-settings.json
//   - Linux: $XDG_CONFIG_HOME/Aellus/ 或 ~/.config/aellus/aellus-settings.json
// 取不到配置目录时回退到可执行文件同级（ResolveBaseDir），保证可用。

func settingsConfPath() string {
	// 飞牛 fpk：配置文件写到飞牛私有运行时数据目录（TRIM_PKGVAR，位于 /vol 持久卷），
	// 应用重启 / NAS 重启后不丢失。防御式校验：必须以 /vol 开头才使用。
	if v := os.Getenv("TRIM_PKGVAR"); v != "" && strings.HasPrefix(v, "/vol") {
		return filepath.Join(v, SettingsFile)
	}
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		return filepath.Join(configDir, "Aellus", SettingsFile)
	}
	// 回退：取不到系统配置目录（HOME 未设等极端情况），用可执行文件同级
	return filepath.Join(ResolveBaseDir(), SettingsFile)
}

// legacySettingsConfPath 返回旧版配置路径（与可执行文件/.app 同级），
// 仅供一次性迁移使用：把旧位置残留的 aellus-settings.json 搬到系统配置目录后删除。
func legacySettingsConfPath() string {
	return filepath.Join(ResolveBaseDir(), SettingsFile)
}

// MigrateLegacySettings 一次性迁移：若新位置无配置、旧位置（二进制同级）有配置，
// 则把旧文件内容搬到新位置并删除旧文件，保证用户无感升级。迁移失败不阻断启动。
func MigrateLegacySettings() {
	newPath := settingsConfPath()
	if newPath == legacySettingsConfPath() {
		return // 回退场景下两者相同，无需迁移
	}
	// 新位置已有配置，无需迁移
	if _, err := os.Stat(newPath); err == nil {
		return
	}
	oldPath := legacySettingsConfPath()
	b, err := os.ReadFile(oldPath)
	if err != nil {
		return // 旧文件不存在或读取失败，无需迁移
	}
	// 校验旧配置内容合法，避免搬运空文件/损坏文件
	var cfg struct {
		SaveDir string `json:"saveDir"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil || cfg.SaveDir == "" || !validSaveDirConfig(cfg.SaveDir) {
		return
	}
	// 写到新位置（先建目录）
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return
	}
	if err := os.WriteFile(newPath, b, 0644); err != nil {
		return
	}
	// 迁移成功，删除旧文件（忽略错误，旧文件残留无害）
	_ = os.Remove(oldPath)
}

// LoadSaveDirConfig 读取已持久化的保存目录；无配置或解析失败时返回空串。
// 配置路径必须是【当前系统】的绝对路径，否则忽略：
//   - Windows 上出现 Unix 风格路径（如 /Users/xxx，没有盘符）→ 忽略，
//     避免在盘符根下创建垃圾目录（Win/Mac 路径本就不同，跨平台同步的配置文件应失效）。
//   - Unix 上出现 Windows 风格路径（C:\...，非以 / 开头）→ 忽略。
func LoadSaveDirConfig() string {
	b, err := os.ReadFile(settingsConfPath())
	if err != nil {
		return ""
	}
	var cfg struct {
		SaveDir string `json:"saveDir"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil || cfg.SaveDir == "" {
		return ""
	}
	if !validSaveDirConfig(cfg.SaveDir) {
		return ""
	}
	return cfg.SaveDir
}

// === 其它配置项（与保存目录共用 aellus-settings.json）===
//
// 「首页引导条是否已被看过」这类一次性标记也写进同一个文件，而不是浏览器 localStorage：
// localStorage 跟着「协议+主机+端口」走，卸载重装应用并不会清掉它，用户会看到「装完第一次
// 见过引导、再装却不出现」的错觉。写在这里则跟随应用数据（飞牛端在 TRIM_PKGVAR 持久卷），
// 卸载时不保留配置就会一并清空，新装后引导条重新出现，符合「首次进入」的语义。
const tipSeenKey = "tipSeen"

// settingsMu 串行化配置文件的读-改-写：保存目录与引导标记共用一个文件，
// 并发写会各自基于旧快照覆盖对方（后写的把先写的字段抹掉）。
var settingsMu sync.Mutex

// readSettingsConfig 读取配置文件为通用 map，供同一文件里的多个配置项共享。
// 文件不存在（首次运行）返回空 map + ok=true，允许写入；
// 读取失败或 JSON 解析失败返回 ok=false，调用方应放弃写入——
// 宁可这次标记没记上（引导条多出现一次），也不能拿残缺内容覆盖文件里已有的设置。
func readSettingsConfig() (map[string]interface{}, bool) {
	b, err := os.ReadFile(settingsConfPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, true
		}
		return nil, false
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	return m, true
}

// writeSettingsConfig 把通用 map 写回配置文件（目录不存在则先建）。
func writeSettingsConfig(m map[string]interface{}) error {
	p := settingsConfPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0644)
}

// LoadTipSeen 读取「首页引导条已被看过」标记；无配置 / 解析失败按未看过处理。
func LoadTipSeen() bool {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	m, ok := readSettingsConfig()
	if !ok {
		return false
	}
	v, _ := m[tipSeenKey].(bool)
	return v
}

// SaveTipSeen 记录「首页引导条已被看过」，之后不再展示。
func SaveTipSeen() error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	m, ok := readSettingsConfig()
	if !ok {
		return errors.New("配置文件读取失败")
	}
	m[tipSeenKey] = true
	return writeSettingsConfig(m)
}

// validSaveDirConfig 判断保存目录配置是否为当前系统合法的绝对路径。
func validSaveDirConfig(p string) bool {
	if runtime.GOOS == "windows" {
		// Windows 绝对路径必须有盘符或 UNC 前缀；"/Users/xxx" 这类路径
		// 虽被 filepath.IsAbs 视为当前盘根，但属于其他系统的路径，拒绝。
		return filepath.IsAbs(p) && filepath.VolumeName(p) != ""
	}
	// Unix：必须以 / 开头（"C:\..." 之类的自然被拒）
	return filepath.IsAbs(p)
}

// SaveSaveDirConfig 把保存目录写回配置文件（桌面端 / 飞牛端持久化用）。
// 读-改-写：文件里可能还有引导标记等其它字段，整份覆盖会把它们抹掉。
func SaveSaveDirConfig(dir string) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	// 这里与 SaveTipSeen 相反：读取失败时仍然写入——本次调用明确要设置保存目录，
	// 旧的（无法解析的）内容本就是无效配置，不值得为保留它而放弃这次保存。
	m, ok := readSettingsConfig()
	if !ok {
		m = map[string]interface{}{}
	}
	m["saveDir"] = dir
	return writeSettingsConfig(m)
}
