package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 记事本示例的无头回归：界面排版、文件读写、撤销/重做、菜单机制。
//
// 交互一律走引擎的公开入口（点击命中测试），不直接调应用内部方法，这样
// 「点菜单标签→浮层展开→点菜单项」这条链路也在覆盖范围内。

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

// clickMenu 走真实菜单链路：点菜单栏标签 → 浮层面板展开 → 点菜单项。
func clickMenu(t *testing.T, np *notepad, title, label string) {
	t.Helper()
	np.doc.Refresh()

	var head engine.HTMLElement
	for _, l := range np.menuLabels {
		if l.InnerText() == title {
			head = l
			break
		}
	}
	if head == nil {
		t.Fatalf("菜单栏里没有 %q", title)
	}
	r := head.GetBoundingClientRect()
	clickAt(t, np.doc, r.Left().Pixel()+2, r.Top().Pixel()+2)

	// 浮层里的菜单项：展开后要重排一次，项才有可点的盒子。
	np.doc.Refresh()
	for _, item := range np.menuPop.Children() {
		if item.InnerText() != label {
			continue
		}
		ir := item.GetBoundingClientRect()
		if ir.Right().Pixel() <= ir.Left().Pixel() {
			t.Fatalf("菜单项 %q 没有布局盒（浮层未展开？）", label)
		}
		clickAt(t, np.doc, (ir.Left().Pixel()+ir.Right().Pixel())/2,
			(ir.Top().Pixel()+ir.Bottom().Pixel())/2)
		return
	}
	t.Fatalf("菜单 %q 里没有 %q", title, label)
}

// 编辑区铺满「视口 - 菜单栏 - 工具栏 - 状态栏」：引擎的 CSS 没有 calc，
// 几何由应用层算并写进内联样式。
func TestEditorFillsWindow(t *testing.T) {
	np := newTestNotepad(t)
	r := np.editor.GetBoundingClientRect()
	top := menuH + toolH
	if got := r.Top().Pixel(); got != top {
		t.Fatalf("编辑区上沿 = %d, want %d", got, top)
	}
	if got := r.Right().Pixel() - r.Left().Pixel(); got != 900 {
		t.Fatalf("编辑区宽 = %d, want 900", got)
	}
	wantH := 620 - top - statusH
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
	wantH := 400 - (menuH + toolH) - statusH
	if got := r.Bottom().Pixel() - r.Top().Pixel(); got != wantH {
		t.Fatalf("缩放后编辑区高 = %d, want %d", got, wantH)
	}
}

// 文件打开/保存：走原生对话框之外的路径（应用层的 loadFile/writeFile）。
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

// 重做：撤销之后能把内容恢复回去；新的改动会让重做栈作废。
func TestRedoRestoresUndoneText(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetAttribute("value", "abc")
	np.undoEdit()
	if got := np.editor.GetAttribute("value"); got != "" {
		t.Fatalf("撤销后编辑区 = %q, want 空", got)
	}
	np.redoEdit()
	if got := np.editor.GetAttribute("value"); got != "abc" {
		t.Fatalf("重做后编辑区 = %q, want %q", got, "abc")
	}

	// 撤销之后又有新改动：重做栈作废。
	np.editor.SetAttribute("value", "xyz")
	np.undoEdit()
	np.editor.SetAttribute("value", "new")
	np.redoEdit()
	if got := np.editor.GetAttribute("value"); got != "new" {
		t.Fatalf("新改动后重做 = %q, want new（重做栈应已作废）", got)
	}
}

// 菜单链路：点「文件」→ 浮层展开 → 点「新建」→ 编辑区清空。
func TestMenuActionClearsEditor(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetAttribute("value", "old text")

	clickMenu(t, np, "文件", "新建")
	if got := np.editor.GetAttribute("value"); got != "" {
		t.Fatalf("「新建」后编辑区 = %q, want 空", got)
	}
}

// 菜单浮层：点标签打开、点菜单项执行后关闭、点别处也关闭。
func TestMenuPopupOpensAndCloses(t *testing.T) {
	np := newTestNotepad(t)

	// 点菜单栏标签：浮层展开。
	np.doc.Refresh()
	head := np.menuLabels[0]
	hr := head.GetBoundingClientRect()
	clickAt(t, np.doc, hr.Left().Pixel()+2, hr.Top().Pixel()+2)
	np.doc.Refresh()
	if !np.menuOpen {
		t.Fatal("点菜单标签后浮层没有打开")
	}
	if got := np.menuPop.GetAttribute("style"); !strings.Contains(got, "display:block") {
		t.Fatalf("浮层样式 = %q, want 含 display:block", got)
	}

	// 点菜单项：动作执行、浮层收起。
	clickMenu(t, np, "编辑", "全选")
	if np.menuOpen {
		t.Fatal("点菜单项后浮层没有关闭")
	}
	if got := np.menuPop.GetAttribute("style"); !strings.Contains(got, "display:none") {
		t.Fatalf("浮层样式 = %q, want 含 display:none", got)
	}

	// 点浮层之外（编辑区）也应关闭菜单。
	np.doc.Refresh()
	head = np.menuLabels[0]
	hr = head.GetBoundingClientRect()
	clickAt(t, np.doc, hr.Left().Pixel()+2, hr.Top().Pixel()+2)
	if !np.menuOpen {
		t.Fatal("浮层没有再次打开")
	}
	r := np.editor.GetBoundingClientRect()
	clickAt(t, np.doc, r.Left().Pixel()+10, r.Top().Pixel()+10)
	if np.menuOpen {
		t.Fatal("点浮层之外后菜单没有关闭")
	}
}

// 无头模式没有原生对话框：「打开」要在状态栏说明白，而不是静默无反应。
func TestHeadlessOpenExplainsNoDialog(t *testing.T) {
	np := newTestNotepad(t)
	clickMenu(t, np, "文件", "打开…")
	if msg := np.stMsg.InnerText(); !strings.Contains(msg, "原生窗口") {
		t.Errorf("状态栏提示 = %q, want 含「原生窗口」", msg)
	}
}

// 格式 → 放大字号：改的是内联样式里的 font-size。
func TestFormatZoomChangesFontSize(t *testing.T) {
	np := newTestNotepad(t)
	clickMenu(t, np, "格式", "放大字号")
	if np.font != baseFont+1 {
		t.Fatalf("放大后字号 = %d, want %d", np.font, baseFont+1)
	}
	if got := np.editor.GetAttribute("style"); !strings.Contains(got, "font-size:15px") {
		t.Fatalf("编辑区内联样式 = %q, want 含 font-size:15px", got)
	}
	clickMenu(t, np, "格式", "默认字号")
	if np.font != baseFont {
		t.Fatalf("复位后字号 = %d, want %d", np.font, baseFont)
	}
}

// 查找/替换：查找报出位置，替换改文本，全部替换改所有匹配。
func TestFindAndReplace(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetAttribute("value", "hello world hello")

	np.findIn.SetAttribute("value", "hello")
	np.findNext()
	if np.matchAt != 0 {
		t.Fatalf("第一次查找位置 = %d, want 0", np.matchAt)
	}
	if msg := np.stMsg.InnerText(); !strings.Contains(msg, "第 1 处") {
		t.Errorf("状态栏提示 = %q, want 含「第 1 处」", msg)
	}

	np.replaceIn.SetAttribute("value", "HI")
	np.replaceCurrent()
	if got := np.editor.GetAttribute("value"); got != "HI world hello" {
		t.Fatalf("替换一处后 = %q, want %q", got, "HI world hello")
	}

	np.replaceAll()
	if got := np.editor.GetAttribute("value"); got != "HI world HI" {
		t.Fatalf("全部替换后 = %q, want %q", got, "HI world HI")
	}
}

// 视图 → 显示状态栏：状态栏可隐藏，编辑区相应变高。
func TestToggleStatusBar(t *testing.T) {
	np := newTestNotepad(t)
	before := np.editor.GetBoundingClientRect().Bottom().Pixel() -
		np.editor.GetBoundingClientRect().Top().Pixel()

	clickMenu(t, np, "视图", "显示状态栏")
	np.doc.Refresh()
	r := np.editor.GetBoundingClientRect()
	got := r.Bottom().Pixel() - r.Top().Pixel()
	if got != before+statusH {
		t.Fatalf("隐藏状态栏后编辑区高 = %d, want %d", got, before+statusH)
	}
	if got := np.stBar.GetAttribute("style"); !strings.Contains(got, "display:none") {
		t.Fatalf("状态栏样式 = %q, want 含 display:none", got)
	}
}
