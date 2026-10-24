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
	doc, err := engine.OpenDocument(vp, "")
	if err != nil {
		t.Fatal(err)
	}
	np := &notepad{vp: vp, doc: doc}
	np.bind()
	doc.Refresh()
	return np
}

// clickAt 点击视口坐标处的元素。
func clickAt(t *testing.T, doc engine.HTMLDocument, x, y int) {
	t.Helper()
	doc.Refresh()
	engine.OnDocumentClick(doc, x, y)
}

// menuHead 返回标题对应的 ui 菜单标签。
func menuHead(t *testing.T, np *notepad, title string) engine.HTMLElement {
	t.Helper()
	for _, el := range np.doc.QuerySelectorAll(".system-ui-menu") {
		if el.Text() == title {
			return el
		}
	}
	t.Fatalf("菜单栏里没有 %q", title)
	return nil
}

// menuItemLabel 取菜单项的文字（第一个无类名的 span；快捷键/图标有类名）。
func menuItemLabel(item engine.HTMLElement) string {
	for _, child := range item.Children() {
		if child.GetAttribute("class") == "" {
			return child.InnerText()
		}
	}
	return ""
}

// statusLeft/statusRight 取 ui 状态栏左右两段的文字。
func statusLeft(np *notepad) string {
	spans := np.doc.QuerySelectorAll(".system-ui-status span")
	if len(spans) == 0 {
		return ""
	}
	return spans[0].InnerText()
}

func statusRight(np *notepad) string {
	el := np.doc.QuerySelector(".system-ui-status-right")
	if el == nil {
		return ""
	}
	return el.InnerText()
}

// openMenu 返回当前展开的 ui 菜单；没有展开的菜单时返回 nil。
func openMenu(np *notepad) engine.HTMLElement {
	for _, el := range np.doc.QuerySelectorAll(".system-ui-menu") {
		if strings.Contains(el.GetAttribute("class"), "is-open") {
			return el
		}
	}
	return nil
}

// clickMenu 走真实菜单链路：点菜单栏标签 → 浮层面板展开 → 点菜单项。
func clickMenu(t *testing.T, np *notepad, title, label string) {
	t.Helper()
	np.doc.Refresh()

	head := menuHead(t, np, title)
	r := head.GetBoundingClientRect()
	clickAt(t, np.doc, r.Left().Pixel()+2, r.Top().Pixel()+2)

	// 浮层里的菜单项：展开后要重排一次，项才有可点的盒子。
	np.doc.Refresh()
	pop := head.QuerySelector(".system-ui-menu-pop")
	if pop == nil {
		t.Fatalf("菜单 %q 没有浮层", title)
	}
	for _, item := range pop.Children() {
		if menuItemLabel(item) != label {
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
	r := np.editorEl().GetBoundingClientRect()
	top := np.theme.MenuHeight + np.theme.ToolHeight
	if got := r.Top().Pixel(); got != top {
		t.Fatalf("编辑区上沿 = %d, want %d", got, top)
	}
	if got := r.Right().Pixel() - r.Left().Pixel(); got != 900 {
		t.Fatalf("编辑区宽 = %d, want 900", got)
	}
	wantH := 620 - top - np.theme.StatusHeight
	if got := r.Bottom().Pixel() - r.Top().Pixel(); got != wantH {
		t.Fatalf("编辑区高 = %d, want %d", got, wantH)
	}
}

// buildPage 不依赖 HTML：空文档上也能建出标题、查找栏与编辑区。
func TestBuildPageCreatesAllControls(t *testing.T) {
	vp := engine.NewHeadlessViewport(900, 620)
	doc, err := engine.OpenDocument(vp, "")
	if err != nil {
		t.Fatal(err)
	}
	buildPage(doc, findBarActions{})
	if got := doc.Title(); got != appName {
		t.Fatalf("文档标题 = %q, want %q", got, appName)
	}
	for _, id := range []string{
		"findbar", "find-text", "btn-find-next",
		"replace-text", "btn-replace", "btn-replace-all",
		"btn-find-close", "editor",
	} {
		if doc.GetElementById(id) == nil {
			t.Fatalf("buildPage 后缺少 #%s", id)
		}
	}
	// 页面是 ui 控件拼的而不是手搭 DOM：类名齐了才有主题外观。
	for _, tc := range []struct{ id, class string }{
		{"findbar", "system-ui-bar"},
		{"find-text", "system-ui-input"},
		{"btn-find-next", "system-ui-btn"},
		{"editor", "system-ui-textarea"},
	} {
		el := doc.GetElementById(tc.id)
		if got := el.GetAttribute("class"); !strings.Contains(got, tc.class) {
			t.Fatalf("#%s 的 class = %q，期望含 %s", tc.id, got, tc.class)
		}
	}
	if got := doc.GetElementById("editor").TagName(); got != "textarea" {
		t.Fatalf("编辑区标签 = %q, want textarea", got)
	}
	// 编辑区是铺满内容区的纸面：平铺变体（无描边、无圆角）。
	if got := doc.GetElementById("editor").GetAttribute("class"); !strings.Contains(got, "is-plain") {
		t.Fatalf("编辑区 class = %q，期望含 is-plain", got)
	}
	if got := len(doc.GetElementById("findbar").Children()); got != 6 {
		t.Fatalf("查找栏子元素数 = %d, want 6", got)
	}
}

// 查找栏的显隐与定位分开管：收起时写 display:none（既看不见也点不到），
// 应用写的 top 不会被冲掉；展开时栏是主题的控件条，控件排成一行并垂直居中。
func TestFindBarVisibilityKeepsPosition(t *testing.T) {
	np := newTestNotepad(t)
	bar := np.doc.QuerySelector("#findbar")
	barTop := np.theme.MenuHeight + np.theme.ToolHeight

	style := bar.GetAttribute("style")
	if np.findBar.Visible() || !strings.Contains(style, "display:none") {
		t.Fatalf("默认应收起：visible=%v style=%q", np.findBar.Visible(), style)
	}
	// 收起的栏不能还能被命中：渲染层按 display:none 跳过，命中层靠零尺寸盒
	// 跳过，两边同口径（否则会点到看不见的按钮）。
	if got := engine.ElementAt(np.doc, 200, barTop+10); got == np.findIn.Element() {
		t.Fatal("收起的查找栏仍被命中，隐藏与渲染不同口径")
	}

	np.showFindBar()
	np.doc.Refresh()
	style = bar.GetAttribute("style")
	if strings.Contains(style, "display:none") {
		t.Fatalf("展开后样式 = %q，期望去掉 display:none", style)
	}
	if !strings.Contains(style, "position:absolute") {
		t.Fatalf("展开后样式 = %q，期望保留定位声明", style)
	}

	// 栏贴在工具栏下面，横贯整个视口，高度取主题的 BarHeight。
	r := bar.GetBoundingClientRect()
	if got := r.Top().Pixel(); got != barTop {
		t.Fatalf("查找栏上沿 = %d, want %d（菜单+工具栏）", got, barTop)
	}
	if got := r.Right().Pixel() - r.Left().Pixel(); got != 900 {
		t.Fatalf("查找栏宽 = %d, want 900（主题 width:100%%）", got)
	}
	if got := r.Bottom().Pixel() - r.Top().Pixel(); got != np.theme.BarHeight {
		t.Fatalf("查找栏高 = %d, want %d（主题 BarHeight）", got, np.theme.BarHeight)
	}
	// 条内第一个控件（查找框）能被命中，且在这条栏里垂直居中。
	if got := engine.ElementAt(np.doc, 20, barTop+np.theme.BarHeight/2); got != np.findIn.Element() {
		t.Fatal("展开后没有命中查找框，条内排布或命中测试不对")
	}
	c := np.findIn.Element().GetBoundingClientRect()
	mid := c.Top().Pixel() + (c.Bottom().Pixel()-c.Top().Pixel())/2
	if d := mid - (r.Top().Pixel() + np.theme.BarHeight/2); d > 1 || d < -1 {
		t.Fatalf("查找框中心偏离栏中心 %d px，期望垂直居中", d)
	}
}

// 状态栏各段各自更新：改文字只影响字符数，文件段（类型+未保存标记）
// 不受干扰，右侧提示不会被编辑事件覆盖。
func TestStatusSegmentsUpdateIndependently(t *testing.T) {
	np := newTestNotepad(t)
	if got, ok := np.workbench.StatusSegment("file"); !ok || !strings.Contains(got, "文本文件") {
		t.Fatalf("文件段 = %q, %v，期望显示文件类型", got, ok)
	}
	np.editor.SetValue("ab\ncd")
	if got, ok := np.workbench.StatusSegment("chars"); !ok || got != "5 个字符" {
		t.Fatalf("字符段 = %q, %v, want 5 个字符", got, ok)
	}
	if got, _ := np.workbench.StatusSegment("file"); !strings.HasSuffix(got, " *") {
		t.Fatalf("编辑后文件段 = %q, want 尾部带未保存标记", got)
	}
	if got := statusRight(np); got != "就绪" {
		t.Fatalf("右侧提示 = %q, want 就绪（不应被编辑事件覆盖）", got)
	}
}

// 选中字数跟着选区走：Ctrl+A 报全量，Shift+左逐字收缩，方向键折叠后清空。
// 选区变化能刷新状态栏，靠的是引擎 setSelection 也发文档变更通知。
func TestStatusSelectionCount(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetValue("你好\nworld") // 8 个 rune（含换行）
	np.selectAll()
	if got, ok := np.workbench.StatusSegment("sel"); !ok || got != "8 个字被选中" {
		t.Fatalf("全选后选中段 = %q, %v, want 8 个字被选中", got, ok)
	}
	engine.OnDocumentKeyDownMods(np.doc, engine.KeyLeft, engine.ModShift)
	if got, _ := np.workbench.StatusSegment("sel"); got != "7 个字被选中" {
		t.Fatalf("Shift+左后选中段 = %q, want 7 个字被选中", got)
	}
	engine.OnDocumentKeyDown(np.doc, engine.KeyRight) // 无 Shift：折叠选区
	if got, _ := np.workbench.StatusSegment("sel"); got != "" {
		t.Fatalf("折叠后选中段 = %q, want 空", got)
	}
}

// 窗口尺寸变化后编辑区跟着变：引擎在视口尺寸变化时补一次变更通知，
// 应用层据此重排（否则编辑区不会跟着窗口长宽变化）。
func TestEditorFollowsViewportResize(t *testing.T) {
	vp := engine.NewHeadlessViewport(900, 620)
	doc, err := engine.OpenDocument(vp, "")
	if err != nil {
		t.Fatal(err)
	}
	np := &notepad{vp: vp, doc: doc}
	np.bind()

	vp.W, vp.H = 640, 400
	doc.Refresh() // 后端在窗口缩放后强制重排，尺寸变化的通知随之发出
	doc.Refresh() // 通知里应用层改写了编辑区的内联样式，再排一次才生效
	// （真实窗口由后端按 ChangeCount 自动补这一遍）

	r := np.editorEl().GetBoundingClientRect()
	if got := r.Right().Pixel() - r.Left().Pixel(); got != 640 {
		t.Fatalf("缩放后编辑区宽 = %d, want 640", got)
	}
	wantH := 400 - (np.theme.MenuHeight + np.theme.ToolHeight) - np.theme.StatusHeight
	if got := r.Bottom().Pixel() - r.Top().Pixel(); got != wantH {
		t.Fatalf("缩放后编辑区高 = %d, want %d", got, wantH)
	}
}

// ui 菜单声明里的快捷键要真正接到 Workbench.HandleKey 上。
func TestShortcutUndoAndFind(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetValue("abc")
	if !np.handleKey('z', engine.ModCtrl) {
		t.Fatal("Ctrl+Z 没有被应用快捷键消费")
	}
	if got := np.editor.Value(); got != "" {
		t.Fatalf("Ctrl+Z 后编辑区 = %q，期望空", got)
	}

	before := np.editorEl().GetBoundingClientRect().Top().Pixel()
	if !np.handleKey('f', engine.ModCtrl) {
		t.Fatal("Ctrl+F 没有被应用快捷键消费")
	}
	np.doc.Refresh()
	if !np.findOpen {
		t.Fatal("Ctrl+F 没有展开查找替换栏")
	}
	r := np.editorEl().GetBoundingClientRect()
	wantTop := np.theme.MenuHeight + np.theme.ToolHeight + np.theme.BarHeight
	if got := r.Top().Pixel(); got != wantTop {
		t.Fatalf("查找栏展开后编辑区上沿 = %d，期望 %d（展开前 %d）", got, wantTop, before)
	}
	findBar := np.doc.QuerySelector("#findbar")
	// 展开时栏只写定位声明（可见性靠「没有 display:none」表达）：
	// BarView 把定位与显隐分开管，切显隐不会把 top 冲掉。
	style := findBar.GetAttribute("style")
	if strings.Contains(style, "display:none") {
		t.Fatalf("查找栏样式 = %q，期望可见", style)
	}
	if !strings.Contains(style, "position:absolute") {
		t.Fatalf("查找栏样式 = %q，期望绝对定位浮在内容上方", style)
	}
}

// 查找框聚焦时，编辑类快捷键不能被应用层抢走；保存类全局命令仍然有效。
func TestShortcutDoesNotStealFromFindInput(t *testing.T) {
	np := newTestNotepad(t)
	np.showFindBar()
	if focus := engine.FocusedElement(np.doc); focus != np.findIn.Element() {
		t.Fatal("查找栏没有拿到焦点")
	}
	for _, key := range []int{'a', 'c', 'v', 'x', 'z'} {
		if np.handleKey(key, engine.ModCtrl) {
			t.Fatalf("Ctrl+%c 在查找框里被应用快捷键抢走了", key)
		}
	}
	if !np.handleKey('s', engine.ModCtrl) {
		t.Fatal("Ctrl+S 应始终是全局快捷键")
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
	if got := np.editor.Value(); got != "hello\nworld" {
		t.Fatalf("打开后编辑区 = %q, want %q", got, "hello\nworld")
	}
	if np.path != path {
		t.Fatalf("当前文件 = %q, want %q", np.path, path)
	}
	if np.dirty {
		t.Fatal("刚打开的文件不应标记为已修改")
	}
	if got, _ := np.workbench.StatusSegment("file"); !strings.Contains(got, "文本文件 (*.txt)") || strings.HasSuffix(got, " *") {
		t.Fatalf("打开后文件段 = %q，期望含文件类型且不带未保存标记", got)
	}

	// 改内容 → 标记已修改 → 保存后落盘。
	np.editor.SetValue("changed")
	if !np.dirty {
		t.Fatal("编辑后未标记为已修改")
	}
	if got, _ := np.workbench.StatusSegment("file"); !strings.HasSuffix(got, " *") {
		t.Fatalf("编辑后文件段 = %q，期望尾部带未保存标记", got)
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
	if got, _ := np.workbench.StatusSegment("file"); strings.HasSuffix(got, " *") {
		t.Fatalf("保存后文件段 = %q，不应带未保存标记", got)
	}

	// 打开不存在的文件：状态栏提示，编辑区内容不变。
	np.editor.SetValue("keep")
	np.loadFile(filepath.Join(dir, "missing.txt"))
	if got := np.editor.Value(); got != "keep" {
		t.Fatalf("打开失败后编辑区 = %q, want keep", got)
	}
}

// 撤销：每次文本变化压一条快照，撤销弹栈并写回。
func TestUndoRestoresPreviousText(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetValue("abc")
	if len(np.undo) == 0 {
		t.Fatal("文本变化后没有压入撤销快照")
	}
	np.undoEdit()
	if got := np.editor.Value(); got != "" {
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
	np.editor.SetValue("abc")
	np.undoEdit()
	if got := np.editor.Value(); got != "" {
		t.Fatalf("撤销后编辑区 = %q, want 空", got)
	}
	np.redoEdit()
	if got := np.editor.Value(); got != "abc" {
		t.Fatalf("重做后编辑区 = %q, want %q", got, "abc")
	}

	// 撤销之后又有新改动：重做栈作废。
	np.editor.SetValue("xyz")
	np.undoEdit()
	np.editor.SetValue("new")
	np.redoEdit()
	if got := np.editor.Value(); got != "new" {
		t.Fatalf("新改动后重做 = %q, want new（重做栈应已作废）", got)
	}
}

// 菜单链路：点「文件」→ 浮层展开 → 点「新建」→ 编辑区清空。
func TestMenuActionClearsEditor(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetValue("old text")

	clickMenu(t, np, "文件", "新建")
	if got := np.editor.Value(); got != "" {
		t.Fatalf("「新建」后编辑区 = %q, want 空", got)
	}
}

// 菜单浮层：点标签打开、点菜单项执行后关闭、点别处也关闭。
func TestMenuPopupOpensAndCloses(t *testing.T) {
	np := newTestNotepad(t)

	// 点菜单栏标签：浮层展开。
	np.doc.Refresh()
	head := menuHead(t, np, "文件")
	hr := head.GetBoundingClientRect()
	clickAt(t, np.doc, hr.Left().Pixel()+2, hr.Top().Pixel()+2)
	np.doc.Refresh()
	if openMenu(np) != head {
		t.Fatal("点菜单标签后浮层没有打开")
	}
	pop := head.QuerySelector(".system-ui-menu-pop")
	if pop == nil || len(pop.Children()) == 0 {
		t.Fatal("菜单浮层没有生成菜单项")
	}

	// 点菜单项：动作执行、浮层收起。
	clickMenu(t, np, "编辑", "全选")
	if openMenu(np) != nil {
		t.Fatal("点菜单项后浮层没有关闭")
	}

	// 点浮层之外（编辑区）也应关闭菜单。
	np.doc.Refresh()
	head = menuHead(t, np, "文件")
	hr = head.GetBoundingClientRect()
	clickAt(t, np.doc, hr.Left().Pixel()+2, hr.Top().Pixel()+2)
	if openMenu(np) != head {
		t.Fatal("浮层没有再次打开")
	}
	r := np.editorEl().GetBoundingClientRect()
	clickAt(t, np.doc, r.Left().Pixel()+10, r.Top().Pixel()+10)
	if openMenu(np) != nil {
		t.Fatal("点浮层之外后菜单没有关闭")
	}
}

// 无头模式没有原生对话框：「打开」要在状态栏说明白，而不是静默无反应。
func TestHeadlessOpenExplainsNoDialog(t *testing.T) {
	np := newTestNotepad(t)
	clickMenu(t, np, "文件", "打开…")
	if msg := statusRight(np); !strings.Contains(msg, "原生窗口") {
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
	if got := np.editorEl().GetAttribute("style"); !strings.Contains(got, "font-size:15px") {
		t.Fatalf("编辑区内联样式 = %q, want 含 font-size:15px", got)
	}
	clickMenu(t, np, "格式", "默认字号")
	if np.font != baseFont {
		t.Fatalf("复位后字号 = %d, want %d", np.font, baseFont)
	}
}

// 行列 ↔ 字节下标要互相对得上：引擎的行列按 rune 计（一个汉字算一列），
// 查找用的却是字节下标，两边混用会让中文文本找错位置、状态栏报错行列。
func TestLineColOffsetRoundTrip(t *testing.T) {
	const text = "一二三四五\nghromex 你好世界 abc\n最后"
	for _, tc := range []struct {
		desc      string
		line, col int
		byteIdx   int
	}{
		{"首行行首", 1, 1, 0},
		{"首行第 4 列", 1, 4, len("一二三")},
		{"次行行首", 2, 1, len("一二三四五\n")},
		{"次行的「你」", 2, 9, len("一二三四五\nghromex ")},
		{"末行行首", 3, 1, len("一二三四五\nghromex 你好世界 abc\n")},
		{"文本末尾（末行第 3 列）", 3, 3, len(text)},
	} {
		if got := offsetOfLineCol(text, tc.line, tc.col); got != tc.byteIdx {
			t.Errorf("%s：offsetOfLineCol(%d,%d) = %d, want %d",
				tc.desc, tc.line, tc.col, got, tc.byteIdx)
		}
		if line, col := offsetToLineCol(text, tc.byteIdx); line != tc.line || col != tc.col {
			t.Errorf("%s：offsetToLineCol(%d) = %d,%d, want %d,%d",
				tc.desc, tc.byteIdx, line, col, tc.line, tc.col)
		}
	}
}

// 中文文本里查找替换：位置按字节报出、行列按 rune 计，两边都得对。
func TestFindAndReplaceInChineseText(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetValue("一二三四五\nghromex 你好世界 abc")

	np.findIn.SetValue("你好")
	np.findNext()
	if want := len("一二三四五\nghromex "); np.matchAt != want {
		t.Fatalf("匹配位置 = %d, want %d（字节下标）", np.matchAt, want)
	}
	if msg := statusRight(np); !strings.Contains(msg, "行 2，列 9") {
		t.Fatalf("状态栏提示 = %q, want 含「行 2，列 9」", msg)
	}

	np.replaceIn.SetValue("HI")
	np.replaceCurrent()
	if got, want := np.editor.Value(), "一二三四五\nghromex HI世界 abc"; got != want {
		t.Fatalf("替换中文后 = %q, want %q", got, want)
	}
}

// 查找/替换：查找报出位置，替换改文本，全部替换改所有匹配。
func TestFindAndReplace(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetValue("hello world hello")

	np.findIn.SetValue("hello")
	np.findNext()
	if np.matchAt != 0 {
		t.Fatalf("第一次查找位置 = %d, want 0", np.matchAt)
	}
	if msg := statusRight(np); !strings.Contains(msg, "第 1 处") {
		t.Errorf("状态栏提示 = %q, want 含「第 1 处」", msg)
	}

	np.replaceIn.SetValue("HI")
	np.replaceCurrent()
	if got := np.editor.Value(); got != "HI world hello" {
		t.Fatalf("替换一处后 = %q, want %q", got, "HI world hello")
	}

	np.replaceAll()
	if got := np.editor.Value(); got != "HI world HI" {
		t.Fatalf("全部替换后 = %q, want %q", got, "HI world HI")
	}
}

// 视图 → 显示状态栏：状态栏可隐藏，编辑区相应变高。
func TestToggleStatusBar(t *testing.T) {
	np := newTestNotepad(t)
	before := np.editorEl().GetBoundingClientRect().Bottom().Pixel() -
		np.editorEl().GetBoundingClientRect().Top().Pixel()

	clickMenu(t, np, "视图", "显示状态栏")
	np.doc.Refresh()
	r := np.editorEl().GetBoundingClientRect()
	got := r.Bottom().Pixel() - r.Top().Pixel()
	if got != before+np.theme.StatusHeight {
		t.Fatalf("隐藏状态栏后编辑区高 = %d, want %d", got, before+np.theme.StatusHeight)
	}
	status := np.doc.QuerySelector(".system-ui-status")
	if status == nil {
		t.Fatal("状态栏不存在")
	}
	if got := status.GetAttribute("style"); !strings.Contains(got, "display:none") {
		t.Fatalf("状态栏样式 = %q, want 含 display:none", got)
	}
}

// 菜单项执行后菜单收起：收起的菜单项不该再被点到。
//
// 引擎曾把 display:none 元素的旧盒子留着，命中测试照样命中它——于是在菜单
// 项原来的位置再点一次，会又执行一次动作（「时间/日期」连插两次日期）。
func TestClosedMenuItemNotClickable(t *testing.T) {
	np := newTestNotepad(t)
	np.editor.SetValue("x")
	np.doc.Refresh()

	// 打开「编辑」菜单，记下「时间/日期」项的位置，再点它。
	head := menuHead(t, np, "编辑")
	hr := head.GetBoundingClientRect()
	clickAt(t, np.doc, hr.Left().Pixel()+2, hr.Top().Pixel()+2)
	np.doc.Refresh()
	x, y := 0, 0
	pop := head.QuerySelector(".system-ui-menu-pop")
	for _, item := range pop.Children() {
		if menuItemLabel(item) == "时间/日期" {
			ir := item.GetBoundingClientRect()
			x = (ir.Left().Pixel() + ir.Right().Pixel()) / 2
			y = (ir.Top().Pixel() + ir.Bottom().Pixel()) / 2
		}
	}
	if x == 0 && y == 0 {
		t.Fatal("菜单里没有「时间/日期」项")
	}
	clickAt(t, np.doc, x, y)
	first := np.editor.Value()
	if first == "x" {
		t.Fatal("「时间/日期」没有插入日期")
	}

	// 菜单已收起：在菜单项原来的位置再点一次（现在落在编辑区上），
	// 不该再执行「时间/日期」。
	np.doc.Refresh()
	engine.OnDocumentClick(np.doc, x, y)
	if got := np.editor.Value(); got != first {
		t.Fatalf("菜单收起后再点原位置，编辑区 = %q, want %q（菜单项不该再被点到）", got, first)
	}
}

// 工具栏只用图标：每个按钮都是 is-icon 方形按钮，盒子里只有 .system-ui-icon，
// 不带文字（Label 只作兜底与读屏用途）。
func TestToolbarUsesIconsOnly(t *testing.T) {
	np := newTestNotepad(t)
	tools := np.doc.GetElementsByClassName("system-ui-tool")
	if len(tools) != 6 {
		t.Fatalf("工具栏按钮数 = %d, want 6（新建/打开/保存/另存为/撤销/重做）", len(tools))
	}
	for i, el := range tools {
		if class := el.GetAttribute("class"); !strings.Contains(class, "is-icon") {
			t.Fatalf("第 %d 个按钮类名 = %q, want 含 is-icon", i+1, class)
		}
		if el.QuerySelector(".system-ui-icon") == nil {
			t.Fatalf("第 %d 个按钮没有图标子元素", i+1)
		}
		if got := el.Text(); got != "" {
			t.Fatalf("图标按钮不该带文字 = %q", got)
		}
	}
}

// 菜单项带图标：展开「文件」后浮层应带 has-icon 类，每个菜单项都要渲染
// .system-ui-menu-icon（图标列没有空位）。
func TestMenuItemsHaveIcons(t *testing.T) {
	np := newTestNotepad(t)
	head := menuHead(t, np, "文件")
	hr := head.GetBoundingClientRect()
	clickAt(t, np.doc, hr.Left().Pixel()+2, hr.Top().Pixel()+2)
	np.doc.Refresh()

	pop := head.QuerySelector(".system-ui-menu-pop")
	if pop == nil {
		t.Fatal("菜单没有浮层")
	}
	if class := pop.GetAttribute("class"); !strings.Contains(class, "has-icon") {
		t.Fatalf("浮层类名 = %q, want 含 has-icon", class)
	}
	items := 0
	for _, item := range pop.Children() {
		if !strings.Contains(item.GetAttribute("class"), "system-ui-menu-item") {
			continue
		}
		items++
		if item.QuerySelector(".system-ui-menu-icon") == nil {
			t.Fatalf("菜单项 %q 没有图标", menuItemLabel(item))
		}
	}
	if items != 6 {
		t.Fatalf("「文件」菜单项数 = %d, want 6（不含分隔线）", items)
	}
}
