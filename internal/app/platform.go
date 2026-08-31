package app

// Platform 隔离所有平台/构建差异（桌面端 macOS/Windows vs 飞牛 fnOS fpk 后台服务）。
//
// app 包本身零 build-tag、纯业务逻辑；所有「因平台而异」的能力都通过本接口注入，
// 由 package main 的 platformImpl（!fpk）/ fpkPlatform（fpk）提供具体实现。
// 这样避免了「build-tag 替换的符号与业务符号互相调用」导致的循环依赖。
type Platform interface {
	// RunTray 进入托盘/菜单栏消息循环（阻塞，直到用户退出）。
	// fpk 端无桌面，等价于「等待 SIGINT/SIGTERM」。
	RunTray(url string)

	// PostOpenNotification 弹一条系统通知（macOS/Windows）。
	// fpk 端无桌面通知，空实现。
	PostOpenNotification(title, body, url string)

	// EnforceSingleInstance 单实例检查；返回 false 表示已有实例在运行，调用方应退出。
	// fpk 端由 fnOS/systemd 保证单实例，直接返回 true。
	EnforceSingleInstance() bool

	// PickFolderDialog 弹出系统原生「选择文件夹」对话框，返回选中的绝对路径；用户取消返回空串。
	// PickDirSupported 返回 false 时（fpk 端）不应调用 PickFolderDialog。
	PickFolderDialog() string
	PickDirSupported() bool

	// PersistSaveDirAllowed 桌面端允许把保存目录持久化到本地配置文件（true）；
	// fpk 端不允许（false，路径完全由飞牛授权决定，重启回到注入值）。
	PersistSaveDirAllowed() bool

	// EnforceAuthBoundary fpk 端强制「保存目录必须落在飞牛授权目录树内」（true）；
	// 桌面端无此限制（false，用户自主决定落盘位置）。
	EnforceAuthBoundary() bool

	// OwnersBaseDir 返回归属 manifest 的存放根目录。
	//   - 桌面端：系统配置目录下的 owners/ 子目录（与 aellus-settings.json 同级），不污染保存目录；
	//   - fpk 端：飞牛私有运行时数据目录（TRIM_PKGVAR/owners），不污染用户共享目录。
	OwnersBaseDir(saveDir string) string

	// LogsDir 返回访问日志 / 操作日志的存放目录。
	//   - 桌面端：系统配置目录下的 logs/ 子目录（与 settings/owners 一致），不污染 .app 同级目录；
	//   - fpk 端：飞牛私有运行时数据目录（TRIM_PKGVAR/logs），不写入应用安装目录。
	LogsDir() string
}
