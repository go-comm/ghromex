package engine

import (
	"strings"
	"testing"
)

// TestListMarkers 列表标记：引擎无 list-item 概念，标记由布局期按父级
// ul/ol 算出、绘制期画在内容左缘左侧（Chrome: ul → disc "•"，ol → "1." "2."…）。
func TestListMarkers(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<ul id="u"><li>甲</li><li>乙</li></ul>
		<ol id="o"><li>一</li><li>二</li><li>三</li></ol>
	</body></html>`
	buf := NewBufferGraphics(200, 200)
	vp := &HeadlessViewport{W: 200, H: 200, G: buf}
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)

	markers := func(sel string) []string {
		var out []string
		for _, e := range doc.QuerySelectorAll(sel) {
			b := inner(e)
			if b == nil {
				continue
			}
			out = append(out, b.listMarker)
		}
		return out
	}
	ul := markers("ul li")
	if len(ul) != 2 || ul[0] != "•" || ul[1] != "•" {
		t.Errorf("ul 的标记 = %q, want 两个 •", ul)
	}
	ol := markers("ol li")
	if len(ol) != 3 || ol[0] != "1." || ol[1] != "2." || ol[2] != "3." {
		t.Errorf("ol 的标记 = %q, want 1. 2. 3.", ol)
	}
	// 标记必须真的画出来（SVG 导出含标记文本）
	svg := doc.DumpSVG()
	if !strings.Contains(svg, "•") {
		t.Errorf("SVG 导出缺少 ul 标记:\n%s", svg)
	}
	if !strings.Contains(svg, "1.") || !strings.Contains(svg, "3.") {
		t.Errorf("SVG 导出缺少 ol 序号标记:\n%s", svg)
	}
}
