#!/usr/bin/env python3
"""把组装好的目录打成 OpenWrt 25.12+ 用的 apk 包（apk-tools v2 包格式）。

为什么需要它
------------
OpenWrt 25.12 把包管理器从 opkg 换成了 apk（Alpine Package Keeper），只认 .apk、
不再认 .ipk。官方 buildroot 是用宿主的 `apk mkpkg` 打包的（见 include/package-pack.mk），
但本项目一贯的做法是「不拉 SDK、只交叉编译 Go + 手工打包」，所以这里自己实现打包器，
保持这条轻量路线。

apk v2 包格式（Alpine wiki Apk_spec）
------------------------------------
    [签名段 gzip] [控制段 gzip] [数据段 gzip]      # 三个 gzip 流直接首尾相接
      ↑ 可选，我们没有密钥，不写

  - 控制段：一个 tar「段」——成员名一律带点前缀（`.PKGINFO`、`.post-install` …），
    且**不能**带 tar 的结尾空块（1024 字节的 0），否则后面的数据段会被当成
    归档结束之后的内容而被忽略。
  - 数据段：一个正常的 tar（带结尾空块），成员是相对根目录的路径（`usr/bin/aellus`，
    不带前导 `./`，不带前导 `/`）。
  - `.PKGINFO`：纯文本，每行 `key = value`（等号两侧各一个空格，apk 的解析很严格）。
  - `datahash`：数据段（**压缩后**的字节）的 sha256，写进 `.PKGINFO`，apk 会校验。

与官方 buildroot 的两个对齐点（不做就会出问题）
----------------------------------------------
  1. 必须自带 `/lib/apk/packages/<pkg>.list`（安装到路由器上的文件清单，每行一个
     绝对路径，排序）。`/lib/functions.sh` 的 `default_postinst` 是拿它来找出本包的
     `/etc/uci-defaults/*`（执行后删除）与 `/etc/init.d/*`（enable + start）的。
     官方由 buildroot 在打包前生成，我们自己在打包时合成。
  2. apk 的生命周期脚本参数是**版本号**，不是 opkg 的 `install`/`upgrade` 动作名；
     且 `default_postinst` 在 apk 路径下靠 `pkgname` 环境变量认包名（脚本文件名
     `.post-install` 用 `${1%.*}` 解不出包名）。所以脚本里必须
     `export pkgname="aellus"` 后再 `default_postinst`（不带参数）。

用法：
    python3 tools/mkapk.py --root <目录> --out <文件.apk> \
        --info name=aellus --info version=1.0.5-r6 --info arch=x86_64 \
        --script post-install=openwrt/aellus/apk/post-install ...
"""

import argparse
import gzip
import hashlib
import io
import os
import stat
import sys
import tarfile
import time

# 默认：给数据段的每个文件写 APK-TOOLS.checksum.SHA1 的 pax 扩展头
#（与官方 abuild-tar 一致，装完 apk audit 才不报文件校验缺失）
CHECKSUM_PAX_KEY = "APK-TOOLS.checksum.SHA1"


def die(msg: str) -> "None":
    sys.stderr.write("[错误] %s\n" % msg)
    sys.exit(1)


def sha1_file(path: str) -> str:
    h = hashlib.sha1()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def collect_files(root: str):
    """收集 root 下的普通文件与符号链接，返回 [(相对路径, 绝对路径)]（已排序）。

    只收文件与符号链接，不收目录 —— 与官方的
    `find . -type f,l -printf "/%P\\n" | sort` 一致；目录成员在 build_data_tar
    里按文件路径的父级合成写入（apk 要求显式 dirent，见该函数注释）。
    """
    out = []
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
        dirnames.sort()
        # 指向目录的符号链接不展开（避免打包器顺着软链跑到授权目录之外）
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        for name in sorted(filenames):
            abs_path = os.path.join(dirpath, name)
            rel = os.path.relpath(abs_path, root).replace(os.sep, "/")
            out.append((rel, abs_path))
    out.sort(key=lambda kv: kv[0].encode("utf-8"))
    return out


def make_tarinfo(tar, abs_path: str, arcname: str, with_checksums: bool, mtime: int):
    # gettarinfo 默认不跟随符号链接（dereference=False），链接会作为链接成员写入
    ti = tar.gettarinfo(abs_path, arcname=arcname)
    ti.uid = 0
    ti.gid = 0
    ti.uname = "root"
    ti.gname = "root"
    # mtime 统一成同一个整数：既保证可复现构建，也避免 tarfile 因为「亚秒级时间戳」
    # 往 pax 扩展头里额外塞一个 mtime 键（那是噪音，apk 不认）
    ti.mtime = mtime
    if ti.isreg() and with_checksums:
        ti.pax_headers = {CHECKSUM_PAX_KEY: sha1_file(abs_path)}
    return ti


def parent_dirs(rel: str):
    """rel 的全部祖先目录（不含根 '.'），如 'etc/init.d/aellus' → {'etc','etc/init.d'}。"""
    parts = rel.split("/")
    return {"/".join(parts[:i]) for i in range(1, len(parts))}


def build_data_tar(root: str, files, list_rel: str, list_body: str,
                   with_checksums: bool, mtime: int) -> bytes:
    """数据段：正常 tar（带结尾空块），外加合成的 .list 文件。

    目录成员不能省：apk 安装每个文件前都会先查它的父目录有没有在包里
    登记过（apk-tools database.c 的 apk_db_diri_query），查不到就报
    “no dirent in archive”并把包标记 broken_files——文件一个都不会落盘。
    官方 abuild / buildroot 打的包都带目录成员，这里同样全部显式写入
    （先所有目录、后文件；目录是其子路径的前缀，字典序天然满足先父后子）。
    """
    fmt = tarfile.PAX_FORMAT if with_checksums else tarfile.USTAR_FORMAT
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w", format=fmt) as tar:
        dirs = set()
        for rel, _ in files:
            if rel != list_rel:
                dirs |= parent_dirs(rel)
        dirs |= parent_dirs(list_rel)
        for d in sorted(dirs):
            ti = tarfile.TarInfo(d)
            ti.type = tarfile.DIRTYPE
            ti.mode = 0o755
            ti.mtime = mtime
            ti.uid = ti.gid = 0
            ti.uname = ti.gname = "root"
            tar.addfile(ti)
        for rel, abs_path in files:
            if rel == list_rel:
                continue  # .list 内容自己合成，不用磁盘上的（可能不存在）
            ti = make_tarinfo(tar, abs_path, rel, with_checksums, mtime)
            if ti.isreg():
                with open(abs_path, "rb") as f:
                    tar.addfile(ti, f)
            else:
                tar.addfile(ti)
        # 文件清单：放在最后，内容与官方一致（绝对路径 + 排序 + 不含自身）。
        # pax 校验和不能漏：apk 要求每个落盘的普通文件都带内嵌校验和
        # （APK-TOOLS.checksum.SHA1），缺了会报
        # “failed to extract ...: file format is obsolete (e.g. missing embedded checksum)”。
        # 其余文件走 make_tarinfo 已带，唯独 .list 是这里手工合成的，要单独补上。
        body = list_body.encode("utf-8")
        ti = tarfile.TarInfo(list_rel)
        if with_checksums:
            ti.pax_headers = {CHECKSUM_PAX_KEY: hashlib.sha1(body).hexdigest()}
        ti.size = len(body)
        ti.mtime = mtime
        ti.mode = 0o644
        ti.uid = ti.gid = 0
        ti.uname = ti.gname = "root"
        tar.addfile(ti, io.BytesIO(body))
    return buf.getvalue()


def build_control_tar(pkginfo: str, scripts, mtime: int) -> bytes:
    """控制段：tar 段（**无**结尾空块），成员名带点前缀，.PKGINFO 放第一个。"""
    buf = io.BytesIO()
    fmt = tarfile.USTAR_FORMAT
    with tarfile.open(fileobj=buf, mode="w", format=fmt) as tar:
        body = pkginfo.encode("utf-8")
        ti = tarfile.TarInfo(".PKGINFO")
        ti.size = len(body)
        ti.mtime = mtime
        ti.mode = 0o644
        ti.uid = ti.gid = 0
        ti.uname = ti.gname = "root"
        tar.addfile(ti, io.BytesIO(body))
        for name, path in scripts:
            ti = tarfile.TarInfo("." + name)
            ti.size = os.path.getsize(path)
            ti.mtime = mtime
            ti.mode = 0o755
            ti.uid = ti.gid = 0
            ti.uname = ti.gname = "root"
            with open(path, "rb") as f:
                tar.addfile(ti, f)
    return cut_tar(buf.getvalue())


def cut_tar(data: bytes) -> bytes:
    """去掉 tar 尾部的 1024 字节空块（以及补齐到块大小的填充），得到「tar 段」。

    不能简单 `rstrip(b"\\0")`：万一最后一个文件的内容本身就以 0 字节结尾，
    会被一起抹掉。这里按成员逐个算出真实的结束位置。
    """
    end = 0
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:") as tar:
        for m in tar:
            end = m.offset_data
            if m.isreg():
                end += (m.size + 511) // 512 * 512
    return data[:end]


def gz(data: bytes) -> bytes:
    """gzip，固定压缩级别 9、mtime=0（可复现构建）。"""
    buf = io.BytesIO()
    with gzip.GzipFile(fileobj=buf, mode="wb", compresslevel=9, mtime=0) as g:
        g.write(data)
    return buf.getvalue()


def main() -> int:
    ap = argparse.ArgumentParser(description="生成 OpenWrt 25.12+ 的 apk 包（v2 格式）")
    ap.add_argument("--root", required=True, help="包内容根目录（解包后落到路由器 / 下）")
    ap.add_argument("--out", required=True, help="输出 .apk 文件路径")
    ap.add_argument("--info", action="append", default=[], metavar="KEY=VALUE",
                    help=".PKGINFO 字段，可重复；name/version/arch 必填")
    ap.add_argument("--script", action="append", default=[], metavar="NAME=PATH",
                    help="生命周期脚本，如 post-install=path；NAME 为 apk 的事件名")
    ap.add_argument("--list-path", default="",
                    help="文件清单在包内的路径，默认 lib/apk/packages/<name>.list")
    ap.add_argument("--no-checksums", action="store_true",
                    help="不写 APK-TOOLS.checksum.SHA1 的 pax 扩展头")
    args = ap.parse_args()

    info = {}
    for kv in args.info:
        if "=" not in kv:
            die("--info 需要 KEY=VALUE 形式：%r" % kv)
        k, v = kv.split("=", 1)
        info[k] = v
    for need in ("name", "version", "arch"):
        if not info.get(need):
            die("--info %s=... 必填" % need)

    scripts = []
    for kv in args.script:
        if "=" not in kv:
            die("--script 需要 NAME=PATH 形式：%r" % kv)
        k, v = kv.split("=", 1)
        if not os.path.isfile(v):
            die("脚本不存在：%s" % v)
        scripts.append((k, v))
    # 控制段成员顺序固定，保证可复现
    scripts.sort(key=lambda kv: kv[0])

    if not os.path.isdir(args.root):
        die("包内容目录不存在：%s" % args.root)

    root = os.path.abspath(args.root)
    files = collect_files(root)
    if not files:
        die("包内容目录是空的：%s" % root)

    list_rel = args.list_path or ("lib/apk/packages/%s.list" % info["name"])
    list_rel = list_rel.lstrip("/")
    # 清单内容：绝对路径、排序、不含自身（与官方 find ... | sort 一致）
    listed = sorted("/" + rel for rel, _ in files if rel != list_rel)
    list_body = "\n".join(listed) + "\n"

    with_checksums = not args.no_checksums
    # 所有成员的 mtime 统一（可复现构建；需要固定时间戳时设 SOURCE_DATE_EPOCH）
    mtime = int(os.environ.get("SOURCE_DATE_EPOCH") or 0)
    data_tar = build_data_tar(root, files, list_rel, list_body, with_checksums, mtime)
    data_gz = gz(data_tar)

    installed_size = len(list_body)
    for rel, abs_path in files:
        if rel == list_rel:
            continue
        if stat.S_ISREG(os.lstat(abs_path).st_mode):
            installed_size += os.lstat(abs_path).st_size

    fields = [
        ("pkgname", info["name"]),
        ("pkgver", info["version"]),
        ("pkgdesc", info.get("description", info["name"])),
        ("url", info.get("url", "")),
        ("license", info.get("license", "MIT")),
        ("origin", info.get("origin", info["name"])),
        ("maintainer", info.get("maintainer", "")),
        ("arch", info["arch"]),
        ("size", str(installed_size)),
        ("builddate", str(int(os.environ.get("SOURCE_DATE_EPOCH") or time.time()))),
        # 注：builddate 保留真实构建时刻（用于排查），文件 mtime 统一为 0，二者互不影响
    ]
    if info.get("depends"):
        for dep in info["depends"].split():
            fields.append(("depend", dep))
    if info.get("provides"):
        for pro in info["provides"].split():
            fields.append(("provides", pro))
    # datahash 必须在最后：它是数据段（压缩后）的 sha256
    fields.append(("datahash", hashlib.sha256(data_gz).hexdigest()))

    pkginfo = "".join("%s = %s\n" % (k, v) for k, v in fields if v != "")
    control_gz = gz(build_control_tar(pkginfo, scripts, mtime))

    out_dir = os.path.dirname(os.path.abspath(args.out))
    if out_dir and not os.path.isdir(out_dir):
        os.makedirs(out_dir)
    with open(args.out, "wb") as f:
        f.write(control_gz)
        f.write(data_gz)

    sys.stderr.write("  %s（控制段 %d B / 数据段 %d B，%d 个文件）\n"
                     % (args.out, len(control_gz), len(data_gz), len(files) + 1))
    return 0


if __name__ == "__main__":
    sys.exit(main())
