#!/bin/bash
# 打包 macOS .app：双击运行，通用二进制（amd64 + arm64），含图标。
#
# 用法:
#   ./build-mac-app.sh          # 产出 dist/Aellus.app
#
# 产物:
#   dist/Aellus.app                 macOS 应用（双击运行）
#   dist/aellus-macos-universal     通用二进制（Intel + Apple Silicon）
#   dist/Aellus.icns                应用图标
#
# 依赖 macOS 自带工具：go / lipo / sips / iconutil / codesign，无需额外安装。
set -euo pipefail

cd "$(dirname "$0")"

APP_NAME="Aellus"
DIST="dist"
LDFLAGS="-s -w"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
SRC_ICON="assets/static/favicon.svg"

echo "编译版本: $VERSION"
echo

# 1. 分别编译 Intel 与 Apple Silicon 两个架构（cgo 链接 Cocoa，用于菜单栏常驻）
echo "→ 编译 darwin/amd64 ..."
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
  CGO_CFLAGS="-arch x86_64" CGO_LDFLAGS="-arch x86_64" \
  go build -trimpath -ldflags="$LDFLAGS -X main.version=$VERSION" -o /tmp/aellus-amd64 .
echo "→ 编译 darwin/arm64 ..."
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
  CGO_CFLAGS="-arch arm64" CGO_LDFLAGS="-arch arm64" \
  go build -trimpath -ldflags="$LDFLAGS -X main.version=$VERSION" -o /tmp/aellus-arm64 .

# 2. 合并为通用二进制
echo "→ 合并为通用二进制 (lipo) ..."
mkdir -p "$DIST"
lipo -create /tmp/aellus-amd64 /tmp/aellus-arm64 -output "$DIST/aellus-macos-universal"
rm -f /tmp/aellus-amd64 /tmp/aellus-arm64

# 3. 从 SVG 生成 .icns 图标
echo "→ 生成图标 $APP_NAME.icns ..."
ICONSET="/tmp/aellus.iconset"
rm -rf "$ICONSET"
mkdir -p "$ICONSET"
sips -s format png -z 16 16     "$SRC_ICON" --out "$ICONSET/icon_16x16.png"       >/dev/null
sips -s format png -z 32 32     "$SRC_ICON" --out "$ICONSET/icon_16x16@2x.png"    >/dev/null
sips -s format png -z 32 32     "$SRC_ICON" --out "$ICONSET/icon_32x32.png"       >/dev/null
sips -s format png -z 64 64     "$SRC_ICON" --out "$ICONSET/icon_32x32@2x.png"    >/dev/null
sips -s format png -z 128 128   "$SRC_ICON" --out "$ICONSET/icon_128x128.png"     >/dev/null
sips -s format png -z 256 256   "$SRC_ICON" --out "$ICONSET/icon_128x128@2x.png"  >/dev/null
sips -s format png -z 256 256   "$SRC_ICON" --out "$ICONSET/icon_256x256.png"     >/dev/null
sips -s format png -z 512 512   "$SRC_ICON" --out "$ICONSET/icon_256x256@2x.png"  >/dev/null
sips -s format png -z 512 512   "$SRC_ICON" --out "$ICONSET/icon_512x512.png"     >/dev/null
sips -s format png -z 1024 1024 "$SRC_ICON" --out "$ICONSET/icon_512x512@2x.png"  >/dev/null
iconutil -c icns "$ICONSET" -o "$DIST/$APP_NAME.icns"
rm -rf "$ICONSET"

# 4. 组装 .app 目录结构
echo "→ 组装 $APP_NAME.app ..."
APP="$DIST/$APP_NAME.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$DIST/aellus-macos-universal" "$APP/Contents/MacOS/aellus"
cp "$DIST/$APP_NAME.icns" "$APP/Contents/Resources/$APP_NAME.icns"
chmod +x "$APP/Contents/MacOS/aellus"

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundleDisplayName</key>
    <string>$APP_NAME</string>
    <key>CFBundleIdentifier</key>
    <string>com.tuhu.aellus</string>
    <key>CFBundleExecutable</key>
    <string>aellus</string>
    <key>CFBundleIconFile</key>
    <string>$APP_NAME</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST

# 5. ad-hoc 签名（本地构建更顺畅，避免部分 Gatekeeper 拦截）
echo "→ ad-hoc 签名 ..."
codesign --force --deep -s - "$APP" >/dev/null 2>&1 || echo "  (签名失败，可忽略)"

echo
echo "✅ 打包完成: $APP"
echo "   通用二进制: $DIST/aellus-macos-universal"
echo "   图标:       $DIST/$APP_NAME.icns"
