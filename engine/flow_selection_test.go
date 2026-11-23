package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 文档级文本选区：普通 HTML 文本（非 input/textarea）的鼠标拖选、复制、
// 高亮导出。此前选区机制只覆盖可编辑控件，页面文字无法选中。

func TestFlowMouseDragSelectsPageText(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; font: 16px monospace; }
	p { margin: 0; }
	</style></head><body><p id="p">hello world</p></body></html>`
	doc := openDoc(t, 400, 120, src)
	p := mustEl(t, doc, "#p")
	r := p.GetBoundingClientRect()
	y := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	reflow := doc.ChangeCount()
	paint := engine.PaintCount(doc)

	if !engine.OnDocumentMouseDown(doc, 0, y) {
		t.Fatal("按下页面文本未进入拖选准备态")
	}
	if !engine.OnDocumentMouseDrag(doc, 399, y) {
		t.Fatal("鼠标移动未扩展页面文本选区")
	}
	engine.OnDocumentMouseUp(doc)

	got, ok := engine.SelectedText(doc)
	if !ok || got != "hello world" {
		t.Fatalf("页面文本拖选 = %q, ok=%v，want %q", got, ok, "hello world")
	}
	// 选区是纯绘制状态：拖选期间不该触发重排，只记重绘账。
	if doc.ChangeCount() != reflow {
		t.Errorf("拖选触发了重排（ChangeCount %d → %d）", reflow, doc.ChangeCount())
	}
	if engine.PaintCount(doc) == paint {
		t.Error("拖选没有记重绘账（PaintCount 未变，高亮不会刷新）")
	}
	// 高亮必须进入 SVG 导出（无头环境下的几何核对口径）。
	if svg := doc.DumpSVG(); !strings.Contains(svg, `fill-opacity="0.51"`) {
		t.Errorf("页面文本选区未导出高亮:\n%s", svg)
	}
}

func TestFlowSelectionAcrossParagraphsCopiesNewline(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; font: 16px monospace; }
	p { margin: 0; }
	</style></head><body><p id="a">Hello</p><p id="b">World</p></body></html>`
	doc := openDoc(t, 400, 160, src)
	a := mustEl(t, doc, "#a")
	b := mustEl(t, doc, "#b")
	ra, rb := a.GetBoundingClientRect(), b.GetBoundingClientRect()
	y1 := (ra.Top().Pixel() + ra.Bottom().Pixel()) / 2
	y2 := (rb.Top().Pixel() + rb.Bottom().Pixel()) / 2

	if !engine.OnDocumentMouseDown(doc, 0, y1) {
		t.Fatal("按下第一段未进入拖选准备态")
	}
	if !engine.OnDocumentMouseDrag(doc, 399, y2) {
		t.Fatal("拖到第二段未扩展选区")
	}
	engine.OnDocumentMouseUp(doc)

	got, ok := engine.SelectedText(doc)
	if !ok || got != "Hello\nWorld" {
		t.Fatalf("跨段落复制 = %q, ok=%v，want %q", got, ok, "Hello\nWorld")
	}
	if n, ok := engine.SelectionLength(doc); !ok || n != 11 {
		t.Fatalf("跨段落选区长度 = %d, ok=%v，want 11", n, ok)
	}
}

func TestFlowClickWithoutDragHasNoSelection(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; font: 16px monospace; }
	p { margin: 0; }
	</style></head><body><p id="p">hello</p></body></html>`
	doc := openDoc(t, 300, 100, src)
	p := mustEl(t, doc, "#p")
	r := p.GetBoundingClientRect()
	y := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	engine.OnDocumentMouseDown(doc, 5, y)
	engine.OnDocumentMouseUp(doc)
	engine.OnDocumentClick(doc, 5, y)
	if text, ok := engine.SelectedText(doc); ok {
		t.Fatalf("单击页面文本不应产生选区，却选中了 %q", text)
	}
}

func TestFlowSelectionClearedByInputDrag(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; font: 16px monospace; }
	p { margin: 0; }
	#i { width: 200px; font-size: 16px; }
	</style></head><body>
	<p id="p">page text</p><input id="i" value="hello world">
	</body></html>`
	doc := openDoc(t, 400, 160, src)
	p := mustEl(t, doc, "#p")
	pr := p.GetBoundingClientRect()
	py := (pr.Top().Pixel() + pr.Bottom().Pixel()) / 2
	engine.OnDocumentMouseDown(doc, 0, py)
	engine.OnDocumentMouseDrag(doc, 399, py)
	engine.OnDocumentMouseUp(doc)
	if _, ok := engine.SelectedText(doc); !ok {
		t.Fatal("页面文本没有选中（前置条件失败）")
	}

	in := mustEl(t, doc, "#i")
	ir := in.GetBoundingClientRect()
	iy := (ir.Top().Pixel() + ir.Bottom().Pixel()) / 2
	if !engine.OnDocumentMouseDown(doc, ir.Left().Pixel()+2, iy) {
		t.Fatal("按下输入框未进入拖选准备态")
	}
	engine.OnDocumentMouseDrag(doc, ir.Right().Pixel()-2, iy)
	engine.OnDocumentMouseUp(doc)

	got, ok := engine.SelectedText(doc)
	if !ok || got != "hello world" {
		t.Fatalf("输入框拖选 = %q, ok=%v，want %q（页面选区未让位）", got, ok, "hello world")
	}
}
