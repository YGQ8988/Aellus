#!/bin/bash
# Aellus macOS 构建脚本（必须在 Mac 上运行）
# 拆分打包：arm64 与 amd64 各自独立生成 .app，不再合并 universal。
# 命名规则：
#   - 构建架构 == 运行脚本电脑的架构 → dist/Aellus.app
#   - 否则 → dist/Aellus-<arch>.app（arch 为 arm64 / x86_64）
# 前置：1) 安装 Go   2) 安装 Xcode 命令行工具: xcode-select --install
# 因为 systray 在 macOS 走 cgo(Cocoa)，无法从 Windows 交叉编译，必须在 Mac 本机编译。
#
# 关键：macOS 26 (Tahoe) 的菜单栏权限系统会直接忽略【未签名】的 app，
# 导致 "Allow in the Menu Bar" 里根本不会出现本程序。所以打包后必须 codesign 签名。
# 本机没有 Developer ID，使用 ad-hoc 签名（--sign -）+ 强化运行时（--options runtime），
# 足以让 macOS 26 把 app 识别为合法本地应用并允许菜单栏图标。
set -e
cd "$(dirname "$0")"
# 禁用 macOS cp 的 AppleDouble（._xxx）副文件，避免 .app 内混入垃圾
export COPYFILE_DISABLE=1
mkdir -p dist .build

# 兼容旧版 macOS：cgo 默认用本机 SDK 版本写入 Mach-O 的 LC_BUILD_VERSION.minos，
# 在 macOS 26 上编译会写成 26.0，导致 macOS 13 等旧系统内核拒绝加载（"应用已损坏"）。
# 显式设为 11.0（Big Sur）：既让 macOS 13 能加载，又保证 UserNotifications 框架
# strong link（< 10.14 或 < 11.0 都会被弱链接，通知授权静默失效、不弹授权横幅）。
# 代码用了 UNNotificationPresentationOptionBanner（macOS 11+），故下限为 11.0。
export MACOSX_DEPLOYMENT_TARGET=11.0
# 强制 cgo 编译目标=11.0。本机 clang 默认 minos=13.0，而 Go cgo 子进程不会把
# MACOSX_DEPLOYMENT_TARGET 透传给 clang，导致 systray/项目 .m 编译出的 object 被抬到 13.0，
# 既刷 "built for newer macOS version (13.0)" 警告，又在 macOS 11 真机上因弱链接符号缺失而崩溃。
# 显式 CGO_CFLAGS 让所有 cgo object 真正按 11.0 编译，覆盖 macOS 11.0–26。
export CGO_CFLAGS="-mmacosx-version-min=11.0"

# 本机架构：arm64 / x86_64
HOST_ARCH="$(uname -m)"

# arch_label：把 goarch / 本机架构映射为命名标识（arm64 / x86_64）。
arch_label() {
  case "$1" in
    arm64)        echo "arm64" ;;
    amd64|x86_64) echo "x86_64" ;;
    *)            echo "$1" ;;
  esac
}

# build_app <goarch>：编译、打包、签名单个架构的 .app。
build_app() {
  local goarch="$1"
  local label="$(arch_label "$goarch")"
  local host_label="$(arch_label "$HOST_ARCH")"

  # 命名：本机架构无后缀；非本机架构追加 -<label>（如 Aellus-arm64 / Aellus-x86_64）
  local app_name="Aellus"
  if [ "$label" != "$host_label" ]; then
    app_name="Aellus-${label}"
  fi
  local app_dir="dist/${app_name}.app"

  echo ""
  echo ">> 编译 ${goarch}（本机 ${HOST_ARCH} → ${app_name}.app）"
  GOOS=darwin GOARCH="${goarch}" CGO_ENABLED=1 \
    go build -trimpath -ldflags="-s -w" -o ".build/aellus-${goarch}" .

  rm -rf "${app_dir}"
  mkdir -p "${app_dir}/Contents/MacOS" "${app_dir}/Contents/Resources"

  cat > "${app_dir}/Contents/Info.plist" << 'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>Aellus</string>
    <key>CFBundleDisplayName</key>
    <string>Aellus</string>
    <key>CFBundleExecutable</key>
    <string>Aellus</string>
    <key>CFBundleIdentifier</key>
    <string>com.aellus.app</string>
    <key>CFBundleVersion</key>
    <string>1.0</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleIconFile</key>
    <string>aellus.icns</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <!-- true = 菜单栏常驻（agent app），Dock 不显示图标；菜单栏图标右键可退出 -->
    <key>LSUIElement</key>
    <true/>
</dict>
</plist>
PLIST

  # 可执行文件名必须与 Info.plist 的 CFBundleExecutable 完全一致（含大小写），
  # 否则 macOS 13 上通知授权（usernoted）无法识别 app，requestAuthorization 返回
  # UNErrorDomain Code=1 (NotificationsNotAllowed)。
  cp ".build/aellus-${goarch}" "${app_dir}/Contents/MacOS/Aellus"
  chmod +x "${app_dir}/Contents/MacOS/Aellus"

  # 应用图标：从项目根 aellus.icns 拷进 Resources/（Info.plist 的 CFBundleIconFile 已指向它）
  if [ -f aellus.icns ]; then
    cp aellus.icns "${app_dir}/Contents/Resources/aellus.icns"
  else
    echo "    [警告] 未找到 aellus.icns，将以默认图标打包"
  fi
  # 刷新图标缓存，确保 Finder/Dock 立即生效
  touch "${app_dir}"
  # 兜底：删除可能残留的 AppleDouble 副文件
  find "${app_dir}" -name '._*' -delete 2>/dev/null || true

  # --force 覆盖、--deep 递归签名内部组件、--sign - 为 ad-hoc、--options runtime 开启强化运行时
  codesign --force --deep --sign - --options runtime "${app_dir}"
  echo "    ${app_name}.app 已生成并签名"
}

build_app arm64
build_app amd64

# 清理中间产物（单架构二进制已被打进各自 .app，留着无用）
rm -f .build/aellus-arm64 .build/aellus-amd64
echo "    已清理 .build/ 中间产物"

echo ""
echo "完成，产物在 dist/："
ls -d dist/*.app
echo ""
echo "首次运行请先移除 quarantine 再双击（本机生成的 app 通常已无 quarantine，保险起见执行一次）："
echo "  xattr -dr com.apple.quarantine dist/Aellus.app"
echo "双击对应架构的 .app 即可（顶部菜单栏出现 Aellus 图标，点开有『打开浏览器 / 退出』）。"
