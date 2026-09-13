#!/bin/bash
# Aellus 飞牛 fnOS 版构建脚本（在项目根目录运行）
# 分架构模式：分别产出 x86_64 与 arm64 两个 .fpk，由应用中心按设备架构分发安装。
# 原理：x86 包 manifest platform=x86、仅含 amd64 二进制；arm 包 manifest platform=arm、仅含 arm64 二进制。
#       cmd/main 已兼容两种包：按 uname -m 找 aellus-x86_64 / aellus-aarch64，找不到回退 aellus。
# 前置：本机有 Go（交叉编译纯标准库，无需 cgo）与 fnpack（tools/fnpack 或 PATH 中）
set -e
cd "$(dirname "$0")"

# 定位 fnpack（macOS/Linux: tools/fnpack；Windows: tools/fnpack.exe；或 PATH 中）
FN_PACK=""
if command -v fnpack >/dev/null 2>&1; then
  FN_PACK="fnpack"
elif [ -f "tools/fnpack" ]; then
  FN_PACK="$(pwd)/tools/fnpack"
elif [ -f "tools/fnpack.exe" ]; then
  FN_PACK="$(pwd)/tools/fnpack.exe"
else
  echo "[错误] 未找到 fnpack，请放入 tools/fnpack（或 tools/fnpack.exe）或加入 PATH"
  exit 1
fi

# Go 路径（跨平台：Windows Git Bash / macOS / Linux）
if ! command -v go >/dev/null 2>&1; then
  for p in /usr/local/go/bin /opt/homebrew/bin /d/Go/bin; do
    [ -x "$p/go" ] && { export PATH="$p:$PATH"; break; }
  done
fi
mkdir -p fnos/app/ui/images
mkdir -p dist
# 用相对路径临时目录（go.exe 是 Windows 程序，不识别 Git Bash 的 /tmp 或 /e/... 绝对路径）
BUILD_TMP=".build"
mkdir -p "${BUILD_TMP}"

VERSION="${1:-1.0.1}"

echo ">> [1/3] 交叉编译 amd64 / arm64"
# -tags fpk：飞牛 NAS 后台服务构建，排除所有桌面代码（托盘/通知/目录选择器/mac 开机项），
# 由 platform_fpks.go 提供 headless 等价实现，不带 systray 等桌面依赖。
GOOS=linux GOARCH=amd64 go build -tags fpk -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o "${BUILD_TMP}/aellus-amd64" .
GOOS=linux GOARCH=arm64 go build -tags fpk -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o "${BUILD_TMP}/aellus-arm64" .
echo "   amd64/arm64 编译完成"

echo ">> [2/3] 打包 x86_64（platform=x86）"
# 清掉 app/ 下可能残留的其他架构二进制，避免混入包内
rm -f fnos/app/aellus fnos/app/aellus-aarch64
cp "${BUILD_TMP}/aellus-amd64" fnos/app/aellus-x86_64
chmod +x fnos/app/aellus-x86_64
sed -i.bak 's/^platform *= .*/platform              = x86/' fnos/manifest && rm -f fnos/manifest.bak
"$FN_PACK" build --directory fnos
mv Aellus.fpk "dist/Aellus-${VERSION}-x86_64.fpk"
echo "   x86_64 打包完成"

echo ">> [3/3] 打包 arm64（platform=arm）"
rm -f fnos/app/aellus fnos/app/aellus-x86_64
cp "${BUILD_TMP}/aellus-arm64" fnos/app/aellus-aarch64
chmod +x fnos/app/aellus-aarch64
sed -i.bak 's/^platform *= .*/platform              = arm/' fnos/manifest && rm -f fnos/manifest.bak
"$FN_PACK" build --directory fnos
mv Aellus.fpk "dist/Aellus-${VERSION}-arm64.fpk"
echo "   arm64 打包完成"

# 还原 manifest 为 all（源码默认，支持全平台），保持 git 干净
sed -i.bak 's/^platform *= .*/platform              = all/' fnos/manifest && rm -f fnos/manifest.bak

# 清理临时二进制（fnos/app 下的编译产物不入库）
rm -f fnos/app/aellus-x86_64 fnos/app/aellus-aarch64
rm -rf "${BUILD_TMP}"
echo "    已清理临时产物"

echo "完成："
ls -la dist/Aellus-${VERSION}-x86_64.fpk dist/Aellus-${VERSION}-arm64.fpk
