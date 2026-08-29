#!/bin/bash
# Aellus 全平台二进制构建脚本
# 在 macOS 上运行可构建全部 8 个目标；在 Linux 上运行仅构建 Linux + Windows
#   （macOS 需 cgo/Cocoa，只能在 Mac 本机构建）
# 产物输出到 dist/，命名：aellus-<os>-<arch>[.exe]
#
# 用法：./build-all.sh [version]   （默认 1.0.0）
set -e
cd "$(dirname "$0")"
mkdir -p dist

VERSION="${1:-1.0.0}"
LDFLAGS_BASE="-s -w -X main.Version=${VERSION}"

# build <goos> <goarch> [extra-ldflags]
build() {
  local goos=$1 goarch=$2 extra=$3
  local ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  local out="dist/aellus-${goos}-${goarch}${ext}"
  local cgo=0
  [ "$goos" = "darwin" ] && cgo=1
  # darwin：强制强链接系统框架，避免 UserNotifications 被弱链接导致通知授权失效
  local extld=""
  [ "$goos" = "darwin" ] && extld="-extldflags=-Wl,-no_weak_imports"

  printf "  %-18s " "${goos}/${goarch}"
  GOOS=$goos GOARCH=$goarch CGO_ENABLED=$cgo \
    go build -trimpath -ldflags="${LDFLAGS_BASE} ${extra} ${extld}" -o "$out" .
  echo "✓ $(ls -lh "$out" | awk '{print $5}')"
}

echo "Aellus 全平台构建 v${VERSION}"
echo "================================"

# macOS（cgo + Xcode CLT，仅 Mac 本机）
echo ""
if [ "$(uname)" = "Darwin" ]; then
  echo "[macOS] (cgo/Cocoa, 需 Xcode CLT)"
  # 兼容旧版 macOS：避免 cgo 默认写入 minos=26.0 导致 macOS 13 等旧系统拒绝加载；
  # 设为 11.0（Big Sur）保证 UserNotifications strong link（代码用了 macOS 11+ 的
  # UNNotificationPresentationOptionBanner，低于 11.0 会弱链接致通知授权失效）。
  export MACOSX_DEPLOYMENT_TARGET=11.0
  build darwin arm64
  build darwin amd64
else
  echo "[macOS] 跳过（需在 Mac 本机构建，cgo 依赖 Cocoa 框架）"
fi

# Windows（纯 Go 交叉编译，windowsgui 子系统不弹黑窗口）
echo ""
echo "[Windows] (纯 Go, -H windowsgui)"
build windows amd64 "-H windowsgui"
build windows arm64 "-H windowsgui"
build windows 386   "-H windowsgui"

# Linux（纯 Go 交叉编译）
echo ""
echo "[Linux] (纯 Go)"
build linux amd64
build linux arm64
build linux 386

echo ""
echo "================================"
echo "完成，产物在 dist/："
ls -lh dist/aellus-* 2>/dev/null | awk '{printf "  %-8s %s\n", $5, $NF}'
