# Aellus 更新&Todo记录

## 未发布（dev 分支）
 - [x] 新增 OpenWrt 24.10 支持：ipk 安装包 + procd 服务脚本（x86_64 / arm64 / MIPS）
 - [x] LuCI 配置页（服务 → Aellus局域网传输）：启用服务勾选、服务端口号、文件储存目录，
       保存并应用后按配置自动启动或停止服务
 - [x] 平台识别：OpenWrt 环境下 /api/addr 的 platform 返回 OpenWrt；
       保存目录以 UCI 为唯一真源，日志落 tmpfs 避免写闪存
 - [x] 修复 ipk 装不上：`opkg install` 报成一屏 get_header_tar: Unknown typeflag: 0x78。
       原因是        macOS bsdtar 默认输出 POSIX pax 格式归档，opkg 自带的解包器不认 'x' 类型头。
       打包改为显式 --format=ustar；新增 tools/ipk_check.py，构建后逐个校验，不通过即中止
 - [x] 修复 LuCI「服务」菜单不显示 Aellus局域网传输：菜单依赖改为仅 acl（去掉多余的 uci 依赖，
       少一条会被判隐藏的条件）；postinst 补清 /tmp/luci-indexcache.<hash>.json 与
       luci-modulecache，并提示「ACL 组在登录时计算，需注销重登」
 - [x] 服务显示名改为「Aellus局域网传输」：LuCI 菜单标题与配置页标题同步改动
       （包内 id / init.d 名 / UCI 配置名仍为 aellus，不影响已装设备升级）
 - [x] 修复 OpenWrt 升级刷 `resolve_conffiles: ... placed at /etc/config/aellus-opkg`：
       根因是包把 /etc/config/aellus 当 conffile 提供，而服务首次启动会回写 UCI（自动探测的
       保存目录），文件内容必然与包内版本不一致，opkg 每次升级都要比对报警。
       改为**包不再附带该配置文件**：默认模板放 /usr/share/aellus/aellus.config.default，
       由 init 脚本首次启动生成；缺失的选项由 config_get 默认值兜底。
       升级路径已处理：prerm 备份配置 → opkg 删旧文件 → postinst 恢复，用户设置不丢
 - [x] LuCI 服务设置页：服务状态按运行状态着色（运行中绿色 / 未运行红色）；
       「访问地址」点击后在新标签页打开
 - [x] 修复 OpenWrt 衍生固件上 `/api/addr` 的 platform 误报「桌面端」：
       原判据只认 DISTRIB_ID 含 OpenWrt 或 ID=openwrt，只覆盖官方原版固件 ——
       ImmortalWrt（DISTRIB_ID='ImmortalWrt' / ID="immortalwrt"）、LEDE（ID="lede"）
       等衍生固件全部漏判。改为识别 OpenWrt 家族，三条判据互为兜底：
       /etc/openwrt_release 存在（OpenWrt 系独有文件）+ os-release 的 ID 或 ID_LIKE
       （含 openwrt/lede）+ /sbin/procd 存在。桌面 Linux 三条都不命中，不会误判
 - [x] OpenWrt 每次构建自动递增小版本号：ipk 的 Version 为 <主版本>-r<N>，N 取自
       openwrt/aellus/Makefile 的 PKG_RELEASE（也是 SDK 构建时用的同一个字段），
       构建成功后回写；主版本号变化时重置为 1，因此已装设备能被识别为升级而非降级
 - [x] 上传页也能直接渲染 `.icns`（与读取页一致）：icns 是容器格式，浏览器即便把它
       标成 image/icns 也显示不出来，原来上传页只有文件扩展名占位。
       读取页走的是服务端 /api/thumb（后端 icns.go 解析），但上传时文件还没上传、
       够不到服务端，故在前端解析容器、取出内嵌 PNG/JPEG 块转 blob URL 渲染；
       优先取短边 ≥128 的最小一块（缩略图仅 48px，不必解 1024px），老式 JP2/ARGB
       容器解析不出时回退为扩展名占位图标，不影响上传本身
 - [x] 对齐上传页与读取页的可预览格式：两页曾各判一套（上传页看浏览器给的 MIME
       `image/*` `video/*`，读取页看写死的扩展名清单），导致 svg / ico / jfif / tif /
       tiff / heif / avi / mkv 等格式在上传页能出预览、读取页却只显示扩展名占位。
       改为两页共用 `static/js/ui.js` 里的一份清单，且「MIME 优先、扩展名兜底」
       （部分系统给不出 File.type，如 Windows 上的 .heic）
 - [x] 读取页支持渲染 `.svg`：服务端对可携带脚本的类型一律按 octet-stream + attachment
       输出（防存储型 XSS），直接把下载链接塞给 `<img>` 渲染不出来。改为取回字节后
       以 `image/svg+xml` 建 blob 再交给 `<img>` —— 与上传页「本地 blob + `<img>`」
       是同一条安全路径：SVG 经 `<img>` 渲染时浏览器走「安全静态模式」，不执行脚本、
       不加载外部引用。服务端那条「绝不内联脚本类」的规则未改动（单测仍红灯保护）
 - [x] 修读取页 `.heic` / `.heif` 缩略图实际显示不出来：Go 的 mime 表（及多数 Linux 的
       /etc/mime.types）没有这两个扩展名，查出来是空串 → 白名单放行却仍按
       octet-stream 输出，配合 nosniff 浏览器不猜类型，缩略图一片空白。
       补显式 MIME 表，并新增单测固化「白名单内的图片扩展名必须能拿到 image/* 类型」
 - [x] 两页的缩略图加载失败（浏览器解不了该格式，如 Chrome 下的 TIFF）统一回退为
       扩展名占位块，不再留一个空白框；读取页同时把它从可预览列表摘掉，
       避免点开灯箱是一张空图
 - [x] 文件名展示口径统一：上传页结果卡与删除确认弹窗（单文件 / 灯箱）原先直接显示
       磁盘名，带上传时拼接的时间戳前缀 `20060102_150405.000000_`，与读取页卡片不一致。
       把 `displayName()`（去前缀）上移到 `static/js/ui.js` 共享层，三处统一调用；
       `data-name` / 删除与下载请求仍用磁盘原名，落盘防重名覆盖的机制不变
 - [x] README「反馈与建议」补充 QQ 沟通群：群名「Aellus局域网传输」、群号 1126916188，
       附进群二维码 `static/img/qq-group.jpg`
 - [x] 新增 OpenWrt 25.12 支持（apk 包）：25.12 把包管理器从 opkg 换成 apk
       （Alpine Package Keeper），只认 `.apk`、不再认 `.ipk`。`build-openwrt.sh`
       默认同时产出 ipk（24.10）与 apk（25.12+），`AELLUS_PKGFMT=ipk|apk` 可只产一种；
       架构名两边一致，TARGETS 表不变，Go 二进制 / LuCI 页面 / init.d / uci-defaults
       一行未改
 - [x] 自研 apk 打包器 `tools/mkapk.py`（沿袭「不拉 SDK」的路线，官方用的是宿主
       `apk mkpkg`）：按 apk-tools v2 输出「控制段 + 数据段两个 gzip 流」，控制段是
       无结尾空块的 tar 段（`.PKGINFO` 打头 + `.post-install` 等带点前缀的脚本），
       `datahash` 取数据段压缩后的 sha256
 - [x] 新增 `openwrt/aellus/apk/` 五份生命周期脚本（post-install / post-upgrade /
       pre-upgrade / pre-deinstall / post-deinstall）。两处与 opkg 不同、必须这样写：
       apk 的脚本 `$1` 是版本号而非 `install`/`upgrade`，靠文件名区分事件；
       `default_postinst` 在 apk 路径下靠 `pkgname` 环境变量认包名（`.post-install`
       用 `${1%.*}` 解不出），故脚本里 `export pkgname="aellus"` 后再无参调用
 - [x] 包内自带 `/lib/apk/packages/aellus.list`（官方由 buildroot 生成，手工打包由
       mkapk.py 合成）：25.12 的 `default_postinst` / `default_prerm` 靠它找出本包的
       `/etc/uci-defaults/*`（执行后删除，放行端口）与 `/etc/init.d/*`（enable + start），
       没有它装完不会自启、端口也不放行
 - [x] 新增 `tools/apk_check.py`：发布前静态校验 apk（gzip 分段、控制段无结尾空块、
       成员点前缀与事件名合法、.PKGINFO 字段、`datahash` 匹配、无 AppleDouble / 越界路径）

## V1.0.5(2026-10-05)
 - [x] 修复飞牛门户内点击下载无反应
 - [x] 移除按环境隐藏下载入口的逻辑
 - [x] 批量下载修复大文件卡顿
 - [x] 单文件下载改为流式,不再整份读入内存

## V1.0.4(2026-09-29)
 - [x] 新增分享按钮,点击弹出文件下载二维码
 - [x] 下载文件名与页面列表展示保持一致(不拼接时间戳)
 - [x] 拍摄/录像改为调用系统原生相机
 - [x] 安全修复
 - [x] 设置页面展示版本号及构建时间
 - [x] 其他平台设置页面移除手动设置保存目录功能，改为弹出系统路径选择来进行设置保存路径

## V1.0.3(2026-09-20)
 - [x] 支持icns格式预览
 - [x] 未知格式预览图与上传保持一致
 - [x] 添加FnDepot商店源文件
 - [x] 飞牛卸载增加是否保留配置选项
 - [x] 若干样式问题优化

## V1.0.2(2026-09-15)
 - [x] 文件浏览页面增加分页
 - [x] 移动端按钮样式对齐PC
 - [x] Windows打包文件名对齐Mac
 - [x] 移除低架构平台打包代码

## V1.0.1(2026-09-13)
 - [x] 飞牛下设置文件保存目录&删除等权限逻辑优化
 - [x] 飞牛安装包分架构打包，减小安装包体积
 - [x] 其他平台文件产物文件名统一

## V1.0.0(2026-09-12)
 - [x] 支持上传下载
 - [x] 支持Mac，Linux，Windows，飞牛OS