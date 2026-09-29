package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// 本文件是安全回归测试：把关键安全行为固化成断言，
// 以后任何改动把这些行为改回去，`go test ./...` 会当场红灯。
//
// 覆盖：
//  1. 文件输出（下载 / 缩略图）不得把上传的 HTML/SVG 等可执行类型内联返回；
//  2. 会改变状态的接口必须携带 X-Aellus-Client（防 CSRF）；
//  3. 裸端口入口必须剥离伪造的 X-Trim-* 身份头；
//  4. 上传不得顺着软链写到授权目录之外；
//  5. 权限判定矩阵（桌面端 / 飞牛端 / 网关是否在线）；
//  6. 授权目录相关接口不对局域网开放；
//  7. 设备名映射的容量与长度上限；
//  8. 缩略图只缩小、不放大（防「放大式」内存耗尽 DoS）；
//  9. 上传表单文本字段限长（防内存耗尽）、日志值清洗（防日志注入）；
// 10. 批量下载不得把隐藏目录内容（含 .aellus-tmp 上传临时文件）打包进 ZIP；
// 11. 批量下载的临时 ZIP 必须落在保存目录内，不得占用系统临时目录；
// 12. GIF 帧数统计必须在不解码像素的前提下得出（多帧解压炸弹防护的前置条件）。

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

// TestThumbNeverUpscales 覆盖缩略图内存放大：源图小于目标宽度时必须返回原文件，
// 绝不能把 1×100 放大成 240×24000（一条免登录请求可分配几百 MB 甚至几十 GB）。
func TestThumbNeverUpscales(t *testing.T) {
	dir := t.TempDir()
	a := &App{platform: secTestPlatform{dir: dir}, absSaveDir: dir}
	mux := a.buildMux(8000)

	tiny := image.NewRGBA(image.Rect(0, 0, 1, 100))
	var tinyBuf bytes.Buffer
	if err := png.Encode(&tinyBuf, tiny); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tiny.png"), tinyBuf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/thumb?dir=&file=tiny.png&w=240", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("细长图缩略状态码 %d", w.Code)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("解析返回内容失败: %v", err)
	}
	if cfg.Width != 1 || cfg.Height != 100 {
		t.Errorf("源图 1x100 不应被放大，实际输出 %dx%d", cfg.Width, cfg.Height)
	}

	// 宽图仍应正常缩小，且输出像素不得超过源图。
	wide := image.NewRGBA(image.Rect(0, 0, 1000, 10))
	var wideBuf bytes.Buffer
	if err := png.Encode(&wideBuf, wide); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wide.png"), wideBuf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/thumb?dir=&file=wide.png&w=240", nil))
	cfg, _, err = image.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("宽图缩略解析失败: %v", err)
	}
	if cfg.Width != 240 {
		t.Errorf("宽图缩略宽度应为 240，实际 %d", cfg.Width)
	}
	if cfg.Width*cfg.Height > 1000*10 {
		t.Errorf("缩略图输出像素 %d 超过源图像素 %d（发生放大）", cfg.Width*cfg.Height, 1000*10)
	}
}

// TestUploadFormFieldsBounded 覆盖上传表单文本字段的内存耗尽：
// 超长 device / rels 应被丢弃（内存有界），请求本身仍应成功。
func TestUploadFormFieldsBounded(t *testing.T) {
	dir := t.TempDir()
	a := &App{platform: secTestPlatform{dir: dir}, absSaveDir: dir}
	mux := a.buildMux(8000)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("device", strings.Repeat("A", 8<<20)) // 8MB 设备名字段
	fw, _ := mw.CreateFormFile("files", "x.txt")
	fw.Write([]byte("hi"))
	mw.WriteField("rels", strings.Repeat("B", 8<<20)) // 8MB rels 字段
	mw.Close()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(clientHeaderName, "1")
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	runtime.ReadMemStats(&m2)
	allocMB := float64(m2.TotalAlloc-m1.TotalAlloc) / 1048576

	if w.Code != http.StatusOK {
		t.Fatalf("小文件 + 超长文本字段应被接受（字段丢弃），实际 %d（body=%s）", w.Code, w.Body.String())
	}
	// 超长 device 被丢弃 → 回退 default；超长 rels 被丢弃 → 用 multipart 文件名。
	ents, err := os.ReadDir(filepath.Join(dir, "default"))
	if err != nil || len(ents) != 1 {
		t.Errorf("期望文件落在 default 目录且仅 1 个，实际 err=%v 条目=%v", err, ents)
	}
	if allocMB > 64 {
		t.Errorf("16MB 文本字段请求分配了 %.1f MB 内存，字段限长未生效", allocMB)
	}
	t.Logf("16MB 文本字段：HTTP %d，服务端分配 %.1f MB（限长生效）", w.Code, allocMB)
}

// TestLogValueSanitized 覆盖日志注入：文件名 / 目录名 / Deviceid 里的换行不得伪造出新日志行。
func TestLogValueSanitized(t *testing.T) {
	dir := t.TempDir()
	a := &App{platform: secTestPlatform{dir: dir}, absSaveDir: dir, operationLogPath: filepath.Join(dir, "operation.log")}
	a.logOp("删除 目录=x 目标=a\n2026-01-01 00:00:00  伪造的启动记录\tb")
	raw, err := os.ReadFile(a.operationLogPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("换行应被清洗为空格（只留 1 行），实际 %d 行：%q", len(lines), string(raw))
	}
	if strings.ContainsAny(string(raw), "\r\t") {
		t.Errorf("日志仍含控制字符：%q", string(raw))
	}
}

// TestVirtualIfaceNames 覆盖「容器流量不算本机」依赖的网卡名单：
// 漏判会导致容器里的进程被当成「运行应用的那台电脑」而拿到管理权。
func TestVirtualIfaceNames(t *testing.T) {
	virtual := []string{
		"lo0", "docker0", "br-1a2b3c4d", "veth1234", "bridge100",
		"vbr0", "ovs_eth0", "virbr0", "cni0", "flannel.1", "cali1234", "kube-ipvs0",
		"vmenet0", "vmbr0", "vmnet8", "vboxnet0", "utun3", "tun0", "wg0", "tailscale0",
	}
	for _, n := range virtual {
		if !isVirtualIface(n) {
			t.Errorf("%s 应判为虚拟/容器网卡（否则容器流量会被当成「本机」）", n)
		}
	}
	for _, n := range []string{"en0", "eth0", "ens18", "eno1", "wlan0", "wlp3s0", "bond0", "enp3s0"} {
		if isVirtualIface(n) {
			t.Errorf("%s 是物理网卡名，不应判为虚拟网卡（否则本机管理会失效）", n)
		}
	}
	if !isLocalIP([]byte{127, 0, 0, 1}) {
		t.Error("回环地址应始终算本机")
	}
	if isLocalIP([]byte{8, 8, 8, 8}) {
		t.Error("公网地址不应算本机")
	}
}

// TestUploadTempDirInsideSaveDir 覆盖 E1：上传临时目录必须落在保存目录内
// （同盘 rename、空间算对盘），且不能是软链（否则临时文件会写到保存目录之外）。
func TestUploadTempDirInsideSaveDir(t *testing.T) {
	root := t.TempDir()
	a := &App{platform: secTestPlatform{dir: root}, absSaveDir: root}

	dir := a.uploadTempDir()
	if dir == "" {
		t.Fatal("保存目录可写时，上传临时目录不应为空")
	}
	if filepath.Dir(dir) != root {
		t.Errorf("上传临时目录应位于保存目录内，实际 %s", dir)
	}
	if !strings.HasPrefix(filepath.Base(dir), ".") {
		t.Errorf("上传临时目录名应以 . 开头（列表接口需过滤），实际 %s", filepath.Base(dir))
	}
	// 文件系统层面确认：临时文件确实落在保存目录那棵子树内。
	tmp, err := os.CreateTemp(dir, "aellus-upload-*")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if !strings.HasPrefix(tmp.Name(), root) {
		t.Errorf("临时文件应落在保存目录内，实际 %s", tmp.Name())
	}

	// 软链场景：.aellus-tmp 若指向外部，必须拒绝并回退（返回空串）。
	outside := filepath.Join(t.TempDir(), "OUTSIDE")
	os.MkdirAll(outside, 0755)
	root2 := t.TempDir()
	os.Symlink(outside, filepath.Join(root2, uploadTempDirName))
	a2 := &App{platform: secTestPlatform{dir: root2}, absSaveDir: root2}
	if got := a2.uploadTempDir(); got != "" {
		t.Errorf("临时目录为软链时应回退（返回空串），实际 %q", got)
	}
}

// batchProbeWriter 在响应首次写出时执行回调（此刻临时 ZIP 已完整、正在被读取发送）。
type batchProbeWriter struct {
	*httptest.ResponseRecorder
	probe func()
	fired bool
}

func (p *batchProbeWriter) Write(b []byte) (int, error) {
	if !p.fired {
		p.fired = true
		p.probe()
	}
	return p.ResponseRecorder.Write(b)
}

// TestBatchDownloadSkipsHiddenPaths 覆盖 B2：批量下载不得把隐藏目录里的内容打包进 ZIP。
// 隐藏语义在列表 / 解析 / 打包各条路径必须一致；WalkDir 对隐藏目录要返回 SkipDir，
// 否则 .aellus-tmp（上传临时文件）与其它隐藏目录内容会随 ZIP 泄露出去。
func TestBatchDownloadSkipsHiddenPaths(t *testing.T) {
	root := t.TempDir()
	a := &App{platform: secTestPlatform{dir: root}, absSaveDir: root}
	mux := a.buildMux(8000)

	os.MkdirAll(filepath.Join(root, "dev"), 0755)
	if err := os.WriteFile(filepath.Join(root, "dev", "ok.txt"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	// 隐藏目录 + 其中的普通文件（列表接口看不到、resolveDir 也进不去）。
	os.MkdirAll(filepath.Join(root, ".hidden"), 0755)
	os.WriteFile(filepath.Join(root, ".hidden", "secret.txt"), []byte("secret"), 0644)
	// 临时目录里的半成品文件（模拟另一台设备正在上传）。
	os.MkdirAll(filepath.Join(root, uploadTempDirName), 0700)
	os.WriteFile(filepath.Join(root, uploadTempDirName, "aellus-upload-xyz"), []byte("inflight"), 0600)
	// 设备目录内的隐藏子目录。
	os.MkdirAll(filepath.Join(root, "dev", ".thumbnails"), 0755)
	os.WriteFile(filepath.Join(root, "dev", ".thumbnails", "meta.txt"), []byte("meta"), 0644)

	req := httptest.NewRequest("POST", "/api/download-batch", strings.NewReader(`{"dir":"","files":[]}`))
	req.Header.Set(clientHeaderName, "1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("批量下载状态码 %d", w.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("解析返回 ZIP 失败: %v", err)
	}
	gotOK := false
	for _, f := range zr.File {
		norm := strings.ReplaceAll(f.Name, `\`, "/")
		for _, seg := range strings.Split(norm, "/") {
			if strings.HasPrefix(seg, ".") {
				t.Errorf("隐藏路径段 %q 被打进 ZIP（条目 %s）：隐藏目录内容不得出现在批量下载中", seg, f.Name)
			}
		}
		if norm == "dev/ok.txt" {
			gotOK = true
		}
	}
	if !gotOK {
		t.Error("正常文件 dev/ok.txt 未被打包（隐藏过滤不应误伤正常文件）")
	}
}

// TestBatchDownloadTempFileInsideSaveDir 覆盖 B1：批量下载的临时 ZIP 必须落在保存目录内的
// 临时目录（同盘、空间算对盘），不得占用系统临时目录——NAS 上 /tmp 常是 tmpfs / 小根分区，
// 打包大目录会把它写满（系统级故障）。同时确认响应结束后临时文件被清理。
func TestBatchDownloadTempFileInsideSaveDir(t *testing.T) {
	root := t.TempDir()
	a := &App{platform: secTestPlatform{dir: root}, absSaveDir: root}
	mux := a.buildMux(8000)
	os.MkdirAll(filepath.Join(root, "dev"), 0755)
	if err := os.WriteFile(filepath.Join(root, "dev", "ok.txt"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	var inTempDir, inSystemTemp []string
	probe := func() {
		// 响应正在发送：临时 ZIP 已完整、尚未删除（defer 在本函数返回后才执行）。
		if ents, err := os.ReadDir(filepath.Join(root, uploadTempDirName)); err == nil {
			for _, e := range ents {
				inTempDir = append(inTempDir, e.Name())
			}
		}
		if ents, err := os.ReadDir(os.TempDir()); err == nil {
			for _, e := range ents {
				if strings.HasPrefix(e.Name(), "aellus-download-") {
					inSystemTemp = append(inSystemTemp, e.Name())
				}
			}
		}
	}

	req := httptest.NewRequest("POST", "/api/download-batch", strings.NewReader(`{"dir":"dev","files":[]}`))
	req.Header.Set(clientHeaderName, "1")
	w := &batchProbeWriter{ResponseRecorder: httptest.NewRecorder(), probe: probe}
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("批量下载状态码 %d", w.Code)
	}
	if len(inSystemTemp) != 0 {
		t.Errorf("临时 ZIP 出现在系统临时目录 %s：%v（应落在保存目录内）", os.TempDir(), inSystemTemp)
	}
	if len(inTempDir) == 0 {
		t.Errorf("响应期间未在 %s 下发现临时 ZIP（临时文件应落在保存目录内）", uploadTempDirName)
	}
	// 响应结束后必须清理干净，不留残留。
	if ents, err := os.ReadDir(filepath.Join(root, uploadTempDirName)); err == nil {
		for _, e := range ents {
			t.Errorf("响应结束后临时文件未清理: %s", e.Name())
		}
	}
}

// TestGIFFirstFrameBlocksDecodeBomb 覆盖 P12：GIF 多帧解压炸弹防护。
//
// 标准库解码 GIF 会把【全部帧】都解出来（每帧按画布大小分配内存），几 MB 的多帧
// GIF 就能放大成数 GB。缩略图只需要第一帧：gifFirstFrame 必须把第一帧截成单帧 GIF，
// 使后续解码只解一帧、内存恒等于单帧面积（从而让 maxPixels 检查重新有效）。
func TestGIFFirstFrameBlocksDecodeBomb(t *testing.T) {
	// 用标准库编一个 3 帧 GIF 作为样本（含图形控制扩展分支）。
	const w, h = 8, 8
	pal := color.Palette{color.Black, color.White}
	var frames []*image.Paletted
	for i := 0; i < 3; i++ {
		img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		if i%2 == 0 {
			img.Pix[0] = 1
		}
		frames = append(frames, img)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: frames, Delay: []int{0, 0, 0}}); err != nil {
		t.Fatalf("构造 GIF 样本失败: %v", err)
	}
	data := buf.Bytes()

	// 样本自检：原文件确实是 3 帧（否则下面的断言没有意义）。
	orig, oerr := gif.DecodeAll(bytes.NewReader(data))
	if oerr != nil {
		t.Fatalf("样本 GIF 无法解码: %v", oerr)
	}
	if len(orig.Image) != 3 {
		t.Fatalf("样本应有 3 帧，实际 %d", len(orig.Image))
	}

	one, ok := gifFirstFrame(data)
	if !ok {
		t.Fatal("合法 GIF 应能截取到第一帧（ok=true）")
	}
	// 关键断言：截取后的 GIF 只有一帧——解码内存不再随帧数放大。
	single, serr := gif.DecodeAll(bytes.NewReader(one))
	if serr != nil {
		t.Fatalf("截取出的单帧 GIF 无法解码: %v", serr)
	}
	if len(single.Image) != 1 {
		t.Errorf("截取后应只有 1 帧，实际 %d（多帧会放大内存）", len(single.Image))
	}
	// 画布尺寸必须保持原样，不能变形。
	if single.Config.Width != orig.Config.Width || single.Config.Height != orig.Config.Height {
		t.Errorf("画布尺寸被改变: %dx%d → %dx%d",
			orig.Config.Width, orig.Config.Height, single.Config.Width, single.Config.Height)
	}
	// 通用解码入口也要能解（thumb.go 走的是 image.Decode）。
	if _, _, derr := image.Decode(bytes.NewReader(one)); derr != nil {
		t.Errorf("image.Decode 解码单帧 GIF 失败: %v", derr)
	}

	// 只缺 Trailer 的 GIF 仍应截取成功：单帧化只读到第一帧为止，不依赖文件尾。
	// （这也是它比"整文件交给标准库解码"更健壮的地方。）
	if _, ok := gifFirstFrame(data[:len(data)-1]); !ok {
		t.Error("缺少 Trailer 但第一帧完整时，仍应能截取第一帧")
	}

	// 畸形 / 截断数据不得 panic，且必须返回 ok=false（调用方据此回退，不去解原文件）。
	noFrame := append(append([]byte{}, data[:13]...), 0x3B) // 只有头 + 结束符：没有任何图像帧
	for _, bad := range [][]byte{
		nil,
		{},
		[]byte("not a gif at all"),
		data[:6],  // 只有 Header
		data[:11], // Header + 不完整的屏幕描述符
		noFrame,
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("畸形 GIF 数据触发 panic: %v", r)
				}
			}()
			if _, ok := gifFirstFrame(bad); ok {
				t.Errorf("畸形/截断数据不应被判为合法 GIF（ok 应为 false）")
			}
		}()
	}
}
