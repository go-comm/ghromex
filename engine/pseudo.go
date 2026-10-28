package engine

import "strings"

// 伪类（pseudo-class）支持：选择器里的 :hover / :focus 等动态状态，
// 以及 :first-child / :nth-child() 等结构伪类、:not() 否定伪类。
//
// 动态伪类的状态存在文档上（hover/active/focus），状态变化按**变更档位**
// 处理（见 applyStateChange / change_level.go）：可能什么都不做、只重算
// 受影响子树的样式 + 重绘、或（规则改了盒模型时）升级为全量重排——与浏览器
// 「改状态 → 重算样式 → 视需要重排」的链路一致。结构伪类只看 DOM 形状，
// 随级联一起算。
//
// 函数型伪类（:not() / :nth-child()）在 ParseSelector 期即把参数预解析
// 为结构化形态（:not 的选择器列表、:nth-child 的 An+B），匹配期零解析
// 开销——级联要对每个元素×每条规则跑匹配，参数解析不能每次都重来。
// 参数非法（:not() 空参、:nth-child(2x)）与未识别的伪类/函数型伪类
// （:is() 等）一律不匹配——CSS 语义是无法识别的选择器使整条规则失效，
// 效果等同「永不匹配」。

// 伪类名（选择器里 ':' 之后的部分，小写）。
const (
	pseudoHover      = "hover"
	pseudoFocus      = "focus"
	pseudoActive     = "active"
	pseudoChecked    = "checked"
	pseudoDisabled   = "disabled"
	pseudoEnabled    = "enabled"
	pseudoFirstChild = "first-child"
	pseudoLastChild  = "last-child"
	pseudoOnlyChild  = "only-child"
	pseudoRoot       = "root"
	pseudoEmpty      = "empty"
	// 函数型伪类（带括号参数）：参数在 ParseSelector 期预解析（selector.go
	// newPseudoClass），匹配期只用结构化结果。
	pseudoNot      = "not"
	pseudoNthChild = "nth-child"
)

// HoveredElement 返回当前被鼠标悬停的元素（无则 nil）。
func HoveredElement(doc HTMLDocument) HTMLElement {
	if d, ok := doc.(*htmlDocument); ok {
		return d.hover
	}
	return nil
}

// SetHoveredElement 设置悬停元素（nil 表示鼠标不在文档上）。
//
// 悬停态变化会改样式（:hover 规则），但**不一定是重排**——按变更档位分级
// 处理（见 applyStateChange / change_level.go）：
//
//   - 页面没有 :hover 规则、悬停目标也不是表单控件 → 什么都不变：不标记、
//     不通知（与下一段「没变就不通知」同口径）；
//   - 规则只改颜色/背景这类纯绘制属性 → 就地重算受影响子树、记重绘账，
//     主循环只重绘不布局（42 万元素页面从 1.72 秒降到零重排）；
//   - 规则可能改盒模型（padding/width/font-…）→ 照旧 markChanged 全量重排。
//
// 悬停元素**没变**时不通知：鼠标移动是最频繁的事件，每次都重排会把
// 主循环拖垮（浏览器同样只在悬停目标变化时重算样式）。
func SetHoveredElement(doc HTMLDocument, el HTMLElement) {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return
	}
	if sameElement(d.hover, el) {
		return
	}
	old := d.hover
	d.hover = el
	// :hover 沿父链继承：链上任何一代的命中都可能翻转，新旧两条链都算候选。
	d.applyStateChange(stateHover, append(stateChain(old), stateChain(el)...))
}

// ActiveElement 返回当前被按下的元素（鼠标按下到抬起之间）。
func ActiveElement(doc HTMLDocument) HTMLElement {
	if d, ok := doc.(*htmlDocument); ok {
		return d.active
	}
	return nil
}

// SetActiveElement 设置按下态元素（nil 表示没有按下）。
// 状态变化按档位分级处理（同 SetHoveredElement：:active 规则全为纯绘制
// 属性时只重绘不重排）。
func SetActiveElement(doc HTMLDocument, el HTMLElement) {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return
	}
	if sameElement(d.active, el) {
		return
	}
	old := d.active
	d.active = el
	d.applyStateChange(stateActive, append(stateChain(old), stateChain(el)...))
}

// stateChain 返回元素自身及其全部祖先（自底向上）。
//
// :hover/:active 沿父链继承（matchHoverState），状态变化可能让链上**任何
// 一代**的选择器命中翻转，链上元素连同其子树（继承属性的传播范围）因此
// 都是级联候选（见 applyStateChange）。nil 返回空切片。
func stateChain(el HTMLElement) []HTMLElement {
	var out []HTMLElement
	for cur := el; cur != nil; cur = cur.ParentElement() {
		out = append(out, cur)
	}
	return out
}

// OnDocumentMouseMove 按坐标更新悬停态：命中测试后把命中的元素记为
// 悬停元素（命中空白处则清除）。返回是否命中了元素。
//
// 应用层在每次鼠标移动时调用它，:hover 样式因此跟手。
func OnDocumentMouseMove(doc HTMLDocument, x, y int) bool {
	target := ElementAt(doc, x, y)
	SetHoveredElement(doc, target)
	if d, ok := doc.(*htmlDocument); ok {
		d.trackTooltip(x, y) // 提示气泡的锚点跟着鼠标走
	}
	return target != nil
}

// matchPseudo 判断元素是否匹配某个伪类。函数型伪类（:not/:nth-child）用
// 解析期预好的结构化参数；bad=true（参数非法）按 CSS「整条选择器无效」
// 的语义永不匹配。
func matchPseudo(doc HTMLDocument, pc pseudoClass, e HTMLElement) bool {
	if pc.bad {
		return false
	}
	switch pc.name {
	case pseudoHover:
		return matchHoverState(doc, e)
	case pseudoFocus:
		return matchFocusState(doc, e)
	case pseudoActive:
		return matchActiveState(doc, e)
	case pseudoChecked:
		b := inner(e)
		return b != nil && hasChecked(b)
	case pseudoDisabled:
		return hasDisabledAttr(e)
	case pseudoEnabled:
		return isFormControlTag(e) && !hasDisabledAttr(e)
	case pseudoFirstChild:
		return isFirstElementChild(e)
	case pseudoLastChild:
		return isLastElementChild(e)
	case pseudoOnlyChild:
		return isFirstElementChild(e) && isLastElementChild(e)
	case pseudoRoot:
		return isRootElement(e)
	case pseudoEmpty:
		return isEmptyElement(e)
	case pseudoNot:
		// 参数列表里任何一个选择器命中即不匹配 :not（全不命中才算命中）。
		for _, sel := range pc.sels {
			if matchSelectorChain(doc, sel, e) {
				return false
			}
		}
		return true
	case pseudoNthChild:
		idx := elementChildIndex(e)
		return idx > 0 && nthMatch(pc.nthA, pc.nthB, idx)
	}
	return false
}

// matchHoverState 判断元素是否处于悬停态。
//
// 悬停态沿父链继承：鼠标停在子元素上时，祖先元素同样匹配 :hover
// （Chrome 同款——:hover 规则常写在父级上，如 "li:hover .title"）。
func matchHoverState(doc HTMLDocument, e HTMLElement) bool {
	d, ok := doc.(*htmlDocument)
	if !ok || d.hover == nil {
		return false
	}
	return isTargetOrAncestor(e, d.hover)
}

// matchActiveState 判断元素是否处于按下态（沿父链继承，同 :hover）。
func matchActiveState(doc HTMLDocument, e HTMLElement) bool {
	d, ok := doc.(*htmlDocument)
	if !ok || d.active == nil {
		return false
	}
	return isTargetOrAncestor(e, d.active)
}

// matchFocusState 判断元素是否有焦点。焦点**不**沿父链继承
// （:focus-within 才是那个语义，引擎未实现）。
func matchFocusState(doc HTMLDocument, e HTMLElement) bool {
	d, ok := doc.(*htmlDocument)
	if !ok || d.focus == nil {
		return false
	}
	return sameElement(d.focus, e)
}

// isTargetOrAncestor 判断 e 是否是 target 本身或 target 的**祖先**。
//
// 用于 :hover/:active 的父链继承：鼠标停在 target 上时，target 的祖先
// 同样处于悬停/按下态（Chrome 同款——:hover 规则常写在父级上）。
func isTargetOrAncestor(e, target HTMLElement) bool {
	eb := inner(e)
	if eb == nil {
		return false
	}
	for cur := target; cur != nil; cur = cur.ParentElement() {
		if inner(cur) == eb {
			return true
		}
	}
	return false
}

// sameElement 比较两个元素是否同一个（按底层元素比，忽略包装器差异）。
func sameElement(a, b HTMLElement) bool {
	ia, ib := inner(a), inner(b)
	if ia == nil || ib == nil {
		return ia == nil && ib == nil
	}
	return ia == ib
}

// isFirstElementChild 判断元素是否是父元素的第一个**元素**子节点
// （文本节点不算兄弟——CSS 的 :first-child 只数元素）。
func isFirstElementChild(e HTMLElement) bool {
	p := e.ParentElement()
	if p == nil {
		return false
	}
	first := firstElementChild(p)
	return first != nil && sameElement(first, e)
}

// isLastElementChild 判断元素是否是父元素的最后一个元素子节点。
func isLastElementChild(e HTMLElement) bool {
	p := e.ParentElement()
	if p == nil {
		return false
	}
	last := lastElementChild(p)
	return last != nil && sameElement(last, e)
}

// firstElementChild 返回父元素的第一个元素子节点（跳过文本节点）。
func firstElementChild(p HTMLElement) HTMLElement {
	for _, c := range p.Children() {
		if _, ok := c.(*textNode); ok {
			continue
		}
		return c
	}
	return nil
}

// lastElementChild 返回父元素的最后一个元素子节点（跳过文本节点）。
func lastElementChild(p HTMLElement) HTMLElement {
	children := p.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if _, ok := children[i].(*textNode); ok {
			continue
		}
		return children[i]
	}
	return nil
}

// isRootElement 判断元素是否是文档根元素（html）。
func isRootElement(e HTMLElement) bool {
	d := docOf(e)
	return d != nil && sameElement(d, e)
}

// isEmptyElement 判断元素是否没有子节点。CSS Selectors 3：文本节点
// 也算子节点，空白文本同样使元素不为空（Selectors 4 才改口径）。
func isEmptyElement(e HTMLElement) bool {
	for _, c := range e.Children() {
		if tn, ok := c.(*textNode); ok {
			if tn.Text() != "" {
				return false
			}
			continue
		}
		return false
	}
	return true
}

// hasDisabledAttr 判断元素带 disabled 属性。布尔属性按**存在性**判定
// （与 checked 同口径）：`<input disabled>` 的属性值为空也算禁用。
func hasDisabledAttr(e HTMLElement) bool {
	b := inner(e)
	if b == nil {
		return false
	}
	_, ok := b.attrs["disabled"]
	return ok
}

// isFormControlTag 判断是否是表单控件标签（:enabled/:disabled 只对表单
// 控件有意义——div 既不匹配 :enabled 也不匹配 :disabled）。
func isFormControlTag(e HTMLElement) bool {
	if e == nil {
		return false
	}
	switch strings.ToLower(e.TagName()) {
	case "input", "button", "select", "textarea", "option", "optgroup", "fieldset":
		return true
	}
	return false
}

// elementChildIndex 返回元素在父元素的元素子节点中的 1-based 序号
//（:nth-child 只数元素，文本节点不算兄弟——同 :first-child 口径）。
// 返回 0 表示该元素未挂在父节点上（永不匹配）。
func elementChildIndex(e HTMLElement) int {
	p := e.ParentElement()
	if p == nil {
		return 0
	}
	idx := 0
	for _, c := range p.Children() {
		if _, ok := c.(*textNode); ok {
			continue
		}
		idx++
		if sameElement(c, e) {
			return idx
		}
	}
	return 0
}

// nthMatch 测试序号 idx 是否满足 An+B（∃ 非负整数 n 使 a·n+b == idx）。
// a=0 时仅 b 生效（b<=0 永不匹配）。
func nthMatch(a, b, idx int) bool {
	k := idx - b
	if a == 0 {
		return k == 0
	}
	if k%a != 0 {
		return false
	}
	return k/a >= 0
}
