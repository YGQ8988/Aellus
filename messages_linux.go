//go:build linux

// 控制台文案：英文版（Linux）。
// 路由器等嵌入式 Linux（iStoreOS/OpenWrt）常无中文字体，中文会显示为黑块，
// 故 Linux 平台统一用 ASCII 英文提示，避免乱码。
package main

const (
	bannerTop     = "  +==============================================+"
	bannerTitle   = "  |             Aellus File Transfer             |"
	bannerVerFmt  = "  |              Version: %-24s|\n"
	bannerBottom  = "  +==============================================+"
	promptSaveDir = "  Save directory (Enter for default %s): "
	msgSaveDir    = "  Save directory: %s\n"
	msgAccessURL  = "  Access URL: http://%s:%d\n"
	msgLanHint    = "     (Open the URL above in a browser on the same LAN)"
	msgStarting   = "  Starting... Press Ctrl+C to stop"
	msgMkdirFail  = "Failed to create save directory: %v\n"
	msgTmplFail   = "Failed to init templates: %v\n"
	msgServerFail = "Failed to start server: %v\n"
	flagDirUsage  = "Save root directory (interactive if empty)"
	flagPortUsage = "Server port (default 8000)"
)
