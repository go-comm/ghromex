package engine

import (
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// margin 折叠（CSS 2.1 §8.3.1）
//
// 引擎已实现"相邻兄弟折叠"（见 measure.go 的 prevBlockMB 逻辑），本文件补
// "父-子穿越折叠"：父块无上/下边框与内边距时，父块的外边距与首/末个流内
// 块级子块的外边距折叠（取大者），子块自身不再重复计入。
//
// 影响：<form> 内首个 <p>（margin:16px 0）此前会与前一个 <p> 的下边距
// 叠加成 32px（Chrome 折叠为 16px）——同一页面对照下整块内容下移 16px，
// 表单首行与浏览器明显错位。
//
// v1 简化：只处理"首/末个流内块级子块"这一层；子块自身又吸收其子块外边距
// 的多层折叠不参与（浏览器按层传递），空块自折叠同样未实现。
// ---------------------------------------------------------------------------

// listMarkerGap 返回 outside 标记右缘与内容左缘之间的间隙。
//
// Chrome 实测两类标记的间隙不同：符号类（圆点/圆圈/方块）约 0.875em
// （14px@16px），编号类（序号/字母/罗马数字）约 0.6em（9.6px@16px）——
// 用同一个间隙会让其中一类明显偏左。
func listMarkerGap(fpx int, t ListStyleType) int {
	if isBulletListStyle(t) {
		return fpx * 7 / 8
	}
	return fpx * 3 / 5
}

// listMarkerInsideGap 返回 inside 标记与首行文字之间的间隙（0.5em）。
func listMarkerInsideGap(fpx int) int {
	return fpx / 2
}

// listMarkerBox 计算列表标记的绘制盒。
//
// outside（默认）：右对齐到内容左缘 − 间隙，垂直与内容**首行**对齐（不是
// 整个 li 盒居中——li 内含嵌套列表时，居中会把标记画到盒子中部，视觉上
// 多出一行"孤立圆点"）。
//
// inside：标记落在内容盒内首行之前（左缘 = 内容左缘），首行文字由布局期
// 整体右移标记宽 + 间隙（见 layoutChildren）。返回 ok=false 表示该元素
// 无标记。
func listMarkerBox(g Graphics, base *htmlElement, comp CSSStyleDeclaration) (x, y, w, h int, ok bool) {
	if base == nil || base.listMarker == "" {
		return 0, 0, 0, 0, false
	}
	fpx, fbold, ffam := fontOf(comp)
	mw, _ := g.MeasureText(base.listMarker, fpx, fbold, ffam)
	_, lh := g.MeasureText("Mg", fpx, fbold, ffam) // 行高（与文字行盒一致）
	if lh <= 0 {
		lh = base.height
	}
	if base.listMarkerInside {
		return base.x, base.y, mw, lh, true
	}
	gap := listMarkerGap(fpx, base.listMarkerKind)
	return base.x - gap - mw, base.y, mw, lh, true
}

// listItemStartsWithInline 判断 li 的首个内容子节点是否属于行内内容
// （文本或行内级盒）。inside 标记只在这种情形下缩进首行：首个内容子节点
// 是块级时，Chrome 会把该块的首行整体右移，引擎不做这层传播——标记退回
// outside 绘制，避免压住块内容。空 li 返回 true（标记仍画在内容盒内）。
func listItemStartsWithInline(el *htmlElement) bool {
	for _, c := range el.children {
		if _, ok := c.(*textNode); ok {
			return true
		}
		cb := inner(c)
		if cb == nil {
			continue
		}
		if cb.computed == nil {
			return true
		}
		d := cb.computed.Display()
		if d == DisplayNone {
			continue
		}
		return d != DisplayBlock && d != DisplayTable
	}
	return true
}

// listMarkerFor 返回列表项的标记文本与标记类型。
//
// 类型取 li 自身的计算值：list-style-type 从 ul/ol 继承，UA 样式表把
// ul 置为 disc、ol 置为 decimal；作者写在 ul/ol/li 上的声明都会生效。
// 编号类标记的序号按同级 li 的次序（CSS 的 list-item 计数器语义）。
// 父级不是 ul/ol 或类型为 none 时返回空标记。
func listMarkerFor(parent *htmlElement, child *htmlElement) (string, ListStyleType) {
	if parent == nil || child == nil || child.computed == nil {
		return "", ListStyleDisc
	}
	tag := strings.ToLower(parent.tagName)
	if tag != "ul" && tag != "ol" {
		return "", ListStyleDisc
	}
	t := child.computed.ListStyleType()
	if t == ListStyleNone {
		return "", t
	}
	if isBulletListStyle(t) {
		return listMarkerText(t, 0), t
	}
	n := 0
	for _, c := range parent.children {
		cb := inner(c)
		if cb == nil {
			continue
		}
		if strings.EqualFold(cb.tagName, "li") {
			n++
			if cb == child {
				return listMarkerText(t, n), t
			}
		}
	}
	return "", t
}

// listMarkerText 按标记类型生成第 n 项（1 起）的标记文本。符号类忽略 n。
func listMarkerText(t ListStyleType, n int) string {
	switch t {
	case ListStyleDisc:
		return "•"
	case ListStyleCircle:
		return "◦"
	case ListStyleSquare:
		return "▪"
	case ListStyleNone:
		return ""
	case ListStyleDecimal:
		return strconv.Itoa(n) + "."
	case ListStyleDecimalLeadingZero:
		if n >= 0 && n < 10 {
			return "0" + strconv.Itoa(n) + "."
		}
		return strconv.Itoa(n) + "."
	case ListStyleLowerAlpha:
		return alphaListMarker(n, false) + "."
	case ListStyleUpperAlpha:
		return alphaListMarker(n, true) + "."
	case ListStyleLowerRoman:
		return romanListMarker(n, false) + "."
	case ListStyleUpperRoman:
		return romanListMarker(n, true) + "."
	}
	return strconv.Itoa(n) + "."
}

// alphaListMarker 把序号转成字母标记（双射 26 进制）：1→a、26→z、
// 27→aa、52→az、53→ba（与 Chrome 的 lower-alpha 一致）。
func alphaListMarker(n int, upper bool) string {
	if n <= 0 {
		return ""
	}
	base := byte('a')
	if upper {
		base = 'A'
	}
	var buf []byte
	for n > 0 {
		n--
		buf = append(buf, base+byte(n%26))
		n /= 26
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// romanListMarker 把序号转成罗马数字：1→i、4→iv、9→ix、14→xiv。
// 超出 1~3999 时回退十进制（CSS 未规定超范围的表示，Chrome 用十进制）。
func romanListMarker(n int, upper bool) string {
	if n <= 0 || n >= 4000 {
		return strconv.Itoa(n)
	}
	values := [...]int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	symbols := [...]string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range values {
		for n >= v {
			b.WriteString(symbols[i])
			n -= v
		}
	}
	s := b.String()
	if !upper {
		s = strings.ToLower(s)
	}
	return s
}

// flowBlockChildren 返回元素内可参与折叠的流内块级子块（按文档顺序）。
// 跳过 display:none 与脱流盒（absolute/fixed）；遇到行内内容（行内盒或
// 含文本的行盒）即停止——行内内容会隔开块级上下文，折叠不成立。
func flowBlockChildren(el *htmlElement) []*htmlElement {
	if el == nil {
		return nil
	}
	var out []*htmlElement
	for _, child := range el.children {
		if tn, ok := child.(*textNode); ok {
			// 纯空白文本不阻断折叠；含实际内容则阻断
			if hasVisibleText(tn.Text()) {
				break
			}
			continue
		}
		cb := inner(child)
		if cb == nil || cb.computed == nil {
			continue
		}
		if cb.computed.Display() == DisplayNone {
			continue
		}
		if cb.computed.Position().isOutFlow() {
			continue // 脱流盒不占位，不阻断也不参与折叠
		}
		if cb.computed.Display() != DisplayBlock {
			break // 行内盒隔开块级上下文
		}
		out = append(out, cb)
	}
	return out
}

// hasVisibleText 报告文本是否含非空白字符。
func hasVisibleText(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', ' ':
			continue
		}
		return true
	}
	return false
}

// firstFlowBlockChild 返回首个可参与折叠的流内块级子块。
func firstFlowBlockChild(el *htmlElement) *htmlElement {
	cs := flowBlockChildren(el)
	if len(cs) == 0 {
		return nil
	}
	return cs[0]
}

// lastFlowBlockChild 返回末个可参与折叠的流内块级子块。
func lastFlowBlockChild(el *htmlElement) *htmlElement {
	cs := flowBlockChildren(el)
	if len(cs) == 0 {
		return nil
	}
	return cs[len(cs)-1]
}

// collapsesWithFirstChild 报告父块的上外边距是否与首个子块的上外边距折叠：
// 父块自身须是块级容器，且无上边框、无上内边距（CSS 2.1 §8.3.1）。
func collapsesWithFirstChild(el *htmlElement, bd, pd edges) bool {
	if el == nil || el.computed == nil {
		return false
	}
	if el.computed.Display() != DisplayBlock {
		return false
	}
	return bd[eTop] == 0 && pd[eTop] == 0
}

// collapsesWithLastChild 同理（下方：下边框/下内边距）。
func collapsesWithLastChild(el *htmlElement, bd, pd edges) bool {
	if el == nil || el.computed == nil {
		return false
	}
	if el.computed.Display() != DisplayBlock {
		return false
	}
	return bd[eBottom] == 0 && pd[eBottom] == 0
}
