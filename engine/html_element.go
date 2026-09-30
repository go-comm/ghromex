package engine

import (
	"io"
	"sort"
	"strings"
)

// 事件类型常量
const (
	EventClick  = "click"
	EventChange = "change"
	EventWheel  = "wheel"
)

// MouseEvent 描述一次鼠标事件，坐标为文档视口坐标。
// DeltaX/DeltaY 仅在 wheel 事件上有意义（像素位移，正向 = 内容向左/向上滚，
// 即视口向下/向右移，与浏览器 WheelEvent.deltaX/Y 同号）。
type MouseEvent struct {
	Type    string
	X       int
	Y       int
	DeltaX  int
	DeltaY  int
	Element HTMLElement
}

// HTMLElement 是 DOM 元素节点的基础接口。
// 扩展新标签时嵌入 HTMLElement，并用 newElement(tag) 初始化内部节点，
// 同时覆盖 AppendChild/RemoveChild/Children/ParentElement/setParentElement
// 以保证包装器正确维护父子链（参考 html_elements.go 中的 div/span）。
type HTMLElement interface {
	ELement

	TagName() string
	String() string

	Style() CSSStyleDeclaration
	SetStyle(style interface{})

	AppendChild(e HTMLElement, es ...HTMLElement)
	RemoveChild(e HTMLElement, es ...HTMLElement)
	Children() []HTMLElement
	ParentElement() HTMLElement
	setParentElement(parent HTMLElement)
	Clone() HTMLElement

	baseElement() *htmlElement

	GetElementsByClassName(name string) []HTMLElement
	GetElementsByTagName(name string) []HTMLElement

	QuerySelector(query string) HTMLElement
	QuerySelectorAll(query string) []HTMLElement

	// GetBoundingClientRect 返回布局完成后的边框盒（不含 margin）。
	GetBoundingClientRect() Rect

	X() int
	setX(x int)
	Y() int
	setY(y int)

	Width() int
	setWidth(w int)
	Height() int
	setHeight(h int)

	Text() string
	SetText(text string)
	InnerText() string
	SetInnerText(text string)
	InnerHTML() string
	SetInnerHTML(html string)

	// OnClick 注册点击事件处理器，f 为 nil 时移除。
	OnClick(f func(ev *MouseEvent))
	// OnChange 注册值变更事件处理器（select 选中项变化等），f 为 nil 时移除。
	OnChange(f func(ev *MouseEvent))
	// OnWheel 注册滚轮事件处理器（先于滚动发生派发），f 为 nil 时移除。
	OnWheel(f func(ev *MouseEvent))

	getPaint() Paint
	setPaint(p Paint)

	writeToJSON(w io.StringWriter, prefix string, indent bool, walk string)
	EncodeJSON(w io.StringWriter, prefix string, indent bool)
	FormatJSON(prefix string, indent bool) string
}

func newElement(tag string) HTMLElement {
	e := &htmlElement{}
	e.self = e
	e.tagName = tag
	e.attrs = make(map[string]string)
	e.style = newStyle()
	return e
}

type htmlElement struct {
	self          HTMLElement // 包装器场景下指向外层对象，保证父链/克隆正确
	tagName       string
	id            string
	text          string
	width         int
	height        int
	x             int
	y             int
	layoutH       int // 布局流占位高度（含 margin），由布局引擎写入
	attrs         map[string]string
	style         CSSStyleDeclaration
	inline        []simpleDecl
	computed      CSSStyleDeclaration
	runs          []textRun
	handlers      map[string]func(*MouseEvent)
	parentElement HTMLElement
	children      []HTMLElement
	transformX    Size
	transformY    Size
	paint         Paint
	// open 为 select 的展开态（下拉浮层是否显示）。
	open bool
	// caret 为聚焦可编辑控件的插入点，单位是 value 的 rune 下标。
	caret int
	// scrollTop 为纵向滚动偏移（像素）。textarea 由光标跟随/滚轮驱动，
	// overflow 滚动容器与根元素（文档级滚动）由 applyScrollOffsets 施加。
	scrollTop int
	// scrollLeft 为横向滚动偏移（像素），语义同 scrollTop。
	scrollLeft int
	// contentW/contentH 为最近一次布局测得的未裁剪内容尺寸（常规流子
	// 的外盒延伸量），是滚动偏移夹紧的依据；textarea 等原子控件为 0
	//（其内容量度走各自专用路径）。
	contentW int
	contentH int
}

func (element *htmlElement) TagName() string {
	return element.tagName
}

func (element *htmlElement) SetAttribute(key string, value string) {
	if element.attrs == nil {
		element.attrs = make(map[string]string)
	}
	element.attrs[key] = value
	switch key {
	case "id":
		element.id = value
	case "style":
		element.inline = ParseInlineStyle(value)
	}
	element.onChanged()
}

func (element *htmlElement) GetAttribute(key string) string {
	return element.attrs[key]
}

func (element *htmlElement) ID() string {
	if element.id == "" {
		element.id = element.attrs["id"]
	}
	return element.id
}

func (element *htmlElement) SetID(id string) {
	element.SetAttribute("id", id)
}

func (element *htmlElement) Class() string {
	return element.attrs["class"]
}

func (element *htmlElement) Style() CSSStyleDeclaration {
	return element.style
}

func (element *htmlElement) SetStyle(style interface{}) {
	// 预留：接受 CSSStyleDeclaration 或内联样式字符串
	switch s := style.(type) {
	case CSSStyleDeclaration:
		element.style = s
	case string:
		element.SetAttribute("style", s)
	}
}

func (element *htmlElement) AppendChild(e HTMLElement, es ...HTMLElement) {
	if e != nil {
		element.children = append(element.children, e)
		e.setParentElement(element.self)
	}
	for _, child := range es {
		if child != nil {
			element.children = append(element.children, child)
			child.setParentElement(element.self)
		}
	}
	element.onChanged()
}

func (element *htmlElement) RemoveChild(e HTMLElement, es ...HTMLElement) {
	for _, victim := range append([]HTMLElement{e}, es...) {
		if victim == nil {
			continue
		}
		for i, child := range element.children {
			if child == victim {
				element.children = append(element.children[:i], element.children[i+1:]...)
				break
			}
		}
	}
	element.onChanged()
}

func (element *htmlElement) Children() []HTMLElement {
	return element.children
}

func (element *htmlElement) ParentElement() HTMLElement {
	return element.parentElement
}

func (element *htmlElement) setParentElement(parent HTMLElement) {
	element.parentElement = parent
}

func (element *htmlElement) baseElement() *htmlElement {
	return element
}

func (element *htmlElement) Width() int {
	return element.width
}

func (element *htmlElement) setWidth(w int) {
	element.width = w
}

func (element *htmlElement) Height() int {
	return element.height
}

func (element *htmlElement) setHeight(h int) {
	element.height = h
}

func (element *htmlElement) X() int {
	return element.x
}

func (element *htmlElement) setX(x int) {
	element.x = x
}

func (element *htmlElement) Y() int {
	return element.y
}

func (element *htmlElement) setY(y int) {
	element.y = y
}

func (element *htmlElement) Text() string {
	return element.text
}

// SetText 用单一文本子节点替换全部子节点。
func (element *htmlElement) SetText(text string) {
	element.children = nil
	element.text = text
	if text != "" {
		tn := NewTextNode(text)
		element.children = []HTMLElement{tn}
		tn.setParentElement(element.self)
	}
	element.onChanged()
}

func (element *htmlElement) InnerText() string {
	var b strings.Builder
	collectInnerText(element.self, &b)
	return b.String()
}

func collectInnerText(e HTMLElement, b *strings.Builder) {
	if tn, ok := e.(*textNode); ok {
		b.WriteString(tn.Text())
		return
	}
	for _, child := range e.Children() {
		collectInnerText(child, b)
	}
}

func (element *htmlElement) SetInnerText(text string) {
	element.SetText(text)
}

func (element *htmlElement) InnerHTML() string {
	// TODO: 序列化子树为 HTML 标记文本，当前退化为 JSON
	var b strings.Builder
	for _, child := range element.children {
		b.WriteString(child.String())
	}
	return b.String()
}

func (element *htmlElement) SetInnerHTML(html string) {
	element.children = nil
	if element.tagName == "html" {
		if doc, ok := element.self.(HTMLDocument); ok {
			ParseHTMLInto(doc, html)
			return
		}
	}
	root := ParseFragment(html)
	for _, child := range root.Children() {
		element.AppendChild(child)
	}
	element.onChanged()
}

// OnClick 注册/移除点击处理器。
func (element *htmlElement) OnClick(f func(ev *MouseEvent)) {
	if f == nil {
		delete(element.handlers, EventClick)
		return
	}
	if element.handlers == nil {
		element.handlers = make(map[string]func(*MouseEvent))
	}
	element.handlers[EventClick] = f
}

// OnChange 注册/移除值变更处理器（当前用于 select 选中项变化）。
func (element *htmlElement) OnChange(f func(ev *MouseEvent)) {
	if f == nil {
		delete(element.handlers, EventChange)
		return
	}
	if element.handlers == nil {
		element.handlers = make(map[string]func(*MouseEvent))
	}
	element.handlers[EventChange] = f
}

// OnWheel 注册/移除滚轮事件处理器（OnDocumentWheel 在滚动前冒泡派发）。
func (element *htmlElement) OnWheel(f func(ev *MouseEvent)) {
	if f == nil {
		delete(element.handlers, EventWheel)
		return
	}
	if element.handlers == nil {
		element.handlers = make(map[string]func(*MouseEvent))
	}
	element.handlers[EventWheel] = f
}

func (element *htmlElement) dispatch(typ string, ev *MouseEvent) bool {
	if f, ok := element.handlers[typ]; ok && f != nil {
		ev.Element = element.self
		f(ev)
		return true
	}
	return false
}

func (element *htmlElement) onChanged() {
	// 沿父链找到最外层文档，通知内容变化（用于触发重新布局/重绘）
	e := element.self
	for e != nil {
		if d, ok := e.(interface{ markChanged() }); ok {
			d.markChanged()
			return
		}
		e = e.ParentElement()
	}
}

// GetBoundingClientRect 返回边框盒。
func (element *htmlElement) GetBoundingClientRect() Rect {
	comp := element.computed
	l := element.x
	t := element.y
	w := element.width
	h := element.height
	if comp != nil {
		b := comp.BorderStyleWidth()
		p := comp.Padding()
		l -= b.Left().Pixel() + p.Left().Pixel()
		t -= b.Top().Pixel() + p.Top().Pixel()
		w += p.Left().Pixel() + p.Right().Pixel() + b.Left().Pixel() + b.Right().Pixel()
		h += p.Top().Pixel() + p.Bottom().Pixel() + b.Top().Pixel() + b.Bottom().Pixel()
	}
	return NewRect(NewSize(SIZE_PIXEL, l, 0), NewSize(SIZE_PIXEL, l+w, 0),
		NewSize(SIZE_PIXEL, t, 0), NewSize(SIZE_PIXEL, t+h, 0))
}

// Clone 复制元素（含属性），不复制子节点，样式对象为全新实例。
func (element *htmlElement) Clone() HTMLElement {
	c := *element
	c.attrs = make(map[string]string, len(element.attrs))
	for k, v := range element.attrs {
		c.attrs[k] = v
	}
	c.style = newStyle()
	c.children = nil
	c.parentElement = nil
	c.runs = nil
	c.computed = nil
	c.handlers = nil
	c.inline = nil
	c.paint = element.paint
	c.open = false
	c.caret = 0
	c.scrollTop = 0
	c.scrollLeft = 0
	c.contentW = 0
	c.contentH = 0
	c.self = &c
	return &c
}

func (element *htmlElement) getPaint() Paint {
	p := element.paint
	if p == nil && element.parentElement != nil {
		return element.parentElement.getPaint()
	}
	return p
}

func (element *htmlElement) setPaint(p Paint) {
	element.paint = p
}

// GetElementsByClassName 深度优先收集 class 属性包含指定名称的后代元素。
func (element *htmlElement) GetElementsByClassName(name string) []HTMLElement {
	var out []HTMLElement
	for _, child := range element.children {
		if hasClass(child, name) {
			out = append(out, child)
		}
		out = append(out, child.GetElementsByClassName(name)...)
	}
	return out
}

func (element *htmlElement) GetElementsByTagName(name string) []HTMLElement {
	var out []HTMLElement
	for _, child := range element.children {
		if strings.EqualFold(child.TagName(), name) {
			out = append(out, child)
		}
		out = append(out, child.GetElementsByTagName(name)...)
	}
	return out
}

func hasClass(e HTMLElement, name string) bool {
	return containsWord(e.GetAttribute("class"), name)
}

func containsWord(list, word string) bool {
	for _, p := range strings.Fields(list) {
		if p == word {
			return true
		}
	}
	return false
}

func (element *htmlElement) QuerySelector(query string) HTMLElement {
	all := element.querySelectorAll(query)
	if len(all) > 0 {
		return all[0]
	}
	return nil
}

func (element *htmlElement) QuerySelectorAll(query string) []HTMLElement {
	return element.querySelectorAll(query)
}

func (element *htmlElement) querySelectorAll(query string) []HTMLElement {
	sel := ParseSelector(query)
	var out []HTMLElement
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		for _, child := range e.Children() {
			if !sel.isEmpty() && matchSelectorChain(sel, child) {
				out = append(out, child)
			}
			walk(child)
		}
	}
	walk(element.self)
	return out
}

func (element htmlElement) writeToJSON(w io.StringWriter, prefix string, indent bool, walk string) {
	if indent {
		w.WriteString(walk)
	}
	w.WriteString("{")
	awalk := walk + prefix
	if indent {
		w.WriteString("\n")
		w.WriteString(awalk)
	}
	w.WriteString("\"tagName\":")
	if indent {
		w.WriteString(" ")
	}
	w.WriteString("\"")
	w.WriteString(element.tagName)
	w.WriteString("\"")

	if len(element.attrs) > 0 {
		w.WriteString(",")
		if indent {
			w.WriteString("\n")
			w.WriteString(awalk)
		}
		w.WriteString("\"attrs\":{")
		keys := make([]string, 0, len(element.attrs))
		for k := range element.attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				w.WriteString(",")
			}
			w.WriteString("\"")
			w.WriteString(k)
			w.WriteString("\":\"")
			w.WriteString(element.attrs[k])
			w.WriteString("\"")
		}
		w.WriteString("}")
	}

	if element.width > 0 || element.height > 0 || element.x != 0 || element.y != 0 {
		w.WriteString(",")
		if indent {
			w.WriteString("\n")
			w.WriteString(awalk)
		}
		w.WriteString("\"box\":\"")
		w.WriteString(itoa(element.x))
		w.WriteString(",")
		w.WriteString(itoa(element.y))
		w.WriteString(",")
		w.WriteString(itoa(element.width))
		w.WriteString(",")
		w.WriteString(itoa(element.height))
		w.WriteString("\"")
	}

	if len(element.text) > 0 {
		w.WriteString(",")
		if indent {
			w.WriteString("\n")
			w.WriteString(awalk)
		}
		w.WriteString("\"text\":")
		if indent {
			w.WriteString(" ")
		}
		w.WriteString("\"")
		w.WriteString(element.text)
		w.WriteString("\"")
	}

	children := element.children
	if len(children) > 0 {
		w.WriteString(",")
		if indent {
			w.WriteString("\n")
			w.WriteString(awalk)
		}
		w.WriteString("\"children\":[")
		{
			twalk := awalk + prefix
			if indent {
				w.WriteString("\n")
			}
			children[0].writeToJSON(w, prefix, indent, twalk)
			for _, child := range children[1:] {
				w.WriteString(",")
				if indent {
					w.WriteString("\n")
				}
				child.writeToJSON(w, prefix, indent, twalk)
			}
			if indent {
				w.WriteString("\n")
				w.WriteString(awalk)
			}
		}
		w.WriteString("]")
	}
	if indent {
		w.WriteString("\n")
		w.WriteString(walk)
	}
	w.WriteString("}")
}

func (element htmlElement) EncodeJSON(w io.StringWriter, prefix string, indent bool) {
	element.writeToJSON(w, prefix, indent, "")
}

func (element htmlElement) FormatJSON(prefix string, indent bool) string {
	var b strings.Builder
	element.EncodeJSON(&b, prefix, indent)
	return b.String()
}

func (element htmlElement) String() string {
	var b strings.Builder
	element.EncodeJSON(&b, "", false)
	return b.String()
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
