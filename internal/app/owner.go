package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// === 设备名映射（device name）===
// 记录「设备 ID → 用户输入的设备名」映射，存到配置数据根目录下的 devices.json。
// 用途：换浏览器 / 清 localStorage 后，上传页仍能按设备 ID 从服务端取回上次的设备名，避免重复输入。
// 存储位置（Aellus/devices.json 或 TRIM_PKGVAR/devices.json）。

// deviceNamesPath 返回设备名映射配置文件路径（配置数据根目录下）。
func (a *App) deviceNamesPath() string {
	return filepath.Join(a.platform.ConfigBaseDir(a.getSaveDir()), "devices.json")
}

// 设备名映射的容量与长度上限。
// 局域网内任何设备都能上传（设计如此），若不设限，攻击者可用大量随机 Deviceid
// 把 devices.json 无限撑大；超长字符串也应截断后再落盘。
const (
	maxDeviceNames   = 500
	maxDeviceIDLen   = 64
	maxDeviceNameLen = 32
)

// truncateRunes 按字符（rune）截断，避免把中文等多字节字符切坏。
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

// recordDeviceName 记录设备 ID → 设备名映射（供下次自动填充）；同名跳过。
func (a *App) recordDeviceName(devID, name string) {
	if devID == "" || name == "" {
		return
	}
	devID = truncateRunes(devID, maxDeviceIDLen)
	name = truncateRunes(name, maxDeviceNameLen)
	a.ownerMu.Lock()
	defer a.ownerMu.Unlock()
	m := map[string]string{}
	if b, err := os.ReadFile(a.deviceNamesPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	if m[devID] == name {
		return
	}
	// 新增条目时校验容量上限（已存在键的更新不受限）。
	if _, exists := m[devID]; !exists && len(m) >= maxDeviceNames {
		return
	}
	m[devID] = name
	b, _ := json.Marshal(m)
	_ = os.MkdirAll(filepath.Dir(a.deviceNamesPath()), 0755)
	_ = os.WriteFile(a.deviceNamesPath(), b, 0644)
}

// deviceNameOf 返回设备 ID 对应的设备名；无记录返回空串。
func (a *App) deviceNameOf(devID string) string {
	if devID == "" {
		return ""
	}
	devID = truncateRunes(devID, maxDeviceIDLen) // 与写入时的截断保持一致
	a.ownerMu.Lock()
	defer a.ownerMu.Unlock()
	m := map[string]string{}
	if b, err := os.ReadFile(a.deviceNamesPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m[devID]
}
