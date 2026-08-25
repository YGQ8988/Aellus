package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
		return filepath.Join(home, "Desktop", SaveDirName)
	}
	return filepath.Join(ResolveBaseDir(), SaveDirName)
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

// SaveSaveDirConfig 把保存目录写回配置文件（桌面端持久化用）。
func SaveSaveDirConfig(dir string) error {
	p := settingsConfPath()
	// 系统配置目录下的 Aellus 子目录可能尚未创建（首次写入），先建目录
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	b, _ := json.Marshal(struct {
		SaveDir string `json:"saveDir"`
	}{dir})
	return os.WriteFile(p, b, 0644)
}
