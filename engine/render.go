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
		// 定位子树不在主 pass 递归里，祖先的 overflow 裁剪必须按父链
		// 重新压栈，否则滚动容器里的脱流内容会画到盒外。
		n := pushAncestorClips(g, pb.e)
		renderNode(g, pb.e, nil, focus)
		popClips(g, n)
	}
	// select 选项浮层最后绘制：它覆盖其后的流内容与定位层，
	// 与命中测试（ElementAt 优先查展开 option）保持同一层序语义。
	// 浮层豁免裁剪（对齐浏览器原生下拉：可画到滚动容器/视口之外）。
	for _, sel := range collectOpenSelects(node) {
		renderSelectPopup(g, sel, focus)
	}
}

// pushAncestorClips 沿父链为定位元素压入全部 overflow 裁剪盒，
// 返回实际压栈层数（调用方画完后用 popClips 逐层出栈，保证严格配对）。
func pushAncestorClips(g Graphics, e HTMLElement) int {
	var chain []*htmlElement
	for p := e.ParentElement(); p != nil; p = p.ParentElement() {
		if b := inner(p); b != nil && isClippingBox(b) {
			chain = append(chain, b)
		}
	}
	n := 0
	for i := len(chain) - 1; i >= 0; i-- {
		b := chain[i]
		pd, _ := resolvedEdges(b, b.computed)
		if pushClip(g, b.x-pd[eLeft], b.y-pd[eTop],
			b.width+pd[eLeft]+pd[eRight], b.height+pd[eTop]+pd[eBottom]) {
			n++
		}
	}
	return n
}

func popClips(g Graphics, n int) {
	for i := 0; i < n; i++ {
		popClip(g)
	}
}

// focusBlue 是聚焦框与控件强调色的取值：Chrome 的表单控件用**系统强调色**
// （Windows 默认 #0078D4，随用户配色变化）；引擎取 Windows 默认强调色硬编码，
// 使聚焦框与 radio/checkbox 的选中标记与浏览器观感一致。
var focusBlue = NewColor(0x00, 0x78, 0xD4, 255)

func renderNode(g Graphics, e HTMLElement, parentPaint Paint, focus HTMLElement) {
	renderNodeTextColor(g, e, parentPaint, focus, nil)
}

// renderNodeTextColor 与 renderNode 同构，但用 textColor 强制覆盖本子树的
// 文字绘制色（select 展开浮层的选中项：#1967D2 高亮底上出白字，Chrome 同款）。
// 覆盖作用于 Paint（文字绘制与子树继承，含级联到 textNode 的值）；元素自身
// 背景/边框仍按 computed 绘制，表单控件专用绘制分支不读该 Paint，不受影响。
func renderNodeTextColor(g Graphics, e HTMLElement, parentPaint Paint, focus HTMLElement, textColor Color) {
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
	if textColor != nil {
		p.SetColor(textColor)
	}

	// 文本节点：绘制布局阶段生成的行片段 + 文本装饰（下划线/删除线）
	if tn, ok := e.(*textNode); ok {
		dec := TextDecorationNone
		if comp != nil {
			dec = comp.TextDecoration()
		}
		for _, run := range tn.node.runs {
			g.DrawText(run.x, run.y, run.w, run.h, p, run.text)
			if dec != TextDecorationNone && run.w > 0 && run.h > 0 {
				drawDecoration(g, dec, run.x, run.y, run.w, run.h, run.asc, p.Size().Pixel(), p)
			}
		}
		return
	}

	focused := focus != nil && inner(focus) == base

	// overflow 裁剪盒在背景/边框之后压栈：边框沿 padding 盒外侧绘制，
	// 属于盒子自身外观，不应被裁掉；真正要裁的是控件内容与子树。
	clipped := false

	// 元素：背景与边框（外扩到边框盒）
	if comp != nil {
		pd, bd := resolvedEdges(base, comp)
		bx := base.x - pd[eLeft] - bd[eLeft]
		by := base.y - pd[eTop] - bd[eTop]
		bw := base.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
		bh := base.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]

		r := resolveRadius(comp, bw, bh)
		// radio 原生外观恒为正圆：UA 的 7px 半径只适配 13px 默认尺寸，
		// 作者改写宽高后仍按圆绘制（Chrome 同口径），也与选中标记的圆形几何一致。
		// 取「半宽向上取整」而非 min/2：13px → 7 与原 UA 值一致（r 略大于半宽
		// 时圆角盒 SDF 退化为整圆），24/40px 等改写尺寸同样成圆。
		if checkKind(base) == "radio" {
			r = (minInt(bw, bh) + 1) / 2
		}

		if bg := comp.BackgroundColor(); bg != nil {
			fillRoundedRect(g, bx, by, bw, bh, r, bg)
		}
		if bc := comp.BorderColor(); bc != nil && comp.BorderStyle() != BorderStyleNone && bw > 0 && bh > 0 {
			strokeRoundedRect(g, bx, by, bw, bh, r, bd, bc)
		}

		// 聚焦框画在边框盒上，必须早于下方 overflow 裁剪压栈：textarea 默认
		// overflow:hidden，裁剪区是 padding 盒，画在边框盒上的聚焦框会被整圈
		// 裁掉（此前聚焦框在裁剪栈内绘制 → textarea 永远看不到聚焦指示）。
		// 单选/复选此前就没有聚焦框（Chrome 同款圆环由选中标记承担），保持。
		if focused && isFormControl(base) && bw > 0 && bh > 0 && checkKind(base) == "" {
			strokeRoundedRect(g, bx, by, bw, bh, r, edges{1, 1, 1, 1}, focusBlue)
		}

		// 列表标记：引擎无 list-item 概念，标记文本由布局期算出（ul → "•"，
		// ol → "1." "2."…），画在内容左缘左侧、右对齐到 −0.75em 处
		//（Chrome 实测：标记盒右缘 = 内容左缘 − 0.5em）。
		if mx, my, mw, mh, ok := listMarkerBox(g, base, comp); ok {
			g.DrawText(mx, my, mw, mh, p, base.listMarker)
		}

		// overflow 非 visible：裁剪区为 padding 盒（CSS 语义），包住下方
		// 的控件内容与全部子元素；非 Clipper 后端或退化盒（w/h ≤ 0）此处
		// 不压栈，clipped 保持 false 与 popClip 严格配对。
		if isClippingBox(base) {
			clipped = pushClip(g, bx+bd[eLeft], by+bd[eTop],
				bw-bd[eLeft]-bd[eRight], bh-bd[eTop]-bd[eBottom])
		}

		// 表单控件：input 按 type 分支（text/password 画 value 文本 + 光标 +
		// 聚焦蓝框，宽度溢出舍头保尾；按钮型居中不截断；radio/checkbox 选中
		// 时补画强调色与勾选标记）；textarea 画折行文本 + 光标；
		// select 画选中文本 + 右侧下拉箭头。
		if isFormControl(base) && bw > 0 && bh > 0 {
			typ := strings.ToLower(base.GetAttribute("type"))
			switch {
			case strings.EqualFold(base.tagName, "select"):
				renderSelectValue(g, base, comp, p, bx, by, bw, bh, r)
			case strings.EqualFold(base.tagName, "textarea"):
				renderTextareaValue(g, base, comp, p, focused)
			case typ == "radio", typ == "checkbox":
				// 控件盒（背景/边框/圆角）已由上方通用路径按 UA 样式绘制；
				// 选中时补画强调色底与对勾/圆环（Chrome 同构，见 check.go）
				if hasChecked(base) {
					renderCheckedMark(g, bx, by, bw, bh, r, typ)
				}
			default:
				size := p.Size().Pixel()
				// password 已在此按 ● 掩码（displayValue），下方测量/截断/
				// 光标前缀宽全部以掩码文本为口径。可见窗口（舍头保尾）与光标
				// 映射共用 inputVisibleWindow——同一口径，避免光标落到被裁掉的
				// 前缀里（插入字符会落到错误位置）。
				val, cut, tw, th := inputVisibleWindow(g, base, size, p.Bold(), p.FontFamily())
				fullRunes := []rune(displayValue(base))
				// 按钮型 value 文本水平居中（对齐 Chrome 按钮文字）；按钮型
				// 不截断（auto 宽已按 value 外盒测量），故此处不会被裁短。
				buttonLike := typ == "submit" || typ == "reset" || typ == "button"
				tx := base.x
				if buttonLike && base.width > tw {
					tx = base.x + (base.width-tw)/2
				}
				ty := base.y
				if th < base.height {
					ty += (base.height - th) / 2
				}
				if focused {
					// 选区高亮先于文字绘制：半透明蓝压在背景上，文字仍按原色
					// 绘制。矩形前缀宽与可见窗口同口径（val/cut），超宽输入
					// 的选区不会错位到被裁掉的前缀。
					if lo, hi, ok := selectionRange(base); ok {
						for _, sr := range inputSelectionRects(g, val, cut, tx, ty, th,
							size, p.Bold(), p.FontFamily(), lo, hi) {
							g.DrawColor(sr.x, sr.y, sr.w, sr.h, selectionColor)
						}
					}
				}
				if val != "" {
					g.DrawText(tx, ty, tw, th, p, val)
				}
				if focused {
					// 光标：1px 竖线（文本色）。默认在值末尾；点击中部后按
					// 可见前缀宽度定位，落在被裁前缀里则贴左缘。
					cc := p.Color()
					if cc == nil {
						cc = NewColor(0, 0, 0, 255)
					}
					cx := tx + tw
					if c := base.caret; c >= 0 && c < len(fullRunes) {
						if c > cut {
							pw, _ := g.MeasureText(string(fullRunes[cut:c]), size, p.Bold(), p.FontFamily())
							cx = tx + pw
						} else {
							cx = tx
						}
					}
					if th > 2 {
						g.DrawColor(cx, ty, 1, th-2, cc)
					}
				}
			}
		}
	}

	for _, child := range base.children {
		if isPositioned(child) {
			continue // 定位子树由 RenderNode 的层叠 pass 统一绘制
		}
		if isFormControl(base) {
			continue // 控件内容由专用绘制负责（select 选项另走浮层 pass）
		}
		renderNodeTextColor(g, child, p, focus, textColor)
	}
	if clipped {
		popClip(g)
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

// drawDecoration 绘制文本装饰（下划线/删除线）：以文本色画一条实线。
// 下划线按基线定位（ascent + 厚度），不能按片段盒底定位：SDL_ttf 的 descent
// 比旧字体表口径小，盒底离基线更近，按盒底画会显得下划线偏高。
func drawDecoration(g Graphics, d TextDecoration, x, y, w, h, ascent, fontSize int, p Paint) {
	if d == TextDecorationNone || w <= 0 || h <= 0 {
		return
	}
	if fontSize <= 0 {
		fontSize = 16
	}
	th := fontSize/14 + 1
	var dy int
	switch d {
	case TextDecorationUnderline:
		// 基线下方留一个线厚的间距；允许略超出片段盒（下线本就在基线下方，
		// 而盒底的 descent 可能只有 2~3px）。
		dy = ascent + th
	case TextDecorationLineThrough:
		dy = h/2 - th/2
	default:
		return
	}
	if dy < 0 {
		dy = 0
	}
	if d == TextDecorationLineThrough && dy+th > h {
		dy = h - th
	}
	if dy < 0 {
		return
	}
	c := NewColor(0, 0, 0, 255)
	if p != nil && p.Color() != nil {
		c = p.Color()
	}
	g.DrawColor(x, y+dy, w, th, c)
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
