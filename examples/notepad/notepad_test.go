package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 记事本示例的无头回归：界面排版、文件读写、撤销栈、菜单机制。
//
// 交互一律走引擎的公开入口（点击/选项浮层），不直接调应用内部方法，
// 这样菜单「点标题→弹选项→选中」这条链路也在覆盖范围内。

func newTestNotepad(t *testing.T) *notepad {
	t.Helper()
	vp := engine.NewHeadlessViewport(900, 620)
	doc, err := engine.OpenDocument(vp, string(page))
	if err != nil {
		t.Fatal(err)
	}
	np := &notepad{vp: vp, doc: doc}
	np.bind()
	doc.SetOnChanged(np.onChanged)
	np.syncLayout()
	doc.Refresh()
	return np
}

// clickAt 点击视口坐标处的元素。
func clickAt(t *testing.T, doc engine.HTMLDocument, x, y int) {
	t.Helper()
	doc.Refresh()
	engine.OnDocumentClick(doc, x, y)
}

// clickOption 点开 <select> 并选中 value 匹配的选项（走真实菜单链路：
// 先点 select 展开浮层，再点选项触发 change）。
func clickOption(t *testing.T, doc engine.HTMLDocument, selID, value string) {
	t.Helper()
	sel := doc.QuerySelector(selID)
	if sel == nil {
		t.Fatalf("找不到 %s", selID)
	}
	r := sel.GetBoundingClientRect()
	clickAt(t, doc, r.Left().Pixel()+2, r.Top().Pixel()+2)
	// 选项浮层的位置在布局期算：展开后要重排一次，选项才有可点的盒子。
	doc.Refresh()
	for _, o := range doc.GetElementsByTagName("option") {
		if o.GetAttribute("value") != value {
			continue
		}
		or := o.GetBoundingClientRect()
		if or.Right().Pixel() <= or.Left().Pixel() {
			t.Fatalf("选项 %q 没有布局盒（选项浮层未展开？）", value)
		}
		clickAt(t, doc, (or.Left().Pixel()+or.Right().Pixel())/2,
			(or.Top().Pixel()+or.Bottom().Pixel())/2)
		return
	}
	t.Fatalf("找不到值为 %q 的选项", value)
}

// 编辑区铺满「视口 - 菜单栏 - 路径栏 - 状态栏」：引擎的 CSS 没有 calc，
// 几何由应用层算并写进内联样式。
func TestEditorFillsWindow(t *testing.T) {
	np := newTestNotepad(t)
	r := np.editor.GetBoundingClientRect()
	if got := r.Top().Pixel(); got != editorY {
		t.Fatalf("编辑区上沿 = %d, want %d", got, editorY)
	}
	if got := r.Right().Pixel() - r.Left().Pixel(); got != 900 {
		t.Fatalf("编辑区宽 = %d, want 900", got)
	}
	wantH := 620 - editorY - statusH
	if got := r.Bottom().Pixel() - r.Top().Pixel(); got != wantH {
		t.Fatalf("编辑区高 = %d, want %d", got, wantH)
	}
}

// 窗口尺寸变化后编辑区跟着变：引擎在视口尺寸变化时补一次变更通知，
// 应用层据此重排（否则编辑区不会跟着窗口长宽变化）。
func TestEditorFollowsViewportResize(t *testing.T) {
	vp := engine.NewHeadlessViewport(900, 620)
	doc, err := engine.OpenDocument(vp, string(page))
	if err != nil {
		t.Fatal(err)
	}
	np := &notepad{vp: vp, doc: doc}
	np.bind()
	doc.SetOnChanged(np.onChanged)
	np.syncLayout()

	vp.W, vp.H = 640, 400
	doc.Refresh() // 后端在窗口缩放后强制重排，尺寸变化的通知随之发出
	doc.Refresh() // 通知里应用层改写了编辑区的内联样式，再排一次才生效
	// （真实窗口由后端按 ChangeCount 自动补这一遍）

	r := np.editor.GetBoundingClientRect()
	if got := r.Right().Pixel() - r.Left().Pixel(); got != 640 {
		t.Fatalf("缩放后编辑区宽 = %d, want 640", got)
	}
	if got := r.Bottom().Pixel() - r.Top().Pixel(); got != 400-editorY-statusH {
		t.Fatalf("缩放后编辑区高 = %d, want %d", got, 400-editorY-statusH)
	}
}

// 文件打开/保存：路径栏代替原生文件对话框。
func TestOpenAndSaveFile(t *testing.T) {
	np := newTestNotepad(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello\nworld"), 0o644); err != nil {
		t.Fatal(err)
	}

	np.loadFile(path)
	if got := np.editor.GetAttribute("value"); got != "hello\nworld" {
		t.Fatalf("打开后编辑区 = %q, want %q", got, "hello\nworld")
	}
	if np.path != path {
		t.Fatalf("当前文件 = %q, want %q", np.path, path)
	}
	if np.dirty {
		t.Fatal("刚打开的文件不应标记为已修改")
	}
	if got := np.pathIn.GetAttribute("value"); got != path {
		t.Fatalf("路径栏 = %q, want %q", got, path)
	}

	// 改内容 → 标记已修改 → 保存后落盘。
	np.editor.SetAttribute("value", "changed")
	if !np.dirty {
		t.Fatal("编辑后未标记为已修改")
	}
	np.saveFile()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "changed" {
		t.Fatalf("保存后文件 = %q, want %q", data, "changed")
	}
	if np.dirty {
		t.Fatal("保存后仍标记为已修改")
	}

	// 打开不存在的文件：状态栏提示，编辑区内容不变。
	np.editor.SetAttribute("value", "keep")
	np.loadFile(filepath.Join(dir, "missing.txt"))
	if got := np.editor.GetAttribute("value"); got != "keep" {
		t.Fatalf("打开失败后编辑区 = %q, want keep", got)
	}
}

// 撤销：每次文本变化压一条快照，撤销弹栈并写回。
func TestUndoRestoresPreviousText(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetAttribute("value", "abc")
	if len(np.undo) == 0 {
		t.Fatal("文本变化后没有压入撤销快照")
	}
	np.undoEdit()
	if got := np.editor.GetAttribute("value"); got != "" {
		t.Fatalf("撤销后编辑区 = %q, want 空", got)
	}
	// 撤销动作自身不再压快照（否则连撤会来回抖）。
	before := len(np.undo)
	np.undoEdit()
	if len(np.undo) != before {
		t.Fatalf("撤销后栈长度 = %d, want %d（撤销动作不该压栈）", len(np.undo), before)
	}
}

// 菜单链路：点「文件」→ 弹选项 → 选「新建」→ 编辑区清空；
// 菜单随后复位成标题（下次点击仍是标题态）。
func TestMenuActionClearsEditor(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetAttribute("value", "old text")

	clickOption(t, np.doc, "#m-file", "new")
	if got := np.editor.GetAttribute("value"); got != "" {
		t.Fatalf("「新建」后编辑区 = %q, want 空", got)
	}
	if sel := np.doc.QuerySelector("#m-file"); engine.SelectValue(sel) != "" {
		t.Fatalf("菜单未复位，select 值 = %q", engine.SelectValue(sel))
	}
}

// 视图 → 放大字号：改的是内联样式里的 font-size。
func TestViewZoomChangesFontSize(t *testing.T) {
	np := newTestNotepad(t)
	clickOption(t, np.doc, "#m-view", "zoomin")
	if np.font != baseFont+1 {
		t.Fatalf("放大后字号 = %d, want %d", np.font, baseFont+1)
	}
	if got := np.editor.GetAttribute("style"); !strings.Contains(got, "font-size:15px") {
		t.Fatalf("编辑区内联样式 = %q, want 含 font-size:15px", got)
	}
	clickOption(t, np.doc, "#m-view", "zoomreset")
	if np.font != baseFont {
		t.Fatalf("复位后字号 = %d, want %d", np.font, baseFont)
	}
}
