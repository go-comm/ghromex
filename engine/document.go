package engine

// HTMLDocument 是文档根节点，持有样式表与所属视口。
type HTMLDocument interface {
	HTMLElement

	CreateElement(tag string) HTMLElement
	GetElementById(id string) HTMLElement
	GetElementsByName(name string) []HTMLElement

	Body() HTMLBodyElement
	Head() HTMLHeadElement

	// Stylesheet 返回文档 <style> 编译出的样式表。
	Stylesheet() *Stylesheet
	// AddStylesheetText 解析并合并一段 CSS。
	AddStylesheetText(css string)
	SetStylesheet(sheet *Stylesheet)

	// Title 返回 head > title 的文本。
	Title() string

	Viewport() Viewport
	// Refresh 基于当前视口重新执行级联与布局。
	Refresh()
	// DumpSVG 将当前布局导出为 SVG 文本（调试/无头验证用）。
	DumpSVG() string

	// SetOnChanged 注册文档内容变化回调（布局失效通知，供后端重排）。
	SetOnChanged(f func())
	// SetOnNavigate 注册链接导航回调：点击带 href 的 <a> 时以 href 调用。
	// 引擎不内置多文档/外跳，跳转策略由应用决定；f 为 nil 时移除。
	SetOnNavigate(f func(href string))
	// ChangeCount 返回内容变化计数，后端据此判断是否需要重排。
	ChangeCount() int64

	Close()
}

func NewDocument() HTMLDocument {
	document := &htmlDocument{}
	document.HTMLElement = newElement("html")
	document.setPaintBase(NewPaint())
	document.head = newHeadElement()
	document.body = newBodyElement()

	// 根节点 self 指向文档包装器：子节点父链上溯到根后可命中 HTMLDocument，
	// 变更通知（markChanged）与事件冒泡依赖该链路。
	if root, ok := document.HTMLElement.(*htmlElement); ok {
		root.self = document
	}
	document.AppendChild(document.head, document.body)

	return document
}

type htmlDocument struct {
	HTMLElement

	viewport Viewport
	sheet    *Stylesheet
	head     HTMLHeadElement
	body     HTMLBodyElement
	focus    HTMLElement // 当前聚焦的元素（可编辑输入、按钮、链接等），nil 无焦点
	// hover 是鼠标当前悬停的元素（:hover 规则的依据），active 是鼠标
	// 按下中的元素（:active 规则）。两者变化都会改样式，故与 focus 一样
	// 走 markChanged → 重排重绘 → 级联重算。
	hover   HTMLElement
	active  HTMLElement
	// dragInput 是鼠标按下时命中的可编辑控件。拖选期间活动端可移出控件
	// 甚至窗口，仍应把坐标反推为同一控件的光标位置（浏览器同语义）；
	// 因此不能在每次 MouseMotion 时重新命中测试。鼠标抬起即清空。
	dragInput  *htmlElement
	changed    int64
	onChanged  func()
	onNavigate func(href string)
	// vpW/vpH 是上次布局时的视口尺寸。窗口缩放后后端只强制重排、并不
	// 改文档内容（onChanged 不触发），应用层无从得知窗口变了——而
	// 「编辑区铺满窗口剩余区域」这类 chrome 必须跟着窗口尺寸走。布局
	// 时比对尺寸，变化则补一次变更通知（见 LayoutDocument）。
	vpW, vpH int
}

func (document *htmlDocument) setPaintBase(p Paint) {
	if base := inner(document); base != nil {
		base.setPaint(p)
	}
}

func (document *htmlDocument) CreateElement(tag string) HTMLElement {
	e := SystemElements().Get(tag)
	if e == nil {
		e = CustomElements().Get(tag)
	}
	if e != nil {
		// CloneElement 对 typed nil 原型返回 nil（调 Clone 会空指针崩溃），
		// 此时退化为未知元素，不让创建流程崩掉。
		if c := CloneElement(e); c != nil {
			return c
		}
	}
	return newHTMLUnknownElement(tag)
}

func (document *htmlDocument) GetElementById(id string) HTMLElement {
	return findElementBy(document, func(e HTMLElement) bool {
		return e.GetAttribute("id") == id
	})
}

func (document *htmlDocument) GetElementsByName(name string) []HTMLElement {
	var out []HTMLElement
	for _, e := range findElementsBy(document, func(el HTMLElement) bool {
		return el.GetAttribute("name") == name
	}) {
		out = append(out, e)
	}
	return out
}

func findElementBy(root HTMLElement, pred func(HTMLElement) bool) HTMLElement {
	for _, child := range root.Children() {
		if pred(child) {
			return child
		}
		if found := findElementBy(child, pred); found != nil {
			return found
		}
	}
	return nil
}

func findElementsBy(root HTMLElement, pred func(HTMLElement) bool) []HTMLElement {
	var out []HTMLElement
	for _, child := range root.Children() {
		if pred(child) {
			out = append(out, child)
		}
		out = append(out, findElementsBy(child, pred)...)
	}
	return out
}

// docOf 沿父链找元素所属的文档：根元素的 self 指向文档包装器，普通
// 元素的 self 指向自身。找不到时返回 nil（接口 nil，调用方需判空）。
func docOf(e HTMLElement) HTMLDocument {
	for cur := e; cur != nil; cur = cur.ParentElement() {
		b := inner(cur)
		if b == nil {
			continue
		}
		if d, ok := b.self.(*htmlDocument); ok {
			return d
		}
	}
	return nil
}

func (document *htmlDocument) Head() HTMLHeadElement {
	return document.head
}

func (document *htmlDocument) Body() HTMLBodyElement {
	return document.body
}

func (document *htmlDocument) Stylesheet() *Stylesheet {
	return document.sheet
}

func (document *htmlDocument) AddStylesheetText(css string) {
	parsed := ParseCSS(css)
	if document.sheet == nil {
		document.sheet = &Stylesheet{}
	}
	document.sheet.Merge(parsed)
	document.markChanged()
}

func (document *htmlDocument) SetStylesheet(sheet *Stylesheet) {
	document.sheet = sheet
	document.markChanged()
}

func (document *htmlDocument) Title() string {
	if document.head == nil {
		return ""
	}
	t := document.head.Title()
	if t == nil {
		return ""
	}
	return t.Text()
}

func (document *htmlDocument) Viewport() Viewport {
	return document.viewport
}

func (document *htmlDocument) Refresh() {
	LayoutDocument(document)
}

func (document *htmlDocument) SetOnChanged(f func()) {
	document.onChanged = f
}

func (document *htmlDocument) SetOnNavigate(f func(href string)) {
	document.onNavigate = f
}

func (document *htmlDocument) ChangeCount() int64 {
	return document.changed
}

func (document *htmlDocument) markChanged() {
	document.changed++
	if document.onChanged != nil {
		document.onChanged()
	}
}

func (document *htmlDocument) Close() {
	document.viewport = nil
}
