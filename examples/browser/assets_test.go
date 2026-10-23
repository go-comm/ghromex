package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// svgHasFill 判断导出 SVG 里是否出现了该颜色（作者声明的 color 落成文本
// fill；计算后的样式没有公开读取入口，SVG 是级联结果的公开出口）。
func svgHasFill(doc engine.HTMLDocument, hex string) bool {
	return strings.Contains(doc.DumpSVG(), `fill="`+hex+`"`)
}

// TestFileURLRoundTrip file:// 地址与本地路径互转：外部样式表的地址全靠它，
// 转错的表现是「样式表读不到」，而且 Windows 上 registrar 的斜杠很容易写反。
func TestFileURLRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "css", "theme.css")
	u := fileURL(path)
	if !strings.HasPrefix(u, "file:///") {
		t.Fatalf("fileURL(%q) = %q, want file:/// 前缀", path, u)
	}
	got, ok := pathFromFileURL(u)
	if !ok {
		t.Fatalf("pathFromFileURL(%q) 失败", u)
	}
	if filepath.Clean(got) != filepath.Clean(path) {
		t.Errorf("往返结果 = %q, want %q", got, path)
	}
}

// TestPathFromFileURLRejects 非本机地址要拒绝：UNC（file://host/share）与
// 普通 URL 都不能被当成本地相对路径去读。
func TestPathFromFileURLRejects(t *testing.T) {
	for _, u := range []string{
		`https://example.com/a.css`,
		`file://server/share/a.css`,
		`file:///`,
		`file:`,
	} {
		if p, ok := pathFromFileURL(u); ok {
			t.Errorf("pathFromFileURL(%q) = %q,true, want false", u, p)
		}
	}
}

// TestPageBaseURL 虚拟地址（about:home，内容来自 go:embed）也要有基准：
// 用所在目录下的 index.html 当名义地址，相对引用才解析得对。
func TestPageBaseURL(t *testing.T) {
	dir := t.TempDir()
	want := fileURL(filepath.Join(dir, "index.html"))
	if got := pageBaseURL(homeURL, dir); got != want {
		t.Errorf("pageBaseURL(%q, %q) = %q, want %q", homeURL, dir, got, want)
	}
	page := filepath.Join(dir, "sub", "page.html")
	if got := pageBaseURL(page, filepath.Dir(page)); got != fileURL(page) {
		t.Errorf("真实文件的基准 = %q, want %q", got, fileURL(page))
	}
}

// TestShowLoadsExternalStylesheets 走完整的浏览器加载流程（show → 工具条
// 注入 → 布局）：<link> 按页面地址解析，@import 按样式表地址解析，两条链
// 都要真的作用到元素上。
func TestShowLoadsExternalStylesheets(t *testing.T) {
	dir := t.TempDir()
	cssDir := filepath.Join(dir, "css")
	if err := os.MkdirAll(filepath.Join(cssDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write(filepath.Join("css", "main.css"), `@import "sub/base.css"; #x { color: #0000FF; }`)
	write(filepath.Join("css", "sub", "base.css"), `#y { color: #FF0000; }`)
	page := write("page.html", `<!doctype html><html><head>
		<link rel="stylesheet" href="css/main.css">
		</head><body style="margin:0"><p id="x">x</p><p id="y">y</p></body></html>`)

	h := &headlessOpener{vp: engine.NewHeadlessViewport(800, 600)}
	app := &browserApp{win: h, homeDir: dir, index: -1}
	data, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if !app.show(page, dir, data, true) {
		t.Fatal("show 返回 false（页面未加载）")
	}
	if !svgHasFill(h.doc, "#0000ff") {
		t.Error("#x 没有 #0000ff：<link> 加载的样式表未生效")
	}
	if !svgHasFill(h.doc, "#ff0000") {
		t.Error("#y 没有 #ff0000：@import 的子目录引用解析错了")
	}
}

// TestLoadLocalStylesheetRejectsNetwork 示例不联网：http(s) 地址必须失败
// （引擎会忽略这份样式表），不能悄悄去请求。
func TestLoadLocalStylesheetRejectsNetwork(t *testing.T) {
	if _, err := loadLocalStylesheet("https://example.com/a.css"); err == nil {
		t.Fatal("http(s) 样式表应被拒绝")
	}
	missing := fileURL(filepath.Join(t.TempDir(), "missing.css"))
	if _, err := loadLocalStylesheet(missing); err == nil {
		t.Fatal("不存在的样式表应返回错误")
	}
}
