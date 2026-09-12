package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 本文件是安全回归测试：把「曾经出现过 / 差点出现」的问题固化成断言，
// 以后任何改动把这些行为改回去，`go test ./...` 会当场红灯，
// 而不是依赖某次人工通读代码去发现。
//
// 覆盖：
//  1. 文件输出（下载 / 缩略图）不得把上传的 HTML/SVG 等可执行类型内联返回；
//  2. 会改变状态的接口必须携带 X-Aellus-Client（防 CSRF）；
//  3. 裸端口入口必须剥离伪造的 X-Trim-* 身份头；
//  4. 上传不得顺着软链写到授权目录之外；
//  5. 权限判定矩阵（桌面端 / 飞牛端 / 网关是否在线）；
//  6. 授权目录相关接口不对局域网开放；
//  7. 设备名映射的容量与长度上限。

// secTestPlatform 是 Platform 接口的最小实现，供本文件测试使用。
type secTestPlatform struct {
	fnos bool
	dir  string
}

func (secTestPlatform) RunTray(string)                              {}
func (secTestPlatform) PostOpenNotification(string, string, string) {}
func (secTestPlatform) EnforceSingleInstance() bool                 { return true }
func (secTestPlatform) PickFolderDialog() string                    { return "" }
func (secTestPlatform) PickDirSupported() bool                      { return false }
func (secTestPlatform) PersistSaveDirAllowed() bool                 { return false }
func (p secTestPlatform) EnforceAuthBoundary() bool                 { return p.fnos }
func (p secTestPlatform) ConfigBaseDir(string) string               { return p.dir }
func (p secTestPlatform) LogsDir() string                           { return p.dir }

// TestFileOutputNeverInlinesScriptable 覆盖 P1：可携带脚本 / 标记的类型不得内联。
func TestFileOutputNeverInlinesScriptable(t *testing.T) {
	for _, name := range []string{"a.html", "a.htm", "a.svg", "a.svgz", "a.xhtml", "a.xml", "a.js", "a.mhtml"} {
		for _, mode := range []fileOutputMode{outputInlineImage, outputInlineMedia, outputDownload} {
			w := httptest.NewRecorder()
			setFileOutputHeaders(w, name, mode)
			if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
				t.Errorf("%s mode=%d: Content-Type=%q，期望 application/octet-stream", name, mode, got)
			}
			if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
				t.Errorf("%s mode=%d: 未强制附件下载（浏览器可能内联渲染执行脚本）", name, mode)
			}
		}
	}
	// 图片必须仍可内联（否则列表缩略图无法显示）。
	if c := inlineContentType(".png", outputInlineImage); !strings.HasPrefix(c, "image/") {
		t.Errorf(".png 应允许内联，实际 %q", c)
	}
	// 下载预览模式：非图片的媒体类型（PDF 等）仍可内联。
	if c := inlineContentType(".pdf", outputInlineMedia); c == "" {
		t.Error(".pdf 在下载预览模式应允许内联")
	}
}

// TestThumbServingUploadedHTMLIsAttachment 是 P1 的端到端回归：
// 上传一个 .html，再经缩略图接口取回，必须是附件下载而不是 text/html。
func TestThumbServingUploadedHTMLIsAttachment(t *testing.T) {
	dir := t.TempDir()
	a := &App{platform: secTestPlatform{dir: dir}, absSaveDir: dir}
	if err := os.WriteFile(filepath.Join(dir, "evil.html"), []byte(`<script>alert(1)</script>`), 0644); err != nil {
		t.Fatal(err)
	}
	mux := a.buildMux(8000)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/thumb?dir=&file=evil.html", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("缩略图接口状态码 %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("上传的 html 被当作 %q 输出（存储型 XSS 回归）", ct)
	}
	if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Error("上传的 html 未强制附件下载（存储型 XSS 回归）")
	}
}

// TestStateChangeRequiresClientHeader 覆盖 P2：跨站请求（浏览器无法携带自定义头）必须被拒，
// 同时确认正常路径（本机 + 标识头）仍然可用。
func TestStateChangeRequiresClientHeader(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/delete", nil)
	if isTrustedStateChange(r) {
		t.Error("无标识头的状态变更请求不应被视为可信")
	}
	r.Header.Set(clientHeaderName, "1")
	if !isTrustedStateChange(r) {
		t.Error("带标识头应被视为可信")
	}
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if isTrustedStateChange(r) {
		t.Error("浏览器声明跨站时应拒绝（纵深防御）")
	}

	dir := t.TempDir()
	a := &App{platform: secTestPlatform{dir: dir}, absSaveDir: dir}
	mux := a.buildMux(8000)
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	do := func(remote string, hdr map[string]string) int {
		req := httptest.NewRequest("POST", "/api/delete", strings.NewReader(`{"dir":"","file":"victim.txt"}`))
		req.RemoteAddr = remote
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}

	if code := do("192.168.1.50:1234", nil); code != http.StatusForbidden {
		t.Errorf("跨站删除应 403，实际 %d", code)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("跨站请求不应删除文件")
	}
	if code := do("192.168.1.50:1234", map[string]string{clientHeaderName: "1"}); code != http.StatusForbidden {
		t.Errorf("局域网设备带标识头删除应 403（非本机、无网关头），实际 %d", code)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("局域网设备不应删除文件")
	}
	if code := do("127.0.0.1:1234", map[string]string{clientHeaderName: "1"}); code != http.StatusOK {
		t.Errorf("本机 + 标识头删除应 200，实际 %d", code)
	}
}

// TestStripTrimHeadersOnRawPort 覆盖网关头信任模型：裸端口入口剥离客户端伪造的 X-Trim-*。
func TestStripTrimHeadersOnRawPort(t *testing.T) {
	var a App
	var seen string
	h := a.stripTrimHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Trim-Userid") + "|" + r.Header.Get("X-Trim-Username")
	}))
	req := httptest.NewRequest("POST", "/api/delete", nil)
	req.Header.Set("X-Trim-Userid", "1000")
	req.Header.Set("X-Trim-Username", "admin")
	req.Header.Set("Deviceid", "d1")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "|" {
		t.Errorf("裸端口入口应剥离伪造的 X-Trim-*，实际 %q", seen)
	}
}

// TestUploadRejectsSymlinkEscape 覆盖 P3：保存目录内存在软链时不得写到目录之外。
func TestUploadRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "OUTSIDE")
	os.MkdirAll(filepath.Join(root, "dev"), 0755)
	os.MkdirAll(outside, 0755)
	os.Symlink(outside, filepath.Join(root, "linked"))     // 设备目录本身是软链
	os.Symlink(outside, filepath.Join(root, "dev", "sub")) // 子目录是软链

	if realInside(root, filepath.Join(root, "linked")) {
		t.Error("realInside 应识别设备目录的软链逃逸")
	}
	if _, _, err := resolveUploadTarget(root, filepath.Join(root, "linked"), "x.txt"); err == nil {
		t.Error("设备目录为软链时应拒绝上传")
	}
	if _, _, err := resolveUploadTarget(root, filepath.Join(root, "dev"), "sub/x.txt"); err == nil {
		t.Error("子目录为软链时应拒绝上传")
	}
	dst, _, err := resolveUploadTarget(root, filepath.Join(root, "dev"), "ok.txt")
	if err != nil || !strings.HasPrefix(dst, root) {
		t.Errorf("正常上传路径不应被拒绝：dst=%q err=%v", dst, err)
	}
}

// TestCanManageRules 固化权限判定矩阵，含「飞牛网关注册成功后不再认本机来源」（防 NAS 上
// 本机进程 / 容器网桥地址越权）以及网关未起来时的回退行为。
func TestCanManageRules(t *testing.T) {
	dir := t.TempDir()
	mkReq := func(remote string, hdr map[string]string) *http.Request {
		r := httptest.NewRequest("POST", "/api/delete", nil)
		r.RemoteAddr = remote
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return r
	}
	const lan = "192.168.1.50:1234"
	gwHdr := map[string]string{"X-Trim-Userid": "1000"}

	desktop := &App{platform: secTestPlatform{dir: dir}, absSaveDir: dir}
	fnos := &App{platform: secTestPlatform{dir: dir, fnos: true}, absSaveDir: dir}
	fnosGw := &App{platform: secTestPlatform{dir: dir, fnos: true}, absSaveDir: dir}
	fnosGw.gatewayActive.Store(true)

	cases := []struct {
		name string
		a    *App
		r    *http.Request
		want bool
	}{
		{"桌面端本机可管理", desktop, mkReq("127.0.0.1:1", nil), true},
		{"桌面端局域网设备不可管理", desktop, mkReq(lan, nil), false},
		{"飞牛网关注入的身份头可管理", fnos, mkReq(lan, gwHdr), true},
		{"飞牛网关未起来时回退本机判定", fnos, mkReq("127.0.0.1:1", nil), true},
		{"飞牛网关在线时本机来源不可管理", fnosGw, mkReq("127.0.0.1:1", nil), false},
		{"飞牛网关在线时网关头仍可管理", fnosGw, mkReq(lan, gwHdr), true},
	}
	for _, c := range cases {
		if got := c.a.canManage(c.r); got != c.want {
			t.Errorf("%s: canManage=%v，期望 %v", c.name, got, c.want)
		}
	}
}

// TestAuthPathsNotOpenToLAN 覆盖 N2：授权目录相关接口只对管理者开放。
func TestAuthPathsNotOpenToLAN(t *testing.T) {
	dir := t.TempDir()
	a := &App{platform: secTestPlatform{dir: dir, fnos: true}, absSaveDir: dir}
	mux := a.buildMux(8000)
	for _, p := range []string{"/api/authpaths", "/api/listdir"} {
		req := httptest.NewRequest("GET", p, nil)
		req.RemoteAddr = "192.168.1.50:1234"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s 局域网设备应 403，实际 %d", p, w.Code)
		}
	}
}

// TestDeviceNameCaps 覆盖设备名映射的容量 / 长度上限（局域网任何人都能上传，防止被撑爆）。
func TestDeviceNameCaps(t *testing.T) {
	dir := t.TempDir()
	a := &App{platform: secTestPlatform{dir: dir}, absSaveDir: dir}
	longID := strings.Repeat("i", 200)
	a.recordDeviceName(longID, strings.Repeat("x", 200))
	if got := a.deviceNameOf(longID); len([]rune(got)) != maxDeviceNameLen {
		t.Errorf("设备名应被截断为 %d 字符，实际 %d", maxDeviceNameLen, len([]rune(got)))
	}
	for i := 0; i < maxDeviceNames+50; i++ {
		a.recordDeviceName("id-"+strconv.Itoa(i), "n")
	}
	b, err := os.ReadFile(a.deviceNamesPath())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) > maxDeviceNames {
		t.Errorf("设备名映射条目数应 ≤ %d，实际 %d", maxDeviceNames, len(m))
	}
}
