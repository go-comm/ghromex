package engine

import "math"

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

		r := resolveRadius(comp, bw, bh)

		if bg := comp.BackgroundColor(); bg != nil {
			fillRoundedRect(g, bx, by, bw, bh, r, bg)
		}
		if bc := comp.BorderColor(); bc != nil && comp.BorderStyle() != BorderStyleNone && bw > 0 && bh > 0 {
			strokeRoundedRect(g, bx, by, bw, bh, r, bd, bc)
		}
	}

	for _, child := range base.children {
		renderNode(g, child, p)
	}
}

// ---------------------------------------------------------------------------
// 圆角绘制：整数扫描线 + 边缘像素覆盖率抗锯齿（AA）。
// 内部仍用 roundedSpanX 合并整段实色（绘制调用数与无 AA 版同量级），
// 只对标弧附近的边缘像素逐点算 SDF 距离→覆盖率，以「颜色×覆盖率 alpha」
// 画 1px 矩形交给后端的 alpha 混合完成平滑：
// Graphics 接口无需新增图元，SDL2/Buffer/Fake 三个后端自动受益。
// SVG 导出走 rx 属性，由浏览器真矢量 AA，不在此列。
// ---------------------------------------------------------------------------

// resolveRadius 解析圆角像素值（百分比以小盒边为基），限制在可绘制范围。
func resolveRadius(comp CSSStyleDeclaration, bw, bh int) int {
	if bw <= 0 || bh <= 0 {
		return 0
	}
	r := resolveLen(comp.BorderRadius(), minInt(bw, bh))
	if r <= 0 {
		return 0
	}
	if c := minInt(bw, bh) / 2; r > c {
		r = c
	}
	return r
}

// roundedSpanX 计算圆角矩形在第 y 行覆盖的闭区间 [x0,x1]；空行返回 ok=false。
// 前提：0 < r <= min(bw,bh)/2。
func roundedSpanX(bx, by, bw, bh, r, y int) (x0, x1 int, ok bool) {
	if bw <= 0 || bh <= 0 || y < by || y >= by+bh {
		return 0, 0, false
	}
	x0, x1 = bx, bx+bw-1
	dy := -1
	if yy := y - by; yy < r {
		dy = r - 1 - yy
	} else if yyb := by + bh - 1 - y; yyb < r {
		dy = r - 1 - yyb
	}
	if dy >= 0 {
		inset := r - int(math.Sqrt(float64(r*r-dy*dy)))
		x0 += inset
		x1 -= inset
	}
	if x0 > x1 {
		return 0, 0, false
	}
	return x0, x1, true
}

// fillRoundedRect 填充圆角矩形（带边缘 AA）；r=0 时保持平面单次填充的快速路径。
func fillRoundedRect(g Graphics, bx, by, bw, bh, r int, c Color) {
	if bw <= 0 || bh <= 0 {
		return
	}
	if r <= 0 {
		g.DrawColor(bx, by, bw, bh, c)
		return
	}
	for y := by; y < by+bh; y++ {
		x0, x1, ok := roundedSpanX(bx, by, bw, bh, r, y)
		if !ok {
			continue
		}
		py := float64(y) + 0.5
		lo := maxInt(bx, x0-2)
		hi := minInt(bx+bw-1, x1+2)
		paintCoverage(g, y, lo, hi, c, func(x int) float64 {
			return coverageAt(float64(x)+0.5, py, bx, by, bw, bh, r)
		})
	}
}

// coverageAt 返回像素点 (px,py) 对圆角矩形的覆盖率 [0,1]。
// 用圆角盒标准 SDF（Inigo Quilez rounded box）：d<0 为内部，
// 以 1px 线性 AA 带映射：d<=-0.5 全覆，d>=0.5 全空。
// r=0 退化为普通矩形 SDF，天然支持。
func coverageAt(px, py float64, bx, by, bw, bh, r int) float64 {
	hw := float64(bw) / 2
	hh := float64(bh) / 2
	fr := float64(r)
	qx := math.Abs(px-(float64(bx)+hw)) - (hw - fr)
	qy := math.Abs(py-(float64(by)+hh)) - (hh - fr)
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - fr
	if d <= -0.5 {
		return 1
	}
	if d >= 0.5 {
		return 0
	}
	return 0.5 - d
}

// scaledAlpha 按覆盖率降低颜色 alpha；全透明返回 ok=false。
// 色相分量保持不变，只改透明度，叠加在后端 src-over 混合下即得 AA。
func scaledAlpha(c Color, cov float64) (Color, bool) {
	r, g, b, a := c.RGBA()
	na := uint32(float64(a)*cov + 0.5)
	switch {
	case na == 0:
		return nil, false
	case na >= 255:
		return c, true
	}
	return NewColor(r, g, b, uint8(na)), true
}

// paintCoverage 扫描第 y 行 [x0,x1]：全覆像素合并成实色段一次绘制，
// 部分覆盖像素逐点按 alpha 绘制；调用区间由 roundedSpanX 加余量限定，
// 逐点成本仅在弧边附近，远低于全盒扫描。
func paintCoverage(g Graphics, y, x0, x1 int, c Color, cov func(x int) float64) {
	runStart := -1
	for x := x0; x <= x1; x++ {
		if cv := cov(x); cv >= 1 {
			if runStart < 0 {
				runStart = x
			}
		} else {
			if runStart >= 0 {
				g.DrawColor(runStart, y, x-runStart, 1, c)
				runStart = -1
			}
			if ec, ok := scaledAlpha(c, cv); ok {
				g.DrawColor(x, y, 1, 1, ec)
			}
		}
	}
	if runStart >= 0 {
		g.DrawColor(runStart, y, x1+1-runStart, 1, c)
	}
}

// strokeRoundedRect 绘制圆角边框环（外圆角 r、内圆角 r-边框宽，带 AA）；
// 环覆盖率 = 外盒覆盖 - 内盒覆盖。r=0 时与原平面四矩形路径一致。
func strokeRoundedRect(g Graphics, bx, by, bw, bh, r int, bd edges, c Color) {
	if r <= 0 {
		if bd[eLeft] > 0 {
			g.DrawColor(bx, by, bd[eLeft], bh, c)
		}
		if bd[eRight] > 0 {
			g.DrawColor(bx+bw-bd[eRight], by, bd[eRight], bh, c)
		}
		if bd[eTop] > 0 {
			g.DrawColor(bx, by, bw, bd[eTop], c)
		}
		if bd[eBottom] > 0 {
			g.DrawColor(bx, by+bh-bd[eBottom], bw, bd[eBottom], c)
		}
		return
	}
	li, rt, tp, bt := maxZero(bd[eLeft]), maxZero(bd[eRight]), maxZero(bd[eTop]), maxZero(bd[eBottom])
	ix, iy := bx+li, by+tp
	iw, ih := bw-li-rt, bh-tp-bt
	ir := r - minInt(minInt(minInt(li, rt), tp), bt)
	if ir < 0 {
		ir = 0
	}
	if iw > 0 && ih > 0 {
		if c := minInt(iw, ih) / 2; ir > c {
			ir = c
		}
	}
	hasInner := iw > 0 && ih > 0
	for y := by; y < by+bh; y++ {
		ox0, ox1, ok := roundedSpanX(bx, by, bw, bh, r, y)
		if !ok {
			continue
		}
		py := float64(y) + 0.5
		cov := func(x int) float64 {
			cv := coverageAt(float64(x)+0.5, py, bx, by, bw, bh, r)
			if cv <= 0 || !hasInner {
				return cv
			}
			cv -= coverageAt(float64(x)+0.5, py, ix, iy, iw, ih, ir)
			if cv < 0 {
				cv = 0
			}
			if cv > 1 {
				cv = 1
			}
			return cv
		}
		segLo := maxInt(bx, ox0-2)
		segHi := minInt(bx+bw-1, ox1+2)
		if hasInner {
			if iLo, iHi, iok := roundedSpanX(ix, iy, iw, ih, ir, y); iok {
				// 内部全覆区不属环，只扫两侧（含内弧余量）；环很薄时两段会相接，合并避免重绘
				mid := minInt(segHi, iLo+1)
				if mid < segLo {
					mid = segLo
				}
				right := maxInt(segLo, iHi-1)
				if right > segHi {
					right = segHi
				}
				paintCoverage(g, y, segLo, mid, c, cov)
				if right > mid+1 {
					paintCoverage(g, y, right, segHi, c, cov)
				}
				continue
			}
		}
		paintCoverage(g, y, segLo, segHi, c, cov)
	}
}
