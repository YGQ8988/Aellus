#!/bin/bash
# Aellus 全平台二进制构建脚本
# 在 macOS 上运行可构建全部 8 个目标；在 Linux 上运行仅构建 Linux + Windows
#   （macOS 需 cgo/Cocoa，只能在 Mac 本机构建）
# 产物输出到 dist/：命名 Aellus-<version>-<os>-<arch>（版本号紧跟产品名）。
# Windows 与 macOS 风格一致：打成 zip（架构体现在 zip 文件名，解压后统一是 Aellus.exe / Aellus.app）。
# darwin / linux 为裸二进制（无外层压缩）。
#
# 用法：./build-all.sh [version]   （默认 1.0.1）
set -e
cd "$(dirname "$0")"
mkdir -p dist

# 版本号：优先命令行参数；不传则读 fnos/manifest（与 build-mac.sh / build-fnos.sh 一致，
# 保证各平台产物版本统一）；兜底 1.0.1。
VERSION="${1:-}"
if [ -z "${VERSION}" ]; then
  VERSION="$(grep -m1 '^version' fnos/manifest 2>/dev/null | awk '{print $3}')"
fi
VERSION="${VERSION:-1.0.1}"
echo "构建版本：${VERSION}"
LDFLAGS_BASE="-s -w"

# arch_out：把 goarch 映射为产物命名用的架构标识：
#   amd64 → x64（64 位 x86）、386 → x86（32 位 x86）、arm64 保持 arm64。
arch_out() {
  case "$1" in
    amd64) echo "x64" ;;
    386)   echo "x86" ;;
    *)     echo "$1" ;;
  esac
}

# build <goos> <goarch> [extra-ldflags]：构建裸二进制（darwin / linux）。
# Windows 走 build_win_zip（打成 zip，内含 Aellus.exe）。
build() {
  local goos=$1 goarch=$2 extra=$3
  # 命名带版本号，版本号紧跟产品名：Aellus-<version>-<os>-<arch>，如 Aellus-1.0.2-darwin-arm64
  local out="dist/Aellus-${VERSION}-${goos}-$(arch_out "$goarch")"
  local cgo=0
  [ "$goos" = "darwin" ] && cgo=1
  # darwin：cgo 走 Cocoa/WebKit/UserNotifications，需在 Mac 本机编译
  local extld=""

  printf "  %-18s " "${goos}/${goarch}"
  GOOS=$goos GOARCH=$goarch CGO_ENABLED=$cgo \
    go build -trimpath -ldflags="${LDFLAGS_BASE} ${extra} ${extld}" -o "$out" .
  echo "✓ $(ls -lh "$out" | awk '{print $5}')"
}

# build_win_zip <goarch>：构建 Windows exe 并打成 zip（与 macOS 的 .app.zip 对齐）。
# zip 名体现架构：Aellus-<version>-windows-<arch>.zip；解压后统一是 Aellus.exe。
# -H windowsgui：GUI 子系统，双击不弹黑窗口。
build_win_zip() {
  local goarch=$1
  local label
  label="$(arch_out "$goarch")"
  local stage=".build/win-${label}"
  local zip_name="dist/Aellus-${VERSION}-windows-${label}.zip"
  mkdir -p "${stage}"
  printf "  %-18s " "windows/${goarch}"
  GOOS=windows GOARCH="${goarch}" CGO_ENABLED=0 \
    go build -trimpath -ldflags="${LDFLAGS_BASE} -H windowsgui" -o "${stage}/Aellus.exe" .
  rm -f "${zip_name}"
  # -j：只存文件名（zip 内为单文件 Aellus.exe，无目录层级）；-X：不存 macOS 扩展属性
  zip -q -j -X "${zip_name}" "${stage}/Aellus.exe"
  echo "✓ $(ls -lh "${zip_name}" | awk '{print $5}')  （zip 内含 Aellus.exe）"
  rm -rf "${stage}"
}

echo "Aellus 全平台构建 v${VERSION}"
echo "================================"

# macOS（cgo + Xcode CLT，仅 Mac 本机）
echo ""
if [ "$(uname)" = "Darwin" ]; then
  echo "[macOS] (cgo/Cocoa, 需 Xcode CLT)"
  # cgo 编译目标固定 macOS 11.0（兼容 macOS 11+，并保证通知框架 strong link）
  export MACOSX_DEPLOYMENT_TARGET=11.0
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
echo "[Windows] (纯 Go, -H windowsgui, 含图标；打包为 zip，内含 Aellus.exe)"
command -v zip >/dev/null 2>&1 || { echo "  [错误] 未找到 zip 命令（打包 Windows 产物需要）"; exit 1; }
gen_winres
build_win_zip amd64
build_win_zip arm64
build_win_zip 386

# Linux（纯 Go 交叉编译）
echo ""
echo "[Linux] (纯 Go)"
build linux amd64
build linux arm64
build linux 386

echo ""
echo "================================"
echo "完成，产物在 dist/："
ls -lh dist/Aellus-${VERSION}-darwin-* dist/Aellus-${VERSION}-linux-* dist/Aellus-${VERSION}-windows-* 2>/dev/null | awk '{printf "  %-8s %s\n", $5, $NF}'
rmdir .build 2>/dev/null || true
