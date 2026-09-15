package app

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestFilesPaging 覆盖 /api/files 的分页行为：
// 不传参数（全量，兼容旧调用）、第一页/末页、越界页（空 + total 真实）、非法参数回退、跨页不重不漏。
func TestFilesPaging(t *testing.T) {
	root := t.TempDir()
	a := &App{platform: secTestPlatform{dir: root}, absSaveDir: root}
	mux := a.buildMux(8000)

	for i := 0; i < 65; i++ {
		name := filepath.Join(root, fmt.Sprintf("f%02d.txt", i))
		if err := os.WriteFile(name, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	get := func(q string) FilesResp {
		req := httptest.NewRequest("GET", "/api/files?"+q, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var r FilesResp
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
			t.Fatalf("解析失败: %v (%s)", err, w.Body.String())
		}
		return r
	}

	// 不传参数：全量返回（旧行为兼容），size=0
	full := get("dir=")
	if len(full.Files) != 65 || full.Total != 65 || full.Size != 0 || full.Page != 1 {
		t.Errorf("全量: files=%d total=%d size=%d page=%d（期望 65/65/0/1）", len(full.Files), full.Total, full.Size, full.Page)
	}
	// 第一页满 30 条
	p1 := get("dir=&page=1&size=30")
	if len(p1.Files) != 30 || p1.Total != 65 || p1.Page != 1 || p1.Size != 30 {
		t.Errorf("第1页: files=%d total=%d page=%d size=%d（期望 30/65/1/30）", len(p1.Files), p1.Total, p1.Page, p1.Size)
	}
	// 末页只剩 5 条
	p3 := get("dir=&page=3&size=30")
	if len(p3.Files) != 5 || p3.Total != 65 || p3.Page != 3 {
		t.Errorf("第3页: files=%d total=%d page=%d（期望 5/65/3）", len(p3.Files), p3.Total, p3.Page)
	}
	// 越界页：空数组 + total 真实（前端据此回退最后一页）
	p9 := get("dir=&page=99&size=30")
	if len(p9.Files) != 0 || p9.Total != 65 || p9.Page != 99 {
		t.Errorf("越界页: files=%d total=%d page=%d（期望 0/65/99）", len(p9.Files), p9.Total, p9.Page)
	}
	// 非法参数回退默认：page=abc 视为 1，size=0 视为全量
	bad := get("dir=&page=abc&size=0")
	if len(bad.Files) != 65 || bad.Page != 1 {
		t.Errorf("非法参数: files=%d page=%d（期望 65/1）", len(bad.Files), bad.Page)
	}
	// 跨页不重不漏（排序已做 mtime 相同时按名字的稳定全序）
	seen := map[string]bool{}
	for _, f := range append(p1.Files, get("dir=&page=2&size=30").Files...) {
		if seen[f.Name] {
			t.Errorf("跨页重复: %s", f.Name)
		}
		seen[f.Name] = true
	}
	if len(seen) != 60 {
		t.Errorf("前两页去重合计 %d（期望 60）", len(seen))
	}
}

// TestDirsPaging 覆盖 /api/dirs 的分页行为：全量与分页共存、第二页余量。
func TestDirsPaging(t *testing.T) {
	root := t.TempDir()
	a := &App{platform: secTestPlatform{dir: root}, absSaveDir: root}
	mux := a.buildMux(8000)

	for i := 0; i < 35; i++ {
		d := filepath.Join(root, fmt.Sprintf("dev%02d", i))
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "a.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	get := func(q string) DirsResp {
		req := httptest.NewRequest("GET", "/api/dirs?"+q, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var r DirsResp
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		return r
	}

	full := get("")
	if len(full.Dirs) != 35 || full.Total != 35 || full.Size != 0 {
		t.Errorf("全量: dirs=%d total=%d size=%d（期望 35/35/0）", len(full.Dirs), full.Total, full.Size)
	}
	p2 := get("page=2&size=30")
	if len(p2.Dirs) != 5 || p2.Total != 35 || p2.Page != 2 || p2.Size != 30 {
		t.Errorf("第2页: dirs=%d total=%d page=%d size=%d（期望 5/35/2/30）", len(p2.Dirs), p2.Total, p2.Page, p2.Size)
	}
}
