package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 输入框选区回归：鼠标拖选、Ctrl+A、Ctrl+C（引擎只提供选区文本）、
// 输入/退格替换选区、方向键折叠选区，以及 SVG 中选区高亮的导出。

func TestInputMouseDragSelectsText(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; }
	#i { width: 300px; font-size: 16px; padding: 2px; border: 1px solid #000; }
	</style></head><body><input id="i" value="hello world"></body></html>`
	doc := openDoc(t, 420, 200, src)
	in := mustEl(t, doc, "#i")
	r := in.GetBoundingClientRect()
	x1 := r.Left().Pixel() + 2
	x2 := r.Right().Pixel() - 2
	y := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	if !engine.OnDocumentMouseDown(doc, x1, y) {
		t.Fatal("按下输入框未进入拖选准备态")
	}
	if !engine.OnDocumentMouseDrag(doc, x2, y) {
		t.Fatal("鼠标移动未扩选")
	}
	engine.OnDocumentMouseUp(doc)

	got, ok := engine.SelectedText(doc)
	if !ok || got != "hello world" {
		t.Fatalf("拖选结果 = %q, ok=%v，want %q（光标映射或选区锚点错位）", got, ok, "hello world")
	}
	// Ctrl+C 在引擎侧只表示“有选区可复制”，后端据此写剪贴板。
	if !engine.OnDocumentKeyDownMods(doc, 'c', engine.ModCtrl) {
		t.Fatal("Ctrl+C 未识别为复制按键")
	}
	if got, ok := engine.SelectedText(doc); !ok || got != "hello world" {
		t.Fatalf("复制按键后选区被破坏: %q, ok=%v", got, ok)
	}
}

func TestInputCtrlASelectAllAndReplace(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; }
	#i { width: 300px; font-size: 16px; }
	</style></head><body><input id="i" value="hello world"></body></html>`
	doc := openDoc(t, 420, 200, src)
	in := mustEl(t, doc, "#i")
	r := in.GetBoundingClientRect()
	engine.OnDocumentClick(doc, (r.Left().Pixel()+r.Right().Pixel())/2,
		(r.Top().Pixel()+r.Bottom().Pixel())/2)

	if !engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl) {
		t.Fatal("Ctrl+A 未消费")
	}
	if got, ok := engine.SelectedText(doc); !ok || got != "hello world" {
		t.Fatalf("Ctrl+A 选区 = %q, ok=%v", got, ok)
	}
	// 选区高亮必须进入 SVG 导出，否则无头环境只能验证状态、看不到几何。
	if svg := doc.DumpSVG(); !strings.Contains(svg, `fill-opacity="0.51"`) {
		t.Errorf("SVG 未导出选区高亮:\n%s", svg)
	}

	// 输入可打印字符应替换整个选区，而不是在末尾追加。
	if !engine.OnDocumentTextInput(doc, "X") {
		t.Fatal("文本输入未消费")
	}
	if got := in.GetAttribute("value"); got != "X" {
		t.Fatalf("选区替换后 value = %q, want X", got)
	}
	if _, ok := engine.SelectedText(doc); ok {
		t.Fatal("替换后选区未折叠")
	}

	// 再全选后按退格：整段删除。
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)
	if !engine.OnDocumentKeyDown(doc, engine.KeyBackspace) {
		t.Fatal("退格未消费")
	}
	if got := in.GetAttribute("value"); got != "" {
		t.Fatalf("退格删除选区后 value = %q, want 空", got)
	}
}

func TestDeleteKeyRemovesForwardAndSelection(t *testing.T) {
	const src = `<!doctype html><html><head><style>body { margin: 0; }</style></head>
	<body><input id="i" value="hello"></body></html>`
	doc := openDoc(t, 320, 200, src)
	in := mustEl(t, doc, "#i")
	r := in.GetBoundingClientRect()
	// 点左缘 → 光标在 0；Delete 删除光标后的字符（不是 Backspace）。
	engine.OnDocumentClick(doc, r.Left().Pixel()+2, (r.Top().Pixel()+r.Bottom().Pixel())/2)
	if !engine.OnDocumentKeyDown(doc, engine.KeyDelete) {
		t.Fatal("Delete 未消费")
	}
	if got := in.GetAttribute("value"); got != "ello" {
		t.Fatalf("Delete 后 value = %q, want ello（KeyDelete 键码错会完全删不动）", got)
	}
	// 有选区时 Delete 删除整个选区。
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)
	if !engine.OnDocumentKeyDown(doc, engine.KeyDelete) {
		t.Fatal("Delete 删除选区未消费")
	}
	if got := in.GetAttribute("value"); got != "" {
		t.Fatalf("Delete 删除选区后 value = %q, want 空", got)
	}
}

func TestInputArrowCollapsesSelection(t *testing.T) {
	const src = `<!doctype html><html><head><style>body { margin: 0; }</style></head>
	<body><input id="i" value="hello"></body></html>`
	doc := openDoc(t, 420, 200, src)
	in := mustEl(t, doc, "#i")
	r := in.GetBoundingClientRect()
	engine.OnDocumentClick(doc, (r.Left().Pixel()+r.Right().Pixel())/2,
		(r.Top().Pixel()+r.Bottom().Pixel())/2)
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)
	if _, ok := engine.SelectedText(doc); !ok {
		t.Fatal("Ctrl+A 未建立选区")
	}
	if !engine.OnDocumentKeyDown(doc, engine.KeyRight) {
		t.Fatal("右方向键未消费")
	}
	if _, ok := engine.SelectedText(doc); ok {
		t.Fatal("方向键后选区未折叠（浏览器会把活动端收敛到端点）")
	}
}

func TestLongInputCtrlASelectsHiddenPrefix(t *testing.T) {
	// 单行 input 超宽时渲染舍头保尾；选区/复制必须仍按完整 value 计算，
	// 否则被裁掉的前缀永远无法复制。
	const long = "0123456789012345678901234567890123456789"
	const src = `<!doctype html><html><head><style>
	body { margin: 0; }
	#i { width: 80px; font-size: 16px; }
	</style></head><body><input id="i" value="` + long + `"></body></html>`
	doc := openDoc(t, 320, 200, src)
	in := mustEl(t, doc, "#i")
	r := in.GetBoundingClientRect()
	engine.OnDocumentClick(doc, r.Right().Pixel()-2, (r.Top().Pixel()+r.Bottom().Pixel())/2)
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)
	if got, ok := engine.SelectedText(doc); !ok || got != long {
		t.Fatalf("超宽 input 全选 = %q, ok=%v，want 完整 %q（选区被可见窗口截断）", got, ok, long)
	}
}

func TestPasswordSelectionCopiesRealValue(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; }
	#p { width: 200px; font-size: 16px; }
	</style></head><body><input id="p" type="password" value="secret"></body></html>`
	doc := openDoc(t, 320, 200, src)
	in := mustEl(t, doc, "#p")
	r := in.GetBoundingClientRect()
	engine.OnDocumentClick(doc, (r.Left().Pixel()+r.Right().Pixel())/2,
		(r.Top().Pixel()+r.Bottom().Pixel())/2)
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)

	if got, ok := engine.SelectedText(doc); !ok || got != "secret" {
		t.Fatalf("password 选区文本 = %q, ok=%v，want 明文 secret（复制取真实值）", got, ok)
	}
	svg := doc.DumpSVG()
	if strings.Contains(svg, "secret") {
		t.Error("SVG 泄露了 password 明文")
	}
	if !strings.Contains(svg, "●") {
		t.Error("SVG 未按 ● 掩码导出 password")
	}
}

func TestTextareaCtrlASelectsAcrossLines(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; }
	#t { width: 200px; height: 80px; font-size: 16px; }
	</style></head><body><textarea id="t"></textarea></body></html>`
	doc := openDoc(t, 420, 240, src)
	ta := mustEl(t, doc, "#t")
	engine.SetValue(ta, "abc\ndef")
	r := ta.GetBoundingClientRect()
	engine.OnDocumentClick(doc, r.Left().Pixel()+2, r.Top().Pixel()+2)
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)

	got, ok := engine.SelectedText(doc)
	if !ok || got != "abc\ndef" {
		t.Fatalf("textarea 全选 = %q, ok=%v，want %q", got, ok, "abc\ndef")
	}
	// 跨行选区每行一块高亮（两行文本各一块）。
	if svg := doc.DumpSVG(); strings.Count(svg, `fill-opacity="0.51"`) < 2 {
		t.Errorf("textarea 跨行选区未逐行导出高亮:\n%s", svg)
	}
}
