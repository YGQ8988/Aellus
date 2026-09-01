package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
)

// handleSettings GET /api/settings 返回当前文件保存路径及是否为默认路径。
func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	cur := a.getSaveDir()
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"saveDir":        cur,
		"isDefault":      filepath.Clean(cur) == filepath.Clean(bootDefaultSaveDir()),
		"persistSaveDir": a.platform.PersistSaveDirAllowed(), // fpk 端为 false：路径不持久化，重启回到飞牛注入值
		"hasTrim":        hasTrimAPI(),                       // fpk（飞牛）环境标识：前端据此隐藏「默认保存在电脑桌面」等桌面专属文案
		"isLocal":        isLocalRequest(r),                  // 访问来源 IP 是否等于服务 IP（本机访问）：前端据此显隐「文件保存路径」模块
		"deviceName":     a.deviceNameOf(deviceID(r)),        // 当前设备 ID 对应的上次设备名（供上传页自动填充）
	})
}

// handleAuthPaths GET /api/authpaths 返回飞牛授权给应用的共享目录（供网页选择保存位置）。
// 优先通过飞牛官方后端 API（trim.file.getSharedAccessibleFolders，Unix socket + TRIM_API_TOKEN）
// 查询管理员在应用设置中授权的目录；非飞牛环境回退到环境变量。
func (a *App) handleAuthPaths(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"paths":   authorizedSavePaths(),
		"hasTrim": hasTrimAPI(),
	})
}

// handleListDir GET /api/listdir?path=... 列出**授权根目录内**的子文件夹，供网页浏览选择保存目录。
// 授权根来自 authorizedSavePaths()（官方 API trim.file.getSharedAccessibleFolders 查询结果，
// 失败回退环境变量）。应用对授权目录本身拥有访问权限，此处仅做普通目录列举；
// 只能在这些授权根内部导航，无法跳出授权边界（越权返回 403）。
func (a *App) handleListDir(w http.ResponseWriter, r *http.Request) {
	roots := authorizedSavePaths()
	if len(roots) == 0 {
		a.writeJSON(w, http.StatusOK, map[string]interface{}{
			"roots": []string{}, "current": "", "dirs": []string{}, "parent": "",
		})
		return
	}
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		p = roots[0]
	}
	if !withinAuthRoots(p, roots) {
		a.writeJSON(w, http.StatusForbidden, map[string]interface{}{"error": "路径不在授权目录内"})
		return
	}
	dirs := []string{}
	if entries, err := os.ReadDir(p); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				cand := filepath.Join(p, e.Name())
				// symlink 增强：跳过指向授权目录外的软链子目录，避免泄露外部路径。
				if realInside(p, cand) {
					dirs = append(dirs, cand)
				}
			}
		}
	}
	sortStrings(dirs)
	parent := filepath.Dir(p)
	if parent == p || !withinAuthRoots(parent, roots) {
		parent = ""
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"roots": roots, "current": p, "dirs": dirs, "parent": parent,
	})
}

// sortStrings 对字符串切片做原地升序排序（从 handleListDir 调用，等价于 sort.Strings）。
func sortStrings(s []string) {
	// 用简单的插入排序避免引入 sort 包到本文件（handlers.go 已引入 sort，
	// 但保持 settings.go 依赖最小化）。
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// resolvePickedDir 尝试将浏览器目录选择器返回的目录名解析为完整路径。
// 按优先级依次在：当前保存目录的父级、用户主目录、桌面、Documents 中查找同名子目录。
// 桌面端（平台 PickFolderDialog）与 fpk 端共用：handleSetSaveDir 在两种构建下都会调用它。
func (a *App) resolvePickedDir(name string) string {
	candidates := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		current := a.getSaveDir()
		parent := filepath.Dir(current)
		if parent != current { // 避免根目录情况
			candidates = append(candidates, filepath.Join(parent, name))
		}
		candidates = append(candidates,
			filepath.Join(home, name),
			filepath.Join(home, "Desktop", name),
			filepath.Join(home, "Documents", name),
			filepath.Join(home, "Downloads", name),
		)
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	// 都找不到时，默认放到桌面下
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Desktop", name)
	}
	return ""
}

// handleSetSaveDir POST /api/set-savedir 修改文件保存路径。
// body: {"dir": "/abs/path"}；dir 为空则恢复默认（桌面 file-drops / fnOS 授权共享目录）。
// 注意：允许局域网任意设备修改保存路径（如手机点选飞牛授权目录），不做"仅本机"限制。
//
// 平台差异（由 Platform.EnforceAuthBoundary / PersistSaveDirAllowed 控制）：
//   - 桌面端：可存任意绝对路径，并持久化到 aellus-settings.json（用户自主决定落盘位置）。
//   - fpk 端：保存目录【完全由飞牛授权做主】——dir 必须落在飞牛授权目录树内
//     （authorizedSavePaths，含授权根本身及其子树），否则拒绝；且【不】持久化到本地
//     文件，重启后回到飞牛启动脚本注入的 AELLUS_SAVE_DIR，避免污染授权语义。
func (a *App) handleSetSaveDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "仅支持 POST"})
		return
	}
	// 桌面端（非飞牛）要求来源 IP 等于服务 IP（仅本机可改），防止局域网任意设备篡改落盘位置。
	// 飞牛端不限来源 IP——保存路径必须落在飞牛授权目录树内（EnforceAuthBoundary + 授权校验），
	// 攻击者最多把路径改成另一个已授权目录，危害有限；飞牛端远程访问可在授权范围内选目录。
	if !a.platform.EnforceAuthBoundary() && !isLocalRequest(r) {
		a.writeJSON(w, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "仅本机可修改保存路径"})
		return
	}
	var req struct {
		Dir string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "请求格式错误"})
		return
	}
	restoreDefault := strings.TrimSpace(req.Dir) == ""
	var dir string
	if restoreDefault {
		// 恢复默认：fnOS 下回到 AELLUS_SAVE_DIR（cmd/main 注入的授权/共享目录），
		// 其他平台回到桌面 file-drops。不能用纯 resolveSaveDir()，否则 fnOS 会被错误地
		// 改写成 ~/Desktop/file-drops（不在授权列表里，前端也无法在下拉中选中）。
		// 该值由飞牛系统分配（授权共享目录或应用私有数据目录），本身是合法落盘位置，
		// 不进入下面的授权校验，避免「注入目录不在用户勾选的授权树内」误报。
		dir = bootDefaultSaveDir()
	} else {
		dir = strings.TrimSpace(req.Dir)
		if !filepath.IsAbs(dir) {
			// 浏览器目录选择器只能拿到目录名，尝试在常见位置查找
			dir = a.resolvePickedDir(dir)
			if dir == "" {
				a.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "无法定位该目录，请输入绝对路径"})
				return
			}
		}
	}

	// fpk 端：用户主动选择了新目录时，强制保存目录落在飞牛授权目录树内（含授权根及其子树）。
	// 恢复默认（restoreDefault）不校验——飞牛注入的默认目录即系统分配的合法落盘位置。
	if a.platform.EnforceAuthBoundary() && !restoreDefault {
		roots := authorizedSavePaths()
		if len(roots) == 0 {
			a.writeJSON(w, http.StatusForbidden, map[string]interface{}{
				"ok":    false,
				"error": "当前未获取到飞牛授权目录，无法设置保存目录",
			})
			return
		}
		// 先用字符串前缀快速拒绝明显越界，再用 realInside 做 symlink 增强校验
		// （dir 可能尚不存在，realInside 会解析其父目录再比对，防软链逃逸）。
		if !withinAuthRoots(dir, roots) && !realInside(roots[0], dir) {
			a.writeJSON(w, http.StatusForbidden, map[string]interface{}{
				"ok":    false,
				"error": "保存目录必须位于飞牛已授权目录内（当前不在授权范围）",
			})
			return
		}
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "无法创建目录：" + err.Error()})
		return
	}
	// MkdirAll 后再次确认真实落点仍在授权树内（防「待建目录经软链指向上级」的 TOCTOU 变种）。
	if a.platform.EnforceAuthBoundary() && !restoreDefault {
		roots := authorizedSavePaths()
		if len(roots) > 0 && !realInside(roots[0], dir) {
			a.writeJSON(w, http.StatusForbidden, map[string]interface{}{
				"ok":    false,
				"error": "保存目录必须位于飞牛已授权目录内（当前不在授权范围）",
			})
			return
		}
	}
	// 可写性校验
	test := filepath.Join(dir, ".aellus-write-test")
	if err := os.WriteFile(test, []byte("ok"), 0644); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "目录不可写：" + err.Error()})
		return
	}
	os.Remove(test)
	a.setSaveDir(dir)
	// 仅桌面端持久化到本地配置文件；fpk 端不持久化（路径由飞牛授权决定，重启回注入值）。
	if a.platform.PersistSaveDirAllowed() {
		_ = SaveSaveDirConfig(dir)
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "saveDir": dir})
}

// handlePickDir 调用系统原生"选取文件夹"对话框，返回用户选择的绝对路径。
// 仅在本机（localhost / 127.0.0.1）访问时可用；fpk 端不支持本地目录选择，返回 501。
func (a *App) handlePickDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "仅支持 POST"})
		return
	}
	// fpk（NAS 后台服务）不存在本机浏览器来触发目录选择，返回 501。
	if !a.platform.PickDirSupported() {
		a.writeJSON(w, http.StatusNotImplemented, map[string]interface{}{
			"ok":    false,
			"error": "此功能仅在桌面端可用（NAS 后台服务不支持本地目录选择）",
		})
		return
	}
	if !isLocalRequest(r) {
		a.writeJSON(w, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "仅本机可调用"})
		return
	}
	dir := a.platform.PickFolderDialog()
	if dir == "" {
		// 用户取消或对话框出错：返回 cancelled
		a.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "cancelled": true})
		return
	}
	// 直接复用 set-savedir 的校验与持久化逻辑
	setReq := struct {
		Dir string `json:"dir"`
	}{Dir: dir}
	body, _ := json.Marshal(setReq)
	req, _ := http.NewRequest(http.MethodPost, "/api/set-savedir", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Host = r.Host
	req.RemoteAddr = r.RemoteAddr // 透传真实客户端 IP，供内部 isLocalRequest 判定
	rr := httptest.NewRecorder()
	a.handleSetSaveDir(rr, req)
	var resp struct {
		OK      bool   `json:"ok"`
		SaveDir string `json:"saveDir"`
		Error   string `json:"error"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.OK {
		a.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "saveDir": resp.SaveDir})
	} else {
		a.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": resp.Error})
	}
}
