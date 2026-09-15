#!/bin/bash
# Aellus macOS 构建脚本（必须在 Mac 上运行）
# 分架构打包：arm64 与 amd64 各自独立构建 .app 并压缩成 zip。
# 产物命名（架构体现在压缩包名，.app 统一叫 Aellus.app）：
#   - dist/Aellus-<version>-mac-arm64.zip   （Apple Silicon）
#   - dist/Aellus-<version>-mac-x64.zip     （Intel）
# 前置：1) 安装 Go   2) 安装 Xcode 命令行工具: xcode-select --install
# 因为 systray 在 macOS 走 cgo(Cocoa)，无法从 Windows 交叉编译，必须在 Mac 本机编译。
# 用法：./build-mac.sh [version]（不传则读 fnos/manifest 的 version，保证与 fpk / Windows 包一致）
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

# 版本号：优先命令行参数，其次 fnos/manifest（唯一版本源，与 fpk / Windows 包保持一致），最后兜底。
VERSION="${1:-}"
if [ -z "${VERSION}" ]; then
  VERSION="$(grep -m1 '^version' fnos/manifest 2>/dev/null | awk '{print $3}')"
fi
VERSION="${VERSION:-1.0.1}"
echo "构建版本：${VERSION}"

# cgo 编译目标固定 macOS 11.0（兼容 macOS 11+，并保证通知框架 strong link）
export MACOSX_DEPLOYMENT_TARGET=11.0
export CGO_CFLAGS="-mmacosx-version-min=11.0"

# 本机架构：arm64 / x86_64
HOST_ARCH="$(uname -m)"

# arch_label：把 goarch / 本机架构（uname -m）映射为命名标识（arm64 / x64）。
arch_label() {
  case "$1" in
    amd64|x86_64) echo "x64" ;;
    *)            echo "$1" ;;
  esac
}

# build_app <goarch>：编译、打包、签名并压缩单个架构的 .app。
# .app 统一命名 dist/Aellus.app（不区分架构）；架构体现在 zip 文件名。
build_app() {
  local goarch="$1"
  local label="$(arch_label "$goarch")"
  local app_dir="dist/Aellus.app"
  local zip_name="dist/Aellus-${VERSION}-mac-${label}.zip"

  echo ""
  echo ">> 编译 ${goarch}（本机 ${HOST_ARCH} → ${zip_name}）"
  GOOS=darwin GOARCH="${goarch}" CGO_ENABLED=1 \
    go build -trimpath -ldflags="-s -w" -o ".build/aellus-${goarch}" .

  rm -rf "${app_dir}"
  mkdir -p "${app_dir}/Contents/MacOS" "${app_dir}/Contents/Resources"

  # 注意：这里用不带引号的 heredoc，以便把 ${VERSION} 展开进 plist
  cat > "${app_dir}/Contents/Info.plist" << PLIST
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
    <string>${VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
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
  echo "    ${app_dir} 已生成并签名"

  # 压缩成 zip：用 ditto 保留扩展属性 / 符号链接（macOS 官方推荐的 .app 打包方式），
  # --keepParent 让 zip 内顶层就是 Aellus.app，解压即得可双击应用。
  rm -f "${zip_name}"
  ditto -c -k --sequesterRsrc --keepParent "${app_dir}" "${zip_name}"
  echo "    ${zip_name} 已生成"

  # 压缩后删除 .app（完整应用已在 zip 内），dist/ 只保留 zip，架构由文件名区分
  rm -rf "${app_dir}"
}

build_app arm64
build_app amd64

# 清理中间产物（单架构二进制已打进各自 .app 并压缩，留着无用）
rm -f .build/aellus-arm64 .build/aellus-amd64
echo "    已清理 .build/ 中间产物"

echo ""
echo "完成，产物在 dist/："
ls -lh dist/*.zip 2>/dev/null | awk '{printf "  %-8s %s\n", $5, $NF}'
echo ""
echo "按架构解压对应 zip 后双击 Aellus.app 即可（顶部菜单栏出现 Aellus 图标，点开有『打开浏览器 / 退出』）。"
echo "首次运行请先移除 quarantine 再双击（本机生成的 app 通常已无 quarantine，保险起见执行一次）："
echo "  unzip dist/Aellus-${VERSION}-mac-arm64.zip   # Apple Silicon"
echo "  unzip dist/Aellus-${VERSION}-mac-x64.zip  # Intel"
echo "  xattr -dr com.apple.quarantine Aellus.app"
