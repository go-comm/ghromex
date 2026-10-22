package engine

import (
	"io"
	"sort"
	"strings"
)

// 事件类型常量
const (
	EventClick   = "click"
	EventChange  = "change"
	EventWheel   = "wheel"
	EventKeyDown = "keydown"
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

// KeyEvent 描述一次按键事件。Key 为键码：ASCII 区对齐 SDL2 的
// SDL_keysym.sym（方向键/Home/End 等扫描码区见 text_input.go 的常量表），
// Mods 为修饰键位掩码（ModCtrl/ModShift，见 text_input.go）。
//
// keydown 派发给聚焦元素；处理器返回 true 表示"已消费"，引擎的默认动作
// （退格/删除/回车换行/方向键移动光标等）不再执行——等价于网页的
// preventDefault。应用层靠它把"回车 = 打开"这类行为接进来（浏览器地址栏、
// 记事本查找框都用得上），不必去改引擎的默认动作。
type KeyEvent struct {
	Type string
	Key  int
	Mods uint16
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
	// OnKeyDown 注册按键事件处理器，f 为 nil 时移除。处理器返回 true 表示
	// 已消费：引擎的默认动作不再执行（等价于网页的 preventDefault）。
	OnKeyDown(f func(ev *KeyEvent) bool)

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
	self     HTMLElement // 包装器场景下指向外层对象，保证父链/克隆正确
	tagName  string
	id       string
	text     string
	width    int
	height   int
	x        int
	y        int
	layoutH  int // 布局流占位高度（含 margin），由布局引擎写入
	attrs    map[string]string
	style    CSSStyleDeclaration
	inline   []simpleDecl
	computed CSSStyleDeclaration
	runs     []textRun
	handlers map[string]func(*MouseEvent)
	// keyHandler 是元素级 keydown 处理器。与 handlers 分开存：键事件的
	// 参数不同（要带键码与修饰键），且要能"消费"按键。
	keyHandler    func(ev *KeyEvent) bool
	parentElement HTMLElement
	children      []HTMLElement
	transformX    Size
	transformY    Size
	paint         Paint
	// open 为 select 的展开态（下拉浮层是否显示）。
	open bool
	// caret 为聚焦可编辑控件的插入点，单位是 value 的 rune 下标。
	caret int
	// selAnchor 是选区的固定端（锚点），与 caret（活动端）共同表示
	// [min(selAnchor,caret), max(...)) 的选区；两者相等即无选区。鼠标
	// 拖选时锚点不动、活动端跟随指针；键入/退格/方向键会把两者收敛，
	// 与浏览器「输入即替换选区、方向键折叠选区」一致。
	selAnchor int
	// caretInit 表示该控件的光标已被显式定位过（首次聚焦/点击落位）。
	// 再次聚焦时保留原光标与选区（失焦不丢编辑状态，见 SetFocusedElement），
	// 只有从未定位过的控件才把光标放到值末尾。
	caretInit bool
	// scrollTop 为纵向滚动偏移（像素）。textarea 由光标跟随/滚轮驱动，
	// overflow 滚动容器与根元素（文档级滚动）由 applyScrollOffsets 施加。
	scrollTop int
	// scrollLeft 为横向滚动偏移（像素），语义同 scrollTop。
	scrollLeft int
	// scrollAnimTop/scrollAnimLeft 是**平滑滚动的当前视觉位置**（整像素）：
	// 滚动目标在 scrollTop/scrollLeft，实际位移由每轮布局按时间推进（Chrome 的
	// 动画式滚动；引擎此前整档跳变，观感生硬）。scrollAnimOn 表示动画在途。
	scrollAnimTop, scrollAnimLeft int
	// scrollShiftTop/scrollShiftLeft 是**已施加到子树上的滚动平移量**。
	// 布局后按绝对偏移平移一次，动画推进时只按增量平移（见
	// applyScrollDeltas）——滚动动画帧因此不必重排。
	scrollShiftTop, scrollShiftLeft int
	// scrollAnimPosTop/PosLeft 是视觉位置的**连续值**（亚像素）：缓动的真值。
	// 渲染用的是整像素的 scrollAnimTop（布局按整像素平移子树）。
	scrollAnimPosTop, scrollAnimPosLeft float64
	// scrollAnimBaseTop/BaseLeft 是当前缓动段的**起点**：连续滚轮时新的一档
	// 从当前视觉位置重新起算，位置连续、不跳（见 scroll.go 的 startScrollAnim）。
	scrollAnimBaseTop, scrollAnimBaseLeft float64
	// scrollAnimElapsedTopNs/ElapsedLeftNs 是本段已推进的**动画时间**：
	// 进度 = 已推进时间 / scrollAnimDuration，与帧率无关（掉帧不会变速）。
	scrollAnimElapsedTopNs, scrollAnimElapsedLeftNs int64
	// scrollAnimLastTopNs/scrollAnimLastLeftNs 是上一帧的时间戳（用于算
	// 实际帧间隔）。**两轴各自一份**：wheel 事件会重置对应轴的时间戳，
	// 共用一份的话，一次高频 wheel 会把另一轴的帧间隔截断——触控板
	// momentum 时 wheel 比帧还密，另一轴的动画几乎停住。
	scrollAnimLastTopNs, scrollAnimLastLeftNs int64
	scrollAnimOn                              bool
	// contentW/contentH 为最近一次布局测得的未裁剪内容尺寸（常规流子
	// 的外盒延伸量），是滚动偏移夹紧的依据；textarea 等原子控件为 0
	//（其内容量度走各自专用路径）。
	contentW int
	contentH int
	// listMarker 是列表项的标记文本（"•" / "1." / "2."…）。引擎没有
	// list-item 概念，标记由布局期按父级 ul/ol 算出、绘制期画在内容左缘
	// 左侧（Chrome 口径：右对齐到内容左缘 −0.5em）。
	listMarker string
	// rsvPD/rsvBD 为布局期解析出的 padding/border 边长（百分比按包含块
	// 宽度解析）。绘制与命中必须复用同一份：百分比 padding 在布局期按
	// 百分比算、在绘制期若按 0 基数算，背景/边框就会画在错误的盒上，
	// 命中框也会与绘制框不符。hasRsv 为 false 表示尚未布局（display:none
	// 等），此时按 0 基数现算。
	rsvPD, rsvBD edges
	hasRsv       bool
	// mtAbsorbed/mbAbsorbed：父块的外边距与本块的上/下外边距折叠时置位
	//（CSS 2.1 §8.3.1 的父-子穿越折叠）。本块布局时据此把自身外边距记 0
	// ——折叠后的值已由父块计入，避免重复计算。读取后即清零（一次性状态，
	// 避免跨布局残留）。
	mtAbsorbed, mbAbsorbed bool
	// effTop/effBot：本块的外边距与首/末个流内块级子块折叠后的结果
	//（父块在排布子块时算出并回填，见 measure.go 的 layoutChildren）。
	// hasEff 为 false 表示未回填（根元素等），此时用样式里的原始 margin。
	effTop, effBot int
	hasEff         bool
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

// OnKeyDown 注册/移除按键处理器。
func (element *htmlElement) OnKeyDown(f func(ev *KeyEvent) bool) {
	element.keyHandler = f
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
		// 边长用布局期解析出的值（百分比按包含块宽度解析）——命中框必须与
		// 绘制出的边框盒是同一个盒；未布局时按 0 基数现算（resolveLen 对
		// nil 安全，不会像直接 .Pixel() 那样空指针崩溃）。
		pd, bd := resolvedEdges(element, comp)
		l -= pd[eLeft] + bd[eLeft]
		t -= pd[eTop] + bd[eTop]
		w += pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
		h += pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
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
	c.selAnchor = 0
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

// containsWord 判断空格分隔的词表里是否含 word（等价于 strings.Fields 后
// 逐个比较，但**不分配**）。
//
// 类名匹配在级联里是热路径：每个元素 × 每条规则 × 每个类选择器都要调一次，
// strings.Fields 的拆分与切片分配曾占整页重排耗时的四分之一（profile:
// strings.Fields 25.8%、containsWord 26.8%）。这里改为按字节扫描空白。
func containsWord(list, word string) bool {
	if word == "" {
		return false
	}
	i := 0
	for i < len(list) {
		for i < len(list) && isASCIISpace(list[i]) {
			i++
		}
		start := i
		for i < len(list) && !isASCIISpace(list[i]) {
			i++
		}
		if i > start && list[start:i] == word {
			return true
		}
	}
	return false
}

// isASCIISpace 判断是否为 CSS 词表分隔用的空白（ASCII 空白集）。
func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
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
	// 伪类匹配要查动态状态（hover/focus/active），故带上所属文档。
	doc := docOf(element.self)
	var out []HTMLElement
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		for _, child := range e.Children() {
			if !sel.isEmpty() && matchSelectorChain(doc, sel, child) {
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
