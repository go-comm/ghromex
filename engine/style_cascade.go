package engine

import "sort"

// 级联来源序号：同优先级内按来源升序应用（后者覆盖前者）。
const (
	originUA = iota
	originAuthor
	originInline
)

// declCandidate 是一条待应用的声明及其级联定位。
type declCandidate struct {
	decl      simpleDecl
	important bool
	origin    int // originUA/originAuthor/originInline
	spec      int // 特异度（同来源内越大越优先）
	order     int // 同特异度内的源码顺序
}

// filterByImportant 按 !important 标记过滤候选（保持原顺序）。
// resolveStylesTree 为整棵树计算级联后的最终样式（写入 base.computed）。
// 优先级：UA 样式表 < 文档样式表（特异度+声明顺序） < 内联 style 属性；
// color/font-size/font-family/font-weight/text-align/text-decoration 沿父链继承。
// 带 !important 的声明整体压过普通声明（作者的重要声明也压过 UA 的重要声明）。
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
		comp.SetTextDecoration(pc.TextDecoration())
	} else {
		comp.SetColor(NewColor(0, 0, 0, 255))
		comp.SetFontSize(NewSize(SIZE_PIXEL, 16, 0))
	}

	// 收集全部候选声明后按 CSS 2.1 §6.4 的顺序应用：
	//   普通声明：UA 规则 → UA type 校正 → 作者规则 → 内联
	//   重要声明：UA → 作者 → 内联（重要声明整体压过普通声明）
	// 分段应用而非统一排序：UA type 校正（如 input[type=radio] 的 13×13 外形）
	// 必须夹在 UA 通用规则与作者规则之间——作者 CSS 才能覆盖它。
	uaCands := collectMatchingDecls(userAgentStylesheet(), e, originUA)
	authorCands := collectMatchingDecls(author, e, originAuthor)
	inlineCands := make([]declCandidate, 0, len(base.inline))
	for i, d := range base.inline {
		inlineCands = append(inlineCands, declCandidate{decl: d, important: d.important,
			origin: originInline, order: i})
	}
	applyCands := func(cands []declCandidate) {
		for _, c := range cands {
			applyDecl(comp, c.decl.prop, c.decl.value)
		}
	}
	applyCands(filterByImportant(uaCands, false))
	applyInputTypeDefaults(comp, base) // input 按 type 的 UA 校正（作者 CSS 可覆盖）
	applyCands(filterByImportant(authorCands, false))
	applyCands(filterByImportant(inlineCands, false))
	applyCands(filterByImportant(uaCands, true))
	applyCands(filterByImportant(authorCands, true))
	applyCands(filterByImportant(inlineCands, true))
	if comp.Color() == nil {
		comp.SetColor(NewColor(0, 0, 0, 255))
	}
	base.computed = comp

	for _, child := range base.children {
		resolveStylesTree(child, base, author)
	}
}

// filterByImportant 按 !important 标记过滤候选（保持原顺序）。
func filterByImportant(cands []declCandidate, important bool) []declCandidate {
	var out []declCandidate
	for _, c := range cands {
		if c.important == important {
			out = append(out, c)
		}
	}
	return out
}

// collectMatchingDecls 收集样式表中命中该元素的规则声明，按特异度与源码
// 顺序排好（同来源内）。
func collectMatchingDecls(sheet *Stylesheet, e HTMLElement, origin int) []declCandidate {
	if sheet == nil {
		return nil
	}
	type matchedRule struct {
		r    cssRule
		spec int
	}
	var matched []matchedRule
	for _, r := range sheet.rules {
		for _, sel := range r.sels {
			if matchSelectorChain(sel, e) {
				matched = append(matched, matchedRule{r: r, spec: sel.Specificity()})
				break
			}
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].spec != matched[j].spec {
			return matched[i].spec < matched[j].spec
		}
		return matched[i].r.order < matched[j].r.order
	})
	var out []declCandidate
	for _, m := range matched {
		for _, d := range m.r.decls {
			out = append(out, declCandidate{decl: d, important: d.important,
				origin: origin, spec: m.spec, order: m.r.order})
		}
	}
	return out
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
