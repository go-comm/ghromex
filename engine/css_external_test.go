package engine_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// mapLoader 是测试用的样式表加载器：记录每次请求的地址，按绝对地址取内容。
// 记录地址本身也是断言的一部分——相对引用解析错了，这里会直接看出来。
type mapLoader struct {
	files map[string]string
	seen  []string
	err   error // 非 nil 时所有加载都失败（模拟 404 / 不联网）
}

func (l *mapLoader) load(href string) (string, error) {
	l.seen = append(l.seen, href)
	if l.err != nil {
		return "", l.err
	}
	if css, ok := l.files[href]; ok {
		return css, nil
	}
	return "", fmt.Errorf("not found: %s", href)
}

func openDocOpts(t *testing.T, w, h int, src string, opts engine.DocumentOptions) engine.HTMLDocument {
	t.Helper()
	doc, err := engine.OpenDocumentWith(engine.NewHeadlessViewport(w, h), src, opts)
	if err != nil {
		t.Fatalf("OpenDocumentWith: %v", err)
	}
	return doc
}

// svgHasFill 判断导出 SVG 里是否出现了该颜色：作者声明的 color 会落成文本
// 片段的 fill（与 svg_dump_test / cascade_semantics_test 同口径）。计算后的
// 样式没有公开读取入口（computed 是包内字段），SVG 是级联结果的公开出口。
func svgHasFill(doc engine.HTMLDocument, hex string) bool {
	return strings.Contains(doc.DumpSVG(), `fill="`+hex+`"`)
}

// TestLinkStylesheetLoads 最基本的链路：<link rel="stylesheet"> 经加载器
// 读回 CSS，规则参与级联。href 相对文档 BaseURL 解析。
func TestLinkStylesheetLoads(t *testing.T) {
	loader := &mapLoader{files: map[string]string{
		"file:///site/css/theme.css": `#x { color: #FF0000; }`,
	}}
	src := `<!doctype html><html><head>
		<link rel="stylesheet" href="css/theme.css">
		</head><body style="margin:0"><p id="x">t</p></body></html>`
	doc := openDocOpts(t, 200, 100, src, engine.DocumentOptions{
		BaseURL:        "file:///site/page.html",
		LoadStylesheet: loader.load,
	})
	if len(loader.seen) != 1 || loader.seen[0] != "file:///site/css/theme.css" {
		t.Fatalf("加载地址 = %v, want [file:///site/css/theme.css]", loader.seen)
	}
	if !svgHasFill(doc, "#ff0000") {
		t.Error("外部样式表未生效：没有 #ff0000 的文本")
	}
}

// TestImportChainResolvesAgainstSheet 嵌套 @import 的基准是**上一层样式表
// 的地址**，不是页面地址。样式表放在 css/ 子目录里时，这一条决定了子目录
// 中的相对引用会不会解析到页面目录去。
func TestImportChainResolvesAgainstSheet(t *testing.T) {
	loader := &mapLoader{files: map[string]string{
		"file:///site/css/main.css": `
			@import "sub/base.css";
			#x { color: #0000FF; }
		`,
		"file:///site/css/sub/base.css": `#x { color: #FF0000; }`,
	}}
	src := `<!doctype html><html><head>
		<link rel="stylesheet" href="css/main.css">
		</head><body style="margin:0"><p id="x">t</p></body></html>`
	doc := openDocOpts(t, 200, 100, src, engine.DocumentOptions{
		BaseURL:        "file:///site/page.html",
		LoadStylesheet: loader.load,
	})
	want := []string{"file:///site/css/main.css", "file:///site/css/sub/base.css"}
	if len(loader.seen) != 2 || loader.seen[0] != want[0] || loader.seen[1] != want[1] {
		t.Fatalf("加载顺序 = %v, want %v", loader.seen, want)
	}
	// @import 的规则排在导入方之前：同特异度时 main.css 自己的蓝色胜出。
	if !svgHasFill(doc, "#0000ff") {
		t.Error("导入方规则未生效：没有 #0000ff 的文本")
	}
	if svgHasFill(doc, "#ff0000") {
		t.Error("@import 的规则压过了导入方：顺序反了（导入应排在前面）")
	}
}

// TestImportCycleTerminates A→B→A 的互相导入必须在链上截断：既不能死循环
// （递归到栈溢出），也不能把同一份样式表合并两次。
func TestImportCycleTerminates(t *testing.T) {
	loader := &mapLoader{files: map[string]string{
		"file:///site/a.css": `@import "b.css"; .a { color: #FF0000; }`,
		"file:///site/b.css": `@import "a.css"; .b { color: #0000FF; }`,
	}}
	src := `<!doctype html><html><head>
		<link rel="stylesheet" href="a.css">
		</head><body style="margin:0">
		<p id="a" class="a">a</p><p id="b" class="b">b</p></body></html>`
	doc := openDocOpts(t, 200, 100, src, engine.DocumentOptions{
		BaseURL:        "file:///site/page.html",
		LoadStylesheet: loader.load,
	})
	if len(loader.seen) != 2 {
		t.Fatalf("加载次数 = %d (%v), want 2（a 不应被重复加载）", len(loader.seen), loader.seen)
	}
	if !svgHasFill(doc, "#ff0000") {
		t.Error(".a 的规则未生效：没有 #ff0000 的文本")
	}
	if !svgHasFill(doc, "#0000ff") {
		t.Error(".b 的规则未生效：没有 #0000ff 的文本")
	}
}

// TestStylesheetOrderFollowsDocument 外部样式表与 <style> 按**文档顺序**
// 参与级联：同特异度时后出现的胜出。顺序反了，覆盖关系就整个反过来。
func TestStylesheetOrderFollowsDocument(t *testing.T) {
	loader := &mapLoader{files: map[string]string{
		"file:///site/a.css": `#x { color: #FF0000; }`,
	}}
	cases := []struct {
		name, head, want, other string
	}{
		{"link 在 style 之后", `<style>#x { color: #0000FF; }</style><link rel="stylesheet" href="a.css">`, "#ff0000", "#0000ff"},
		{"link 在 style 之前", `<link rel="stylesheet" href="a.css"><style>#x { color: #0000FF; }</style>`, "#0000ff", "#ff0000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := `<!doctype html><html><head>` + c.head +
				`</head><body style="margin:0"><p id="x">t</p></body></html>`
			doc := openDocOpts(t, 200, 100, src, engine.DocumentOptions{
				BaseURL:        "file:///site/page.html",
				LoadStylesheet: loader.load,
			})
			if !svgHasFill(doc, c.want) {
				t.Errorf("缺少 %s：后出现的样式表没有胜出", c.want)
			}
			if svgHasFill(doc, c.other) {
				t.Errorf("出现了 %s：文档顺序被颠倒了", c.other)
			}
		})
	}
}

// TestStylesheetFailureIsIgnored 加载失败（404 / 不联网）只跳过那一份样式表：
// 页面照常打开、其余样式照常生效。缺一份 CSS 不该让页面白屏。
func TestStylesheetFailureIsIgnored(t *testing.T) {
	loader := &mapLoader{err: errors.New("network unavailable")}
	src := `<!doctype html><html><head>
		<link rel="stylesheet" href="a.css">
		<style>#x { color: #00FF00; }</style>
		</head><body style="margin:0"><p id="x">t</p></body></html>`
	doc := openDocOpts(t, 200, 100, src, engine.DocumentOptions{
		BaseURL:        "file:///site/page.html",
		LoadStylesheet: loader.load,
	})
	if len(loader.seen) != 1 {
		t.Fatalf("应尝试加载一次，实际 %d 次", len(loader.seen))
	}
	if !svgHasFill(doc, "#00ff00") {
		t.Error("内联 <style> 未生效：没有 #00ff00 的文本（加载失败不该影响其它样式）")
	}
}

// TestLinkIgnoredWithoutLoader 没装加载器时 <link> 只是被忽略（与旧版本
// 行为一致），不能因为解析到样式表链接就崩掉或阻塞打开。
func TestLinkIgnoredWithoutLoader(t *testing.T) {
	src := `<!doctype html><html><head>
		<link rel="stylesheet" href="a.css">
		</head><body style="margin:0"><p id="x">t</p></body></html>`
	doc := openDoc(t, 200, 100, src)
	if doc.QuerySelector("#x") == nil {
		t.Fatal("文档结构未建立")
	}
}

// TestNonStylesheetLinkIgnored rel 不是 stylesheet 的 <link>（图标、预加载
// 等）不该触发加载。
func TestNonStylesheetLinkIgnored(t *testing.T) {
	loader := &mapLoader{files: map[string]string{}}
	src := `<!doctype html><html><head>
		<link rel="icon" href="favicon.ico">
		<link rel="preload" href="font.woff2">
		</head><body style="margin:0"><p>t</p></body></html>`
	openDocOpts(t, 200, 100, src, engine.DocumentOptions{
		BaseURL:        "file:///site/page.html",
		LoadStylesheet: loader.load,
	})
	if len(loader.seen) != 0 {
		t.Errorf("非样式表 <link> 触发了加载: %v", loader.seen)
	}
}

// TestImportStatementDoesNotEatFollowingRule 没有加载器时 @import 也要被当成
// 独立语句收掉：目前扫描器把它后面的规则一起跳过的话，页面会「只生效一半」。
func TestImportStatementDoesNotEatFollowingRule(t *testing.T) {
	src := `<!doctype html><html><head><style>
		@import url("missing.css");
		#x { color: #FF0000; }
		</style></head><body style="margin:0"><p id="x">t</p></body></html>`
	doc := openDoc(t, 200, 100, src)
	if !svgHasFill(doc, "#ff0000") {
		t.Error("@import 之后的规则被吞掉了：没有 #ff0000 的文本")
	}
}

// TestStylesheetLinkWithAuthoredMedia 带 media 属性的样式表也照常加载：
// 引擎不分媒体（@media 同样跳过），屏幕渲染器把 rel=stylesheet 的都当作者
// 样式表。这条是**有意行为**的钉子，不是疏漏。
func TestStylesheetLinkWithAuthoredMedia(t *testing.T) {
	loader := &mapLoader{files: map[string]string{
		"file:///site/print.css": `#x { color: #FF0000; }`,
	}}
	src := `<!doctype html><html><head>
		<link rel="stylesheet" media="print" href="print.css">
		</head><body style="margin:0"><p id="x">t</p></body></html>`
	doc := openDocOpts(t, 200, 100, src, engine.DocumentOptions{
		BaseURL:        "file:///site/page.html",
		LoadStylesheet: loader.load,
	})
	if !svgHasFill(doc, "#ff0000") {
		t.Error("带 media 属性的样式表未加载：没有 #ff0000 的文本")
	}
}
