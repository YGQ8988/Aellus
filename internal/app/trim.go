package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// === 飞牛开放 API 与授权目录（纯函数，无 App 状态） ===

// authorizedSavePaths 返回当前应用可访问的授权目录（去重后的真实绝对路径）：
// 合并两个来源——飞牛官方 API trim.file.getSharedAccessibleFolders 查询的共享授权目录，
// 以及环境变量 TRIM_DATA_ACCESSIBLE_PATHS / TRIM_DATA_SHARE_PATHS（data-share 共享目录 +
// 管理员在应用设置授权的目录）。两者可能只覆盖部分目录，合并取并集再按真实路径去重，
// 避免「API 只返回 1 个目录时漏掉应用设置里授权的其它目录」。
func authorizedSavePaths() []string {
	raw := []string{}
	// 1) 官方 API 查询共享授权目录（失败静默，交给环境变量兜底）
	if paths, err := trimQuerySharedFolders(); err == nil {
		raw = append(raw, paths...)
	}
	// 2) 环境变量补充（data-share 共享目录 + 用户授权目录，冒号分隔）
	for _, key := range []string{"TRIM_DATA_ACCESSIBLE_PATHS", "TRIM_DATA_SHARE_PATHS"} {
		if v := os.Getenv(key); v != "" {
			raw = append(raw, splitPathList(v)...)
		}
	}
	// 3) 解析每个授权根为真实绝对路径（跟随飞牛可能注入的软链，如 @appshare），
	// 让边界校验对齐真实存储位置，避免软链绕过，同时去重。
	seen := map[string]bool{}
	var out []string
	for _, p := range raw {
		if p == "" {
			continue
		}
		real, err := realResolve(p)
		if err != nil {
			real = filepath.Clean(p)
		}
		if !seen[real] {
			seen[real] = true
			out = append(out, real)
		}
	}
	return out
}

// withinAuthRoots 判断 p 是否在任一授权根目录内（p 等于根或为其子路径）。
// 使用 realInside 做 symlink 增强：即使 p 经软链指向授权外真实位置也会被拒绝。
func withinAuthRoots(p string, roots []string) bool {
	cp := filepath.Clean(p)
	for _, root := range roots {
		cr := filepath.Clean(root)
		if cp == cr || strings.HasPrefix(cp, cr+string(filepath.Separator)) {
			// 字符串落在授权根内【还不够】：p 本身或其任一层父目录可能是软链，
			// 真实位置在授权之外（共享目录对局域网可写，谁都能建这种软链）。
			// 若只凭字符串前缀就放行，软链即可把保存目录引到授权边界之外——
			// 后续 realInside(saveDir, ...) 会把双方都解析成外部路径，判定为"内部"。
			// 故命中后必须再过一次真实路径校验。
			return realInside(root, p)
		}
		// 字符串不在根内，但解析软链后可能确实落在里面（如 root 自身是软链）。
		if realInside(root, p) {
			return true
		}
	}
	return false
}

// IsPersistedSaveDirValid 判断持久化的保存目录在飞牛授权边界下是否仍然有效。
// 飞牛端要求路径仍落在「允许的根」内（授权目录 + 出厂默认目录；管理员可能在应用设置里
// 移除授权，导致已持久化路径失效）；取不到任何根时视为无效。
// 桌面端无授权边界，调用方不应在非飞牛环境调用此函数。
func IsPersistedSaveDirValid(dir string) bool {
	roots := saveDirAllowedRoots()
	if len(roots) == 0 {
		return false
	}
	return withinAuthRoots(dir, roots)
}

// saveDirAllowedRoots 返回「允许作为保存目录的根」= 飞牛授权目录 + 飞牛注入的出厂默认目录。
//
// 默认目录位于应用私有数据目录（界面展示为「存储空间N/应用文件/{appname}/drops」），
// 通常不会出现在飞牛「授权目录」列表里，但它是启动脚本（cmd/main）分配的合法落盘位置、
// 也是应用的出厂默认值，因此必须与授权目录并列允许——否则用户改成其它目录后，
// 再手动选回默认目录会被误报「保存目录必须位于飞牛已授权目录内」。
func saveDirAllowedRoots() []string {
	roots := authorizedSavePaths()
	if d := bootDefaultSaveDir(); d != "" {
		roots = append(roots, d)
	}
	return roots
}

// trimQuerySharedFolders 调用飞牛官方后端 API 查询共享授权目录（trim.file.getSharedAccessibleFolders，
// Scope: trim.file.sharedAccess，要求 fnOS >= 1.2.0401、App >= 1.34.0）。
func trimQuerySharedFolders() ([]string, error) {
	rawData, err := trimBackendAPI("trim.file.getSharedAccessibleFolders", nil)
	if err != nil {
		return nil, err
	}
	// data 可能是 {paths:[...]} 对象，也可能是直接的路径数组（不同飞牛版本实现差异），兼容解析。
	var obj struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(rawData, &obj); err == nil && obj.Paths != nil {
		out := make([]string, 0, len(obj.Paths))
		for _, s := range obj.Paths {
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	}
	var arr []string
	if err := json.Unmarshal(rawData, &arr); err == nil {
		out := make([]string, 0, len(arr))
		for _, s := range arr {
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	}
	log.Printf("[trim] getSharedAccessibleFolders 返回结构无法解析: %s", string(rawData))
	return nil, errors.New("返回格式异常")
}

// trimConvertPaths 调用飞牛后端 API trim.file.convertPath（Scope: trim.file.path）
// 把内部路径（如 /vol1/1000/photo）转换成语义化展示路径（如「存储空间1/admin 的文件/photo」），
// 供设置页面展示。返回 map[原始路径]语义路径；调用失败返回空 map（调用方回退展示原始路径）。
// 要求 fnOS >= 1.2.0401、App >= 1.34.0。
func trimConvertPaths(paths []string) map[string]string {
	out := map[string]string{}
	if len(paths) == 0 {
		return out
	}
	rawData, err := trimBackendAPI("trim.file.convertPath", map[string]interface{}{
		"path":     paths,
		"language": "zh-CN",
	})
	if err != nil {
		log.Printf("[trim] convertPath 失败（回退展示原始路径）: %v", err)
		return out
	}
	// data 可能是 {status, result:[{path,semanticPath},...]} 对象，也可能是直接的 result 数组
	// （不同飞牛版本实现有差异），两者都兼容解析。
	type convItem struct {
		Path         string `json:"path"`
		SemanticPath string `json:"semanticPath"`
	}
	var obj struct {
		Result []convItem `json:"result"`
	}
	if err := json.Unmarshal(rawData, &obj); err == nil && obj.Result != nil {
		for _, it := range obj.Result {
			if it.Path != "" && it.SemanticPath != "" {
				out[it.Path] = it.SemanticPath
			}
		}
		return out
	}
	var arr []convItem
	if err := json.Unmarshal(rawData, &arr); err == nil {
		for _, it := range arr {
			if it.Path != "" && it.SemanticPath != "" {
				out[it.Path] = it.SemanticPath
			}
		}
		return out
	}
	log.Printf("[trim] convertPath 返回结构无法解析（回退展示原始路径）: %s", string(rawData))
	return out
}

// trimBackendAPI 调用飞牛官方后端开放 API（POST /api/v1/trimapp，经 Unix Socket 访问）。
// 认证：Authorization: Bearer <TRIM_API_TOKEN>（系统启动应用脚本时注入，每次调用现取，不持久化）。
// 返回 data 字段的原始 JSON（不同接口的 data 结构不同，可能是对象或数组，交给调用方解析）。
func trimBackendAPI(req string, data interface{}) (json.RawMessage, error) {
	token := os.Getenv("TRIM_API_TOKEN")
	if token == "" {
		return nil, errors.New("TRIM_API_TOKEN 未设置")
	}
	payload, err := json.Marshal(map[string]interface{}{
		"reqId":   strconv.FormatInt(time.Now().UnixNano(), 10),
		"req":     req,
		"appName": "Aellus",
		"data":    data,
	})
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", trimAPISocket)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	httpReq, err := http.NewRequest(http.MethodPost, "http://localhost/api/v1/trimapp", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	// 限长读取：该响应来自本机飞牛的 Unix Socket 服务，正常很小；加个上限避免
	// 对端异常/被攻破时用超大响应占内存。
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("trim api %s 失败: code=%d msg=%s", req, out.Code, out.Msg)
	}
	return out.Data, nil
}

// splitPathList 按 : 分隔路径列表，但忽略 Windows 盘符冒号（如 E:\... 中的冒号不视为分隔符）。
// 飞牛(Linux)路径如 /vol1/a:/vol1/b 正常按 : 切分。
func splitPathList(s string) []string {
	var out []string
	var cur strings.Builder
	rs := []rune(s)
	isAlpha := func(c rune) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
	for i, c := range rs {
		if c == ':' {
			// 盘符冒号：前一个字符是字母且后一个是 \ 或 /，视为路径的一部分，不切分
			if i > 0 && i+1 < len(rs) && isAlpha(rs[i-1]) && (rs[i+1] == '\\' || rs[i+1] == '/') {
				cur.WriteRune(c)
				continue
			}
			if cur.Len() > 0 {
				out = append(out, strings.TrimSpace(cur.String()))
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(c)
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}
