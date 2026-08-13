//go:build !linux

// 控制台文案：中文版（Windows / macOS / 其他非 Linux 平台）。
// Linux 平台见 messages_linux.go（英文版，因为路由器等嵌入式系统常无中文字体）。
package platform

const (
	BannerTop     = "  +==============================================+"
	BannerTitle   = "  |              Aellus 文件互传                 |"
	BannerVerFmt  = "  |              版本: %-24s|\n"
	BannerBottom  = "  +==============================================+"
	PromptSaveDir = "  文件保存目录（回车默认 %s）: "
	MsgSaveDir    = "  保存目录: %s\n"
	MsgAccessURL  = "  访问地址: http://%s:%d\n"
	MsgLanHint    = "     (同局域网内，浏览器打开上面地址)"
	MsgStarting   = "  启动中... 按 Ctrl+C 停止"
	MsgMkdirFail  = "创建保存目录失败: %v\n"
	MsgTmplFail   = "模板初始化失败: %v\n"
	MsgServerFail = "服务启动失败: %v\n"
	FlagDirUsage  = "文件保存根目录（不填则交互式输入）"
	FlagPortUsage = "服务端口（默认 8000）"
)
