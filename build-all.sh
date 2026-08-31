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

# arch_out：产物命名用 x86_64（Unix 惯例）替代 goarch 的 amd64，其余保持。
arch_out() {
  case "$1" in
    amd64) echo "x86_64" ;;
    *)     echo "$1" ;;
  esac
}

# build <goos> <goarch> [extra-ldflags]
build() {
  local goos=$1 goarch=$2 extra=$3
  local ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  local out="dist/aellus-${goos}-$(arch_out "$goarch")${ext}"
  local cgo=0
  [ "$goos" = "darwin" ] && cgo=1
  # darwin：cgo 走 Cocoa/WebKit/UserNotifications，需在 Mac 本机编译
  local extld=""

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
  # 强制 cgo 目标=11.0：本机 clang 默认 minos=13.0，且 Go cgo 子进程不透传
  # MACOSX_DEPLOYMENT_TARGET，会导致 cgo object 被抬到 13.0（macOS 11 真机弱链接崩溃）。
  export CGO_CFLAGS="-mmacosx-version-min=11.0"
  build darwin arm64
  build darwin amd64
else
  echo "[macOS] 跳过（需在 Mac 本机构建，cgo 依赖 Cocoa 框架）"
fi

# gen_winres：用 go-winres 从 winres/aellus.ico 生成 Windows 的 .syso 资源（图标 + 清单 + 版本信息）。
# .syso 会被 go build 按目标架构自动链接进 exe（资源管理器里显示的图标、右键"属性→详细信息"的版本/描述）。
# 注意：.syso 必须生成在项目根目录（go build 只会在包所在目录按 rsrc_windows_<arch>.syso 命名约定自动链接，
# 放到子目录图标会丢失），因此即使源图标在 winres/ 下，产物 .syso 也写在根目录。
# 找不到 go-winres 时回退使用仓库里已提交的 .syso；两者都没有才报错。
gen_winres() {
  local gw=""
  if command -v go-winres >/dev/null 2>&1; then
    gw="go-winres"
  elif [ -x "$(go env GOPATH)/bin/go-winres" ]; then
    gw="$(go env GOPATH)/bin/go-winres"
  fi
  if [ -z "$gw" ]; then
    if [ -f rsrc_windows_amd64.syso ]; then
      echo "  [winres] 未找到 go-winres，使用仓库内已有的 .syso（图标非最新时请重装工具后重跑）"
      return
    fi
    echo "  [错误] 未找到 go-winres，且没有已提交的 .syso"
    echo "        请先执行: go install github.com/tc-hib/go-winres@latest"
    exit 1
  fi
  if [ ! -f winres/aellus.ico ]; then
    echo "  [警告] 未找到 winres/aellus.ico，跳过图标资源生成（保留已有 .syso）"
    return
  fi
  echo "  [winres] 从 winres/aellus.ico 生成 Windows 图标/清单/版本资源 (v${VERSION})"
  "$gw" simply --arch amd64,arm64,386 --icon winres/aellus.ico \
    --manifest gui --product-name "Aellus" --file-description "Aellus 局域网文件快传" \
    --product-version "${VERSION}" --file-version "${VERSION}" \
    --original-filename "aellus.exe" --copyright "Aellus"
}

# Windows（纯 Go 交叉编译，windowsgui 子系统不弹黑窗口）
echo ""
echo "[Windows] (纯 Go, -H windowsgui, 含图标)"
gen_winres
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
