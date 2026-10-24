package main

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// openPage 无头打开本页（带外链样式表加载器），返回文档。
func openPage(t *testing.T) engine.HTMLDocument {
	t.Helper()
	doc, err := engine.OpenDocumentWith(engine.NewHeadlessViewport(1120, 820), string(page), docOptions())
	if err != nil {
		t.Fatalf("OpenDocumentWith: %v", err)
	}
	t.Cleanup(func() { doc.Close() })
	return doc
}

// TestStylesheetLoader 只认本页内嵌的 stylebook.css：解析结果按尾名匹配，
// 其余地址报错（引擎收到错误会忽略该表并继续渲染）。
func TestStylesheetLoader(t *testing.T) {
	opts := docOptions()
	if opts.LoadStylesheet == nil {
		t.Fatal("LoadStylesheet 为 nil：外链样式表不会被加载")
	}
	got, err := opts.LoadStylesheet("file:///examples/stylebook/stylebook.css")
	if err != nil {
		t.Fatalf("加载本页样式表: %v", err)
	}
	if !strings.Contains(got, ".banner") {
		t.Error("返回的样式表缺少 .banner 规则")
	}
	if got != string(stylesheet) {
		t.Error("返回内容与 go:embed 的 stylebook.css 不一致")
	}
	if _, err := opts.LoadStylesheet("file:///elsewhere/other.css"); err == nil {
		t.Error("未知样式表地址应报错（否则样式缺失没人发现）")
	}
}

// TestExternalStylesheetApplied 外链链路端到端：<link href="stylebook.css">
// 解析 → 加载器回调 → 级联生效。用一条只在作者样式表里出现的颜色做证据
// （无 CSS 时页面上不存在 #0b57d0）。
func TestExternalStylesheetApplied(t *testing.T) {
	doc := openPage(t)
	if got := doc.Title(); !strings.Contains(got, "样式全集") {
		t.Errorf("Title = %q, want 含「样式全集」", got)
	}
	if !strings.Contains(doc.DumpSVG(), "#0b57d0") {
		t.Error("导出 SVG 里没有 #0b57d0：<link> 外链样式表未生效")
	}
}

// TestSectionsPresent 每一节都要存在且有非零盒子：删掉某节或整节布局塌掉
// 即失败（只断言结构，不锁像素）。
func TestSectionsPresent(t *testing.T) {
	doc := openPage(t)
	ids := []string{
		"sec-tags-inline", "sec-tags-block", "sec-semantic", "sec-tags-media",
		"sec-forms", "sec-table", "sec-box", "sec-border", "sec-bg",
		"sec-text", "sec-overflow", "sec-values", "sec-selectors",
		"sec-pseudo", "sec-unsupported",
		// 选择器演示的锚点
		"sel-id", "sel-spec", "sel-imp",
	}
	for _, id := range ids {
		el := doc.GetElementById(id)
		if el == nil {
			t.Errorf("#%s 不存在", id)
			continue
		}
		r := el.GetBoundingClientRect()
		if r.Right().Pixel() <= r.Left().Pixel() || r.Bottom().Pixel() <= r.Top().Pixel() {
			t.Errorf("#%s 盒子为空: %v", id, r)
		}
	}
}

// TestCoverageCounts 标签覆盖数：这些标签都要真的进 DOM（结构回归护栏）。
func TestCoverageCounts(t *testing.T) {
	doc := openPage(t)
	checks := []struct {
		sel string
		min int
	}{
		{"input", 10}, {"select", 2}, {"textarea", 1}, {"table", 4},
		{"li", 6}, {"section", 2}, {"article", 2}, {"img", 1},
		{"canvas", 1}, {"iframe", 1}, {"option", 6}, {"fieldset", 1},
		{"blockquote", 1}, {"figure", 1}, {"pre", 1}, {"dl", 1},
	}
	for _, c := range checks {
		if got := len(doc.QuerySelectorAll(c.sel)); got < c.min {
			t.Errorf("<%s> 数量 = %d, want >= %d", c.sel, got, c.min)
		}
	}
}

// TestSemanticRows 语义块的两行是本页的核心对照内容：
//   - raw 行无作者 display → 引擎按 inline-block 原子盒**并排**（无 UA 规则的
//     证据）；若有一天 UA 补了块级规则，这里会失败——届时同步改对照页与 CHANGELOG；
//   - fixed 行作者补了 display:block → 必须**逐行堆叠**。
func TestSemanticRows(t *testing.T) {
	doc := openPage(t)
	raw := doc.GetElementById("sec-semantic").QuerySelector(".raw")
	if raw == nil {
		t.Fatal("raw 行不存在")
	}
	kids := raw.Children()
	if len(kids) < 2 {
		t.Fatalf("raw 行子元素 = %d, want >= 2", len(kids))
	}
	a, b := kids[0].GetBoundingClientRect(), kids[1].GetBoundingClientRect()
	if b.Left().Pixel() < a.Right().Pixel()-1 {
		t.Errorf("raw 行第 1/2 个子元素没有并排（a=%v b=%v）："+
			"无 UA 规则的语义标签应退化为 inline-block 并排", a, b)
	}

	fixed := doc.GetElementById("sec-semantic").QuerySelector(".fixed")
	if fixed == nil {
		t.Fatal("fixed 行不存在")
	}
	kids = fixed.Children()
	if len(kids) < 2 {
		t.Fatalf("fixed 行子元素 = %d, want >= 2", len(kids))
	}
	a, b = kids[0].GetBoundingClientRect(), kids[1].GetBoundingClientRect()
	if b.Top().Pixel() < a.Bottom().Pixel()-1 {
		t.Errorf("fixed 行第 1/2 个子元素没有堆叠（a=%v b=%v）：display:block 应逐行堆叠", a, b)
	}
}
