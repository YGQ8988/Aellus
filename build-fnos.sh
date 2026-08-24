#!/bin/bash
# build-fnos.sh — 打包 Aellus 为飞牛 fnOS 应用安装包 (.fpk)
#
# 产物: fnos/Aellus-<version>.fpk
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

echo "编译飞牛 fnOS 应用包 (.fpk)"
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

# 3. 生成图标（favicon.svg → 64/256 PNG）
echo "→ 生成图标 ..."
sips -s format png -z 64 64   "$SRC_ICON" --out "$FNOS/ICON.PNG"               >/dev/null
sips -s format png -z 256 256 "$SRC_ICON" --out "$FNOS/ICON_256.PNG"           >/dev/null
sips -s format png -z 64 64   "$SRC_ICON" --out "$FNOS/app/ui/images/icon_64.png"  >/dev/null
sips -s format png -z 256 256 "$SRC_ICON" --out "$FNOS/app/ui/images/icon_256.png" >/dev/null

# 4. 复制二进制到打包目录
echo "→ 复制 Linux 二进制 ..."
cp "$DIST/aellus-linux-amd64" "$FNOS/app/aellus-linux-amd64"
cp "$DIST/aellus-linux-arm64" "$FNOS/app/aellus-linux-arm64"
chmod +x "$FNOS/app/aellus-linux-amd64" "$FNOS/app/aellus-linux-arm64"

# 5. 打包（fnpack 输出到当前目录，故 cd 到 fnos 执行）
echo "→ fnpack build ..."
( cd "$FNOS" && "$FNPACK" build )

# 6. 定位产物并移到 dist/
FPK=$(ls -t "$FNOS"/*.fpk 2>/dev/null | head -1)
if [ -z "$FPK" ]; then
  echo "❌ 未生成 .fpk 文件"
  exit 1
fi
mv "$FPK" "$DIST/$(basename "$FPK")"
FPK="$DIST/$(basename "$FPK")"

echo ""
echo "✅ 打包完成"
ls -lh "$FPK" | awk '{print "   文件: " $NF}'
ls -lh "$FPK" | awk '{print "   大小: " $5}'
echo ""
echo "将 $FPK 上传到飞牛 fnOS 设备，在应用中心安装即可。"
