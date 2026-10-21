package engine

import "github.com/go-comm/ghromex/core"

// Viewport 是渲染输出目标的抽象：一个有尺寸、可绘图的窗口/画布。
// 具体后端（如 SDL2）实现该接口；文档通过 OpenDocument 绑定到视口。
//
// 接口本体在 core 包（见 core.Viewport）：平台后端只依赖 core，不反向
// 依赖 engine。这里的别名保证对外 API 不变、两边实现的是同一类型。
type Viewport = core.Viewport

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

// OpenBufferedDocument 在软件光栅化后端上打开文档，并完成首次级联、布局
// 与绘制，返回缓冲区与文档，供逐像素断言。
//
// 它把「建视口 → OpenDocument → RenderNode」三步收成一步：视口与缓冲的尺寸
// 由同一处给定，不会写岔（尺寸不一致时，像素断言会读到空白处）。
func OpenBufferedDocument(w, h int, src string) (*BufferGraphics, HTMLDocument, error) {
	vp, buf := NewBufferedViewport(w, h)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		return nil, nil, err
	}
	RenderNode(buf, doc)
	return buf, doc, nil
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
	layoutBox(g, base, v.ViewportWidth(), v.ViewportHeight(), 0, 0, v.ViewportWidth(), v.ViewportHeight())
	layoutPositioned(g, base, v.ViewportWidth(), v.ViewportHeight())
	applyScrollOffsets(g, base, v.ViewportWidth(), v.ViewportHeight())
	layoutSelectPopups(g, base, v.ViewportWidth(), v.ViewportHeight())
	// 视口尺寸变了 → 补一次变更通知。窗口缩放只改视口、不改文档内容，
	// onChanged 不会触发，应用层（编辑器铺满窗口、状态栏重排等）就
	// 收不到任何信号。放在布局之后通知：此时 DOM 结构已稳定，应用层
	// 在回调里改样式只会让本次布局的结果过期一次（后端按 ChangeCount
	// 再排一遍即可收敛）。
	if d, ok := doc.(*htmlDocument); ok {
		w, h := v.ViewportWidth(), v.ViewportHeight()
		if d.vpW != w || d.vpH != h {
			d.vpW, d.vpH = w, h
			d.markChanged()
		}
	}
}

// HeadlessViewport 提供无显示环境下的测试视口（FakeGraphics）。
type HeadlessViewport struct {
	W, H int
	G    Graphics
}

// NewHeadlessViewport 创建无显示视口：w×h 视口 + FakeGraphics 绘制后端，
// 供无显示器环境做级联/布局与 SVG 导出（不产生任何窗口）。
//
// 要出图/逐像素断言像素，用 NewBufferedViewport（软件光栅化后端）。
func NewHeadlessViewport(w, h int) *HeadlessViewport {
	return &HeadlessViewport{W: w, H: h, G: NewFakeGraphics()}
}

// NewBufferedViewport 创建无显示视口 + 软件光栅化后端（BufferGraphics）：
// 视口与缓冲同尺寸，绘制指令落进缓冲区，可 ColorAt 逐像素断言、SavePNG 出图。
//
// 与 NewHeadlessViewport 只差绘制后端：两者的文本度量同口径
// （BufferGraphics 的度量与 FakeGraphics 相同），换后端不改布局结果。
func NewBufferedViewport(w, h int) (*HeadlessViewport, *BufferGraphics) {
	buf := NewBufferGraphics(w, h)
	return &HeadlessViewport{W: w, H: h, G: buf}, buf
}

// ViewportWidth 返回视口宽度（px）。
func (v *HeadlessViewport) ViewportWidth() int { return v.W }

// ViewportHeight 返回视口高度（px）。
func (v *HeadlessViewport) ViewportHeight() int { return v.H }

// Graphics 返回该视口使用的绘制后端（默认 FakeGraphics）。
func (v *HeadlessViewport) Graphics() Graphics { return v.G }
