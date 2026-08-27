<p align="center">
  <img src="static/img/favicon.svg" width="120" alt="Aellus">
</p>

<h1 align="center">Aellus · 局域网文件互传</h1>

<p align="center">
  <em>The power comes from AI</em><br>
  <em>The design comes from Yang Guangqing!</em><br>
  <em>The style comes from Yang Junwen!</em>
</p>

> Aellus 之名取自希腊神话中哈耳皮埃（Harpy）三姐妹之长 **Aello**，意为「风暴疾飞」——
> 正如文件在局域网内如风暴般瞬息传递。

Aellus 是一个轻量的局域网文件互传服务。在电脑（macOS / Windows / Linux）或飞牛 NAS 上启动后，同局域网内的手机或 PC 用浏览器访问，即可上传文件、浏览与下载已收到的文件。**零部署、免流量、跨平台、单文件分发**。

---

## ✨ 功能特性

- **上传 / 读取 / 下载**：手机或 PC 浏览器一键发送文件到主机，或浏览已上传文件并下载
- **按设备分目录**：上传时填设备名，文件自动归档到 `file-drops/<设备名>/`
- **自动加时间戳**：文件名带精确到毫秒的时间戳，永不覆盖
- **设备归属权限**：记录每个文件的上传来源，只能删除本设备上传的文件，他人上传的仅可下载
- **图片 / 视频预览**：上传后即时预览，读取页缩略图可在线播放，全屏灯箱浏览
- **批量打包下载**：勾选多个文件或整目录一键打包 zip
- **响应式布局**：同时适配 PC 与手机浏览器
- **保存目录可配置**：在首页设置里修改文件保存路径，配置持久化到系统配置目录，重启自动生效
- **桌面端常驻**：macOS 菜单栏 / Windows 系统托盘常驻图标，后台运行不抢焦点
- **飞牛 NAS 支持**：可作为飞牛 fnOS 后台服务运行（`.fpk` 包），复用飞牛授权目录机制
- **路径穿越防护**：严格校验目录名 / 文件名，禁止 `..` 越界与隐藏文件访问
- **单文件分发**：前端资源经 `//go:embed` 编译进二进制，运行时无需任何外部文件

---

## 🖥 环境依赖

| 项 | 要求 |
|----|------|
| 操作系统 | macOS / Windows / Linux（同一份 Go 代码交叉编译） |
| 运行时 | **无需安装任何运行时**（不依赖 Python / Node / 浏览器内核），双击即用 |
| 网络 | 主机与手机 / 其他设备在**同一局域网**内 |
| 自行编译（可选） | Go 1.21+ |

> 已发布的版本是**单文件可执行程序**：前端 `static/`、`templates/` 在编译期通过 `//go:embed` 打进二进制，运行时目录里不需要这些文件。

---

## 🚀 快速使用

### 1. 运行

**Windows：** 双击 `aellus.exe` 即可。程序会自动打开默认浏览器并跳转到访问地址，同时在**系统托盘**显示图标——右键菜单可「打开浏览器」或「退出」。

**macOS：**
```bash
chmod +x aellus-darwin-arm64    # Apple Silicon（M1/M2/M3）
# 或 chmod +x aellus-darwin-amd64  # Intel
./aellus-darwin-arm64
```
或双击 `Aellus.app`，顶部菜单栏出现 Aellus 图标。

**Linux：**
```bash
chmod +x aellus-linux-amd64
./aellus-linux-amd64
```

启动成功会输出访问地址，例如（macOS / Windows 中文，Linux 默认英文）：
```
Aellus 已启动 (Go 单文件版)
保存目录：.../Desktop/file-drops
本机局域网 IP：192.168.1.111
访问地址：http://localhost:8000
手机访问：http://192.168.1.111:8000
按 Ctrl+C 停止
```

### 2. 访问使用

- **本机**：浏览器打开 `http://localhost:8000`（Windows 双击后会自动打开）
- **手机 / 其他设备**：浏览器打开启动时显示的 `http://<主机局域网IP>:8000`

首页提供两个入口：
- 📤 **上传文件** → 填设备名 → 选文件 / 拍照 / 录像 → 上传
- 📂 **读取文件** → 选择目录 → 浏览文件 → 下载或预览

### 3. 自行构建

三个构建脚本各自独立，在对应环境运行：

| 产物 | 脚本 | 运行环境 | 说明 |
|------|------|---------|------|
| macOS `.app` | `build-mac.sh` | Mac 本机（需 Xcode CLT） | 通用二进制（arm64+amd64），已签名 |
| 全平台裸二进制 | `build-all.sh` | 任意平台 | 8 个目标：macOS / Windows / Linux 各架构 |
| 飞牛 fnOS `.fpk` | `build-fnos.sh` | 需 `fnpack` 工具 | x86 + arm，纯后台服务、无托盘 |

```bash
bash build-mac.sh     # 打包 dist/Aellus.app
bash build-all.sh     # 打包 dist/aellus-{os}-{arch} 共 8 个裸二进制
bash build-fnos.sh    # 打包 dist/Aellus-*.fpk
```

> `build-all.sh` 产物直接输出到 `dist/`；`build-mac.sh` 与 `build-fnos.sh` 共用 `.build/` 中间目录，**不能并行执行**（`build-fnos.sh` 结束时会清理 `.build/`），需串行运行。
>
> fpk 构建通过 `-tags fpk` 选择 `internal/platform/platform_fpks.go`（headless 实现），显式排除所有桌面代码（系统托盘 / 原生通知 / 原生文件夹选择 / systray 依赖）；桌面端构建不加该标签，使用 `platform_impl.go`，保持托盘与通知体验。

---

## 📂 目录结构

```
aellus/
├── main.go                       # 启动入口：embed 资源 + 平台选择 + App 构造 + Serve + 托盘
├── go.mod                        # Go module（唯一外部依赖 systray；fpk 构建自动剥离）
├── internal/app/                 # 业务逻辑（零 build-tag，纯 Go；平台差异通过 Platform 接口注入）
│   ├── app.go                    # App struct + New/Serve + 常量定义
│   ├── platform.go               # Platform 接口定义（隔离 build-tag 差异）
│   ├── routes.go                 # 路由注册
│   ├── handlers.go               # HTTP handler（上传/下载/浏览/删除/批量下载）
│   ├── settings.go               # 设置 handler（保存目录/授权目录/文件夹选择）
│   ├── config.go                 # 配置解析（保存目录/配置文件读写/旧配置迁移）
│   ├── netx.go                   # 网络工具（局域网 IP/端口监听）
│   ├── pathx.go                  # 路径安全（设备名/文件名/穿越防护）
│   ├── resolve.go                # 目录/文件路径解析
│   ├── owner.go                  # 上传归属 manifest（设备级文件归属）
│   ├── thumb.go                  # 缩略图生成
│   ├── trim.go                   # 飞牛授权目录 API
│   ├── middleware.go             # 中间件（日志/安全响应头/no-cache）
│   ├── logx.go                   # 日志写入
│   └── types.go                  # 数据结构定义
├── internal/platform/            # 平台层（build-tag 选择编译；实现 Platform 接口）
│   ├── platform_impl.go          # 桌面端实现（!fpk；托盘/通知/单实例/文件夹选择）
│   ├── platform_fpks.go          # fpk 端实现（//go:build fpk；headless 桩）
│   ├── tray_darwin.go            # macOS 菜单栏（systray + cgo）
│   ├── tray_windows.go           # Windows 托盘（纯 syscall，零依赖）
│   ├── tray_other.go             # Linux 信号阻塞（无托盘）
│   ├── notify_darwin.{go,m}      # macOS 系统通知（Cocoa）
│   ├── notify_windows.go         # Windows 气球通知
│   ├── notify_other.go           # 通知空实现
│   ├── pickdir_windows.go        # Windows 原生目录对话框
│   ├── pickdir_other.go          # 目录选择占位
│   ├── single_windows.go         # Windows 单实例（Mutex）
│   ├── single_other.go           # Unix 单实例（flock）
│   ├── app_agent_darwin.go       # macOS AppKit 激活策略（后台运行，Dock 不弹跳）
│   ├── forceSetTemplateIcon.m    # macOS 菜单栏图标 cgo ObjC
│   ├── menuicon.png              # macOS 菜单栏图标（embed）
│   └── favicon.ico               # Windows 托盘 / exe 图标（embed）
├── templates/                    # HTML 页面（已编译进二进制）
│   ├── home.html                 # 首页
│   ├── upload.html               # 上传页
│   ├── browse.html               # 读取页
│   └── callback.html             # 飞牛授权回调页
├── static/                       # 前端静态资源（已编译进二进制）
│   ├── common.css                # 公共样式 + 设计变量
│   ├── components.css            # 组件样式
│   ├── home.css / upload.css / browse.css   # 各页样式
│   ├── upload.js / browse.js     # 各页交互逻辑
│   ├── ui.js                     # 通用 UI 工具
│   ├── qrcode.js                 # 二维码生成
│   ├── trim-web-app.js           # 飞牛 web app 交互
│   ├── logo-icon.png             # 页面 logo
│   └── favicon.svg               # 标签页图标
├── build-mac.sh                  # macOS .app 构建脚本
├── build-all.sh                  # 全平台裸二进制构建脚本
├── build-fnos.sh                 # 飞牛 fnOS .fpk 构建脚本
├── aellus.icns                   # macOS 应用图标
├── fnos/                         # 飞牛 fnOS 打包资源（manifest / config / cmd）
└── README.md
```

### 上传文件落盘位置

桌面端默认保存在**用户桌面**下的 `file-drops/`（可在首页设置里改）：

```
~/Desktop/file-drops/
└── <设备名>/
    └── 20260804_112601079_截图.png   # 时间戳_原文件名
```

飞牛 NAS 端保存目录由飞牛「应用设置 → 授权目录」注入，落在授权目录树内。

---

## ⚙️ 配置说明

### 保存目录持久化

桌面端用户在首页设置里修改的保存路径，会写入**系统配置目录**下的 `aellus-settings.json`，重启自动生效：

| 平台 | 配置目录 |
|------|---------|
| macOS | `~/Library/Application Support/Aellus/` |
| Windows | `%APPDATA%\Aellus\` |
| Linux | `~/.config/Aellus/` |

归属 manifest（记录文件上传来源）也集中存放在该目录下的 `owners/` 子目录，不再散落在保存目录里污染用户可见文件。旧版散落的数据会在启动时自动迁移。

### 环境变量

| 变量 | 作用 | 默认 |
|------|------|------|
| `AELLUS_PORT` | 监听端口（被占用自动 +1） | `8000` |
| `AELLUS_SAVE_DIR` | 保存目录（飞牛 cmd/main 注入） | 桌面 `~/Desktop/file-drops` |
| `AELLUS_LANG` | 控制台输出语言：`en` / `zh` | Linux=`en`，其余=`zh` |
| `AELLUS_HEADLESS` | `1` 时跳过托盘 GUI，仅常驻 HTTP（CI / 调试） | 未设置 |

### 源码常量

核心默认值集中在 `internal/app/app.go` 的常量区，按需修改后重新编译：

```go
const (
    saveDirName = "file-drops"  // 文件保存目录名（桌面端路径：~/Desktop/file-drops）
    DefaultPort = 8000          // 默认端口；被占用自动尝试 8001、8002……
)
```

---

## 🛠 服务管理

- **macOS**：双击 `Aellus.app` 启动，顶部菜单栏出现 Aellus 图标，后台运行（不在 Dock 弹跳）；点菜单「退出」停止
- **Windows**：双击 `aellus.exe` 启动；右键系统托盘图标 →「退出」停止（也可任务管理器结束进程）
- **Linux**：终端运行二进制；`Ctrl+C` 停止
- **飞牛 NAS**：作为 fnOS 后台服务运行，由 fnOS / systemd 管理生命周期

> 未配置开机自启。如需自启：macOS 可配置 launchd，Windows 可配置任务计划程序，Linux 可配置 systemd。

---

## 🔒 安全说明

- **路径穿越防护**：所有目录名、文件名参数均经过校验——禁止 `/`、`\`，`..` 越界即拒绝，确保路径始终限定在保存目录范围内
- **设备名过滤**：仅保留字母、数字、中文、`-`、`_`，其余字符自动剔除
- **设备归属权限**：每个文件记录上传来源设备，仅本设备上传的文件可删除，他人上传的仅可下载，避免误删
- **飞牛授权目录**：fpk 端强制保存目录必须落在飞牛授权目录树内，由飞牛注入边界
- **仅局域网可用**：服务监听 `0.0.0.0` 但不暴露到公网，需在同一局域网内访问
- **桌面端无身份认证**：当前版本面向可信局域网环境，未设登录鉴权，请勿在公共网络使用

---

## 📜 免责声明

- **按现状提供**：本软件以「AS IS」方式提供，作者不对软件的可用性、准确性或适用性作任何明示或暗示的担保
- **数据责任**：本软件用于局域网文件传输，**不收集、不上传任何用户数据到云端**。但用户传输的文件内容、保存目录由用户自行管理，作者不对因使用本软件导致的任何数据丢失、损坏或泄露承担责任——请对重要文件自行备份
- **使用环境**：本软件桌面端无身份认证，面向可信局域网环境设计。在公共网络或不可信网络环境下使用带来的安全风险由用户自行承担
- **合规使用**：用户应遵守所在地区的法律法规，仅用于合法用途。因不当使用本软件产生的一切后果由用户自行承担

---

## ❓ 常见问题

**Q：手机 / 其他设备打不开页面？**
- 确认主机和设备在同一个局域网内
- 公司网络可能开启了「客户端隔离」，用手机开热点测试验证
- 首次启动若系统弹窗「是否允许程序接受入站连接」（macOS 防火墙 / Windows Defender 防火墙），点**允许**

**Q：换网络后访问地址变了？**
- 局域网 IP 会随网络变化，看启动时打印的 `访问地址：` / `Mobile:` 一行即可

**Q：Windows 双击没反应？**
- 本程序是「无控制台窗口」的 GUI 程序，正常运行时本就不会弹出黑窗口；双击后请查看**系统托盘**是否有图标，并确认默认浏览器是否已打开
- 若完全无反应，可能是端口被占用或杀毒软件拦截，建议在终端手动 `.\aellus.exe` 看报错

**Q：Linux 终端中文显示成黑方块？**
- 部分终端字体缺中文字形导致；Linux 版默认已输出英文规避。如需中文，设 `AELLUS_LANG=zh` 并确保终端字体含中文字形

**Q：上传大文件（录屏几百 MB）失败？**
- 采用流式读写，不会一次性占满内存；若超时请检查网络稳定性

---

## 🧰 技术栈

| 层 | 技术 |
|----|------|
| 运行平台 | macOS / Windows / Linux + 飞牛 fnOS（单文件，零运行时依赖） |
| 后端 | Go 1.21+ 标准库（`net/http`、`embed`、`syscall` 系统托盘） |
| 前端 | 原生 HTML5 / CSS3 / JavaScript（无框架） |
| 打包 | 静态资源 `//go:embed` 编译进二进制；macOS `build-mac.sh` 打 .app；飞牛 `fnpack` 打 .fpk |
| 传输 | HTTP（局域网点对点，不走云端） |

---

## 💬 反馈与建议

如果有更好的功能想法或改进建议，欢迎提 [Issues](https://github.com/YGQ8988/Aellus/issues)。

---

## ☕ 赞赏

如果 Aellus 对您有所帮助，可以请我们喝一杯咖啡吗？

<table>
  <tr>
    <td align="center">
      <img src="static/qr-dev-ygq.png" width="180" alt="开发者 染洛凉 支付宝二维码"><br>
      <sub>开发者 染洛凉</sub>
    </td>
    <td align="center">
      <img src="static/qr-designer-yixi.png" width="180" alt="设计师 一西啊 支付宝二维码"><br>
      <sub>设计师 一西啊</sub>
    </td>
  </tr>
</table>
