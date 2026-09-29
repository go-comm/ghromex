package engine_test

import (
	"strings"
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
	b := doc.QuerySelector("#b")
	c := doc.QuerySelector("#c")
	if a == nil || b == nil || c == nil {
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

	// Tab 在可编辑输入间循环（password 同在编辑集）：#a → #b → #c → 回绕 #a
	if !engine.OnDocumentKeyDown(doc, engine.KeyTab) {
		t.Fatal("Tab must be consumed")
	}
	if engine.FocusedElement(doc) != b {
		t.Fatalf("focus after Tab = %v, want #b(password)", engine.FocusedElement(doc))
	}
	engine.OnDocumentKeyDown(doc, engine.KeyTab)
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

// password 与 text 同为可编辑输入：可点击聚焦、可键入/删除；绘制与 SVG
// 导出一律按 ● 掩码、明文不落画（此前 password 不在编辑集，点不进也敲不进）。
func TestPasswordInputEditableAndMasked(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, inputPage)
	if err != nil {
		t.Fatal(err)
	}
	b := doc.QuerySelector("#b")
	if b == nil {
		t.Fatal("password input not found")
	}

	// 点击密码框 → 聚焦
	x, y := focusXY(t, doc, "#b")
	engine.OnDocumentClick(doc, x, y)
	if engine.FocusedElement(doc) != b {
		t.Fatalf("focus after click = %v, want #b(password)", engine.FocusedElement(doc))
	}

	// 键入与删除
	if !engine.OnDocumentTextInput(doc, "secret") {
		t.Fatal("TextInput must be consumed while password focused")
	}
	if v := b.GetAttribute("value"); v != "secret" {
		t.Fatalf("value = %q, want secret", v)
	}
	if !engine.OnDocumentKeyDown(doc, engine.KeyBackspace) {
		t.Fatal("Backspace must be consumed")
	}
	if v := b.GetAttribute("value"); v != "secre" {
		t.Fatalf("value after Backspace = %q, want secre", v)
	}

	// 绘制：6 位掩码（重新输入满 6 位再查）、明文绝不出现
	if !engine.OnDocumentTextInput(doc, "t") {
		t.Fatal("TextInput must be consumed")
	}
	dots := strings.Repeat("●", 6)
	buf := engine.NewBufferGraphics(400, 300)
	engine.RenderNode(buf, doc)
	gotMask, gotPlain := false, false
	for _, s := range buf.Texts {
		if s == dots {
			gotMask = true
		}
		if strings.Contains(s, "secret") {
			gotPlain = true
		}
	}
	if !gotMask {
		t.Fatalf("掩码文本未绘制, Texts=%v", buf.Texts)
	}
	if gotPlain {
		t.Fatalf("password 明文被绘制, Texts=%v", buf.Texts)
	}

	// SVG 导出与渲染同构：含掩码、不含明文
	svg := doc.DumpSVG()
	if !strings.Contains(svg, dots) {
		t.Fatalf("SVG 未含掩码文本: %.200s", svg)
	}
	if strings.Contains(svg, "secret") {
		t.Fatal("SVG 含 password 明文")
	}
}
