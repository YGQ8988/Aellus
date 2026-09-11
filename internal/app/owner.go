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

// recordDeviceName 记录设备 ID → 设备名映射（供下次自动填充）；同名跳过。
func (a *App) recordDeviceName(devID, name string) {
	if devID == "" || name == "" {
		return
	}
	a.ownerMu.Lock()
	defer a.ownerMu.Unlock()
	m := map[string]string{}
	if b, err := os.ReadFile(a.deviceNamesPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	if m[devID] == name {
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
	a.ownerMu.Lock()
	defer a.ownerMu.Unlock()
	m := map[string]string{}
	if b, err := os.ReadFile(a.deviceNamesPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m[devID]
}
