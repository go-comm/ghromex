package engine

import "strings"

// MeasureNode 布局入口：对根节点在 (mw, mh) 可用空间内执行一次布局。
// 若样式尚未级联（computed 为空），先以空样式表级联。
func MeasureNode(g Graphics, node HTMLElement, mw, mh int) {
	if g == nil {
		g = NewFakeGraphics()
	}
	base := inner(node)
	if base == nil {
		return
	}
	if base.computed == nil {
		resolveStylesTree(node, nil, nil)
	}
	layoutBox(g, base, mw, mh, 0, 0, mw, mh)
	layoutPositioned(g, base, mw, mh)
	applyScrollOffsets(g, node, mw, mh)
	layoutSelectPopups(g, node, mw, mh)
}

// MeasureDocumentWithStylesheet 完整流水线入口：级联 + 布局，不依赖 Viewport。
func MeasureDocumentWithStylesheet(g Graphics, doc HTMLDocument, mw, mh int) {
	base := inner(doc)
	if base == nil {
		return
	}
	var sheet *Stylesheet
	if d, ok := doc.(*htmlDocument); ok {
		sheet = d.sheet
	}
	resolveStylesTree(doc, nil, sheet)
	layoutBox(g, base, mw, mh, 0, 0, mw, mh)
	layoutPositioned(g, base, mw, mh)
	applyScrollOffsets(g, doc, mw, mh)
	layoutSelectPopups(g, doc, mw, mh)
}

// ---------------------------------------------------------------------------
// 盒模型边
// ---------------------------------------------------------------------------

type edges [4]int

const (
	eTop = iota
	eRight
	eBottom
	eLeft
)

// resolveEdgeRect 解析盒模型四边。百分比**一律按包含块宽度**解析
// （CSS 2.1 §8.3.2：margin/padding 的百分比相对包含块宽度，纵向边也不
// 例外）——旧实现纵向用 baseH（高度），与 CSS 及 Chrome 不符。
func resolveEdgeRect(r Rect, baseW int) edges {
	var e edges
	if r == nil {
		return e
	}
	e[eTop] = maxZero(resolveLen(r.Top(), baseW))
	e[eRight] = maxZero(resolveLen(r.Right(), baseW))
	e[eBottom] = maxZero(resolveLen(r.Bottom(), baseW))
	e[eLeft] = maxZero(resolveLen(r.Left(), baseW))
	return e
}

// resolvedEdges 返回元素布局期解析出的 padding/border 边长；未布局过
// （display:none 等）则按 0 基数现算。绘制、命中、SVG 导出三处必须走
// 这里，保证"布局算出的盒"与"画出来的盒"是同一个盒。
func resolvedEdges(el *htmlElement, comp CSSStyleDeclaration) (pd, bd edges) {
	if el != nil && el.hasRsv {
		return el.rsvPD, el.rsvBD
	}
	if comp == nil {
		return
	}
	pd = resolveEdgeRect(comp.Padding(), 0)
	bd = resolveEdgeRect(comp.BorderStyleWidth(), 0)
	return
}

func maxZero(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// shiftInlineX 将行内原子盒（inline/inline-block 子树）整体水平平移 dx。
// 引擎内部坐标为绝对值（内容盒 x 与各文本 run x），逐层平移即可，
// 渲染与命中测试随之一致。
func shiftInlineX(e HTMLElement, dx int) {
	if dx == 0 || e == nil {
		return
	}
	if tn, ok := e.(*textNode); ok {
		for i := range tn.node.runs {
			tn.node.runs[i].x += dx
		}
		return
	}
	base := inner(e)
	if base == nil {
		return
	}
	base.setX(base.x + dx)
	for _, c := range base.children {
		shiftInlineX(c, dx)
	}
}

// ---------------------------------------------------------------------------
// 单盒布局
// ---------------------------------------------------------------------------

// outerBoxW 返回元素真实外边距盒宽（内容宽 + padding + border + margin）。
// layoutBox 对 block 子返回的是"占位空间宽"，不能用作真实宽度
// （shrink-wrap 统计与 right 锚定回推均需真实值）。
// baseW 为 margin 百分比的包含块宽度基数（须与布局期同口径，否则
// 百分比 margin 的盒宽与实际排版不符）。
func outerBoxW(el *htmlElement, baseW int) int {
	if el == nil {
		return 0
	}
	if el.computed == nil {
		return el.width
	}
	mg := resolveEdgeRect(el.computed.Margin(), baseW)
	pd, bd := resolvedEdges(el, el.computed)
	return el.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight] + mg[eLeft] + mg[eRight]
}

// inputButtonTextWidth 返回表单控件「内容随文本收缩」时的外盒宽
// （文本 + padding + border，与 UA border-box 尺寸同口径；
// layoutBox 会再扣 padding/border 得内容宽）；不适用返回 -1。
func inputButtonTextWidth(g Graphics, el *htmlElement, comp CSSStyleDeclaration, pctW int) int {
	if comp == nil || el == nil {
		return -1
	}
	var text string
	arrowReserve := 0
	switch {
	case strings.EqualFold(el.tagName, "input"):
		switch strings.ToLower(el.GetAttribute("type")) {
		case "submit", "reset", "button":
		default:
			return -1
		}
		text = el.GetAttribute("value")
	case strings.EqualFold(el.tagName, "select"):
		// select 的 auto 宽按显示文本 + 右侧下拉箭头位（selectArrowWidth）
		// 收缩——与绘制同口径，否则显示文本会被箭头挤掉。
		text = selectDisplayText(el)
		arrowReserve = selectArrowWidth
	default:
		return -1
	}
	fontPx := 13
	if s := resolveLen(comp.FontSize(), 0); s > 0 {
		fontPx = s
	}
	w, _ := g.MeasureText(text, fontPx, comp.FontWeight() == FontWeightBold, comp.FontFamily())
	bd := resolveEdgeRect(comp.BorderStyleWidth(), pctW)
	pd := resolveEdgeRect(comp.Padding(), pctW)
	return w + arrowReserve + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
}

// defaultFormControlHeight 为 height:auto 的原子表单控件给出内容盒兜底高度
// （字体行高 + padding + border），避免空控件塌缩成 0 高。
func defaultFormControlHeight(g Graphics, comp CSSStyleDeclaration, pctW int) int {
	if g == nil || comp == nil {
		return 21
	}
	fontPx := 13
	if s := resolveLen(comp.FontSize(), 0); s > 0 {
		fontPx = s
	}
	_, h := g.MeasureText("Mg", fontPx, comp.FontWeight() == FontWeightBold, comp.FontFamily())
	if h <= 0 {
		h = fontPx
	}
	pd := resolveEdgeRect(comp.Padding(), pctW)
	bd := resolveEdgeRect(comp.BorderStyleWidth(), pctW)
	h += pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
	if h <= 0 {
		h = 21
	}
	return h
}

// layoutBox 布局一个元素盒。
// outerW/outerH 为该盒在父容器内容区内可用的外边距盒空间；
// ox/oy 为外边距盒原点；pctW 为百分比解析基数（包含块内容宽，CSS 规定百分比
// width/padding 等相对包含块而非行内剩余空间，inline 分支不得混用）；
// pctH 为**高度**百分比的解析基数（包含块内容高；<0 表示包含块高度不确定，
// 此时百分比高度按 auto 处理，CSS 2.1 §10.5）。
// 返回外边距盒宽高，并写入内容盒的 x/y/width/height 与 layoutH。
func layoutBox(g Graphics, el *htmlElement, outerW, outerH, ox, oy, pctW, pctH int) (retW, retH int) {
	comp := el.computed
	if comp == nil {
		comp = newStyle()
		el.computed = comp
	}
	disp := comp.Display()
	if disp == DisplayNone {
		el.layoutH = 0
		el.contentW, el.contentH = 0, 0
		return 0, 0
	}

	mg := resolveEdgeRect(comp.Margin(), pctW)
	bd := resolveEdgeRect(comp.BorderStyleWidth(), pctW)
	pd := resolveEdgeRect(comp.Padding(), pctW)
	// 缓存布局期解析出的边长：绘制、命中测试与 SVG 导出必须复用同一份——
	// 否则百分比 padding 在布局期按百分比算、在绘制期按 0 算，背景/边框会画
	// 在错误的盒上，命中框也与绘制框不符。
	el.rsvPD, el.rsvBD, el.hasRsv = pd, bd, true

	// 内容宽度。box-sizing:border-box 时显式 width/height 含 padding+border（不含 margin）。
	borderBox := comp.BoxSizing() == BoxSizingBorderBox
	contentW := resolveLen(comp.Width(), pctW)
	if contentW >= 0 && borderBox {
		if cw := contentW - pd[eLeft] - pd[eRight] - bd[eLeft] - bd[eRight]; cw > 0 {
			contentW = cw
		} else {
			contentW = 0
		}
	}
	shrinkWrap := false
	if contentW < 0 { // auto
		if w := inputButtonTextWidth(g, el, comp, pctW); w >= 0 {
			// input[submit/reset/button]：auto 宽按 value 文本测量（空元素
			// 无子节点，常规 shrink-to-fit 拿不到文字宽）。返回的是外盒宽
			//（文本+padding+border），按 border-box 语义扣减后即为内容宽——
			// 渲染层溢出截断以 base.width-4 为界，文本+余量才完整。
			if cw := w - pd[eLeft] - pd[eRight] - bd[eLeft] - bd[eRight]; cw > 0 {
				contentW = cw
			} else {
				contentW = 0
			}
		} else {
			contentW = outerW - mg[eLeft] - mg[eRight] - bd[eLeft] - bd[eRight] - pd[eLeft] - pd[eRight]
			if contentW < 0 {
				contentW = 0
			}
			// 非块级与脱流盒（absolute/fixed）：auto 宽度按内容收缩
			if disp != DisplayBlock || comp.Position().isOutFlow() {
				shrinkWrap = true
			}
		}
	}

	// 父-子 margin 穿越折叠（CSS 2.1 §8.3.1）：本块的外边距若已与首/末个
	// 流内块级子块折叠（父块在 layoutChildren 里算出并回填 effTop/effBot），
	// 这里用折叠后的值；本块的外边距若被**它的父块**吸收（mtAbsorbed），
	// 则自身记 0——折叠值已由父块计入，避免重复计算。
	if el.hasEff {
		mg[eTop] = el.effTop
		mg[eBottom] = el.effBot
	}
	if el.mtAbsorbed {
		mg[eTop] = 0
		el.mtAbsorbed = false
	}
	if el.mbAbsorbed {
		mg[eBottom] = 0
		el.mbAbsorbed = false
	}

	cx := ox + mg[eLeft] + bd[eLeft] + pd[eLeft]
	cy := oy + mg[eTop] + bd[eTop] + pd[eTop]
	el.setX(cx)
	el.setY(cy)

	remainH := outerH - mg[eTop] - mg[eBottom] - bd[eTop] - bd[eBottom] - pd[eTop] - pd[eBottom]

	// 自身声明高度（能确定时）。必须在排子元素**之前**算出：子元素的百分比
	// 高度以包含块（= 本元素）的内容高为基数，CSS 2.1 §10.5。
	// 百分比高度在包含块高度不确定（auto）时按 auto 处理。
	declaredH := -1
	if h := comp.Height(); h != nil {
		if h.Type() == SIZE_PERCENT {
			if pctH >= 0 {
				declaredH = resolveLen(h, pctH)
			}
		} else {
			declaredH = resolveLen(h, pctH)
		}
	}
	if declaredH >= 0 && borderBox {
		if ch := declaredH - pd[eTop] - pd[eBottom] - bd[eTop] - bd[eBottom]; ch > 0 {
			declaredH = ch
		} else {
			declaredH = 0
		}
	}
	// shrink-to-fit 盒的可用宽是收缩前的临时值，按它居中会把文本 run
	// 推到最终收缩后的盒外；而收缩后单行本就铺满盒宽，强制左对齐等价。
	align := comp.TextAlign()
	if shrinkWrap {
		align = TextAlignLeft
	}
	var flowW, flowH int
	if isAtomicFormControl(el) {
		// select/textarea 是原子盒：option 由第三遍布局（layoutSelectPopups）
		// 定位，textarea 的内容是 value 属性而非子节点，均不进常规流。
		// 其滚动量度不在此（textarea 走 layoutTextarea 的行数×行高）。
		flowW, flowH = 0, 0
	} else {
		// 子元素的百分比高度基数 = 本元素内容高（declaredH；auto 时为 -1
		// 表示包含块高度不确定，子元素的百分比高按 auto 处理）。
		flowW, flowH = layoutChildren(g, el, contentW, remainH, declaredH, cx, cy, align)
	}
	// 未裁剪内容尺寸：overflow 滚动的夹紧依据（每轮布局重算，滚动偏移
	// 由 applyScrollOffsets 按它 clamp）。
	el.contentW, el.contentH = flowW, flowH

	contentH := declaredH
	if contentH < 0 {
		contentH = flowH
		if contentH <= 0 && isAtomicFormControl(el) {
			// UA 未给高度（作者 height:auto）时按字体行高撑开，
			// 否则空控件会塌缩成 0 高不可见。
			contentH = defaultFormControlHeight(g, comp, pctW)
		}
	}

	if shrinkWrap {
		// 收缩包裹：宽度取内容行最大宽度
		fitW := flowW + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight] + mg[eLeft] + mg[eRight]
		if fitW > outerW {
			fitW = outerW
		}
		if fitW < 4 {
			fitW = 4
		}
		contentW = fitW - pd[eLeft] - pd[eRight] - bd[eLeft] - bd[eRight] - mg[eLeft] - mg[eRight]
		if contentW < 0 {
			contentW = 0
		}
	}

	el.setWidth(contentW)
	el.setHeight(contentH)

	outerBoxW := contentW + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight] + mg[eLeft] + mg[eRight]
	outerBoxH := contentH + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom] + mg[eTop] + mg[eBottom]
	el.layoutH = outerBoxH

	if disp == DisplayBlock {
		return outerW, outerBoxH
	}
	return outerBoxW, outerBoxH
}

// shiftBox 将元素子树整体平移 (dx, dy)：盒坐标 + 文本 run。
// 用于 relative 偏移与 absolute 的 right/bottom 锚定回推；
// 命中测试/绘制均读最终坐标，自然一致。
// position:fixed 的子树跳过：fixed 以视口为包含块，不随祖先滚动平移
// （Chrome 语义）——文档级滚动时 fixed 元素必须留在视口原位。
func shiftBox(e HTMLElement, dx, dy int) {
	if e == nil || (dx == 0 && dy == 0) {
		return
	}
	base := inner(e)
	if base == nil {
		return
	}
	base.setX(base.x + dx)
	base.setY(base.y + dy)
	for i := range base.runs {
		base.runs[i].x += dx
		base.runs[i].y += dy
	}
	for _, c := range base.children {
		if cb := inner(c); cb != nil && cb.computed != nil &&
			cb.computed.Position() == PositionFixed {
			continue
		}
		shiftBox(c, dx, dy)
	}
}

// ---------------------------------------------------------------------------
// 定位布局（第二 pass）：position relative/absolute/fixed
// ---------------------------------------------------------------------------

// containingBlock 返回 absolute 元素的包含块（最近非 static 祖先的 padding box；
// 无则视口）。fixed 直接用视口，由调用方区分。
func containingBlock(el *htmlElement, vw, vh int) (x, y, w, h int) {
	for p := el.parentElement; p != nil; p = p.ParentElement() {
		pb := inner(p)
		if pb == nil || pb.computed == nil {
			continue
		}
		pos := pb.computed.Position()
		if pos == PositionStatic {
			continue
		}
		// 祖先自身也是 fixed 时 CSS 仍作包含块；relative/absolute 用其 padding box。
		pd, _ := resolvedEdges(pb, pb.computed)
		return pb.x - pd[eLeft], pb.y - pd[eTop],
			pb.width + pd[eLeft] + pd[eRight], pb.height + pd[eTop] + pd[eBottom]
	}
	return 0, 0, vw, vh
}

// resolveInset 解析 inset 边值；auto/nil 返回 ok=false。
// 不能用 resolveLen 的 -1 哨兵判 auto——CSS inset 允许负值（如 top:-20px），
// 两者必须区分。
func resolveInset(s Size, base int) (int, bool) {
	if s == nil || s.Type() == SIZE_AUTO {
		return 0, false
	}
	return resolveLen(s, base), true
}

// layoutAbsBox 布局一个脱流盒（absolute/fixed）。
// CSS 语义中 left/top 锚定的是 margin box 外缘，而 layoutBox 的 ox/oy
// 正是 margin box 原点，直接复用。v1 简化（注释于文档）：
//   - width/height auto 按 shrink-to-fit（left+right 同时给出时浏览器
//     会拉伸填满，这里取内容宽，典型角标/浮层单行内容宽度一致）；
//   - left/top 均 auto 时取包含块原点（不实现 static position）。
func layoutAbsBox(g Graphics, e HTMLElement, el *htmlElement, vw, vh int) {
	comp := el.computed
	var cbX, cbY, cbW, cbH int
	if comp.Position() == PositionFixed {
		cbX, cbY, cbW, cbH = 0, 0, vw, vh
	} else {
		cbX, cbY, cbW, cbH = containingBlock(el, vw, vh)
	}
	in := comp.Inset()
	l, lok := resolveInset(in.Left(), cbW)
	r, rok := resolveInset(in.Right(), cbW)
	t, tok := resolveInset(in.Top(), cbH)
	b, bok := resolveInset(in.Bottom(), cbH)

	// 水平：左锚直接给原点；仅右锚先按可用宽布局，再回推；双锚取夹段。
	availX, ox := cbW, cbX
	switch {
	case lok && rok:
		availX = cbW - l - r
	case lok:
		availX = cbW - l
	case rok:
		availX = cbW - r
	}
	if lok {
		ox = cbX + l
	}

	availY, oy := cbH, cbY
	switch {
	case tok && bok:
		availY = cbH - t - b
	case tok:
		availY = cbH - t
	case bok:
		availY = cbH - b
	}
	if tok {
		oy = cbY + t
	}

	// layoutBox 对 block 返回的占位空间宽不可用于锚定回推，忽略之。
	// 高度百分比以包含块高度 cbH 为基数（CSS 2.1 §10.5）——不是"剩余可用
	// 高度"：同一元素只改 top 就不该得到不同的高度。
	_, oh := layoutBox(g, el, maxInt(availX, 0), maxInt(availY, 0), ox, oy, cbW, cbH)
	if !lok && rok {
		// right 锚定：目标右缘 cbX+cbW-r 减去盒子当前位置（ox+真实宽）。
		// 注意必须含 ox：包含块不在原点时漏它会整体偏移 ox（CB 在 (40,40)
		// 的回归用例锁定）。
		mg := resolveEdgeRect(comp.Margin(), cbW)
		pd, bd := resolvedEdges(el, comp)
		realW := el.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight] + mg[eLeft] + mg[eRight]
		shiftBox(e, cbX+cbW-r-(ox+realW), 0)
	}
	if !tok && bok {
		// oh（layoutBox 返回的 outerBoxH）为真实外盒高；同样需减盒子当前 y=oy
		shiftBox(e, 0, cbY+cbH-b-(oy+oh))
	}
}

// applyRelativeOffset 对已在常规流中布局完成的 relative 盒施加 inset 偏移。
// left/top 为正向偏移；仅给 right/bottom 时反向（CSS 取 -right/-bottom）。
// 百分比基数为**包含块宽度**（Chrome 口径：relative 的纵向 inset 百分比
// 也按宽度算）；vw 为无父元素时的兜底（视口宽）。
func applyRelativeOffset(e HTMLElement, el *htmlElement, vw int) {
	in := el.computed.Inset()
	base := vw
	if p := inner(el.parentElement); p != nil {
		base = p.width
	}
	dx, dy := 0, 0
	if v, ok := resolveInset(in.Left(), base); ok {
		dx = v
	} else if v, ok := resolveInset(in.Right(), base); ok {
		dx = -v
	}
	if v, ok := resolveInset(in.Top(), base); ok {
		dy = v
	} else if v, ok := resolveInset(in.Bottom(), base); ok {
		dy = -v
	}
	shiftBox(e, dx, dy)
}

// layoutPositioned 第二 pass：按 DOM 前序遍历，先处理自身定位再下钻子树，
// 保证嵌套 absolute 的包含块坐标已确定。
func layoutPositioned(g Graphics, el *htmlElement, vw, vh int) {
	for _, child := range el.children {
		che := inner(child)
		if che == nil || che.computed == nil || che.computed.Display() == DisplayNone {
			continue
		}
		switch che.computed.Position() {
		case PositionAbsolute, PositionFixed:
			layoutAbsBox(g, child, che, vw, vh)
		case PositionRelative:
			applyRelativeOffset(child, che, vw)
		}
		layoutPositioned(g, che, vw, vh)
	}
}

// ---------------------------------------------------------------------------
// 子节点流式布局：块级堆叠 + 行内混排（原子行内盒 + 文本词换行）
// ---------------------------------------------------------------------------

type lineItem struct {
	tn    *textNode
	el    HTMLElement // 行内原子盒（inline/inline-block），文本项为 nil
	token string
	x     int
	y     int
	w     int
	h     int
}

// layoutChildren 排布子节点。contentH 为父内容区的**可用**高度（auto 高度
// 撑开时逐行消耗）；pctH 为子元素高度百分比的基数（父元素内容高，<0 表示
// 包含块高度不确定 → 子的百分比高按 auto 处理）。
func layoutChildren(g Graphics, el *htmlElement, contentW, contentH, pctH, cx, cy int, align TextAlign) (flowW, flowH int) {
	cursorX := cx
	cursorY := cy
	lineH := 0
	var curLine []lineItem
	maxRight := cx
	// prevBlockMB：上一个相邻块级子的 margin-bottom；-1 = 尚无
	//（或中间隔了行内内容/为首个子），此时不折叠。
	prevBlockMB := -1

	fontPx := 16
	bold := false
	family := ""
	if el.computed != nil {
		if s := resolveLen(el.computed.FontSize(), 0); s > 0 {
			fontPx = s
		}
		bold = el.computed.FontWeight() == FontWeightBold
		family = el.computed.FontFamily()
	}

	// 行内格式上下文里的空格：CSS 把连续空白折叠为单个空格，且只保留在
	// 行内内容之间。tokenizeText 会丢弃首尾空白，纯空白文本节点因此整体
	// 消失（`<span>A</span> <span>B</span>` 的空格被吞）。这里用
	// pendingSpace 把空格延迟到"下一个行内项"再落位：遇到块级子元素则
	// 丢弃（CSS 移除行尾空格）。
	spaceW, spaceH := 0, 0
	if g != nil {
		spaceW, spaceH = g.MeasureText(" ", fontPx, bold, family)
	}
	pendingSpace := false
	pendingTN := (*textNode)(nil)
	lastInline := false

	// flushPendingSpace 把待定空格落位到当前行（作为独立文本 run）。
	// 必须在**下一个行内盒布局之前**调用：行内盒的位置由 layoutBox 按游标
	// 决定，空格若在其后追加，盒子会落在空格之前。
	flushPendingSpace := func() {
		if !pendingSpace {
			return
		}
		pendingSpace = false
		if spaceW <= 0 || pendingTN == nil {
			return
		}
		curLine = append(curLine, lineItem{tn: pendingTN, token: " ",
			x: cursorX, y: cursorY, w: spaceW, h: spaceH})
		cursorX += spaceW
		if spaceH > lineH {
			lineH = spaceH
		}
		if cursorX > maxRight {
			maxRight = cursorX
		}
	}

	flushLine := func() {
		// text-align：行宽不足内容宽时整体平移（溢出时 dx 钳 0，左对齐语义不变）
		dx := 0
		if align != TextAlignLeft && len(curLine) > 0 {
			if free := contentW - (cursorX - cx); free > 0 {
				if align == TextAlignCenter {
					dx = free / 2
				} else {
					dx = free
				}
			}
		}
		for _, it := range curLine {
			if it.tn != nil {
				y := it.y + (lineH-it.h)/2
				it.tn.node.runs = append(it.tn.node.runs, textRun{
					text: it.token, x: it.x + dx, y: y, w: it.w, h: it.h,
				})
			} else if it.el != nil && dx != 0 {
				shiftInlineX(it.el, dx)
			}
		}
		if lineH > 0 {
			cursorY += lineH
		}
		lineH = 0
		curLine = curLine[:0]
		cursorX = cx
	}

	addItem := func(it lineItem) {
		if len(curLine) > 0 && it.x+it.w-cx > contentW && contentW > 0 {
			flushLine()
			it.x = cursorX
			it.y = cursorY
		}
		curLine = append(curLine, it)
		cursorX += it.w
		if it.h > lineH {
			lineH = it.h
		}
		if cursorX > maxRight {
			maxRight = cursorX
		}
		// 行内内容隔开两侧块盒：CSS 规定 margin 折叠仅发生在真正相邻的
		// 块盒之间（行盒 / inline-block / 脱流盒不算分隔，但行内容算）。
		prevBlockMB = -1
		lastInline = true
	}

	for _, child := range el.children {
		if tnc, ok := child.(*textNode); ok {
			tnc.node.runs = nil
			raw := tnc.Text()
			// 前导空白：上一项是行内内容时，该空格属于行内间隙，须保留。
			if lastInline && hasLeadingBlank(raw) {
				pendingSpace = true
				pendingTN = tnc
			}
			toks := tokenizeText(raw)
			if len(toks) > 0 {
				// 待定空格必须**先落位**再取游标：否则空格的宽度被计入游标，
				// 而字符仍按冲刷前的游标定位 → 首字与后续字之间凭空多出一段
				// 空隙（"不接收" 渲染成 "不 接收"）。
				flushPendingSpace()
			}
			for _, tok := range toks {
				tw, th := g.MeasureText(tok, fontPx, bold, family)
				if tw <= 0 {
					continue
				}
				addItem(lineItem{tn: tnc, token: tok, x: cursorX, y: cursorY, w: tw, h: th})
			}
			// 尾随空白：其后若还有行内内容则保留为间隙空格；若后面是块级
			// 子元素则丢弃（CSS 移除行尾空格）。
			if hasTrailingBlank(raw) && (len(toks) > 0 || lastInline) {
				pendingSpace = true
				pendingTN = tnc
			}
			continue
		}

		che := inner(child)
		if che == nil || che.computed == nil {
			continue
		}
		cdisp := che.computed.Display()
		if cdisp == DisplayNone {
			continue
		}
		if che.computed.Position().isOutFlow() {
			// absolute/fixed 脱离常规流：不推进游标、不占行高，
			// 由 layoutPositioned 第二 pass 定位（嵌套脱流盒在其子树内同样被跳过）。
			continue
		}

		if cdisp == DisplayBlock {
			flushLine()
			// 相邻块级兄弟 margin 折叠（CSS 2.1 §8.3.1）：间隙取
			// max(前兄 margin-bottom, 本子 margin-top) 而非两者之和。
			// 游标已含前兄 mb（layoutH 含 margin），回退两者较小值即等效。
			//
			// 本子块的有效外边距：再与首/末个流内块级子块折叠一层（父-子
			// 穿越折叠，§8.3.1）——子块无上/下边框与内边距时，父子外边距
			// 折叠取大者，子块自身随后不再重复计入（置位其 mt/mbAbsorbed）。
			// 折叠值回填子块（effTop/effBot），其自身布局与此处兄弟折叠共用
			// 同一口径。
			mgc := resolveEdgeRect(che.computed.Margin(), contentW)
			effTop, effBot := mgc[eTop], mgc[eBottom]
			cheBD := resolveEdgeRect(che.computed.BorderStyleWidth(), contentW)
			chePD := resolveEdgeRect(che.computed.Padding(), contentW)
			if c := firstFlowBlockChild(che); c != nil && collapsesWithFirstChild(che, cheBD, chePD) {
				cm := resolveEdgeRect(c.computed.Margin(), contentW)
				if cm[eTop] > effTop {
					effTop = cm[eTop]
				}
				c.mtAbsorbed = true
			}
			if c := lastFlowBlockChild(che); c != nil && collapsesWithLastChild(che, cheBD, chePD) {
				cm := resolveEdgeRect(c.computed.Margin(), contentW)
				if cm[eBottom] > effBot {
					effBot = cm[eBottom]
				}
				c.mbAbsorbed = true
			}
			che.effTop, che.effBot, che.hasEff = effTop, effBot, true

			if prevBlockMB >= 0 {
				cursorY -= minInt(prevBlockMB, effTop)
			}
			layoutBox(g, che, contentW, maxZero(contentH-(cursorY-cy)), cx, cursorY, contentW, pctH)
			cursorY += che.layoutH
			prevBlockMB = effBot
			// 块级子元素结束行内格式上下文：行尾空格被 CSS 移除，且后续内容
			// 不再与前面的行内内容共享间隙空格。
			pendingSpace = false
			lastInline = false
			// 块子的真实外盒宽计入 shrink-wrap 宽度统计：游标换行后复位到 cx，
			// 原先漏算块宽（inline-block 容器仅含块子时会收缩到近似 0 宽）。
			// 非裁剪盒（overflow:visible）另按「未裁剪内容宽」参与 maxRight：
			// 祖先的 contentW 链式传播到根元素，是文档级横向滚动（宽块溢出
			// 视口）的依据；overflow 非 visible 的盒自身即裁剪边界，内容不外传。
			w := cx + outerBoxW(che, contentW)
			if !isClippingBox(che) {
				if ext := che.contentW + (outerBoxW(che, contentW) - che.width); ext > w-cx {
					w = cx + ext
				}
			}
			if w > maxRight {
				maxRight = w
			}
			continue
		}

		// inline / inline-block → 原子盒。先在游标处试布局，溢出则换行重排。
		// 行内盒的位置由 layoutBox 按游标决定——待定空格必须先落位。
		flushPendingSpace()
		// 行内盒不参与父-子 margin 折叠：清掉可能残留的折叠值。
		che.hasEff = false
		remain := contentW - (cursorX - cx)
		if remain < 0 {
			remain = 0
		}
		ow, oh := layoutBox(g, che, remain, maxZero(contentH-(cursorY-cy)), cursorX, cursorY, contentW, pctH)
		if ow <= 0 && oh <= 0 {
			continue // 零尺寸且无外盒的原子盒（如空 head）不占行高
		}
		if len(curLine) > 0 && cursorX+ow-cx > contentW && contentW > 0 {
			flushLine()
			ow, oh = layoutBox(g, che, contentW, maxZero(contentH-(cursorY-cy)), cx, cursorY, contentW, pctH)
		}
		it := lineItem{x: cursorX, y: cursorY, w: maxInt(ow, 1), h: maxInt(oh, 1), el: che}
		addItem(it)
	}

	flushLine()
	return maxRight - cx, cursorY - cy
}

// hasLeadingBlank 报告文本是否以空白开头（CSS 空白字符集）。
func hasLeadingBlank(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', ' ':
			return true
		}
		return false
	}
	return false
}

// hasTrailingBlank 报告文本是否以空白结尾。
func hasTrailingBlank(s string) bool {
	rs := []rune(s)
	for i := len(rs) - 1; i >= 0; i-- {
		switch rs[i] {
		case ' ', '\t', '\n', '\r', '\f', ' ':
			return true
		}
		return false
	}
	return false
}

// tokenizeText 将文本切为可断行的词：
// CJK 按单字、ASCII 连续串按整词、空白折叠为单个空格（首尾不产生空格）。
func tokenizeText(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	pendingSpace := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\u00a0':
			flush()
			if len(out) > 0 {
				pendingSpace = true
			}
		case r < 0x80:
			if pendingSpace {
				out = append(out, " ")
				pendingSpace = false
			}
			cur = append(cur, r)
		default:
			if pendingSpace {
				out = append(out, " ")
				pendingSpace = false
			}
			flush()
			out = append(out, string(r))
		}
	}
	flush()
	return out
}
