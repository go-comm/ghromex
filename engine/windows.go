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
	return OpenDocumentWith(v, src, DocumentOptions{})
}

// OpenDocumentWith 同 OpenDocument，额外传入文档选项（外部样式表加载器与
// 地址基准）。
//
// 选项必须在这里给、而不是等文档建好再设置：<link rel="stylesheet"> 与
// CSS 的 @import 都在**解析阶段**就解析并加载，文档返回时引用已经处理完了
// ——事后 SetStylesheetLoader 只影响此后新增的链接，页面里原有的引用不会
// 补加载。
func OpenDocumentWith(v Viewport, src string, opts DocumentOptions) (HTMLDocument, error) {
	if v == nil {
		return nil, &engineError{msg: "viewport is required"}
	}
	doc := NewDocument()
	impl := doc.(*htmlDocument)
	impl.baseURL = opts.BaseURL
	impl.loadSheet = opts.LoadStylesheet
	parseInto(doc, doc.Head(), doc.Body(), src, doc)
	impl.viewport = v
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
//
// 快速路径：自上次全量布局以来**只发生过滚动**（changed 与 scrollMark 同幅
// 增长，见 markScrollChanged）时，级联与盒模型都没变，只把滚动偏移平移到
// 已布局的盒子上（applyScrollOffsets，按绝对偏移重新施加，安全因为本次没
// 有重排、盒子还带着上一轮的平移量——它内部按 scrollShift* 差值换算）。
// 大文档每档滚轮触发的 4s+ 全量重排因此降为一次纯平移。
//
// 视口尺寸变化是否决项：平移改不了盒宽，尺寸变了本次就必须全量（见
// viewportResized）。
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
	// hd 为 nil 表示这不是引擎自己的文档实现（宿主自定义），只能全量布局。
	hd, _ := doc.(*htmlDocument)
	resized := false
	if hd != nil {
		// 先比对尺寸再判快路径：本次布局用的就是新尺寸，尺寸这笔账要落在
		// 记账**之前**，否则 changed 比 lastLayoutChange 多出一笔永不算数的
		// 账，下一档滚轮的「只发生了滚动」判定失效（差 1 就不等）→ 整棵树
		// 重排（实测 42 万元素页面 1.75 秒，发生在开页后与每次缩放后的第一
		// 档滚轮上）。
		resized = hd.viewportResized(v)
		if delta, scrollDelta := hd.changed-hd.lastLayoutChange, hd.scrollMark-hd.lastAppliedScrollMark; !resized && delta > 0 && delta == scrollDelta {
			// 仅滚动：跳过级联与布局。盒子还带着上一轮已施加的平移量
			// （scrollShift*），必须按**增量**平移（applyScrollDeltas），
			// 不能用 applyScrollOffsets 的绝对平移——那会二次位移。
			applyScrollDeltas(g, base, v.ViewportWidth(), v.ViewportHeight())
			layoutSelectPopups(g, base, v.ViewportWidth(), v.ViewportHeight())
			hd.lastLayoutChange = hd.changed
			hd.lastAppliedScrollMark = hd.scrollMark
			return
		}
	}
	resolveStylesTree(doc, nil, doc.Stylesheet())
	layoutBox(g, base, v.ViewportWidth(), v.ViewportHeight(), 0, 0, v.ViewportWidth(), v.ViewportHeight(), nil)
	layoutPositioned(g, base, v.ViewportWidth(), v.ViewportHeight())
	applyScrollOffsets(g, base, v.ViewportWidth(), v.ViewportHeight())
	layoutSelectPopups(g, base, v.ViewportWidth(), v.ViewportHeight())
	if hd != nil {
		// 记账在通知之前：通知里宿主可能改样式（又一次 markChanged），那笔
		// 账属于下一轮，不能让本轮的 lastLayoutChange 把它算成「已布局」。
		hd.lastLayoutChange = hd.changed
		hd.lastAppliedScrollMark = hd.scrollMark
		// 视口尺寸变了 → 补一次通知。窗口缩放只改视口、不改文档内容，
		// onChanged 不会触发，应用层（编辑器铺满窗口、状态栏重排等）就
		// 收不到任何信号。放在布局之后通知：此时 DOM 结构已稳定，应用层
		// 在回调里改样式只会让本次布局的结果过期一次（后端按 ChangeCount
		// 再排一遍即可收敛）。只通知、不 +changed——尺寸变化本次布局已经
		// 消化掉了，多加一笔账会把滚动快路径永久废掉（见 notifyChanged）。
		if resized {
			hd.notifyChanged()
		}
	}
}

// viewportResized 比对并记录视口尺寸，返回是否变了。变了则本次不能走滚动
// 快路径：平移改不了盒宽，必须全量级联+布局（见 LayoutDocument）。
func (d *htmlDocument) viewportResized(v Viewport) bool {
	w, h := v.ViewportWidth(), v.ViewportHeight()
	if d.vpW == w && d.vpH == h {
		return false
	}
	d.vpW, d.vpH = w, h
	return true
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
// 视口与缓冲同尺寸，绘制指令落进缓冲区，可 ColorAt 逐像素断言、DumpPNG 出图。
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
