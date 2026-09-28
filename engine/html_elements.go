package engine

// init 注册系统内置标签原型。默认盒行为与 UA 样式表（style_ua.go）配合生效。
func init() {
	var elements = []HTMLElement{
		newHeadElement(),
		newBodyElement(),
		newDivElement(),
		newSpanElement(),
	}

	for _, element := range elements {
		SystemElements().Define(element.TagName(), element)
	}
}

type HTMLHeadElement interface {
	HTMLElement

	Title() HTMLElement
}

func newHeadElement() HTMLHeadElement {
	e := &htmlHeadElement{}
	e.HTMLElement = newElement("head")
	e.title = newElement("title")
	e.HTMLElement.AppendChild(e.title)
	return e
}

type htmlHeadElement struct {
	HTMLElement

	title HTMLElement
}

func (e *htmlHeadElement) Title() HTMLElement {
	return e.title
}

func (e *htmlHeadElement) AppendChild(child HTMLElement, es ...HTMLElement) {
	e.HTMLElement.AppendChild(child, es...)
}

func (e *htmlHeadElement) RemoveChild(child HTMLElement, es ...HTMLElement) {
	e.HTMLElement.RemoveChild(child, es...)
}

func (e *htmlHeadElement) Children() []HTMLElement { return e.HTMLElement.Children() }

func (e *htmlHeadElement) ParentElement() HTMLElement { return e.HTMLElement.ParentElement() }

func (e *htmlHeadElement) setParentElement(p HTMLElement) { e.HTMLElement.setParentElement(p) }

func (e *htmlHeadElement) Clone() HTMLElement {
	c := *e
	inner := e.HTMLElement.Clone().(*htmlElement)
	c.HTMLElement = inner
	c.title = nil
	return &c
}

type HTMLBodyElement interface {
	HTMLElement

	SetOnLoad(f func())
}

func newBodyElement() HTMLBodyElement {
	e := &htmlBodyElement{}
	e.HTMLElement = newElement("body")
	return e
}

type htmlBodyElement struct {
	HTMLElement
}

func (e *htmlBodyElement) SetOnLoad(f func()) {
	// TODO: 文档就绪回调
}

func (e *htmlBodyElement) AppendChild(child HTMLElement, es ...HTMLElement) {
	e.HTMLElement.AppendChild(child, es...)
}

func (e *htmlBodyElement) RemoveChild(child HTMLElement, es ...HTMLElement) {
	e.HTMLElement.RemoveChild(child, es...)
}

func (e *htmlBodyElement) Children() []HTMLElement { return e.HTMLElement.Children() }

func (e *htmlBodyElement) ParentElement() HTMLElement { return e.HTMLElement.ParentElement() }

func (e *htmlBodyElement) setParentElement(p HTMLElement) { e.HTMLElement.setParentElement(p) }

func (e *htmlBodyElement) Clone() HTMLElement {
	c := *e
	inner := e.HTMLElement.Clone().(*htmlElement)
	c.HTMLElement = inner
	return &c
}

// HTMLUnknownElement 未知标签的占位类型（行内原子盒）。
type HTMLUnknownElement struct {
	HTMLElement
}

func newHTMLUnknownElement(tagName string) HTMLElement {
	return newElement(tagName)
}

type HTMLDivElement struct {
	HTMLElement
}

func newDivElement() HTMLElement {
	e := &HTMLDivElement{}
	e.HTMLElement = newElement("div")
	return e
}

func (e *HTMLDivElement) AppendChild(child HTMLElement, es ...HTMLElement) {
	e.HTMLElement.AppendChild(child, es...)
}

func (e *HTMLDivElement) RemoveChild(child HTMLElement, es ...HTMLElement) {
	e.HTMLElement.RemoveChild(child, es...)
}

func (e *HTMLDivElement) Children() []HTMLElement { return e.HTMLElement.Children() }

func (e *HTMLDivElement) ParentElement() HTMLElement { return e.HTMLElement.ParentElement() }

func (e *HTMLDivElement) setParentElement(p HTMLElement) { e.HTMLElement.setParentElement(p) }

func (e *HTMLDivElement) Clone() HTMLElement {
	c := *e
	inner := e.HTMLElement.Clone().(*htmlElement)
	c.HTMLElement = inner
	return &c
}

type HTMLSpanElement struct {
	HTMLElement
}

func newSpanElement() HTMLElement {
	e := &HTMLSpanElement{}
	e.HTMLElement = newElement("span")
	return e
}

func (e *HTMLSpanElement) AppendChild(child HTMLElement, es ...HTMLElement) {
	e.HTMLElement.AppendChild(child, es...)
}

func (e *HTMLSpanElement) RemoveChild(child HTMLElement, es ...HTMLElement) {
	e.HTMLElement.RemoveChild(child, es...)
}

func (e *HTMLSpanElement) Children() []HTMLElement { return e.HTMLElement.Children() }

func (e *HTMLSpanElement) ParentElement() HTMLElement { return e.HTMLElement.ParentElement() }

func (e *HTMLSpanElement) setParentElement(p HTMLElement) { e.HTMLElement.setParentElement(p) }

func (e *HTMLSpanElement) Clone() HTMLElement {
	c := *e
	inner := e.HTMLElement.Clone().(*htmlElement)
	c.HTMLElement = inner
	return &c
}
