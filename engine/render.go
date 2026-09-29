package engine

import (
	"math"
	"sort"
	"strings"
)

// CanvasBackground 实现 CSS 画布背景传播（canvas background propagation）：
// 画布底色取根元素（html）的背景，根无背景时退到 body。返回 nil 表示两者
// 都没有（调用方用默认白底）。真实浏览器正是靠这条规则让 body 的浅色背景
// 铺满整个窗口，而不是只铺到内容高度。
func CanvasBackground(doc HTMLDocument) Color {
	base := inner(doc)
	if base == nil {
		return nil
	}
	if bg := opaqueBackgroundColor(base); bg != nil {
		return bg
	}
	for _, c := range base.children {
		if el := inner(c); el != nil && strings.EqualFold(el.TagName(), "body") {
			return opaqueBackgroundColor(el)
		}
	}
	return nil
}

// opaqueBackgroundColor 取元素背景色；未设置（nil）或全透明视为无背景。
func opaqueBackgroundColor(e *htmlElement) Color {
	if e == nil || e.computed == nil {
		return nil
	}
	bg := e.computed.BackgroundColor()
	if bg == nil {
		return nil
	}
	if _, _, _, a := bg.RGBA(); a == 0 {
		return nil
	}
	return bg
}

// positionedBox 是定位层（第二层）的收集项：非 static 元素及其层序信息。
type positionedBox struct {
	e     HTMLElement
	z     int
	order int
}

// isPositioned 判断已级联元素是否进入定位层（position != static）。
func isPositioned(e HTMLElement) bool {
	b := inner(e)
	return b != nil && b.computed != nil && b.computed.Position() != PositionStatic
}

// collectPositioned 按文档序收集全部非 static 后代，再按
// (z-index 升序，文档序稳定) 排序——渲染与 SVG 导出共用同一层叠模型。
// display:none 子树整体跳过（其内定位元素也不参与层叠）。
func collectPositioned(root HTMLElement) []positionedBox {
	var out []positionedBox
	order := 0
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		b := inner(e)
		if b == nil {
			return
		}
		for _, c := range b.children {
			cb := inner(c)
			if cb == nil || cb.computed == nil {
				continue
			}
			if cb.computed.Display() == DisplayNone {
				continue
			}
			if isPositioned(c) {
				out = append(out, positionedBox{e: c, z: cb.computed.ZIndex(), order: order})
				order++
			}
			walk(c)
		}
	}
	walk(root)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].z != out[j].z {
			return out[i].z < out[j].z
		}
		return out[i].order < out[j].order
	})
	return out
}

// RenderNode 从根节点开始绘制整棵树。g 为渲染后端。
//
// 层叠简化模型（对齐浏览器主案例）：常规流内容按文档序先绘；
// 所有非 static 元素（relative/absolute/fixed，含默认 z:auto）整体盖在
// 静态内容之上，按 collectPositioned 的 (z-index，文档序) 逐个绘制。
// 定位子树统一由本 pass 绘制：主 pass 递归遇到 positioned 子节点直接跳过。
// 未实现嵌套层叠上下文（opacity/transform 成组、负 z 压到背景下等细节），
// 需要时再扩展。
func RenderNode(g Graphics, node HTMLElement) {
	if g == nil {
		g = NewFakeGraphics()
	}
	var focus HTMLElement
	if d, ok := node.(*htmlDocument); ok {
		focus = d.focus
	}
	renderNode(g, node, nil, focus)
	for _, pb := range collectPositioned(node) {
		// paint 传 nil 无损：继承字段（color/font/text-align）在级联时已
		// 下推到每个元素的 computed，renderNode 内部会用自身 comp 组装。
		renderNode(g, pb.e, nil, focus)
	}
}

// focusBlue 是 v1 硬编码的聚焦框颜色（演示主题主色）。
var focusBlue = NewColor(37, 99, 235, 255)

func renderNode(g Graphics, e HTMLElement, parentPaint Paint, focus HTMLElement) {
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
		p.SetBackground(parentPaint.Background())
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
		// 背景供文字合成用：自身有实底则覆盖继承值（文字落在本元素背景上）。
		// 半透明背景不做精确合成（LCD 仍用它近似），v1 取实底即可。
		if c := comp.BackgroundColor(); c != nil {
			if _, _, _, a := c.RGBA(); a >= 250 {
				p.SetBackground(c)
			}
		}
	}

	// 文本节点：绘制布局阶段生成的行片段
	if tn, ok := e.(*textNode); ok {
		for _, run := range tn.node.runs {
			g.DrawText(run.x, run.y, run.w, run.h, p, run.text)
		}
		return
	}

	focused := focus != nil && inner(focus) == base

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

		// input：按 type 分支渲染。text/password 画 value 文本 + 常亮光标 +
		// 聚焦蓝框（最小输入闭环 v1；文本左对齐、垂直居中，宽度溢出时舍头
		// 保留尾——光标在尾部，所见即正在输入的末尾）。
		// submit/reset/button 画按钮（盒体样式已由 UA 默认给出），
		// value 文本水平+垂直居中。radio/checkbox 仅画控件盒（白底/灰边/
		// 圆角——radio 全圆，均由上方通用路径按 UA 样式绘制），v1 不含
		// checked 选中标记与点击切换。
		if strings.EqualFold(base.tagName, "input") && bw > 0 && bh > 0 {
			typ := strings.ToLower(base.GetAttribute("type"))
			switch typ {
			case "radio", "checkbox":
			default:
				size := p.Size().Pixel()
				val := base.GetAttribute("value")
				if typ == "password" && val != "" {
					// 密码掩码：每位一个实心圆点（与浏览器一致）
					val = strings.Repeat("●", len([]rune(val)))
				}
				tw, th := g.MeasureText(val, size, p.Bold(), p.FontFamily())
				runes := []rune(val)
				// 按钮型不截断：auto 宽已按 value 外盒测量，两侧 padding 即呼吸
				// 空间（Chrome 同语义）；文本型保留 -4 余量给光标。
				buttonLike := typ == "submit" || typ == "reset" || typ == "button"
				for !buttonLike && tw > base.width-4 && len(runes) > 1 {
					runes = runes[1:]
					val = string(runes)
					tw, th = g.MeasureText(val, size, p.Bold(), p.FontFamily())
				}
				tx := base.x
				if buttonLike && base.width > tw {
					// 按钮型 value 文本水平居中（对齐 Chrome 按钮文字）
					tx = base.x + (base.width-tw)/2
				}
				ty := base.y
				if th < base.height {
					ty += (base.height - th) / 2
				}
				if val != "" {
					g.DrawText(tx, ty, tw, th, p, val)
				}
				if focused {
					// 光标：紧跟文本尾部的 1px 竖线（文本色）
					cc := p.Color()
					if cc == nil {
						cc = NewColor(0, 0, 0, 255)
					}
					if th > 2 {
						g.DrawColor(tx+tw+1, ty, 1, th-2, cc)
					}
					strokeRoundedRect(g, bx, by, bw, bh, r, edges{1, 1, 1, 1}, focusBlue)
				}
			}
		}
	}

	for _, child := range base.children {
		if isPositioned(child) {
			continue // 定位子树由 RenderNode 的层叠 pass 统一绘制
		}
		renderNode(g, child, p, focus)
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
