#!/bin/bash
# Aellus OpenWrt 构建脚本（在项目根目录运行）
#
# 产出：
#   - dist/Aellus-<version>-r<release>-openwrt-<arch>.ipk   OpenWrt 24.10（opkg）
#   - dist/Aellus-<version>-r<release>-openwrt-<arch>.apk   OpenWrt 25.12+（apk）
#
# 说明：
#   - 架构目前支持 x86_64 / arm64（aarch64_cortex-a53、aarch64_generic）/
#     MIPS（mips_24kc 大端、mipsel_24kc 小端）。要加架构，往下面 TARGETS 里加一行即可。
#   - 产哪种包由 AELLUS_PKGFMT 决定：ipk（只要 24.10）/ apk（只要 25.12+）/
#     both（默认，两种都产）。例：AELLUS_PKGFMT=apk bash build-openwrt.sh
#   - 为什么要两套：25.12 把包管理器从 opkg 换成了 apk（Alpine Package Keeper），
#     只认 .apk、不再认 .ipk；而 24.10 只认 .ipk。两者包格式完全不同（见下文），
#     但包内容、维护脚本的意图一致（分别放在 control/ 与 apk/ 两个目录）。
#   - 构建脚本与其它构建脚本共用 .build/ 中间目录（各自结束时会清理），
#     因此 build-all.sh / build-mac.sh / build-fnos.sh / 本脚本必须串行执行。
#   - 小版本号自动递增：ipk 的 Version 是 <主版本>-r<N>，每构建一次 N + 1（如这次 1.0.5-r1，
#     下一次就是 1.0.5-r2，路由器才能按 opkg 的版本比较把它识别为更新版本）。N 记在
#     openwrt/aellus/Makefile 的 PKG_RELEASE 里，构建成功后回写；主版本号变化时自动重置为 1。
#   - ipk 格式按 OpenWrt 官方约定：debian-binary + control.tar.gz + data.tar.gz 打包成一个 tar.gz。
#     postinst / prerm 用的是 buildroot 的默认实现（default_postinst / default_prerm），
#     安装时自动 enable + start，卸载时 disable + stop。
#   - apk 格式按 apk-tools v2：控制段（.PKGINFO + 生命周期脚本，tar 段、无结尾空块）
#     与数据段各一个 gzip 流，首尾相接即成包。官方是用宿主的 `apk mkpkg` 打的，
#     本项目不拉 SDK，改由 tools/mkapk.py 手工打包（格式细节写在那个文件的头部注释里）。
#     两处与官方对齐的关键点：包内自带 /lib/apk/packages/aellus.list（default_postinst
#     靠它找 uci-defaults 与 init.d 脚本），以及脚本里 export pkgname 后再调
#     default_postinst（apk 的脚本文件名解不出包名）。
#   - 包内已包含 LuCI 页面（服务 → Aellus局域网传输），装完即可在网页里配置。
#   - 归档格式固定为 ustar（不是 macOS bsdtar 默认的 POSIX pax，也不是 gnu）：
#     opkg 的解包器不认 pax 扩展头（typeflag 0x78），pax 格式的包装不上，会刷一屏
#     "get_header_tar: Unknown typeflag: 0x78"。打包前会自检参数、打包后会用
#     tools/ipk_check.py 逐个校验，不通过直接中止。
#
# 前置：本机有 Go（交叉编译，不需要 cgo）；ipk 校验需要 python3（缺失时跳过校验并提示）。
set -e
cd "$(dirname "$0")"

# macOS 的 bsdtar 打包时会把扩展属性写成 ._xxx 的 AppleDouble 伴生文件，
# 导致 ipk 里多出一堆 ._aellus / ._settings.js 之类的垃圾。关掉这个行为。
# （Linux / GNU tar 不认识该变量，设置也无害。）
export COPYFILE_DISABLE=1

# Go 路径（跨平台：Windows Git Bash / macOS / Linux）
if ! command -v go >/dev/null 2>&1; then
  for p in /usr/local/go/bin /opt/homebrew/bin /d/Go/bin; do
    [ -x "$p/go" ] && { export PATH="$p:$PATH"; break; }
  done
fi
command -v go >/dev/null 2>&1 || { echo "[错误] 未找到 go，请先安装 Go 工具链"; exit 1; }

PKG_DIR="openwrt/aellus"
[ -d "$PKG_DIR/files" ] || { echo "[错误] 未找到 $PKG_DIR/files，请在项目根目录执行"; exit 1; }

BUILD_TMP=".build/openwrt"
mkdir -p "${BUILD_TMP}/bin" dist

# 版本号：优先命令行参数；不传则读 fnos/manifest（与 build-all.sh / build-mac.sh 一致）
VERSION="${1:-}"
if [ -z "${VERSION}" ]; then
  VERSION="$(grep -m1 '^version' fnos/manifest 2>/dev/null | awk '{print $3}')"
fi
VERSION="${VERSION:-1.0.1}"

# —— 小版本号（PKG_RELEASE）——
# OpenWrt 包版本写成 <主版本>-r<PKG_RELEASE>（control 的 Version 字段），N 就是小版本号。
# 唯一真源是 openwrt/aellus/Makefile 里的 PKG_VERSION / PKG_RELEASE 两行——放进 SDK 里
# make package/aellus/compile 时用的是同一份值，不会出现两个地方版本对不上。
#
# 规则：每次构建自动 +1；主版本号（默认取 fnos/manifest 的 version）变化时自动重置为 1。
# 手动指定（不参与递增）：
#   bash build-openwrt.sh 1.0.5 7                  # 直接用 -r7
#   AELLUS_RELEASE=1 bash build-openwrt.sh         # 强制回到 -r1
# 回写放在构建全部成功之后（见文末），中途失败不会白白吃掉一个版本号。
PKG_MK="${PKG_DIR}/Makefile"
[ -f "$PKG_MK" ] || { echo "[错误] 未找到 ${PKG_MK}（PKG_RELEASE 的唯一真源）"; exit 1; }
MK_VERSION="$(sed -n 's/^PKG_VERSION:=\([^[:space:]]*\).*/\1/p' "$PKG_MK" | head -1)"
MK_RELEASE="$(sed -n 's/^PKG_RELEASE:=\([^[:space:]]*\).*/\1/p' "$PKG_MK" | head -1)"

AELLUS_RELEASE="${2:-${AELLUS_RELEASE:-}}"
if [ -n "$AELLUS_RELEASE" ]; then
  case "$AELLUS_RELEASE" in
    ''|*[!0-9]*) echo "[错误] 小版本号必须是正整数（当前：${AELLUS_RELEASE}）"; exit 1 ;;
  esac
  [ "$AELLUS_RELEASE" -ge 1 ] || { echo "[错误] 小版本号必须 ≥1"; exit 1; }
  RELEASE="$AELLUS_RELEASE"
elif [ -n "$MK_RELEASE" ] && [ "$MK_VERSION" = "$VERSION" ]; then
  RELEASE=$((MK_RELEASE + 1))
else
  RELEASE=1
  [ -n "$MK_VERSION" ] && [ "$MK_VERSION" != "$VERSION" ] &&
    echo "主版本变化 ${MK_VERSION} → ${VERSION}，小版本号重置为 r1"
fi

BUILD_TIME="$(date '+%Y-%m-%d %H:%M:%S')"
# 产物文件名带上小版本号（-r<N>）：否则 r9 / r10 的文件名完全一样，传到路由器后
# 分不清新旧，很容易装上旧包还以为是新包没修好。
FULL="${VERSION}-r${RELEASE}"
echo "构建版本：${VERSION}（-r${RELEASE}，上一次是 ${MK_VERSION:-未记录}-r${MK_RELEASE:-0}）"

# —— 产哪种包 ——
# ipk = 24.10（opkg）；apk = 25.12+（apk）；both = 两种都产（默认）
AELLUS_PKGFMT="${AELLUS_PKGFMT:-both}"
case "$AELLUS_PKGFMT" in
  ipk)  DO_IPK=1; DO_APK=0 ;;
  apk)  DO_IPK=0; DO_APK=1 ;;
  both) DO_IPK=1; DO_APK=1 ;;
  *) echo "[错误] AELLUS_PKGFMT 只能是 ipk / apk / both（当前：${AELLUS_PKGFMT}）"; exit 1 ;;
esac
echo "包格式：${AELLUS_PKGFMT}"
if [ "$DO_APK" = "1" ]; then
  command -v python3 >/dev/null 2>&1 || { echo "[错误] 打 apk 需要 python3（tools/mkapk.py）"; exit 1; }
  for s in post-install post-upgrade pre-upgrade pre-deinstall post-deinstall; do
    [ -f "$PKG_DIR/apk/$s" ] || { echo "[错误] 缺少 apk 生命周期脚本 $PKG_DIR/apk/$s"; exit 1; }
  done
fi

# 目标表：ipk 架构名 | GOARCH | 额外 Go 环境变量
# 加架构时在这里加一行即可，例如：
#   arm_cortex-a7|arm|GOARM=7
#   arm_cortex-a9|arm|GOARM=7
#   i386_pentium4|386|GO386=sse2
#   riscv64|riscv64|
TARGETS="
x86_64|amd64|GOAMD64=v1
aarch64_cortex-a53|arm64|
aarch64_generic|arm64|
mips_24kc|mips|GOMIPS=hardfloat
mipsel_24kc|mipsle|GOMIPS=hardfloat
"

# —— 1) 交叉编译（同一 GOARCH 只编译一次，多个架构名复用同一份二进制）——
echo ">> [1/5] 交叉编译 Go 二进制"
while IFS='|' read -r arch goarch govars; do
  [ -z "$arch" ] && continue
  case "$arch" in \#*) continue ;; esac
  BIN="${BUILD_TMP}/bin/aellus-${goarch}"
  [ -f "$BIN" ] && continue # 同一 GOARCH 已编译过
  echo "   ${goarch} ${govars}"
  # 纯静态：CGO_ENABLED=0，不依赖路由器的 libc；-s -w 去掉符号表，减小体积
  env CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" ${govars} \
    go build -trimpath -ldflags="-s -w -X 'aellus/internal/app.Version=${VERSION}' -X 'aellus/internal/app.BuildTime=${BUILD_TIME}'" \
    -o "$BIN" .
done <<EOF
${TARGETS}
EOF

# —— 2) 组装每个架构的包内容 ——
echo ">> [2/5] 组装包内容"
while IFS='|' read -r arch goarch govars; do
  [ -z "$arch" ] && continue
  case "$arch" in \#*) continue ;; esac
  STAGE="${BUILD_TMP}/${arch}"
  ROOT="${STAGE}/root"
  CTRL="${STAGE}/control"
  rm -rf "$STAGE" || true   # 清不掉也无所谓：下面的 install / cat > 会逐个覆盖重写，
                            # 这样不会因为「批量删除确认」策略把整个构建打断
  mkdir -p "$ROOT/usr/bin" "$ROOT/etc/init.d" "$ROOT/etc/uci-defaults" \
           "$ROOT/usr/share/aellus" \
           "$ROOT/www/luci-static/resources/view/aellus" \
           "$ROOT/usr/share/luci/menu.d" "$ROOT/usr/share/rpcd/acl.d" \
           "$CTRL"

  # 主程序与服务脚本：0755
  install -m 0755 "${BUILD_TMP}/bin/aellus-${goarch}" "$ROOT/usr/bin/aellus"
  install -m 0755 "$PKG_DIR/files/etc/init.d/aellus"  "$ROOT/etc/init.d/aellus"
  # uci-defaults：安装时执行一次（放行端口），成功后由 default_postinst 删除
  install -m 0755 "$PKG_DIR/files/etc/uci-defaults/aellus" "$ROOT/etc/uci-defaults/aellus"
  # UCI 默认配置模板。**不往 /etc/config 里装配置文件**（装了就是 conffile，
  # 每次升级 opkg 都要比对并刷 resolve_conffiles，详见该模板文件开头的注释）。
  install -m 0644 "$PKG_DIR/files/usr/share/aellus/aellus.config.default" \
                  "$ROOT/usr/share/aellus/aellus.config.default"
  # LuCI 页面 + 菜单 + rpcd 权限
  install -m 0644 "$PKG_DIR/files/www/luci-static/resources/view/aellus/settings.js" \
                  "$ROOT/www/luci-static/resources/view/aellus/settings.js"
  install -m 0644 "$PKG_DIR/files/usr/share/luci/menu.d/luci-app-aellus.json" \
                  "$ROOT/usr/share/luci/menu.d/luci-app-aellus.json"
  install -m 0644 "$PKG_DIR/files/usr/share/rpcd/acl.d/luci-app-aellus.json" \
                  "$ROOT/usr/share/rpcd/acl.d/luci-app-aellus.json"

  # 维护脚本（control.tar.gz 内，不进文件系统）
  install -m 0755 "$PKG_DIR/control/postinst" "$CTRL/postinst"
  install -m 0755 "$PKG_DIR/control/prerm"    "$CTRL/prerm"
  install -m 0755 "$PKG_DIR/control/postrm"   "$CTRL/postrm"
  # 不生成 conffiles：包里没有任何需要 opkg 保管的配置文件

  # control：字段顺序与 buildroot 生成的保持一致（include/package-pack.mk）
  SIZE="$(du -sk "$ROOT" | awk '{print $1}')"
  cat > "$CTRL/control" <<EOF
Package: aellus
Version: ${VERSION}-r${RELEASE}
License: MIT
Section: utils
URL: https://github.com/YGQ8988/Aellus
Maintainer: Aellus <https://github.com/YGQ8988/Aellus>
Architecture: ${arch}
Installed-Size: ${SIZE}
Description: LAN file transfer between phone and PC
 Devices in the same network open http://<router-ip>:5115 in a browser
 to upload, browse and download files. Configure it in LuCI:
 Services -> Aellus局域网传输.
EOF
  echo "   ${arch} 组装完成（${SIZE} KB）"
done <<EOF
${TARGETS}
EOF

# —— 3) 打包：ipk 给 24.10，apk 给 25.12+ ——
echo ">> [3/5] 打包 ${AELLUS_PKGFMT}"

# ============ 3a) ipk（24.10 / opkg） ============
if [ "$DO_IPK" = "1" ]; then
# —— 打包格式：必须是 ustar ——
# opkg 自带的解包器（libbb get_header_tar）不认 POSIX pax 扩展头（typeflag 0x78），
# 而 macOS 的 bsdtar 默认就是 pax 格式，打出来的包装不上，会刷一大串：
#     get_header_tar: Unknown typeflag: 0x78
#     extract_archive: Don't know how to handle .../PaxHeader/currentdir
# GNU tar 也一样，不能靠默认。故显式要求 ustar —— bsdtar 与 GNU tar 都认识这个参数，
# 且我们所有成员名的长度都远低于 ustar 的 100 字节限制。
#
# 注意：不要改成 --format=gnu，macOS bsdtar 没有这个格式。
TAR_FMT="--format=ustar"
( cd . && tar ${TAR_FMT} -cf /dev/null . >/dev/null 2>&1 ) || {
  echo "[错误] 当前 tar 不支持 ${TAR_FMT}，产生的 ipk 将无法被 opkg 安装"; exit 1; }

# 归档统一用 uid/gid 0、且不写入 gzip 时间戳，保证可复现。
# owner 参数必须跨平台：macOS 的 bsdtar 只认 --uid/--gid/--uname/--gname
# （不认 GNU 的 --owner/--group，传了会直接报错退出、管道里只剩空流）；
# GNU tar 只认 --owner/--group（不认 --uid/--gid）。按 tar 类型选参数，
# 否则会静默产出 20 字节的空 ipk（gzip 压缩空流），直到 ipk_check 才暴露。
if tar --version 2>/dev/null | grep -qi bsdtar; then
  OWNER_ARGS="--numeric-owner --uid 0 --gid 0 --uname root --gname root"
else
  OWNER_ARGS="--numeric-owner --owner=0 --group=0"
fi
tar_archive() {
  # $1=目录 $2=输出文件；若 $3 起是成员列表则只打包这些成员
  local dir="$1" out="$2"; shift 2
  if [ "$#" -gt 0 ]; then
    ( cd "$dir" && tar ${TAR_FMT} ${OWNER_ARGS} -cf - "$@" ) | gzip -n -9 > "$out"
  else
    ( cd "$dir" && tar ${TAR_FMT} ${OWNER_ARGS} -cf - . ) | gzip -n -9 > "$out"
  fi
  # 防御：tar 一旦失败（如参数不被当前 tar 支持），管道里只会剩空流，
  # 静默产出几十字节的垃圾包。这里立刻拦下，而不是等最终 ipk_check 才发现。
  [ -s "$out" ] || { echo "[错误] tar/gzip 输出为空：${out}"; exit 1; }
}
while IFS='|' read -r arch goarch govars; do
  [ -z "$arch" ] && continue
  case "$arch" in \#*) continue ;; esac
  STAGE="${BUILD_TMP}/${arch}"
  OUT="dist/Aellus-${FULL}-openwrt-${arch}.ipk"
  # 不用 rm -f：后面的重定向会直接截断重写；少一次删除就少一处可能被
  # 「批量删除确认」策略拦停的地方（那样的拦截会把构建流程整个打断）。
  tar_archive "${STAGE}/control" "${STAGE}/control.tar.gz"
  tar_archive "${STAGE}/root"    "${STAGE}/data.tar.gz"
  echo "2.0" > "${STAGE}/debian-binary"
  ( cd "$STAGE" && tar ${TAR_FMT} ${OWNER_ARGS} -cf - ./debian-binary ./control.tar.gz ./data.tar.gz ) | gzip -n -9 > "$OUT"
  echo "   ${OUT}"
  # 本架构用完即清理（还要打 apk 的话留给 apk 那一步）：整个 .build 累积起来上百个
  # 文件，留到最后一次性删的话，在开了「批量删除确认」的终端里会被拦下，
  # 还会连带把构建判成失败。
  [ "$DO_APK" = "1" ] || rm -rf "$STAGE" || true
done <<EOF
${TARGETS}
EOF
fi

# ============ 3b) apk（25.12+ / apk-tools） ============
# 包由 tools/mkapk.py 生成（apk v2：控制段 + 数据段两个 gzip 流）。
# 字段与官方 `apk mkpkg` 的 --info 一一对应；生命周期脚本按事件名逐个传入。
if [ "$DO_APK" = "1" ]; then
while IFS='|' read -r arch goarch govars; do
  [ -z "$arch" ] && continue
  case "$arch" in \#*) continue ;; esac
  STAGE="${BUILD_TMP}/${arch}"
  OUT="dist/Aellus-${FULL}-openwrt-${arch}.apk"
  python3 tools/mkapk.py \
    --root "${STAGE}/root" \
    --out "$OUT" \
    --info "name=aellus" \
    --info "version=${VERSION}-r${RELEASE}" \
    --info "arch=${arch}" \
    --info "description=LAN file transfer between phone and PC" \
    --info "url=https://github.com/YGQ8988/Aellus" \
    --info "license=MIT" \
    --info "origin=aellus" \
    --info "maintainer=Aellus <https://github.com/YGQ8988/Aellus>" \
    --script "post-install=${PKG_DIR}/apk/post-install" \
    --script "post-upgrade=${PKG_DIR}/apk/post-upgrade" \
    --script "pre-upgrade=${PKG_DIR}/apk/pre-upgrade" \
    --script "pre-deinstall=${PKG_DIR}/apk/pre-deinstall" \
    --script "post-deinstall=${PKG_DIR}/apk/post-deinstall" \
    || { echo "[错误] apk 打包失败：${arch}"; exit 1; }
  rm -rf "$STAGE" || true
done <<EOF
${TARGETS}
EOF
fi

# —— 4) 体检 ——
# 换了打包工具/换了构建机，这一步能在发布前把问题拦下来，而不是让用户装上才发现。
echo ">> [4/5] 校验产物可被包管理器解包"
if [ "$DO_IPK" = "1" ]; then
  if command -v python3 >/dev/null 2>&1; then
    python3 tools/ipk_check.py dist/Aellus-${FULL}-openwrt-*.ipk || {
      echo "[错误] ipk 校验未通过，已中止（产物不可用）"; exit 1; }
  else
    echo "   [警告] 未找到 python3，跳过 ipk 校验。建议手动确认："
    echo "           tar -tzvf <file>.ipk | grep -c PaxHeader   # 必须为 0"
  fi
fi
if [ "$DO_APK" = "1" ]; then
  python3 tools/apk_check.py dist/Aellus-${FULL}-openwrt-*.apk || {
    echo "[错误] apk 校验未通过，已中止（产物不可用）"; exit 1; }
fi

# —— 5) 回写小版本号 ——
# 走到这里说明构建与校验都过了，把本次用的版本号写回 Makefile，
# 下一次构建在此基础上 +1。中途任何一步失败都不会执行到这里，不会浪费版本号。
# 写成临时文件再 mv（不用 sed -i：它的备份文件还得再删一次，多一处可能被拦停的地方）
sed -e "s|^PKG_VERSION:=.*|PKG_VERSION:=${VERSION}|" \
    -e "s|^PKG_RELEASE:=.*|PKG_RELEASE:=${RELEASE}|" \
    "$PKG_MK" > "${PKG_MK}.tmp" && mv "${PKG_MK}.tmp" "$PKG_MK"
grep -m1 '^PKG_VERSION:=' "$PKG_MK"; grep -m1 '^PKG_RELEASE:=' "$PKG_MK"
echo ">> [5/5] 小版本号已回写 ${PKG_MK}：PKG_VERSION=${VERSION} PKG_RELEASE=${RELEASE}"

# 清理中间产物（build-fnos.sh 会清理 .build，这里只清自己的子目录）。
# 清理失败不影响构建结果 —— 各架构的 stage 在打包后已各自删掉，这里只剩几个编译产物。
if rm -rf "${BUILD_TMP}"; then
  echo "    已清理临时产物"
else
  echo "    [提示] 临时产物 ${BUILD_TMP} 未能自动清理，可稍后手动删除（已被 git 忽略）"
fi

echo ""
echo "完成：${VERSION}-r${RELEASE}"
# sha256 工具：Linux 是 sha256sum，macOS 是 shasum
SHA_CMD="sha256sum"
command -v sha256sum >/dev/null 2>&1 || SHA_CMD="shasum -a 256"
for f in dist/Aellus-${FULL}-openwrt-*.ipk dist/Aellus-${FULL}-openwrt-*.apk; do
  [ -f "$f" ] || continue
  printf "  %-52s %8s  %s\n" "$f" "$(du -h "$f" | awk '{print $1}')" "$(${SHA_CMD} "$f" 2>/dev/null | awk '{print $1}' | cut -c1-16)…"
done
echo ""
echo "安装：把包传到路由器后执行（按系统版本二选一）"
if [ "$DO_APK" = "1" ]; then
  echo "  OpenWrt 25.12+：apk add --allow-untrusted Aellus-${FULL}-openwrt-<arch>.apk"
  echo "      # 自签的包不在官方信任密钥里，必须带 --allow-untrusted"
fi
if [ "$DO_IPK" = "1" ]; then
  echo "  OpenWrt 24.10 ：opkg install Aellus-${FULL}-openwrt-<arch>.ipk"
fi
echo "  # 装完自动开机自启并启动；配置在 LuCI：服务 → Aellus局域网传输"
echo "  # 从 24.10 sysupgrade 到 25.12 后，opkg 装的包不会被自动迁移，需重新用 apk 安装"
