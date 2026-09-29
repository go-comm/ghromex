package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

const anchorPage = `<!doctype html><html><head><style>
body { margin: 0; }
</style></head><body>
<p id="p"><a id="l" href="https://example.com/form">link text</a></p>
<a id="n">plain</a>
<a id="u" href="#sec" style="text-decoration:none">no underline</a>
</body></html>`

// <a> 的 UA 样式：链接色 + 下划线，且 text-decoration 可被内联样式覆盖。
func TestAnchorUAStyle(t *testing.T) {
	doc := openDoc(t, 400, 300, anchorPage)
	svg := doc.DumpSVG()

	if !strings.Contains(svg, `fill="#2563eb"`) {
		t.Error("链接色未生效（UA a { color:#2563EB }）")
	}
	// 文本按空白切成多段（"link"+" "+"text" = 3 段、"plain" = 1 段），
	// 三段链接文字都带 UA 下划线；#u 内联 none 覆盖后不带。
	linkRuns, plainRuns, uRuns, underlined := 0, 0, 0, 0
	for _, line := range strings.Split(svg, "\n") {
		if !strings.Contains(line, "<text ") || !strings.Contains(line, "</text>") {
			continue
		}
		hasDec := strings.Contains(line, `text-decoration="underline"`)
		if hasDec {
			underlined++
		}
		switch {
		case strings.Contains(line, `>link</text>`), strings.Contains(line, `>text</text>`):
			linkRuns++
			if !hasDec {
				t.Errorf("链接文字缺下划线: %s", line)
			}
		case strings.Contains(line, `>plain</text>`):
			plainRuns++
			if !hasDec {
				t.Errorf("无 href 的 <a> 也应有 UA 下划线: %s", line)
			}
		case strings.Contains(line, `>underline</text>`) || strings.Contains(line, `>no</text>`):
			uRuns++
			if hasDec {
				t.Errorf("内联 text-decoration:none 未覆盖 UA: %s", line)
			}
		}
	}
	if linkRuns < 2 || plainRuns != 1 || uRuns < 2 {
		t.Errorf("段落统计 link=%d plain=%d u=%d, want link>=2 plain=1 u>=2（文本切分或导出异常）",
			linkRuns, plainRuns, uRuns)
	}
	if underlined == 0 {
		t.Error("没有任何段落带下划线")
	}
}

// 点击 <a href> → 文档导航回调收到 href；无 href 不触发；
// 导航在 click 冒泡之后执行。
func TestAnchorNavigateOnClick(t *testing.T) {
	doc := openDoc(t, 400, 300, anchorPage)
	var got string
	doc.SetOnNavigate(func(href string) { got = href })

	x, y := focusXY(t, doc, "#l")
	engine.OnDocumentClick(doc, x, y)
	if got != "https://example.com/form" {
		t.Fatalf("navigate href = %q, want https://example.com/form", got)
	}

	got = ""
	x, y = focusXY(t, doc, "#n")
	engine.OnDocumentClick(doc, x, y)
	if got != "" {
		t.Fatalf("无 href 的 a 不应导航, got %q", got)
	}

	// click 处理器先于导航执行，仍能读到文档状态
	seen := ""
	mustEl(t, doc, "#l").OnClick(func(ev *engine.MouseEvent) {
		seen = ev.Element.GetAttribute("href")
	})
	engine.OnDocumentClick(doc, x, y) // #n：不改变 got
	got = ""
	x, y = focusXY(t, doc, "#l")
	engine.OnDocumentClick(doc, x, y)
	if seen != "https://example.com/form" {
		t.Fatalf("click 处理器读到 %q, want href", seen)
	}
	if got != "https://example.com/form" {
		t.Fatalf("导航应在冒泡之后执行, got %q", got)
	}
}

// text-decoration 随父链继承到行内子元素，未设置的元素不带装饰。
func TestTextDecorationInherited(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; }
	.wrap { text-decoration: underline; }
	</style></head><body>
	<div class="wrap"><span id="s">inner</span></div>
	<span id="t">plain</span>
	</body></html>`
	doc := openDoc(t, 400, 300, src)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, `text-decoration="underline"`) {
		t.Fatal("text-decoration 未沿父链继承到 span")
	}
	for _, line := range strings.Split(svg, "\n") {
		if strings.Contains(line, `>plain</text>`) && strings.Contains(line, "text-decoration") {
			t.Fatalf("未设置装饰的元素被加了装饰: %s", line)
		}
	}
	if !strings.Contains(svg, `>plain</text>`) {
		t.Fatal("普通文本未导出")
	}
	buf := engine.NewBufferGraphics(400, 300)
	engine.RenderNode(buf, doc)
	if buf.DrawCalls == 0 {
		t.Fatal("装饰线未触发绘制")
	}
}
