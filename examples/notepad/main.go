// 示例：仿 Windows 记事本。
//
// 界面：菜单栏（文件/编辑/视图/帮助）、路径栏、铺满窗口的多行编辑区、
// 状态栏（文件名、光标位置、字符数、提示消息）。真实窗口下「打开…」
// 「另存为…」用 Windows 原生文件对话框（Shell 的 IFileDialog），
// 路径栏仍可直接输入路径（无头模式没有原生对话框，只能用它）。
//
//	go run ./examples/notepad
//	go run ./examples/notepad -file notes.txt      # 启动时打开文件
//	go run ./examples/notepad -exit-after 3s       # 调试/CI
//	go run ./examples/notepad -dump-svg out.svg    # 无头导出
//
// 页面 index.html 与本文件同目录，用 go:embed 打进二进制。
//
// 实现要点：
//   - 下拉菜单借 <select> 的选项浮层实现：引擎没有通用弹层，select 的
//     展开态恰好是「点标题弹出选项列表」，选中触发 change，示例程序
//     据此执行动作并把 select 复位成菜单标题；
//   - 编辑区几何（left/top/width/height）由程序按窗口尺寸计算：引擎的
//     CSS 没有 calc，"视口高度 - 固定栏高" 这类尺寸只能由应用层算；
//     窗口缩放由引擎的「视口尺寸变化」变更通知驱动（见 engine 的
//     LayoutDocument），文档内容不变时 onChanged 不触发，收不到信号；
//   - 菜单点击会先让编辑区失焦，但引擎保留编辑状态（光标与选区不失，
//     只是不画高亮），所以动作前把焦点交还编辑区即可继续操作。
package main

import (
	_ "embed"
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
)

//go:embed index.html
var page []byte

const (
	winW = 900
	winH = 620

	menuH   = 30 // 菜单栏高
	toolH   = 36 // 工具栏高
	findH   = 32 // 查找替换栏高
	statusH = 24 // 状态栏高

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

	if *dumpSVG != "" {
		vp := engine.NewHeadlessViewport(winW, winH)
		doc, err := engine.OpenDocument(vp, string(page))
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开文档失败:", err)
			os.Exit(1)
		}
		np := &notepad{vp: vp, doc: doc}
		np.bind()
		np.syncLayout()
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

	doc, err := win.OpenDocument(string(page))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开文档失败:", err)
		os.Exit(1)
	}

	np := &notepad{vp: win, win: win, doc: doc}
	np.bind()
	doc.SetOnChanged(np.onChanged)
	np.syncLayout()
	np.updateStatus()
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
	vp  viewport
	win *browser.Window // 真实窗口（无头导出模式下为 nil）
	doc engine.HTMLDocument

	editor                        engine.HTMLElement // 编辑区（textarea）
	menuBar                       engine.HTMLElement // 菜单栏
	menuPop                       engine.HTMLElement // 菜单浮层
	menuLabels                    []engine.HTMLElement
	findBar                       engine.HTMLElement // 查找替换栏
	findIn                        engine.HTMLElement // 查找内容输入框
	replaceIn                     engine.HTMLElement // 替换内容输入框
	stBar                         engine.HTMLElement // 状态栏
	stName, stPos, stCount, stMsg engine.HTMLElement

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
	menuOpen     bool   // 菜单浮层是打开的
	lastW, lastH int    // 上次排版时的视口尺寸
	lastTitle    string // 上次写入窗口的标题（避免每次按键都调平台接口）
}

// bind 取出页面元素并接上事件。
func (n *notepad) bind() {
	n.editor = n.doc.QuerySelector("#editor")
	n.menuBar = n.doc.QuerySelector("#menubar")
	n.menuPop = n.doc.QuerySelector("#menupop")
	n.findBar = n.doc.QuerySelector("#findbar")
	n.findIn = n.doc.QuerySelector("#find-text")
	n.replaceIn = n.doc.QuerySelector("#replace-text")
	n.stBar = n.doc.QuerySelector("#statusbar")
	n.stName = n.doc.QuerySelector("#st-name")
	n.stPos = n.doc.QuerySelector("#st-pos")
	n.stCount = n.doc.QuerySelector("#st-count")
	n.stMsg = n.doc.QuerySelector("#st-msg")
	if n.editor == nil {
		return
	}
	n.cur = n.editorValue()
	n.font = baseFont
	n.showStatus = true
	n.matchAt = -1

	n.buildMenuBar()
	n.bindToolbar()
	n.bindFindBar()

	// 点菜单栏以外的地方关闭菜单浮层。点在菜单标签上时由标签自己处理
	// （标签的处理器先跑，随后冒泡到 body——所以这里按坐标让开）。
	n.doc.Body().OnClick(func(ev *engine.MouseEvent) {
		if ev.Y < menuH {
			return
		}
		n.closeMenu()
	})
}

// bindToolbar 把工具栏按钮接到动作分发上。
func (n *notepad) bindToolbar() {
	buttons := []struct {
		id  string
		act string
	}{
		{"#btn-new", "new"},
		{"#btn-open", "open"},
		{"#btn-save", "save"},
		{"#btn-saveas", "saveas"},
		{"#btn-undo", "undo"},
		{"#btn-redo", "redo"},
	}
	for _, b := range buttons {
		el := n.doc.QuerySelector(b.id)
		if el == nil {
			continue
		}
		act := b.act
		el.OnClick(func(*engine.MouseEvent) { n.action(act) })
	}
}

// bindFindBar 把查找替换栏的按钮接到动作分发上。
func (n *notepad) bindFindBar() {
	buttons := []struct {
		id  string
		act string
	}{
		{"#btn-find-next", "findnext"},
		{"#btn-replace", "replace"},
		{"#btn-replace-all", "replaceall"},
		{"#btn-find-close", "findclose"},
	}
	for _, b := range buttons {
		el := n.doc.QuerySelector(b.id)
		if el == nil {
			continue
		}
		act := b.act
		el.OnClick(func(*engine.MouseEvent) { n.action(act) })
	}
}

// action 分发菜单动作。
func (n *notepad) action(act string) {
	switch act {
	case "new":
		n.newFile()
	case "open":
		n.openFile()
	case "opendir":
		n.openDir()
	case "save":
		n.saveFile()
	case "saveas":
		n.saveAs()
	case "exit":
		if n.win != nil {
			n.win.RequestClose()
		}
	case "undo":
		n.undoEdit()
	case "redo":
		n.redoEdit()
	case "cut":
		n.cut()
	case "copy":
		n.copySelection()
	case "paste":
		n.paste()
	case "delete":
		n.deleteSelection()
	case "selectall":
		n.selectAll()
	case "find":
		n.showFindBar()
	case "findnext":
		n.findNext()
	case "replace":
		n.replaceCurrent()
	case "replaceall":
		n.replaceAll()
	case "findclose":
		n.hideFindBar()
	case "time":
		n.insertTime()
	case "zoomin":
		n.zoom(1)
	case "zoomout":
		n.zoom(-1)
	case "zoomreset":
		n.zoom(0)
	case "fontmono":
		n.setMono(true)
	case "fontsans":
		n.setMono(false)
	case "statusbar":
		n.toggleStatus()
	case "about":
		n.message(appName + " — 基于 Ghromex 引擎的示例（纯 Go，无 CGO）")
	}
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
	editorTop := menuH + toolH
	if n.findOpen {
		editorTop += findH
	}
	barH := statusH
	if !n.showStatus {
		barH = 0
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
	setText(n.stName, name)

	if line, col, ok := engine.CaretPosition(n.editor); ok {
		setText(n.stPos, fmt.Sprintf("行 %d，列 %d", line, col))
	}
	setText(n.stCount, strconv.Itoa(len([]rune(n.cur)))+" 个字符")
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

// setText 写文本，内容不变则跳过：SetText 会触发文档变更通知，
// 而变更通知又会回调 updateStatus——不比较内容就会无限递归。
func setText(el engine.HTMLElement, s string) {
	if el == nil {
		return
	}
	if el.InnerText() == s {
		return
	}
	el.SetText(s)
}

// message 在状态栏提示一条消息。
func (n *notepad) message(s string) { setText(n.stMsg, s) }

// focusEditor 把焦点交还编辑区。
//
// 点菜单会先让编辑区失焦（引擎的失焦只停画、不丢编辑状态），所以动作
// 之前把焦点要回来：光标与选区随焦点一起回来，粘贴/剪切才有落点。
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

// openDir 选一个文件夹（原生「选择文件夹」对话框），把结果填进路径栏。
func (n *notepad) openDir() {
	parent, err := n.dialogParent()
	if err != nil {
		n.message("无头模式没有原生对话框，请直接在路径栏填写目录")
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
