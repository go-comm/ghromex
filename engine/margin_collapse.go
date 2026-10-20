package engine

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
