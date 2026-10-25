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

	// slot 非 nil 时这条不是命令，而是嵌进工具栏的现成控件（见 Slot）：
	// 不参与快捷键注册与可用态刷新，Label/Run/Enable 都忽略。
	slot Control
}

// Item 构造一条菜单命令。run 为 nil 时命令只关闭菜单。
func Item(label, shortcut string, run func() error) Action {
	return Action{Label: label, Shortcut: shortcut, Run: run}
}

// Tool 构造一个工具栏按钮（无快捷键），进 Spec.Tools、渲染成
// .system-ui-tool；与内容区控件的 NewButton 不是一回事。
func Tool(label string, run func() error) Action {
	return Action{Label: label, Run: run}
}

// Sep 构造一条分隔线（菜单里是横线，工具栏里是竖线）。
func Sep() Action { return Action{Sep: true} }

// Slot 把一个现成控件（输入框、下拉、按钮、标签等）嵌进工具栏，与命令
// 按钮同排：
//
//	addr := ui.NewTextInput(doc, "")
//	...
//	Tools: []ui.Action{
//		ui.Tool("刷新", refresh).WithIcon("refresh"),
//		ui.Slot(addr),
//		ui.Slot(ui.NewButton(doc, "打开", open).SetVariant("default")),
//	}
//
// 命令按钮是无边框的（悬停才铺底）；要「一眼可点」的动作按钮或纯文本
// 提示，走槽位放描边按钮 / Label，不做伪按钮。控件句柄仍归应用：取值、
// 设值、禁用直接调控件自己的方法，不必按 id 回查元素。槽位不是命令：
// 没有快捷键与可用态，禁用请用控件自己的 SetEnabled。
func Slot(c Control) Action {
	return Action{slot: c}
}

// When 给命令挂可用性判断，返回新值便于链式书写：
//
//	ui.Item("保存", "Ctrl+S", save).When(editor.Dirty)
func (a Action) When(f func() bool) Action {
	a.Enable = f
	return a
}

// WithIcon 给命令挂图标，返回新值便于链式书写：
//
//	ui.Tool("保存", save).WithIcon("save")
//
// 图标名见包内 iconGlyphs（文件/编辑、方向/导航、对齐/排版、播放/媒体、
// 视图/窗口和通用命令），常用如 up/down/left/right、align-left/align-center/
// align-right、play/pause/stop、zoom-in/zoom-out；也可以直接写字符。同一份
// Action 放进菜单与工具栏时，两处都用这个图标。
func (a Action) WithIcon(name string) Action {
	a.Icon = name
	return a
}

// Menu 是一个顶级菜单。
type Menu struct {
	Title string
	Items []Action
}

// Spec 描述一套工作台。
type Spec struct {
	// Title/Width/Height 只给 App 用（建窗口）；ui.Install 忽略。
	Title  string
	Width  int
	Height int

	// Theme 为空时用 Light()。
	Theme *Theme

	// NoBodyReset 跳过对 body 的样式复位（margin/padding/color/font）；
	// 画布底色（html background-color）不受影响，始终注入。
	//
	// 默认 Install 会复位 body：UA 的 8px 外边距与默认字体不该让每个页面
	// 重复写一遍。两类页面需要关掉它：
	//   - 演示 UA 样式的页面——body 的 margin:8px 本身就是演示内容；
	//   - 靠 body padding 给固定栏让位的页面——复位会把排版顶歪。
	//
	// 关掉之后 body 的外观由页面自己负责。
	NoBodyReset bool

	Menus []Menu
	Tools []Action

	// Status/StatusRight 是状态栏的初始左右文本。
	//
	// StatusSegments 非空时优先使用分段；Status 只在没有分段时作为
	// 单段状态栏的快捷写法（等价于一个 Key 为 "status" 的段）。
	// StatusRight 是右侧消息的快捷写法，等价于一个 AnchorRight、
	// Key 为保留键 "status-right" 的段。
	Status         string
	StatusRight    string
	StatusSegments []StatusSegment

	// Focus 返回「命令执行后该把焦点还给谁」。菜单/工具栏是普通 div，
	// 点它们会让页面里的输入框失焦；命令跑完把焦点还回去，用户才能
	// 接着打字（记事本点「保存」后光标应该还在原处）。
	Focus func() engine.HTMLElement
}

// Workbench（工作台）是一套装在某个文档上的周边构件：内容四周的菜单栏、
// 工具栏、状态栏，加上主题样式与快捷键表。
//
// 生命周期与文档绑定：换文档要重新 Install（App 通过
// browser.Window.SetOnDocument 自动做这件事）。Workbench 不持有窗口，
// 所以无头环境（ui.Install + DumpSVG/PNG）也能完整测试。
type Workbench struct {
	doc   engine.HTMLDocument
	theme Theme
	spec  Spec

	menubar engine.HTMLElement
	menus   []*menuView

	toolbar engine.HTMLElement
	tools   []*toolView

	status          engine.HTMLElement
	statusTrack     engine.HTMLElement
	statusLeft      engine.HTMLElement
	statusRightBox  engine.HTMLElement // 右侧固定区容器：右钉段 + 糖消息
	statusRight     engine.HTMLElement // 糖消息元素，可为 nil（无消息）
	statusRightText string             // 糖消息文本：跨 SetStatusSegments 重建保留
	rightVisible    int                // 已生成的右钉段数（糖消息不计）
	lastRightSeg    engine.HTMLElement // 最右一个右钉段（贴边位归它，可为 nil）
	statusSegs      []statusSegView
	spacerBottom    engine.HTMLElement

	keys []keyBind
	open *menuView

	// onQuit 由 App 注入（纯文档层没有窗口）：命令返回 ErrQuit 时调用。
	onQuit func()
}

// Install 把工作台装进一个已打开的文档，返回可继续操作的 Workbench。
//
// 做四件事：
//  1. 追加主题样式表（在作者样式之后，同特异度时工作台胜出）；
//  2. 在 body 首尾插占位块——fixed 的栏不占文档流，页面内容要让出空间；
//  3. 用当前 Spec 生成菜单栏/工具栏/状态栏；
//  4. 绑定快捷键与「点别处关菜单」。
//
// doc 为 nil 时返回 nil（调用方不必判空文档）。
func Install(doc engine.HTMLDocument, spec Spec) *Workbench {
	if doc == nil {
		return nil
	}
	theme := Light()
	if spec.Theme != nil {
		theme = spec.Theme.withDefaults()
	}
	// 组件样式全部挂在 html.system-ui 下；根类由 Install 统一加，
	// 页面不用自己记得写，也不会因为漏写而让工作台失去样式。
	addClass(doc, uiRootClass)

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
	doc.AddStylesheetText(theme.css(menuH, chromeH, theme.StatusHeight, !spec.NoBodyReset))
	// 提示气泡是绘制层、拿不到级联样式，配色单独随主题下发。
	engine.SetTooltipColors(doc, theme.tooltipColors())

	s := &Workbench{doc: doc, theme: theme, spec: spec}
	body := doc.Body()

	// 顶部占位：必须插在页面内容之前。放在 body 首而不是给 body 加
	// padding，页面自己的内边距才不会被覆盖。
	body.InsertBefore(newEl(doc, "div", "system-ui-spacer-top", ""), firstChild(body))

	if toolH > 0 {
		s.buildToolbar()
	}
	s.buildStatus()
	s.spacerBottom = newEl(doc, "div", "system-ui-spacer-bottom", "")
	body.AppendChild(s.spacerBottom)
	// 菜单栏最后插入：菜单浮层会盖住工具栏，而引擎的命中测试按 DOM
	// 顺序「后者胜出」，菜单栏排在工具栏之前时浮层项会被工具栏截胡。
	if menuH > 0 {
		s.buildMenubar()
	}

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

	// 点工作台以外的地方关菜单。挂在 body 上而不是菜单栏上：菜单是
	// fixed 的，点页面任意位置都该收起。浮层是菜单标签的子节点，
	// insideMenu 会把浮层区域算作「内部」，不会刚打开就被关掉。
	body.OnClick(func(ev *engine.MouseEvent) {
		if s.open == nil || s.insideMenu(ev.X, ev.Y) {
			return
		}
		s.CloseMenus()
	})

	s.Refresh()
	return s
}

// Doc 返回工作台所在的文档。
func (s *Workbench) Doc() engine.HTMLDocument {
	if s == nil {
		return nil
	}
	return s.doc
}

// SetStatusVisible 显示或隐藏状态栏，同时收起/恢复底部占位。
//
// 应用可以把它接到「视图 → 显示状态栏」这类命令上；隐藏时底部占位一起
// 收起，页面内容可以使用腾出的高度，不会被固定栏挡住。
func (s *Workbench) SetStatusVisible(visible bool) {
	if s == nil {
		return
	}
	display := "none"
	if visible {
		display = "block"
	}
	if s.status != nil {
		s.status.SetAttribute("style", "display:"+display)
	}
	if s.spacerBottom != nil {
		s.spacerBottom.SetAttribute("style", "display:"+display)
	}
}

// SetTheme 在不重建菜单/工具栏/页面的前提下换一套主题。
//
// 新样式表追加在旧样式之后，同特异度时后写胜出；Workbench 的尺寸跟着新主题
// 重算，浅色/深色（尺寸相同）切换时只有配色变化。旧样式表不会被移除，
// 反复切换会让样式表增长，适合交互式切换而不是高频轮换。
func (s *Workbench) SetTheme(theme Theme) {
	if s == nil || s.doc == nil {
		return
	}
	theme = theme.withDefaults()
	if s.theme == theme {
		return
	}
	menuH, toolH := 0, 0
	if len(s.spec.Menus) > 0 {
		menuH = theme.MenuHeight
	}
	if len(s.spec.Tools) > 0 {
		toolH = theme.ToolHeight
	}
	s.doc.AddStylesheetText(theme.css(menuH, menuH+toolH, theme.StatusHeight, !s.spec.NoBodyReset))
	engine.SetTooltipColors(s.doc, theme.tooltipColors())
	s.theme = theme
	s.spec.Theme = &theme
}

// Refresh 重算工具栏按钮的可用态。
//
// 只在命令执行后（以及安装时）调用：可用性不是每帧都变的量，轮询
// 只会让主循环每帧重排。菜单项在打开菜单时现算，不需要刷。
func (s *Workbench) Refresh() {
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
func (s *Workbench) HandleKey(key int, mods uint16) bool {
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
func (s *Workbench) invoke(a Action) {
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
	// 点工作台会让输入框失焦（引擎「点空白失焦」的语义）；命令跑完
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
