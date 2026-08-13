# Aellus · 局域网文件互传

> Power comes from AI
> The design comes from Yang Guangqing!
> The style comes from Yang Junwen!

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
- **按设备分目录**：上传时填设备名，文件自动归档到 `aellus-drops/<设备名>/`
- **自动加时间戳**：文件名带精确到毫秒的时间戳，永不覆盖
- **响应式布局**：同时适配 PC 浏览器与手机浏览器，PC 端多列网格、手机端单列卡片
- **图片/视频预览**：上传后即时预览；读取页缩略图可在线播放
- **灯箱预览**：点缩略图或预览按钮弹出全屏灯箱，支持左右切换（‹ › 按钮 / 键盘 ← →）、Esc 关闭，预览中可一键下载当前文件
- **批量下载**：勾选多个文件打包 zip 下载，或一键打包整个目录
- **下载反馈**：所有下载按钮带 loading 动画并在下载中禁用，防止重复点击
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

### 下载对应平台的二进制

| 平台 | 文件 |
|------|------|
| Windows (x86_64) | `aellus-windows-amd64.exe` |
| Windows (ARM64) | `aellus-windows-arm64.exe` |
| Windows (x86) | `aellus-windows-386.exe` |
| Linux (x86_64) | `aellus-linux-amd64` |
| Linux (ARM64) | `aellus-linux-arm64` |
| Linux (x86) | `aellus-linux-386` |
| Linux (ARM 32位) | `aellus-linux-arm` |
| macOS (Intel) | `aellus-darwin-amd64` |
| macOS (Apple Silicon) | `aellus-darwin-arm64` |

> 二进制体积约 **8 MB**，Linux 版为**静态链接**（`statically linked`），目标机无需任何库。

---

## 🚀 快速开始

### 1. 下载并运行

**macOS / Linux：**
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
  ╔══════════════════════════════════════╗
  ║          Aellus 文件互传             ║
  ║          版本: 2a43e00              ║
  ╚══════════════════════════════════════╝

  📁 文件保存目录（回车默认 /Users/you/Desktop/aellus-drops）: ↵

  📁 保存目录: /Users/you/Desktop/aellus-drops
  🌐 访问地址: http://192.168.1.111:8000
     (同局域网内，浏览器打开上面地址)
  🚀 启动中... 按 Ctrl+C 停止
```

### 2. 访问使用

- **本机**：浏览器打开 `http://localhost:8000`
- **手机/其他设备**：浏览器打开启动时显示的 `http://<电脑的局域网IP>:8000`

首页提供两个入口：
- 📤 **上传文件** → 填设备名 → 选文件 / 拍照 / 录像 → 上传
- 📂 **读取文件** → 选择目录 → 浏览文件 → 下载或预览

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
├── main.go            # 入口：参数解析 / 交互输入 / 获取IP / 启动服务
├── config.go          # 配置：保存目录 / 监听地址 / 端口 / 日志路径
├── handlers.go        # 路由与处理函数：页面 / 上传 / 列表 / 下载 / 批量打包
├── safepath.go        # 路径穿越校验 + 设备名过滤
├── lanip.go           # 局域网 IP 获取（UDP socket 连接法，跨平台无系统命令）
├── logmw.go           # 访问日志中间件 + 操作日志
├── go.mod             # Go 模块定义
├── build.sh           # 交叉编译脚本（一台机器出三端二进制）
├── Makefile           # make build / clean / run / vet
├── assets/            # 前端资源（//go:embed 编译进二进制）
│   ├── embed.go       #   embed 指令：打包 templates/ + static/
│   ├── templates/     #   HTML 模板（home / upload / browse）
│   └── static/        #   CSS / JS / favicon
├── screenshots/       # 三端页面截图（Android / iOS / PC）
└── README.md
```

> 运行时会自动在**可执行文件同目录**生成三类日志（无需额外配置）：
>
> | 日志文件 | 记录内容 |
> |---------|---------|
> | `access.log` | 每次访问的来源 IP / 浏览器 / 请求路径 / 响应状态 |
> | `operation.log` | 上传成功 / 批量打包下载 |
> | `server.log` | （仅后台运行重定向时）启动信息与报错 |
>
> `access.log` 各字段带中文标注，格式示例：
> ```
> 2026-08-06 16:21:21  访问来源IP: 192.168.1.99  请求方式: GET  请求URL路径: /browse  响应状态: 200  浏览器UA: "Mozilla/5.0 (iPhone...)"
> ```
> `operation.log` 格式示例：
> ```
> 2026-08-10 20:37:29  ✅ op-test | 20260810_203729752_optest.txt | 0.00MB
> 2026-08-10 20:37:30  📦 打包下载 | op-test | 1 个文件
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

在任意平台（macOS / Linux / Windows）上执行，**一次性产出全部平台全架构产物**：

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
- macOS Gatekeeper 会拦截未签名二进制。解决：
  ```bash
  xattr -d com.apple.quarantine aellus-darwin-arm64
  ```
  或在「系统设置 → 隐私与安全性」中点「仍要打开」。

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
| 前端 | 原生 HTML5 / CSS3 / JavaScript（无框架，通过 `//go:embed` 打包进二进制） |
| 传输 | HTTP（局域网点对点，不走云端） |
| 依赖 | **零**——单文件二进制，目标机无需安装任何运行时 |

---

## 💬 反馈与建议

如果您有更好的功能想法或改进建议，欢迎提 [Issues](https://github.com/YGQ8988/aellus/issues)！
