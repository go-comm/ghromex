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

	viewport  Viewport
	sheet     *Stylesheet
	head      HTMLHeadElement
	body      HTMLBodyElement
	focus     HTMLElement // 当前聚焦的可编辑输入（v1 仅 input），nil 无焦点
	changed   int64
	onChanged func()
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
		return e.Clone()
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
