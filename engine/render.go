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

// collectPositioned 收集 root 当前层叠上下文里的定位元素，再按
// (z-index 升序，文档序稳定) 排序——渲染与 SVG 导出共用同一层叠模型。
//
// 关键点：z-index 非 0 的定位元素（以及 fixed）会形成新的层叠上下文，
// 其子树必须整体在祖先上下文里参与排序，不能把子元素拿到全局去和祖先的
// 兄弟比较。否则 fixed 顶栏 z:100 里的 relative/absolute 子树会被顶栏
// 自己的背景盖住（引擎早期就踩过这个坑）。遇到这类元素时只收集它本身，
// 不再向下走；它的子树由 paintPositioned 在该上下文内部递归收集。
//
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
				if createsStackingContext(c) {
					continue // 子树属于新的层叠上下文，由该元素自己画
				}
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

// createsStackingContext 判断定位元素是否建立新的层叠上下文。
// 引擎的 z-index 解析把 auto 与 0 都存成 0，因此这里把 0 当作 auto：
// 显式 z-index:0 的隔离少一层，但不影响常见的「顶栏 z:100 + 内部 absolute」
// 场景。position:fixed 按 CSS 规范无条件建立上下文。
func createsStackingContext(e HTMLElement) bool {
	b := inner(e)
	if b == nil || b.computed == nil {
		return false
	}
	p := b.computed.Position()
	if p == PositionFixed {
		return true
	}
	return p != PositionStatic && b.computed.ZIndex() != 0
}

// RenderNode 从根节点开始绘制整棵树。g 为渲染后端。
//
// 层叠模型（对齐浏览器主案例）：常规流内容按文档序先绘；定位元素盖在
// 静态内容之上，按 (z-index，文档序) 排序。z-index 非 0 的定位元素与
// fixed 元素会建立层叠上下文，其内部定位子树在该上下文内单独排序后再
// 整体落层，避免「祖先背景盖住自己 z:auto/absolute 子元素」。
// 定位子树由本 pass 绘制：主 pass 递归遇到 positioned 子节点直接跳过。
// 未实现 opacity/transform 成组、负 z 压到背景下等更细规则，需要时再扩展。
//
// 视口裁剪：绘制时跳过完全落在视口外的盒子（滚动时页面下半部分不再白
// 画一遍）。这是 Chrome「只合成可见瓦片」的等价物——滚动动画每帧都要
// 重画整页，不裁剪的话一帧上千次绘制调用，帧时间顶到 vsync 之外就掉帧。
func RenderNode(g Graphics, node HTMLElement) {
	if g == nil {
		g = NewFakeGraphics()
	}
	var focus HTMLElement
	var doc *htmlDocument
	vpW, vpH := 0, 0
	if d, ok := node.(*htmlDocument); ok {
		doc = d
		focus = d.focus
		if v := d.Viewport(); v != nil {
			vpW, vpH = v.ViewportWidth(), v.ViewportHeight()
		}
	}
	renderNode(g, node, nil, focus, vpW, vpH)
	for _, pb := range collectPositioned(node) {
		paintPositioned(g, pb.e, focus, vpW, vpH)
	}
	// select 选项浮层最后绘制：它覆盖其后的流内容与定位层，
	// 与命中测试（ElementAt 优先查展开 option）保持同一层序语义。
	// 浮层豁免裁剪（对齐浏览器原生下拉：可画到滚动容器/视口之外）。
	for _, sel := range collectOpenSelects(node) {
		renderSelectPopup(g, sel, focus)
	}
	// title 提示气泡压在最顶（select 浮层之上）：系统提示不该被任何页面
	// 内容盖住；可见性只由 TickTooltip / 鼠标事件推进，这里照状态落笔。
	if doc != nil && doc.tooltipShown {
		renderTooltip(g, doc, vpW, vpH)
	}
}

// paintPositioned 绘制一个定位元素及其静态子树；若它建立了层叠上下文，
// 再递归绘制其内部的定位子树。
//
// paint 传 nil 无损：继承字段（color/font/text-align）在级联时已下推到
// 每个元素的 computed，renderNode 内部会用自身 comp 组装。定位子树不在
// 主 pass 递归里，祖先的 overflow 裁剪必须按父链重新压栈，否则滚动容器
// 里的脱流内容会画到盒外。
func paintPositioned(g Graphics, e HTMLElement, focus HTMLElement, vpW, vpH int) {
	n := pushAncestorClips(g, e)
	// 定位子树不在主 pass 的递归里，祖先 opacity 的包装不会自动套上，
	// 这里按父链乘积补包（不含 e 自身——renderNode 会按它自己的
	// computed 再套一层）。递归时不传包装后的 g：下一层重新按整条父链
	// 计算，避免重复相乘。
	pg := withOpacity(g, ancestorOpacity(e))
	renderNode(pg, e, nil, focus, vpW, vpH)
	// 定位元素画完后，给它最近的「圆角 + 裁剪」祖先补盖四角：定位内容
	// 是在定位 pass 里画的（主 pass 早已补盖过一次），不补盖就会盖回
	// 圆角（大黑卡的右上角被溢出到角外的圆形装饰盖成方角）。
	// 只在定位元素确实压到角区时才补（否则每个定位元素都要多画一遍）。
	if box := nearestRoundedClipAncestor(e); box != nil && overlapsCornerAreas(e, box) {
		drawClipCornerMask(pg, box, false)
	}
	if createsStackingContext(e) {
		for _, pb := range collectPositioned(e) {
			paintPositioned(g, pb.e, focus, vpW, vpH)
		}
	}
	popClips(g, n)
}

// ancestorOpacity 返回 e 的祖先链上 opacity 的乘积（不含 e 自身）。
func ancestorOpacity(e HTMLElement) float64 {
	k := 1.0
	for p := e.ParentElement(); p != nil; p = p.ParentElement() {
		if b := inner(p); b != nil && b.computed != nil {
			k *= float64(b.computed.Opacity())
		}
	}
	return k
}

// nearestRoundedClipAncestor 返回最近的「圆角 + 裁剪」祖先（overflow 非
// visible 且 border-radius > 0）。定位内容溢出到它的圆角外时需要补盖。
func nearestRoundedClipAncestor(e HTMLElement) *htmlElement {
	for p := e.ParentElement(); p != nil; p = p.ParentElement() {
		b := inner(p)
		if b == nil || b.computed == nil || !isClippingBox(b) {
			continue
		}
		pd, bd := resolvedEdges(b, b.computed)
		bw := b.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
		bh := b.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
		if resolveRadius(b.computed, bw, bh) > 0 {
			return b
		}
	}
	return nil
}

// overlapsCornerAreas 判断元素是否压到裁剪盒的圆角区域（四角 r×r 的方块）。
// 只压到中间直线段的不用补盖。
func overlapsCornerAreas(e HTMLElement, box *htmlElement) bool {
	b := inner(e)
	if b == nil || box == nil || box.computed == nil {
		return false
	}
	pd, bd := resolvedEdges(box, box.computed)
	bx := box.x - pd[eLeft] - bd[eLeft]
	by := box.y - pd[eTop] - bd[eTop]
	bw := box.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
	bh := box.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
	r := resolveRadius(box.computed, bw, bh)
	if r <= 0 {
		return false
	}
	// 元素盒（边框盒）
	epd, ebd := resolvedEdges(b, b.computed)
	ex := b.x - epd[eLeft] - ebd[eLeft]
	ey := b.y - epd[eTop] - ebd[eTop]
	ew := b.width + epd[eLeft] + epd[eRight] + ebd[eLeft] + ebd[eRight]
	eh := b.height + epd[eTop] + epd[eBottom] + ebd[eTop] + ebd[eBottom]
	if ew <= 0 || eh <= 0 {
		return false
	}
	// 四个角区（r×r）
	corners := [4][4]int{
		{bx, by, r, r},
		{bx + bw - r, by, r, r},
		{bx, by + bh - r, r, r},
		{bx + bw - r, by + bh - r, r, r},
	}
	for _, c := range corners {
		if ex < c[0]+c[2] && ex+ew > c[0] && ey < c[1]+c[3] && ey+eh > c[1] {
			return true
		}
	}
	return false
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

// offscreenBox 判断盒子是否完全落在视口外（画了也看不见）。
//
// vpW/vpH 为 0 表示视口未知（无头渲染、SVG 导出等）→ 一律不裁剪。
//
// 只按盒子本身判断，不连带裁子树：overflow:visible 的子内容可能从父盒
// 里溢出来而仍然可见，由子节点各自判断。
func offscreenBox(bx, by, bw, bh, vpW, vpH int) bool {
	if vpW <= 0 || vpH <= 0 {
		return false
	}
	if bw <= 0 || bh <= 0 {
		return true // 空盒没有可画的东西
	}
	return bx+bw <= 0 || bx >= vpW || by+bh <= 0 || by >= vpH
}

func renderNode(g Graphics, e HTMLElement, parentPaint Paint, focus HTMLElement, vpW, vpH int) {
	renderNodeTextColor(g, e, parentPaint, focus, nil, vpW, vpH)
}

// renderNodeTextColor 与 renderNode 同构，但用 textColor 强制覆盖本子树的
// 文字绘制色（select 展开浮层的选中项：#1967D2 高亮底上出白字，Chrome 同款）。
// 覆盖作用于 Paint（文字绘制与子树继承，含级联到 textNode 的值）；元素自身
// 背景/边框仍按 computed 绘制，表单控件专用绘制分支不读该 Paint，不受影响。
//
// vpW/vpH 是可见区域（0 = 未知 → 不裁剪）：完全落在视口外的盒子不画。
func renderNodeTextColor(g Graphics, e HTMLElement, parentPaint Paint, focus HTMLElement, textColor Color, vpW, vpH int) {
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

	// opacity：引擎没有图层合成，按「本子树所有绘制颜色的 alpha 乘算」
	// 近似（见 opacity.go 的说明）。必须在任何绘制之前包装：元素自身的
	// 背景/边框也属于这一组。
	if comp != nil {
		if op := comp.Opacity(); op < 1 {
			g = withOpacity(g, float64(op))
		}
	}

	// visibility:hidden/collapse 的盒子照常占位，只是自身不绘制、不参与
	// 命中；子级显式 visible 仍可显示（CSS 语义），因此这里只挡自身绘制，
	// 子节点照常递归。
	visible := comp == nil || comp.Visibility() == VisibilityVisible

	// 组装本节点绘制属性：继承 + 自身样式
	p := NewPaint()
	if parentPaint != nil {
		p.SetSize(parentPaint.Size())
		p.SetColor(parentPaint.Color())
		p.SetFontFamily(parentPaint.FontFamily())
		p.SetBold(parentPaint.Bold())
		setPaintItalic(p, paintItalic(parentPaint))
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
		// font-style 是继承属性，computed 里已是最终值：直接按它写标记，
		// 不能只「斜体时置位」——父级斜体、子级 normal 时必须能复位。
		setPaintItalic(p, comp.FontStyle() == FontStyleItalic)
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
		if comp != nil && comp.Visibility() != VisibilityVisible {
			return // 继承来的 hidden/collapse：文本不落墨
		}
		dec := TextDecorationNone
		if comp != nil {
			dec = comp.TextDecoration()
		}
		for _, run := range tn.node.runs {
			// 视口外的片段不画：长页滚动时页面下半部分的文字占了大部分
			// 绘制调用，逐段裁掉能把一帧的调用数降到可见部分那点。
			if offscreenBox(run.x, run.y, run.w, run.h, vpW, vpH) {
				continue
			}
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

	// 圆角裁剪盒的补盖需要这些值（画完子树后用）：盒的边框盒几何、圆角
	// 半径、边框四边宽。
	var bx, by, bw, bh, r int
	var bd edges

	// 元素：背景与边框（外扩到边框盒）
	if comp != nil {
		pd, bde := resolvedEdges(base, comp)
		bx = base.x - pd[eLeft] - bde[eLeft]
		by = base.y - pd[eTop] - bde[eTop]
		bw = base.width + pd[eLeft] + pd[eRight] + bde[eLeft] + bde[eRight]
		bh = base.height + pd[eTop] + pd[eBottom] + bde[eTop] + bde[eBottom]
		bd = bde

		r = resolveRadius(comp, bw, bh)
		// radio 原生外观恒为正圆：UA 的 7px 半径只适配 13px 默认尺寸，
		// 作者改写宽高后仍按圆绘制（Chrome 同口径），也与选中标记的圆形几何一致。
		// 取「半宽向上取整」而非 min/2：13px → 7 与原 UA 值一致（r 略大于半宽
		// 时圆角盒 SDF 退化为整圆），24/40px 等改写尺寸同样成圆。
		if checkKind(base) == "radio" {
			r = (minInt(bw, bh) + 1) / 2
		}

		// 盒子整体在视口外：背景/边框/控件外观都不画（子树仍要遍历——
		// overflow:visible 的子内容可能露在视口里，由子节点各自判断）。
		inView := !offscreenBox(bx, by, bw, bh, vpW, vpH)

		// 阴影在背景之前画：CSS 的 box-shadow 属于盒子装饰，必须压在
		// 背景/边框下面（半透明背景才能透出阴影）。它可能延伸到盒外，
		// 因此单独按阴影包围盒做视口裁剪——盒子本体在屏外时阴影仍可能
		// 露在视口里。
		shadows := comp.BoxShadow()
		if visible && len(shadows) > 0 {
			if sx, sy, sw, sh, ok := shadowBounds(shadows, bx, by, bw, bh); ok &&
				!offscreenBox(sx, sy, sw, sh, vpW, vpH) {
				paintBoxShadows(g, bx, by, bw, bh, r, shadows, comp.Color())
			}
		}

		paintBD, borderColors, hasBorder := borderPaintState(comp, bd)

		// empty-cells: hide 的单元格：背景与边框不画（CSS 2.1 §17.5.1.1），
		// 几何与命中照旧（Chrome 同口径）。
		if visible && inView && !base.noCellDeco {
			if bg := comp.BackgroundColor(); bg != nil {
				fillRoundedRect(g, bx, by, bw, bh, r, bg)
			}
			if hasBorder && bw > 0 && bh > 0 {
				strokeRoundedRectSides(g, bx, by, bw, bh, r, paintBD, borderColors)
			}
		}

		// 内阴影（inset）画在背景/边框之后、内容之前：CSS 把内阴影铺在
		// padding 盒内（边框内侧），压在背景上、让内容落在它上面。
		if visible && inView && !base.noCellDeco && len(shadows) > 0 {
			ipx := bx + bd[eLeft]
			ipy := by + bd[eTop]
			ipw := bw - bd[eLeft] - bd[eRight]
			iph := bh - bd[eTop] - bd[eBottom]
			// 内圆角 = 外圆角 − 最宽边框（CSS：边框内侧的圆角随边框收窄）。
			ipr := r - maxEdge(bd)
			if ipr < 0 {
				ipr = 0
			}
			paintInsetBoxShadows(g, ipx, ipy, ipw, iph, ipr, shadows, comp.Color())
		}

		// 聚焦框画在边框盒上，必须早于下方 overflow 裁剪压栈：textarea 默认
		// overflow:hidden，裁剪区是 padding 盒，画在边框盒上的聚焦框会被整圈
		// 裁掉（此前聚焦框在裁剪栈内绘制 → textarea 永远看不到聚焦指示）。
		// 单选/复选此前就没有聚焦框（Chrome 同款圆环由选中标记承担），保持。
		if visible && inView && focused && isFormControl(base) && bw > 0 && bh > 0 && checkKind(base) == "" {
			strokeRoundedRect(g, bx, by, bw, bh, r, edges{1, 1, 1, 1}, focusBlue)
		}

		// 列表标记：引擎无 list-item 概念，标记文本由布局期算出（ul → "•"，
		// ol → "1." "2."…），画在内容左缘左侧、右对齐到 −0.75em 处
		//（Chrome 实测：标记盒右缘 = 内容左缘 − 0.5em）。
		if visible && inView {
			if mx, my, mw, mh, ok := listMarkerBox(g, base, comp); ok {
				g.DrawText(mx, my, mw, mh, p, base.listMarker)
			}
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
		if visible && inView && isFormControl(base) && bw > 0 && bh > 0 {
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
				// 窗口按 rune 取整后（或光标保头时）可见文本可能超出控件内盒
				// 一个字符宽：按 padding 盒裁剪，文字不外漏（Chrome 的 input
				// 文本不越右缘）。舍头保尾旧口径 tw ≤ avail，不触发此裁剪。
				textClip := false
				if tw > base.width-4 && base.width > 0 {
					textClip = pushClip(g, bx+bd[eLeft], by+bd[eTop],
						bw-bd[eLeft]-bd[eRight], bh-bd[eTop]-bd[eBottom])
				}
				fullRunes := []rune(displayValue(base))
				// 按钮型 value 文本水平居中（对齐 Chrome 按钮文字）；按钮型
				// 不截断（auto 宽已按 value 外盒测量），故此处不会被裁短。
				buttonLike := typ == "submit" || typ == "reset" || typ == "button"
				tx := base.x
				if buttonLike && base.width > tw {
					tx = base.x + (base.width-tw)/2
				}
				ty := base.y
				// 行盒（高 th=声明 line-height，未声明时为自然行高）按内容盒
				// 垂直居中；比控件高（工具栏槽位继承条 line-height）时上下均
				// 溢出——CSS 行盒语义是溢出均分，不是钳到顶边。
				ty += (base.height - th) / 2
				// 文字纹理落点另加 half-leading：DrawText 把纹理顶钉在传入 y，
				// 而纹理高=自然行高；th 是声明 line-height 时两者不等，行距的
				// 一半要加在纹理之上，否则文字贴着行盒顶偏上（browser 地址栏
				// 实测 4px）。与常规流 flushLine 的 half-leading 同一口径。
				textY := ty
				if _, natH := g.MeasureText("Mg", size, p.Bold(), p.FontFamily()); th > natH {
					textY += (th - natH) / 2
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
					g.DrawText(tx, textY, tw, th, p, val)
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
				if textClip {
					popClip(g)
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
		renderNodeTextColor(g, child, p, focus, textColor, vpW, vpH)
	}
	if clipped {
		// 圆角裁剪盒：把四角"圆角形状之外"的部分用元素背后的底色盖掉，
		// 等效于按 border-radius 裁剪（详见 clipCornerMask 的说明）。
		// hidden 元素自身不落墨，四角补盖也一并跳过（否则会在父背景上
		// 留下一块与背景同色的"补丁"，把下层内容盖掉）。
		if visible && r > 0 {
			drawClipCornerMask(g, base, focused)
		}
		popClip(g)
	}
}

// drawClipCornerMask 给圆角裁剪盒补盖四角（见 clipCornerMask 的说明）。
//
// focused 为真时额外补画聚焦框：聚焦框沿圆角形状画在边框盒上，被盖掉的
// 角要补回来。定位 pass 里的补盖不补聚焦框（聚焦框画在主 pass，
// 定位 pass 的补盖只影响定位内容覆盖到的那几块角）。
func drawClipCornerMask(g Graphics, base *htmlElement, focused bool) {
	comp := base.computed
	if comp == nil {
		return
	}
	pd, bd := resolvedEdges(base, comp)
	bx := base.x - pd[eLeft] - bd[eLeft]
	by := base.y - pd[eTop] - bd[eTop]
	bw := base.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
	bh := base.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
	clipCornerMask(g, base, bx, by, bw, bh, resolveRadius(comp, bw, bh), bd, comp, focused)
}

// clipCornerMask 把圆角裁剪盒四角"圆角形状之外"的部分盖掉。
//
// 引擎的裁剪栈是矩形（SDL_RenderSetClipRect 只能给矩形），而 CSS 语义是
// 裁到 border-radius 的形状：子元素（圆形装饰、图标等）溢出到圆角外时，
// 浏览器按圆角形状裁掉，引擎的矩形裁剪会让它们盖住圆角（大黑卡的右上角
// 变成方角）。这里在画完内容后，用**元素背后的底色**盖掉那几块区域，
// 等效于按圆角形状裁剪（背后不是纯色时是近似）。
//
// 盖完要把边框/聚焦框补画一次：它们沿着圆角形状，角上被盖掉的部分要回来。
func clipCornerMask(g Graphics, base *htmlElement, bx, by, bw, bh, r int,
	bd edges, comp CSSStyleDeclaration, focused bool) {
	// 裁剪区是 padding 盒（CSS 语义），圆角按内半径 = 外半径 − 边框宽。
	padTop := bd[eTop]
	padLeft := bd[eLeft]
	px, py := bx+padLeft, by+padTop
	pw, ph := bw-padLeft-bd[eRight], bh-padTop-bd[eBottom]
	ir := r - maxInt(maxInt(bd[eLeft], bd[eRight]), maxInt(bd[eTop], bd[eBottom]))
	if ir <= 0 || pw <= 0 || ph <= 0 {
		return // 无圆角（或圆角被边框吃掉）→ 矩形裁剪已足够
	}
	bg := behindBackground(base)
	if bg == nil {
		return // 背后底色未知：宁可不动（矩形裁剪）
	}
	// 四角的"圆角之外"区域：角标精灵的覆盖率取反（四个角互为镜像）。
	sd, ok := g.(SpriteDrawer)
	if !ok {
		return // 后端不支持精灵 blit：保持矩形裁剪（画面略逊但不报错）
	}
	sd.DrawSprite(px, py, cornerMaskSprite(ir, bg, false, false))           // 左上
	sd.DrawSprite(px+pw-ir, py, cornerMaskSprite(ir, bg, true, false))      // 右上
	sd.DrawSprite(px, py+ph-ir, cornerMaskSprite(ir, bg, false, true))      // 左下
	sd.DrawSprite(px+pw-ir, py+ph-ir, cornerMaskSprite(ir, bg, true, true)) // 右下

	// 边框/聚焦框沿圆角形状，角上被盖掉的部分补回来。
	if comp != nil {
		paintBD, colors, has := borderPaintState(comp, bd)
		if has && bw > 0 && bh > 0 {
			strokeRoundedRectSides(g, bx, by, bw, bh, r, paintBD, colors)
		}
	}
	if focused && isFormControl(base) && bw > 0 && bh > 0 && checkKind(base) == "" {
		strokeRoundedRect(g, bx, by, bw, bh, r, edges{1, 1, 1, 1}, focusBlue)
	}
}

// behindBackground 返回元素背后应显示的底色：最近的祖先里不透明的背景色；
// 都没有则用画布底色（CSS canvas propagation），再没有就白底。
//
// 圆角裁剪盒的角上应透出"背后"（浏览器如此），引擎只能近似成一块纯色。
func behindBackground(e HTMLElement) Color {
	for p := e.ParentElement(); p != nil; p = p.ParentElement() {
		b := inner(p)
		if b == nil || b.computed == nil {
			continue
		}
		if bg := b.computed.BackgroundColor(); bg != nil {
			if _, _, _, a := bg.RGBA(); a >= 250 {
				return bg
			}
		}
	}
	if d, ok := docOf(e).(*htmlDocument); ok && d != nil {
		if bg := CanvasBackground(d); bg != nil {
			return bg
		}
	}
	return NewColor(0xFF, 0xFF, 0xFF, 0xFF)
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
//
// 覆盖率只���**弧边**上逐点算（每像素一次 math.Hypot，很贵）：圆角盒的
// SDF（见 coverageAt）决定了——上下各 r 行之外的整行必为全覆盖，中间行
// 一次实色填充即可；角行里也只有左右两角 |x-角| ≤ r 的窄带可能有部分
// 覆盖，中段 [bx+r, bx+bw-r-1] 必为全覆盖。整盒逐点算会让每帧的绘制
// 成本翻几倍（profile：paintCoverage 占绘制耗时 57%，圆角卡片多的页面尤甚）。
func fillRoundedRect(g Graphics, bx, by, bw, bh, r int, c Color) {
	if bw <= 0 || bh <= 0 {
		return
	}
	if r <= 0 {
		g.DrawColor(bx, by, bw, bh, c)
		return
	}
	// 后端支持位图 blit 时：实色部分 5 条矩形 + 四角各 1 次 blit（9 次绘制），
	// 替掉逐像素画整圈边缘（几十次 1×1 绘制/盒）。绘制调用是 GPU 路径上的
	// 主要开销（每次都是驱动调用），圆角卡片多的页面一帧能到三万次调用，
	// 帧时间被拖到几十毫秒——滚动动画（120ms）被切成两三帧，看起来一卡一卡
	// 还闪。不支持 blit 的后端走下面的逐像素路径（结果一致，只是慢）。
	if sd, ok := g.(SpriteDrawer); ok {
		fillRoundedRectSprites(g, sd, bx, by, bw, bh, r, c)
		return
	}
	// 中间带（上下各 r 行之外）整片覆盖：一条矩形画完，不要逐行画——
	// 逐行的话一个 300×200 的盒就是 176 次 FillRect（一帧上万次 SDL
	// 调用，GPU 路径上每次都是驱动调用 → 帧时间几十毫秒，滚动动画被切成
	// 几帧，看起来一卡一卡还闪）。
	midY0, midY1 := by+r, by+bh-r
	if midY1 > midY0 {
		g.DrawColor(bx, midY0, bw, midY1-midY0, c)
	}
	for y := by; y < by+bh; y++ {
		if y >= midY0 && y < midY1 {
			continue // 中间带已整片画完
		}
		x0, x1, ok := roundedSpanX(bx, by, bw, bh, r, y)
		if !ok {
			continue
		}
		py := float64(y) + 0.5
		lo := maxInt(bx, x0-2)
		hi := minInt(bx+bw-1, x1+2)
		// 角上窄带逐点算覆盖率（查表，见 roundedCov）；中段必为全覆盖
		// （SDF 推导：qx<0 时 d = qy - r ≤ -0.5），实色一次画完。
		// 局部坐标：u = 像素离最近竖边的距离，v = 离最近横边的距离。
		v := y - by
		if vb := by + bh - 1 - y; vb < v {
			v = vb
		}
		leftEnd := minInt(hi, bx+r-1)
		rightStart := maxInt(lo, bx+bw-r)
		if rightStart <= leftEnd+1 {
			// 盒太窄（没有中段）：整段逐点算。
			paintCoverage(g, y, lo, hi, c, func(x int) float64 {
				return coverageAt(float64(x)+0.5, py, bx, by, bw, bh, r)
			})
			continue
		}
		paintCoverage(g, y, lo, leftEnd, c, func(x int) float64 {
			return cornerCov(r, x-bx, v)
		})
		g.DrawColor(leftEnd+1, y, rightStart-leftEnd-1, 1, c)
		paintCoverage(g, y, rightStart, hi, c, func(x int) float64 {
			return cornerCov(r, bx+bw-1-x, v)
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

// ---------------------------------------------------------------------------
// 圆角覆盖率查表
// ---------------------------------------------------------------------------

// roundedCovMaxR 是查表覆盖的最大半径；更大的圆角（少见）走逐点计算。
const roundedCovMaxR = 20

// roundedCov[r][u][v] 是半径 r、角上局部坐标 (u,v) 处的覆盖率。
//
// 关键性质：圆角盒 SDF（coverageAt）在**角带**里的取值只与 (r,u,v) 有关，
// 与盒的宽高无关——像素中心相对角的偏移是 (u+0.5, v+0.5)，代入 SDF 得
// qx = r-u-0.5、qy = r-v-0.5，于是
//
//	d = hypot(max(qx,0), max(qy,0)) + min(max(qx,qy),0) - r
//
// 只剩 r/u/v 三个整数。把它们预计算成表，绘制时只做数组索引——
// 圆角多的页面（卡片网格）这一项曾占绘制耗时的一半以上（每像素一次
// math.Hypot）。表大小 Σr² = 2870 项（r ≤ 20），约 23KB。
var roundedCov [roundedCovMaxR + 1][][]float64

func init() {
	for r := 1; r <= roundedCovMaxR; r++ {
		m := make([][]float64, r)
		for u := 0; u < r; u++ {
			row := make([]float64, r)
			for v := 0; v < r; v++ {
				row[v] = cornerCoverage(r, u, v)
			}
			m[u] = row
		}
		roundedCov[r] = m
	}
}

// cornerCoverage 计算半径 r、角上局部坐标 (u,v) 处的覆盖率。
// 与 coverageAt 同一套圆角盒 SDF，只是用局部坐标表达（结果与盒宽高无关）。
func cornerCoverage(r, u, v int) float64 {
	fr := float64(r)
	qx := fr - float64(u) - 0.5
	qy := fr - float64(v) - 0.5
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - fr
	if d <= -0.5 {
		return 1
	}
	if d >= 0.5 {
		return 0
	}
	return 0.5 - d
}

// cornerCov 返回角上局部坐标 (u,v)、半径 r 处的覆盖率：表内直接查；
// r 超过表上限（或坐标越界）时退回逐点计算。
func cornerCov(r, u, v int) float64 {
	if r <= roundedCovMaxR && u >= 0 && u < r && v >= 0 && v < r {
		return roundedCov[r][u][v]
	}
	// 退回：以 (0,0) 为角的 2r×2r 盒——角带里的 SDF 与盒大小无关。
	return coverageAt(float64(u)+0.5, float64(v)+0.5, 0, 0, 2*r, 2*r, r)
}

// ellipseCov 计算内圆角椭圆（半轴 rx/ry，角局部坐标）的覆盖率。
//
// 与 cornerCov 同一套「像素中心到边界的近似有符号距离 → 0.5px AA」口径。
// rx == ry 时退化成圆，与 cornerCov 逐像素一致；rx != ry 时用归一化圆
// 距离乘较小半轴近似，对圆角边框的 1px 过渡足够。
func ellipseCov(rx, ry, u, v int) float64 {
	if rx <= 0 || ry <= 0 {
		return 0
	}
	x := (float64(u) + 0.5 - float64(rx)) / float64(rx)
	y := (float64(v) + 0.5 - float64(ry)) / float64(ry)
	d := (math.Hypot(x, y) - 1) * float64(minInt(rx, ry))
	if d <= -0.5 {
		return 1
	}
	if d >= 0.5 {
		return 0
	}
	return 0.5 - d
}

// ---------------------------------------------------------------------------
// 圆角角标精灵
// ---------------------------------------------------------------------------

// cornerRadii 是某个内圆角的椭圆半轴。
//
// 四侧边框宽不同时，内角不是圆：横向半轴 = 外半径 - 该角的左右边框宽，
// 纵向半轴 = 外半径 - 该角的上下边框宽。内椭圆与外界圆同心，某条边
// 宽度为 0 时该轴保持外半径，内椭圆仍与外圆在空边处相切，不会沿空边
// 留下一段「假边框」。
type cornerRadii struct {
	rx, ry int
}

// spriteKey 是角标精灵的缓存键。
//
// 精灵只与（半径、颜色、镜像方向）有关，与盒的大小无关——同一页面上大量
// 同样式的圆角卡片共用同一份像素。
//
// 边框环的精灵多几个维度：内圆角椭圆 (ir)、四侧边框宽 (li,rt,tp,bt)、内盒
// 尺寸 (iw,ih)、角序号 corner。环的内盒相对外盒是偏移的，四侧边框宽不同
// → 四角并不互为镜像，故按角分别缓存（ring 为真时这些字段才有效）。
type spriteKey struct {
	r              int
	ir             cornerRadii
	li, rt, tp, bt int
	iw, ih         int
	cr, cg, cb, ca uint8
	// 边框环角块有两条邻边，颜色可能不同；普通角标只用第一组。
	cr2, cg2, cb2, ca2 uint8
	fx, fy             bool
	mask               bool // 补盖精灵（圆角之外的部分），与普通角标图案不同
	ring               bool // 边框环角标
	shadow             bool // 阴影精灵（模糊后的圆角盒）
	inset              bool // 内阴影精灵（形状外为 1、形状内为 0 的掩码）
	sblur              int  // 阴影模糊半径（shadow 为真时有效）
	// 外阴影必须把元素自己的边框盒挖掉（CSS：外阴影只画在边框盒外）。
	// 洞的坐标是精灵内局部坐标，随偏移/扩散/盒尺寸变化，必须进缓存键。
	holeX, holeY, holeW, holeH, holeR int
	corner                            int // 0=左上 1=右上 2=左下 3=右下（ring 时有效）
}

// spriteEntry 是缓存里的一条记录：精灵 + 像素字节数 + 最近使用时刻。
type spriteEntry struct {
	sprite   *Sprite
	bytes    int
	lastUsed uint64
}

// 精灵缓存的上限：条目数与像素字节数。
//
// 键里含颜色——页面若逐帧改圆角盒的颜色（动画），会不断产生新键；无上限
// 的缓存就是内存泄漏（旧精灵的像素一直没人回收）。上限与后端文本纹理
// 缓存同思路：超出按最近最少使用淘汰。
const (
	maxSpriteItems = 4096    // 条目上限
	maxSpriteBytes = 4 << 20 // 像素字节上限（4MB）
)

// spriteCache 缓存圆角角标精灵（普通角标、补盖角标、边框环角标共用一张表，
// 用键里的 mask/ring 区分）。
var (
	spriteCache      = map[spriteKey]*spriteEntry{}
	spriteCacheClock uint64 // 单调时钟：记录最近使用时刻
	spriteCacheBytes int    // 当前缓存的像素字节总数
)

// lookupSprite 取键对应的精灵；命中则刷新最近使用时刻。
func lookupSprite(key spriteKey) (*Sprite, bool) {
	e, ok := spriteCache[key]
	if !ok {
		return nil, false
	}
	spriteCacheClock++
	e.lastUsed = spriteCacheClock
	return e.sprite, true
}

// storeSprite 把精灵存入缓存并按需淘汰。
func storeSprite(key spriteKey, s *Sprite) {
	spriteCacheClock++
	spriteCache[key] = &spriteEntry{sprite: s, bytes: len(s.Pix), lastUsed: spriteCacheClock}
	spriteCacheBytes += len(s.Pix)
	evictSprites()
}

// evictSprites 超出上限时按最近最少使用淘汰。
//
// 淘汰必须是 LRU 而非 FIFO：FIFO 按插入顺序淘汰，会把页面上复用最多的精灵
// 先淘汰掉——工作集略大于上限时每帧都要重建精灵（重建 + 重新上传纹理），
// 缓存颠簸。
//
// 一次淘汰一批（降到 90%），摊平扫描成本：满页逐帧换色时每次 miss 都扫
// 全表，帧时间会被吃掉。
func evictSprites() {
	if len(spriteCache) <= maxSpriteItems && spriteCacheBytes <= maxSpriteBytes {
		return
	}
	targetItems := maxSpriteItems * 9 / 10
	targetBytes := maxSpriteBytes * 9 / 10
	for len(spriteCache) > targetItems || spriteCacheBytes > targetBytes {
		oldKey, old, ok := lruSprite()
		if !ok {
			return
		}
		delete(spriteCache, oldKey)
		spriteCacheBytes -= old.bytes
	}
}

// lruSprite 找出最近最少使用的精灵条目。
func lruSprite() (spriteKey, *spriteEntry, bool) {
	var oldKey spriteKey
	var old *spriteEntry
	found := false
	for k, e := range spriteCache {
		if !found || e.lastUsed < old.lastUsed {
			oldKey, old, found = k, e, true
		}
	}
	return oldKey, old, found
}

// cornerMaskSprite 返回"圆角形状之外"的角标精灵（覆盖率取反），颜色是底色。
//
// 用于圆角裁剪盒的四角补盖：盖掉圆角之外的部分，等效于按 border-radius
// 裁剪（见 clipCornerMask）。四个角是同一图案的镜像，故按 (fx,fy) 分别
// 缓存。
func cornerMaskSprite(r int, c Color, fx, fy bool) *Sprite {
	cr, cg, cb, ca := c.RGBA()
	key := spriteKey{r: r, cr: cr, cg: cg, cb: cb, ca: ca, fx: fx, fy: fy, mask: true}
	if s, ok := lookupSprite(key); ok {
		return s
	}
	pix := make([]byte, r*r*4)
	for v := 0; v < r; v++ {
		for u := 0; u < r; u++ {
			su, sv := u, v
			if fx {
				su = r - 1 - u
			}
			if fy {
				sv = r - 1 - v
			}
			out := 1 - cornerCov(r, su, sv) // 圆角之外的部分
			if out <= 0 {
				continue
			}
			na := uint32(float64(ca)*out + 0.5)
			if na == 0 {
				continue
			}
			if na > 255 {
				na = 255
			}
			i := (v*r + u) * 4
			pix[i+0], pix[i+1], pix[i+2], pix[i+3] = cr, cg, cb, uint8(na)
		}
	}
	s := &Sprite{W: r, H: r, Pix: pix}
	storeSprite(key, s)
	return s
}

// ringCornerSprite 返回圆角边框环在某个角上的精灵（r×r 的 RGBA8888）。
//
// 精灵是 r×r：alpha = 环覆盖率 × 颜色 alpha（与逐像素路径的 scaledAlpha
// 同口径），RGB 是颜色本身——blit 后的混合结果与逐像素画完全一致。
func ringCornerSprite(r int, ir cornerRadii, li, rt, tp, bt, iw, ih int, c Color, corner int) *Sprite {
	return ringCornerSpriteSides(r, ir, li, rt, tp, bt, iw, ih, c, c, corner)
}

func ringCornerSpriteSides(r int, ir cornerRadii, li, rt, tp, bt, iw, ih int,
	horizontal, vertical Color, corner int) *Sprite {
	if horizontal == nil {
		horizontal = vertical
	}
	if vertical == nil {
		vertical = horizontal
	}
	if horizontal == nil {
		return &Sprite{W: 0, H: 0}
	}
	hr, hg, hb, ha := horizontal.RGBA()
	vr, vg, vb, va := vertical.RGBA()
	key := spriteKey{r: r, ir: ir, li: li, rt: rt, tp: tp, bt: bt,
		iw: iw, ih: ih, cr: hr, cg: hg, cb: hb, ca: ha,
		cr2: vr, cg2: vg, cb2: vb, ca2: va, ring: true, corner: corner}
	if s, ok := lookupSprite(key); ok {
		return s
	}
	pix := make([]byte, r*r*4)
	for v := 0; v < r; v++ {
		for u := 0; u < r; u++ {
			cov := ringCornerCoverage(r, ir, li, rt, tp, bt, iw, ih, corner, u, v)
			if cov <= 0 {
				continue
			}
			du, dv := u, v
			if corner == 1 || corner == 3 {
				du = r - 1 - u
			}
			if corner >= 2 {
				dv = r - 1 - v
			}
			c := vertical
			if du > dv {
				c = horizontal
			}
			cr, cg, cb, ca := c.RGBA()
			na := uint32(float64(ca)*cov + 0.5)
			if na == 0 {
				continue
			}
			if na > 255 {
				na = 255
			}
			i := (v*r + u) * 4
			pix[i+0], pix[i+1], pix[i+2], pix[i+3] = cr, cg, cb, uint8(na)
		}
	}
	s := &Sprite{W: r, H: r, Pix: pix}
	storeSprite(key, s)
	return s
}

// ---------------------------------------------------------------------------
// box-shadow
// ---------------------------------------------------------------------------

// maxShadowSpritePixels 限制单张阴影精灵的像素数。
//
// 阴影精灵把整块模糊结果缓存成位图，尺寸随盒子线性增长；满屏大容器的阴影
// 会轻松超过精灵缓存总上限（4MB），存进去立刻被 LRU 淘汰、下一帧又重建。
// 超过这个阈值的盒子改走「多层圆角矩形」近似，避免缓存颠簸。
const maxShadowSpritePixels = 700000

// shadowBounds 返回一组外阴影的包围盒（含模糊外扩），用于视口裁剪。
// 内阴影整体落在 padding 盒内，不扩大绘制范围，不参与这里的包围盒计算
// （全是 inset 阴影时 ok=false，调用方按盒子本体做视口裁剪即可）。
func shadowBounds(shadows []Shadow, bx, by, bw, bh int) (x, y, w, h int, ok bool) {
	if bw <= 0 || bh <= 0 {
		return 0, 0, 0, 0, false
	}
	x0, y0 := 1<<30, 1<<30
	x1, y1 := -1<<30, -1<<30
	for _, sh := range shadows {
		if sh.Inset {
			continue
		}
		sw, shh := bw+2*sh.Spread, bh+2*sh.Spread
		if sw <= 0 || shh <= 0 {
			continue
		}
		pad := maxZero(sh.Blur)
		ax := bx + sh.X - sh.Spread - pad
		ay := by + sh.Y - sh.Spread - pad
		aw := sw + 2*pad
		ah := shh + 2*pad
		if ax < x0 {
			x0 = ax
		}
		if ay < y0 {
			y0 = ay
		}
		if ax+aw > x1 {
			x1 = ax + aw
		}
		if ay+ah > y1 {
			y1 = ay + ah
		}
	}
	if x1 <= x0 || y1 <= y0 {
		return 0, 0, 0, 0, false
	}
	return x0, y0, x1 - x0, y1 - y0, true
}

// paintBoxShadows 在元素背景之前绘制全部外阴影。
//
// CSS 规定第一条阴影在最上层，所以按声明顺序**倒着画**。阴影不参与布局、
// 也不影响命中测试，只是盒子装饰。这个函数在元素自己的 overflow 裁剪压栈
// 之前调用，因此阴影可以伸到盒外；祖先的 overflow 裁剪仍然有效。
func paintBoxShadows(g Graphics, bx, by, bw, bh, r int, shadows []Shadow, current Color) {
	if bw <= 0 || bh <= 0 {
		return
	}
	if current == nil {
		current = NewColor(0, 0, 0, 255)
	}
	for i := len(shadows) - 1; i >= 0; i-- {
		sh := shadows[i]
		if sh.Inset {
			continue
		}
		c := sh.Color
		if c == nil {
			c = current
		}
		if _, _, _, a := c.RGBA(); a == 0 {
			continue
		}
		sw, shh := bw+2*sh.Spread, bh+2*sh.Spread
		if sw <= 0 || shh <= 0 {
			continue
		}
		sr := r + sh.Spread
		if sr < 0 {
			sr = 0
		}
		if lim := minInt(sw, shh) / 2; sr > lim {
			sr = lim
		}
		sx := bx + sh.X - sh.Spread
		sy := by + sh.Y - sh.Spread
		// 无偏移、无正扩散的无模糊阴影完全落在边框盒内，会被洞全裁掉；
		// 直接跳过，省一次全透明贴图。
		if sh.Blur == 0 && sh.X == 0 && sh.Y == 0 && sh.Spread <= 0 {
			continue
		}
		pad := maxZero(sh.Blur)
		spw, sph := sw+2*pad, shh+2*pad
		if spw <= 0 || sph <= 0 {
			continue
		}
		// 精灵内局部坐标：元素边框盒（要挖掉的洞）相对精灵原点的位置。
		// 精灵原点 = sx-pad = bx + X - spread - pad，所以洞 x = pad - X + spread。
		holeX := pad - sh.X + sh.Spread
		holeY := pad - sh.Y + sh.Spread
		// 后端支持 blit 且精灵尺寸可控时走预模糊位图：一次生成、多次复用，
		// 每帧只剩一次贴图。否则退化为圆角矩形填充（有模糊时叠多层近似软边；
		// 不会为大阴影分配几十 MB 的临时位图）。
		if sd, ok := g.(SpriteDrawer); ok && spw*sph <= maxShadowSpritePixels {
			sd.DrawSprite(sx-pad, sy-pad,
				shadowSprite(spw, sph, sr, pad, c, holeX, holeY, bw, bh, r))
			continue
		}
		// 退化路径：圆角矩形填充（有模糊时多层）。若后端支持矩形裁剪，
		// 就按元素边框盒把盒外区域切成上/下/左/右四条带分别画，等价于
		// 把洞挖掉；不支持裁剪时只能整块画（盒内可能透出阴影，见 README
		// 已知限制）。
		if cl, ok := g.(Clipper); ok {
			paintShadowLayersClipped(g, cl, sx-pad, sy-pad, sw+2*pad, shh+2*pad,
				bx, by, bw, bh, sx, sy, sw, shh, sr, pad, c)
			continue
		}
		paintShadowLayers(g, sx, sy, sw, shh, sr, pad, c)
	}
}

// paintShadowLayers 是阴影精灵的退化路径：由外向内叠若干层圆角矩形。
//
// 元素背景会盖住中间区域，真正可见的是边缘一圈；层数取得少一些就能得到
// 可接受的软边。每层 alpha 取得较小，避免叠加后比声明颜色更黑。
func paintShadowLayers(g Graphics, x, y, w, h, r, blur int, c Color) {
	if blur <= 0 {
		// 无模糊就是实色圆角矩形，不需要分层（分层会把 alpha 叠成 0.9
		// 倍左右的近似值，反而偏离声明颜色）。
		fillRoundedRect(g, x, y, w, h, r, c)
		return
	}
	const layers = 6
	scale := 2.0 / float64(layers)
	for i := layers; i >= 1; i-- {
		grow := blur * i / layers
		ec, ok := scaledAlpha(c, scale)
		if !ok {
			continue
		}
		rr := r + grow
		if lim := minInt(w+2*grow, h+2*grow) / 2; rr > lim {
			rr = lim
		}
		fillRoundedRect(g, x-grow, y-grow, w+2*grow, h+2*grow, rr, ec)
	}
}

// paintShadowLayersClipped 是退化路径的「挖洞」版本：用四条矩形裁剪
// 带把元素边框盒排除在外，再在每条带里画多层阴影。
//
// 四条带互不重叠，并完整覆盖边框盒之外：上带（y<by，全宽，含上方两个
// 角区）、下带（y>=by+bh，全宽）、左带/右带（y 在盒高内，x 在盒外）。
// 这样既不需要后端的「清除」混合模式，也不会在同一像素上叠两次阴影。
func paintShadowLayersClipped(g Graphics, cl Clipper, sx, sy, sw, sh int,
	bx, by, bw, bh, shadowX, shadowY, shadowW, shadowH, r, blur int, c Color) {
	bands := [4][4]int{
		{sx, sy, sw, by - sy},                  // 上
		{sx, by + bh, sw, sy + sh - (by + bh)}, // 下
		{sx, by, bx - sx, bh},                  // 左
		{bx + bw, by, sx + sw - (bx + bw), bh}, // 右
	}
	for _, band := range bands {
		if band[2] <= 0 || band[3] <= 0 {
			continue
		}
		cl.PushClip(band[0], band[1], band[2], band[3])
		paintShadowLayers(g, shadowX, shadowY, shadowW, shadowH, r, blur, c)
		cl.PopClip()
	}
}

// ---------------------------------------------------------------------------
// box-shadow: inset（内阴影）
// ---------------------------------------------------------------------------

// maxEdge 返回四条边里的最大值。内阴影的内圆角按最宽边框扣减（外圆角
// 减边框宽度 = 边框内侧圆角）。
func maxEdge(e edges) int {
	return maxInt(maxInt(e[eTop], e[eBottom]), maxInt(e[eLeft], e[eRight]))
}

// insetShadowShape 返回内阴影的「不投影形状」：padding 盒按 spread 向内
// 收缩、再按偏移平移后的圆角矩形。
//
// ok=false 表示形状已被收缩成空——此时整个 padding 盒都落在阴影里，
// 直接铺满即可（模糊也不改变结果：掩码处处为 1）。
func insetShadowShape(px, py, pw, ph, pr int, sh Shadow) (x, y, w, h, r int, ok bool) {
	w = pw - 2*sh.Spread
	h = ph - 2*sh.Spread
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0, 0, false
	}
	x = px + sh.X + sh.Spread
	y = py + sh.Y + sh.Spread
	r = pr - sh.Spread
	if r < 0 {
		r = 0
	}
	if lim := minInt(w, h) / 2; r > lim {
		r = lim
	}
	return x, y, w, h, r, true
}

// paintInsetBoxShadows 绘制全部内阴影（box-shadow 带 inset）。
//
// CSS 语义：以 **padding 盒**为边界——阴影区是「padding 盒内、偏移并按
// spread 收缩后的形状之外」，最后按 padding 盒（含圆角）裁切。等价于
// 「形状之外全部不透明」，模糊后裁到 padding 盒：这就是浏览器里边缘最深、
// 向内容方向淡出的内发光。第一条阴影在最上层，故按声明顺序倒着画。
//
// 与元素自身 overflow 无关：内阴影整体在 padding 盒内，不参与命中。
func paintInsetBoxShadows(g Graphics, px, py, pw, ph, pr int, shadows []Shadow, current Color) {
	if pw <= 0 || ph <= 0 {
		return
	}
	if current == nil {
		current = NewColor(0, 0, 0, 255)
	}
	for i := len(shadows) - 1; i >= 0; i-- {
		sh := shadows[i]
		if !sh.Inset {
			continue
		}
		c := sh.Color
		if c == nil {
			c = current
		}
		if _, _, _, a := c.RGBA(); a == 0 {
			continue
		}
		sx, sy, sw, shh, sr, ok := insetShadowShape(px, py, pw, ph, pr, sh)
		if !ok {
			// 形状被 spread 收缩成空：整块 padding 盒都是阴影。
			fillRoundedRect(g, px, py, pw, ph, pr, c)
			continue
		}
		// 负 spread 把形状扩到 padding 盒之外时阴影完全不可见；掩码路径
		// 也能得到同样结果，但这里直接跳过省一次精灵生成（有模糊时形状
		// 边缘的模糊尾巴仍可能探进 padding 盒，不能跳）。
		if sh.Blur == 0 && sw >= pw && shh >= ph &&
			sx <= px && sy <= py && sx+sw >= px+pw && sy+shh >= py+ph {
			continue
		}
		pad := maxZero(sh.Blur)
		spw, sph := pw+2*pad, ph+2*pad
		if sd, ok := g.(SpriteDrawer); ok && spw*sph <= maxShadowSpritePixels {
			// 精灵内局部坐标：padding 盒从 (pad, pad) 起，形状按偏移落位。
			sd.DrawSprite(px-pad, py-pad, insetShadowSprite(spw, sph, pr, pad, c,
				sx-(px-pad), sy-(py-pad), sw, shh, sr))
			continue
		}
		// 退化路径：裁到 padding 盒后画形状之外的四条实色带（忽略模糊与
		// 形状圆角）。不支持裁剪的后端只能不画——没有「挖洞」能力时，
		// 整块铺色会盖住内容，比缺一条内阴影更糟。
		paintInsetShadowFallback(g, px, py, pw, ph, sx, sy, sw, shh, c)
	}
}

// paintInsetShadowFallback 是不支持精灵 blit 的后端的退化路径：裁到
// padding 盒，画形状之外的四条实色带。
//
// 上/下两条整宽，左右两条夹在中间——互不重叠，每个像素只画一次。模糊与
// 形状圆角在这个路径里被忽略（观感近似，不影响布局与命中）。
func paintInsetShadowFallback(g Graphics, px, py, pw, ph, sx, sy, sw, shh int, c Color) {
	cl, ok := g.(Clipper)
	if !ok {
		return
	}
	cl.PushClip(px, py, pw, ph)
	defer cl.PopClip()
	if top := sy - py; top > 0 {
		g.DrawColor(px, py, pw, minInt(top, ph), c)
	}
	if bot := py + ph - (sy + shh); bot > 0 {
		g.DrawColor(px, sy+shh, pw, minInt(bot, ph), c)
	}
	midY := maxInt(sy, py)
	midH := minInt(sy+shh, py+ph) - midY
	if midH > 0 {
		if left := sx - px; left > 0 {
			g.DrawColor(px, midY, minInt(left, pw), midH, c)
		}
		if right := px + pw - (sx + sw); right > 0 {
			g.DrawColor(sx+sw, midY, minInt(right, pw), midH, c)
		}
	}
}

// insetShadowSprite 返回内阴影的预模糊位图。
//
// 掩码口径：形状之外全是阴影（1），形状内是 0。形状之外**包括 padding 盒
// 外**——那片区域最终会被裁掉，但保留 1 才能让模糊在 padding 边缘保持满色；
// 若把盒外当 0，内发光会在边缘突然变淡（Chrome 不是这样）。
//
// 精灵内局部坐标：padding 盒从 (blur, blur) 开始；shapeX/Y/W/H/R 是「不
// 投影形状」在精灵内的位置。模糊完成后按 padding 盒（含圆角）裁切。
func insetShadowSprite(w, h, pr, blur int, c Color,
	shapeX, shapeY, shapeW, shapeH, shapeR int) *Sprite {
	cr, cg, cb, ca := c.RGBA()
	key := spriteKey{r: pr, iw: w, ih: h, sblur: blur,
		cr: cr, cg: cg, cb: cb, ca: ca, shadow: true, inset: true,
		holeX: shapeX, holeY: shapeY, holeW: shapeW, holeH: shapeH, holeR: shapeR}
	if s, ok := lookupSprite(key); ok {
		return s
	}
	mask := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cov := coverageAt(float64(x)+0.5, float64(y)+0.5,
				shapeX, shapeY, shapeW, shapeH, shapeR)
			mask[y*w+x] = float32(1 - cov)
		}
	}
	if blur > 0 {
		radius := blur / 2
		if radius < 1 {
			radius = 1
		}
		mask = blurMask(mask, w, h, radius, 3)
	}
	pw, ph := w-2*blur, h-2*blur
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cov := coverageAt(float64(x)+0.5, float64(y)+0.5, blur, blur, pw, ph, pr)
			mask[y*w+x] *= float32(cov)
		}
	}
	pix := make([]byte, w*h*4)
	for i, cov := range mask {
		if cov <= 0 {
			continue
		}
		na := uint32(float64(ca)*float64(cov) + 0.5)
		if na == 0 {
			continue
		}
		if na > 255 {
			na = 255
		}
		j := i * 4
		pix[j+0], pix[j+1], pix[j+2], pix[j+3] = cr, cg, cb, uint8(na)
	}
	s := &Sprite{W: w, H: h, Pix: pix}
	storeSprite(key, s)
	return s
}

// shadowSprite 返回预模糊的阴影位图：RGB 是阴影色，alpha 是圆角盒覆盖率
// 经三次盒式模糊后的结果（盒式模糊叠三次已足够接近高斯，观感与浏览器
// 的软阴影一致）。精灵四周留 blur 像素，避免模糊尾巴被裁掉。
//
// holeX/Y/W/H/R 是元素自身边框盒在精灵内的局部几何。CSS 外阴影只画在
// 边框盒外侧，因此模糊完成后还要把洞挖掉——否则元素背景透明时，盒内
// 会透出一整块阴影色。洞放在模糊之后挖，和浏览器「先模糊再按边框盒
// 裁切」的结果一致。
func shadowSprite(w, h, r, blur int, c Color,
	holeX, holeY, holeW, holeH, holeR int) *Sprite {
	cr, cg, cb, ca := c.RGBA()
	key := spriteKey{r: r, iw: w, ih: h, sblur: blur,
		cr: cr, cg: cg, cb: cb, ca: ca, shadow: true,
		holeX: holeX, holeY: holeY, holeW: holeW, holeH: holeH, holeR: holeR}
	if s, ok := lookupSprite(key); ok {
		return s
	}
	sw, sh := w-2*blur, h-2*blur
	if sw <= 0 || sh <= 0 {
		return &Sprite{W: 0, H: 0}
	}
	if lim := minInt(sw, sh) / 2; r > lim {
		r = lim
	}
	mask := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			mask[y*w+x] = float32(coverageAt(float64(x)+0.5, float64(y)+0.5,
				blur, blur, sw, sh, r))
		}
	}
	if blur > 0 {
		radius := blur / 2
		if radius < 1 {
			radius = 1
		}
		mask = blurMask(mask, w, h, radius, 3)
	}
	if holeW > 0 && holeH > 0 {
		if lim := minInt(holeW, holeH) / 2; holeR > lim {
			holeR = lim
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				cov := coverageAt(float64(x)+0.5, float64(y)+0.5,
					holeX, holeY, holeW, holeH, holeR)
				if cov > 0 {
					mask[y*w+x] *= float32(1 - cov)
				}
			}
		}
	}
	pix := make([]byte, w*h*4)
	for i, cov := range mask {
		if cov <= 0 {
			continue
		}
		na := uint32(float64(ca)*float64(cov) + 0.5)
		if na == 0 {
			continue
		}
		if na > 255 {
			na = 255
		}
		j := i * 4
		pix[j+0], pix[j+1], pix[j+2], pix[j+3] = cr, cg, cb, uint8(na)
	}
	s := &Sprite{W: w, H: h, Pix: pix}
	storeSprite(key, s)
	return s
}

// blurMask 对 alpha 掩码做可分离盒式模糊；passes 次叠加近似高斯。
func blurMask(src []float32, w, h, radius, passes int) []float32 {
	if radius <= 0 || passes <= 0 || w <= 0 || h <= 0 {
		return src
	}
	cur := src
	tmp := make([]float32, len(src))
	for p := 0; p < passes; p++ {
		boxBlurH(cur, tmp, w, h, radius)
		boxBlurV(tmp, cur, w, h, radius)
	}
	return cur
}

func boxBlurH(src, dst []float32, w, h, r int) {
	norm := 1.0 / float64(2*r+1)
	for y := 0; y < h; y++ {
		row := y * w
		var sum float64
		for x := -r; x <= r; x++ {
			if x >= 0 && x < w {
				sum += float64(src[row+x])
			}
		}
		for x := 0; x < w; x++ {
			dst[row+x] = float32(sum * norm)
			out, in := x-r, x+r+1
			if out >= 0 {
				sum -= float64(src[row+out])
			}
			if in < w {
				sum += float64(src[row+in])
			}
		}
	}
}

func boxBlurV(src, dst []float32, w, h, r int) {
	norm := 1.0 / float64(2*r+1)
	for x := 0; x < w; x++ {
		var sum float64
		for y := -r; y <= r; y++ {
			if y >= 0 && y < h {
				sum += float64(src[y*w+x])
			}
		}
		for y := 0; y < h; y++ {
			dst[y*w+x] = float32(sum * norm)
			out, in := y-r, y+r+1
			if out >= 0 {
				sum -= float64(src[out*w+x])
			}
			if in < h {
				sum += float64(src[in*w+x])
			}
		}
	}
}

// ringCornerCoverage 返回"外圆角盒 - 内圆角盒（四边宽不同时为椭圆）"在
// 某个角、角块局部坐标 (u,v) 处的环覆盖率（外覆盖 - 内覆盖，夹到 [0,1]）。
//
// 环的几何按"四条直边 + 四个 r×r 角块"分解：直边是实色，角块逐像素算
// 覆盖率。角块的外覆盖按该角的镜像取（右侧/下侧角是左/上角的镜像），
// 内覆盖用内盒相对该角的位置算（内盒相对外盒是偏移的，四侧边框宽不同
// → 四角的内盒位置与内椭圆半轴各不相同）。
func ringCornerCoverage(r int, ir cornerRadii, li, rt, tp, bt, iw, ih, corner, u, v int) float64 {
	ou, ov := u, v
	if corner == 1 || corner == 3 {
		ou = r - 1 - u
	}
	if corner >= 2 {
		ov = r - 1 - v
	}
	outer := cornerCov(r, ou, ov)
	if outer <= 0 {
		return 0
	}
	if iw <= 0 || ih <= 0 {
		return outer // 没有内盒（退化边框）：环即外盒
	}
	cov := outer - ringInnerCoverage(r, ir, li, rt, tp, bt, iw, ih, corner, u, v)
	if cov < 0 {
		cov = 0
	}
	if cov > 1 {
		cov = 1
	}
	return cov
}

// ringInnerCoverage 返回内盒在"外盒某个角的局部坐标 (u,v)"处的覆盖率。
//
// 局部坐标以该角为原点；iu/iv 是像素相对**内盒最近角**向内的距离（右侧
// 与下侧从内盒的右/下边往回数）。像素在内盒之外 → 0；在内盒的角块里 →
// 内椭圆覆盖率；否则（直边段）→ 1。
func ringInnerCoverage(r int, ir cornerRadii, li, rt, tp, bt, iw, ih, corner, u, v int) float64 {
	iu, iv := ringInnerLocal(r, li, rt, tp, bt, corner, u, v)
	if iu < 0 || iv < 0 {
		return 0 // 在内盒之外（角的外侧）
	}
	if iu >= iw || iv >= ih {
		return 0 // 越过内盒的对边
	}
	// 内角是椭圆：只有同时落在两个半轴内的像素才走椭圆覆盖率，
	// 否则已经在内盒的直边段上，整片覆盖。某条边宽度为 0 时对应
	// 半轴等于外半径，内椭圆在空边处与外圆相切，不产生残留条带。
	if ir.rx > 0 && ir.ry > 0 && iu < ir.rx && iv < ir.ry {
		return ellipseCov(ir.rx, ir.ry, iu, iv)
	}
	return 1
}

// ringInnerLocal 返回像素相对内盒最近角向内的距离 (iu, iv)。
func ringInnerLocal(r, li, rt, tp, bt, corner, u, v int) (int, int) {
	switch corner {
	case 0: // 左上
		return u - li, v - tp
	case 1: // 右上
		return r - rt - u - 1, v - tp
	case 2: // 左下
		return u - li, r - bt - v - 1
	default: // 右下
		return r - rt - u - 1, r - bt - v - 1
	}
}

// cornerSprite 返回半径 r、颜色 c、镜像方向 (fx,fy) 的圆角角标精灵。
//
// 精灵是 r×r 的 RGBA8888：alpha = 覆盖率 × 颜色 alpha（与逐像素路径的
// scaledAlpha 同口径），RGB 是颜色本身——blit 的混合结果与逐像素画完全
// 一致。四个角是同一图案的镜像，故按 (fx,fy) 分别缓存。
func cornerSprite(r int, c Color, fx, fy bool) *Sprite {
	cr, cg, cb, ca := c.RGBA()
	key := spriteKey{r: r, cr: cr, cg: cg, cb: cb, ca: ca, fx: fx, fy: fy}
	if s, ok := lookupSprite(key); ok {
		return s
	}
	pix := make([]byte, r*r*4)
	for v := 0; v < r; v++ {
		for u := 0; u < r; u++ {
			su, sv := u, v
			if fx {
				su = r - 1 - u
			}
			if fy {
				sv = r - 1 - v
			}
			cov := cornerCov(r, su, sv)
			na := uint32(float64(ca)*cov + 0.5)
			if na == 0 {
				continue // 完全不着色：blit 时不覆盖底色
			}
			if na > 255 {
				na = 255
			}
			i := (v*r + u) * 4
			pix[i+0], pix[i+1], pix[i+2], pix[i+3] = cr, cg, cb, uint8(na)
		}
	}
	s := &Sprite{W: r, H: r, Pix: pix}
	storeSprite(key, s)
	return s
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

// fillRoundedRectSprites 用「实色矩形 + 四角精灵」画圆角盒。
//
// 覆盖划分：四角各占 r×r 的方块（由精灵画，方块内既含弧边像素也含
// 实心像素），中间的五条矩形（上下两条边带 + 左右两条边带 + 中间带）
// 画实色部分。两者不重叠，每个像素只被画一次——故混合结果与逐像素
// 路径完全一致。
func fillRoundedRectSprites(g Graphics, sd SpriteDrawer, bx, by, bw, bh, r int, c Color) {
	// 半径不能超过短边一半：否则四个角块会互相重叠（同一像素被画两次，
	// alpha 混合叠加 → 颜色变深）。resolveRadius 已做同样的钳制，这里
	// 再兜一次（直接调用本函数的测试/其它调用方未必经过它）。
	if lim := minInt(bw, bh) / 2; r > lim {
		r = lim
	}
	if r <= 0 {
		g.DrawColor(bx, by, bw, bh, c)
		return
	}
	// 中间带（上下各 r 行之外）
	if midY1 := by + bh - r; midY1 > by+r {
		g.DrawColor(bx, by+r, bw, midY1-(by+r), c)
	}
	// 上/下边带（左右各让出 r 像素给角块）
	if segW := bw - 2*r; segW > 0 {
		g.DrawColor(bx+r, by, segW, r, c)
		g.DrawColor(bx+r, by+bh-r, segW, r, c)
	}
	// 左/右边带（上下各让出 r 像素给角块）
	if segH := bh - 2*r; segH > 0 {
		g.DrawColor(bx, by+r, r, segH, c)
		g.DrawColor(bx+bw-r, by+r, r, segH, c)
	}
	// 四角：整块一次 blit（角块内的实心像素也由精灵画，与上面的矩形
	// 不重叠）
	sd.DrawSprite(bx, by, cornerSprite(r, c, false, false))
	sd.DrawSprite(bx+bw-r, by, cornerSprite(r, c, true, false))
	sd.DrawSprite(bx, by+bh-r, cornerSprite(r, c, false, true))
	sd.DrawSprite(bx+bw-r, by+bh-r, cornerSprite(r, c, true, true))
}

// strokeRoundedRectSprites 用「四条直边 + 四角精灵」画圆角边框环。
//
// 覆盖划分：环 = 外圆角盒 - 内圆角盒。直边部分（左右边带 + 上下边带）
// 是实色矩形；四个角的 r×r 块由环精灵画（外覆盖 - 内覆盖）。两者不重叠，
// 每个像素只被画一次——混合结果与逐像素路径一致。
// strokeRoundedRectSprites 是单色兼容入口。
func strokeRoundedRectSprites(g Graphics, sd SpriteDrawer, bx, by, bw, bh, r,
	li, rt, tp, bt, iw, ih int, irs [4]cornerRadii, c Color) {
	strokeRoundedRectSpritesSides(g, sd, bx, by, bw, bh, r, li, rt, tp, bt, iw, ih, irs,
		uniformBorderColors(c))
}

func strokeRoundedRectSpritesSides(g Graphics, sd SpriteDrawer, bx, by, bw, bh, r,
	li, rt, tp, bt, iw, ih int, irs [4]cornerRadii, colors [4]Color) {
	strokeRoundedRectBandsSides(g, bx, by, bw, bh, r, li, rt, tp, bt, colors)
	// 四角：环的角标精灵。相邻两边各取颜色，角块内部按分界线选色。
	// 四角的内椭圆半轴各不同（左右/上下边框宽可能不等），逐个传入。
	sd.DrawSprite(bx, by, ringCornerSpriteSides(r, irs[0], li, rt, tp, bt, iw, ih,
		colors[eTop], colors[eLeft], 0))
	sd.DrawSprite(bx+bw-r, by, ringCornerSpriteSides(r, irs[1], li, rt, tp, bt, iw, ih,
		colors[eTop], colors[eRight], 1))
	sd.DrawSprite(bx, by+bh-r, ringCornerSpriteSides(r, irs[2], li, rt, tp, bt, iw, ih,
		colors[eBottom], colors[eLeft], 2))
	sd.DrawSprite(bx+bw-r, by+bh-r, ringCornerSpriteSides(r, irs[3], li, rt, tp, bt, iw, ih,
		colors[eBottom], colors[eRight], 3))
}

// strokeRoundedRectBands 是单色兼容入口。
func strokeRoundedRectBands(g Graphics, bx, by, bw, bh, r, li, rt, tp, bt int, c Color) {
	strokeRoundedRectBandsSides(g, bx, by, bw, bh, r, li, rt, tp, bt, uniformBorderColors(c))
}

// strokeRoundedRectBandsSides 画圆角边框环的四条直边（实色部分）。
//
// 上下边带左右各让出 r 像素给角块，左右边带上下各让出 r 像素给角块——
// 与角块不重叠，每个像素只画一次。
func strokeRoundedRectBandsSides(g Graphics, bx, by, bw, bh, r, li, rt, tp, bt int, colors [4]Color) {
	if segH := bh - 2*r; segH > 0 {
		if li > 0 && colors[eLeft] != nil {
			g.DrawColor(bx, by+r, li, segH, colors[eLeft])
		}
		if rt > 0 && colors[eRight] != nil {
			g.DrawColor(bx+bw-rt, by+r, rt, segH, colors[eRight])
		}
	}
	if segW := bw - 2*r; segW > 0 {
		if tp > 0 && colors[eTop] != nil {
			g.DrawColor(bx+r, by, segW, tp, colors[eTop])
		}
		if bt > 0 && colors[eBottom] != nil {
			g.DrawColor(bx+r, by+bh-bt, segW, bt, colors[eBottom])
		}
	}
}

// borderPaintState 把计算样式的四边颜色/样式摊平成绘制状态。
//
// 返回的 edges 会把 border-style:none（或透明色）的边宽清零：布局阶段
// 已经按声明宽度占好了位置，绘制时若仍按原宽画环，就会在「本该空着」的
// 边上留下同色底。声明未给颜色时按 currentColor 回退（CSS 初始值），
// 这样 border:1px solid 不写颜色时也能看见；返回的 colors 里，宽度为 0
// 的边保持 nil，供圆角/直角角块回退到邻边颜色。
func borderPaintState(comp CSSStyleDeclaration, bd edges) (edges, [4]Color, bool) {
	var colors [4]Color
	if comp == nil {
		return bd, colors, false
	}
	current := comp.Color()
	if current == nil {
		current = NewColor(0, 0, 0, 255)
	}
	has := false
	for i := 0; i < 4; i++ {
		side := BorderSide(i)
		if comp.BorderStyleFor(side) == BorderStyleNone {
			bd[i] = 0
			continue
		}
		c := comp.BorderColorFor(side)
		if c == nil {
			c = current
		}
		if _, _, _, a := c.RGBA(); a == 0 {
			bd[i] = 0
			continue
		}
		if bd[i] <= 0 {
			// 宽度为 0 的边不参与绘制，颜色也必须留 nil：圆角角块会
			// 在相邻边之间选色，若这里填成 currentColor，`border-bottom:0`
			// 的底角会被错画成黑色（而不是只用左/右边色）。
			continue
		}
		colors[i] = c
		has = true
	}
	return bd, colors, has
}

// sameColor 按 RGBA 比较颜色。Color 是接口，直接 == 比的是动态指针
// （NewColor 每次返回新指针），值相同也会判不等。
func sameColor(a, b Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// uniformBorderColors 把单色包成四边同色，供聚焦框/旧测试等单色调用复用
// 多色绘制路径。
func uniformBorderColors(c Color) [4]Color {
	return [4]Color{c, c, c, c}
}

// strokeRoundedRect 是单色兼容入口。
func strokeRoundedRect(g Graphics, bx, by, bw, bh, r int, bd edges, c Color) {
	strokeRoundedRectSides(g, bx, by, bw, bh, r, bd, uniformBorderColors(c))
}

// strokeRoundedRectSides 绘制四边可独立着色的圆角边框环。
func strokeRoundedRectSides(g Graphics, bx, by, bw, bh, r int, bd edges, colors [4]Color) {
	if r <= 0 {
		li, rt, tp, bt := maxZero(bd[eLeft]), maxZero(bd[eRight]), maxZero(bd[eTop]), maxZero(bd[eBottom])
		// 四边同色且不透明（最常见的 1px 边框）直接让上下横带盖住左右：
		// 没有斜接需求，也避免给每个盒子多出四次角块绘制。半透明同色
		// 不能走这条：横带在角上叠两次会变得比声明更不透明。
		uniform := false
		if c := colors[eTop]; c != nil {
			_, _, _, ca := c.RGBA()
			uniform = ca == 255 &&
				sameColor(c, colors[eRight]) &&
				sameColor(c, colors[eBottom]) &&
				sameColor(c, colors[eLeft])
		}
		if uniform {
			if li > 0 {
				g.DrawColor(bx, by, li, bh, colors[eLeft])
			}
			if rt > 0 {
				g.DrawColor(bx+bw-rt, by, rt, bh, colors[eRight])
			}
			if tp > 0 {
				g.DrawColor(bx, by, bw, tp, colors[eTop])
			}
			if bt > 0 {
				g.DrawColor(bx, by+bh-bt, bw, bt, colors[eBottom])
			}
			return
		}
		// 异色：直边让出角块，角块按「外角到内角」的斜线只落一次色。
		// 这样半透明边框在角上不会被两条带叠两次。
		cw, ch := bw-li-rt, bh-tp-bt
		if cw < 0 || ch < 0 {
			// 边框比盒子还宽：退化成立即覆盖，保证至少能看见颜色。
			if li > 0 && colors[eLeft] != nil {
				g.DrawColor(bx, by, li, bh, colors[eLeft])
			}
			if rt > 0 && colors[eRight] != nil {
				g.DrawColor(bx+bw-rt, by, rt, bh, colors[eRight])
			}
			if tp > 0 && colors[eTop] != nil {
				g.DrawColor(bx, by, bw, tp, colors[eTop])
			}
			if bt > 0 && colors[eBottom] != nil {
				g.DrawColor(bx, by+bh-bt, bw, bt, colors[eBottom])
			}
			return
		}
		if li > 0 && ch > 0 && colors[eLeft] != nil {
			g.DrawColor(bx, by+tp, li, ch, colors[eLeft])
		}
		if rt > 0 && ch > 0 && colors[eRight] != nil {
			g.DrawColor(bx+bw-rt, by+tp, rt, ch, colors[eRight])
		}
		if tp > 0 && cw > 0 && colors[eTop] != nil {
			g.DrawColor(bx+li, by, cw, tp, colors[eTop])
		}
		if bt > 0 && cw > 0 && colors[eBottom] != nil {
			g.DrawColor(bx+li, by+bh-bt, cw, bt, colors[eBottom])
		}
		strokeSquareCornersSides(g, bx, by, bw, bh, li, rt, tp, bt, colors)
		return
	}
	li, rt, tp, bt := maxZero(bd[eLeft]), maxZero(bd[eRight]), maxZero(bd[eTop]), maxZero(bd[eBottom])
	iw, ih := bw-li-rt, bh-tp-bt
	irs := innerCornerRadii(r, li, rt, tp, bt, iw, ih)
	// 后端支持位图 blit 时：四条直边 + 四角精灵（8 次绘制），替掉逐行逐
	// 像素画整圈环（一个盒子几十次 1×1 绘制，是绘制调用的主要来源）。
	if sd, ok := g.(SpriteDrawer); ok {
		strokeRoundedRectSpritesSides(g, sd, bx, by, bw, bh, r, li, rt, tp, bt, iw, ih, irs, colors)
		return
	}
	strokeRoundedRectPixelsSides(g, bx, by, bw, bh, r, li, rt, tp, bt, iw, ih, irs, colors)
}

// innerCornerRadii 返回四个内圆角的椭圆半轴（顺序同 corner：左上、右上、
// 左下、右下）。
//
// 内椭圆与外圆同心：横向半轴扣掉该角左右方向的边框宽，纵向半轴扣掉上下
// 方向的边框宽。四边等宽时两轴相等，退化成原来的内圆；某条边宽度为 0 时
// 该轴保持外半径，内椭圆与外圆在空边处相切，因此 `border-bottom:0` 不会
// 在底边留下一小段由左/右边色画出的「假底边」。iw/ih 是内盒尺寸，用于
// 边框极厚、盒极窄时的夹紧。
func innerCornerRadii(r, li, rt, tp, bt, iw, ih int) [4]cornerRadii {
	clamp := func(v, lim int) int {
		if v < 0 {
			return 0
		}
		if lim > 0 && v > lim {
			return lim
		}
		return v
	}
	// 左上、右上、左下、右下。
	return [4]cornerRadii{
		{clamp(r-li, iw), clamp(r-tp, ih)},
		{clamp(r-rt, iw), clamp(r-tp, ih)},
		{clamp(r-li, iw), clamp(r-bt, ih)},
		{clamp(r-rt, iw), clamp(r-bt, ih)},
	}
}

// strokeSquareCornersSides 给 r=0 的直角边框补斜接（miter）角。
//
// 直角边框的角区是「横向边 × 纵向边」的矩形，CSS 用外角到内角的斜线
// 分成两块；两邻边宽度不同时（如 top=4、left=2）斜线按 4:2 的比例走，
// 不是 45°。这里逐行算出斜线位置，每行最多两次 DrawColor，避免在粗
// 边框上出现上/下边整块盖住左右边的方块角。
func strokeSquareCornersSides(g Graphics, bx, by, bw, bh, li, rt, tp, bt int, colors [4]Color) {
	top, right, bottom, left := colors[eTop], colors[eRight], colors[eBottom], colors[eLeft]
	// fillOrSplit 先处理「同色/只有一边有色」的简单情形，再按斜线逐行
	// 分色；斜线位置用外角到内角的方向向量，宽度不等时自然得到非 45°。
	fillOrSplit := func(x, y, w, h int, horiz, vert Color, vertRight, fromBottom bool) {
		if w <= 0 || h <= 0 {
			return
		}
		switch {
		case horiz == nil && vert == nil:
			return
		case vert == nil:
			g.DrawColor(x, y, w, h, horiz)
			return
		case horiz == nil:
			g.DrawColor(x, y, w, h, vert)
			return
		case sameColor(horiz, vert):
			g.DrawColor(x, y, w, h, horiz)
			return
		}
		// 横向边靠外角一侧，纵向边在另一侧。vertRight=true 表示纵向边
		// 在角块右边（上右/下右），同一条斜线要镜像到右侧。
		for v := 0; v < h; v++ {
			u0 := (v*w + h - 1) / h
			if u0 > w {
				u0 = w
			}
			yy := y + v
			if fromBottom {
				yy = y + h - 1 - v // 下侧角块从外角（底行）往里扫
			}
			if u0 > 0 {
				if vertRight {
					g.DrawColor(x+w-u0, yy, u0, 1, vert)
				} else {
					g.DrawColor(x, yy, u0, 1, vert)
				}
			}
			if u0 < w {
				if vertRight {
					g.DrawColor(x, yy, w-u0, 1, horiz)
				} else {
					g.DrawColor(x+u0, yy, w-u0, 1, horiz)
				}
			}
		}
	}
	// 上左/下左：纵向边在左。
	fillOrSplit(bx, by, li, tp, top, left, false, false)
	fillOrSplit(bx, by+bh-bt, li, bt, bottom, left, false, true)
	// 上右/下右：纵向边在右。
	fillOrSplit(bx+bw-rt, by, rt, tp, top, right, true, false)
	fillOrSplit(bx+bw-rt, by+bh-bt, rt, bt, bottom, right, true, true)
}

// strokeRoundedRectPixels 是单色兼容入口。
func strokeRoundedRectPixels(g Graphics, bx, by, bw, bh, r, li, rt, tp, bt,
	iw, ih int, irs [4]cornerRadii, c Color) {
	strokeRoundedRectPixelsSides(g, bx, by, bw, bh, r, li, rt, tp, bt, iw, ih, irs,
		uniformBorderColors(c))
}

// strokeRoundedRectPixelsSides 逐行逐像素画圆角边框环（不支持精灵 blit 的
// 后端走这条路径）。几何与精灵路径同构：四条直边 + 四个 r×r 角块（角块
// 逐像素算"外覆盖 - 内覆盖"），两者不重叠——输出与精灵路径逐像素一致。
func strokeRoundedRectPixelsSides(g Graphics, bx, by, bw, bh, r, li, rt, tp, bt,
	iw, ih int, irs [4]cornerRadii, colors [4]Color) {
	strokeRoundedRectBandsSides(g, bx, by, bw, bh, r, li, rt, tp, bt, colors)
	// 四个角块：逐点算环覆盖率后按 alpha 画 1×1。角上相邻两边颜色不同
	// 时按角平分线（u/v 与各自外角的距离比较）分色。
	corners := [4][2]int{{bx, by}, {bx + bw - r, by}, {bx, by + bh - r}, {bx + bw - r, by + bh - r}}
	for ci, o := range corners {
		for v := 0; v < r; v++ {
			for u := 0; u < r; u++ {
				cov := ringCornerCoverage(r, irs[ci], li, rt, tp, bt, iw, ih, ci, u, v)
				if cov <= 0 {
					continue
				}
				c := cornerBandColor(colors, ci, r, u, v)
				if c == nil {
					continue
				}
				if ec, ok := scaledAlpha(c, cov); ok {
					g.DrawColor(o[0]+u, o[1]+v, 1, 1, ec)
				}
			}
		}
	}
}

// cornerBandColor 选择圆角角块里某个像素属于哪条邻边。
//
// 两条边的颜色分界是过外角与内角的 45° 线：分别量像素到外角在 x/y 方向
// 的距离，横向边（上/下）在 du > dv 一侧，纵向边（左/右）在另一侧。
// Chrome 在角上做的是渐变过渡，这里用硬分界近似——角只有 r×r 像素，
// 单边边框的常见场景里看不出差别。
func cornerBandColor(colors [4]Color, corner, r, u, v int) Color {
	du, dv := u, v
	if corner == 1 || corner == 3 {
		du = r - 1 - u
	}
	if corner >= 2 {
		dv = r - 1 - v
	}
	horizontal, vertical := colors[eTop], colors[eLeft]
	switch corner {
	case 1:
		vertical = colors[eRight]
	case 2:
		horizontal = colors[eBottom]
	case 3:
		horizontal, vertical = colors[eBottom], colors[eRight]
	}
	// 宽度为 0 的边颜色是 nil（见 borderPaintState）：角块里属于它的
	// 那片区域应改用邻边颜色，不能直接跳过——否则不支持精灵的后端会在
	// 角上留下透明缺口。精灵路径的 ringCornerSpriteSides 同口径。
	if horizontal == nil {
		horizontal = vertical
	}
	if vertical == nil {
		vertical = horizontal
	}
	if du > dv {
		return horizontal
	}
	return vertical
}
