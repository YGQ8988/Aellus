#!/usr/bin/env python3
"""校验 apk 包是否结构完整、能被 apk（OpenWrt 25.12+）读出来。

只做「发布前拦得住」的静态检查（不装、不跑脚本）：

  1. 能否正确切成 gzip 流段（控制段 + 数据段）；
  2. 控制段是不是 tar「段」——没有结尾空块、成员名带点前缀、.PKGINFO 在第一个；
  3. .PKGINFO 必填字段齐不齐，格式是不是 `key = value`；
  4. datahash 是否等于数据段（压缩后）的 sha256 —— 不对 apk 会拒绝这个包；
  5. 数据段里有没有 macOS 的 `._xxx` AppleDouble、绝对路径 / `..` 之类的坏成员；
  6. 生命周期脚本名是否都是 apk 认识的事件名。

用法：
    python3 tools/apk_check.py dist/Aellus-*.apk [更多文件...]
退出码 0 = 全部通过，1 = 至少一个包有问题。
"""

import gzip
import hashlib
import io
import os
import sys
import tarfile
import zlib

# apk-tools 认识的生命周期脚本（apk-tools src/apk_package.h）
APK_EVENTS = {
    "pre-install", "post-install",
    "pre-upgrade", "post-upgrade",
    "pre-deinstall", "post-deinstall",
    "trigger",
}
REQUIRED_INFO = ("pkgname", "pkgver", "arch", "datahash")


def split_raw_streams(raw: bytes):
    """把首尾相接的多个 gzip 流按边界切开，返回各段的**原始（未解压）字节**。"""
    segs = []
    rest = raw
    while rest:
        if rest[:2] != b"\x1f\x8b":
            raise ValueError("剩余 %d 字节不是 gzip 流（缺 1f 8b 魔数）" % len(rest))
        dec = zlib.decompressobj(16 + zlib.MAX_WBITS)
        dec.decompress(rest)
        if not dec.eof:
            raise ValueError("gzip 流不完整（解压未到末尾）")
        consumed = len(rest) - len(dec.unused_data)
        segs.append(rest[:consumed])
        rest = dec.unused_data
        if rest and rest.strip(b"\x00") == b"":
            break  # 只剩填充，忽略
    return segs


def check(path: str) -> int:
    problems = []
    with open(path, "rb") as f:
        raw = f.read()

    try:
        segs = split_raw_streams(raw)
    except Exception as e:  # noqa: BLE001
        print("✗ %s：无法按 gzip 分段（%s）" % (path, e))
        return 1
    if len(segs) < 2:
        print("✗ %s：只切出 %d 段，apk 包至少要有控制段 + 数据段" % (path, len(segs)))
        return 1
    if len(segs) > 3:
        problems.append("切出 %d 段，apk v2 最多 3 段（签名 / 控制 / 数据）" % len(segs))
    # 有签名段时是 3 段（签名在前），取最后两段
    control_gz, data_gz = segs[-2], segs[-1]
    try:
        control = gzip.decompress(control_gz)
        data = gzip.decompress(data_gz)
    except Exception as e:  # noqa: BLE001
        print("✗ %s：gzip 解压失败（%s）" % (path, e))
        return 1

    # —— 控制段 ——
    try:
        ctar = tarfile.open(fileobj=io.BytesIO(control), mode="r:")
    except Exception as e:  # noqa: BLE001
        print("✗ %s：控制段不是有效 tar（%s）" % (path, e))
        return 1
    names = ctar.getnames()
    if not names:
        problems.append("控制段为空")
    elif names[0] != ".PKGINFO":
        problems.append("控制段第一个成员必须是 .PKGINFO，实际是 %r" % names[0])
    for n in names:
        if not n.startswith("."):
            problems.append("控制段成员名必须带点前缀：%r" % n)
            continue
        base = n[1:]
        if base != "PKGINFO" and base not in APK_EVENTS:
            problems.append("控制段里的 %r 不是 apk 认识的生命周期脚本名" % n)
    # tar 段不能有结尾空块（否则 apk 会认为归档已结束，读不到后面的数据段）
    if len(control) >= 1024 and control[-1024:] == b"\x00" * 1024:
        problems.append("控制段带了 tar 结尾空块（1024 个 0），后面的数据段会被丢弃")

    info = {}
    if ".PKGINFO" in names:
        body = ctar.extractfile(".PKGINFO").read().decode("utf-8", "replace")
        for line in body.splitlines():
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if " = " not in line:
                problems.append(".PKGINFO 这行不是 `key = value` 格式：%r" % line)
                continue
            k, v = line.split(" = ", 1)
            info[k] = v
        for k in REQUIRED_INFO:
            if k not in info:
                problems.append(".PKGINFO 缺字段 %s" % k)
        if "datahash" in info:
            got = hashlib.sha256(data_gz).hexdigest()
            if got != info["datahash"]:
                problems.append("datahash 不匹配：.PKGINFO 写 %s，数据段实际 sha256 是 %s"
                                % (info["datahash"], got))

    # —— 数据段 ——
    members = []
    try:
        dtar = tarfile.open(fileobj=io.BytesIO(data), mode="r:")
    except Exception as e:  # noqa: BLE001
        problems.append("数据段不是有效 tar（%s）" % e)
        dtar = None
    if dtar is not None:
        for m in dtar.getmembers():
            n = m.name
            members.append(n)
            if n.startswith("/") or n.startswith("./") or ".." in n.split("/"):
                problems.append("数据段路径不合法：%r" % n)
            if os.path.basename(n).startswith("._"):
                problems.append("数据段混入了 AppleDouble 伴生文件：%r（打包时没关 COPYFILE_DISABLE？）" % n)

    if problems:
        print("✗ %s" % path)
        for p in problems:
            print("    - %s" % p)
        return 1

    print("✓ %s  pkgname=%s pkgver=%s arch=%s  控制段 %dB / 数据段 %dB，%d 个成员"
          % (os.path.basename(path), info.get("pkgname", "?"), info.get("pkgver", "?"),
             info.get("arch", "?"), len(control), len(data), len(members)))
    return 0


def main() -> int:
    files = sys.argv[1:]
    if not files:
        sys.stderr.write("用法：python3 tools/apk_check.py <file.apk> [更多文件...]\n")
        return 2
    bad = 0
    for f in files:
        if not os.path.isfile(f):
            print("✗ %s（文件不存在）" % f)
            bad += 1
            continue
        if check(f):
            bad += 1
    if bad:
        print("\n%d 个包未通过校验" % bad)
        return 1
    print("\n全部通过")
    return 0


if __name__ == "__main__":
    sys.exit(main())
