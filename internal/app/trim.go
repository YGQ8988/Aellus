package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// === 飞牛开放 API 与授权目录（纯函数，无 App 状态） ===

// hasTrimAPI 是否具备飞牛开放 API 接入环境（token 存在且 socket 可达）。
func hasTrimAPI() bool {
	if os.Getenv("TRIM_API_TOKEN") == "" {
		return false
	}
	_, err := os.Stat(trimAPISocket)
	return err == nil
}

// authorizedSavePaths 返回当前应用可访问的授权目录：
// 优先调用飞牛官方后端 API trim.file.getSharedAccessibleFolders 查询共享授权目录；
// 调用失败（非飞牛环境 / token 缺失 / socket 不存在 / 接口报错）时回退到环境变量
// TRIM_DATA_ACCESSIBLE_PATHS / TRIM_DATA_SHARE_PATHS。
func authorizedSavePaths() []string {
	var raw []string
	if paths, err := trimQuerySharedFolders(); err == nil && len(paths) > 0 {
		raw = paths
	} else {
		for _, key := range []string{"TRIM_DATA_ACCESSIBLE_PATHS", "TRIM_DATA_SHARE_PATHS"} {
			if v := os.Getenv(key); v != "" {
				raw = append(raw, splitPathList(v)...)
			}
		}
	}
	// 解析每个授权根为真实绝对路径（跟随飞牛可能注入的软链，如 @appshare），
	// 让边界校验对齐真实存储位置，避免软链绕过。
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
			return true
		}
		// symlink 增强：p 或 root 为软链时，解析真实路径再比对。
		if realInside(root, p) {
			return true
		}
	}
	return false
}

// trimQuerySharedFolders 调用飞牛官方后端 API 查询共享授权目录（trim.file.getSharedAccessibleFolders，
// Scope: trim.file.sharedAccess，要求 fnOS >= 1.2.0401、App >= 1.34.0）。
func trimQuerySharedFolders() ([]string, error) {
	data, err := trimBackendAPI("trim.file.getSharedAccessibleFolders", nil)
	if err != nil {
		return nil, err
	}
	raw, ok := data["paths"].([]interface{})
	if !ok {
		return nil, errors.New("返回格式异常")
	}
	var out []string
	for _, p := range raw {
		if s, ok := p.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

// trimBackendAPI 调用飞牛官方后端开放 API（POST /api/v1/trimapp，经 Unix Socket 访问）。
// 认证：Authorization: Bearer <TRIM_API_TOKEN>（系统启动应用脚本时注入，每次调用现取，不持久化）。
func trimBackendAPI(req string, data interface{}) (map[string]interface{}, error) {
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
		Code int                    `json:"code"`
		Msg  string                 `json:"msg"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
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
