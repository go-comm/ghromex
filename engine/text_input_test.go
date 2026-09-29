package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

const inputPage = `<!doctype html><html><head><style>
body { margin: 0; }
.in { display: inline-block; width: 180px; padding: 6px; border: 1px solid #CBD5E1; }
</style></head><body>
<div><input class="in" id="a" type="text"></div>
<div><input class="in" id="b" type="password"></div>
<div><input class="in" id="c" type="text"></div>
</body></html>`

func focusXY(t *testing.T, doc engine.HTMLDocument, sel string) (int, int) {
	t.Helper()
	el := doc.QuerySelector(sel)
	if el == nil {
		t.Fatalf("%s not found", sel)
	}
	r := el.GetBoundingClientRect()
	return (r.Left().Pixel() + r.Right().Pixel()) / 2, (r.Top().Pixel() + r.Bottom().Pixel()) / 2
}

func TestInputFocusAndEdit(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, inputPage)
	if err != nil {
		t.Fatal(err)
	}
	a := doc.QuerySelector("#a")
	c := doc.QuerySelector("#c")
	if a == nil || c == nil {
		t.Fatal("inputs not found")
	}

	if engine.FocusedElement(doc) != nil {
		t.Fatal("initial focus must be nil")
	}

	// 点击可编辑 input → 聚焦
	x, y := focusXY(t, doc, "#a")
	engine.OnDocumentClick(doc, x, y)
	if engine.FocusedElement(doc) != a {
		t.Fatalf("focus after click = %v, want #a", engine.FocusedElement(doc))
	}

	// 输入文本
	if !engine.OnDocumentTextInput(doc, "hello") {
		t.Fatal("TextInput must be consumed while focused")
	}
	if v := a.GetAttribute("value"); v != "hello" {
		t.Fatalf("value = %q, want hello", v)
	}

	// Backspace 按 rune 删除（多字节安全）
	engine.OnDocumentTextInput(doc, "你好")
	if !engine.OnDocumentKeyDown(doc, engine.KeyBackspace) {
		t.Fatal("Backspace must be consumed")
	}
	if v := a.GetAttribute("value"); v != "hello你" {
		t.Fatalf("value = %q, want hello你", v)
	}

	// Tab 在可编辑输入间循环：#a → #c（password 不在 v1 编辑集）→ 回绕 #a
	if !engine.OnDocumentKeyDown(doc, engine.KeyTab) {
		t.Fatal("Tab must be consumed")
	}
	if engine.FocusedElement(doc) != c {
		t.Fatalf("focus after Tab = %v, want #c", engine.FocusedElement(doc))
	}
	engine.OnDocumentKeyDown(doc, engine.KeyTab)
	if engine.FocusedElement(doc) != a {
		t.Fatalf("focus after wrap Tab = %v, want #a", engine.FocusedElement(doc))
	}

	// 点击空白 → 失焦
	engine.OnDocumentClick(doc, 390, 290)
	if engine.FocusedElement(doc) != nil {
		t.Fatal("click blank must clear focus")
	}

	// 无焦点时输入不消费
	if engine.OnDocumentTextInput(doc, "x") {
		t.Fatal("TextInput without focus must not be consumed")
	}
}

func TestInputValueRendered(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, inputPage)
	if err != nil {
		t.Fatal(err)
	}
	a := doc.QuerySelector("#a")
	a.SetAttribute("value", "secret name")
	engine.LayoutDocument(doc)

	buf := engine.NewBufferGraphics(400, 300)
	engine.RenderNode(buf, doc)
	found := false
	for _, s := range buf.Texts {
		if s == "secret name" {
			found = true
		}
	}
	if !found {
		t.Fatalf("input value text not drawn, Texts=%v", buf.Texts)
	}
}
