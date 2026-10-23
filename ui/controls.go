// 本文件是页面基础控件：标签、文本视图、按钮、勾选、下拉与输入框。
//
// 控件是引擎原生元素上的薄包装：应用仍然用 AppendChild/OnClick 那一套，
// 只是少写类名、取值/设值/禁用态和事件适配。样式全部来自 theme.go。
package ui

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/go-comm/ghromex/engine"
)

// Control 是页面控件的最小公共接口。
//
// 控件本身不是新的布局系统：Element() 返回的仍是普通 engine.HTMLElement，
// 可以放进任何页面容器。
type Control interface {
	Element() engine.HTMLElement
}

var controlSeq uint64

// ensureID 给控件补一个文档内唯一的 id，供 label 的 for 关联使用。
func ensureID(el engine.HTMLElement) string {
	if el == nil {
		return ""
	}
	if id := el.ID(); id != "" {
		return id
	}
	id := fmt.Sprintf("system-ui-control-%d", atomic.AddUint64(&controlSeq, 1))
	el.SetID(id)
	return id
}

// LabelView 是单行静态标签，底层是 <label>。
type LabelView struct {
	el engine.HTMLElement
}

// NewLabel 创建标签。需要关联输入控件时调用 For。
func NewLabel(doc engine.HTMLDocument, text string) *LabelView {
	return &LabelView{el: newEl(doc, "label", "system-ui-label", text)}
}

func (v *LabelView) Element() engine.HTMLElement { return v.el }

// SetText 更新标签文字。
func (v *LabelView) SetText(text string) {
	if v != nil && v.el.Text() != text {
		v.el.SetText(text)
	}
}

// For 把标签关联到目标控件（设置 label 的 for 属性）。
func (v *LabelView) For(target engine.HTMLElement) *LabelView {
	if v == nil || target == nil {
		return v
	}
	if id := ensureID(target); id != "" {
		v.el.SetAttribute("for", id)
	}
	return v
}

// TextView 是只读多行文本视图，底层是 div。
//
// 它不参与焦点和编辑；需要可编辑多行输入时用 TextAreaView。TextView
// 用 white-space:pre 保留换行，高度由主题给默认值，应用可再改内联样式。
type TextView struct {
	el engine.HTMLElement
}

// NewTextView 创建只读多行文本视图。
func NewTextView(doc engine.HTMLDocument, text string) *TextView {
	return &TextView{el: newEl(doc, "div", "system-ui-textview", text)}
}

func (v *TextView) Element() engine.HTMLElement { return v.el }

// SetText 更新文本内容。
func (v *TextView) SetText(text string) {
	if v != nil && v.el.Text() != text {
		v.el.SetText(text)
	}
}

// ButtonView 是内容区按钮，底层是 <button type="button">。
//
// 工具栏的 ui.Button 仍是 Action 构造器；这里用 NewButton 区分。
type ButtonView struct {
	el      engine.HTMLElement
	onClick func()
	variant string
	enabled bool
}

// NewButton 创建按钮。onClick 可以为 nil。
func NewButton(doc engine.HTMLDocument, label string, onClick func()) *ButtonView {
	el := newEl(doc, "button", "system-ui-btn", label)
	el.SetAttribute("type", "button")
	v := &ButtonView{el: el, onClick: onClick, enabled: true}
	v.syncClass()
	el.OnClick(func(*engine.MouseEvent) {
		if v.enabled && v.onClick != nil {
			v.onClick()
		}
	})
	return v
}

func (v *ButtonView) Element() engine.HTMLElement { return v.el }

// SetLabel 更新按钮文字。
func (v *ButtonView) SetLabel(label string) {
	if v != nil && v.el.Text() != label {
		v.el.SetText(label)
	}
}

// SetEnabled 设置按钮可用态。禁用时同时写 disabled 属性和 is-disabled 类，
// 点击处理器也会再判一次，避免类名尚未刷新时被点到。
func (v *ButtonView) SetEnabled(on bool) {
	if v == nil || v.enabled == on {
		return
	}
	v.enabled = on
	if on {
		v.el.RemoveAttribute("disabled")
	} else {
		v.el.SetAttribute("disabled", "disabled")
	}
	v.syncClass()
}

// SetVariant 设置语义变体，如 "primary"、"danger"；也可以直接传
// "is-primary"。
func (v *ButtonView) SetVariant(name string) *ButtonView {
	if v == nil {
		return v
	}
	name = strings.TrimSpace(name)
	if name != "" && !strings.HasPrefix(name, "is-") {
		name = "is-" + name
	}
	v.variant = name
	v.syncClass()
	return v
}

func (v *ButtonView) syncClass() {
	class := "system-ui-btn"
	if v.variant != "" {
		class += " " + v.variant
	}
	if !v.enabled {
		class += " is-disabled"
	}
	setClass(v.el, class)
}

// CheckboxView 是原生 checkbox 加包裹式 label：点文字也能切换。
type CheckboxView struct {
	el       engine.HTMLElement
	input    engine.HTMLElement
	label    engine.HTMLElement
	enabled  bool
	onChange func(bool)
}

// NewCheckbox 创建勾选框。label 为空时只保留控件本身。
func NewCheckbox(doc engine.HTMLDocument, label string, checked bool) *CheckboxView {
	el := newEl(doc, "label", "system-ui-check", "")
	input := newEl(doc, "input", "system-ui-check-input", "")
	input.SetAttribute("type", "checkbox")
	text := newEl(doc, "span", "system-ui-check-label", label)
	el.AppendChild(input, text)
	if checked {
		input.SetAttribute("checked", "")
	}
	v := &CheckboxView{el: el, input: input, label: text, enabled: true}
	input.OnChange(func(*engine.MouseEvent) {
		if v.enabled && v.onChange != nil {
			v.onChange(engine.IsChecked(input))
		}
	})
	return v
}

func (v *CheckboxView) Element() engine.HTMLElement { return v.el }

// Checked 返回当前勾选状态。
func (v *CheckboxView) Checked() bool {
	if v == nil {
		return false
	}
	return engine.IsChecked(v.input)
}

// SetChecked 设置勾选状态；程序化赋值不触发 change。
func (v *CheckboxView) SetChecked(on bool) {
	if v != nil {
		engine.SetChecked(v.input, on)
	}
}

// SetLabel 更新勾选框文字。
func (v *CheckboxView) SetLabel(label string) {
	if v != nil && v.label.Text() != label {
		v.label.SetText(label)
	}
}

// OnChange 注册勾选变化回调；传 nil 移除。
func (v *CheckboxView) OnChange(f func(bool)) {
	if v != nil {
		v.onChange = f
	}
}

// SetEnabled 设置可用态。
func (v *CheckboxView) SetEnabled(on bool) {
	if v == nil || v.enabled == on {
		return
	}
	v.enabled = on
	if on {
		v.input.RemoveAttribute("disabled")
		removeClass(v.el, "is-disabled")
		return
	}
	v.input.SetAttribute("disabled", "disabled")
	addClass(v.el, "is-disabled")
}

// SelectOption 是下拉框的一项。
type SelectOption struct {
	Value string
	Label string
}

// SelectView 是原生 <select> 的薄包装。
type SelectView struct {
	doc            engine.HTMLDocument
	el             engine.HTMLElement
	enabled        bool
	suppressChange bool
	onChange       func(string)
}

// NewSelect 创建下拉框；初始选中项由浏览器/引擎默认（通常第一项），
// 需要指定时调用 SetValue。
func NewSelect(doc engine.HTMLDocument, opts []SelectOption) *SelectView {
	el := newEl(doc, "select", "system-ui-select", "")
	v := &SelectView{doc: doc, el: el, enabled: true}
	v.SetOptions(opts)
	el.OnChange(func(*engine.MouseEvent) {
		if v.enabled && !v.suppressChange && v.onChange != nil {
			v.onChange(engine.SelectValue(el))
		}
	})
	return v
}

func (v *SelectView) Element() engine.HTMLElement { return v.el }

// Value 返回当前选中项的值。
func (v *SelectView) Value() string {
	if v == nil {
		return ""
	}
	return engine.SelectValue(v.el)
}

// SetValue 按值选中一项；找不到时返回 false 并保持原值。
func (v *SelectView) SetValue(value string) bool {
	if v == nil {
		return false
	}
	v.suppressChange = true
	ok := engine.SetSelectValue(v.el, value)
	v.suppressChange = false
	return ok
}

// SetOptions 重建选项，尽量保留当前选中值。
func (v *SelectView) SetOptions(opts []SelectOption) {
	if v == nil || v.el == nil {
		return
	}
	current := engine.SelectValue(v.el)
	children := append([]engine.HTMLElement(nil), v.el.Children()...)
	v.el.RemoveChild(nil, children...)
	// 清掉旧的 value，避免新选项里没有旧值时仍留着失效的选择。
	v.el.RemoveAttribute("value")
	for _, opt := range opts {
		o := newEl(v.doc, "option", "", opt.Label)
		if opt.Value != "" {
			o.SetAttribute("value", opt.Value)
		}
		v.el.AppendChild(o)
	}
	if current != "" {
		v.suppressChange = true
		engine.SetSelectValue(v.el, current)
		v.suppressChange = false
	}
}

// OnChange 注册选中项变化回调；传 nil 移除。
func (v *SelectView) OnChange(f func(string)) {
	if v != nil {
		v.onChange = f
	}
}

// SetEnabled 设置可用态。
func (v *SelectView) SetEnabled(on bool) {
	if v == nil || v.enabled == on {
		return
	}
	v.enabled = on
	if on {
		v.el.RemoveAttribute("disabled")
		return
	}
	v.el.SetAttribute("disabled", "disabled")
}

// TextInputView 是单行输入框。
//
// 引擎目前没有 input/change 事件，实时联动请用 OnKeyDown，或在命令执行时
// 读取 Value。
type TextInputView struct {
	el      engine.HTMLElement
	enabled bool
}

// NewTextInput 创建单行输入框。
func NewTextInput(doc engine.HTMLDocument, value string) *TextInputView {
	el := newEl(doc, "input", "system-ui-input", "")
	el.SetAttribute("type", "text")
	v := &TextInputView{el: el, enabled: true}
	if value != "" {
		engine.SetValue(el, value)
	}
	return v
}

func (v *TextInputView) Element() engine.HTMLElement { return v.el }

// Value 返回当前值。
func (v *TextInputView) Value() string {
	if v == nil {
		return ""
	}
	return engine.GetValue(v.el)
}

// SetValue 设置值，光标移到末尾。
func (v *TextInputView) SetValue(value string) {
	if v != nil {
		engine.SetValue(v.el, value)
	}
}

// SetEnabled 设置可用态。
func (v *TextInputView) SetEnabled(on bool) {
	if v == nil || v.enabled == on {
		return
	}
	v.enabled = on
	if on {
		v.el.RemoveAttribute("disabled")
		return
	}
	v.el.SetAttribute("disabled", "disabled")
}

// OnKeyDown 注册按键处理器；返回 true 表示消费。
func (v *TextInputView) OnKeyDown(f func(*engine.KeyEvent) bool) {
	if v != nil {
		v.el.OnKeyDown(f)
	}
}

// TextAreaView 是可编辑多行输入框。
type TextAreaView struct {
	el      engine.HTMLElement
	enabled bool
	variant string
}

// NewTextArea 创建可编辑多行输入框。
func NewTextArea(doc engine.HTMLDocument, value string) *TextAreaView {
	el := newEl(doc, "textarea", "system-ui-textarea", "")
	v := &TextAreaView{el: el, enabled: true}
	if value != "" {
		engine.SetValue(el, value)
	}
	return v
}

func (v *TextAreaView) Element() engine.HTMLElement { return v.el }

// SetVariant 设置外观变体，如 "plain"（贴边铺满：无描边、无圆角）；
// 也可以直接传 "is-plain"。传空串回到默认描边。
func (v *TextAreaView) SetVariant(name string) *TextAreaView {
	if v == nil {
		return v
	}
	name = strings.TrimSpace(name)
	if name != "" && !strings.HasPrefix(name, "is-") {
		name = "is-" + name
	}
	v.variant = name
	v.syncClass()
	return v
}

func (v *TextAreaView) syncClass() {
	class := "system-ui-textarea"
	if v.variant != "" {
		class += " " + v.variant
	}
	setClass(v.el, class)
}

// Value 返回当前值。
func (v *TextAreaView) Value() string {
	if v == nil {
		return ""
	}
	return engine.GetValue(v.el)
}

// SetValue 设置值，光标移到末尾。
func (v *TextAreaView) SetValue(value string) {
	if v != nil {
		engine.SetValue(v.el, value)
	}
}

// SetEnabled 设置可用态。
func (v *TextAreaView) SetEnabled(on bool) {
	if v == nil || v.enabled == on {
		return
	}
	v.enabled = on
	if on {
		v.el.RemoveAttribute("disabled")
		return
	}
	v.el.SetAttribute("disabled", "disabled")
}

// OnKeyDown 注册按键处理器；返回 true 表示消费。
func (v *TextAreaView) OnKeyDown(f func(*engine.KeyEvent) bool) {
	if v != nil {
		v.el.OnKeyDown(f)
	}
}

// FieldView 是「标签 + 控件 + 提示/错误」的纵向表单项。
type FieldView struct {
	el   engine.HTMLElement
	hint engine.HTMLElement
	err  engine.HTMLElement
}

// NewField 创建表单项。control 为 nil 时只生成标签与提示区。
func NewField(doc engine.HTMLDocument, label string, control Control) *FieldView {
	el := newEl(doc, "div", "system-ui-field", "")
	l := NewLabel(doc, label)
	el.AppendChild(l.Element())

	if control != nil && control.Element() != nil {
		el.AppendChild(control.Element())
		switch strings.ToLower(control.Element().TagName()) {
		case "input", "select", "textarea":
			l.For(control.Element())
		}
	}

	hint := newEl(doc, "div", "system-ui-field-hint", "")
	hint.SetAttribute("style", "display:none")
	err := newEl(doc, "div", "system-ui-field-error", "")
	err.SetAttribute("style", "display:none")
	el.AppendChild(hint, err)
	return &FieldView{el: el, hint: hint, err: err}
}

func (v *FieldView) Element() engine.HTMLElement { return v.el }

// SetHint 更新提示文字；空串隐藏提示行。
func (v *FieldView) SetHint(text string) {
	if v == nil {
		return
	}
	v.hint.SetText(text)
	if text == "" {
		v.hint.SetAttribute("style", "display:none")
		return
	}
	v.hint.RemoveAttribute("style")
}

// SetError 更新错误文字；空串隐藏错误行并去掉表单项的错误描边。
func (v *FieldView) SetError(text string) {
	if v == nil {
		return
	}
	v.err.SetText(text)
	if text == "" {
		v.err.SetAttribute("style", "display:none")
		removeClass(v.el, "is-error")
		return
	}
	v.err.RemoveAttribute("style")
	addClass(v.el, "is-error")
}

// BarView 是页面里的横向控件条：一组控件排成一行（查找替换栏、地址栏这类）。
//
// 它和外壳的工具栏不是一回事：工具栏是 fixed 在视口上的应用外壳，控件条属于
// 页面内容，摆在哪里由应用决定，所以 NewBar 不写定位。条内控件按 inline-block
// 排开（引擎没有 flex，ui 的主题排工具栏也是这个办法），窗口太窄放不下时会
// 从行尾掉到下一行——要 nowrap 的话在主题里加 white-space。
//
// 定位声明与可见性分开管：应用用 SetInline 写自己那份内联声明（例如
// "position:absolute;left:0;top:72px"），SetVisible 只在其上补 display，
// 两者不会互相冲掉。
type BarView struct {
	el      engine.HTMLElement
	inline  string
	visible bool
}

// NewBar 创建控件条，controls 依次从左到右排入（nil 跳过）。
func NewBar(doc engine.HTMLDocument, controls ...Control) *BarView {
	v := &BarView{el: newEl(doc, "div", "system-ui-bar", ""), visible: true}
	v.Append(controls...)
	return v
}

func (v *BarView) Element() engine.HTMLElement { return v.el }

// Append 追加控件，顺序即从左到右。
func (v *BarView) Append(controls ...Control) *BarView {
	if v == nil {
		return v
	}
	var first engine.HTMLElement
	var rest []engine.HTMLElement
	for _, c := range controls {
		if c == nil || c.Element() == nil {
			continue
		}
		if first == nil {
			first = c.Element()
			continue
		}
		rest = append(rest, c.Element())
	}
	if first != nil {
		v.el.AppendChild(first, rest...)
	}
	return v
}

// SetInline 设置应用自己那份内联声明（定位、宽度等）；传空串清掉。
// 显隐请用 SetVisible，直接往元素写 style 会被 SetVisible 覆盖。
func (v *BarView) SetInline(decls string) *BarView {
	if v == nil {
		return v
	}
	v.inline = strings.Trim(strings.TrimSpace(decls), ";")
	v.syncStyle()
	return v
}

// SetVisible 显示/隐藏控件条。隐藏写的是 display:none：引擎会连布局盒一起
// 清掉，条里的控件既看不见也点不到（与渲染层同口径）。
func (v *BarView) SetVisible(on bool) *BarView {
	if v == nil || v.visible == on {
		return v
	}
	v.visible = on
	v.syncStyle()
	return v
}

// Visible 报告控件条当前是否可见。
func (v *BarView) Visible() bool { return v != nil && v.visible }

func (v *BarView) syncStyle() {
	style := v.inline
	if !v.visible {
		if style != "" {
			style += ";"
		}
		style += "display:none"
	}
	switch {
	case style == "":
		v.el.RemoveAttribute("style")
	case v.el.GetAttribute("style") != style:
		v.el.SetAttribute("style", style)
	}
}
