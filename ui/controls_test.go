package ui

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// controlPage 装好主题并把控件挂到页面容器上。
func controlPage(t *testing.T) (engine.HTMLDocument, engine.HTMLElement) {
	t.Helper()
	doc := newTestDoc(t, 640, 480, `<div id="page"></div>`)
	Install(doc, Spec{})
	doc.Refresh()
	page := doc.GetElementById("page")
	if page == nil {
		t.Fatal("缺少页面容器")
	}
	return doc, page
}

func TestButtonControl(t *testing.T) {
	doc, page := controlPage(t)
	clicked := 0
	b := NewButton(doc, "保存", func() { clicked++ })
	page.AppendChild(b.Element())
	doc.Refresh()

	clickEl(t, doc, b.Element())
	if clicked != 1 {
		t.Fatalf("点击次数 = %d, want 1", clicked)
	}

	b.SetEnabled(false)
	doc.Refresh()
	clickEl(t, doc, b.Element())
	if clicked != 1 {
		t.Fatalf("禁用后点击次数 = %d, want 1", clicked)
	}
	if got := b.Element().GetAttribute("disabled"); got == "" {
		t.Fatal("禁用后缺少 disabled 属性")
	}

	b.SetEnabled(true)
	doc.Refresh()
	clickEl(t, doc, b.Element())
	if clicked != 2 {
		t.Fatalf("重新启用后点击次数 = %d, want 2", clicked)
	}
	if got := b.Element().GetAttribute("disabled"); got != "" {
		t.Fatalf("重新启用后 disabled = %q, want 空", got)
	}

	b.SetLabel("另存为")
	b.SetVariant("primary")
	if got := b.Element().Text(); got != "另存为" {
		t.Fatalf("按钮文字 = %q, want 另存为", got)
	}
	if !strings.Contains(b.Element().GetAttribute("class"), "is-primary") {
		t.Fatalf("按钮 class = %q, want 含 is-primary", b.Element().GetAttribute("class"))
	}
}

func TestCheckboxControl(t *testing.T) {
	doc, page := controlPage(t)
	var changes []bool
	c := NewCheckbox(doc, "自动保存", false)
	c.OnChange(func(on bool) { changes = append(changes, on) })
	page.AppendChild(c.Element())
	doc.Refresh()

	if c.Checked() {
		t.Fatal("初始状态应为未选中")
	}
	clickEl(t, doc, c.Element())
	if !c.Checked() {
		t.Fatal("点击后应选中")
	}
	if len(changes) != 1 || !changes[0] {
		t.Fatalf("change 回调 = %v, want [true]", changes)
	}

	c.SetChecked(false)
	if c.Checked() {
		t.Fatal("SetChecked(false) 后仍选中")
	}
	clickEl(t, doc, c.Element())
	if !c.Checked() {
		t.Fatal("再次点击后应选中")
	}
	if len(changes) != 2 || !changes[1] {
		t.Fatalf("change 回调 = %v, want [true true]", changes)
	}

	c.SetLabel("保存时自动备份")
	if got := c.Element().InnerText(); !strings.Contains(got, "保存时自动备份") {
		t.Fatalf("勾选框文字 = %q, want 含新标签", got)
	}
}

func TestSelectControl(t *testing.T) {
	doc, page := controlPage(t)
	s := NewSelect(doc, []SelectOption{
		{Value: "a", Label: "甲"},
		{Value: "b", Label: "乙"},
	})
	page.AppendChild(s.Element())
	doc.Refresh()

	if got := s.Value(); got != "a" {
		t.Fatalf("初始值 = %q, want a", got)
	}
	if !s.SetValue("b") {
		t.Fatal("SetValue(b) 应成功")
	}
	if got := s.Value(); got != "b" {
		t.Fatalf("SetValue 后 = %q, want b", got)
	}

	var changed string
	s.OnChange(func(value string) { changed = value })
	// 程序化 SetValue 不应触发用户 change 回调。
	if !s.SetValue("a") {
		t.Fatal("SetValue(a) 应成功")
	}
	if changed != "" {
		t.Fatalf("程序化 SetValue 触发了 change: %q", changed)
	}
	// 引擎直接改值会派发 change，验证 wrapper 的回调适配。
	if !engine.SetSelectValue(s.Element(), "b") {
		t.Fatal("engine.SetSelectValue(b) 应成功")
	}
	if changed != "b" {
		t.Fatalf("change 回调 = %q, want b", changed)
	}

	s.SetOptions([]SelectOption{
		{Value: "c", Label: "丙"},
		{Value: "d", Label: "丁"},
	})
	if got := s.Value(); got != "c" {
		t.Fatalf("重建选项后值 = %q, want c", got)
	}
	if !s.SetValue("d") {
		t.Fatal("SetValue(d) 应成功")
	}

	s.SetEnabled(false)
	if got := s.Element().GetAttribute("disabled"); got == "" {
		t.Fatal("禁用后缺少 disabled 属性")
	}
	s.SetEnabled(true)
	if got := s.Element().GetAttribute("disabled"); got != "" {
		t.Fatalf("启用后 disabled = %q, want 空", got)
	}
}

func TestTextInputAndTextAreaControls(t *testing.T) {
	doc, page := controlPage(t)

	in := NewTextInput(doc, "abc")
	page.AppendChild(in.Element())
	doc.Refresh()
	if got := in.Value(); got != "abc" {
		t.Fatalf("输入框初始值 = %q, want abc", got)
	}
	in.SetValue("xyz")
	if got := in.Value(); got != "xyz" {
		t.Fatalf("输入框设置后 = %q, want xyz", got)
	}
	in.SetEnabled(false)
	if got := in.Element().GetAttribute("disabled"); got == "" {
		t.Fatal("输入框禁用后缺少 disabled 属性")
	}
	in.SetEnabled(true)
	if got := in.Element().GetAttribute("disabled"); got != "" {
		t.Fatalf("输入框启用后 disabled = %q, want 空", got)
	}

	ta := NewTextArea(doc, "第一行\n第二行")
	page.AppendChild(ta.Element())
	doc.Refresh()
	if got := ta.Value(); got != "第一行\n第二行" {
		t.Fatalf("多行输入初始值 = %q", got)
	}
	ta.SetValue("新内容")
	if got := ta.Value(); got != "新内容" {
		t.Fatalf("多行输入设置后 = %q, want 新内容", got)
	}
}

func TestLabelTextViewAndField(t *testing.T) {
	doc, page := controlPage(t)

	tv := NewTextView(doc, "第一行\n第二行")
	page.AppendChild(tv.Element())
	doc.Refresh()
	if got := tv.Element().Text(); got != "第一行\n第二行" {
		t.Fatalf("TextView 文本 = %q", got)
	}
	if got := boxOf(tv.Element()).h; got != 140 {
		t.Fatalf("TextView 高度 = %d, want 140", got)
	}
	tv.SetText("更新")
	if got := tv.Element().Text(); got != "更新" {
		t.Fatalf("TextView 更新后 = %q", got)
	}

	in := NewTextInput(doc, "x")
	field := NewField(doc, "文件名称", in)
	page.AppendChild(field.Element())
	doc.Refresh()

	label := field.Element().QuerySelector("label")
	if label == nil {
		t.Fatal("表单项缺少标签")
	}
	if got, want := label.GetAttribute("for"), in.Element().ID(); got != want || got == "" {
		t.Fatalf("标签 for = %q, want %q", got, want)
	}

	field.SetHint("最多 255 个字符")
	if got := field.Element().QuerySelector(".system-ui-field-hint").Text(); got != "最多 255 个字符" {
		t.Fatalf("提示文字 = %q", got)
	}
	field.SetError("文件名不能为空")
	if got := field.Element().QuerySelector(".system-ui-field-error").Text(); got != "文件名不能为空" {
		t.Fatalf("错误文字 = %q", got)
	}
	if !strings.Contains(field.Element().GetAttribute("class"), "is-error") {
		t.Fatalf("错误态 class = %q", field.Element().GetAttribute("class"))
	}
	field.SetError("")
	if strings.Contains(field.Element().GetAttribute("class"), "is-error") {
		t.Fatalf("清除错误后 class = %q", field.Element().GetAttribute("class"))
	}
}

func TestStatusSegments(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{
		StatusSegments: []StatusSegment{
			{Key: "file", Text: "未命名", Width: 220},
			{Key: "pos", Text: "行 1，列 1", Width: 140, Align: engine.TextAlignRight},
			{Key: "chars", Text: "0 个字符"},
		},
		StatusRight: "就绪",
	})
	doc.Refresh()

	segs := doc.GetElementsByClassName("system-ui-status-seg")
	if len(segs) != 3 {
		t.Fatalf("状态栏段数 = %d, want 3", len(segs))
	}
	if got := segs[0].Text(); got != "未命名" {
		t.Fatalf("第一段 = %q, want 未命名", got)
	}
	if got := segs[1].Text(); got != "行 1，列 1" {
		t.Fatalf("第二段 = %q", got)
	}
	if !strings.Contains(segs[0].GetAttribute("style"), "width:220px") {
		t.Fatalf("第一段样式 = %q, want 固定宽 220px", segs[0].GetAttribute("style"))
	}
	if !strings.Contains(segs[1].GetAttribute("style"), "text-align:right") {
		t.Fatalf("第二段样式 = %q, want 右对齐", segs[1].GetAttribute("style"))
	}
	if got := len(doc.GetElementsByClassName("system-ui-status-sep")); got != 2 {
		t.Fatalf("分隔线数 = %d, want 2", got)
	}
	if !hasWord(doc.QuerySelector(".system-ui-status").GetAttribute("class"), "has-right") {
		t.Fatal("有右侧消息时应带 has-right 类")
	}

	if !sh.SetStatusSegment("pos", "行 2，列 3") {
		t.Fatal("SetStatusSegment(pos) 应找到段")
	}
	if got, ok := sh.StatusSegment("pos"); !ok || got != "行 2，列 3" {
		t.Fatalf("StatusSegment(pos) = %q, %v", got, ok)
	}
	if sh.SetStatusSegment("missing", "x") {
		t.Fatal("不存在的 Key 不应返回 true")
	}

	sh.SetStatusRight("")
	if hasWord(doc.QuerySelector(".system-ui-status").GetAttribute("class"), "has-right") {
		t.Fatal("右侧消息为空时应移除 has-right")
	}
	sh.SetStatusRight("完成")
	if !hasWord(doc.QuerySelector(".system-ui-status").GetAttribute("class"), "has-right") {
		t.Fatal("重新设置右侧消息后应恢复 has-right")
	}

	// Hidden 段不生成，也不留下多余的分隔线；自适应段不写固定宽度。
	sh.SetStatusSegments([]StatusSegment{
		{Key: "a", Text: "A", Width: 80},
		{Key: "b", Text: "B", Hidden: true},
		{Key: "c", Text: "C"},
	})
	doc.Refresh()
	segs = doc.GetElementsByClassName("system-ui-status-seg")
	if len(segs) != 2 || segs[0].Text() != "A" || segs[1].Text() != "C" {
		t.Fatalf("隐藏段后 = %v, want [A C]", textsOf(segs))
	}
	if got := len(doc.GetElementsByClassName("system-ui-status-sep")); got != 1 {
		t.Fatalf("隐藏段后分隔线数 = %d, want 1", got)
	}
	if got := segs[1].GetAttribute("style"); strings.Contains(got, "width:") {
		t.Fatalf("自适应段不应有固定宽度: %q", got)
	}
}

func TestStatusLegacyAPI(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	sh := Install(doc, Spec{Status: "就绪"})
	doc.Refresh()

	if got := sh.statusLeft.Text(); got != "就绪" {
		t.Fatalf("旧状态栏文本 = %q, want 就绪", got)
	}
	sh.SetStatus("新消息")
	if got := sh.statusLeft.Text(); got != "新消息" {
		t.Fatalf("SetStatus 后 = %q, want 新消息", got)
	}

	sh.SetStatusSegments([]StatusSegment{
		{Key: "a", Text: "A"},
		{Key: "b", Text: "B"},
	})
	sh.SetStatus("X")
	segs := doc.GetElementsByClassName("system-ui-status-seg")
	if len(segs) != 2 || segs[0].Text() != "X" || segs[1].Text() != "B" {
		t.Fatalf("自定义分段后 SetStatus 结果 = %v", textsOf(segs))
	}
}

func textsOf(els []engine.HTMLElement) []string {
	out := make([]string, len(els))
	for i, el := range els {
		out[i] = el.Text()
	}
	return out
}

// 控件条：横向排成一行、定位与显隐分开管，条里的控件照样能点。
func TestBarControl(t *testing.T) {
	doc, page := controlPage(t)

	clicked := 0
	find := NewTextInput(doc, "")
	btn := NewButton(doc, "查找", func() { clicked++ })
	bar := NewBar(doc, find, btn).SetInline("position:absolute;left:0;top:40px")
	page.AppendChild(bar.Element())
	doc.Refresh()

	if got := bar.Element().GetAttribute("class"); !strings.Contains(got, "system-ui-bar") {
		t.Fatalf("控件条 class = %q，期望含 system-ui-bar", got)
	}
	if got := len(bar.Element().Children()); got != 2 {
		t.Fatalf("控件条子元素数 = %d, want 2", got)
	}

	// 可见时按钮点得到，并记下它的中心点，收起后在同一处再点一次。
	r := bar.Element().GetBoundingClientRect()
	ra := find.Element().GetBoundingClientRect()
	rb := btn.Element().GetBoundingClientRect()
	cx := (rb.Left().Pixel() + rb.Right().Pixel()) / 2
	cy := (rb.Top().Pixel() + rb.Bottom().Pixel()) / 2
	clickAt(t, doc, cx, cy)
	if clicked != 1 {
		t.Fatalf("点击次数 = %d, want 1（条里的按钮该能点着）", clicked)
	}

	bar.SetVisible(false)
	doc.Refresh()
	// 收起：只补 display:none，应用那份定位声明得留着；隐藏后不能还能点。
	style := bar.Element().GetAttribute("style")
	if !strings.Contains(style, "display:none") || !strings.Contains(style, "top:40px") {
		t.Fatalf("收起时样式 = %q，期望同时有 display:none 与 top:40px", style)
	}
	// 收起后再点同一处：既要命中不到，也不能触发动作。这里不走 clickAt
	// （它对「没命中任何元素」直接 fatal），而是直接用命中查询与派发入口。
	if hit := engine.ElementAt(doc, cx, cy); hit == btn.Element() {
		t.Fatal("隐藏的控件条里的按钮仍被命中，display:none 没挡住命中测试")
	}
	engine.OnDocumentClick(doc, cx, cy)
	doc.Refresh()
	if clicked != 1 {
		t.Fatalf("隐藏的控件条里的按钮仍可被点击（又点了 %d 次）", clicked-1)
	}

	bar.SetVisible(true)
	doc.Refresh()
	style = bar.Element().GetAttribute("style")
	if strings.Contains(style, "display:none") || !strings.Contains(style, "top:40px") {
		t.Fatalf("展开时样式 = %q，期望去掉 display:none 并保留 top", style)
	}
	clickAt(t, doc, cx, cy)
	if clicked != 2 {
		t.Fatalf("点击次数 = %d, want 2（重新展开后该又能点）", clicked)
	}

	// 两个控件排在同一行（inline-block），水平不重叠；垂直靠条的内边距
	// 居中（内边距 = (BarHeight - ButtonHeight) / 2，控件不写坐标）。
	if rb.Left().Pixel() <= ra.Left().Pixel() {
		t.Fatalf("控件没有横向排开：输入框左 %d，按钮左 %d",
			ra.Left().Pixel(), rb.Left().Pixel())
	}
	th := Light()
	down := (th.BarHeight - th.ButtonHeight) / 2
	for _, c := range []struct {
		name   string
		top    int
		bottom int
	}{{"输入框", ra.Top().Pixel(), ra.Bottom().Pixel()}, {"按钮", rb.Top().Pixel(), rb.Bottom().Pixel()}} {
		if c.top < r.Top().Pixel() || c.bottom > r.Bottom().Pixel() {
			t.Fatalf("%s 竖直方向超出控件条：盒 %d~%d，条 %d~%d",
				c.name, c.top, c.bottom, r.Top().Pixel(), r.Bottom().Pixel())
		}
		if got := c.top - r.Top().Pixel(); got != down {
			t.Fatalf("%s 距条上沿 %d px, want %d px（条内垂直居中）", c.name, got, down)
		}
	}
	// 空条不占位：不传控件、或传 nil 接口，都不该往条里塞元素。
	if got := len(NewBar(doc).Element().Children()); got != 0 {
		t.Fatalf("空控件条子元素数 = %d, want 0", got)
	}
	var none Control
	if got := len(NewBar(doc, none).Element().Children()); got != 0 {
		t.Fatalf("传 nil 控件后子元素数 = %d, want 0", got)
	}
}

// 平铺变体：贴边铺满的编辑区不带描边与圆角（class 由 SetVariant 维护）。
func TestTextAreaPlainVariant(t *testing.T) {
	doc, page := controlPage(t)

	ta := NewTextArea(doc, "x")
	page.AppendChild(ta.Element())
	ta.SetVariant("plain")
	doc.Refresh()
	if got := ta.Element().GetAttribute("class"); got != "system-ui-textarea is-plain" {
		t.Fatalf("class = %q, want %q", got, "system-ui-textarea is-plain")
	}
	ta.SetVariant("is-plain") // 传全名也认，且不重复追加
	if got := ta.Element().GetAttribute("class"); got != "system-ui-textarea is-plain" {
		t.Fatalf("重复设置变体后 class = %q", got)
	}
	ta.SetVariant("")
	if got := ta.Element().GetAttribute("class"); got != "system-ui-textarea" {
		t.Fatalf("清空变体后 class = %q, want system-ui-textarea", got)
	}
}
