# Aellus —— OpenWrt 包定义（官方 package.mk 格式）
#
# 两种构建方式：
#  1) 日常发布（推荐）：直接用项目根目录的 build-openwrt.sh 产 ipk（24.10）与
#     apk（25.12+），不需要 SDK。它每跑一次会把下面两行的 PKG_VERSION / PKG_RELEASE
#     自动同步后回写本文件。
#  2) 放进 OpenWrt 源码树 / SDK：把本目录拷成 package/aellus（或挂成自定义 feed），
#     执行 make package/aellus/compile。默认安装预编译二进制 files/aellus；
#     把源码放到 ./src 后设置 AELLUS_GO=1，则改用宿主 Go 工具链交叉编译。
#
# 包内容遵循官方约定：
#   /usr/bin/aellus                 主程序
#   /etc/init.d/aellus              procd 服务脚本（USE_PROCD=1）
#   /etc/uci-defaults/aellus        安装时放行端口（官方 uci-defaults 机制）
#   /usr/share/aellus/*.default     UCI 默认配置模板（首次启动时生成 /etc/config/aellus）
#   /www/luci-static/resources/...  LuCI 页面（服务 → Aellus局域网传输）
#   /usr/share/luci/menu.d/...      LuCI 菜单注册
#   /usr/share/rpcd/acl.d/...       rpcd 权限（UCI 读写 + 服务状态查询）

include $(TOPDIR)/rules.mk

# PKG_RELEASE 即 OpenWrt 的「小版本号」，最终落成 control 里的 Version: 1.0.5-r1。
# 约定由 build-openwrt.sh 维护：每次构建 +1；主版本（PKG_VERSION）变化时自动重置为 1。
# 手动指定：bash build-openwrt.sh <version> <release> 或 AELLUS_RELEASE=<n> bash build-openwrt.sh。
PKG_NAME:=aellus
PKG_VERSION:=1.0.5
PKG_RELEASE:=13

PKG_LICENSE:=MIT
PKG_LICENSE_FILES:=LICENSE
PKG_MAINTAINER:=Aellus <https://github.com/YGQ8988/Aellus>
PKG_BUILD_PARALLEL:=1

include $(INCLUDE_DIR)/package.mk

# OpenWrt 架构名 → Go 交叉编译参数（与 build-openwrt.sh 里的目标表保持一致）
GO_ARCH:=amd64
GO_VARS:=GOAMD64=v1
ifeq ($(ARCH),x86_64)
	GO_ARCH:=amd64
	GO_VARS:=GOAMD64=v1
endif
ifeq ($(ARCH),i386_pentium4)
	GO_ARCH:=386
	GO_VARS:=GO386=sse2
endif
ifeq ($(findstring aarch64,$(ARCH)),aarch64)
	GO_ARCH:=arm64
	GO_VARS:=
endif
ifeq ($(findstring arm_,$(ARCH)),arm_)
	GO_ARCH:=arm
	GO_VARS:=GOARM=7
endif
ifeq ($(findstring riscv64,$(ARCH)),riscv64)
	GO_ARCH:=riscv64
	GO_VARS:=
endif
# 大端 mips 先写，小端 mipsel 后写覆盖（mipsel_24kc 同样包含 "mips" 子串）
ifeq ($(findstring mips,$(ARCH)),mips)
	GO_ARCH:=mips
	GO_VARS:=GOMIPS=hardfloat
endif
ifeq ($(findstring mipsel,$(ARCH)),mipsel)
	GO_ARCH:=mipsle
	GO_VARS:=GOMIPS=hardfloat
endif

define Package/aellus
	SECTION:=utils
	CATEGORY:=Network
	TITLE:=LAN file transfer between phone and PC
	URL:=https://github.com/YGQ8988/Aellus
endef

define Package/aellus/description
	Aellus is a LAN file transfer service. Devices in the same network open
	http://<router-ip>:5115 in a browser to upload, browse and download files.
	Configuration: LuCI -> Services -> Aellus局域网传输.
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
endef

define Build/Compile
	if [ -f ./files/aellus ]; then \
		$(CP) ./files/aellus $(PKG_BUILD_DIR)/aellus; \
	elif [ -f ./src/go.mod ] && [ "$(AELLUS_GO)" = "1" ]; then \
		$(CP) ./src/. $(PKG_BUILD_DIR)/; \
		( cd $(PKG_BUILD_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=$(GO_ARCH) $(GO_VARS) \
			go build -trimpath -ldflags "-s -w -X 'aellus/internal/app.Version=$(PKG_VERSION)'" \
			-o aellus . ); \
	else \
		echo "aellus: 需要预编译二进制 files/aellus，或源码目录 src/ 且 AELLUS_GO=1" >&2; \
		exit 1; \
	fi
endef

# 刻意**不**定义 conffiles：配置文件 /etc/config/aellus 不由本包提供，
# 而是由 /etc/init.d/aellus 首次启动时从默认模板生成。
# 原因见 files/usr/share/aellus/aellus.config.default 的注释 —— 一旦它是包里的 conffile，
# opkg 每次升级都会拿它跟磁盘上的文件比内容；而 UCI 的任何写操作都必然让两边不一致，
# 于是每次升级都要刷一句 resolve_conffiles 并多出一个 /etc/config/aellus-opkg。
define Package/aellus/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/aellus $(1)/usr/bin/aellus

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/aellus $(1)/etc/init.d/aellus

	# 默认配置模板（不是 /etc/config/aellus 本体，见上面的注释）
	$(INSTALL_DIR) $(1)/usr/share/aellus
	$(INSTALL_DATA) ./files/usr/share/aellus/aellus.config.default $(1)/usr/share/aellus/aellus.config.default

	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/etc/uci-defaults/aellus $(1)/etc/uci-defaults/aellus

	# LuCI 页面（服务 → Aellus局域网传输）
	$(INSTALL_DIR) $(1)/www/luci-static/resources/view/aellus
	$(INSTALL_DATA) ./files/www/luci-static/resources/view/aellus/settings.js $(1)/www/luci-static/resources/view/aellus/settings.js
	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d
	$(INSTALL_DATA) ./files/usr/share/luci/menu.d/luci-app-aellus.json $(1)/usr/share/luci/menu.d/luci-app-aellus.json
	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./files/usr/share/rpcd/acl.d/luci-app-aellus.json $(1)/usr/share/rpcd/acl.d/luci-app-aellus.json
endef

# 三个维护脚本都在这里定义，内容与 control/{postinst,prerm,postrm} 保持一致
# （build-openwrt.sh 走后者，SDK 走这里；两处改一处必须同步另一处）。
#
# 注意：这里定义的是「ipk 语义」的 postinst/prerm/postrm。25.12 起包管理器换成 apk，
# 官方 package-pack.mk 会把它们分别包进 apk 的 post-install / pre-deinstall /
# post-deinstall 事件（并自动补 export pkgname、default_postinst 等样板），
# 由 SDK 构建时无需改动本文件；而 build-openwrt.sh 手工打 apk 走的是 apk/ 目录下
# 那五份生命周期脚本 —— 那里的逻辑与这里一一对应，改这里也要同步改 apk/。
#
# postinst：官方 default_postinst（跑 uci-defaults、enable + start）+ 恢复 prerm 备份的
#           UCI 配置（必须放在 default_postinst 之前，否则服务先起来就把默认配置生成了）
#           + 清 LuCI 缓存。
# prerm   ：官方 default_prerm（disable + stop）+ 升级前把 UCI 配置备份到 /tmp。
#           注意 prerm 返回非 0 会让 opkg 中止安装，末尾必须 exit 0。
# postrm  ：buildroot 没有默认实现，自定义：清理 uci-defaults 加的防火墙规则。
define Package/aellus/postinst
#!/bin/sh
[ -s "$${IPKG_INSTROOT}/lib/functions.sh" ] || exit 0
. "$${IPKG_INSTROOT}/lib/functions.sh"

# 升级时 opkg 会把旧包文件清单里的文件全删掉（opkg_install.c: remove_data_files_and_list），
# 其中包含老版本自带的 /etc/config/aellus；prerm 已备份到这里，此刻放回去。
if [ -z "$${IPKG_INSTROOT}" ]; then
	if [ -s /tmp/aellus-config.bak ] && [ ! -s /etc/config/aellus ]; then
		mv /tmp/aellus-config.bak /etc/config/aellus 2>/dev/null
		logger -t aellus "升级：已保留原有的 /etc/config/aellus"
	fi
	rm -f /tmp/aellus-config.bak 2>/dev/null
fi

default_postinst $$0 $$@

if [ -z "$${IPKG_INSTROOT}" ]; then
	rm -f /tmp/luci-indexcache /tmp/luci-indexcache.*.json 2>/dev/null
	rm -rf /tmp/luci-modulecache 2>/dev/null
fi

exit 0
endef

define Package/aellus/prerm
#!/bin/sh
[ -s "$${IPKG_INSTROOT}/lib/functions.sh" ] || exit 0
. "$${IPKG_INSTROOT}/lib/functions.sh"
default_prerm $$0 $$@

if [ -z "$${IPKG_INSTROOT}" ] && [ "$${1:-}" = "upgrade" ] && [ -s /etc/config/aellus ]; then
	cp /etc/config/aellus /tmp/aellus-config.bak 2>/dev/null
fi

exit 0
endef
define Package/aellus/postrm
#!/bin/sh
[ -s "$${IPKG_INSTROOT}/lib/functions.sh" ] || exit 0
. "$${IPKG_INSTROOT}/lib/functions.sh"
[ -n "$${IPKG_INSTROOT}" ] && exit 0

if uci -q get firewall.aellus >/dev/null; then
	uci -q delete firewall.aellus
	uci -q commit firewall
	/etc/init.d/firewall reload >/dev/null 2>&1
fi

rm -f /tmp/luci-indexcache /tmp/luci-modulecache

exit 0
endef

$(eval $(call BuildPackage,aellus))
