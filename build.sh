#!/bin/bash
# Aellus 交叉编译脚本：一台机器一次性产出 Windows / macOS / Linux 三端单文件二进制。
#
# 用法:
#   ./build.sh            # 编译全部目标到 dist/
#   ./build.sh clean      # 清理 dist/
#
# 产物 (CGO_ENABLED=0 纯静态, -s -w 去符号, 体积 ~8MB):
#   dist/aellus-windows-amd64.exe   Windows x86_64
#   dist/aellus-linux-amd64         Linux   x86_64 (静态链接, 零依赖)
#   dist/aellus-darwin-amd64        macOS   Intel
#   dist/aellus-darwin-arm64        macOS   Apple Silicon
set -euo pipefail

cd "$(dirname "$0")"

DIST=dist
LDFLAGS="-s -w"   # 去除调试符号与 DWARF, 减小体积
TARGETS=(
  "windows amd64 .exe"
  "linux   amd64 ''"
  "darwin  amd64 ''"
  "darwin  arm64 ''"
)

case "${1:-build}" in
  clean)
    rm -rf "$DIST"
    echo "已清理 $DIST/"
    exit 0
    ;;
  build) ;;
  *)
    echo "用法: $0 [build|clean]"
    exit 1
    ;;
esac

mkdir -p "$DIST"

# 版本号: 优先用 git tag/commit, 否则 dev
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
echo "编译版本: $VERSION"
echo

for t in "${TARGETS[@]}"; do
  # 解析 "os arch ext"
  set -- $t
  goos="$1"; goarch="$2"; ext="$3"
  ext="${ext//\'}"   # 去掉引号
  [ "$ext" = "''" ] && ext=""

  out="$DIST/aellus-${goos}-${goarch}${ext}"
  echo "→ 编译 $goos/$goarch ..."
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="$LDFLAGS -X main.version=$VERSION" \
    -o "$out" .
done

echo
echo "✅ 编译完成，产物:"
ls -lh "$DIST"
