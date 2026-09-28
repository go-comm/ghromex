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
	layoutBox(g, base, mw, mh, 0, 0)
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
	layoutBox(g, base, mw, mh, 0, 0)
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
// ox/oy 为外边距盒原点。返回外边距盒宽高，并写入内容盒的 x/y/width/height 与 layoutH。
func layoutBox(g Graphics, el *htmlElement, outerW, outerH, ox, oy int) (retW, retH int) {
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

	mg := resolveEdgeRect(comp.Margin(), outerW, outerH)
	bd := resolveEdgeRect(comp.BorderStyleWidth(), outerW, outerH)
	pd := resolveEdgeRect(comp.Padding(), outerW, outerH)

	// 内容宽度
	contentW := resolveLen(comp.Width(), outerW)
	shrinkWrap := false
	if contentW < 0 { // auto
		contentW = outerW - mg[eLeft] - mg[eRight] - bd[eLeft] - bd[eRight] - pd[eLeft] - pd[eRight]
		if contentW < 0 {
			contentW = 0
		}
		if disp != DisplayBlock {
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

		if cdisp == DisplayBlock {
			flushLine()
			layoutBox(g, che, contentW, maxZero(contentH-(cursorY-cy)), cx, cursorY)
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
		ow, oh := layoutBox(g, che, remain, maxZero(contentH-(cursorY-cy)), cursorX, cursorY)
		if ow <= 0 && oh <= 0 {
			continue // 零尺寸且无外盒的原子盒（如空 head）不占行高
		}
		if len(curLine) > 0 && cursorX+ow-cx > contentW && contentW > 0 {
			flushLine()
			ow, oh = layoutBox(g, che, contentW, maxZero(contentH-(cursorY-cy)), cx, cursorY)
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
