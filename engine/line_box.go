package engine

// 行盒（line box）的两件事：line-height 决定行盒多高、基线落在哪；
// vertical-align 决定行内盒相对基线摆在哪。两者共用同一套「基线上下
// 高度」口径，都在 flushLine 里汇合，故放在一个文件里。

// resolvedLineHeight 返回元素的 line-height 像素值；-1 表示 normal
// （行高取字体度量）。
//
// 倍数与百分比都相对**元素自身**的字号解析（CSS 2.1 §10.8.1），
// 故此处用元素自己的 font-size，而不是继承链上的任何值——继承下来的
// 是倍数本身（父级 1.5 + 子级 font-size:32px → 子级行高 48px）。
func resolvedLineHeight(comp CSSStyleDeclaration) int {
	if comp == nil {
		return -1
	}
	switch comp.LineHeight().Kind() {
	case LineHeightFactor:
		fs := resolveLen(comp.FontSize(), 0)
		if fs <= 0 {
			fs = 16
		}
		return int(float64(fs)*float64(comp.LineHeight().Factor()) + 0.5)
	case LineHeightLength:
		return int(comp.LineHeight().Length() + 0.5)
	default: // normal
		return -1
	}
}

// applyLeading 按 line-height 给定的行高重算基线以上/以下的高度。
//
// CSS 2.1 §10.8.1：行距（leading）= line-height − (ascent + descent)，
// 一半加在文字之上、一半加在之下。half-leading 取 floor(leading/2 + 0.5)
// ——leading 为奇数时向上取整，与 Chrome 的基线落点一致（Chrome 的基线
// 此时在 .5 像素处，绘制时向上取整）。
//
// ascent = A + half；descent = L − ascent（而不是 D + half）：这样两者之和
// 恰为 L，行盒高度严格等于声明的 line-height（Chrome 同口径）。
//
// L < A + D（行高小于字体自然行高）时 descent 为负——文字溢出行盒，
// Chrome 同样允许。lh < 0（normal）时原样返回。
func applyLeading(asc, desc, lh int) (int, int) {
	if lh < 0 {
		return asc, desc
	}
	leading := lh - (asc + desc)
	half := floorDiv(leading+1, 2)
	a := asc + half
	return a, lh - a
}

// floorDiv 是向下取整的整数除法（Go 的 / 是向零取整，负数上差 1）。
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// ownLineHeight 返回元素自身的行高像素值：声明的 line-height 解析结果；
// normal 时取该元素字体的自然行高（ascent + descent）——vertical-align 的
// 百分比以此为基数（CSS Inline 3：百分比相对元素自身的 line-height）。
func ownLineHeight(g Graphics, comp CSSStyleDeclaration) int {
	if lh := resolvedLineHeight(comp); lh >= 0 {
		return lh
	}
	if g == nil || comp == nil {
		return 0
	}
	px, bold, family := fontOf(comp)
	a, d := g.FontMetrics("", px, bold, family)
	return a + d
}

// verticalAlignShift 计算原子盒相对基线的位移（正数=向上抬高）。
//
// 除 top/bottom（依赖行盒的最终 ascent/descent，在 flushLine 里定）外，
// 其余取值的位移只依赖父级内容区与盒自身度量：
//   - super/sub：按**父级字号**位移（Chrome 的 <sup>/<sub> 同口径：
//     父字号/3+1 与 父字号/5+1，与字体无关）。
//   - middle：盒的垂直中点与「父级基线 + 半个 x 高度」对齐。
//   - text-top/text-bottom：与父级**内容区**对齐——内容区是字体的
//     ascent/descent，不含 line-height 的 Leading（CSS 2.1 §10.1）。
//   - raise：长度/百分比，相对基线上移指定距离。
func verticalAlignShift(mode VerticalAlign, a, d, contentAsc, contentDesc, fontPx, raise int) int {
	switch mode {
	case VerticalAlignSuper:
		return fontPx/3 + 1
	case VerticalAlignSub:
		return -(fontPx / 5 + 1)
	case VerticalAlignMiddle:
		return xHeightOf(fontPx)/2 - (a-d)/2
	case VerticalAlignTextTop:
		return contentAsc - a
	case VerticalAlignTextBottom:
		return d - contentDesc
	case VerticalAlignRaise:
		return raise
	default: // baseline
		return 0
	}
}

// xHeightOf 估算字体的 x 高度（小写字母 x 的高度）。
//
// SDL_ttf 没有暴露 x 高度（FreeType 的 face->size->metrics.x_height 没有
// 对外接口），只能按字号比例估。0.52 是常见西文字体的 x 高度/字号比
// （Arial 实测 0.519、Times 0.448、Segoe UI 0.53）；vertical-align:middle
// 只用它算「半个 x 高度」的偏移，差 1px 不影响观感。
func xHeightOf(fontPx int) int {
	return fontPx * 52 / 100
}

// verticalAlignRaise 解析 vertical-align 的长度/百分比抬正值。
// 百分比相对元素**自身**的 line-height（CSS Inline 3）。
func verticalAlignRaise(comp CSSStyleDeclaration, lhPx int) int {
	if comp == nil {
		return 0
	}
	off := comp.VerticalAlignOffset()
	if off == nil {
		return 0
	}
	switch off.Type() {
	case SIZE_PIXEL:
		return off.Pixel()
	case SIZE_PERCENT:
		base := lhPx
		if base < 0 {
			base = 0
		}
		return int(float64(base)*float64(off.Percent())/100 + 0.5)
	}
	return 0
}
