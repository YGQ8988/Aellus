package app

import (
	"net/http"
)

// 版本信息：由打包脚本通过 ldflags 在编译时注入（见各 build-*.sh）：
//
//	-X 'aellus/internal/app.Version=1.0.4'
//	-X 'aellus/internal/app.BuildTime=2026-09-29 08:30:00'
//	-X 'aellus/internal/app.BuildStaticHash=<sha256>'
//
// 本地 go run / 未注入时为占位值。BuildTime 即实际打包时间（每次构建取当前时间）。
//
// BuildStaticHash 是 templates/ + static/ 全部文件的内容指纹（算法见
// tools/static_hash.sh），同样在编译时注入。它存在的唯一目的：给发布前体检
// （tools/embed_check.py）提供「这份二进制是用哪一版前端编出来的」的凭据 ——
// 只比对文件名清单发现不了「模板内容改了但二进制没重编」，那正是两次
// 「版本号涨了功能没变」事故的实际形态。未注入（本地 go run）时为空串。
var (
	Version         = "dev"
	BuildTime       = "unknown"
	BuildStaticHash = ""
)

// init 里引用一次 BuildStaticHash：没有任何代码用到的变量会被链接器的死代码消除
// 裁掉，-X 注入就落空了（指纹根本不进二进制，体检永远失败）。init 的引用让链接器
// 必须保留它；条件恒假的比较让运行时零开销。
func init() {
	if BuildStaticHash != "" && len(BuildStaticHash) != 64 {
		BuildStaticHash = ""
	}
}

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
