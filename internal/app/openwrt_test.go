package app

import (
	"os"
	"path/filepath"
	"testing"
)

// fakePlatform 是 Platform 的最小实现，只关心 EnforceAuthBoundary，
// 其余方法空实现（platformLabel 不会调用它们）。
type fakePlatform struct{ authBoundary bool }

func (p fakePlatform) RunTray(url string)                  {}
func (p fakePlatform) PostOpenNotification(t, b, u string) {}
func (p fakePlatform) EnforceSingleInstance() bool         { return true }
func (p fakePlatform) PickFolderDialog() string            { return "" }
func (p fakePlatform) PickDirSupported() bool              { return false }
func (p fakePlatform) PersistSaveDirAllowed() bool         { return false }
func (p fakePlatform) EnforceAuthBoundary() bool           { return p.authBoundary }
func (p fakePlatform) ConfigBaseDir(saveDir string) string { return saveDir }
func (p fakePlatform) LogsDir() string                     { return "" }

// TestIsOpenWrtRelease 覆盖两种官方标识格式，以及常见发行版不应误判。
func TestIsOpenWrtRelease(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name: "openwrt_release 官方内容",
			content: `DISTRIB_ID='OpenWrt'
DISTRIB_RELEASE='24.10.1'
DISTRIB_REVISION='r28597-0425664679'
DISTRIB_TARGET='x86/64'
`,
			want: true,
		},
		{
			name:    "os-release ID 带引号",
			content: "ID=\"openwrt\"\nNAME=\"OpenWrt\"\n",
			want:    true,
		},
		{
			name:    "os-release ID 不带引号",
			content: "ID=openwrt\nPRETTY_NAME=\"OpenWrt 24.10.1\"\n",
			want:    true,
		},
		{
			name:    "Debian 不应误判",
			content: "ID=debian\nNAME=\"Debian GNU/Linux\"\n",
			want:    false,
		},
		{
			// 关键点：只按行匹配 DISTRIB_ID / ID，注释或 URL 里出现字样不算，
			// 否则装了 OpenWrt 相关工具的 Debian 机器会被误判。
			name:    "仅 HOME_URL 提到 OpenWrt 不算",
			content: "ID=debian\nHOME_URL=\"https://openwrt.org/\"\n# built on OpenWrt SDK\n",
			want:    false,
		},
		{
			name:    "注释里的 DISTRIB_ID 不算（行首非 DISTRIB_ID=）",
			content: "# DISTRIB_ID='OpenWrt'\nID=debian\n",
			want:    false,
		},
		{
			name:    "空文件",
			content: "",
			want:    false,
		},
		{
			name:    "无关内容",
			content: "ID=\"ubuntu\"\nID_LIKE=debian\n",
			want:    false,
		},
		// —— 衍生固件：官方 OpenWrt 之外最常见的两种，都靠 ID_LIKE 表明血脉 ——
		{
			// 实测反馈：ImmortalWrt 的 DISTRIB_ID 是 'ImmortalWrt'（不含子串 "OpenWrt"），
			// ID 是 "immortalwrt"。旧实现比对 ID=openwrt 会漏判成桌面端。
			name: "ImmortalWrt 的 os-release",
			content: "NAME=\"ImmortalWrt\"\nVERSION=\"21.02.7\"\n" +
				"ID=\"immortalwrt\"\nID_LIKE=\"openwrt\"\nVERSION_ID=\"21.02.7\"\n",
			want: true,
		},
		{
			name:    "LEDE 衍生：ID_LIKE 多值列表",
			content: "NAME=\"LEDE\"\nID=\"lede\"\nID_LIKE=\"lede openwrt\"\nVERSION_ID=\"17.02.1\"\n",
			want:    true,
		},
		{
			name:    "ID_LIKE 带早于 openwrt 的前缀值",
			content: "ID=\"istoreos\"\nID_LIKE=\"openwrt lede\"\n",
			want:    true,
		},
		{
			name:    "ID_LIKE=debian 不应误判",
			content: "ID=\"mint\"\nID_LIKE=\"ubuntu debian\"\n",
			want:    false,
		},
	}
	for _, c := range cases {
		if got := isOpenWrtRelease(c.content); got != c.want {
			t.Errorf("%s: isOpenWrtRelease() = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestDetectOpenWrt 覆盖文件级判据：哪份文件缺失都应有下一条兜住。
// 用临时目录替换包里的路径变量，不依赖真实的 /etc。
func TestDetectOpenWrt(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("写 %s 失败: %v", p, err)
		}
		return p
	}

	marker := write("openwrt_release-ImmortalWrt", "DISTRIB_ID='ImmortalWrt'\n")
	osRel := write("os-release-immortalwrt", "ID=\"immortalwrt\"\nID_LIKE=\"openwrt\"\n")
	osRelDebian := write("os-release-debian", "ID=debian\n")
	procd := write("procd", "#!/sbin/procd\n")

	// 保存原值，测试结束还原（这些是包级变量，不能污染其它用例）
	oldOS, oldMarker, oldProcd := osReleasePaths, openWrtMarker, procdPaths
	defer func() { osReleasePaths, openWrtMarker, procdPaths = oldOS, oldMarker, oldProcd }()

	cases := []struct {
		name   string
		marker string
		osRel  []string
		procd  []string
		want   bool
	}{
		{
			name:   "只有 openwrt_release（ImmortalWrt，内容是 ImmortalWrt 不是 OpenWrt）",
			marker: marker, osRel: nil, procd: nil, want: true,
		},
		{
			name:   "openwrt_release 被裁剪，靠 os-release 的 ID_LIKE 兜住",
			marker: filepath.Join(dir, "不存在的文件"), osRel: []string{osRel}, procd: nil, want: true,
		},
		{
			name:   "两份都没有，靠 procd 兜住",
			marker: filepath.Join(dir, "不存在的文件"), osRel: nil, procd: []string{procd}, want: true,
		},
		{
			name:   "桌面 Linux：三份都不是 OpenWrt 的",
			marker: filepath.Join(dir, "不存在的文件"), osRel: []string{osRelDebian}, procd: nil, want: false,
		},
		{
			name:   "什么都没有",
			marker: filepath.Join(dir, "不存在的文件"), osRel: nil, procd: nil, want: false,
		},
	}
	for _, c := range cases {
		openWrtMarker = c.marker
		osReleasePaths = c.osRel
		procdPaths = c.procd
		if got := detectOpenWrt(); got != c.want {
			t.Errorf("%s: detectOpenWrt() = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPlatformLabel 验证 /api/addr 展示的平台名：飞牛优先，其次是 OpenWrt。
func TestPlatformLabel(t *testing.T) {
	cases := []struct {
		name         string
		authBoundary bool
		isOpenWrt    bool
		want         string
	}{
		{"飞牛 fpk 构建", true, false, "飞牛 fnOS"},
		// 飞牛构建即便跑在 OpenWrt 上也不该被判成 OpenWrt（fpk 只装在 fnOS 上）
		{"飞牛优先于 OpenWrt", true, true, "飞牛 fnOS"},
		{"OpenWrt 路由器", false, true, "OpenWrt"},
		{"桌面端", false, false, "桌面端（macOS / Windows / Linux）"},
	}
	for _, c := range cases {
		got := platformLabel(fakePlatform{authBoundary: c.authBoundary}, c.isOpenWrt)
		if got != c.want {
			t.Errorf("%s: platformLabel() = %q, want %q", c.name, got, c.want)
		}
	}
}
