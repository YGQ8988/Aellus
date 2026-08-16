#!/bin/bash
# Aellus 交叉编译脚本：一台机器一次性产出 Windows / macOS / Linux 三端全架构单文件二进制。
#
# 用法:
#   ./build.sh            # 编译全部目标到 dist/
#   ./build.sh clean      # 清理 dist/
#
# 产物 (CGO_ENABLED=0 纯静态, -s -w 去符号, 体积 ~8MB):
#   Windows: amd64 / arm64 / 386              (.exe)
#   macOS:   amd64 / arm64
#   Linux:   amd64 / arm64 / 386 / arm        (静态链接, 零依赖)
#
#   其中 arm64 覆盖 Apple Silicon / Windows on Snapdragon / 树莓派；
#   arm(32位) 覆盖老旧 ARM 嵌入式设备 (GOARM=7)。
set -euo pipefail

cd "$(dirname "$0")"

DIST=dist
LDFLAGS="-s -w"   # 去除调试符号与 DWARF, 减小体积

# 固定构建工具链为 Go 1.22.12：与 .app 保持一致，让 macOS CLI 二进制同样支持 macOS 10.15
#（Intel amd64；arm64 由 Go 强制最低 11.0）。GOTOOLCHAIN 自动下载/切换，GOPROXY 默认国内镜像。
GO_TOOLCHAIN="go1.22.12"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

# 架构矩阵: "GOOS:GOARCH:文件后缀"
# 后缀为空表示无后缀；Windows 用 .exe
TARGETS=(
  # Windows
  "windows:amd64:.exe"
  "windows:arm64:.exe"
  "windows:386:.exe"
  # macOS (仅 amd64 + arm64)
  "darwin:amd64:"
  "darwin:arm64:"
  # Linux
  "linux:amd64:"
  "linux:arm64:"
  "linux:386:"
  "linux:arm:"
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

ok=0
fail=0
for t in "${TARGETS[@]}"; do
  IFS=':' read -r goos goarch ext <<< "$t"
  out="$DIST/aellus-${goos}-${goarch}${ext}"
  printf "→ 编译 %-8s %-6s ... " "$goos" "$goarch"
  # GOARM=7 兼容大多数 32 位 ARM 设备 (ARMv7)，对 arm 架构生效，其他架构忽略此变量
  if GOTOOLCHAIN="$GO_TOOLCHAIN" GOPROXY="$GOPROXY" \
    GOARM=7 CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="$LDFLAGS -X main.version=$VERSION" \
    -o "$out" . 2>/tmp/aellus-build-err; then
    echo "✅ $(ls -lh "$out" | awk '{print $5}')"
    ok=$((ok+1))
  else
    echo "❌ 失败"
    sed 's/^/    /' /tmp/aellus-build-err >&2
    fail=$((fail+1))
  fi
done
rm -f /tmp/aellus-build-err

echo
echo "✅ 成功 $ok 个, ❌ 失败 $fail 个"
echo "产物清单:"
ls -lh "$DIST" 2>/dev/null || echo "  (无产物)"

[ "$fail" -eq 0 ] || exit 1
