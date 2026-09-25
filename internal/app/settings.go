package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
)

// handleSettings GET /api/settings 返回当前文件保存路径及是否为默认路径。
//
// 注意：保存目录的【真实内部路径】只对管理者返回。本接口无鉴权，局域网任意设备
// 都能调，把 /vol1/... 这类 NAS 内部路径交出去会辅助目录结构探测——与 /api/authpaths
// 「不向局域网暴露授权目录结构」的设计保持一致。无权限时路径字段留空
// （前端本就不对无权限者展示「文件保存路径」模块，行为不变）。
func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	cur := a.getSaveDir()
	manage := a.canManage(r)
	saveDir := ""
	saveDirDisplay := ""
	isDefault := false
	if manage {
		saveDir = cur
		saveDirDisplay = cur
		// 飞牛环境：把内部路径 /vol1/... 转成语义化展示路径（如「存储空间1/admin 的文件/photo」）。
		if a.platform.EnforceAuthBoundary() {
			if m := trimConvertPaths([]string{cur}); m[cur] != "" {
				saveDirDisplay = m[cur]
			}
		}
		isDefault = filepath.Clean(cur) == filepath.Clean(bootDefaultSaveDir())
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"saveDir":        saveDir,
		"saveDirDisplay": saveDirDisplay,
		"isDefault":      isDefault,
		"hasTrim":        a.platform.EnforceAuthBoundary(), // 是否飞牛环境（fpk 构建）：前端据此显隐飞牛授权目录等模块
		"canManage":      manage,                           // 是否有删除/修改保存目录权限（飞牛应用按网关注入的 X-Trim-Userid、非飞牛按本机 IP）：前端据此显隐「文件保存路径」模块
		"deviceName":     a.deviceNameOf(deviceID(r)),      // 当前设备 ID 对应的上次设备名（供上传页自动填充）
	})
}

// handleAuthPaths GET /api/authpaths 返回飞牛授权给应用的共享目录（供网页选择保存位置）。
// 优先通过飞牛官方后端 API（trim.file.getSharedAccessibleFolders，Unix socket + TRIM_API_TOKEN）
// 查询管理员在应用设置中授权的目录；非飞牛环境回退到环境变量。
func (a *App) handleAuthPaths(w http.ResponseWriter, r *http.Request) {
	// 该接口只服务「文件保存路径」设置面板（仅管理者可见），故要求管理权限：
	// 局域网设备即使直接调接口也只能拿到 403，避免暴露 NAS 的授权目录结构。
	if !a.canManage(r) {
		a.writeJSON(w, http.StatusForbidden, map[string]interface{}{"error": "无权限"})
		return
	}
	paths := authorizedSavePaths()
	labels := map[string]string{}
	// 飞牛环境：把授权目录内部路径转成语义化展示路径，供设置页面下拉展示。
	if a.platform.EnforceAuthBoundary() {
		labels = trimConvertPaths(paths)
	}
	// 出厂默认目录（应用私有数据目录）不在授权列表里，但服务端允许随时切回它，
	// 故单独返回给前端，由前端始终并入下拉选项（见 home.html renderAuthPaths）。
	def := bootDefaultSaveDir()
	defLabel := def
	if a.platform.EnforceAuthBoundary() && def != "" {
		if m := trimConvertPaths([]string{def}); m[def] != "" {
			defLabel = "默认目录 · " + m[def]
		} else {
			defLabel = "默认目录 · " + def
		}
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"paths":        paths,
		"pathLabels":   labels,
		"hasTrim":      a.platform.EnforceAuthBoundary(),
		"defaultPath":  def,
		"defaultLabel": defLabel,
	})
}

// handleListDir GET /api/listdir?path=... 列出**授权根目录内**的子文件夹，供网页浏览选择保存目录。
// 授权根来自 authorizedSavePaths()（官方 API trim.file.getSharedAccessibleFolders 查询结果，
// 失败回退环境变量）。应用对授权目录本身拥有访问权限，此处仅做普通目录列举；
// 只能在这些授权根内部导航，无法跳出授权边界（越权返回 403）。
func (a *App) handleListDir(w http.ResponseWriter, r *http.Request) {
	// 同 handleAuthPaths：只给管理者用（列授权目录内的子目录），局域网设备 403。
	if !a.canManage(r) {
		a.writeJSON(w, http.StatusForbidden, map[string]interface{}{"error": "无权限"})
		return
	}
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
	// name 来自请求体（浏览器目录选择器的返回值），必须先按「单段合法名」校验：
	// 少了这一步，"../../.." 会被 filepath.Join 清洗成上层任意目录，而桌面端
	// 没有授权边界校验，保存目录就能被指到用户主目录等位置——之后局域网设备
	// 通过浏览/下载即可读到那里的文件。
	if !isValidName(name) {
		return ""
	}
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
// 权限与删除逻辑一致（见 canManage）：飞牛端要求请求携带网关注入的身份头（即从门户内
// 发起，登录态已由飞牛网关校验）；桌面端要求来自本机；局域网设备经 IP:端口 直连一律拒绝。
//
// 平台差异（由 Platform.EnforceAuthBoundary / PersistSaveDirAllowed 控制）：
//   - 桌面端：可存任意绝对路径，并持久化到 aellus-settings.json（用户自主决定落盘位置）。
//   - fpk 端：保存目录【完全由飞牛授权做主】——dir 必须落在飞牛授权目录树内
//     （authorizedSavePaths，含授权根本身及其子树），否则拒绝；同样持久化到本地配置
//     （TRIM_PKGVAR 持久卷），重启后继续使用，但加载时会再次校验是否仍在授权目录内，
//     授权被移除则回退到飞牛启动脚本注入的 AELLUS_SAVE_DIR。
func (a *App) handleSetSaveDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "仅支持 POST"})
		return
	}
	// 修改保存路径权限与删除一致（见 canManage）：
	// 飞牛端需经门户（网关注入身份头）修改，桌面端需来自本机；局域网设备直连一律拒绝。
	if !a.canManage(r) {
		a.writeJSON(w, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "无权限修改保存路径"})
		return
	}
	var req struct {
		Dir string `json:"dir"`
	}
	// 限长读取：与其它 POST 接口一致，避免超大请求体撑内存。
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
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

	// fpk 端：用户主动选择了新目录时，强制保存目录落在「允许的根」内
	// （飞牛授权目录 + 出厂默认目录，见 saveDirAllowedRoots）。
	if a.platform.EnforceAuthBoundary() && !restoreDefault {
		if len(authorizedSavePaths()) == 0 && bootDefaultSaveDir() == "" {
			a.writeJSON(w, http.StatusForbidden, map[string]interface{}{
				"ok":    false,
				"error": "当前未获取到飞牛授权目录，无法设置保存目录",
			})
			return
		}
		// withinAuthRoots 会逐根比对（字符串前缀 + symlink 真实路径增强）：
		// dir 可能尚不存在，realInside 会解析其存在的最深父目录再比对，防软链逃逸。
		// 注意不要在这里额外单判 roots[0]——授权多个目录时那样会误拒其它授权根下的目录。
		if !withinAuthRoots(dir, saveDirAllowedRoots()) {
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
	// MkdirAll 后再次确认真实落点仍在允许的根内（防「待建目录经软链指向上级」的 TOCTOU 变种）。
	// 同样逐根比对，避免只判 roots[0] 造成的误拒。
	if a.platform.EnforceAuthBoundary() && !restoreDefault {
		if roots := saveDirAllowedRoots(); len(roots) > 0 && !withinAuthRoots(dir, roots) {
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
	// 持久化保存目录到本地配置（桌面端 + 飞牛端都持久化）；飞牛端写到 TRIM_PKGVAR 持久卷，
	// 重启加载时再校验是否仍在授权目录内（见 main 的 IsPersistedSaveDirValid）。
	if a.platform.PersistSaveDirAllowed() {
		_ = SaveSaveDirConfig(dir)
	}
	saveDirDisplay := dir
	// 飞牛环境：返回语义化展示路径，供设置页面展示（避免暴露 /vol1/... 内部路径）。
	if a.platform.EnforceAuthBoundary() {
		if m := trimConvertPaths([]string{dir}); m[dir] != "" {
			saveDirDisplay = m[dir]
		}
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "saveDir": dir, "saveDirDisplay": saveDirDisplay})
}

// handlePickDir 调用系统原生"选取文件夹"对话框，返回用户选择的绝对路径。
// 仅本机来源可调用（见 isLocalRequest）；fpk 端无本机浏览器，不支持目录选择，返回 501。
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
