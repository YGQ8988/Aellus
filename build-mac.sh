#!/bin/bash
# Aellus macOS 一键构建脚本（必须在 Mac 上运行）
# 前置：1) 安装 Go   2) 安装 Xcode 命令行工具: xcode-select --install
#       3) 安装 Pillow: pip3 install pillow   （用于生成菜单栏图标 PNG）
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
# 显式设为 10.13，链接器对 arm64 自动钳到 11.0（arm64 Mac 最低系统），amd64 保持 10.13。
export MACOSX_DEPLOYMENT_TARGET=10.13

echo ">> [1/4] 编译 Apple Silicon (arm64)"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o .build/aellus-darwin-arm64 .

echo ">> [2/4] 编译 Intel Mac (amd64)"
GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o .build/aellus-darwin-amd64 .

echo ">> [3/4] 合并为通用二进制 (Universal / fat) —— 取代 shell 启动器，让 .app 直接是可执行 Mach-O"
# 通用二进制是 macOS 标准的双架构方案，避免 shell 脚本 exec 二进制带来的 bundle 身份丢失问题。
lipo -create -output .build/aellus-universal .build/aellus-darwin-arm64 .build/aellus-darwin-amd64
lipo -info .build/aellus-universal

echo ">> [4/4] 打包成可双击的 dist/Aellus.app（主可执行文件直接是通用二进制）"
rm -rf dist/Aellus.app
mkdir -p dist/Aellus.app/Contents/MacOS dist/Aellus.app/Contents/Resources
cat > dist/Aellus.app/Contents/Info.plist << 'PLIST'
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
    <string>10.13</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <!-- true = 菜单栏常驻（agent app），Dock 不显示图标；菜单栏图标右键可退出 -->
    <key>LSUIElement</key>
    <true/>
</dict>
</plist>
PLIST
cp .build/aellus-universal dist/Aellus.app/Contents/MacOS/aellus
chmod +x dist/Aellus.app/Contents/MacOS/aellus
# 应用图标：从项目根 aellus.icns 拷进 Resources/（Info.plist 的 CFBundleIconFile 已指向它）
if [ -f aellus.icns ]; then
  cp aellus.icns dist/Aellus.app/Contents/Resources/aellus.icns
  echo "    已拷贝 aellus.icns 到 Resources/（应用图标）"
else
  echo "    [警告] 未找到 aellus.icns，将以默认图标打包"
fi
# 刷新图标缓存，确保 Finder/Dock 立即生效
touch dist/Aellus.app
# 兜底：删除可能残留的 AppleDouble 副文件
find dist/Aellus.app -name '._*' -delete 2>/dev/null || true

echo ">> 代码签名（ad-hoc + 强化运行时）—— 这是 macOS 26 菜单栏图标能出现的前提"
# --force 覆盖、--deep 递归签名内部组件、--sign - 为 ad-hoc、--options runtime 开启强化运行时
codesign --force --deep --sign - --options runtime dist/Aellus.app
echo "    签名完成"

echo ">> 清理中间产物（单架构二进制与 universal 已被合并进 dist/Aellus.app，留着无用）"
rm -f .build/aellus-darwin-arm64 .build/aellus-darwin-amd64 .build/aellus-universal
echo "    已清理 .build/ 中间产物"

echo "完成：dist/Aellus.app 已生成并签名"
echo "首次运行请先移除 quarantine 再双击（本机生成的 app 通常已无 quarantine，保险起见执行一次）："
echo "  xattr -dr com.apple.quarantine dist/Aellus.app"
echo "双击 dist/Aellus.app 即可（顶部菜单栏出现 Aellus 图标，点开有『打开浏览器 / 退出』）。"
