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
	vpW, vpH := 0, 0
	if d, ok := node.(*htmlDocument); ok {
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
	renderNode(g, e, nil, focus, vpW, vpH)
	// 定位元素画完后，给它最近的「圆角 + 裁剪」祖先补盖四角：定位内容
	// 是在定位 pass 里画的（主 pass 早已补盖过一次），不补盖就会盖回
	// 圆角（大黑卡的右上角被溢出到角外的圆形装饰盖成方角）。
	// 只在定位元素确实压到角区时才补（否则每个定位元素都要多画一遍）。
	if box := nearestRoundedClipAncestor(e); box != nil && overlapsCornerAreas(e, box) {
		drawClipCornerMask(g, box, false)
	}
	if createsStackingContext(e) {
		for _, pb := range collectPositioned(e) {
			paintPositioned(g, pb.e, focus, vpW, vpH)
		}
	}
	popClips(g, n)
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

		if inView {
			if bg := comp.BackgroundColor(); bg != nil {
				fillRoundedRect(g, bx, by, bw, bh, r, bg)
			}
			if bc := comp.BorderColor(); bc != nil && comp.BorderStyle() != BorderStyleNone && bw > 0 && bh > 0 {
				strokeRoundedRect(g, bx, by, bw, bh, r, bd, bc)
			}
		}

		// 聚焦框画在边框盒上，必须早于下方 overflow 裁剪压栈：textarea 默认
		// overflow:hidden，裁剪区是 padding 盒，画在边框盒上的聚焦框会被整圈
		// 裁掉（此前聚焦框在裁剪栈内绘制 → textarea 永远看不到聚焦指示）。
		// 单选/复选此前就没有聚焦框（Chrome 同款圆环由选中标记承担），保持。
		if inView && focused && isFormControl(base) && bw > 0 && bh > 0 && checkKind(base) == "" {
			strokeRoundedRect(g, bx, by, bw, bh, r, edges{1, 1, 1, 1}, focusBlue)
		}

		// 列表标记：引擎无 list-item 概念，标记文本由布局期算出（ul → "•"，
		// ol → "1." "2."…），画在内容左缘左侧、右对齐到 −0.75em 处
		//（Chrome 实测：标记盒右缘 = 内容左缘 − 0.5em）。
		if inView {
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
		if inView && isFormControl(base) && bw > 0 && bh > 0 {
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
		renderNodeTextColor(g, child, p, focus, textColor, vpW, vpH)
	}
	if clipped {
		// 圆角裁剪盒：把四角"圆角形状之外"的部分用元素背后的底色盖掉，
		// 等效于按 border-radius 裁剪（详见 clipCornerMask 的说明）。
		if r > 0 {
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
		if bc := comp.BorderColor(); bc != nil && comp.BorderStyle() != BorderStyleNone && bw > 0 && bh > 0 {
			strokeRoundedRect(g, bx, by, bw, bh, r, bd, bc)
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

// ---------------------------------------------------------------------------
// 圆角角标精灵
// ---------------------------------------------------------------------------

// spriteKey 是角标精灵的缓存键。
//
// 精灵只与（半径、颜色、镜像方向）有关，与盒的大小无关——同一页面上大量
// 同样式的圆角卡片共用同一份像素。
//
// 边框环的精灵多几个维度：内圆角半径 ir、四侧边框宽 (li,rt,tp,bt)、内盒
// 尺寸 (iw,ih)、角序号 corner。环的内盒相对外盒是偏移的，四侧边框宽不同
// → 四角并不互为镜像，故按角分别缓存（ring 为真时这些字段才有效）。
type spriteKey struct {
	r              int
	ir             int
	li, rt, tp, bt int
	iw, ih         int
	cr, cg, cb, ca uint8
	fx, fy         bool
	mask           bool // 补盖精灵（圆角之外的部分），与普通角标图案不同
	ring           bool // 边框环角标
	corner         int  // 0=左上 1=右上 2=左下 3=右下（ring 时有效）
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
func ringCornerSprite(r, ir, li, rt, tp, bt, iw, ih int, c Color, corner int) *Sprite {
	cr, cg, cb, ca := c.RGBA()
	key := spriteKey{r: r, ir: ir, li: li, rt: rt, tp: tp, bt: bt,
		iw: iw, ih: ih, cr: cr, cg: cg, cb: cb, ca: ca, ring: true, corner: corner}
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

// ringCornerCoverage 返回"外圆角盒 - 内圆角盒"在某个角、角块局部坐标
// (u,v) 处的环覆盖率（外覆盖 - 内覆盖，夹到 [0,1]）。
//
// 环的几何按"四条直边 + 四个 r×r 角块"分解：直边是实色，角块逐像素算
// 覆盖率。角块的外覆盖按该角的镜像取（右侧/下侧角是左/上角的镜像），
// 内覆盖用内盒相对该角的位置算（内盒相对外盒是偏移的，四侧边框宽不同
// → 四角的内盒位置各不相同）。
func ringCornerCoverage(r, ir, li, rt, tp, bt, iw, ih, corner, u, v int) float64 {
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
// 角覆盖率；否则（直边段）→ 1。
func ringInnerCoverage(r, ir, li, rt, tp, bt, iw, ih, corner, u, v int) float64 {
	iu, iv := ringInnerLocal(r, li, rt, tp, bt, corner, u, v)
	if iu < 0 || iv < 0 {
		return 0 // 在内盒之外（角的外侧）
	}
	if iu >= iw || iv >= ih {
		return 0 // 越过内盒的对侧边
	}
	if iu < ir && iv < ir {
		return cornerCov(ir, iu, iv)
	}
	return 1 // 内盒的直边段：整片覆盖
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
func strokeRoundedRectSprites(g Graphics, sd SpriteDrawer, bx, by, bw, bh, r,
	li, rt, tp, bt, iw, ih, ir int, c Color) {
	strokeRoundedRectBands(g, bx, by, bw, bh, r, li, rt, tp, bt, c)
	// 四角：环的角标精灵
	sd.DrawSprite(bx, by, ringCornerSprite(r, ir, li, rt, tp, bt, iw, ih, c, 0))
	sd.DrawSprite(bx+bw-r, by, ringCornerSprite(r, ir, li, rt, tp, bt, iw, ih, c, 1))
	sd.DrawSprite(bx, by+bh-r, ringCornerSprite(r, ir, li, rt, tp, bt, iw, ih, c, 2))
	sd.DrawSprite(bx+bw-r, by+bh-r, ringCornerSprite(r, ir, li, rt, tp, bt, iw, ih, c, 3))
}

// strokeRoundedRectBands 画圆角边框环的四条直边（实色部分）。
//
// 上下边带左右各让出 r 像素给角块，左右边带上下各让出 r 像素给角块——
// 与角块不重叠，每个像素只画一次。
func strokeRoundedRectBands(g Graphics, bx, by, bw, bh, r, li, rt, tp, bt int, c Color) {
	if segH := bh - 2*r; segH > 0 {
		if li > 0 {
			g.DrawColor(bx, by+r, li, segH, c)
		}
		if rt > 0 {
			g.DrawColor(bx+bw-rt, by+r, rt, segH, c)
		}
	}
	if segW := bw - 2*r; segW > 0 {
		if tp > 0 {
			g.DrawColor(bx+r, by, segW, tp, c)
		}
		if bt > 0 {
			g.DrawColor(bx+r, by+bh-bt, segW, bt, c)
		}
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
	// 后端支持位图 blit 时：四条直边 + 四角精灵（8 次绘制），替掉逐行逐
	// 像素画整圈环（一个盒子几十次 1×1 绘制，是绘制调用的主要来源）。
	if sd, ok := g.(SpriteDrawer); ok {
		strokeRoundedRectSprites(g, sd, bx, by, bw, bh, r, li, rt, tp, bt, iw, ih, ir, c)
		return
	}
	strokeRoundedRectPixels(g, bx, by, bw, bh, r, li, rt, tp, bt, iw, ih, ir, c)
}

// strokeRoundedRectPixels 逐行逐像素画圆角边框环（不支持精灵 blit 的后端
// 走这条路径）。几何与精灵路径同构：四条直边 + 四个 r×r 角块（角块逐
// 像素算"外覆盖 - 内覆盖"），两者不重叠——输出与精灵路径逐像素一致。
func strokeRoundedRectPixels(g Graphics, bx, by, bw, bh, r, li, rt, tp, bt,
	iw, ih, ir int, c Color) {
	strokeRoundedRectBands(g, bx, by, bw, bh, r, li, rt, tp, bt, c)
	// 四个角块：逐点算环覆盖率后按 alpha 画 1×1。
	corners := [4][2]int{{bx, by}, {bx + bw - r, by}, {bx, by + bh - r}, {bx + bw - r, by + bh - r}}
	for ci, o := range corners {
		for v := 0; v < r; v++ {
			for u := 0; u < r; u++ {
				cov := ringCornerCoverage(r, ir, li, rt, tp, bt, iw, ih, ci, u, v)
				if cov <= 0 {
					continue
				}
				if ec, ok := scaledAlpha(c, cov); ok {
					g.DrawColor(o[0]+u, o[1]+v, 1, 1, ec)
				}
			}
		}
	}
}
