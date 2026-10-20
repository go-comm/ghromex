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

// listMarkerBox 计算列表标记的绘制盒：右对齐到内容左缘 −0.75em，垂直与内容
// **首行**对齐（不是整个 li 盒居中——li 内含嵌套列表时，居中会把标记画到
// 盒子中部，视觉上多出一行"孤立圆点"）。返回 ok=false 表示该元素无标记。
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
	// 标记盒右缘 = 内容左缘 − 间隙。Chrome 实测两类标记的间隙不同：disc 圆点
	// 约 0.875em（14px@16px），decimal 序号约 0.6em（9.6px@16px）——用同一个
	// 间隙会让其中一类明显偏左。
	gap := fpx * 3 / 5
	if base.listMarker == "•" {
		gap = fpx * 7 / 8
	}
	return base.x - gap - mw, base.y, mw, lh, true
}

// listMarkerFor 返回列表项的标记文本：ul 用 disc（"•"），ol 用 decimal
// （"1." "2."…，序号按同级 li 的次序）；父级不是列表则返回 ""。
// 引擎没有 list-item 概念，标记在布局期算出、绘制期由 render.go 画出。
func listMarkerFor(parent *htmlElement, child *htmlElement) string {
	if parent == nil || child == nil {
		return ""
	}
	switch strings.ToLower(parent.tagName) {
	case "ul":
		return "•"
	case "ol":
		n := 0
		for _, c := range parent.children {
			cb := inner(c)
			if cb == nil {
				continue
			}
			if strings.EqualFold(cb.tagName, "li") {
				n++
				if cb == child {
					return strconv.Itoa(n) + "."
				}
			}
		}
	}
	return ""
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
