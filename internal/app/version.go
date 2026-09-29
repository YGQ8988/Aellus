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
	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":    Version,
		"build_time": BuildTime,
	})
}
