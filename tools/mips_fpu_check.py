#!/usr/bin/env python3
"""校验 MIPS 二进制不含硬浮点指令（必须 GOMIPS=softfloat 编译）。

为什么需要这道检查
------------------
Go 的 mips / mipsle 架构默认是 hardfloat：编译器真的会生成 FPU 指令去执行浮点运算。
而 OpenWrt 这两类目标（mips_24kc：ath79/ar71xx 等；mipsel_24kc：ramips/mt7621 等）
的 CPU 都没有 FPU，OpenWrt 内核默认也不编译浮点模拟器（官方 bug FS#1929
「ramips mt7621 target does not enable FPU emulation by default」）。硬浮点二进制
一旦跑到浮点指令就会 SIGILL（Illegal instruction），进程秒退 —— procd 只报

    Instance aellus::aellus s in a crash loop 6 crashes, 0 seconds since last crash

现象是「包装得上、服务起不来」，主程序自己的日志一行都没有，极难定位。

因此构建脚本里对 mips / mipsle 一律加 GOMIPS=softfloat，让 Go 用整数指令模拟浮点
（在有 FPU 的机器上同样能跑，无需按机型分叉）。本脚本是发布前的兜底：把编译出的
MIPS 二进制体检一遍，改回 hardfloat 会被拦下，不再等到路由器上 crash loop 才发现。

怎么判
------
两道独立的凭据，任一检出硬浮点即失败：

1) ELF 的 .gnu.attributes 段 Tag_GNU_MIPS_ABI_FP（权威 ABI 标记，binutils 规范）：
       3 = soft float（合格）
       1/2/4/5/6/7 = 各种 hard float（不合格）
   Go 1.20+ 的链接器会为 MIPS 目标写入该标记，与编译时的 GOMIPS 一致。

2) 扫描可执行段（.text 等 SHF_EXECINSTR）的指令字：MIPS 浮点指令的 opcode
   （bits 31..26）落在 COP1 族（0x11 COP1 / 0x31 LWC1 / 0x35 LDC1 / 0x39 SWC1 /
   0x3D SDC1）。softfloat 产物该计数为 0，hardfloat 产物为成百上千。
   若 .gnu.attributes 缺失（更老的 Go / 非 Go 工具链），退回此判据。

用法
----
    python3 tools/mips_fpu_check.py <mips 二进制> <mipsle 二进制> [更多...]

退出码 0 = 全部合格；1 = 检出硬浮点（或文件无法解析）；2 = 用法错误。

注：本脚本只在 macOS / Linux / CI 上作为发布前的体检用，不参与运行时。
"""

import struct
import sys

# MIPS FPU 指令的 opcode（bits 31..26）：COP1 与 cop1 的访存指令
FPU_OPCODES = frozenset((0x11, 0x31, 0x35, 0x39, 0x3D))
# Tag_GNU_MIPS_ABI_FP 的取值（binutils include/elf/mips.h）
FP_ABI_SOFT = 3
FP_ABI_NAMES = {
    0: "any",
    1: "hard-float double",
    2: "hard-float single",
    3: "soft-float",
    4: "hard-float old-64",
    5: "hard-float xx",
    6: "hard-float 64",
    7: "hard-float 64a",
}


class ElfError(Exception):
    pass


def _uleb128(blob, i):
    """解析 uleb128，返回 (值, 下一个下标)。"""
    val = 0
    shift = 0
    while True:
        if i >= len(blob):
            raise ElfError("uleb128 越界")
        b = blob[i]
        i += 1
        val |= (b & 0x7F) << shift
        if not (b & 0x80):
            return val, i
        shift += 7


def load_sections(path):
    """读 ELF（32/64 位、任意字节序）的段表，返回 (endian, blob, {段名: (shoff, size, flags)})。"""
    with open(path, "rb") as fp:
        blob = fp.read()
    if len(blob) < 52 or blob[:4] != b"\x7fELF":
        raise ElfError("不是合法的 ELF 文件")
    endian = "<" if blob[5] == 1 else ">"
    cls = blob[4]
    if cls == 1:  # ELF32
        e_shoff = struct.unpack_from(endian + "I", blob, 32)[0]
        e_shentsize, e_shnum, e_shstrndx = struct.unpack_from(endian + "HHH", blob, 46)
        def sec_at(off):
            name, typ = struct.unpack_from(endian + "II", blob, off)
            flags, addr, shoff, size = struct.unpack_from(endian + "IIII", blob, off + 8)
            return name, typ, flags, addr, shoff, size
    elif cls == 2:  # ELF64
        e_shoff = struct.unpack_from(endian + "Q", blob, 40)[0]
        e_shentsize, e_shnum, e_shstrndx = struct.unpack_from(endian + "HHH", blob, 58)
        def sec_at(off):
            name, typ = struct.unpack_from(endian + "II", blob, off)
            flags, addr, shoff, size = struct.unpack_from(endian + "QQQQ", blob, off + 8)
            return name, typ, flags, addr, shoff, size
    else:
        raise ElfError(f"不支持的 ELF class: {cls}")
    secs = [sec_at(e_shoff + i * e_shentsize) for i in range(e_shnum)]
    if not (0 <= e_shstrndx < len(secs)):
        raise ElfError("缺少节名字符串表")
    shstr_name, _, _, _, shstr_off, shstr_size = secs[e_shstrndx]
    shstr = blob[shstr_off:shstr_off + shstr_size]
    by_name = {}
    for name, typ, flags, addr, shoff, size in secs:
        end = shstr.find(b"\0", name)
        n = shstr[name:end].decode(errors="replace") if end >= 0 else ""
        if n:
            by_name[n] = (shoff, size, flags)
    return endian, blob, by_name


def gnu_fp_abi(blob, endian):
    """解析 .gnu.attributes 段，返回 Tag_GNU_MIPS_ABI_FP 的值；缺失则 None。

    段格式（binutils 规范）：
        'A'(1) + length(4) + vendor 字符串(以\\0 结尾)
        + 子节: tag(uleb128) + length(4) + 属性序列(各为 tag uleb128 + value uleb128)
    """
    if not blob or blob[0] != 0x41:
        return None
    total = struct.unpack_from(endian + "I", blob, 1)[0]
    end = min(1 + total, len(blob))
    i = 5
    while i < end and blob[i] != 0:
        i += 1
    i += 1  # 跳过 vendor 字符串
    while i < end - 4:
        tag, i = _uleb128(blob, i)
        slen = struct.unpack_from(endian + "I", blob, i)[0]
        i += 4
        sub_end = min(i + slen, end)
        if tag == 1:  # Tag_File
            while i < sub_end - 1:
                atag, i = _uleb128(blob, i)
                val, i = _uleb128(blob, i)
                if atag == 4:  # Tag_GNU_MIPS_ABI_FP
                    return val
        else:
            i = sub_end
    return None


def fpu_instruction_count(blob, endian, sections):
    """扫描所有可执行段，返回 MIPS FPU 指令（COP1 族 opcode）的数量。"""
    total = 0
    for shoff, size, flags in sections.values():
        if not (flags & 4):  # SHF_EXECINSTR
            continue
        if size < 4 or shoff + size > len(blob):
            continue
        data = blob[shoff:shoff + size]
        for off in range(0, size - 3, 4):
            word = struct.unpack_from(endian + "I", data, off)[0]
            if (word >> 26) & 0x3F in FPU_OPCODES:
                total += 1
    return total


def check(path):
    """体检单个文件。返回 (ok: bool, 描述 str)。"""
    name = path.split("/")[-1]
    try:
        endian, blob, sections = load_sections(path)
    except ElfError as exc:
        return False, f"✗ {name}：无法解析 ELF（{exc}），跳过校验会放行隐患，按失败处理"
    if not sections:
        return False, f"✗ {name}：ELF 内没有任何节，无法校验"

    abi = None
    sec = sections.get(".gnu.attributes")
    if sec is not None:
        shoff, size, _ = sec
        abi = gnu_fp_abi(blob[shoff:shoff + size], endian)

    fpu_cnt = fpu_instruction_count(blob, endian, sections)

    marker_ok = abi == FP_ABI_SOFT
    instr_ok = fpu_cnt == 0

    # 逐项拼体检结论
    if abi is None:
        abi_txt = "无 .gnu.attributes 标记（退回指令扫描）"
    elif abi == FP_ABI_SOFT:
        abi_txt = "ABI 标记 soft-float（=3）"
    else:
        abi_txt = f"ABI 标记 {FP_ABI_NAMES.get(abi, '未知(' + str(abi) + ')')}（≠3）"
    if fpu_cnt == 0:
        instr_txt = ".text 无 FPU 指令"
    else:
        instr_txt = f".text 检出 {fpu_cnt} 条 FPU 指令（COP1 族）"

    if marker_ok and instr_ok:
        return True, f"✓ {name}：{abi_txt}，{instr_txt}，softfloat 合格"
    if not marker_ok and not instr_ok:
        return False, f"✗ {name}：{abi_txt}，{instr_txt} —— 硬浮点，改回 GOMIPS=softfloat 重新编译"
    if not marker_ok:
        return False, f"✗ {name}：{abi_txt}，{instr_txt} —— ABI 标记即 hardfloat，改回 GOMIPS=softfloat 重新编译"
    return False, f"✗ {name}：{abi_txt}，{instr_txt} —— 标记与指令矛盾，按硬浮点处理"


def main(argv):
    if len(argv) < 2:
        print(__doc__)
        return 2
    bad = 0
    for path in argv[1:]:
        ok, msg = check(path)
        print(msg)
        if not ok:
            bad = 1
    if bad:
        print("\n建议：给 mips / mipsle 目标加上 GOMIPS=softfloat 后清掉 .build 重新构建。")
    return bad


if __name__ == "__main__":
    sys.exit(main(sys.argv))
