#!/usr/bin/env python3
"""校验「编译出来的二进制」与「当前源码时的静态资源」是否一致。

为什么需要这道检查
------------------
Aellus 的前端（templates/ 与 static/）是通过 //go:embed 在**编译期**写进可执行文件的，
运行时不再读取磁盘上的源文件。这带来一个隐蔽的坑：

    只要二进制没有被重新编译（Go 构建缓存命中、或构建脚本复用了上一次留下的产物），
    包里的程序就是旧源码 —— 连 undeleted 的前端文件的名字都还是旧列表。

现象非常具有迷惑性：版本号正常递增（r16 → r17），包也能正常装上，但功能毫无变化，
看着像「改了没生效 / 修复失败」。真实原因只是二进制没重编。本次就是这么踩到的：
.build/openwrt/bin 下残留了旧二进制，构建脚本里 `同一 GOARCH 已编译过就跳过` 的判断
只看文件存在与否，于是直接复用了旧产物，生成的 ipk 里根本没有新加的静态资源。

因此这里反过来看另一件事：**二进制里到底有没有当前源树里的静态文件**。
embed FS 在二进制里会存一份「文件名 → 数据」的目录表，文件名以裸字符串形式留在
只读数据段里（即使编译时 -s -w 去掉了符号表，这些名字依然在），所以只要逐个检查
源树里的相对路径能否在二进制里被找到，就能判断这份二进制是不是用当前源码编的。

用法
----
    python3 tools/embed_check.py <可执行文件或从包里解出的二进制> [更多文件...]

退出码 0 = 全部通过；1 = 有静态资源缺失（二进制是用旧源码编的，产物不可用）。

注：本脚本只在 macOS / Linux / CI 上作为发布前的体检用，不参与运行时。
"""

import hashlib
import os
import re
import subprocess
import sys

# 与 main.go 的 //go:embed 保持一致的两个目录。改这里之前先看 main.go。
EMBED_ROOTS = ("templates", "static")

# 每个页面模板都必须引用的公共脚本。
# 这是真实事故换来的规则：browse.html 曾在加 <script> 时手滑把 ui.js 挤掉，
# 页面直接白屏 —— $ / displayName / confirmDialog / toast 这些全局全在 ui.js 里
# （ui.js 里 window.$ = $），base.js 是最先加载的垫层。少引用一个，浏览器报
# 「ReferenceError: $ is not defined」，而构建侧一切正常。
COMMON_SCRIPTS = ("static/js/base.js", "static/js/ui.js")

# 模板里引用本地静态资源的写法：src="static/..." 或 href="static/..."（不含外链 http）
REF_RE = re.compile(r'(?:src|href)="(static/[^"]+)"')


def embed_paths(root):
    """列出源树里应当被 embed 进去的相对路径名。

    与 Go embed 的规则对齐：`dir/*` 会递归包含子目录内容，但跳过以 `.` 或 `_`
    开头的条目（Go 官方约定）。隐藏文件（如 .DS_Store）本来就不会进二进制，
    拿它们去比对只会误报。
    """
    out = []
    for base in EMBED_ROOTS:
        for dirpath, dirnames, filenames in os.walk(os.path.join(root, base)):
            dirnames[:] = [d for d in dirnames if not d.startswith((".", "_"))]
            for name in filenames:
                if name.startswith((".", "_")):
                    continue
                rel = os.path.relpath(os.path.join(dirpath, name), root)
                out.append(rel.replace(os.sep, "/"))
    return sorted(out)


def static_hash(root):
    """计算 templates/ + static/ 的内容指纹，与 tools/static_hash.sh 严格对应。

    这是比「文件名清单」强得多的凭据：清单查不出「模板内容改了、二进制没重编」
    ——文件还是那些文件，内容已经不一样。构建脚本在编译时把这份指纹用 ldflags
    -X 写进二进制（internal/app.BuildStaticHash），这里重算一遍去二进制里找，
    找不到就说明二进制是别的源码状态编出来的。
    """
    files = []
    for base in EMBED_ROOTS:
        for dirpath, dirnames, filenames in os.walk(os.path.join(root, base)):
            dirnames[:] = [d for d in dirnames if not d.startswith((".", "_"))]
            for name in filenames:
                if name.startswith((".", "_")):
                    continue
                files.append(os.path.relpath(os.path.join(dirpath, name), root))
    h = hashlib.sha256()
    for rel in sorted(files):
        h.update(rel.encode())
        h.update(b"\0")
        with open(os.path.join(root, rel), "rb") as fp:
            h.update(fp.read())
        h.update(b"\0")
    return h.hexdigest()


def check(path, expect):
    with open(path, "rb") as fp:
        blob = fp.read()
    missing = [rel for rel in expect if rel.encode() not in blob]
    return missing


def template_problems(root):
    """检查每个页面模板：引用的本地资源必须存在，公共脚本必须都在。

    「引用了不存在的文件」与「该引的没引」是同一类手滑的两种表现，
    浏览器端的报错（404 / ReferenceError）要等到用户打开页面才暴露，
    而这里在构建时就能拦下。
    """
    problems = []
    tdir = os.path.join(root, "templates")
    for name in sorted(os.listdir(tdir)):
        if not name.endswith(".html"):
            continue
        with open(os.path.join(tdir, name), encoding="utf-8") as fp:
            html = fp.read()
        refs = REF_RE.findall(html)
        for rel in refs:
            path = rel.split("?", 1)[0].split("#", 1)[0]
            if not os.path.exists(os.path.join(root, path)):
                problems.append(f"{name}: 引用了不存在的资源 {rel}")
        for must in COMMON_SCRIPTS:
            if must not in refs:
                problems.append(f"{name}: 缺少公共脚本引用 {must}"
                                f"（$/displayName/confirmDialog 等全局在其中）")
    return problems


def main(argv):
    if len(argv) < 2:
        print(__doc__)
        return 2
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    expect = embed_paths(root)
    fingerprint = static_hash(root)
    bad = 0
    tprobs = template_problems(root)
    if tprobs:
        bad = 1
        print("✗ 模板引用检查未通过：")
        for p in tprobs:
            print(f"    {p}")
    else:
        print(f"✓ 模板引用检查：{len(os.listdir(os.path.join(root, 'templates')))} 个模板的"
              f"本地资源引用完整，公共脚本（base.js / ui.js）都在")
    for target in argv[1:]:
        name = os.path.basename(target)
        with open(target, "rb") as fp:
            blob = fp.read()
        missing = [rel for rel in expect if rel.encode() not in blob]
        has_fp = fingerprint.encode() in blob
        if missing or not has_fp:
            bad = 1
            if missing:
                print(f"✗ {name}：二进制里缺少 {len(missing)}/{len(expect)} 个静态资源，"
                      f"它是用旧源码编出来的，产物不可用")
                for rel in missing[:10]:
                    print(f"    缺 {rel}")
                if len(missing) > 10:
                    print(f"    … 其余 {len(missing) - 10} 个省略")
            if not has_fp:
                print(f"✗ {name}：前端内容指纹不一致 —— 二进制不是用【当前】templates/static"
                      f" 编出来的（内容级变更未重编，或早于指纹机制的旧产物）")
        else:
            print(f"✓ {name}：{len(expect)} 个资源齐全，前端内容指纹一致（源码与二进制一致）")
    if bad:
        print("\n建议：确认中间产物已清理后重新构建（.build 目录与 Go 缓存都会导致复用旧二进制）。")
    return bad


if __name__ == "__main__":
    sys.exit(main(sys.argv))
