# Aellus · 局域网文件互传

> The Power From AI!
> The Design By Yang Guangqing!
> The Style By Yang Junwen!

> *Aello——希腊神话中哈耳皮埃（Harpyiai）之一，宙斯座下的风暴信使，振翅生风，迅捷无踪。*
> *Aellus 取其「疾风传讯」之意：局域网内，文件如风而至。*

Aellus 是一个轻量的局域网文件互传服务。在电脑（macOS / Windows / Linux）本地启动后，同局域网内的手机或 PC 用浏览器访问，即可上传文件到电脑、或浏览/下载已上传文件。专为测试工程师"测试机截图/录屏传到电脑"的高频场景设计，**零部署、免流量、跨平台**——无需安装任何 App，浏览器即用。

---

## 📸 效果图

### Android

| 首页 | 上传 | 读取 |
|:---:|:---:|:---:|
| <img src="screenshots/android_home.jpg" width="260"> | <img src="screenshots/android_upload.jpg" width="260"> | <img src="screenshots/android_browse.jpg" width="260"> |

### iOS

| 首页 | 上传 | 读取 |
|:---:|:---:|:---:|
| <img src="screenshots/ios_home.png" width="260"> | <img src="screenshots/ios_upload.png" width="260"> | <img src="screenshots/ios_browse.png" width="260"> |

### PC

| 首页 | 上传 | 读取 |
|:---:|:---:|:---:|
| <img src="screenshots/pc_home.png" width="270"> | <img src="screenshots/pc_upload.png" width="270"> | <img src="screenshots/pc_browse.png" width="270"> |

---

## ✨ 功能特性

- **上传文件**：截图 / 录屏 / 拍照，一键发送到电脑
- **读取文件**：浏览已上传的文件列表，支持在线预览与下载
- **二维码扫码访问**：首页访问地址旁提供二维码，本机点击弹窗展示，其他设备扫码即达，免手输地址
- **按设备分目录**：上传时填设备名，文件自动归档到 `aellus-drops/<设备名>/`
- **自动加时间戳**：文件名带精确到毫秒的时间戳，永不覆盖
- **响应式布局**：同时适配 PC 浏览器与手机浏览器，PC 端多列网格、手机端单列卡片
- **图片/视频预览**：上传后即时预览；读取页缩略图可在线播放
- **灯箱预览**：点缩略图或预览按钮弹出全屏灯箱，支持左右切换（‹ › 按钮 / 键盘 ← →）、Esc 关闭，预览中可一键下载当前文件
- **批量下载**：勾选多个文件打包 zip 下载，或一键打包整个目录
- **删除文件 / 目录**：单文件删除、批量删除、灯箱内删除，以及整目录删除（单个 + 批量），删除前二次确认，操作记录日志
- **访问分级**：删除入口仅本机访问可见，远程设备（手机等）访问时自动隐藏，避免误删
- **下载反馈**：所有下载按钮带 loading 动画并在下载中禁用，防止重复点击
- **macOS 菜单栏常驻**：`Aellus.app` 双击后状态栏出现图标，菜单提供「打开页面 / 退出」，启动时弹通知显示文件存储目录与访问地址；切换网络后菜单栏地址**自动刷新**，无需重启
- **访问日志**：自动记录每次访问的来源 IP、浏览器类型与请求路径，写入 `access.log`
- **安全防护**：严格的路径穿越校验，禁止 `/`、`\` 注入，`..` 经 resolve 越界即拒绝；隐藏文件（`.` 开头）不展示且不可访问
- **无需 App**：手机端无需安装任何应用，浏览器即可使用
- **零依赖**：单文件二进制，下载即用，无需安装任何运行时

---

## 🖥 环境要求

| 项 | 要求 |
|----|------|
| 操作系统 | **macOS / Windows / Linux** 均可 |
| 运行时 | **无需任何依赖**——单文件二进制，下载即用 |
| 网络 | 电脑与手机/其他设备在**同一局域网**内 |

### 下载对应平台的产物

| 平台 | 文件 |
|------|------|
| **macOS 应用（推荐）** | `Aellus.app`（双击运行，菜单栏常驻，Intel + Apple Silicon 通用） |
| Windows (x86_64) | `aellus-windows-amd64.exe` |
| Windows (ARM64) | `aellus-windows-arm64.exe` |
| Windows (x86) | `aellus-windows-386.exe` |
| Linux (x86_64) | `aellus-linux-amd64` |
| Linux (ARM64) | `aellus-linux-arm64` |
| Linux (x86) | `aellus-linux-386` |
| Linux (ARM 32位) | `aellus-linux-arm` |
| macOS (Intel) | `aellus-darwin-amd64`（命令行版） |
| macOS (Apple Silicon) | `aellus-darwin-arm64`（命令行版） |

> 二进制体积约 **8 MB**，Linux 版为**静态链接**（`statically linked`），目标机无需任何库。
>
> macOS 提供两种形态：**`Aellus.app`**（菜单栏应用，双击即用）与**命令行版**（终端/服务器场景）。其余平台均为命令行程序。

---

## 🚀 快速开始

### 1. 下载并运行

**macOS（菜单栏应用，推荐）：** 双击 `Aellus.app`，状态栏出现图标即运行成功。点击图标可「打开页面 / 退出」，启动时会弹通知显示访问地址。

**macOS / Linux（命令行版）：**
```bash
chmod +x aellus-darwin-arm64    # 首次赋予执行权限
./aellus-darwin-arm64
```

**Windows：** 双击 `aellus-windows-amd64.exe`，或在命令行运行：
```cmd
aellus-windows-amd64.exe
```

启动后会提示输入文件保存目录（回车即用默认 `~/Desktop/aellus-drops/`），随后输出访问地址，例如：

```
  +==============================================+
  |              Aellus 文件互传                 |
  |              版本: dev                       |
  +==============================================+

  文件保存目录（回车默认 /Users/you/Desktop/aellus-drops）: ↵

  保存目录: /Users/you/Desktop/aellus-drops
  访问地址: http://192.168.1.111:8000
     (同局域网内，浏览器打开上面地址)
  启动中... 按 Ctrl+C 停止
```

> Linux（路由器等嵌入式系统）无中文字体，控制台提示自动切换为英文，避免中文显示为乱码/黑块。

### 2. 访问使用

- **本机**：浏览器打开 `http://localhost:8000`
- **手机/其他设备**：浏览器打开启动时显示的 `http://<电脑的局域网IP>:8000`，或在本机首页点访问栏的二维码图标，手机扫码即达（免手输地址）

首页提供两个入口：
- 📤 **上传文件** → 填设备名 → 选文件 / 拍照 / 录像 → 上传
- 📂 **读取文件** → 选择目录 → 浏览文件 → 下载 / 预览 / 删除

### 命令行参数

```bash
./aellus --dir <保存目录>     # 指定保存目录（跳过交互输入，适合后台运行）
./aellus --port 9000         # 指定端口（默认 8000）
./aellus --dir ~/drops --port 9000   # 组合使用
```

非交互启动（指定 `--dir`）适合用 `nohup` / 系统服务后台运行：
```bash
nohup ./aellus --dir ~/Desktop/aellus-drops --port 8000 > server.log 2>&1 &
```

---

## 📂 目录结构

```
aellus/
├── main.go              # 入口：参数解析 / 交互输入 / 获取IP / 启动服务
├── go.mod               # Go 模块定义
├── build.sh             # 交叉编译脚本（9 架构三端命令行二进制）
├── build-mac-app.sh     # macOS .app 打包（通用二进制 + 图标 + 菜单栏）
├── Makefile             # make build / clean / run / vet
│
├── internal/            # 内部实现（按职责分包）
│   ├── app/             # 核心业务逻辑
│   │   ├── config.go    #   配置：保存目录 / 监听地址 / 端口 / 日志路径
│   │   ├── handlers.go  #   路由与处理：页面 / 上传 / 列表 / 下载 / 批量打包 / 删除
│   │   ├── safepath.go  #   路径穿越校验 + 设备名过滤
│   │   ├── lanip.go     #   局域网 IP 获取（UDP socket 连接法）
│   │   └── logmw.go     #   访问日志中间件 + 操作日志
│   └── platform/        # 平台差异抽象（build tag 隔离）
│       ├── console_windows.go   #   Windows 控制台字体切换（解决中文方框）
│       ├── console_other.go     #   非 Windows 控制台（空实现）
│       ├── messages_other.go    #   控制台文案：中文（Windows / macOS）
│       ├── messages_linux.go    #   控制台文案：英文（Linux，路由器无中文字体）
│       ├── menubar_darwin.go    #   macOS 菜单栏常驻（cgo：状态栏 + 菜单 + 通知 + IP 自动刷新）
│       ├── menubar_stub.go      #   菜单栏占位（非 cgo / 非 macOS）
│       ├── notify_darwin.go     #   macOS 系统通知
│       ├── notify_other.go      #   通知占位（非 macOS）
│       ├── terminal_darwin.go   #   TTY 判断（macOS，ioctl 精确识别终端）
│       └── terminal_other.go    #   TTY 判断（其他平台）
│
├── assets/              # 前端资源（//go:embed 编译进二进制）
│   ├── embed.go         #   embed 指令：打包 templates/ + static/
│   ├── templates/       #   HTML 模板（home / upload / browse）
│   └── static/          # CSS / JS / favicon / 二维码库 / 署名轮播
├── screenshots/         # 三端页面截图（Android / iOS / PC）
└── README.md
```

> 运行时会自动在**可执行文件同目录**生成两类日志（无需额外配置）：
>
> | 日志文件 | 记录内容 |
> |---------|---------|
> | `access.log` | 每次访问的来源 IP / 浏览器 / 请求路径 / 响应状态 |
> | `operation.log` | 上传成功 / 批量打包下载 / 删除文件 |
>
> `access.log` 各字段带中文标注，格式示例：
> ```
> 2026-08-06 16:21:21  访问来源IP: 192.168.1.99  请求方式: GET  请求URL路径: /browse  响应状态: 200  浏览器UA: "Mozilla/5.0 (iPhone...)"
> ```
> `operation.log` 格式示例：
> ```
> 2026-08-10 20:37:29  ✅ op-test | 20260810_203729752_optest.txt | 0.00MB
> 2026-08-10 20:37:30  📦 打包下载 | op-test | 1 个文件
> 2026-08-13 17:50:39  🗑️ 删除 | default | 1 个文件: _delete_test_file.txt
> 2026-08-13 18:02:10  🗑️ 删除目录 | 2 个: iPhone12, vivox300
> ```

### 上传文件落盘位置

默认 `~/Desktop/aellus-drops/`（启动时可自定义）：

```
aellus-drops/
└── <设备名>/
    └── 20260804_112601079_截图.png   # 时间戳_原文件名
```

---

## 🔨 从源码构建

需安装 [Go 1.21+](https://go.dev/dl/)。

### 交叉编译全平台全架构二进制（推荐）

在任意平台（macOS / Linux / Windows）上执行，**一次性产出全部平台全架构命令行产物**：

```bash
./build.sh           # 编译全平台全架构到 dist/
./build.sh clean     # 清理 dist/
```

或用 Make：

```bash
make                 # 等价于 ./build.sh
make windows-arm64   # 单独编译某平台架构
make vet             # 静态检查
make run             # 编译本机版本并运行
```

产物位于 `dist/`：

```
dist/
├── aellus-windows-amd64.exe    Windows x86_64
├── aellus-windows-arm64.exe    Windows ARM64
├── aellus-windows-386.exe      Windows x86
├── aellus-linux-amd64          Linux   x86_64  (静态链接)
├── aellus-linux-arm64          Linux   ARM64   (静态链接)
├── aellus-linux-386            Linux   x86     (静态链接)
├── aellus-linux-arm            Linux   ARM 32  (静态链接)
├── aellus-darwin-amd64         macOS   Intel
└── aellus-darwin-arm64         macOS   Apple Silicon
```

### 打包 macOS 应用（菜单栏常驻）

在 macOS 上执行，产出 `Aellus.app`（通用二进制，Intel + Apple Silicon 双架构，含图标与菜单栏）：

```bash
./build-mac-app.sh
```

产物：

```
dist/
├── Aellus.app                   macOS 菜单栏应用（双击运行）
├── aellus-macos-universal       通用二进制（x86_64 + arm64）
└── Aellus.icns                  应用图标
```

> `.app` 版本通过 cgo 链接 Cocoa 实现菜单栏常驻；命令行版（`build.sh`）为纯静态编译（`CGO_ENABLED=0`），无此依赖。

> ⚠️ 最低支持：Intel（amd64）为 macOS 10.15 Catalina；Apple Silicon（arm64）为 macOS 11.0 Big Sur（Apple Silicon 本身最早也只跑 Big Sur，由 Go 工具链强制）。构建脚本固定用 Go 1.22.12（最后一个支持 10.15 的 Go 版本，经 `GOTOOLCHAIN` 自动下载）并通过 `MACOSX_DEPLOYMENT_TARGET=10.15` 固定最低系统版本，否则在高版本 macOS 上构建会把最低版本写成构建机版本，导致低版本 macOS 报「此版本不能与此版本的 macOS 配合使用」。

### 仅编译本机版本

```bash
go build -o aellus .
./aellus
```

> Go 的交叉编译是原生能力：`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build` 一行命令即可在 macOS 上产出 Windows 二进制，无需安装任何交叉工具链。`CGO_ENABLED=0` 确保纯静态链接，Linux 产物无任何 `.so` 依赖。

---

## 🔒 安全说明

- **路径穿越防护**：所有目录名、文件名参数均经过 `safeSubpath` 校验——禁止 `/`、`\`，`..` 经 resolve 后越界即拒绝，确保路径始终限定在保存目录范围内
- **设备名过滤**：仅保留字母、数字、中文、`-`、`_`，其余字符自动剔除
- **删除可见性分级**：删除入口（文件 / 目录 / 批量删除）仅当访问来源 IP 为本机（loopback 或等于服务 LAN IP）时显示，远程设备访问时隐藏，降低误删风险
- **删除确认**：删除操作前二次确认，且复用同一套路径校验，防止越权删除
- **仅局域网可用**：服务监听 `0.0.0.0` 但不暴露到公网，需在同一局域网内访问
- **无身份认证**：当前版本面向可信局域网环境，未设登录鉴权，请勿在公共网络使用

---

## ❓ 常见问题

**Q：手机/其他设备打不开页面？**
- 确认电脑和设备在同一个局域网内
- 公司网络可能开启了"客户端隔离"，用手机开热点测试验证
- 首次启动若系统弹窗"是否允许接入网络"（防火墙），点**允许**

**Q：换网络后访问地址变了？**
- 局域网 IP 会随网络变化，重启程序即可看到新 IP

**Q：macOS 提示"无法打开，因为无法验证开发者"？**
- macOS Gatekeeper 会拦截未签名程序。解决：
  ```bash
  xattr -d com.apple.quarantine Aellus.app      # .app 版本
  xattr -d com.apple.quarantine aellus-darwin-arm64   # 命令行版
  ```
  或在「系统设置 → 隐私与安全性」中点「仍要打开」。

**Q：macOS 双击 `.app` 后没看到界面？**
- `.app` 是菜单栏应用，不会打开窗口。启动后看屏幕**右上角菜单栏**的图标；首次启动会弹「允许 Aellus 发送通知吗」，点允许后可收到访问地址通知。

**Q：Windows 双击 exe 中文显示为方框？**
- 程序启动时会自动将控制台字体切换为支持中文的新宋体。若仍异常，可手动将 cmd 字体设为「新宋体」，并确保 `chcp 65001` 为 UTF-8 代码页。

**Q：Linux 路由器上中文显示黑块？**
- 嵌入式 Linux（iStoreOS/OpenWrt 等）无中文字体，程序已自动切换为英文提示；网页端（浏览器）不受影响。

**Q：Windows 双击 exe 闪退？**
- 改为在命令行运行：打开 `cmd` 或 PowerShell，`cd` 到 exe 所在目录，运行 `aellus-windows-amd64.exe`，即可看到交互提示和错误信息。

**Q：上传大文件（录屏几百 MB）失败？**
- 程序未设上传大小限制，支持大文件流式上传；若超时请检查网络稳定性

**Q：重启电脑后服务没自动启动？**
- 本服务未配置开机自启，需手动运行。如需自启：macOS 可配置 launchd，Windows 可加入「启动」文件夹或用任务计划程序，Linux 可写 systemd unit

---

## 🧰 技术栈

| 层 | 技术 |
|----|------|
| 运行平台 | macOS / Windows / Linux（同一份代码，Go 交叉编译出三端单文件二进制） |
| 后端 | **Go 1.21+**（标准库 `net/http` + `html/template` + `archive/zip` + `embed`） |
| macOS 菜单栏 | cgo + Cocoa / AppKit（`NSStatusItem` + `UNUserNotificationCenter`，仅 `.app` 打包时启用） |
| 前端 | 原生 HTML5 / CSS3 / JavaScript（无框架，通过 `//go:embed` 打包进二进制） |
| 传输 | HTTP（局域网点对点，不走云端） |
| 依赖 | **零**——单文件二进制，目标机无需安装任何运行时 |

---

## 💬 反馈与建议

如果您有更好的功能想法或改进建议，欢迎提 [Issues](https://github.com/YGQ8988/Aellus/issues)！
