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
)

//go:embed index.html
var page []byte

const (
	winW = 900
	winH = 620

	menuH   = 28 // 菜单栏高
	pathH   = 34 // 路径栏高
	statusH = 24 // 状态栏高
	editorY = menuH + pathH

	pathLeft = 132 // 路径输入框左缘（前面是「打开」「保存」两个按钮）
	baseFont = 14  // 编辑区默认字号
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
	pathIn                        engine.HTMLElement // 路径输入框
	stName, stPos, stCount, stMsg engine.HTMLElement

	path         string // 当前文件路径（空 = 未命名）
	cur          string // 编辑区最近一次的文本（撤销快照与状态栏都用它）
	undo         []string
	undoing      bool   // 正在执行撤销：此时不再压入快照
	dirty        bool   // 有未保存的修改
	font         int    // 编辑区字号（视图 → 放大/缩小）
	lastW, lastH int    // 上次排版时的视口尺寸
	lastTitle    string // 上次写入窗口的标题（避免每次按键都调平台接口）
}

// bind 取出页面元素并接上事件。
func (n *notepad) bind() {
	n.editor = n.doc.QuerySelector("#editor")
	n.pathIn = n.doc.QuerySelector("#path")
	n.stName = n.doc.QuerySelector("#st-name")
	n.stPos = n.doc.QuerySelector("#st-pos")
	n.stCount = n.doc.QuerySelector("#st-count")
	n.stMsg = n.doc.QuerySelector("#st-msg")
	if n.editor == nil {
		return
	}
	n.cur = n.editorValue()
	n.font = baseFont

	// 菜单：change 事件带的是被选中的 option 值（"" 是菜单标题本身）。
	for _, id := range []string{"#m-file", "#m-edit", "#m-view", "#m-help"} {
		n.bindMenu(id)
	}
	if b := n.doc.QuerySelector("#btn-open"); b != nil {
		b.OnClick(func(*engine.MouseEvent) { n.action("open") })
	}
	if b := n.doc.QuerySelector("#btn-save"); b != nil {
		b.OnClick(func(*engine.MouseEvent) { n.action("save") })
	}
}

// bindMenu 把一个 <select> 菜单接到动作分发上。
//
// 引擎的 <select> 展开态就是下拉菜单（选项浮层画在控件下方），选中某项
// 触发 change；动作执行完把 select 复位成菜单标题（value 为 "" 的那一项），
// 下次点击仍是标题态。
func (n *notepad) bindMenu(id string) {
	sel := n.doc.QuerySelector(id)
	if sel == nil {
		return
	}
	sel.OnChange(func(*engine.MouseEvent) {
		act := engine.SelectValue(sel)
		if act == "" {
			return // 复位动作（不是用户选择）
		}
		engine.SetSelectValue(sel, "")
		n.action(act)
	})
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
	case "cut":
		n.cut()
	case "copy":
		n.copySelection()
	case "paste":
		n.paste()
	case "selectall":
		n.selectAll()
	case "time":
		n.insertTime()
	case "zoomin":
		n.zoom(1)
	case "zoomout":
		n.zoom(-1)
	case "zoomreset":
		n.zoom(0)
	case "about":
		n.message(appName + " — 基于 Ghromex 引擎的记事本示例")
	}
}

// onChanged 是文档变更回调：文本变化进撤销栈、视口变化重排编辑区。
//
// 引擎在「视口尺寸变化」时也会补一次变更通知（窗口缩放不改文档内容，
// 只靠 onChanged 是收不到信号的），因此这里统一处理两种来源。
func (n *notepad) onChanged() {
	n.syncLayout()
	if v := n.editorValue(); v != n.cur {
		if !n.undoing {
			n.undo = append(n.undo, n.cur)
			if len(n.undo) > maxUndo {
				n.undo = n.undo[1:]
			}
		}
		n.cur = v
		n.dirty = true
	}
	n.updateStatus()
}

// syncLayout 按视口尺寸重排编辑区与路径输入框。
//
// 引擎的 CSS 没有 calc，"视口高度 - 菜单栏 - 路径栏 - 状态栏" 这类尺寸
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

	eh := h - editorY - statusH
	if eh < 40 {
		eh = 40
	}
	n.editor.SetAttribute("style", fmt.Sprintf(
		"position:absolute;left:0;top:%dpx;width:%dpx;height:%dpx;font-size:%dpx;",
		editorY, w, eh, n.font))

	if n.pathIn != nil {
		iw := w - pathLeft - 8
		if iw < 120 {
			iw = 120
		}
		n.pathIn.SetAttribute("style", fmt.Sprintf(
			"position:absolute;left:%dpx;top:6px;width:%dpx;height:22px;", pathLeft, iw))
	}
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
	} else {
		name = shortName(name)
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
	n.path = ""
	n.dirty = false
	n.setPath("")
	n.message("新建文档")
}

// setPath 写路径输入框（内容不变则跳过，避免多余的变更通知）。
func (n *notepad) setPath(p string) {
	if n.pathIn == nil {
		return
	}
	if n.pathIn.GetAttribute("value") == p {
		return
	}
	n.pathIn.SetAttribute("value", p)
}

// openFile 打开文件：真实窗口下弹原生「打开文件」对话框，无头模式
// （无窗口）退回到路径栏。
func (n *notepad) openFile() {
	if n.win != nil {
		paths := n.pickFiles("打开", false)
		if len(paths) == 0 {
			return // 用户取消
		}
		n.loadFile(paths[0])
		return
	}
	p := strings.TrimSpace(n.pathIn.GetAttribute("value"))
	if p == "" {
		n.message("请先在路径栏填写文件路径")
		return
	}
	n.loadFile(p)
}

// pickFiles 弹原生「打开文件」对话框，返回用户选中的文件。
//
// 对话框是模态的：调用期间会阻塞当前线程（父窗口的 SDL 事件循环随之
// 暂停），这与原生应用里模态对话框的表现一致。
func (n *notepad) pickFiles(title string, multi bool) []string {
	if n.win == nil {
		return nil
	}
	paths, err := n.win.OpenFileDialog(browser.DialogOptions{
		Title:       title,
		DefaultDir:  n.currentDir(),
		Filters:     fileFilters(),
		MultiSelect: multi,
	})
	if err != nil {
		n.message("打开对话框失败：" + err.Error())
		return nil
	}
	return paths
}

// openDir 选一个文件夹（原生「选择文件夹」对话框），把结果填进路径栏。
func (n *notepad) openDir() {
	if n.win == nil {
		n.message("无头模式没有原生对话框，请直接在路径栏填写目录")
		return
	}
	dir, err := n.win.OpenDirectoryDialog(browser.DialogOptions{
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
	n.setPath(dir)
	n.message("已选择 " + dir)
}

// fileFilters 是记事本用到的文件类型过滤器。
func fileFilters() []browser.FileFilter {
	return []browser.FileFilter{
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
	n.path = path
	n.dirty = false
	n.setPath(path)
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

// saveAs 另存为：真实窗口下弹原生「另存为」对话框，无头模式退回到
// 路径栏。
func (n *notepad) saveAs() {
	if n.win != nil {
		path, err := n.win.SaveFileDialog(browser.DialogOptions{
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
		return
	}
	p := strings.TrimSpace(n.pathIn.GetAttribute("value"))
	if p == "" {
		n.message("请先在路径栏填写文件路径")
		return
	}
	n.writeFile(p)
}

// writeFile 把编辑区内容写到 path，并把它设为当前文件。
func (n *notepad) writeFile(path string) {
	if err := os.WriteFile(path, []byte(n.cur), 0o644); err != nil {
		n.message("保存失败：" + err.Error())
		return
	}
	n.path = path
	n.dirty = false
	n.setPath(path)
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
	n.undoing = true
	engine.SetValue(n.editor, prev)
	n.undoing = false
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
	browser.SetClipboardText(txt)
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
	browser.SetClipboardText(txt)
	engine.OnDocumentKeyDown(n.doc, engine.KeyDelete)
	n.message("已剪切 " + strconv.Itoa(len([]rune(txt))) + " 个字符")
}

// paste 从系统剪贴板粘贴到光标处（有选区则替换选区）。
func (n *notepad) paste() {
	n.focusEditor()
	txt := browser.ClipboardText()
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
