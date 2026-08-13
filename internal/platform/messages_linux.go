//go:build linux

// 控制台文案：英文版（Linux）。
// 路由器等嵌入式 Linux（iStoreOS/OpenWrt）常无中文字体，中文会显示为黑块，
// 故 Linux 平台统一用 ASCII 英文提示，避免乱码。
package platform

const (
	BannerTop     = "  +==============================================+"
	BannerTitle   = "  |             Aellus File Transfer             |"
	BannerVerFmt  = "  |              Version: %-24s|\n"
	BannerBottom  = "  +==============================================+"
	PromptSaveDir = "  Save directory (Enter for default %s): "
	MsgSaveDir    = "  Save directory: %s\n"
	MsgAccessURL  = "  Access URL: http://%s:%d\n"
	MsgLanHint    = "     (Open the URL above in a browser on the same LAN)"
	MsgStarting   = "  Starting... Press Ctrl+C to stop"
	MsgMkdirFail  = "Failed to create save directory: %v\n"
	MsgTmplFail   = "Failed to init templates: %v\n"
	MsgServerFail = "Failed to start server: %v\n"
	FlagDirUsage  = "Save root directory (interactive if empty)"
	FlagPortUsage = "Server port (default 8000)"
)
