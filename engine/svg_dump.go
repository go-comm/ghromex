package engine

import (
	"fmt"
	"math"
	"strings"
)

// DumpSVG 将布局结果导出为 SVG 文本：背景/边框为 rect，文本片段为 text。
// 用于无显示器环境验证布局与绘制意图。
func (document *htmlDocument) DumpSVG() string {
	v := document.viewport
	w, h := 0, 0
	if v != nil {
		w, h = v.ViewportWidth(), v.ViewportHeight()
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", w, h, w, h)
	// 画布底色同渲染路径：CSS canvas propagation（html/body 背景），无则白。
	fill := "#ffffff"
	if bg := CanvasBackground(document); bg != nil {
		r, g, bl, _ := bg.RGBA()
		fill = fmt.Sprintf("#%02x%02x%02x", r, g, bl)
	}
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" fill="%s"/>`+"\n", w, h, fill)
	ctr := 0
	dumpSVGNode(&b, document, nil, &ctr)
	// 定位层：与 RenderNode 同一层叠模型（主 pass 已跳过 positioned 子树）；
	// z-index/fixed 建立的层叠上下文内部再递归导出，祖先 overflow 裁剪
	// 按父链补上（与渲染路径 pushAncestorClips 同构）。
	for _, pb := range collectPositioned(document) {
		dumpSVGPositioned(&b, pb.e, &ctr)
	}
	// select 选项浮层最后导出（与渲染层序一致：覆盖流内容与定位层）
	for _, sel := range collectOpenSelects(document) {
		dumpSelectPopup(&b, sel, &ctr)
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// svgClip 是一个已注册的裁剪引用（clipPath id + 区域）。
type svgClip struct {
	id string
	r  clipRect
}

// ancestorSVGClips 沿父链收集 overflow 裁剪盒，逐个注册 clipPath（id 递增）。
func ancestorSVGClips(e HTMLElement, ctr *int) []svgClip {
	var chain []*htmlElement
	for p := e.ParentElement(); p != nil; p = p.ParentElement() {
		if b := inner(p); b != nil && isClippingBox(b) {
			chain = append(chain, b)
		}
	}
	out := make([]svgClip, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		out = append(out, makeSVGClip(chain[i], ctr))
	}
	return out
}

// makeSVGClip 为元素的 padding 盒注册一个 clipPath 并返回引用。
func makeSVGClip(base *htmlElement, ctr *int) svgClip {
	pd, _ := resolvedEdges(base, base.computed)
	r := clipRect{
		x: base.x - pd[eLeft], y: base.y - pd[eTop],
		w: base.width + pd[eLeft] + pd[eRight],
		h: base.height + pd[eTop] + pd[eBottom],
	}
	id := fmt.Sprintf("clip%d", *ctr)
	*ctr++
	return svgClip{id: id, r: r}
}

// openSVGClip 输出 clipPath 定义 + 开标签，返回追加后的裁剪链。
func openSVGClip(b *strings.Builder, clips []svgClip, c svgClip) []svgClip {
	fmt.Fprintf(b, `<clipPath id="%s"><rect x="%d" y="%d" width="%d" height="%d"/></clipPath>`+"\n",
		c.id, c.r.x, c.r.y, c.r.w, c.r.h)
	fmt.Fprintf(b, `<g clip-path="url(#%s)">`+"\n", c.id)
	return append(append([]svgClip{}, clips...), c)
}

// dumpSelectPopup 导出展开的 select 浮层：容器底/边框 + 选中项高亮 + 各 option。
func dumpSelectPopup(b *strings.Builder, sel *htmlElement, ctr *int) {
	opts := selectOptions(sel.self)
	if len(opts) == 0 {
		return
	}
	x0, y0, x1, y1, ok := selectPopupBounds(sel)
	if !ok || x1 <= x0 || y1 <= y0 {
		return
	}
	// 底 → 选中项高亮 → 描边（描边必须在高亮之后，否则被高亮盖掉）
	fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="#ffffff"/>`+"\n",
		x0, y0, x1-x0, y1-y0)
	s := selectedOptionBase(sel)
	if s != nil {
		if sx, sy, sw, sh := borderBoxRect(s); sw > 0 && sh > 0 {
			fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n",
				sx, sy, sw, sh, hexColor(0x19, 0x67, 0xD2))
		}
	}
	fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="#767676" stroke-width="1"/>`+"\n",
		x0, y0, x1-x0, y1-y0)
	for _, o := range opts {
		// 选中项文字白字，与渲染路径 renderNodeTextColor 同构（Chrome #1967D2 底白字）
		if o == s {
			dumpSVGNodeFill(b, o, nil, ctr, "#ffffff")
		} else {
			dumpSVGNode(b, o, nil, ctr)
		}
	}
}

// dumpSVGPositioned 导出定位元素；若它建立层叠上下文，再按同一模型
// 递归导出上下文内部的定位子树。
func dumpSVGPositioned(b *strings.Builder, e HTMLElement, ctr *int) {
	dumpSVGNode(b, e, ancestorSVGClips(e, ctr), ctr)
	if createsStackingContext(e) {
		for _, pb := range collectPositioned(e) {
			dumpSVGPositioned(b, pb.e, ctr)
		}
	}
}

func dumpSVGNode(b *strings.Builder, e HTMLElement, clips []svgClip, ctr *int) {
	dumpSVGNodeFill(b, e, clips, ctr, "")
}

// dumpSVGNodeFill 与 dumpSVGNode 同构，fillOverride 非空时覆盖本子树的文字
// 填充色（select 浮层选中项白字；仅作用于 text 的 fill，盒背景/边框不变）。
func dumpSVGNodeFill(b *strings.Builder, e HTMLElement, clips []svgClip, ctr *int, fillOverride string) {
	if e == nil {
		return
	}
	base := inner(e)
	if base == nil {
		return
	}
	comp := base.computed
	if comp != nil && comp.Display() == DisplayNone {
		return
	}

	if tn, ok := e.(*textNode); ok {
		fs := 16
		fill := "#000000"
		fw := ""
		fam := ""
		if base.computed != nil {
			if s := resolveLen(base.computed.FontSize(), 0); s > 0 {
				fs = s
			}
			if c := base.computed.Color(); c != nil {
				r, g, bl, _ := c.RGBA()
				fill = fmt.Sprintf("#%02x%02x%02x", r, g, bl)
			}
			if base.computed.FontWeight() == FontWeightBold {
				fw = ` font-weight="bold"`
			}
			fam = base.computed.FontFamily()
		}
		if fillOverride != "" {
			fill = fillOverride
		}
		ff := fmt.Sprintf(` font-family="%s"`, escapeXML(svgFontStack(fam)))
		decAttr := ""
		if base.computed != nil {
			switch base.computed.TextDecoration() {
			case TextDecorationUnderline:
				decAttr = ` text-decoration="underline"`
			case TextDecorationLineThrough:
				decAttr = ` text-decoration="line-through"`
			}
		}
		for _, run := range tn.node.runs {
			// y 是 SVG 的基线坐标：基线 = 片段顶边 + 该片段的 ascent
			// （与下划线等装饰同口径，见 drawDecoration）。用片段底边
			// （y+h）会把文字整体画低一个 descent。
			fmt.Fprintf(b, `<text x="%d" y="%d" font-size="%d"%s%s%s fill="%s">%s</text>`+"\n",
				run.x, run.y+run.asc, fs, ff, fw, decAttr, fill, escapeXML(run.text))
		}
		return
	}

	if comp != nil {
		pd, bd := resolvedEdges(base, comp)
		bx := base.x - pd[eLeft] - bd[eLeft]
		by := base.y - pd[eTop] - bd[eTop]
		bw := base.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
		bh := base.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
		br := resolveRadius(comp, bw, bh)
		// 与渲染路径同口径：radio 盒恒为正圆（见 renderNode）
		if checkKind(base) == "radio" {
			br = (minInt(bw, bh) + 1) / 2
		}
		rxAttr := ""
		if br > 0 {
			rxAttr = fmt.Sprintf(` rx="%d"`, br)
		}
		// 阴影在背景之前导出：与渲染路径同序。SVG 用 feGaussianBlur 表达
		// 模糊，标准偏差取 CSS 模糊半径的一半（CSS 规范：模糊半径 = 2σ）。
		shadows := comp.BoxShadow()
		for i := len(shadows) - 1; i >= 0; i-- {
			sh := shadows[i]
			if sh.Inset {
				continue
			}
			c := sh.Color
			if c == nil {
				c = comp.Color()
			}
			if c == nil {
				c = NewColor(0, 0, 0, 255)
			}
			r, g, bl, al := c.RGBA()
			if al == 0 {
				continue
			}
			sw, shh := bw+2*sh.Spread, bh+2*sh.Spread
			if sw <= 0 || shh <= 0 {
				continue
			}
			sr := br + sh.Spread
			if sr < 0 {
				sr = 0
			}
			if lim := minInt(sw, shh) / 2; sr > lim {
				sr = lim
			}
			sx, sy := bx+sh.X-sh.Spread, by+sh.Y-sh.Spread
			srx := ""
			if sr > 0 {
				srx = fmt.Sprintf(` rx="%d"`, sr)
			}
			// 外阴影只画在边框盒之外：用 mask 的白矩形表示阴影形状，
			// 再用黑矩形（元素边框盒）挖洞。mask 加在 filter 之后，
			// 等价于浏览器「先模糊、再按边框盒裁切」。
			pad := maxZero(sh.Blur)
			filterDef := ""
			filterAttr := ""
			if sh.Blur > 0 {
				fid := fmt.Sprintf("bshadow%d", *ctr)
				*ctr++
				filterAttr = fmt.Sprintf(` filter="url(#%s)"`, fid)
				// 用 userSpaceOnUse 把滤镜区域按 blur 扩到阴影外接盒，
				// 不能写 -50%/200%：小盒子配大模糊时百分比区域不够，
				// 模糊尾巴会被 SVG 裁掉。
				filterDef = fmt.Sprintf(`<filter id="%s" filterUnits="userSpaceOnUse" x="%d" y="%d" width="%d" height="%d">`+
					`<feGaussianBlur stdDeviation="%.1f"/></filter>`,
					fid, sx-pad, sy-pad, sw+2*pad, shh+2*pad, float64(sh.Blur)/2)
			}
			mid := fmt.Sprintf("bshadowmask%d", *ctr)
			*ctr++
			// 白色矩形铺满整个 mask 区域（含模糊外扩），黑色矩形挖掉
			// 元素边框盒。若白矩形只画阴影本体，模糊到本体外的尾巴会被
			// mask 的透明区裁掉，软边变成硬边。
			maskDef := fmt.Sprintf(`<mask id="%s" maskUnits="userSpaceOnUse" x="%d" y="%d" width="%d" height="%d">`+
				`<rect x="%d" y="%d" width="%d" height="%d" fill="#ffffff"/>`+
				`<rect x="%d" y="%d" width="%d" height="%d" fill="#000000"%s/></mask>`,
				mid, sx-pad, sy-pad, sw+2*pad, shh+2*pad,
				sx-pad, sy-pad, sw+2*pad, shh+2*pad,
				bx, by, bw, bh, rxAttr)
			fmt.Fprintf(b, `<defs>%s%s</defs>`+"\n", filterDef, maskDef)
			fmt.Fprintf(b, `<g mask="url(#%s)"><rect x="%d" y="%d" width="%d" height="%d" fill="%s" fill-opacity="%.2f"%s%s/></g>`+"\n",
				mid, sx, sy, sw, shh, hexColor(r, g, bl), float64(al)/255, srx, filterAttr)
		}
		// empty-cells: hide 的单元格不导出背景/边框（与渲染路径同口径）。
		if bg := comp.BackgroundColor(); bg != nil && bw > 0 && bh > 0 && !base.noCellDeco {
			r, g, bl, al := bg.RGBA()
			if al > 0 {
				fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" fill-opacity="%.2f"%s/>`+"\n",
					bx, by, bw, bh, hexColor(r, g, bl), float64(al)/255, rxAttr)
			}
		}
		if bw > 0 && bh > 0 && !base.noCellDeco {
			paintBD, colors, has := borderPaintState(comp, bd)
			if has {
				uniform := true
				for i := 1; i < 4; i++ {
					if paintBD[i] != paintBD[0] || !sameColor(colors[i], colors[0]) {
						uniform = false
						break
					}
				}
				if uniform && paintBD[0] > 0 {
					// 四边同宽同色：沿用单个 stroke rect，圆角几何最准。
					lw := paintBD[0]
					r, g, bl, _ := colors[0].RGBA()
					srx := ""
					if br > 0 { // 描边路径中心线内缩 lw/2，rx 相应修正
						srx = fmt.Sprintf(` rx="%d"`, maxZero(br-lw/2))
					}
					fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="%s" stroke-width="%d"%s/>`+"\n",
						bx+lw/2, by+lw/2, bw-lw, bh-lw, hexColor(r, g, bl), lw, srx)
				} else {
					// 四边不同色/不同宽：SVG 的 stroke 只能单色，改为四块
					// 实色矩形，并用外圆角矩形裁剪。圆角内缘是直角（调试
					// 导出可接受的近似），但单边边框的颜色与位置能对上。
					id := fmt.Sprintf("brd%d", *ctr)
					*ctr++
					fmt.Fprintf(b, `<clipPath id="%s"><rect x="%d" y="%d" width="%d" height="%d"%s/></clipPath>`+"\n",
						id, bx, by, bw, bh, rxAttr)
					fmt.Fprintf(b, `<g clip-path="url(#%s)">`+"\n", id)
					emit := func(x, y, w, h int, c Color) {
						if w <= 0 || h <= 0 || c == nil {
							return
						}
						r, g, bl, _ := c.RGBA()
						fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n",
							x, y, w, h, hexColor(r, g, bl))
					}
					emit(bx, by, bw, paintBD[eTop], colors[eTop])
					emit(bx, by+bh-paintBD[eBottom], bw, paintBD[eBottom], colors[eBottom])
					emit(bx, by, paintBD[eLeft], bh, colors[eLeft])
					emit(bx+bw-paintBD[eRight], by, paintBD[eRight], bh, colors[eRight])
					b.WriteString("</g>\n")
				}
			}
		}
	}

	// 列表标记：与渲染路径同口径（内容左缘左侧、右对齐，间隙按标记类型
	// 分别取 Chrome 实测值；垂直对齐首行），否则 SVG 导出与屏幕渲染不同构
	//（看不到圆点/序号）。
	if base.listMarker != "" {
		fpx, _, ffam := fontOf(comp)
		mw, _ := fakeMeasureText(base.listMarker, fpx)
		_, lh := fakeMeasureText("Mg", fpx)
		if lh <= 0 {
			lh = base.height
		}
		gap := fpx * 3 / 5
		if base.listMarker == "•" {
			gap = fpx * 7 / 8
		}
		mx := base.x - gap - mw
		my := base.y
		fill := "#000000"
		if c := comp.Color(); c != nil {
			r, g, bl, _ := c.RGBA()
			fill = hexColor(r, g, bl)
		}
		fmt.Fprintf(b, `<text x="%d" y="%d" font-size="%d" font-family="%s" fill="%s">%s</text>`+"\n",
			mx, my+lh, fpx, escapeXML(svgFontStack(ffam)), fill, escapeXML(base.listMarker))
	}

	// overflow 非 visible：控件文本与子树包进 clip-path（与渲染路径同构）。
	// 边框在裁剪外，已在上方输出；clipPath 定义就地内联（不渲染）。
	clipped := isClippingBox(base)
	if clipped {
		clips = openSVGClip(b, clips, makeSVGClip(base, ctr))
	}

	if comp != nil && isFormControl(base) {
		pd, bd := resolvedEdges(base, comp)
		dumpControlText(b, base, comp,
			base.x-pd[eLeft]-bd[eLeft], base.y-pd[eTop]-bd[eTop],
			base.width+pd[eLeft]+pd[eRight]+bd[eLeft]+bd[eRight],
			base.height+pd[eTop]+pd[eBottom]+bd[eTop]+bd[eBottom])
	}

	for _, child := range base.children {
		if isPositioned(child) {
			continue // 由 DumpSVG 的定位层统一追加
		}
		if isFormControl(base) {
			continue // 控件文本已由 dumpControlText 输出（select 浮层另走一层）
		}
		dumpSVGNodeFill(b, child, clips, ctr, fillOverride)
	}
	if clipped {
		b.WriteString("</g>\n")
	}
}

func hexColor(r, g, b uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// dumpControlText 导出表单控件的文本（与 render.go 同构的定位逻辑：
//   - input：垂直居中、按钮型水平居中且不截断、password ● 掩码、
//     文本型溢出舍头保留尾；
//   - textarea：折行后的逐行文本（盒外整行不导出）；
//   - select：选中文本左对齐垂直居中（下拉箭头属装饰，不导出）。
//
// 这些控件没有文本子节点，若不在此补画，SVG 导出会缺所有控件文字
// （渲染路径在元素盒绘制时直接读 value 属性，两条路径必须保持同构）。
func dumpControlText(b *strings.Builder, base *htmlElement, comp CSSStyleDeclaration, bx, by, bw, bh int) {
	if bw <= 0 || bh <= 0 {
		return
	}
	fs := 13
	fill := "#000000"
	fam := ""
	if comp != nil {
		if s := resolveLen(comp.FontSize(), 0); s > 0 {
			fs = s
		}
		if c := comp.Color(); c != nil {
			r, g, bl, _ := c.RGBA()
			fill = hexColor(r, g, bl)
		}
		fam = comp.FontFamily()
	}
	// 度量与渲染路径一致走 Fake 估算口径（dump 无 Graphics 实例）
	fg := NewFakeGraphics()
	emitText := func(x, y int, text string) {
		fmt.Fprintf(b, `<text x="%d" y="%d" font-size="%d" font-family="%s" fill="%s">%s</text>`+"\n",
			x, y, fs, escapeXML(svgFontStack(fam)), fill, escapeXML(text))
	}
	// 选区高亮：与渲染路径同色同位置（fill-opacity 对应 selectionColor
	// 的 alpha 0x82/255）。SVG 是静态导出，仍导出当前选区状态，便于无头
	// 测试核对拖选/Ctrl+A 后的高亮几何。
	emitSelection := func(r clipRect) {
		// 颜色/透明度直接取渲染用的 selectionColor，避免两处各写一份常量。
		cr, cg, cb, ca := selectionColor.RGBA()
		fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" fill-opacity="%.2f"/>`+"\n",
			r.x, r.y, r.w, r.h, hexColor(cr, cg, cb), float64(ca)/255)
	}
	bold := comp.FontWeight() == FontWeightBold

	if strings.EqualFold(base.tagName, "select") {
		// 与渲染路径同口径：可用宽 = 内容宽 - 箭头位，超宽舍尾保留头。
		val := selectDisplayClipped(NewFakeGraphics(), base, base.width-selectArrowWidth,
			fs, comp.FontWeight() == FontWeightBold, fam)
		if val == "" {
			return
		}
		_, h := fakeMeasureText(val, fs)
		tx, ty := base.x, base.y
		if h < base.height {
			ty += (base.height - h) / 2
		}
		emitText(tx, ty+h, val)
		return
	}

	if strings.EqualFold(base.tagName, "textarea") {
		l := layoutTextarea(fg, base)
		// 与渲染路径同口径：有裁剪盒（overflow 非 visible）时只丢与盒子
		// 不相交的行，半截行交给 clip-path 截断；否则整行丢弃。
		clipOK := isClippingBox(base)
		if isFocusedElement(base) {
			for _, sr := range textareaSelectionRects(fg, base, l, fs, bold, fam, clipOK) {
				emitSelection(sr)
			}
		}
		for i, ln := range l.lines {
			y := l.y + i*l.lh
			if ln.text == "" {
				continue
			}
			if clipOK {
				if y+l.lh <= base.y || y >= base.y+base.height {
					continue
				}
			} else if y < base.y || y+l.lh > base.y+base.height {
				continue // 盒外整行不导出（渲染路径同样丢弃）
			}
			emitText(l.x, y+l.lh, ln.text)
		}
		return
	}

	if !strings.EqualFold(base.tagName, "input") {
		return
	}
	typ := strings.ToLower(base.GetAttribute("type"))
	if typ == "radio" || typ == "checkbox" {
		// 控件盒已由通用 rect 路径导出；选中时补画标记（与渲染路径同构）
		if hasChecked(base) {
			dumpCheckMark(b, comp, bx, by, bw, bh, typ)
		}
		return
	}
	val, cut, w, h := inputVisibleWindow(fg, base, fs, bold, fam)
	if val == "" {
		return
	}
	buttonLike := typ == "submit" || typ == "reset" || typ == "button"
	tx := base.x
	if buttonLike && base.width > w {
		tx = base.x + (base.width-w)/2
	}
	ty := base.y
	if h < base.height {
		ty += (base.height - h) / 2
	}
	if lo, hi, ok := selectionRange(base); ok && isFocusedElement(base) {
		for _, sr := range inputSelectionRects(fg, val, cut, tx, ty, h, fs, bold, fam, lo, hi) {
			emitSelection(sr)
		}
	}
	emitText(tx, ty+h, val)
}

// isFocusedElement 判断元素是否为当前聚焦控件。
//
// 渲染路径只给聚焦控件画选区高亮（render.go 的 focused 判定），SVG
// 导出必须同口径：失焦后引擎保留编辑状态（选区仍在，见
// SetFocusedElement），但屏幕上并不画高亮，静态导出也不该画。
func isFocusedElement(base *htmlElement) bool {
	if base == nil {
		return false
	}
	d, ok := docOf(base.self).(*htmlDocument)
	if !ok || d == nil {
		return false
	}
	f := inner(d.focus)
	return f != nil && f == base
}

// dumpCheckMark 导出 radio/checkbox 的选中标记（与 renderCheckedMark 同构：
// checkbox = 强调色圆角方块 + 白色对勾折线；radio = 强调色外圆环 + 实心内圆，
// 留白由环内缘与内圆半径之差自然形成）。
func dumpCheckMark(b *strings.Builder, comp CSSStyleDeclaration,
	bx, by, bw, bh int, typ string) {
	if bw <= 0 || bh <= 0 {
		return
	}
	acc := hexColor(37, 99, 235) // focusBlue / checkAccent
	if typ == "checkbox" {
		rxAttr := ""
		if r := resolveRadius(comp, bw, bh); r > 0 {
			rxAttr = fmt.Sprintf(` rx="%d"`, r)
		}
		fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"%s/>`+"\n",
			bx, by, bw, bh, acc, rxAttr)
		sx := float64(bw) / 13
		sy := float64(bh) / 13
		pt := func(p [2]float64) string {
			return fmt.Sprintf("%.1f %.1f", float64(bx)+p[0]*sx, float64(by)+p[1]*sy)
		}
		fmt.Fprintf(b, `<path d="M %s L %s L %s" fill="none" stroke="#ffffff" stroke-width="1"/>`+"\n",
			pt(checkPoints[0]), pt(checkPoints[1]), pt(checkPoints[2]))
		return
	}
	cx := float64(bx) + float64(bw)/2
	cy := float64(by) + float64(bh)/2
	rOut := math.Min(float64(bw), float64(bh)) / 2
	// 环：[0.69r, r] → 描边宽 0.31r，中心线 0.845r；内圆 0.54r（与 fillRadioMark 同口径）
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="%s" stroke-width="%.1f"/>`+"\n",
		cx, cy, rOut*0.845, acc, rOut*0.31)
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`+"\n",
		cx, cy, rOut*0.54, acc)
}

// fakeMeasureText 与 FakeGraphics.MeasureText 同口径的文本估算，
// 供无 Graphics 实例的导出路径使用（布局度量亦出自该口径，坐标自洽）。
func fakeMeasureText(text string, fontSize int) (w, h int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	for _, r := range text {
		if r > 0x2E80 {
			w += fontSize
		} else if r == ' ' {
			w += fontSize / 2
		} else {
			w += fontSize * 3 / 5
		}
	}
	h = fontSize * 13 / 10
	return
}

// svgFontStack 为导出的 SVG 文本补全字体栈，让浏览器预览与引擎实际落字一致：
// 未指定 family 时按引擎 standard（sans-serif → Arial）；
// 末尾始终附加 CJK per-script fallback（微软雅黑）。浏览器按"字形覆盖"逐字回退，
// 拉丁命中前者、汉字落到后者，与引擎的分段落字等价。缺省不附加时浏览器会用自己的
// 默认中文字体（常为宋体），预览观感就与引擎不符了。仅调试导出用途。
func svgFontStack(cssFamily string) string {
	cssFamily = strings.TrimSpace(cssFamily)
	if cssFamily == "" {
		cssFamily = "Arial" // 引擎 standard = sans-serif（Arial）
	}
	return cssFamily + ", 'Microsoft YaHei'"
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	// 属性值里的双引号也要转义：font-family 常带引号
	//（font-family="Microsoft YaHei", "Segoe UI"），不转义导出的就不是
	// 合法 XML，浏览器直接拒绝渲染。
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
