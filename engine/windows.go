package engine

// Viewport 是渲染输出目标的抽象：一个有尺寸、可绘图的窗口/画布。
// 具体后端（如 SDL2）实现该接口；文档通过 OpenDocument 绑定到视口。
type Viewport interface {
	ViewportWidth() int
	ViewportHeight() int
	Graphics() Graphics
}

// OpenDocument 解析 HTML/CSS 源码并完成首次级联与布局。
func OpenDocument(v Viewport, src string) (HTMLDocument, error) {
	if v == nil {
		return nil, &engineError{msg: "viewport is required"}
	}
	doc := NewDocument()
	parseInto(doc, doc.Head(), doc.Body(), src, doc)
	doc.(*htmlDocument).viewport = v
	LayoutDocument(doc)
	return doc, nil
}

// LayoutDocument 对文档执行一次完整的级联 + 布局。
func LayoutDocument(doc HTMLDocument) {
	v := doc.Viewport()
	if v == nil {
		return
	}
	base := inner(doc)
	if base == nil {
		return
	}
	g := v.Graphics()
	if g == nil {
		g = NewFakeGraphics()
	}
	resolveStylesTree(doc, nil, doc.Stylesheet())
	layoutBox(g, base, v.ViewportWidth(), v.ViewportHeight(), 0, 0, v.ViewportWidth())
	layoutPositioned(g, base, v.ViewportWidth(), v.ViewportHeight())
}

// HeadlessViewport 提供无显示环境下的测试视口（FakeGraphics）。
type HeadlessViewport struct {
	W, H int
	G    Graphics
}

func NewHeadlessViewport(w, h int) *HeadlessViewport {
	return &HeadlessViewport{W: w, H: h, G: NewFakeGraphics()}
}

func (v *HeadlessViewport) ViewportWidth() int  { return v.W }
func (v *HeadlessViewport) ViewportHeight() int { return v.H }
func (v *HeadlessViewport) Graphics() Graphics  { return v.G }
