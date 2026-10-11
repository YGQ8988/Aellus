package app

import (
	"net/http"
)

// 版本信息：由打包脚本通过 ldflags 在编译时注入（见各 build-*.sh）：
//
//	-X 'aellus/internal/app.Version=1.0.4'
//	-X 'aellus/internal/app.BuildTime=2026-09-29 08:30:00'
//
// 本地 go run / 未注入时为占位值。BuildTime 即实际打包时间（每次构建取当前时间）。
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// handleInfo GET /api/info 返回版本号与构建时间（无副作用、无鉴权，任何设备可读）。
// 设置页标题旁展示「v版本号 · 构建时间」，帮助用户核对所装包是否最新。
func (a *App) handleInfo(w http.ResponseWriter, r *http.Request) {
	// 版本号与构建时间只对管理者返回：该接口原先免鉴权，任何局域网设备都能取到，
	// 而「精确构建时间」可用于判断某台设备是否打了某个安全补丁（版本号也能匹配
	// 已知缺陷版本），属于不必要的指纹暴露。设置页的版本徽标本就只在管理者可见的
	// 「设置」里展示（普通用户看到的是「关于 Aellus」，无此徽标），故不影响界面。
	if !a.canManage(r) {
		a.writeJSON(w, http.StatusOK, map[string]interface{}{
			"version":    "",
			"build_time": "",
		})
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":    Version,
		"build_time": BuildTime,
	})
}
