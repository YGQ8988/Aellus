#!/usr/bin/env python3
"""Verify a Windows PE file has icon + version resources embedded.
Properly walks the IMAGE_RESOURCE_DIRECTORY tree (handles leaf vs subdir).
Usage: check_pe_icon.py <file.exe>
"""
import struct
import sys


def u16(b, o): return struct.unpack_from("<H", b, o)[0]
def u32(b, o): return struct.unpack_from("<I", b, o)[0]


def parse_pe(path):
    with open(path, "rb") as f:
        data = f.read()
    if data[:2] != b"MZ":
        raise SystemExit("not a PE (no MZ)")
    pe_off = u32(data, 0x3C)
    if data[pe_off:pe_off+4] != b"PE\0\0":
        raise SystemExit("not a PE (no PE sig)")
    nsec = u16(data, pe_off + 6)
    opt_off = pe_off + 24
    magic = u16(data, opt_off)
    if magic == 0x20B:
        dd_off = opt_off + 112
    elif magic == 0x10B:
        dd_off = opt_off + 96
    else:
        raise SystemExit(f"unknown optional magic {magic:#x}")
    res_rva, res_size = u32(data, dd_off + 2*8), u32(data, dd_off + 2*8 + 4)
    ver_rva, ver_size = u32(data, dd_off + 16*8), u32(data, dd_off + 16*8 + 4)
    opt_size = u16(data, pe_off + 20)
    sec_off = opt_off + opt_size
    sections = []
    for i in range(nsec):
        s = sec_off + i * 40
        name = data[s:s+8].rstrip(b"\0").decode(errors="replace")
        vsize, vaddr, rawsize, rawptr = u32(data, s+8), u32(data, s+12), u32(data, s+16), u32(data, s+20)
        sections.append((name, vaddr, vsize, rawptr, rawsize))

    def rva2off(rva):
        for name, vaddr, vsize, rawptr, rawsize in sections:
            if vaddr <= rva < vaddr + max(vsize, rawsize):
                return rawptr + (rva - vaddr)
        return None

    def walk_entries(rva):
        off = rva2off(rva)
        if off is None:
            return
        nNamed, nID = u16(data, off+12), u16(data, off+14)
        for i in range(nNamed + nID):
            e = off + 16 + i * 8
            name_id = u32(data, e)
            off_data = u32(data, e+4)
            # 资源目录条目里的偏移是相对资源段基址（res_rva）的，需加上基址才是绝对 RVA
            yield name_id, bool(off_data & 0x80000000), res_rva + (off_data & 0x7fffffff)

    def collect(rva, path):
        for name_id, is_dir, target in walk_entries(rva):
            if is_dir:
                yield from collect(target, path + [name_id])
            else:
                doff = rva2off(target)
                if doff is None:
                    continue
                d_rva, d_size = u32(data, doff), u32(data, doff+4)
                d_off = rva2off(d_rva)
                if d_off is None:
                    continue
                yield path + [name_id], data[d_off:d_off+d_size]

    # Level 1: resource types
    level1 = {}
    for name_id, is_dir, target in walk_entries(res_rva):
        if is_dir:
            level1[name_id] = target

    icon_blobs = []
    if 3 in level1:
        for path, blob in collect(level1[3], [3]):
            icon_blobs.append((path, blob))

    png_n = sum(1 for _, b in icon_blobs if b[:8] == b"\x89PNG\r\n\x1a\n")
    dib_n = sum(1 for _, b in icon_blobs if b[:2] == b"BM" or (len(b) > 4 and b[0] == 0x28))
    return {
        "rt_icon": 3 in level1,
        "rt_group_icon": 14 in level1,
        "version": ver_rva != 0 and ver_size > 0,
        "icon_total": len(icon_blobs),
        "icon_png": png_n,
        "icon_dib": dib_n,
        "sizes": sorted([len(b) for _, b in icon_blobs]),
    }


def main():
    r = parse_pe(sys.argv[1])
    print("PE 资源检查结果:")
    print(f"  RT_ICON(3) 存在: {r['rt_icon']}")
    print(f"  RT_GROUP_ICON(14) 存在: {r['rt_group_icon']}")
    print(f"  RT_VERSION(16) 存在: {r['version']} (版本信息)")
    print(f"  RT_ICON 图标数量: {r['icon_total']} (PNG: {r['icon_png']}, DIB/BMP: {r['icon_dib']})")
    print(f"  各图标数据大小: {r['sizes']}")


if __name__ == "__main__":
    main()
