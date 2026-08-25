#!/bin/bash
# build-fnos.sh — 打包 Aellus 为飞牛 fnOS 应用安装包 (.fpk)
#
# 拆分架构：x86 与 arm 各出一个独立包，由 manifest 的 platform 字段限制安装目标。
# 产物: dist/Aellus-1.0.0-x86.fpk  (amd64)
#       dist/Aellus-1.0.0-arm.fpk  (arm64)
#
# 依赖: 已运行 build.sh 生成 dist/aellus-linux-{amd64,arm64}
#       macOS 自带 sips / curl
#       fnpack 自动下载到 /tmp/fnpack
#
# 用法: bash build-fnos.sh

set -e

ROOT="$(cd "$(dirname "$0")" && pwd)"
DIST="$ROOT/dist"
FNOS="$ROOT/fnos"
FNPACK="${FNPACK:-/tmp/fnpack}"
FNPACK_VER="1.2.3"
SRC_ICON="$ROOT/assets/static/favicon.svg"
MANIFEST="$FNOS/manifest"

echo "编译飞牛 fnOS 应用包 (.fpk) — 拆分 x86 / arm"
echo "----------------------------------------"

# 1. 确保二进制存在
echo "→ 检查 Linux 二进制 ..."
for arch in amd64 arm64; do
  BIN="$DIST/aellus-linux-$arch"
  if [ ! -f "$BIN" ]; then
    echo "❌ 缺少 $BIN"
    echo "   请先运行 bash build.sh"
    exit 1
  fi
done

# 2. 下载 fnpack
if [ ! -x "$FNPACK" ]; then
  echo "→ 下载 fnpack $FNPACK_VER ..."
  case "$(uname -m)" in
    arm64)  DL_ARCH="darwin-arm64" ;;
    *)      DL_ARCH="darwin-amd64" ;;
  esac
  curl -sL -o "$FNPACK" "https://static2.fnnas.com/fnpack/fnpack-${FNPACK_VER}-${DL_ARCH}"
  chmod +x "$FNPACK"
fi

# 3. 生成图标（favicon.svg → 64/256 PNG，两个架构共用）
echo "→ 生成图标 ..."
mkdir -p "$FNOS/app/ui/images"
sips -s format png -z 64 64   "$SRC_ICON" --out "$FNOS/ICON.PNG"                   >/dev/null
sips -s format png -z 256 256 "$SRC_ICON" --out "$FNOS/ICON_256.PNG"               >/dev/null
sips -s format png -z 64 64   "$SRC_ICON" --out "$FNOS/app/ui/images/icon_64.png"  >/dev/null
sips -s format png -z 256 256 "$SRC_ICON" --out "$FNOS/app/ui/images/icon_256.png" >/dev/null

# 4. 清理旧的单包架构二进制（拆分后每个 fpk 只含一个 aellus，避免被 fnpack 打进包里）
rm -f "$FNOS/app/aellus-linux-amd64" "$FNOS/app/aellus-linux-arm64"

# 设置 manifest 的 platform 字段（保留原有对齐）
set_platform() {
  sed -i.bak -E "s/^(platform[[:space:]]*=).*/\\1 $1/" "$MANIFEST" && rm -f "$MANIFEST.bak"
}

# 单次打包：$1=架构标签(x86/arm) $2=源二进制路径
build_one() {
  local tag="$1" src="$2"
  echo "→ 打包 $tag 版 ..."
  cp "$src" "$FNOS/app/aellus"
  chmod +x "$FNOS/app/aellus"
  set_platform "$tag"
  ( cd "$FNOS" && "$FNPACK" build ) >/dev/null
  mv "$FNOS/Aellus.fpk" "$DIST/Aellus-1.0.0-$tag.fpk"
}

# 5. 分别打包 x86 / arm
build_one x86 "$DIST/aellus-linux-amd64"
build_one arm "$DIST/aellus-linux-arm64"

# 6. 还原 manifest 为 all，app/aellus 留 amd64（便于本地直接 fnpack build 调试）
set_platform all
cp "$DIST/aellus-linux-amd64" "$FNOS/app/aellus"

echo ""
echo "✅ 打包完成"
ls -lh "$DIST"/Aellus-1.0.0-*.fpk | awk '{print "   " $NF "  (" $5 ")"}'
echo ""
echo "将 .fpk 上传到飞牛 fnOS 设备，在应用中心安装即可（x86 设备选 x86 包，ARM 设备选 arm 包）。"
