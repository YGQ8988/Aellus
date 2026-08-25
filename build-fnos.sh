#!/bin/bash
# Aellus 飞牛 fnOS 版构建脚本（在项目根目录运行）
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

echo ">> [1/3] 交叉编译"
# -tags fpk：飞牛 NAS 后台服务构建，排除所有桌面代码（托盘/通知/目录选择器/mac 开机项），
# 由 platform_fpks.go 提供 headless 等价实现，不带 systray 等桌面依赖。
GOOS=linux GOARCH=amd64 go build -tags fpk -trimpath -ldflags="-s -w -X main.Version=1.0.0" -o "${BUILD_TMP}/aellus-amd64" .
GOOS=linux GOARCH=arm64 go build -tags fpk -trimpath -ldflags="-s -w -X main.Version=1.0.0" -o "${BUILD_TMP}/aellus-arm64" .
echo "   amd64/arm64 编译完成"

echo ">> [2/3] 打包 x86 版"
cp "${BUILD_TMP}/aellus-amd64" fnos/app/aellus
chmod +x fnos/app/aellus
sed -i.bak 's/^platform     = .*/platform     = x86/' fnos/manifest && rm -f fnos/manifest.bak
"$FN_PACK" build --directory fnos
# 当前 fnpack 版本固定产出 Aellus.fpk（不带版本/平台），需手动重命名以免被下一轮覆盖
mv Aellus.fpk "dist/Aellus-1.0.0-x86.fpk"

echo ">> [3/3] 打包 arm 版"
cp "${BUILD_TMP}/aellus-arm64" fnos/app/aellus
chmod +x fnos/app/aellus
sed -i.bak 's/^platform     = .*/platform     = arm/' fnos/manifest && rm -f fnos/manifest.bak
"$FN_PACK" build --directory fnos
mv Aellus.fpk "dist/Aellus-1.0.0-arm.fpk"

# 还原为 x86 状态
cp "${BUILD_TMP}/aellus-amd64" fnos/app/aellus
sed -i.bak 's/^platform     = .*/platform     = x86/' fnos/manifest && rm -f fnos/manifest.bak

# 清理中间产物：.build/ 下的临时二进制是过路货，成品 fpk 已生成在 dist/ 目录
rm -rf "${BUILD_TMP}"
echo "    已清理临时目录 ${BUILD_TMP}/"

echo "完成："
ls -la dist/Aellus-*.fpk
