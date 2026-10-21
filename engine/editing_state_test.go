package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 编辑状态回归：失焦不丢光标与选区、光标位置查询、视口尺寸变化通知。
//
// 这三样都是页内菜单型应用（记事本一类）需要的底座：
//   - 失焦丢选区 → 点「复制」菜单时选区已被清空，菜单动作无选中可用；
//   - 光标位置 → 状态栏要显示「行 N，列 M」；
//   - 视口变化通知 → 窗口缩放不改文档内容，应用层收不到信号，
//     「编辑区铺满窗口」这类 chrome 就不会跟着窗口长宽变化。

// 选中文本后点击别处失焦，再把焦点交还控件：光标与选区原样保留。
func TestSelectionSurvivesBlur(t *testing.T) {
	const src = `<!doctype html><html><head><style>body { margin: 0; }</style></head>
	<body><input id="i" value="hello world"></body></html>`
	doc := openDoc(t, 420, 200, src)
	in := mustEl(t, doc, "#i")
	r := in.GetBoundingClientRect()
	engine.OnDocumentClick(doc, r.Left().Pixel()+2, (r.Top().Pixel()+r.Bottom().Pixel())/2)
	engine.OnDocumentKeyDownMods(doc, 'a', engine.ModCtrl)
	if got, ok := engine.SelectedText(doc); !ok || got != "hello world" {
		t.Fatalf("全选 = %q, ok=%v", got, ok)
	}

	// 点空白失焦：选区停止绘制，但状态保留。
	engine.OnDocumentClick(doc, 410, 190)
	if engine.FocusedElement(doc) != nil {
		t.Fatal("点空白后焦点未清除")
	}
	// 焦点交还后，选区与光标都还在（页内菜单动作据此作用于编辑区）。
	engine.SetFocusedElement(doc, in)
	if got, ok := engine.SelectedText(doc); !ok || got != "hello world" {
		t.Fatalf("重新聚焦后选区 = %q, ok=%v，want %q（失焦丢了编辑状态）", got, ok, "hello world")
	}
}

// 光标位置：单行 input 恒为第 1 行；textarea 按折行结果分行。
func TestCaretPosition(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; }
	#i { width: 300px; font-size: 16px; }
	#t { width: 200px; height: 80px; font-size: 16px; }
	</style></head><body>
	<input id="i" value="hello">
	<textarea id="t"></textarea>
	</body></html>`
	doc := openDoc(t, 420, 240, src)
	in := mustEl(t, doc, "#i")
	engine.SetFocusedElement(doc, in)
	if line, col, ok := engine.CaretPosition(in); !ok || line != 1 || col != 6 {
		t.Fatalf("input 光标位置 = %d,%d ok=%v，want 1,6", line, col, ok)
	}

	ta := mustEl(t, doc, "#t")
	engine.SetValue(ta, "abc\ndef")
	engine.SetFocusedElement(doc, ta)
	// 值末尾 = 第二行第 4 列。
	if line, col, ok := engine.CaretPosition(ta); !ok || line != 2 || col != 4 {
		t.Fatalf("textarea 光标位置 = %d,%d ok=%v，want 2,4", line, col, ok)
	}
	// 非可编辑元素没有光标位置。
	if _, _, ok := engine.CaretPosition(doc.QuerySelector("body")); ok {
		t.Fatal("body 不应有光标位置")
	}
}

// 视口尺寸变化要通知文档：窗口缩放不改内容，只靠 onChanged 收不到信号。
func TestViewportResizeNotifiesDocument(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, `<!doctype html><html><body><div>x</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	doc.SetOnChanged(func() { changes++ })

	vp.W, vp.H = 500, 400
	doc.Refresh()
	if changes == 0 {
		t.Fatal("视口尺寸变化未触发变更通知（应用层收不到缩放信号）")
	}

	// 尺寸没变时不应反复通知（否则每次重排都会多一次变更→重排循环）。
	after := changes
	doc.Refresh()
	if changes != after {
		t.Fatalf("尺寸未变却多通知了 %d 次（会形成变更→重排循环）", changes-after)
	}
}
