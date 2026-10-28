#!/usr/bin/env python3
"""从「根目录」生成一个 ustar tar.gz，并强制写死每个成员的权限位。

为什么需要它（而不是让 tar / install 直接读磁盘权限）
------------------------------------------------------
在某些构建环境下，构建目录所在文件系统**不保留 Unix 权限位**
（外接 exFAT / FAT 卷、部分网络挂载等）：install -m 与 chmod 都是
空操作，落盘的文件只能拿到卷的默认权限（常显示为 0700）。若直接把
这种权限打进 ipk / apk，装到路由器上后：

  - /www/luci-static/.../settings.js 是 0700 → uhttpd（非 root 用户）
    读不了 → LuCI 设置页加载该 JS 报 HTTP 403；
  - acl.d / menu.d / usr/bin / init.d 也是 0700，但它们由 root 进程
    （rpcd / luci / procd）读取，反而正常，于是出现「菜单能显示、
    设置页 JS 却 403」的怪象。

因此打包环节必须无视磁盘权限，显式给定正确权限：
  - 目录：0755
  - 可执行文件 / 脚本（etc/init.d/*、usr/bin/*、etc/uci-defaults/*、
    control/* 下的维护脚本、*.sh、以及 postinst/prerm/postrm 等）：0755
  - 其余普通文件（配置、JSON、静态资源、control 文件本体等）：0644

用法：
    python3 tools/pack_root.py --root <目录> --out <文件.tar.gz> [--mtime N]
"""

import argparse
import gzip
import io
import os
import tarfile
import time

# 需要 0755 的维护脚本基名（出现在 control/ 下时按目录前缀已覆盖，这里兜底）。
EXEC_NAMES = {
    "postinst", "preinst", "postrm", "prerm",
    "post-upgrade", "pre-upgrade", "pre-deinstall", "post-deinstall",
}


def file_mode(rel: str) -> int:
    """根据相对路径给出普通文件应使用的权限位。"""
    base = rel.rsplit("/", 1)[-1]
    if rel.startswith(("etc/init.d/", "usr/bin/", "etc/uci-defaults/", "control/")):
        return 0o755
    if base in EXEC_NAMES or rel.endswith(".sh"):
        return 0o755
    return 0o644


def build(root: str, out: str, mtime: int) -> int:
    root = os.path.abspath(root)
    if not os.path.isdir(root):
        raise SystemExit("根目录不存在：%s" % root)

    files = []
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
        dirnames.sort()
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        for name in sorted(filenames):
            abs_path = os.path.join(dirpath, name)
            rel = os.path.relpath(abs_path, root).replace(os.sep, "/")
            files.append((rel, abs_path))
    files.sort(key=lambda kv: kv[0].encode("utf-8"))

    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w", format=tarfile.USTAR_FORMAT) as tar:
        # 目录成员：先全部父级、再文件（字典序天然满足先父后子）
        dirs = set()
        for rel, _ in files:
            parts = rel.split("/")
            for i in range(1, len(parts)):
                dirs.add("/".join(parts[:i]))
        for d in sorted(dirs):
            ti = tarfile.TarInfo(d)
            ti.type = tarfile.DIRTYPE
            ti.mode = 0o755
            ti.mtime = mtime
            ti.uid = ti.gid = 0
            ti.uname = ti.gname = "root"
            tar.addfile(ti)
        # 文件成员：权限由 file_mode 强制，不读磁盘
        for rel, abs_path in files:
            ti = tar.gettarinfo(abs_path, arcname=rel)
            ti.mode = file_mode(rel)
            ti.uid = ti.gid = 0
            ti.uname = ti.gname = "root"
            ti.mtime = mtime
            if ti.isreg():
                with open(abs_path, "rb") as f:
                    tar.addfile(ti, f)
            else:
                tar.addfile(ti)

    data = buf.getvalue()
    with open(out, "wb") as f:
        with gzip.GzipFile(fileobj=f, mode="wb", compresslevel=9, mtime=0) as g:
            g.write(data)

    return len(files)


def main() -> int:
    ap = argparse.ArgumentParser(description="从根目录生成带强制权限的 ustar tar.gz（ipk 用）")
    ap.add_argument("--root", required=True, help="包内容根目录（解包后落到 / 下）")
    ap.add_argument("--out", required=True, help="输出 .tar.gz 路径")
    ap.add_argument("--mtime", type=int, default=0, help="统一文件时间戳（可复现构建）")
    args = ap.parse_args()
    n = build(args.root, args.out, args.mtime)
    sys.stderr.write("  %s（%d 个文件，权限已强制）\n" % (args.out, n))
    return 0


if __name__ == "__main__":
    import sys
    sys.exit(main())
