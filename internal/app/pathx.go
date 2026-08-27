package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// === 路径安全纯函数（无状态，可被任意包安全调用） ===

// sanitizeDevice 设备名安全过滤：只保留 字母、数字、中文、-、_
// 空值或过滤后为空时回退到 "default"——即用户没填设备名上传时，文件归入 default 子目录。
// 这一步防止设备名里塞入路径分隔符等危险字符。
func sanitizeDevice(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			(r >= 0x4e00 && r <= 0x9fa5) || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

// resolveUploadTarget 根据客户端上传时的“逻辑名”(可能是带目录的相对路径，
// 如 "MyFolder/sub/file.jpg"，来自网页内文件夹选择器) 计算最终落盘路径。
// 返回：dstPath(实际文件绝对路径)、displayName(用于回显的相对名)、error。
// 安全性：逐段 sanitize，拒绝 ".."/隐藏段/含分隔符的段；最终仍用 isInside 兜底防穿越。
func resolveUploadTarget(deviceDir, rawName string) (string, string, error) {
	rawName = strings.ReplaceAll(rawName, "\\", "/")
	segs := strings.Split(rawName, "/")
	cleanSegs := make([]string, 0, len(segs))
	for _, s := range segs {
		s = strings.TrimLeft(s, ".")
		if s == "" || s == "." || s == ".." {
			continue
		}
		// 只拒绝含路径分隔符的段（真正的越权/分隔问题）；
		// 文件名中正常出现的 ".."（如 clip..final.mp4、report..v2.pdf）属合法文件名，应放行。
		// 真正的目录穿越由末尾 isInside() 兜底拦截。
		if strings.ContainsAny(s, "/\\") {
			return "", "", fmt.Errorf("invalid path segment: %q", s)
		}
		cleanSegs = append(cleanSegs, s)
	}
	if len(cleanSegs) == 0 {
		return "", "", fmt.Errorf("empty filename")
	}

	base := cleanSegs[len(cleanSegs)-1]
	dirs := cleanSegs[:len(cleanSegs)-1]

	dirPart := deviceDir
	if len(dirs) > 0 {
		dirPart = filepath.Join(append([]string{deviceDir}, dirs...)...)
		if err := os.MkdirAll(dirPart, 0755); err != nil {
			return "", "", err
		}
	}

	var dstPath, displayName string
	if len(dirs) > 0 {
		// 文件夹上传：保留目录结构，文件名不再加时间戳前缀（结构已区分）。
		// 同名文件存在时自动加序号后缀，避免静默覆盖丢数据。
		dstPath = filepath.Join(dirPart, base)
		if _, e := os.Stat(dstPath); e == nil {
			ext := filepath.Ext(base)
			nameNoExt := strings.TrimSuffix(base, ext)
			for i := 1; ; i++ {
				candidate := fmt.Sprintf("%s_%d%s", nameNoExt, i, ext)
				candPath := filepath.Join(dirPart, candidate)
				if _, e := os.Stat(candPath); e == nil {
					continue
				}
				base = candidate
				dstPath = candPath
				break
			}
		}
		displayName = filepath.Join(append([]string{}, append(dirs, base)...)...)
	} else {
		// 普通文件：加时间戳前缀避免重名覆盖。
		ts := time.Now().Format("20060102_150405.000000")
		displayName = ts + "_" + base
		dstPath = filepath.Join(deviceDir, displayName)
	}

	if !isInside(deviceDir, dstPath) {
		return "", "", fmt.Errorf("path escapes device dir")
	}
	return dstPath, displayName, nil
}

// isValidName 判断一个“单段名字”（目录名或文件名）是否合法：
// 不能是空、不能是 . 或 ..、不能以 . 开头（隐藏）、不能包含路径分隔符或 ".."。
// 目录和文件在我们的设计里都只应该是单层名字，不允许有任何路径层级。
func isValidName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	return true
}

// isInside 判断 target 是否严格位于 root 目录内部（防路径穿越的核心校验）。
// 用 filepath.Rel 求相对路径，如果相对路径以 ".." 开头或就是 ".."，说明越界了。
func isInside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

// realResolve 将路径解析为真实绝对路径（跟随所有符号链接），用于防御 symlink 穿越。
// 目标路径可能尚不存在（如待创建的保存目录），此时先解析其存在的最深父目录，
// 再把末段名拼回，得到「若创建成功后的真实落点」，再做边界校验——这样既能校验
// 已存在路径，也能在校验阶段（MkdirAll 之前）拦截「symlink 指向授权外的待建目录」。
func realResolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	// 先尝试整体解析（路径已存在时跟随 symlink）。
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	// 不存在：逐级向上找存在的父目录，解析它，再拼回剩余段。
	cur := abs
	rest := ""
	for {
		parent := filepath.Dir(cur)
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			if rest == "" {
				return real, nil
			}
			return filepath.Join(real, rest), nil
		}
		if parent == cur {
			// 已到根仍不存在，退回 Clean 后的绝对路径。
			return filepath.Clean(abs), nil
		}
		if rest == "" {
			rest = filepath.Base(cur)
		} else {
			rest = filepath.Join(filepath.Base(cur), rest)
		}
		cur = parent
	}
}

// realInside 是 isInside 的「防 symlink 穿越」增强版：先把 root 与 target 都解析为
// 真实绝对路径（跟随符号链接），再做前缀校验。即使授权目录内存在指向外部的软链，
// 解析后的真实路径也会越界从而被拒绝。
func realInside(root, target string) bool {
	realRoot, err1 := realResolve(root)
	realTarget, err2 := realResolve(target)
	if err1 != nil || err2 != nil {
		// 解析失败（如 root 本身不存在且无法定位父目录）时退化为字符串校验，
		// 避免误拦合法请求，但仍保留 isInside 的基础穿越防护。
		return isInside(root, target)
	}
	rel, err := filepath.Rel(realRoot, realTarget)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}
