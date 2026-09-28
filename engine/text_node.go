package engine

import (
	"io"
	"strings"
)

// textNode 是元素内的文本子节点，布局引擎会为每一行文本生成一个绘制片段。
type textNode struct {
	HTMLElement

	node *htmlElement
}

// textRun 是布局阶段为一段文本生成的绘制片段（按行切分）。
type textRun struct {
	text string
	x    int
	y    int
	w    int
	h    int
}

// NewTextNode 创建一个文本节点。
func NewTextNode(text string) HTMLElement {
	t := &textNode{}
	e := newElement("#text").(*htmlElement)
	e.text = text
	e.self = t
	t.HTMLElement = e
	t.node = e
	return t
}

func (t *textNode) TagName() string { return "#text" }

func (t *textNode) AppendChild(e HTMLElement, es ...HTMLElement) {}

func (t *textNode) RemoveChild(e HTMLElement, es ...HTMLElement) {}

func (t *textNode) Children() []HTMLElement { return nil }

func (t *textNode) ParentElement() HTMLElement { return t.node.parentElement }

func (t *textNode) setParentElement(p HTMLElement) { t.node.parentElement = p }

func (t *textNode) baseElement() *htmlElement { return t.node }

func (t *textNode) Text() string { return t.node.text }

func (t *textNode) SetText(s string) {
	t.node.text = s
	t.node.onChanged()
}

func (t *textNode) Clone() HTMLElement {
	c := &textNode{}
	e := &htmlElement{}
	*e = *t.node
	e.parentElement = nil
	e.computed = nil
	e.attrs = make(map[string]string, len(t.node.attrs))
	for k, v := range t.node.attrs {
		e.attrs[k] = v
	}
	e.self = c
	c.HTMLElement = e
	c.node = e
	return c
}

func (t *textNode) writeToJSON(w io.StringWriter, prefix string, indent bool, walk string) {
	if indent {
		w.WriteString(walk)
	}
	w.WriteString("{\"tagName\":\"#text\",\"text\":\"")
	w.WriteString(t.node.text)
	w.WriteString("\"}")
}

func (t *textNode) EncodeJSON(w io.StringWriter, prefix string, indent bool) {
	t.writeToJSON(w, prefix, indent, "")
}

func (t *textNode) FormatJSON(prefix string, indent bool) string {
	var b strings.Builder
	t.EncodeJSON(&b, prefix, indent)
	return b.String()
}

func (t *textNode) String() string {
	var b strings.Builder
	t.EncodeJSON(&b, "", false)
	return b.String()
}
