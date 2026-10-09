#!/usr/bin/env python3
"""校验 ipk 包能否被 OpenWrt 24.10 的 opkg 正确解包。

背景：opkg 自带的解包器（libbb get_header_tar）只认老式 ustar/gun 归档。
如果打包端用了 POSIX pax 格式（macOS bsdtar 的默认行为就是它），归档里会多出
`PaxHeader/...` 成员，typeflag 为 'x'（0x78），opkg 会报：

    get_header_tar: Unknown typeflag: 0x78
    extract_archive: Don't know how to handle .../PaxHeader/currentdir

而且它对每个成员都报一遍，安装直接失败。同理 macOS 还会塞 `._xxx` AppleDouble
伴生文件，也要一并拦下。

本脚本不依赖 tarfile 的"解释"，而是按字节解析 tar 头，模拟 opkg 看到的东西。

用法：
    python3 tools/ipk_check.py dist/Aellus-*.ipk [更多文件...]
退出码 0 = 全部通过，1 = 至少一个包有问题。
"""

import gzip
import io
import sys

# opkg libbb get_header_tar 能处理的文件类型
SUPPORTED = {
    b"0": "普通文件",
    b"\x00": "普通文件（旧式）",
    b"1": "硬链接",
    b"2": "符号链接",
    b"5": "目录",
}


def gunzip_maybe(raw: bytes) -> bytes:
    """是 gzip 就解压，否则原样返回。"""
    if raw[:2] == b"\x1f\x8b":
        return gzip.decompress(raw)
    return raw


def scan_tar(raw: bytes):
    """遍历 tar 的所有成员，返回 [(typeflag, name, data_or_None)]。"""
    members = []
    off = 0
    n = len(raw)
    while off + 512 <= n:
        blk = raw[off : off + 512]
        if blk == b"\x00" * 512:  # 结束块
            break
        name = blk[0:100].rstrip(b"\x00").decode("utf-8", "replace")
        size_field = blk[124:136].strip(b"\x00 ").strip()
        try:
            size = int(size_field or b"0", 8)
        except ValueError:
            size = 0
        data_off = off + 512
        data = raw[data_off : data_off + size]
        members.append((blk[156:157], name, data))
        off = data_off + ((size + 511) // 512) * 512
    return members


def check(ipk_path: str) -> bool:
    raw = gunzip_maybe(open(ipk_path, "rb").read())
    problems = []
    stats = {"members": 0, "flags": {}}

    def walk(raw_inner, layer, depth=0):
        members = scan_tar(raw_inner)
        if not members:
            problems.append(f"{layer}: 归档里没有解析到任何成员")
            return
        for tf, name, data in members:
            stats["members"] += 1
            stats["flags"][tf] = stats["flags"].get(tf, 0) + 1
            base = name.rsplit("/", 1)[-1]
            if tf not in SUPPORTED:
                problems.append(
                    f"{layer}: 成员 {name!r} 的 typeflag 0x{tf.hex()} "
                    f"opkg 不支持（多半是 POSIX pax 头，请用 --format=ustar 重新打包）"
                )
            if "PaxHeader" in name:
                problems.append(f"{layer}: 存在 pax 扩展头成员 {name!r}")
            if base.startswith("._"):
                problems.append(
                    f"{layer}: 存在 macOS AppleDouble 伴生文件 {name!r} "
                    f"（打包时请 export COPYFILE_DISABLE=1）"
                )
            # 递归检查内层的 control.tar.gz / data.tar.gz
            if data[:2] == b"\x1f\x8b" and name.endswith((".tar.gz", ".tgz")):
                try:
                    walk(gunzip_maybe(data), f"{layer}::{name}", depth + 1)
                except Exception as e:  # noqa: BLE001
                    problems.append(f"{layer}::{name}: 内层解包失败 {e}")

    walk(raw, "ipk")

    flags_desc = "  ".join(
        f"0x{tf.hex()}({SUPPORTED.get(tf, '不支持').split('（')[0]})×{c}"
        for tf, c in sorted(stats["flags"].items())
    )
    print(f"{ipk_path}")
    print(f"    成员 {stats['members']} 个  typeflag: {flags_desc}")
    if problems:
        for p in problems[:12]:
            print(f"    ✗ {p}")
        if len(problems) > 12:
            print(f"    … 另有 {len(problems) - 12} 处")
        print("    => 安装会失败，请不要发布")
        return False
    print("    ✓ opkg 可直接安装")
    return True


def main() -> int:
    if len(sys.argv) < 2:
        print(__doc__.strip())
        return 2
    ok = True
    for path in sys.argv[1:]:
        try:
            ok &= check(path)
        except Exception as e:  # noqa: BLE001
            print(f"{path}\n    ✗ 无法解析：{e}")
            ok = False
    print("")
    print("全部通过" if ok else "存在问题的包（见上）")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
