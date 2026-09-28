package engine

import "sort"

// resolveStylesTree 为整棵树计算级联后的最终样式（写入 base.computed）。
// 优先级：UA 样式表 < 文档样式表（特异度+声明顺序） < 内联 style 属性；
// color/font-size/font-family/font-weight/text-align 沿父链继承。
func resolveStylesTree(e HTMLElement, parentBase *htmlElement, author *Stylesheet) {
	base := inner(e)
	if base == nil {
		return
	}
	comp := newStyle()

	// 继承性默认值
	if parentBase != nil && parentBase.computed != nil {
		pc := parentBase.computed
		comp.SetColor(pc.Color())
		if fs := pc.FontSize(); fs != nil {
			comp.SetFontSize(fs)
		}
		comp.SetFontFamily(pc.FontFamily())
		comp.SetFontWeight(pc.FontWeight())
		comp.SetTextAlign(pc.TextAlign())
	} else {
		comp.SetColor(NewColor(0, 0, 0, 255))
		comp.SetFontSize(NewSize(SIZE_PIXEL, 14, 0))
	}

	applyMatchingRules(comp, userAgentStylesheet(), base, e)
	applyMatchingRules(comp, author, parentBase, e)
	for _, d := range base.inline {
		applyDecl(comp, d.prop, d.value)
	}
	if comp.Color() == nil {
		comp.SetColor(NewColor(0, 0, 0, 255))
	}
	base.computed = comp

	for _, child := range base.children {
		resolveStylesTree(child, base, author)
	}
}

func applyMatchingRules(comp CSSStyleDeclaration, sheet *Stylesheet, parentBase *htmlElement, e HTMLElement) {
	_ = parentBase
	if sheet == nil {
		return
	}
	var matched []cssRule
	for _, r := range sheet.rules {
		for _, sel := range r.sels {
			if matchSelectorChain(sel, e) {
				matched = append(matched, r)
				break
			}
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		si, sj := matched[i].sels[0].Specificity(), matched[j].sels[0].Specificity()
		if si != sj {
			return si < sj
		}
		return matched[i].order < matched[j].order
	})
	for _, r := range matched {
		for _, d := range r.decls {
			applyDecl(comp, d.prop, d.value)
		}
	}
}

// inner 取得任意 HTMLElement 的底层结构（含包装器与文本节点）。
func inner(e HTMLElement) *htmlElement {
	switch t := e.(type) {
	case nil:
		return nil
	case *htmlElement:
		return t
	case *textNode:
		return t.node
	case baseElementProvider:
		return t.baseElement()
	default:
		return nil
	}
}

// baseElementProvider 由所有包装器类型自动满足（嵌入的 HTMLElement 提升）。
type baseElementProvider interface {
	baseElement() *htmlElement
}
