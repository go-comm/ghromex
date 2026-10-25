package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// newTestDoc 打开一个无头文档并完成首次布局。
func newTestDoc(t *testing.T, w, h int, page string) engine.HTMLDocument {
	t.Helper()
	vp := engine.NewHeadlessViewport(w, h)
	doc, err := engine.OpenDocument(vp, page)
	if err != nil {
		t.Fatalf("打开文档失败: %v", err)
	}
	doc.Refresh()
	return doc
}

// clickEl 在元素中心点一次（点击 → 重排，后续读到的坐标才是新的）。
func clickEl(t *testing.T, doc engine.HTMLDocument, el engine.HTMLElement) {
	t.Helper()
	if el == nil {
		t.Fatal("点击的元素不存在")
	}
	clickAt(t, doc, el.X()+el.Width()/2, el.Y()+el.Height()/2)
}

func clickAt(t *testing.T, doc engine.HTMLDocument, x, y int) {
	t.Helper()
	// OnDocumentClick 的返回值是「有没有处理器被触发」，不是「有没有命中」；
	// 判断命中要用 ElementAt（点纯展示元素时返回 false 很正常）。
	if engine.ElementAt(doc, x, y) == nil {
		t.Fatalf("点击 (%d,%d) 没有命中任何元素", x, y)
	}
	engine.OnDocumentClick(doc, x, y)
	doc.Refresh()
}

// box 读元素布局后的边框盒。
//
// 不用 X()/Y()/Width()/Height()：那组是内容盒（有 padding/border 的元素
// 会小一圈），断言几何时容易把「内容盒」当成「外框」。
type box struct{ x, y, w, h int }

func boxOf(e engine.HTMLElement) box {
	r := e.GetBoundingClientRect()
	l, t := r.Left().Pixel(), r.Top().Pixel()
	return box{x: l, y: t, w: r.Right().Pixel() - l, h: r.Bottom().Pixel() - t}
}

// 菜单栏/工具栏/状态栏的位置与占位块：工作台装好后不能盖住页面内容。
func TestInstallBuildsChrome(t *testing.T) {
	th := Light()
	doc := newTestDoc(t, 640, 480, `<div id="page">内容</div>`)
	sh := Install(doc, Spec{
		Menus:  []Menu{{Title: "文件", Items: []Action{Item("保存", "Ctrl+S", nil)}}},
		Tools:  []Action{Tool("保存", nil)},
		Status: "就绪",
	})
	doc.Refresh()

	bar := doc.QuerySelector(".system-ui-menubar")
	if bar == nil {
		t.Fatal("缺少菜单栏")
	}
	if b := boxOf(bar); b.x != 0 || b.y != 0 || b.w != 640 || b.h != th.MenuHeight {
		t.Fatalf("菜单栏框 = %+v，期望 (0,0) 640x%d", b, th.MenuHeight)
	}

	toolbar := doc.QuerySelector(".system-ui-toolbar")
	if toolbar == nil {
		t.Fatal("缺少工具栏")
	}
	if b := boxOf(toolbar); b.y != th.MenuHeight || b.h != th.ToolHeight {
		t.Fatalf("工具栏框 = %+v，期望 y=%d h=%d", b, th.MenuHeight, th.ToolHeight)
	}

	status := doc.QuerySelector(".system-ui-status")
	if status == nil {
		t.Fatal("缺少状态栏")
	}
	if b := boxOf(status); b.y != 480-th.StatusHeight || b.h != th.StatusHeight {
		t.Fatalf("状态栏框 = %+v，期望贴底 h=%d", b, th.StatusHeight)
	}

	// 占位块：顶部 = 菜单栏 + 工具栏，底部 = 状态栏。
	top := doc.QuerySelector(".system-ui-spacer-top")
	if top == nil || boxOf(top).h != th.MenuHeight+th.ToolHeight {
		t.Fatalf("顶部占位块 = %v，期望高 %d", top, th.MenuHeight+th.ToolHeight)
	}
	bottom := doc.QuerySelector(".system-ui-spacer-bottom")
	if bottom == nil || boxOf(bottom).h != th.StatusHeight {
		t.Fatalf("底部占位块 = %v，期望高 %d", bottom, th.StatusHeight)
	}

	// 页面内容必须排在顶部占位块之后，否则会被栏盖住。
	page := doc.GetElementById("page")
	if page == nil {
		t.Fatal("页面内容丢失")
	}
	if page.Y() < top.Y()+boxOf(top).h {
		t.Fatalf("页面内容 y=%d 被顶部占位块（到 %d）盖住", page.Y(), top.Y()+boxOf(top).h)
	}

	if sh.statusLeft.Text() != "就绪" {
		t.Fatalf("状态栏文本 = %q", sh.statusLeft.Text())
	}
}

// Install 复位 body：工作台画布没有 UA 外边距，颜色字体跟随主题——页面
// 不必再自写 margin:0 / padding:0 / color / font-family 这些样板。
func TestInstallResetsBody(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div id="page">内容</div>`)
	Install(doc, Spec{Status: "就绪"})
	doc.Refresh()

	if b := boxOf(doc.Body()); b.x != 0 || b.y != 0 || b.w != 640 {
		t.Fatalf("body 框 = %+v，期望贴左上且铺满宽（UA 外边距未复位）", b)
	}
}

// Spec.NoBodyReset 跳过 body 复位：演示 UA 样式的页面（body 的 margin:8px
// 本身就是内容）、靠 body padding 给固定栏让位的页面，需要保住原样。
func TestInstallSkipsBodyReset(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div id="page">内容</div>`)
	Install(doc, Spec{Status: "就绪", NoBodyReset: true})
	doc.Refresh()

	if b := boxOf(doc.Body()); b.x != 8 {
		t.Fatalf("body 框 = %+v，期望保留 UA 的 8px 外边距", b)
	}
}

// 没有菜单/工具栏时不生成对应的栏，顶部占位块高度随之收敛。
func TestInstallWithoutBars(t *testing.T) {
	doc := newTestDoc(t, 400, 300, `<div>内容</div>`)
	Install(doc, Spec{Status: "就绪"})
	doc.Refresh()

	if doc.QuerySelector(".system-ui-menubar") != nil {
		t.Fatal("没有菜单时不该生成菜单栏")
	}
	if doc.QuerySelector(".system-ui-toolbar") != nil {
		t.Fatal("没有工具栏配置时不该生成工具栏")
	}
	if got := doc.QuerySelector(".system-ui-spacer-top").Height(); got != 0 {
		t.Fatalf("顶部占位块 = %d，期望 0", got)
	}
}

func TestInstallNilDocument(t *testing.T) {
	if sh := Install(nil, Spec{}); sh != nil {
		t.Fatal("doc 为 nil 时应返回 nil")
	}
}

// 组件样式全部挂在 html.system-ui 下；Install 负责加根类，并保留
// 页面已有的 class，不能因为加了作用域把 <html class="app"> 覆盖掉。
func TestInstallAddsRootScope(t *testing.T) {
	doc := newTestDoc(t, 400, 300, `<html class="app"><body><div>内容</div></body></html>`)
	Install(doc, Spec{})
	if !hasWord(doc.GetAttribute("class"), uiRootClass) {
		t.Fatalf("根元素 class = %q，缺少 %s", doc.GetAttribute("class"), uiRootClass)
	}
	if !hasWord(doc.GetAttribute("class"), "app") {
		t.Fatalf("根元素原有 class 丢失: %q", doc.GetAttribute("class"))
	}
}

func hasWord(s, word string) bool {
	for _, f := range strings.Fields(s) {
		if f == word {
			return true
		}
	}
	return false
}

// 点标签开菜单、点菜单项执行并收起。
func TestMenuOpensRunsAndCloses(t *testing.T) {
	var ran string
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{
			Item("新建", "Ctrl+N", func() error { ran = "new"; return nil }),
			Sep(),
			Item("保存", "Ctrl+S", func() error { ran = "save"; return nil }),
		}}},
	})
	doc.Refresh()

	label := doc.QuerySelector(".system-ui-menu")
	clickEl(t, doc, label)
	if sh.open == nil {
		t.Fatal("点菜单标签应展开菜单")
	}
	pop := doc.QuerySelector(".system-ui-menu-pop")
	if pop == nil || boxOf(pop).h == 0 {
		t.Fatal("展开后浮层应可见（display:block）")
	}
	if w := boxOf(pop).w; w != 216 {
		t.Fatalf("浮层宽度 = %d，期望 216", w)
	}
	if doc.QuerySelector(".system-ui-menu-sep") == nil {
		t.Fatal("分隔线没有渲染")
	}

	items := doc.GetElementsByClassName("system-ui-menu-item")
	if len(items) != 2 {
		t.Fatalf("菜单项数量 = %d，期望 2", len(items))
	}
	clickEl(t, doc, items[1])
	if ran != "save" {
		t.Fatalf("执行的命令 = %q，期望 save", ran)
	}
	if sh.open != nil {
		t.Fatal("执行命令后菜单应收起")
	}
}

// 菜单浮层会盖住工具栏。引擎的命中测试按 DOM 顺序「后者胜出」，
// 菜单栏必须排在工具栏之后，否则浮层里的项会被工具栏按钮截胡。
func TestMenuPopupOverToolbarIsClickable(t *testing.T) {
	ran := ""
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{
			Item("保存", "", func() error { ran = "save"; return nil }),
		}}},
		Tools: []Action{Tool("工具", func() error { ran = "tool"; return nil })},
	})
	doc.Refresh()

	clickEl(t, doc, doc.QuerySelector(".system-ui-menu"))
	clickEl(t, doc, doc.QuerySelector(".system-ui-menu-item"))
	if ran != "save" {
		t.Fatalf("点浮层菜单项执行了 %q，期望 save（不应被工具栏截胡）", ran)
	}
}

// 点同一个标签第二次收起；点浮层的空白处不应收起（事件会冒泡到标签）。
func TestMenuToggleAndInnerClick(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{Item("新建", "", nil)}}},
	})
	doc.Refresh()

	label := doc.QuerySelector(".system-ui-menu")
	clickEl(t, doc, label)
	if sh.open == nil {
		t.Fatal("应展开")
	}
	pop := doc.QuerySelector(".system-ui-menu-pop")
	// 浮层内边距区域：事件目标是浮层本身，不是标签——不能二次 toggle。
	// 用边框盒 + 3px 落在内边距里（内容盒原点已经越过 1px 边框 + 5px 内边距，
	// 直接加 2px 会点到第一项上）。
	pb := boxOf(pop)
	clickAt(t, doc, pb.x+3, pb.y+3)
	if sh.open == nil {
		t.Fatal("点浮层内部不应收起菜单")
	}
	clickEl(t, doc, label)
	if sh.open != nil {
		t.Fatal("再次点标签应收起")
	}
}

// 点工作台以外的地方收起菜单。
func TestClickOutsideClosesMenu(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div id="page" style="width:300px;height:200px">内容</div>`)
	sh := Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{Item("新建", "", nil)}}},
	})
	doc.Refresh()

	clickEl(t, doc, doc.QuerySelector(".system-ui-menu"))
	if sh.open == nil {
		t.Fatal("应展开")
	}
	page := doc.GetElementById("page")
	clickEl(t, doc, page)
	if sh.open != nil {
		t.Fatal("点页面内容应收起菜单")
	}
}

// 禁用命令：菜单项变灰、点击不执行、仍然收起（点了但没反应）。
func TestDisabledMenuCommand(t *testing.T) {
	ran := false
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{
			Item("保存", "Ctrl+S", func() error { ran = true; return nil }).When(func() bool { return false }),
		}}},
	})
	doc.Refresh()

	clickEl(t, doc, doc.QuerySelector(".system-ui-menu"))
	item := doc.QuerySelector(".system-ui-menu-item")
	if !strings.Contains(item.GetAttribute("class"), "is-disabled") {
		t.Fatalf("禁用项类名 = %q", item.GetAttribute("class"))
	}
	clickEl(t, doc, item)
	if ran {
		t.Fatal("禁用命令不应执行")
	}
	// 点了没反应，菜单保持展开（与桌面系统的禁用菜单项一致）。
	if sh.open == nil {
		t.Fatal("点禁用项不应收起菜单")
	}
}

// 快捷键：Ctrl/Shift/Alt 按家族匹配（左右 Ctrl 都算），字母大小写不敏感。
func TestShortcutMatching(t *testing.T) {
	ran := 0
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{
			Item("保存", "Ctrl+S", func() error { ran++; return nil }),
			Item("另存为", "Ctrl+Shift+S", func() error { ran += 10; return nil }),
			Item("回退", "Alt+Left", func() error { ran += 100; return nil }),
		}}},
	})

	cases := []struct {
		name string
		key  int
		mods uint16
		want int
	}{
		{"小写 Ctrl+S", 's', engine.ModCtrl, 1},
		{"大写 Ctrl+S", 'S', engine.ModCtrl, 1},
		{"只按左 Ctrl", 's', engine.ModCtrl &^ 0x80, 1}, // 0x40：SDL 只置左键位
		{"Ctrl+Shift+S", 's', engine.ModCtrl | engine.ModShift, 10},
		{"Alt+Left", engine.KeyLeft, engine.ModAlt, 100},
	}
	for _, c := range cases {
		ran = 0
		if !sh.HandleKey(c.key, c.mods) {
			t.Fatalf("%s: 应消费按键", c.name)
		}
		if ran != c.want {
			t.Fatalf("%s: 命中命令权重 = %d，期望 %d", c.name, ran, c.want)
		}
	}

	// 不匹配的组合不应消费，也不应执行。
	for _, c := range []struct {
		name string
		key  int
		mods uint16
	}{
		{"无修饰键", 's', 0},
		{"Ctrl+X", 'x', engine.ModCtrl},
		{"Ctrl+Shift+Alt+S", 's', engine.ModCtrl | engine.ModShift | engine.ModAlt},
	} {
		ran = 0
		if sh.HandleKey(c.key, c.mods) {
			t.Fatalf("%s: 不该消费按键", c.name)
		}
		if ran != 0 {
			t.Fatalf("%s: 不该执行命令", c.name)
		}
	}
}

// 禁用命令的快捷键也要吞掉：否则 Ctrl+S 会继续落到页面。
func TestDisabledShortcutConsumed(t *testing.T) {
	ran := false
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{
			Item("保存", "Ctrl+S", func() error { ran = true; return nil }).When(func() bool { return false }),
		}}},
	})

	if !sh.HandleKey('s', engine.ModCtrl) {
		t.Fatal("禁用命令的快捷键也应被消费")
	}
	if ran {
		t.Fatal("禁用命令不应执行")
	}
}

// Escape 收起菜单；没有菜单打开时不消费。
func TestEscapeClosesMenu(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{Item("新建", "", nil)}}},
	})
	doc.Refresh()

	if sh.HandleKey(engine.KeyEscape, 0) {
		t.Fatal("没有打开的菜单时 Escape 不该被消费")
	}
	clickEl(t, doc, doc.QuerySelector(".system-ui-menu"))
	if sh.open == nil {
		t.Fatal("应展开")
	}
	if !sh.HandleKey(engine.KeyEscape, 0) {
		t.Fatal("Escape 应收起菜单并消费按键")
	}
	if sh.open != nil {
		t.Fatal("Escape 后菜单应收起")
	}
}

// 工具栏按钮：点击执行；Refresh 后禁用态跟着 Enable 变化。
func TestToolbarButtonAndRefresh(t *testing.T) {
	ran := 0
	on := true
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Tools: []Action{
			Tool("保存", func() error { ran++; return nil }).When(func() bool { return on }),
			Sep(),
			Tool("复制", func() error { ran += 10; return nil }),
		},
	})
	doc.Refresh()

	buttons := doc.GetElementsByClassName("system-ui-btn")
	if len(buttons) != 2 {
		t.Fatalf("按钮数量 = %d，期望 2", len(buttons))
	}
	if doc.QuerySelector(".system-ui-sep") == nil {
		t.Fatal("工具栏分隔线没有渲染")
	}

	clickEl(t, doc, buttons[0])
	if ran != 1 {
		t.Fatalf("点击按钮执行次数 = %d", ran)
	}

	on = false
	sh.Refresh()
	doc.Refresh()
	if !strings.Contains(buttons[0].GetAttribute("class"), "is-disabled") {
		t.Fatalf("禁用后类名 = %q", buttons[0].GetAttribute("class"))
	}
	clickEl(t, doc, buttons[0])
	if ran != 1 {
		t.Fatal("禁用按钮不应执行")
	}
}

// 工具栏槽位：现成控件（输入框/下拉）嵌进工具栏后仍是应用手里的那一个——
// 取值设值直接调控件句柄；槽位不是命令，不进按钮刷新表。
func TestToolbarSlots(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	addr := NewTextInput(doc, "")
	sel := NewSelect(doc, []SelectOption{{Value: "a", Label: "甲"}, {Value: "b", Label: "乙"}})
	sh := Install(doc, Spec{
		Tools: []Action{
			Tool("刷新", func() error { return nil }).WithIcon("refresh"),
			Slot(addr),
			Slot(sel),
		},
	})
	doc.Refresh()

	bar := doc.QuerySelector(".system-ui-toolbar")
	if bar == nil {
		t.Fatal("没有生成工具栏")
	}
	if got := len(bar.QuerySelectorAll(".system-ui-tool-slot")); got != 2 {
		t.Fatalf("槽位数 = %d，期望 2", got)
	}
	// 槽位里是控件本体：句柄读写的就是工具栏里的那个元素。
	addr.SetValue("hello")
	if got := addr.Value(); got != "hello" {
		t.Fatalf("经句柄设值后 = %q", got)
	}
	// 槽位不是命令：只有命令按钮进可用态刷新表。
	if got := len(sh.tools); got != 1 {
		t.Fatalf("命令按钮数 = %d，期望 1（槽位不参与刷新）", got)
	}
	// 槽位控件随工具栏布局：高度与按钮同为 ToolControlHeight，位置在栏内。
	if got := boxOf(sel.Element()); got.h != Light().ToolControlHeight || got.y < boxOf(bar).y {
		t.Fatalf("下拉框框 = %+v，期望与按钮同高（%d）且在栏内", got, Light().ToolControlHeight)
	}
}

// 工具栏：带图标的命令画成方形图标按钮，文字留在 Label 里当兜底；
// 没有图标（或图标名拼错）时退回文字按钮。
func TestToolbarIcons(t *testing.T) {
	th := Light()
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	Install(doc, Spec{Tools: []Action{
		Tool("保存", nil).WithIcon("save"),
		Tool("复制", nil),
		Tool("关于", nil).WithIcon("no-such-icon"),
	}})
	doc.Refresh()

	buttons := doc.GetElementsByClassName("system-ui-btn")
	if len(buttons) != 3 {
		t.Fatalf("按钮数量 = %d，期望 3", len(buttons))
	}
	saved := buttons[0]
	if class := saved.GetAttribute("class"); !strings.Contains(class, "is-icon") {
		t.Fatalf("图标按钮类名 = %q，期望含 is-icon", class)
	}
	if b := boxOf(saved); b.w != th.ToolControlHeight || b.h != th.ToolControlHeight {
		t.Fatalf("图标按钮尺寸 = %dx%d，期望正方形 %d", b.w, b.h, th.ToolControlHeight)
	}
	// 图标按钮只显示字形，不显示 Label 文字。
	if got := saved.Text(); got != "" {
		t.Fatalf("图标按钮自身文本 = %q，期望空（标签不应画出来）", got)
	}
	kids := saved.Children()
	if len(kids) != 1 || kids[0].Text() != iconGlyphs["save"] {
		t.Fatalf("图标按钮子元素 = %v，期望一个字形 %q", kids, iconGlyphs["save"])
	}

	label := buttons[1]
	if class := label.GetAttribute("class"); !strings.Contains(class, "is-label") {
		t.Fatalf("文字按钮类名 = %q，期望含 is-label", class)
	}
	if got := label.Text(); got != "复制" {
		t.Fatalf("文字按钮文本 = %q，期望 复制", got)
	}

	// 拼错的图标名按「没有图标」处理，避免把名字当文字画出来。
	if class := buttons[2].GetAttribute("class"); !strings.Contains(class, "is-label") {
		t.Fatalf("拼错的图标名应按文字按钮处理，类名 = %q", class)
	}
	if got := buttons[2].Text(); got != "关于" {
		t.Fatalf("拼错的图标名不应显示图标，文本 = %q", got)
	}
}

// 工具栏按钮带原生 title 提示（标签 + 快捷键），悬停气泡由引擎绘制——
// 图标按钮丢掉的文字靠它还给用户。
func TestToolbarTitleTips(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	Install(doc, Spec{Tools: []Action{
		Item("保存", "Ctrl+S", nil).WithIcon("save"),
		Item("重命名", "F2", nil),      // 纯快捷键也有提示
		Tool("无快捷键", nil),           // 只有标签
		{Label: "", Shortcut: "F1"}, // 只有快捷键
	}})

	tools := doc.QuerySelectorAll(".system-ui-tool")
	if len(tools) != 4 {
		t.Fatalf("工具按钮数 = %d，期望 4", len(tools))
	}
	want := []string{"保存  Ctrl+S", "重命名  F2", "无快捷键", "F1"}
	for i, w := range want {
		if got := tools[i].GetAttribute("title"); got != w {
			t.Errorf("按钮 %d 的提示 = %q，期望 %q", i, got, w)
		}
	}
}

// 菜单：任意一项有图标就整张菜单预留图标列，浮层相应加宽。
func TestMenuIcons(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	Install(doc, Spec{Menus: []Menu{{Title: "文件", Items: []Action{
		Item("保存", "Ctrl+S", nil).WithIcon("save"),
		Item("另存为…", "", nil),
	}}}})
	doc.Refresh()

	clickEl(t, doc, doc.QuerySelector(".system-ui-menu"))
	pop := doc.QuerySelector(".system-ui-menu-pop")
	if pop == nil {
		t.Fatal("菜单浮层没有展开")
	}
	if class := pop.GetAttribute("class"); !strings.Contains(class, "has-icon") {
		t.Fatalf("带图标的浮层类名 = %q，期望含 has-icon", class)
	}
	if w := boxOf(pop).w; w != 240 {
		t.Fatalf("带图标浮层宽度 = %d，期望 240", w)
	}
	icons := doc.GetElementsByClassName("system-ui-menu-icon")
	if len(icons) != 1 {
		t.Fatalf("菜单图标数量 = %d，期望 1（只有保存有图标）", len(icons))
	}
	if got := icons[0].Text(); got != iconGlyphs["save"] {
		t.Fatalf("菜单图标字形 = %q，期望 %q", got, iconGlyphs["save"])
	}
}

// iconGlyph：登记名取字形；未登记的短字符串按自定义字形透传；长名字返回空。
func TestIconGlyph(t *testing.T) {
	if got := iconGlyph("save"); got != "\uE74E" {
		t.Fatalf("save 字形 = %q", got)
	}
	if got := iconGlyph("replace"); got != "\uE8AB" {
		t.Fatalf("replace 字形 = %q", got)
	}
	if got := iconGlyph("＋"); got != "＋" {
		t.Fatalf("自定义字符 = %q，期望原样透传", got)
	}
	if got := iconGlyph("savee"); got != "" {
		t.Fatalf("未登记的长名字 = %q，期望空", got)
	}
	if got := iconGlyph(""); got != "" {
		t.Fatalf("空图标名 = %q，期望空", got)
	}
}

// 同一个 Action 同时放进菜单和工具栏时，快捷键只注册一次。
func TestShortcutRegisteredOnce(t *testing.T) {
	save := Item("保存", "Ctrl+S", nil)
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Menus: []Menu{{Title: "文件", Items: []Action{save}}},
		Tools: []Action{save},
	})
	if len(sh.keys) != 1 {
		t.Fatalf("快捷键绑定数 = %d，期望 1", len(sh.keys))
	}
}

// 点工作台会让输入框失焦：命令执行后按 Spec.Focus 还回去。
func TestCommandRestoresFocus(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div id="page">内容</div><input id="ed" type="text">`)
	doc.Refresh()
	editor := doc.GetElementById("ed")

	// 前提：点普通 div 会清掉焦点（引擎「点空白失焦」的语义）。
	engine.SetFocusedElement(doc, editor)
	if engine.FocusedElement(doc) != editor {
		t.Fatal("输入框没能聚焦")
	}
	clickEl(t, doc, doc.GetElementById("page"))
	if engine.FocusedElement(doc) != nil {
		t.Fatal("点普通 div 应清掉焦点")
	}

	Install(doc, Spec{
		Tools: []Action{Tool("保存", func() error { return nil })},
		Focus: func() engine.HTMLElement { return editor },
	})
	doc.Refresh()

	// 命令执行前把焦点放到输入框：点击工作台时它会被清掉，命令跑完应还回来。
	engine.SetFocusedElement(doc, editor)
	btn := doc.QuerySelector(".system-ui-btn")
	x, y := btn.X()+btn.Width()/2, btn.Y()+btn.Height()/2
	// 点击流程里引擎会先按命中目标更新焦点（普通 div → 清空），再派发
	// click；命令就是在「焦点已被清掉」的状态下执行的。
	clickAt(t, doc, x, y)

	if got := engine.FocusedElement(doc); got != editor {
		t.Fatalf("命令执行后焦点 = %v，期望回到输入框", got)
	}
}

// 命令返回错误写到状态栏左侧。
func TestCommandErrorGoesToStatus(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		Tools:  []Action{Tool("打开", func() error { return os.ErrNotExist })},
		Status: "就绪",
	})
	doc.Refresh()

	clickEl(t, doc, doc.QuerySelector(".system-ui-btn"))
	if got := sh.statusLeft.Text(); got != os.ErrNotExist.Error() {
		t.Fatalf("状态栏 = %q，期望错误文本", got)
	}
}

// SetStatus/SetStatusRight 是幂等的：文本没变时不触发文档变更。
func TestSetStatus(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{Status: "就绪"})
	doc.Refresh()

	sh.SetStatus("已保存")
	sh.SetStatusRight("3 个文件")
	if sh.statusLeft.Text() != "已保存" || sh.statusRight.Text() != "3 个文件" {
		t.Fatalf("状态栏 = %q / %q", sh.statusLeft.Text(), sh.statusRight.Text())
	}

	before := doc.ChangeCount()
	sh.SetStatus("已保存")
	sh.SetStatusRight("3 个文件")
	if doc.ChangeCount() != before {
		t.Fatal("设置相同文本不应产生文档变更")
	}
}

// 状态栏隐藏时，底部占位也要一起收起；重新显示时两者都恢复。
func TestSetStatusVisible(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{Status: "就绪"})
	status := doc.QuerySelector(".system-ui-status")
	spacer := doc.QuerySelector(".system-ui-spacer-bottom")
	if status == nil || spacer == nil {
		t.Fatal("状态栏或底部占位不存在")
	}

	sh.SetStatusVisible(false)
	doc.Refresh()
	if got := status.GetAttribute("style"); !strings.Contains(got, "display:none") {
		t.Fatalf("隐藏后状态栏样式 = %q", got)
	}
	if got := spacer.GetAttribute("style"); !strings.Contains(got, "display:none") {
		t.Fatalf("隐藏后底部占位样式 = %q", got)
	}

	sh.SetStatusVisible(true)
	doc.Refresh()
	if got := status.GetAttribute("style"); !strings.Contains(got, "display:block") {
		t.Fatalf("恢复后状态栏样式 = %q", got)
	}
	if got := spacer.GetAttribute("style"); !strings.Contains(got, "display:block") {
		t.Fatalf("恢复后底部占位样式 = %q", got)
	}
}

// 主题零值补全 + 生成的 CSS 里不能残留占位符。
func TestThemeDefaultsAndCSSTokens(t *testing.T) {
	th := Theme{Accent: "#0F766E", Radius: 12}.withDefaults()
	if th.Accent != "#0F766E" || th.Radius != 12 {
		t.Fatal("显式设置的字段被覆盖")
	}
	if th.Bg != Light().Bg || th.MenuHeight != Light().MenuHeight {
		t.Fatal("零值字段没有用 Light() 补齐")
	}
	css := th.css(30, 68, 24, true)
	if strings.Contains(css, "$") {
		t.Fatalf("CSS 里残留未替换的占位符:\n%s", css)
	}
	if !strings.Contains(css, "#0F766E") {
		t.Fatal("强调色没有写进 CSS")
	}
	if !strings.Contains(css, "height: 68px") {
		t.Fatal("顶部占位块高度没有写进 CSS")
	}
}

// App 每次换文档都重新装工作台，快捷键路由跟着新 Workbench 走。
func TestAppReinstallsOnNewDocument(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	app, err := New(Spec{
		Title:  "ui-test",
		Width:  320,
		Height: 240,
		Menus:  []Menu{{Title: "文件", Items: []Action{Item("保存", "Ctrl+S", nil)}}},
		Status: "就绪",
	})
	if err != nil {
		t.Skipf("SDL 不可用: %v", err)
	}
	defer app.Close()

	first, err := app.LoadHTML(`<div>第一页</div>`)
	if err != nil {
		t.Fatalf("装载第一页失败: %v", err)
	}
	if app.Workbench() == nil || app.Workbench().Doc() != first {
		t.Fatal("第一页没有装工作台")
	}
	if first.QuerySelector(".system-ui-menubar") == nil {
		t.Fatal("第一页缺少菜单栏")
	}

	second, err := app.LoadHTML(`<div>第二页</div>`)
	if err != nil {
		t.Fatalf("装载第二页失败: %v", err)
	}
	if app.Workbench().Doc() != second {
		t.Fatal("换页后 Workbench 没跟着换文档")
	}
	if second.QuerySelector(".system-ui-menubar") == nil {
		t.Fatal("第二页没有重装菜单栏")
	}
	if !app.Workbench().HandleKey('s', engine.ModCtrl) {
		t.Fatal("换页后快捷键应当仍然生效")
	}
}

// TestChromeIgnoresPageTextAlign 页面 body 写 text-align:center（提示页、
// 落地页很常见）时，外壳三栏必须仍左对齐。
//
// 外壳是装进**页面文档**的普通元素，text-align 又是继承属性：不复位的话
// 工具栏整排按钮会跟着页面居中，看起来像「顶部漂到中间」。真实症状是
// 示例浏览器打开外部链接（提示页 body 居中）后工具栏按钮全跑到屏幕中间。
func TestChromeIgnoresPageTextAlign(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<!doctype html><html><body style="text-align:center"><div>内容</div></body></html>`)
	Install(doc, Spec{
		Menus:          []Menu{{Title: "文件", Items: []Action{Item("保存", "Ctrl+S", nil)}}},
		Tools:          []Action{Tool("保存", nil), Tool("打开", nil)},
		StatusSegments: []StatusSegment{{Key: "page", Text: "就绪"}},
	})
	doc.Refresh()

	// 菜单栏无内边距：首个菜单贴左缘。
	if b := boxOf(doc.QuerySelector(".system-ui-menu")); b.x != 0 {
		t.Errorf("菜单 x = %d, want 0（被页面 text-align 挤到中间了）", b.x)
	}
	// 工具栏左右各 8px 内边距：首个工具按钮落在 8。
	if b := boxOf(doc.QuerySelector(".system-ui-tool")); b.x != 8 {
		t.Errorf("工具按钮 x = %d, want 8（被页面 text-align 挤到中间了）", b.x)
	}
	// 状态栏左右各 12px 内边距：首段（is-first 收掉左内边距）落在 12。
	if b := boxOf(doc.QuerySelector(".system-ui-status-seg")); b.x != 12 {
		t.Errorf("状态段 x = %d, want 12（被页面 text-align 挤到中间了）", b.x)
	}
}
