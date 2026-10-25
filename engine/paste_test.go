package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 粘贴（Ctrl+V）回归：引擎侧只负责「把一段文本落到光标处」，剪贴板
// 读写由后端完成。覆盖插入点、替换选区、单行折叠多行、textarea 保留
// 换行、password 掩码。

const pastePage = `<!doctype html><html><head><style>
body { margin: 0; }
#i { width: 300px; font-size: 16px; }
#p { width: 300px; font-size: 16px; }
#t { width: 200px; height: 80px; font-size: 16px; }
</style></head><body>
<input id="i" type="text">
<input id="p" type="password">
<textarea id="t"></textarea>
</body></html>`

// focusInput 聚焦指定控件（点其左缘 → 光标落在值首）。
func focusInput(t *testing.T, doc engine.HTMLDocument, sel string) engine.HTMLElement {
	t.Helper()
	in := mustEl(t, doc, sel)
	r := in.GetBoundingClientRect()
	engine.OnDocumentClick(doc, r.Left().Pixel()+2, (r.Top().Pixel()+r.Bottom().Pixel())/2)
	if engine.FocusedElement(doc) != in {
		t.Fatalf("聚焦 %s 失败", sel)
	}
	return in
}

func TestPasteInsertsAtCaret(t *testing.T) {
	doc := openDoc(t, 420, 240, pastePage)
	in := focusInput(t, doc, "#i")

	if !engine.OnDocumentPaste(doc, "hello") {
		t.Fatal("粘贴未消费")
	}
	if v := in.GetAttribute("value"); v != "hello" {
		t.Fatalf("粘贴后 value = %q, want hello", v)
	}
	// 光标落在插入文本之后：继续粘贴应追加而不是插到中间。
	if !engine.OnDocumentPaste(doc, " world") {
		t.Fatal("二次粘贴未消费")
	}
	if v := in.GetAttribute("value"); v != "hello world" {
		t.Fatalf("二次粘贴后 value = %q, want %q", v, "hello world")
	}
}

func TestPasteReplacesSelection(t *testing.T) {
	doc := openDoc(t, 420, 240, pastePage)
	in := focusInput(t, doc, "#i")

	engine.OnDocumentTextInput(doc, "hello world")
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)
	if got, ok := engine.SelectedText(doc); !ok || got != "hello world" {
		t.Fatalf("全选 = %q, ok=%v", got, ok)
	}

	if !engine.OnDocumentPaste(doc, "X") {
		t.Fatal("粘贴未消费")
	}
	if v := in.GetAttribute("value"); v != "X" {
		t.Fatalf("粘贴替换选区后 value = %q, want X", v)
	}
	if _, ok := engine.SelectedText(doc); ok {
		t.Fatal("粘贴后选区未折叠")
	}
}

// 单行 input 粘贴多行文本：换行折成空格（浏览器口径），连续换行只留
// 一个空格；textarea 保留换行。
func TestPasteLineBreakHandling(t *testing.T) {
	doc := openDoc(t, 420, 240, pastePage)
	in := focusInput(t, doc, "#i")
	if !engine.OnDocumentPaste(doc, "a\nb\n\nc") {
		t.Fatal("粘贴未消费")
	}
	if v := in.GetAttribute("value"); v != "a b c" {
		t.Fatalf("单行粘贴多行后 value = %q, want %q", v, "a b c")
	}
	// \r\n 视作一个换行
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)
	engine.OnDocumentPaste(doc, "x\r\ny")
	if v := in.GetAttribute("value"); v != "x y" {
		t.Fatalf("\\r\\n 粘贴后 value = %q, want %q", v, "x y")
	}

	ta := focusInput(t, doc, "#t")
	if !engine.OnDocumentPaste(doc, "a\nb") {
		t.Fatal("textarea 粘贴未消费")
	}
	if v := ta.GetAttribute("value"); v != "a\nb" {
		t.Fatalf("textarea 粘贴后 value = %q, want %q", v, "a\nb")
	}
	// textarea 的 CRLF/孤立 CR 归一成 '\n'（Windows 剪贴板），不留下 '\r'
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)
	engine.OnDocumentPaste(doc, "c\r\nd\re")
	if v := ta.GetAttribute("value"); v != "c\nd\ne" {
		t.Fatalf("textarea 粘贴 CRLF 后 value = %q, want %q", v, "c\nd\ne")
	}
}

// password 可粘贴：值是明文，绘制/SVG 一律按 ● 掩码。
func TestPasteIntoPasswordMasked(t *testing.T) {
	doc := openDoc(t, 420, 240, pastePage)
	p := focusInput(t, doc, "#p")

	if !engine.OnDocumentPaste(doc, "secret") {
		t.Fatal("粘贴未消费")
	}
	if v := p.GetAttribute("value"); v != "secret" {
		t.Fatalf("password value = %q, want secret", v)
	}
	dots := strings.Repeat("●", 6)
	buf := engine.NewBufferGraphics(420, 240)
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
}

// 无焦点 / 空内容时不消费（后端据此不改动文档）。
func TestPasteWithoutFocusNotConsumed(t *testing.T) {
	doc := openDoc(t, 420, 240, pastePage)
	if engine.OnDocumentPaste(doc, "hello") {
		t.Fatal("无焦点时粘贴不应被消费")
	}
	focusInput(t, doc, "#i")
	if engine.OnDocumentPaste(doc, "") {
		t.Fatal("空内容粘贴不应被消费")
	}
}

// 文本输入通道不得插入控制字符：Windows 把 Ctrl+C / Ctrl+V 这类组合
// 键以控制码（0x03 / 0x16）送进文本通道，照单插入会留下看不见的字符。
func TestTextInputDropsControlChars(t *testing.T) {
	doc := openDoc(t, 420, 240, pastePage)
	in := focusInput(t, doc, "#i")

	if !engine.OnDocumentTextInput(doc, "ab\x16cd") {
		t.Fatal("文本输入未消费")
	}
	if v := in.GetAttribute("value"); v != "abcd" {
		t.Fatalf("value = %q, want abcd（控制字符未被丢弃）", v)
	}
}
