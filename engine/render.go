package engine

// RenderNode 从根节点开始绘制整棵树。g 为渲染后端。
func RenderNode(g Graphics, node HTMLElement) {
	if g == nil {
		g = NewFakeGraphics()
	}
	renderNode(g, node, nil)
}

func renderNode(g Graphics, e HTMLElement, parentPaint Paint) {
	if e == nil {
		return
	}
	base := inner(e)
	if base == nil {
		return
	}
	comp := base.computed
	disp := DisplayInlineBlock
	if comp != nil {
		disp = comp.Display()
	}
	if disp == DisplayNone {
		return
	}

	// 组装本节点绘制属性：继承 + 自身样式
	p := NewPaint()
	if parentPaint != nil {
		p.SetSize(parentPaint.Size())
		p.SetColor(parentPaint.Color())
		p.SetFontFamily(parentPaint.FontFamily())
		p.SetBold(parentPaint.Bold())
	}
	if comp != nil {
		if c := comp.Color(); c != nil {
			p.SetColor(c)
		}
		if fs := resolveLen(comp.FontSize(), 0); fs > 0 {
			p.SetSize(NewSize(SIZE_PIXEL, fs, 0))
		}
		p.SetFontFamily(comp.FontFamily())
		p.SetBold(comp.FontWeight() == FontWeightBold)
	}

	// 文本节点：绘制布局阶段生成的行片段
	if tn, ok := e.(*textNode); ok {
		for _, run := range tn.node.runs {
			g.DrawText(run.x, run.y, run.w, run.h, p, run.text)
		}
		return
	}

	// 元素：背景与边框（外扩到边框盒）
	if comp != nil {
		bd := resolveEdgeRect(comp.BorderStyleWidth(), 0, 0)
		pd := resolveEdgeRect(comp.Padding(), 0, 0)
		bx := base.x - pd[eLeft] - bd[eLeft]
		by := base.y - pd[eTop] - bd[eTop]
		bw := base.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
		bh := base.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]

		if bg := comp.BackgroundColor(); bg != nil {
			g.DrawColor(bx, by, bw, bh, bg)
		}
		if bc := comp.BorderColor(); bc != nil && comp.BorderStyle() != BorderStyleNone {
			if bd[eLeft] > 0 {
				g.DrawColor(bx, by, bd[eLeft], bh, bc)
			}
			if bd[eRight] > 0 {
				g.DrawColor(bx+bw-bd[eRight], by, bd[eRight], bh, bc)
			}
			if bd[eTop] > 0 {
				g.DrawColor(bx, by, bw, bd[eTop], bc)
			}
			if bd[eBottom] > 0 {
				g.DrawColor(bx, by+bh-bd[eBottom], bw, bd[eBottom], bc)
			}
		}
	}

	for _, child := range base.children {
		renderNode(g, child, p)
	}
}
