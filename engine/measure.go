package engine

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
	layoutBox(g, base, mw, mh, 0, 0, mw)
	layoutPositioned(g, base, mw, mh)
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
	layoutBox(g, base, mw, mh, 0, 0, mw)
	layoutPositioned(g, base, mw, mh)
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

func resolveEdgeRect(r Rect, baseW, baseH int) edges {
	var e edges
	if r == nil {
		return e
	}
	e[eTop] = maxZero(resolveLen(r.Top(), baseH))
	e[eRight] = maxZero(resolveLen(r.Right(), baseW))
	e[eBottom] = maxZero(resolveLen(r.Bottom(), baseH))
	e[eLeft] = maxZero(resolveLen(r.Left(), baseW))
	return e
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

// layoutBox 布局一个元素盒。
// outerW/outerH 为该盒在父容器内容区内可用的外边距盒空间；
// ox/oy 为外边距盒原点；pctW 为百分比解析基数（包含块内容宽，CSS 规定百分比
// width/padding 等相对包含块而非行内剩余空间，inline 分支不得混用）。
// 返回外边距盒宽高，并写入内容盒的 x/y/width/height 与 layoutH。
func layoutBox(g Graphics, el *htmlElement, outerW, outerH, ox, oy, pctW int) (retW, retH int) {
	comp := el.computed
	if comp == nil {
		comp = newStyle()
		el.computed = comp
	}
	disp := comp.Display()
	if disp == DisplayNone {
		el.layoutH = 0
		return 0, 0
	}

	mg := resolveEdgeRect(comp.Margin(), pctW, outerH)
	bd := resolveEdgeRect(comp.BorderStyleWidth(), pctW, outerH)
	pd := resolveEdgeRect(comp.Padding(), pctW, outerH)

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
		contentW = outerW - mg[eLeft] - mg[eRight] - bd[eLeft] - bd[eRight] - pd[eLeft] - pd[eRight]
		if contentW < 0 {
			contentW = 0
		}
		// 非块级与脱流盒（absolute/fixed）：auto 宽度按内容收缩
		if disp != DisplayBlock || comp.Position().isOutFlow() {
			shrinkWrap = true
		}
	}

	cx := ox + mg[eLeft] + bd[eLeft] + pd[eLeft]
	cy := oy + mg[eTop] + bd[eTop] + pd[eTop]
	el.setX(cx)
	el.setY(cy)

	remainH := outerH - mg[eTop] - mg[eBottom] - bd[eTop] - bd[eBottom] - pd[eTop] - pd[eBottom]
	// shrink-to-fit 盒的可用宽是收缩前的临时值，按它居中会把文本 run
	// 推到最终收缩后的盒外；而收缩后单行本就铺满盒宽，强制左对齐等价。
	align := comp.TextAlign()
	if shrinkWrap {
		align = TextAlignLeft
	}
	flowW, flowH := layoutChildren(g, el, contentW, remainH, cx, cy, align)

	contentH := resolveLen(comp.Height(), outerH)
	if contentH < 0 {
		contentH = flowH
	} else if borderBox {
		if ch := contentH - pd[eTop] - pd[eBottom] - bd[eTop] - bd[eBottom]; ch > 0 {
			contentH = ch
		} else {
			contentH = 0
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
		pd := resolveEdgeRect(pb.computed.Padding(), 0, 0)
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
	_, oh := layoutBox(g, el, maxInt(availX, 0), maxInt(availY, 0), ox, oy, cbW)
	if !lok && rok {
		// right 锚定：目标右缘 cbX+cbW-r 减去盒子当前位置（ox+真实宽）。
		// 注意必须含 ox：包含块不在原点时漏它会整体偏移 ox（CB 在 (40,40)
		// 的回归用例锁定）。
		mg := resolveEdgeRect(comp.Margin(), 0, 0)
		bd := resolveEdgeRect(comp.BorderStyleWidth(), 0, 0)
		pd := resolveEdgeRect(comp.Padding(), 0, 0)
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
// 偏移量可正可负；百分比 v1 不支持（base 传 0 自然归 0）。
func applyRelativeOffset(e HTMLElement, el *htmlElement) {
	in := el.computed.Inset()
	dx, dy := 0, 0
	if v, ok := resolveInset(in.Left(), 0); ok {
		dx = v
	} else if v, ok := resolveInset(in.Right(), 0); ok {
		dx = -v
	}
	if v, ok := resolveInset(in.Top(), 0); ok {
		dy = v
	} else if v, ok := resolveInset(in.Bottom(), 0); ok {
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
			applyRelativeOffset(child, che)
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

func layoutChildren(g Graphics, el *htmlElement, contentW, contentH, cx, cy int, align TextAlign) (flowW, flowH int) {
	cursorX := cx
	cursorY := cy
	lineH := 0
	var curLine []lineItem
	maxRight := cx

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
	}

	for _, child := range el.children {
		if tnc, ok := child.(*textNode); ok {
			tnc.node.runs = nil
			for _, tok := range tokenizeText(tnc.Text()) {
				tw, th := g.MeasureText(tok, fontPx, bold, family)
				if tw <= 0 {
					continue
				}
				addItem(lineItem{tn: tnc, token: tok, x: cursorX, y: cursorY, w: tw, h: th})
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
			layoutBox(g, che, contentW, maxZero(contentH-(cursorY-cy)), cx, cursorY, contentW)
			cursorY += che.layoutH
			if cursorX > maxRight {
				maxRight = cursorX
			}
			continue
		}

		// inline / inline-block → 原子盒。先在游标处试布局，溢出则换行重排。
		remain := contentW - (cursorX - cx)
		if remain < 0 {
			remain = 0
		}
		ow, oh := layoutBox(g, che, remain, maxZero(contentH-(cursorY-cy)), cursorX, cursorY, contentW)
		if ow <= 0 && oh <= 0 {
			continue // 零尺寸且无外盒的原子盒（如空 head）不占行高
		}
		if len(curLine) > 0 && cursorX+ow-cx > contentW && contentW > 0 {
			flushLine()
			ow, oh = layoutBox(g, che, contentW, maxZero(contentH-(cursorY-cy)), cx, cursorY, contentW)
		}
		it := lineItem{x: cursorX, y: cursorY, w: maxInt(ow, 1), h: maxInt(oh, 1), el: che}
		addItem(it)
	}

	flushLine()
	return maxRight - cx, cursorY - cy
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
