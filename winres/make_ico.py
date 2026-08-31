#!/usr/bin/env python3
"""Assemble a multi-size ICO from PNG files (PNG-compressed entries, Windows Vista+).
Usage: make_ico.py <out.ico> <size.png> [size.png ...]
"""
import struct
import sys


def build_ico(entries):
    """entries: list of (size, png_bytes). Returns ICO bytes."""
    header = struct.pack("<HHH", 0, 1, len(entries))
    dir_entries = b""
    data = b""
    offset = 6 + 16 * len(entries)
    for size, png in entries:
        dim = 0 if size >= 256 else size
        dir_entries += struct.pack(
            "<BBBBHHII",
            dim,      # width  (0 = 256)
            dim,      # height (0 = 256)
            0,        # color count
            0,        # reserved
            1,        # planes
            32,       # bit count
            len(png), # bytes in resource
            offset,   # image offset
        )
        data += png
        offset += len(png)
    return header + dir_entries + data


def main():
    out = sys.argv[1]
    entries = []
    for path in sys.argv[2:]:
        with open(path, "rb") as f:
            png = f.read()
        # size from filename like icon_16.png
        size = int(path.split("_")[-1].split(".")[0])
        entries.append((size, png))
    # sort by size ascending (conventional)
    entries.sort(key=lambda e: e[0])
    ico = build_ico(entries)
    with open(out, "wb") as f:
        f.write(ico)
    print(f"wrote {out}: {len(ico)} bytes, {len(entries)} images: {[e[0] for e in entries]}")


if __name__ == "__main__":
    main()
