// 本文件是组件库的装配与命令执行：Spec/Action 模型、Install、快捷键与
// 刷新；菜单/工具栏/状态栏的实现分别在 menu.go/toolbar.go/status.go。
package ui

import (
	"errors"

	"github.com/go-comm/ghromex/engine"
)

// ErrQuit 是「退出」命令的返回值：App 收到后请求关窗。
//
// 用返回值而不是让命令直接调 Window.Close：命令表可以在无头环境
// （ui.Install，没有窗口）里跑，退出语义必须由宿主决定。
var ErrQuit = errors.New("ui: quit")

// Quit 是标准的退出命令，可直接放进菜单：ui.Item("退出", "Alt+Q", ui.Quit)。
func Quit() error { return ErrQuit }

// Action 是一条应用命令，菜单项与工具栏按钮共用。
//
// 命令与呈现分离：同一份 Action 既能在菜单里出现，也能作为工具栏按钮，
// 快捷键只写一次。
type Action struct {
	Label    string // 显示文字（图标按钮上作为无图标时的兜底文字）
	Shortcut string // 展示与匹配用："Ctrl+S"、"Alt+Left"；空表示没有
	Icon     string // 图标名（见 iconGlyphs）或直接给字符；空表示没有
	Sep      bool   // 是否是分隔线（此时其余字段忽略）
	Run      func() error
	// Enable 返回命令当前是否可用；nil 表示恒可用。菜单打开时、按钮
	// 刷新时求值——不做每帧轮询。
	Enable func() bool
}

// Item 构造一条菜单命令。run 为 nil 时命令只关闭菜单。
func Item(label, shortcut string, run func() error) Action {
	return Action{Label: label, Shortcut: shortcut, Run: run}
}

// Button 构造一个工具栏按钮（无快捷键）。
func Button(label string, run func() error) Action {
	return Action{Label: label, Run: run}
}

// Sep 构造一条分隔线（菜单里是横线，工具栏里是竖线）。
func Sep() Action { return Action{Sep: true} }

// When 给命令挂可用性判断，返回新值便于链式书写：
//
//	ui.Item("保存", "Ctrl+S", save).When(editor.Dirty)
func (a Action) When(f func() bool) Action {
	a.Enable = f
	return a
}

// WithIcon 给命令挂图标，返回新值便于链式书写：
//
//	ui.Button("保存", save).WithIcon("save")
//
// 图标名见包内 iconGlyphs（new/open/save/copy/undo/redo/search/info…），
// 也可以直接写字符。同一份 Action 放进菜单与工具栏时，两处都用这个图标。
func (a Action) WithIcon(name string) Action {
	a.Icon = name
	return a
}

// Menu 是一个顶级菜单。
type Menu struct {
	Title string
	Items []Action
}

// Spec 描述一套外壳。
type Spec struct {
	// Title/Width/Height 只给 App 用（建窗口）；ui.Install 忽略。
	Title  string
	Width  int
	Height int

	// Theme 为空时用 Light()。
	Theme *Theme

	Menus []Menu
	Tools []Action

	// Status/StatusRight 是状态栏的初始左右文本。
	Status      string
	StatusRight string

	// Focus 返回「命令执行后该把焦点还给谁」。菜单/工具栏是普通 div，
	// 点它们会让页面里的输入框失焦；命令跑完把焦点还回去，用户才能
	// 接着打字（记事本点「保存」后光标应该还在原处）。
	Focus func() engine.HTMLElement
}

// Shell 是一套装在某个文档上的外壳。
//
// 生命周期与文档绑定：换文档要重新 Install（App 通过
// browser.Window.SetOnDocument 自动做这件事）。Shell 不持有窗口，
// 所以无头环境（ui.Install + DumpSVG/PNG）也能完整测试。
type Shell struct {
	doc   engine.HTMLDocument
	theme Theme
	spec  Spec

	menubar engine.HTMLElement
	menus   []*menuView

	toolbar engine.HTMLElement
	tools   []*toolView

	status      engine.HTMLElement
	statusLeft  engine.HTMLElement
	statusRight engine.HTMLElement

	keys []keyBind
	open *menuView

	// onQuit 由 App 注入（纯文档层没有窗口）：命令返回 ErrQuit 时调用。
	onQuit func()
}

// Install 把外壳装进一个已打开的文档，返回可继续操作的 Shell。
//
// 做四件事：
//  1. 追加主题样式表（在作者样式之后，同特异度时外壳胜出）；
//  2. 在 body 首尾插占位块——fixed 的栏不占文档流，页面内容要让出空间；
//  3. 用当前 Spec 生成菜单栏/工具栏/状态栏；
//  4. 绑定快捷键与「点别处关菜单」。
//
// doc 为 nil 时返回 nil（调用方不必判空文档）。
func Install(doc engine.HTMLDocument, spec Spec) *Shell {
	if doc == nil {
		return nil
	}
	theme := Light()
	if spec.Theme != nil {
		theme = spec.Theme.withDefaults()
	}

	menuH, toolH := 0, 0
	if len(spec.Menus) > 0 {
		menuH = theme.MenuHeight
	}
	if len(spec.Tools) > 0 {
		toolH = theme.ToolHeight
	}
	chromeH := menuH + toolH

	// 样式表最后注入：页面自己的 <style>/<link> 在解析阶段就合并完了，
	// 这里追加的规则在同特异度时后写胜出。
	doc.AddStylesheetText(theme.css(menuH, chromeH, theme.StatusHeight))

	s := &Shell{doc: doc, theme: theme, spec: spec}
	body := doc.Body()

	// 顶部占位：必须插在页面内容之前。放在 body 首而不是给 body 加
	// padding，页面自己的内边距才不会被覆盖。
	body.InsertBefore(newEl(doc, "div", "ui-spacer-top", ""), firstChild(body))

	if menuH > 0 {
		s.buildMenubar()
	}
	if toolH > 0 {
		s.buildToolbar()
	}
	s.buildStatus()
	body.AppendChild(newEl(doc, "div", "ui-spacer-bottom", ""))

	// 快捷键：菜单项与工具栏按钮都参与，先出现的绑定优先。
	// 同一个 Action 常同时放进菜单和工具栏（示例就是这样），按
	// 「键 + 修饰键」去重，避免同一命令注册两遍。
	seen := make(map[[2]int]bool)
	addKey := func(a Action) {
		if a.Sep || a.Shortcut == "" {
			return
		}
		key, mods, ok := parseShortcut(a.Shortcut)
		if !ok {
			return
		}
		sig := [2]int{key, int(mods)}
		if seen[sig] {
			return
		}
		seen[sig] = true
		s.keys = append(s.keys, keyBind{key: key, mods: mods, act: a})
	}
	for _, m := range spec.Menus {
		for _, a := range m.Items {
			addKey(a)
		}
	}
	for _, a := range spec.Tools {
		addKey(a)
	}

	// 点外壳以外的地方关菜单。挂在 body 上而不是菜单栏上：菜单是
	// fixed 的，点页面任意位置都该收起。浮层是菜单标签的子节点，
	// insideMenu 会把浮层区域算作「内部」，不会刚打开就被关掉。
	body.OnClick(func(ev *engine.MouseEvent) {
		if s.open == nil || s.insideMenu(ev.X, ev.Y) {
			return
		}
		s.CloseMenus()
	})

	if spec.Status != "" || spec.StatusRight != "" {
		s.SetStatus(spec.Status)
		s.SetStatusRight(spec.StatusRight)
	}
	s.Refresh()
	return s
}

// Doc 返回外壳所在的文档。
func (s *Shell) Doc() engine.HTMLDocument {
	if s == nil {
		return nil
	}
	return s.doc
}

// SetStatus 更新状态栏左侧文本。
func (s *Shell) SetStatus(text string) {
	if s == nil || s.statusLeft == nil {
		return
	}
	if s.statusLeft.Text() == text {
		return
	}
	s.statusLeft.SetText(text)
}

// SetStatusRight 更新状态栏右侧文本（提示/计数/快捷键）。
func (s *Shell) SetStatusRight(text string) {
	if s == nil || s.statusRight == nil {
		return
	}
	if s.statusRight.Text() == text {
		return
	}
	s.statusRight.SetText(text)
}

// Refresh 重算工具栏按钮的可用态。
//
// 只在命令执行后（以及安装时）调用：可用性不是每帧都变的量，轮询
// 只会让主循环每帧重排。菜单项在打开菜单时现算，不需要刷。
func (s *Shell) Refresh() {
	if s == nil {
		return
	}
	for _, tv := range s.tools {
		setClass(tv.el, toolClass(tv.act))
	}
}

// HandleKey 处理应用级按键：返回 true 表示已消费。
//
// 由 App 挂在 browser.Window.SetKeyHandler 上，位置在 Ctrl+C/V 与页面
// 之前——输入框聚焦时 Ctrl+S 也必须先到应用。
func (s *Shell) HandleKey(key int, mods uint16) bool {
	if s == nil {
		return false
	}
	if s.open != nil && key == engine.KeyEscape {
		s.CloseMenus()
		return true
	}
	key = normalizeKey(key)
	for _, b := range s.keys {
		if b.key != key || !sameMods(mods, b.mods) {
			continue
		}
		// 禁用命令也吞掉按键：否则 Ctrl+S 会继续落到页面，变成
		// 「输入框里插入控制字符」之类的怪事。
		if enabled(b.act) {
			s.invoke(b.act)
		}
		return true
	}
	return false
}

// invoke 执行一条命令：关菜单 → 判可用 → 执行 → 刷新 → 还焦点。
func (s *Shell) invoke(a Action) {
	s.CloseMenus()
	if !enabled(a) {
		s.Refresh()
		return
	}
	var err error
	if a.Run != nil {
		err = a.Run()
	}
	s.Refresh()
	// 点外壳会让输入框失焦（引擎「点空白失焦」的语义）；命令跑完
	// 把焦点还给应用指定的元素，键盘操作才能无缝接上。
	if s.spec.Focus != nil && engine.FocusedElement(s.doc) == nil {
		if el := s.spec.Focus(); el != nil {
			engine.SetFocusedElement(s.doc, el)
		}
	}
	switch {
	case errors.Is(err, ErrQuit):
		if s.onQuit != nil {
			s.onQuit()
		}
	case err != nil:
		// 命令失败就把原因放在状态栏——比静默吞掉好查得多。
		s.SetStatus(err.Error())
	}
}
