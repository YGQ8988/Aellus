#!/bin/bash
# 计算 templates/ + static/ 的内容指纹（sha256），供 ldflags -X 注入二进制。
#
# 为什么需要它：前端是 //go:embed 塞进二进制的，「改了前端但二进制没重编」从外面
# 完全看不出来 —— 版本号照常注入、包照常能装。光比对「文件名清单」不够：
# 模板内容被改（哪怕只是加回一行 <script>）而二进制是旧的，清单一模一样。
# 所以把【整个前端树的内容哈希】在编译时写进二进制（internal/app.BuildStaticHash），
# 发布前的体检（tools/embed_check.py）重算一遍源树指纹、与二进制里记录的比对，
# 不一致就是「这份二进制不是当前源码编的」，直接中止发布。
#
# 算法（tools/embed_check.py 的 static_hash() 与此严格对应，改动必须两边同步）：
#   1. 收集 templates/ 与 static/ 下所有普通文件，跳过 . / _ 开头的条目
#      （与 Go embed 的约定一致，隐藏文件本来就不会进二进制）
#   2. 按 LC_ALL=C 字节序对相对路径排序
#   3. 依次喂给 sha256：相对路径 + \0 + 文件内容 + \0
#
# 用法：STATIC_HASH="$(bash tools/static_hash.sh)"
set -e
cd "$(dirname "$0")/.."

# macOS 是 shasum -a 256，Linux 是 sha256sum
SHA_CMD="sha256sum"
command -v sha256sum >/dev/null 2>&1 || SHA_CMD="shasum -a 256"

find templates static -type f ! -name '.*' ! -name '_*' \
  | LC_ALL=C sort \
  | while IFS= read -r f; do
      printf '%s\0' "$f"
      cat "$f"
      printf '\0'
    done | $SHA_CMD | awk '{print $1}'
