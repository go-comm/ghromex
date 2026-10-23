// 示例：仿 Windows 记事本。
//
// 界面：ui 生成的菜单栏（文件/编辑/格式/视图/帮助，菜单项带图标）、纯图标
// 工具栏与状态栏，加上铺满窗口的多行编辑区、查找替换栏。真实窗口下
// 「打开…」「另存为…」用 Windows 原生文件对话框（Shell 的 IFileDialog）；
// 无头模式没有原生对话框，会在状态栏提示。
//
//	go run ./examples/notepad
//	go run ./examples/notepad -file notes.txt      # 启动时打开文件
//	go run ./examples/notepad -exit-after 3s       # 调试/CI
//	go run ./examples/notepad -dump-svg out.svg    # 无头导出
//
// 页面完全由 page.go 用 Go 构造，没有 HTML 文件。
//
// 实现要点：
//   - 菜单、工具栏、快捷键统一由 ui.Spec 声明，不再手写浮层和按钮绑定；
//   - 编辑区几何（left/top/width/height）由程序按窗口尺寸计算：引擎的
//     CSS 没有 calc，"视口高度 - 固定栏高" 这类尺寸只能由应用层算；
//     固定栏高度直接取 ui.Theme，和 ui.Install 插入的占位块一致；
//   - 窗口缩放由引擎的「视口尺寸变化」变更通知驱动（见 engine 的
//     LayoutDocument），文档内容不变时 onChanged 不触发，收不到信号；
//   - ui 在命令执行后通过 Spec.Focus 把焦点还给编辑区；直接调用动作的
//     路径仍显式 focusEditor。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/runtime/clipboard"
	"github.com/go-comm/ghromex/runtime/dialog"
	"github.com/go-comm/ghromex/ui"
)

const (
	winW = 900
	winH = 620

	findH = 32 // 查找替换栏高（菜单/工具栏/状态栏高度由 ui.Theme 提供）

	baseFont = 14 // 编辑区默认字号
	minFont  = 8
	maxFont  = 32
	maxUndo  = 200 // 撤销栈深度
	appName  = "记事本"
	untitled = "未命名"
)

func main() {
	file := flag.String("file", "", "启动时打开的文本文件")
	exitAfter := flag.Duration("exit-after", 0, "指定时长后自动退出，如 3s（调试/CI 用）")
	dumpSVG := flag.String("dump-svg", "", "无头模式：完成级联与布局后把结果导出为 SVG 并退出")
	flag.Parse()

	theme := ui.Light()

	if *dumpSVG != "" {
		vp := engine.NewHeadlessViewport(winW, winH)
		doc, err := engine.OpenDocument(vp, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开文档失败:", err)
			os.Exit(1)
		}
		np := &notepad{vp: vp, doc: doc, theme: theme}
		np.bind()
		doc.Refresh()
		if err := os.WriteFile(*dumpSVG, []byte(doc.DumpSVG()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("SVG 已写入:", *dumpSVG)
		return
	}

	win, err := browser.New(winW, winH, appName+" — Ghromex")
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	defer win.Close()

	doc, err := win.OpenDocument("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开文档失败:", err)
		os.Exit(1)
	}

	np := &notepad{vp: win, win: win, doc: doc, theme: theme}
	np.bind()
	if *file != "" {
		np.loadFile(*file)
	}

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}

// viewport 是记事本用到的视口能力（尺寸来源）。真实窗口与无头视口都满足它，
// 于是「按窗口尺寸排版」这段逻辑在两种模式下完全一致。
type viewport interface {
	ViewportWidth() int
	ViewportHeight() int
}

// notepad 是记事本的全部状态。
type notepad struct {
	vp    viewport
	win   *browser.Window // 真实窗口（无头导出模式下为 nil）
	doc   engine.HTMLDocument
	shell *ui.Shell
	theme ui.Theme

	editor    engine.HTMLElement // 编辑区（textarea）
	findBar   engine.HTMLElement // 查找替换栏
	findIn    engine.HTMLElement // 查找内容输入框
	replaceIn engine.HTMLElement // 替换内容输入框

	path         string // 当前文件路径（空 = 未命名）
	cur          string // 编辑区最近一次的文本（撤销快照与状态栏都用它）
	undo         []string
	redo         []string
	applying     bool   // 正在执行撤销/重做/替换：此时不再压入快照
	dirty        bool   // 有未保存的修改
	font         int    // 编辑区字号（格式 → 放大/缩小）
	mono         bool   // 编辑区用等宽字体
	showStatus   bool   // 显示状态栏
	findOpen     bool   // 查找替换栏是展开的
	matchAt      int    // 上一次找到的匹配位置（-1 = 没有）
	lastW, lastH int    // 上次排版时的视口尺寸
	lastTitle    string // 上次写入窗口的标题（避免每次按键都调平台接口）
}

// bind 在空文档上搭页面、取出元素、安装 ui 外壳并接上事件。
func (n *notepad) bind() {
	if n.doc.QuerySelector("#editor") == nil {
		buildPage(n.doc)
	}
	n.editor = n.doc.QuerySelector("#editor")
	n.findBar = n.doc.QuerySelector("#findbar")
	n.findIn = n.doc.QuerySelector("#find-text")
	n.replaceIn = n.doc.QuerySelector("#replace-text")
	if n.editor == nil {
		return
	}
	n.cur = n.editorValue()
	n.font = baseFont
	n.showStatus = true
	n.matchAt = -1
	if n.theme.MenuHeight == 0 {
		n.theme = ui.Light()
	}

	n.shell = ui.Install(n.doc, n.buildSpec())
	if n.win != nil {
		n.win.SetKeyHandler(n.handleKey)
	}
	n.bindFindBar()
	n.doc.SetOnChanged(n.onChanged)
	n.syncLayout()
	n.updateStatus()
	n.message("就绪")
}

// bindFindBar 把查找替换栏的按钮接到具体动作上。
func (n *notepad) bindFindBar() {
	buttons := []struct {
		id  string
		run func()
	}{
		{"#btn-find-next", n.findNext},
		{"#btn-replace", n.replaceCurrent},
		{"#btn-replace-all", n.replaceAll},
		{"#btn-find-close", n.hideFindBar},
	}
	for _, b := range buttons {
		el := n.doc.QuerySelector(b.id)
		if el == nil {
			continue
		}
		run := b.run
		el.OnClick(func(*engine.MouseEvent) { run() })
	}
}

// handleKey 把应用快捷键接到 ui.Shell。
//
// 查找/替换输入框聚焦时，Ctrl+C/V/X/A/Z 这类编辑快捷键应该留给输入框
// 自己，只放行保存/打开/查找/退出这类全局命令；否则会去操作背后的编辑区。
func (n *notepad) handleKey(key int, mods uint16) bool {
	if focus := engine.FocusedElement(n.doc); focus != nil && focus != n.editor && !isGlobalShortcut(key, mods) {
		return false
	}
	return n.shell != nil && n.shell.HandleKey(key, mods)
}

// isGlobalShortcut 判断快捷键在输入框聚焦时是否仍然全局有效。
func isGlobalShortcut(key int, mods uint16) bool {
	if key >= 'A' && key <= 'Z' {
		key += 'a' - 'A'
	}
	ctrl := mods&engine.ModCtrl != 0
	shift := mods&engine.ModShift != 0
	alt := mods&engine.ModAlt != 0
	if alt && !ctrl && !shift {
		return key == 'q'
	}
	if ctrl && !alt {
		if key == 's' {
			return true // Ctrl+S / Ctrl+Shift+S
		}
		return !shift && (key == 'n' || key == 'o' || key == 'f')
	}
	return false
}

// onChanged 是文档变更回调：文本变化进撤销栈、视口变化重排编辑区。
//
// 引擎在「视口尺寸变化」时也会补一次变更通知（窗口缩放不改文档内容，
// 只靠 onChanged 是收不到信号的），因此这里统一处理两种来源。
func (n *notepad) onChanged() {
	n.syncLayout()
	if v := n.editorValue(); v != n.cur {
		if !n.applying {
			n.undo = append(n.undo, n.cur)
			if len(n.undo) > maxUndo {
				n.undo = n.undo[1:]
			}
			n.redo = nil // 有了新改动，重做栈作废
		}
		n.cur = v
		n.dirty = true
	}
	if n.shell != nil {
		// 撤销/重做栈刚变过，工具栏按钮的可用态要跟着更新。
		n.shell.Refresh()
	}
	n.updateStatus()
}

// syncLayout 按视口尺寸重排编辑区。
//
// 引擎的 CSS 没有 calc，"视口高度 - 菜单栏 - 工具栏 - 状态栏" 这类尺寸
// 只能由应用层算；引擎也没有"宽度自动减固定量"的盒模型，故整段几何都
// 在这里写进内联样式。视口尺寸变化由引擎的变更通知驱动（见 onChanged）。
func (n *notepad) syncLayout() {
	if n.editor == nil {
		return
	}
	w, h := n.vp.ViewportWidth(), n.vp.ViewportHeight()
	if w == n.lastW && h == n.lastH {
		return
	}
	n.lastW, n.lastH = w, h

	// 查找替换栏展开时把编辑区往下挤一行；状态栏隐藏时编辑区多占一行。
	// 菜单/工具栏/状态栏的高度来自 ui 主题，和 ui.Install 插入的占位块一致。
	barTop := n.theme.MenuHeight + n.theme.ToolHeight
	editorTop := barTop
	if n.findOpen {
		editorTop += findH
	}
	barH := n.theme.StatusHeight
	if !n.showStatus {
		barH = 0
	}
	if n.findBar != nil {
		display := "none"
		if n.findOpen {
			display = "block"
		}
		n.findBar.SetAttribute("style", fmt.Sprintf("top:%dpx;display:%s", barTop, display))
	}
	eh := h - editorTop - barH
	if eh < 40 {
		eh = 40
	}
	family := "Consolas, 'Courier New', monospace"
	if !n.mono {
		family = "'Microsoft YaHei', 'Segoe UI', system-ui, sans-serif"
	}
	n.editor.SetAttribute("style", fmt.Sprintf(
		"position:absolute;left:0;top:%dpx;width:%dpx;height:%dpx;font-size:%dpx;font-family:%s;",
		editorTop, w, eh, n.font, family))
}

// editorValue 返回编辑区当前文本。
func (n *notepad) editorValue() string {
	if n.editor == nil {
		return ""
	}
	return n.editor.GetAttribute("value")
}

// updateStatus 刷新状态栏：文件名（带未保存标记）、光标位置、字符数。
// 三段独立更新，右侧消息不随每次光标移动被覆盖。
func (n *notepad) updateStatus() {
	name := n.path
	if name == "" {
		name = untitled
	} else {
		name = shortName(name)
	}
	if n.dirty {
		name += " *"
	}
	pos := ""
	if line, col, ok := engine.CaretPosition(n.editor); ok {
		pos = fmt.Sprintf("行 %d，列 %d", line, col)
	}
	if n.shell != nil {
		n.shell.SetStatusSegment("file", name)
		n.shell.SetStatusSegment("pos", pos)
		n.shell.SetStatusSegment("chars", strconv.Itoa(len([]rune(n.cur)))+" 个字符")
	}
	n.updateTitle()
}

// updateTitle 把文件名写进窗口标题（记事本的习惯）。
func (n *notepad) updateTitle() {
	if n.win == nil {
		return
	}
	name := n.path
	if name == "" {
		name = untitled
	}
	if n.dirty {
		name += " *"
	}
	title := name + " - " + appName
	if title == n.lastTitle {
		return // 每次按键都会走到这里，标题没变就别去调平台接口
	}
	n.lastTitle = title
	n.win.SetTitle(title)
}

// shortName 取路径的文件名部分（标题栏只显示它）。
func shortName(path string) string {
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// message 在状态栏右侧提示一条消息。
func (n *notepad) message(s string) {
	if n.shell != nil {
		n.shell.SetStatusRight(s)
	}
}

// focusEditor 把焦点交还编辑区。
//
// ui 在菜单/工具栏命令执行后也会通过 Spec.Focus 还焦点；这里仍保留一次
// 显式聚焦，供直接调用动作的路径使用（例如测试和快捷键）。
// 光标与选区随焦点一起回来，粘贴/剪切才有落点。
func (n *notepad) focusEditor() {
	if n.editor == nil {
		return
	}
	engine.SetFocusedElement(n.doc, n.editor)
}

// newFile 清空编辑区。
func (n *notepad) newFile() {
	engine.SetValue(n.editor, "")
	n.undo = nil
	n.redo = nil
	n.path = ""
	n.dirty = false
	if n.shell != nil {
		n.shell.Refresh()
	}
	n.updateStatus()
	n.message("新建文档")
}

// openFile 打开文件：弹原生「打开文件」对话框。
func (n *notepad) openFile() {
	paths := n.pickFiles("打开", false)
	if len(paths) == 0 {
		return // 用户取消
	}
	n.loadFile(paths[0])
}

// pickFiles 弹原生「打开文件」对话框，返回用户选中的文件。
//
// 对话框是模态的：调用期间会阻塞当前线程（父窗口的 SDL 事件循环随之
// 暂停），这与原生应用里模态对话框的表现一致。
func (n *notepad) pickFiles(title string, multi bool) []string {
	parent, err := n.dialogParent()
	if err != nil {
		n.message(err.Error())
		return nil
	}
	opts := dialog.Options{
		Title:      title,
		DefaultDir: n.currentDir(),
		Filters:    fileFilters(),
	}
	var paths []string
	if multi {
		paths, err = dialog.OpenMultipleFilesDialog(parent, opts)
	} else {
		paths, err = dialog.OpenFileDialog(parent, opts)
	}
	if err != nil {
		n.message("打开对话框失败：" + err.Error())
		return nil
	}
	return paths
}

// dialogParent 取原生对话框的父窗口句柄；无头模式没有窗口。
func (n *notepad) dialogParent() (dialog.Parent, error) {
	if n.win == nil {
		return 0, fmt.Errorf("无头模式没有原生窗口，无法弹原生对话框")
	}
	hwnd, err := n.win.NativeHandle()
	if err != nil {
		return 0, err
	}
	return dialog.Parent(hwnd), nil
}

// openDir 选一个文件夹（原生「选择文件夹」对话框），结果在状态栏提示。
func (n *notepad) openDir() {
	parent, err := n.dialogParent()
	if err != nil {
		n.message("无头模式没有原生对话框，无法选择文件夹")
		return
	}
	dir, err := dialog.OpenDirectoryDialog(parent, dialog.Options{
		Title:      "选择文件夹",
		DefaultDir: n.currentDir(),
	})
	if err != nil {
		n.message("选择文件夹失败：" + err.Error())
		return
	}
	if dir == "" {
		return // 用户取消
	}
	n.message("已选择 " + dir)
}

// fileFilters 是记事本用到的文件类型过滤器。
func fileFilters() []dialog.FileFilter {
	return []dialog.FileFilter{
		{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
		{Name: "所有文件 (*.*)", Pattern: "*.*"},
	}
}

// currentDir 返回对话框的初始目录：当前文件所在目录（没有当前文件时
// 留空，让系统用上次的目录）。
func (n *notepad) currentDir() string {
	if n.path == "" {
		return ""
	}
	return filepath.Dir(n.path)
}

// defaultSaveName 返回「另存为」的初始文件名。
func (n *notepad) defaultSaveName() string {
	if n.path == "" {
		return "新建文档.txt"
	}
	return filepath.Base(n.path)
}

// loadFile 读取文件并载入编辑区。
func (n *notepad) loadFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		n.message("打开失败：" + err.Error())
		return
	}
	engine.SetValue(n.editor, string(data))
	n.undo = nil
	n.redo = nil
	n.path = path
	n.dirty = false
	if n.shell != nil {
		n.shell.Refresh()
	}
	n.updateStatus()
	n.message("已打开 " + path)
}

// saveFile 保存到当前文件；没有当前文件时按「另存为」处理。
func (n *notepad) saveFile() {
	if n.path == "" {
		n.saveAs()
		return
	}
	n.writeFile(n.path)
}

// saveAs 另存为：弹原生「另存为」对话框。
func (n *notepad) saveAs() {
	parent, err := n.dialogParent()
	if err != nil {
		n.message("另存为失败：" + err.Error())
		return
	}
	path, err := dialog.SaveFileDialog(parent, dialog.Options{
		Title:       "另存为",
		DefaultDir:  n.currentDir(),
		DefaultName: n.defaultSaveName(),
		Filters:     fileFilters(),
	})
	if err != nil {
		n.message("另存为失败：" + err.Error())
		return
	}
	if path == "" {
		return // 用户取消
	}
	n.writeFile(path)
}

// writeFile 把编辑区内容写到 path，并把它设为当前文件。
func (n *notepad) writeFile(path string) {
	if err := os.WriteFile(path, []byte(n.cur), 0o644); err != nil {
		n.message("保存失败：" + err.Error())
		return
	}
	n.path = path
	n.dirty = false
	n.updateStatus()
	n.message("已保存 " + path)
}

// undoEdit 撤销上一步编辑（按文本快照回退）。
//
// 引擎没有撤销栈：示例程序在文档变更回调里比对文本，变了就把旧值压栈
// （一次改动一条快照），撤销即弹栈并写回。撤销动作本身不再压栈。
func (n *notepad) undoEdit() {
	if len(n.undo) == 0 {
		n.message("没有可撤销的操作")
		return
	}
	prev := n.undo[len(n.undo)-1]
	n.undo = n.undo[:len(n.undo)-1]
	n.redo = append(n.redo, n.cur) // 当前内容进重做栈，重做时再弹回来
	n.applying = true
	engine.SetValue(n.editor, prev)
	n.applying = false
	if n.shell != nil {
		n.shell.Refresh()
	}
	n.message("已撤销")
}

// copySelection 把选区复制到系统剪贴板。
func (n *notepad) copySelection() {
	n.focusEditor()
	txt, ok := engine.SelectedText(n.doc)
	if !ok {
		n.message("没有选中的内容")
		return
	}
	clipboard.SetText(txt)
	n.message("已复制 " + strconv.Itoa(len([]rune(txt))) + " 个字符")
}

// cut 剪切：复制选区后删除它。
func (n *notepad) cut() {
	n.focusEditor()
	txt, ok := engine.SelectedText(n.doc)
	if !ok {
		n.message("没有选中的内容")
		return
	}
	clipboard.SetText(txt)
	engine.OnDocumentKeyDown(n.doc, engine.KeyDelete)
	n.message("已剪切 " + strconv.Itoa(len([]rune(txt))) + " 个字符")
}

// paste 从系统剪贴板粘贴到光标处（有选区则替换选区）。
func (n *notepad) paste() {
	n.focusEditor()
	txt := clipboard.Text()
	if txt == "" {
		n.message("剪贴板是空的")
		return
	}
	engine.OnDocumentPaste(n.doc, txt)
}

// selectAll 全选编辑区内容。
func (n *notepad) selectAll() {
	n.focusEditor()
	engine.OnDocumentKeyDownMods(n.doc, 'a', engine.ModCtrl)
}

// insertTime 在光标处插入当前时间/日期。
func (n *notepad) insertTime() {
	n.focusEditor()
	engine.OnDocumentTextInput(n.doc, time.Now().Format("2006-01-02 15:04:05"))
}

// zoom 调整编辑区字号：delta>0 放大、<0 缩小、0 恢复默认。
func (n *notepad) zoom(delta int) {
	switch {
	case delta == 0:
		n.font = baseFont
	default:
		n.font += delta
		if n.font < minFont {
			n.font = minFont
		}
		if n.font > maxFont {
			n.font = maxFont
		}
	}
	n.lastW = 0 // 强制重排：字号变了要重写内联样式
	n.syncLayout()
	n.message(fmt.Sprintf("字号 %dpx", n.font))
}
