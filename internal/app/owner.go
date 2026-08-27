package app

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// === 设备归属（owner）===
//
// 记录“某目录下每个直接子项（文件/子目录）由哪个设备上传”的映射，实现文件级归属：
// 只锁定本设备上传的那一项，其他设备上传的项不受影响。
// 规则：项的上传设备与本设备一致（或旧数据无记录）才可删；其他设备上传的项仅可下载。
//
// 存储位置由 Platform.OwnersBaseDir 决定（构建标签隔离）：
//   - 桌面端：集中写到系统配置目录下的 owners/ 子目录（~/Library/Application Support/Aellus/owners/
//     或 %APPDATA%\Aellus\owners\），不再散落在用户保存目录里。
//   - fpk 端：集中写到飞牛私有运行时数据目录（TRIM_PKGVAR/owners，经 AELLUS_OWNERS_DIR 注入），
//     不在用户共享目录散落隐藏文件。
//
// 兼容迁移：readOwners 在新位置找不到时，依次回退到旧版散落格式（saveDir/sha1.json、
// dir/.aellus-owners），读到后写回新集中位置并删旧文件；启动时 MigrateLegacyOwners
// 主动扫描搬运，两层保证历史归属数据无缝衔接。

// encodeDir 把目录绝对路径编码为安全的文件名（路径含 / 不能直接当文件名）。
// 用 SHA1 hex，足够唯一且不可逆需求无关（仅需稳定映射）。纯函数。
func encodeDir(dir string) string {
	sum := sha1.Sum([]byte(dir))
	return hex.EncodeToString(sum[:]) + ".json"
}

// deviceSigFromUA 从 User-Agent 提取“设备签名”：去掉浏览器标识（Safari/Edg/Chrome 等），
// 只保留设备类别/系统/型号，使【同一设备的不同浏览器】签名一致、不同设备尽量不同。
// 返回带 "ua:" 前缀的字符串；不带前缀的旧归属（随机 deviceId）视为兼容旧数据、不参与限制。纯函数。
func deviceSigFromUA(ua string) string {
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "iphone"):
		return "ua:iphone-ios"
	case strings.Contains(lower, "ipad"):
		return "ua:ipad-ios"
	case strings.Contains(lower, "ipod"):
		return "ua:ipod-ios"
	case strings.Contains(lower, "macintosh"):
		return "ua:mac-osx"
	case strings.Contains(lower, "windows nt"):
		return "ua:windows-nt"
	case strings.Contains(lower, "android"):
		// UA 形如 "Mozilla/5.0 (Linux; Android 13; Pixel 7 Build/TQ3A...) AppleWebKit/..."
		// 取 "android 13" 段与紧跟的 "; 型号" 段（截断 build/），如 android-13-pixel7
		sig := "android"
		if i := strings.Index(lower, "android"); i >= 0 {
			rest := lower[i:]
			segs := strings.SplitN(rest, ";", 3)
			ver := strings.TrimSpace(strings.TrimPrefix(segs[0], "android"))
			if ver != "" {
				sig += "-" + strings.ReplaceAll(ver, " ", "")
			}
			if len(segs) > 1 {
				model := strings.TrimSpace(segs[1])
				if k := strings.Index(model, "build/"); k > 0 {
					model = strings.TrimSpace(model[:k])
				}
				model = strings.ReplaceAll(model, " ", "")
				if model != "" {
					sig += "-" + model
				}
			}
		}
		return "ua:" + sanitizeDevice(sig)
	default:
		return "ua:unknown"
	}
}

// ownersFilePath 返回 dir 对应的 manifest 文件路径（位于 OwnersBaseDir 下，
// 文件名由目录路径编码得到，集中存放、不再散落在各共享目录里）。
func (a *App) ownersFilePath(dir string) string {
	return filepath.Join(a.platform.OwnersBaseDir(a.getSaveDir()), encodeDir(dir))
}

// readOwners 读取 dir 的归属 manifest，返回 子项名 -> 设备ID；无 manifest 返回空 map。
// 兼容迁移：新集中位置缺失时，依次回退到旧版散落格式（saveDir/sha1.json、dir/.aellus-owners），
// 读到后写回新集中位置、删除旧文件，使历史归属数据无缝衔接且不继续污染共享目录。
func (a *App) readOwners(dir string) map[string]string {
	m := map[string]string{}
	path := a.ownersFilePath(dir)
	b, err := os.ReadFile(path)
	if err != nil {
		// 兼容迁移1：旧版桌面端把 manifest 散落在 saveDir 里（saveDir/sha1(dir).json）
		saveDir := a.getSaveDir()
		if saveDir != "" {
			legacy := filepath.Join(saveDir, encodeDir(dir))
			if lb, lerr := os.ReadFile(legacy); lerr == nil {
				json.Unmarshal(lb, &m)
				if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr == nil {
					if wb, _ := json.Marshal(m); wb != nil {
						os.WriteFile(path, wb, 0644)
					}
				}
				os.Remove(legacy)
				return m
			}
		}
		// 兼容迁移2：更早版本用 .aellus-owners 单文件散落在 dir 下
		old := filepath.Join(dir, ownersFile)
		ob, oerr := os.ReadFile(old)
		if oerr != nil {
			return m
		}
		json.Unmarshal(ob, &m)
		if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr == nil {
			if wb, _ := json.Marshal(m); wb != nil {
				os.WriteFile(path, wb, 0644)
			}
		}
		os.Remove(old)
		return m
	}
	json.Unmarshal(b, &m)
	return m
}

// writeOwners 写回 manifest（内部调用，调用方需先持 a.ownerMu 锁）。
func (a *App) writeOwners(dir string, m map[string]string) {
	path := a.ownersFilePath(dir)
	if dirErr := os.MkdirAll(filepath.Dir(path), 0755); dirErr != nil {
		return
	}
	b, _ := json.Marshal(m)
	os.WriteFile(path, b, 0644)
}

// recordOwner 在 dir 的 manifest 中记录子项 name 的上传来源（客户端 IP + UA 设备签名）；
// 已有其他来源记录时不覆盖（首个上传者为准）。
func (a *App) recordOwner(dir, name, ip, ua string) {
	if ip == "" && ua == "" || name == "" || strings.HasPrefix(name, ".") {
		return
	}
	a.ownerMu.Lock()
	defer a.ownerMu.Unlock()
	m := a.readOwners(dir)
	if _, ok := m[name]; ok {
		return // 已有归属（无论谁），不覆盖
	}
	b, _ := json.Marshal(map[string]string{"ip": ip, "ua": ua})
	m[name] = string(b)
	a.writeOwners(dir, m)
}

// ownerOf 返回 dir 下子项 name 的归属记录（原始存储值）；无记录返回空串。
func (a *App) ownerOf(dir, name string) string {
	if name == "" || strings.HasPrefix(name, ".") {
		return ""
	}
	a.ownerMu.Lock()
	defer a.ownerMu.Unlock()
	return a.readOwners(dir)[name]
}

// deletable 判断某项是否可由当前请求删除：归属为空（旧数据）或
// 请求 IP 与归属 IP 相同（同一设备换任何浏览器）或 UA 签名相同（IP 变化后兜底）→ 可删。
// 旧版纯 "ua:" 前缀归属按签名比对；更早的随机 ID 归属视为旧数据放行。纯函数。
func deletable(owner, ip, ua string) bool {
	if owner == "" {
		return true
	}
	var o struct {
		IP string `json:"ip"`
		UA string `json:"ua"`
	}
	if json.Unmarshal([]byte(owner), &o) == nil && (o.IP != "" || o.UA != "") {
		return o.IP == ip || o.UA == ua
	}
	if strings.HasPrefix(owner, "ua:") {
		return owner == ua
	}
	return true
}

// removeOwner 从 dir 的 manifest 中移除子项 name（删除文件/目录后同步清理，保持 manifest 干净）。
func (a *App) removeOwner(dir, name string) {
	if name == "" || strings.HasPrefix(name, ".") {
		return
	}
	a.ownerMu.Lock()
	defer a.ownerMu.Unlock()
	m := a.readOwners(dir)
	if _, ok := m[name]; !ok {
		return
	}
	delete(m, name)
	a.writeOwners(dir, m)
}

// MigrateLegacyOwners 一次性迁移：把旧版桌面端散落在 saveDir 里的归属 manifest
// （<sha1>.json）搬到集中目录（OwnersBaseDir），保留历史归属数据并清理旧位置。
//
// 只搬文件名精确匹配 saveDir 及其直接子目录 sha1 的文件（manifest 的 dir 参数只可能是
// 这两类），零误扫用户上传的文件。迁移失败不阻断启动。仅桌面端调用。
func MigrateLegacyOwners(saveDir, newBaseDir string) {
	if saveDir == "" || newBaseDir == "" || saveDir == newBaseDir {
		return
	}
	// 收集所有可能的 manifest 对应目录：saveDir 本身 + 其直接子目录
	dirs := []string{saveDir}
	if entries, err := os.ReadDir(saveDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, filepath.Join(saveDir, e.Name()))
			}
		}
	}
	if err := os.MkdirAll(newBaseDir, 0755); err != nil {
		return
	}
	for _, dir := range dirs {
		name := encodeDir(dir)
		oldPath := filepath.Join(saveDir, name)
		newPath := filepath.Join(newBaseDir, name)
		b, err := os.ReadFile(oldPath)
		if err != nil {
			continue // 旧位置无此 manifest，跳过
		}
		// 新位置已存在则不覆盖，直接删旧文件
		if _, err := os.Stat(newPath); err == nil {
			os.Remove(oldPath)
			continue
		}
		if err := os.WriteFile(newPath, b, 0644); err != nil {
			continue
		}
		os.Remove(oldPath)
	}
}
