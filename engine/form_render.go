package engine

import "strings"

// 表单控件（input / textarea / select）的专用绘制。
// 通用路径已画完背景与边框，这里只负责控件内容与交互态。

// isFormControl 判断元素是否为需要专用绘制的表单控件。
func isFormControl(b *htmlElement) bool {
	if b == nil {
		return false
	}
	switch strings.ToLower(b.tagName) {
	case "input", "textarea", "select":
		return true
	}
	return false
}

// selectArrowWidth 是 select 右侧下拉箭头的预留宽度。绘制时箭头占内容盒
// 右缘这么多像素，auto 宽度也须预留同样的空间——否则显示文本会被自己的
// 箭头挤掉（此前 auto 宽按「文本 + 两个空格」估算，比 16px 少约 9px，
// 实测 auto 宽 select 把自己的值截成 "Alph"）。
const selectArrowWidth = 16

// selectDisplayClipped 按可用宽度截断 select 的显示文本（超宽舍尾保留头：
// 光标不在 select 尾部）。绘制与 SVG 导出共用，保证同一控件两处文本一致
// （此前导出全值、渲染截断，用 SVG 校验布局会误判）。
func selectDisplayClipped(g Graphics, b *htmlElement, avail, size int, bold bool, family string) string {
	val := selectDisplayText(b)
	runes := []rune(val)
	for len(runes) > 1 {
		tw, _ := g.MeasureText(val, size, bold, family)
		if tw <= avail {
			break
		}
		runes = runes[:len(runes)-1]
		val = string(runes)
	}
	return val
}

// renderSelectValue 绘制 select 收起态的选中文本与右侧下拉箭头。
// 文本超宽舍尾保留头（与 input 舍头保尾相反：光标不在 select 尾部）。
func renderSelectValue(g Graphics, b *htmlElement, comp CSSStyleDeclaration, p Paint, bx, by, bw, bh, r int) {
	val := selectDisplayClipped(g, b, b.width-selectArrowWidth, p.Size().Pixel(), p.Bold(), p.FontFamily())
	size := p.Size().Pixel()
	tw, th := g.MeasureText(val, size, p.Bold(), p.FontFamily())
	if val != "" {
		tx, ty := b.x, b.y
		if th < b.height {
			ty += (b.height - th) / 2
		}
		g.DrawText(tx, ty, tw, th, p, val)
	}
	// 下拉箭头：内容盒右缘内 4px 处的实心倒三角
	ac := comp.BorderColor()
	if ac == nil {
		ac = NewColor(0x76, 0x76, 0x76, 255)
	}
	cx := b.x + b.width - 4
	cy := b.y + (b.height-4)/2
	if cy < by {
		cy = by
	}
	if cy+4 > by+bh {
		cy = by + bh - 4
	}
	for i := 0; i < 4; i++ {
		g.DrawColor(cx-3+i, cy+i, 7-2*i, 1, ac)
	}
}

// renderTextareaValue 绘制 textarea 的折行文本与光标。
// 盒外的行不画；后端支持裁剪（Clipper）时半截行也画，交由 overflow 裁剪
// 截断（滚动时行随 scrollTop 平移，必然出现半截行），否则退化为整行丢弃，
// 保持"不画出盒外"的历史行为。
// 聚焦框不在此画：它位于边框盒，须早于 overflow 裁剪压栈（见 render.go）。
func renderTextareaValue(g Graphics, b *htmlElement, comp CSSStyleDeclaration, p Paint,
	focused bool) {
	px, bold, family := fontOf(comp)
	l := layoutTextarea(g, b)
	_, clipOK := g.(Clipper)
	dec := TextDecorationNone
	if comp != nil {
		dec = comp.TextDecoration()
	}
	for i, ln := range l.lines {
		y := l.y + i*l.lh
		if ln.text == "" {
			continue
		}
		if clipOK {
			if y+l.lh <= b.y || y >= b.y+b.height {
				continue // 与盒子完全不相交才丢弃
			}
		} else if y < b.y || y+l.lh > b.y+b.height {
			continue // 无裁剪原语：盒外整行丢弃，避免溢出到相邻内容
		}
		g.DrawText(l.x, y, ln.w, l.lh, p, ln.text)
		if dec != TextDecorationNone {
			drawDecoration(g, dec, l.x, y, ln.w, l.lh, px, p)
		}
	}
	if !focused {
		return
	}
	// 光标：1px 竖线（文本色），与盒子相交即绘制（裁剪后端会截断）
	cx, cy := l.caretXY(g, b.caret, px, bold, family)
	visible := cx >= b.x && cx <= b.x+b.width &&
		cy >= b.y && cy+l.lh <= b.y+b.height
	if clipOK {
		visible = cx >= b.x && cx <= b.x+b.width &&
			cy+l.lh > b.y && cy < b.y+b.height
	}
	if visible {
		cc := p.Color()
		if cc == nil {
			cc = NewColor(0, 0, 0, 255)
		}
		g.DrawColor(cx, cy, 1, maxInt(l.lh-2, 1), cc)
	}
	// 聚焦框不在此画：它位于边框盒，须早于 overflow 裁剪压栈（见 render.go）。
}

// renderSelectPopup 绘制展开的选项浮层：容器底与边框 → 选中项高亮 →
// 各 option 自身（背景/边框/文字，复用 renderNode；选中项文字改白）。
// 必须画在常规层与定位层之后，才能盖住其下的流内容。
func renderSelectPopup(g Graphics, sel *htmlElement, focus HTMLElement) {
	opts := selectOptions(sel.self)
	if len(opts) == 0 {
		return
	}
	x0, y0, x1, y1, ok := selectPopupBounds(sel)
	if !ok || x1 <= x0 || y1 <= y0 {
		return
	}
	w, h := x1-x0, y1-y0
	g.DrawColor(x0, y0, w, h, NewColor(0xFF, 0xFF, 0xFF, 0xFF))
	s := selectedOptionBase(sel)
	if s != nil {
		if sx, sy, sw, sh := borderBoxRect(s); sw > 0 && sh > 0 {
			g.DrawColor(sx, sy, sw, sh, selectHighlight)
		}
	}
	// 边框压在高亮之上：高亮项的边框盒与容器四边重合，先画边框会被覆盖
	bc := NewColor(0x76, 0x76, 0x76, 0xFF)
	if sel.computed != nil {
		if c := sel.computed.BorderColor(); c != nil {
			bc = c
		}
	}
	strokeRoundedRect(g, x0, y0, w, h, 0, edges{1, 1, 1, 1}, bc)
	for _, o := range opts {
		if o == s {
			// 蓝底随种子 Paint 下传：option 自身无背景色，而 LCD 后端按
			// paint.Background() 与落点底色合成、nil 时按白底——白字配
			// 白底合成会渲成实心白块（字看不清）。这里把高亮底作为
			// 沿父链的最近实底交给后端，白字才能压在蓝底上出正确字形。
			seed := NewPaint()
			seed.SetBackground(selectHighlight)
			renderNodeTextColor(g, o, seed, focus, selectHighlightText)
		} else {
			renderNode(g, o, nil, focus)
		}
	}
}

// selectHighlight / selectHighlightText 是展开浮层选中项的高亮底色与文字色。
// Chrome 实测（本机 Chrome 展开 <select>）：选中行 #1967D2 实心蓝 + 白字，
// 未选中行白底黑字——旧值 #CFE2FF 浅蓝黑字与 Chrome 不符，已按实测修正。
var (
	selectHighlight     = NewColor(0x19, 0x67, 0xD2, 255)
	selectHighlightText = NewColor(0xFF, 0xFF, 0xFF, 255)
)
