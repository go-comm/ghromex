package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

const textareaPage = `<!doctype html><html><head><style>
body { margin: 0; }
#t { display: block; width: 100px; height: 60px; }
</style></head><body>
<textarea id="t">aaaaaaaaaa bbbbbbbbbb cccccccccc</textarea>
</body></html>`

// 解析：textarea 的初始内容进 value 属性（原始文本元素），不建文本子节点；
// 盒尺寸按作者 width/height；渲染按宽度折行为多行。
func TestTextareaParseWrapAndRender(t *testing.T) {
	doc := openDoc(t, 400, 300, textareaPage)
	ta := mustEl(t, doc, "#t")

	const wantVal = "aaaaaaaaaa bbbbbbbbbb cccccccccc"
	if v := ta.GetAttribute("value"); v != wantVal {
		t.Fatalf("value = %q, want %q", v, wantVal)
	}
	if n := len(ta.Children()); n != 0 {
		t.Fatalf("textarea 子节点 = %d, want 0（应按原始文本解析为 value）", n)
	}
	r := ta.GetBoundingClientRect()
	if w := r.Right().Pixel() - r.Left().Pixel(); w != 100 {
		t.Errorf("盒宽 = %d, want 100", w)
	}
	if h := r.Bottom().Pixel() - r.Top().Pixel(); h != 60 {
		t.Errorf("盒高 = %d, want 60", h)
	}

	buf := engine.NewBufferGraphics(400, 300)
	engine.RenderNode(buf, ta)
	joined := strings.Join(buf.Texts, "|")
	for _, want := range []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"} {
		if !strings.Contains(joined, want) {
			t.Errorf("渲染缺折行片段 %q，texts=%q", want, joined)
		}
	}
	// 每个折行片段都必须落在内容盒内（无裁剪原语，溢出即画出界）
	lx := ta.X()
	if lx == 0 {
		t.Fatal("内容原点应含 border+padding")
	}
	for _, s := range buf.Texts {
		if !strings.Contains(s, "aaaa") && !strings.Contains(s, "bbbb") && !strings.Contains(s, "cccc") {
			continue
		}
		if w, _ := fakeWidth(t, doc, s); w > 90 {
			t.Errorf("行 %q 宽 %d 超出内容宽 90", s, w)
		}
	}
}

// fakeWidth 用 dump 的估算口径量一段文本宽（仅测试用）。
func fakeWidth(t *testing.T, doc engine.HTMLDocument, s string) (int, int) {
	t.Helper()
	g := engine.NewFakeGraphics()
	w, h := g.MeasureText(s, 13, false, "")
	return w, h
}

// 编辑闭环：点选落光标 → 就地插入 → Enter 换行 → Backspace 删除 → 方向键移动。
func TestTextareaCaretEditing(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body { margin: 0; }
#t { display: block; width: 120px; height: 60px; }
</style></head><body>
<textarea id="t">hello
world</textarea>
</body></html>`
	doc := openDoc(t, 400, 300, src)
	ta := mustEl(t, doc, "#t")

	// 首个换行按规范忽略 → value = "hello\nworld"
	if v := ta.GetAttribute("value"); v != "hello\nworld" {
		t.Fatalf("value = %q, want hello\\nworld", v)
	}

	// 点击第二行行首 → 聚焦 + 光标落到该行
	r := ta.GetBoundingClientRect()
	x := r.Left().Pixel() + 4 + 1         // border 1 + padding 4，再进 1px
	y := r.Top().Pixel() + 1 + 4 + 16 + 2 // 第二行（行高 16）
	engine.OnDocumentClick(doc, x, y)
	if engine.FocusedElement(doc) != ta {
		t.Fatalf("点击 textarea 未聚焦: %v", engine.FocusedElement(doc))
	}
	if !engine.OnDocumentTextInput(doc, "X") {
		t.Fatal("TextInput 未被消费")
	}
	if v := ta.GetAttribute("value"); v != "hello\nXworld" {
		t.Fatalf("点击定位插入后 value = %q, want hello\\nXworld", v)
	}

	// Home → 行首，再插入
	engine.OnDocumentKeyDown(doc, engine.KeyHome)
	engine.OnDocumentTextInput(doc, "Y")
	if v := ta.GetAttribute("value"); v != "hello\nYXworld" {
		t.Fatalf("Home 后插入 value = %q", v)
	}
	// End → 行尾
	engine.OnDocumentKeyDown(doc, engine.KeyEnd)
	engine.OnDocumentTextInput(doc, "!")
	if v := ta.GetAttribute("value"); v != "hello\nYXworld!" {
		t.Fatalf("End 后插入 value = %q", v)
	}
	// 方向键：Left 一格后插入
	engine.OnDocumentKeyDown(doc, engine.KeyLeft)
	engine.OnDocumentTextInput(doc, "@")
	if v := ta.GetAttribute("value"); v != "hello\nYXworld@!" {
		t.Fatalf("Left 后插入 value = %q", v)
	}
	// Backspace：先删刚插入的 @，再把光标移到行首删掉换行接起两行
	engine.OnDocumentKeyDown(doc, engine.KeyBackspace)
	if v := ta.GetAttribute("value"); v != "hello\nYXworld!" {
		t.Fatalf("Backspace 后 value = %q", v)
	}
	engine.OnDocumentKeyDown(doc, engine.KeyHome) // 第二行行首
	engine.OnDocumentKeyDown(doc, engine.KeyBackspace)
	if v := ta.GetAttribute("value"); v != "helloYXworld!" {
		t.Fatalf("删行首换行后 value = %q, want helloYXworld!", v)
	}
	// Enter 换行（textarea 专属；input 上不应消费）
	engine.OnDocumentKeyDown(doc, engine.KeyEnter)
	if v := ta.GetAttribute("value"); v != "hello\nYXworld!" {
		t.Fatalf("Enter 后 value = %q", v)
	}
}

// input 上 Enter 不消费（无表单提交语义）；Backspace/方向键照常。
func TestEnterNotConsumedOnInput(t *testing.T) {
	doc := openDoc(t, 400, 300, inputPage)
	f := doc.QuerySelector("#a")
	engine.SetFocusedElement(doc, f)
	engine.OnDocumentTextInput(doc, "ab")
	if engine.OnDocumentKeyDown(doc, engine.KeyEnter) {
		t.Error("input 上 Enter 不应被消费")
	}
	if v := f.GetAttribute("value"); v != "ab" {
		t.Fatalf("value = %q, want ab", v)
	}
	engine.OnDocumentKeyDown(doc, engine.KeyBackspace)
	if v := f.GetAttribute("value"); v != "a" {
		t.Fatalf("Backspace 后 value = %q, want a", v)
	}
}

// Tab 循环应把 textarea 一并纳入可聚焦控件。
func TestTabCycleIncludesTextarea(t *testing.T) {
	const src = `<!doctype html><html><head><style>body{margin:0}</style></head><body>
	<input id="i" type="text" value="one">
	<textarea id="t">two</textarea>
	</body></html>`
	doc := openDoc(t, 400, 300, src)
	i := mustEl(t, doc, "#i")
	ta := mustEl(t, doc, "#t")

	engine.SetFocusedElement(doc, i)
	if engine.FocusedElement(doc) != i {
		t.Fatal("input 未聚焦")
	}
	if !engine.OnDocumentKeyDown(doc, engine.KeyTab) {
		t.Fatal("Tab 未被消费")
	}
	if engine.FocusedElement(doc) != ta {
		t.Fatalf("Tab 后焦点 = %v, want textarea", engine.FocusedElement(doc))
	}
	// 聚焦时光标应在值末尾，继续输入追加
	engine.OnDocumentTextInput(doc, "!")
	if v := ta.GetAttribute("value"); v != "two!" {
		t.Fatalf("value = %q, want two!", v)
	}
}

// SetValue/GetValue 覆盖 textarea（写值 + 光标归位）。
func TestTextareaSetValue(t *testing.T) {
	doc := openDoc(t, 400, 300, textareaPage)
	ta := mustEl(t, doc, "#t")
	engine.SetValue(ta, "line1\nline2")
	if v := engine.GetValue(ta); v != "line1\nline2" {
		t.Fatalf("GetValue = %q", v)
	}
	// 赋值后点末尾应追加（光标在末尾）
	r := ta.GetBoundingClientRect()
	engine.OnDocumentClick(doc, r.Right().Pixel()-4, r.Bottom().Pixel()-4)
	engine.OnDocumentTextInput(doc, "Z")
	if v := ta.GetAttribute("value"); v != "line1\nline2Z" {
		t.Fatalf("末尾追加失败: %q", v)
	}
}
