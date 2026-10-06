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
	// 祖先的 overflow 裁剪按父链补上（与渲染路径 pushAncestorClips 同构）。
	for _, pb := range collectPositioned(document) {
		dumpSVGNode(&b, pb.e, ancestorSVGClips(pb.e, &ctr), &ctr)
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
	pd := resolveEdgeRect(base.computed.Padding(), 0, 0)
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
			fmt.Fprintf(b, `<text x="%d" y="%d" font-size="%d"%s%s%s fill="%s">%s</text>`+"\n",
				run.x, run.y+run.h, fs, ff, fw, decAttr, fill, escapeXML(run.text))
		}
		return
	}

	if comp != nil {
		bd := resolveEdgeRect(comp.BorderStyleWidth(), 0, 0)
		pd := resolveEdgeRect(comp.Padding(), 0, 0)
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
		if bg := comp.BackgroundColor(); bg != nil && bw > 0 && bh > 0 {
			r, g, bl, al := bg.RGBA()
			if al > 0 {
				fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" fill-opacity="%.2f"%s/>`+"\n",
					bx, by, bw, bh, hexColor(r, g, bl), float64(al)/255, rxAttr)
			}
		}
		if bc := comp.BorderColor(); bc != nil && comp.BorderStyle() != BorderStyleNone && bw > 0 && bh > 0 {
			lw := bd[eLeft]
			if lw < bd[eTop] {
				lw = bd[eTop]
			}
			if lw > 0 {
				r, g, bl, _ := bc.RGBA()
				srx := ""
				if br > 0 { // 描边路径中心线内缩 lw/2，rx 相应修正
					srx = fmt.Sprintf(` rx="%d"`, maxZero(br-lw/2))
				}
				fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="%s" stroke-width="%d"%s/>`+"\n",
					bx+lw/2, by+lw/2, bw-lw, bh-lw, hexColor(r, g, bl), lw, srx)
			}
		}
	}

	// overflow 非 visible：控件文本与子树包进 clip-path（与渲染路径同构）。
	// 边框在裁剪外，已在上方输出；clipPath 定义就地内联（不渲染）。
	clipped := isClippingBox(base)
	if clipped {
		clips = openSVGClip(b, clips, makeSVGClip(base, ctr))
	}

	if comp != nil && isFormControl(base) {
		bd := resolveEdgeRect(comp.BorderStyleWidth(), 0, 0)
		pd := resolveEdgeRect(comp.Padding(), 0, 0)
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
	emitText := func(x, y int, text string) {
		fmt.Fprintf(b, `<text x="%d" y="%d" font-size="%d" font-family="%s" fill="%s">%s</text>`+"\n",
			x, y, fs, escapeXML(svgFontStack(fam)), fill, escapeXML(text))
	}

	if strings.EqualFold(base.tagName, "select") {
		val := selectDisplayText(base)
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
		l := layoutTextarea(NewFakeGraphics(), base)
		// 与渲染路径同口径：有裁剪盒（overflow 非 visible）时只丢与盒子
		// 不相交的行，半截行交给 clip-path 截断；否则整行丢弃。
		clipOK := isClippingBox(base)
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
	val := displayValue(base) // password 在此按 ● 掩码，与渲染路径同口径
	if val == "" {
		return
	}
	w, h := fakeMeasureText(val, fs)
	runes := []rune(val)
	buttonLike := typ == "submit" || typ == "reset" || typ == "button"
	for !buttonLike && w > base.width-4 && len(runes) > 1 {
		runes = runes[1:]
		val = string(runes)
		w, h = fakeMeasureText(val, fs)
	}
	tx := base.x
	if buttonLike && base.width > w {
		tx = base.x + (base.width-w)/2
	}
	ty := base.y
	if h < base.height {
		ty += (base.height - h) / 2
	}
	emitText(tx, ty+h, val)
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
	return s
}
