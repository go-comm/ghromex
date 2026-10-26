package engine

import (
	"sort"
	"strings"
)

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

// defaultStyleTemplate 是 newStyle() 的一次性副本（具体结构体），用于复用 computed
// 对象而非每轮新建：每元素每级联保存一份胖结构体（~600B），复用后
// 15 万节点的页面每轮省 ~90MB 分配 + GC 压力。
var defaultStyleTemplate = newStyle().(*cssStyleDeclaration)

// resolveStylesTree 为整棵树计算级联后的最终样式（写入 base.computed）。
// 优先级：UA 样式表 < 文档样式表（特异度+声明顺序） < 内联 style 属性；
// color/font-size/font-family/font-weight/text-align/text-decoration 沿父链继承。
// 带 !important 的声明整体压过普通声明（作者的重要声明也压过 UA 的重要声明）。
func resolveStylesTree(e HTMLElement, parentBase *htmlElement, author *Stylesheet) {
	base := inner(e)
	if base == nil {
		return
	}
	// 文档沿树传下去：伪类匹配要查动态状态（hover/focus/active）。
	resolveStylesTreeIn(docOf(e), e, base, parentBase, author)
}

// resolveStylesTreeIn 是 resolveStylesTree 的递归实现；doc 沿树传下去，
// 供选择器匹配查询伪类状态（hover/focus/active 在文档上）。
func resolveStylesTreeIn(doc HTMLDocument, e HTMLElement, base *htmlElement,
	parentBase *htmlElement, author *Stylesheet) {
	comp := base.computed
	if comp == nil {
		comp = newStyle()
		base.computed = comp
	}
	// 复用 computed：把模板默认值整体复制回去（清零 non-inherited 字段
	// 为 CSS 初始值）。继承字段随后会被 applyInheritedStyle 覆盖。
	if cs, ok := comp.(*cssStyleDeclaration); ok {
		*cs = *defaultStyleTemplate
	}

	// 继承性默认值
	if parentBase != nil && parentBase.computed != nil {
		applyInheritedStyle(comp, parentBase.computed)
	} else {
		comp.SetColor(NewColor(0, 0, 0, 255))
		comp.SetFontSize(NewSize(SIZE_PIXEL, 16, 0))
	}

	// 收集全部候选声明后按 CSS 2.1 §6.4 的顺序应用：
	//   普通声明：UA 规则 → UA type 校正 → 作者规则 → 内联
	//   重要声明：UA → 作者 → 内联（重要声明整体压过普通声明）
	// 分段应用而非统一排序：UA type 校正（如 input[type=radio] 的 13×13 外形）
	// 必须夹在 UA 通用规则与作者规则之间——作者 CSS 才能覆盖它。
	uaCands := collectMatchingDecls(doc, userAgentStylesheet(), e, originUA)
	authorCands := collectMatchingDecls(doc, author, e, originAuthor)
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
	applyInputTypeDefaults(comp, base)        // input 按 type 的 UA 校正（作者 CSS 可覆盖）
	applyInteractionDefaults(comp, base, doc) // 悬停/按下的 UA 默认反馈（作者 CSS 可覆盖）
	applyCands(filterByImportant(authorCands, false))
	applyCands(filterByImportant(inlineCands, false))
	applyCands(filterByImportant(uaCands, true))
	applyCands(filterByImportant(authorCands, true))
	applyCands(filterByImportant(inlineCands, true))
	if comp.Color() == nil {
		comp.SetColor(NewColor(0, 0, 0, 255))
	}
	// Chrome：「font-family 落到 monospace 通用族 + 字号仍是默认值（作者从未
	// 显式声明过 font-size）」时用默认等宽字号 13px，而非正文默认 16px。
	// 实测（Chrome headless）：body 不写字号、.f-mono{font-family:monospace}
	// 的计算字号是 13px；祖先显式 font-size:16px 时保持 16px。缺这条规则
	// 时整行字大一号、行高多 4px，与浏览器对照会差一整行。
	sizeDeclared := false
	for _, cands := range [][]declCandidate{uaCands, authorCands, inlineCands} {
		for _, c := range cands {
			if c.decl.prop == "font-size" {
				sizeDeclared = true
				break
			}
		}
	}
	if cs, ok := comp.(*cssStyleDeclaration); ok {
		if !sizeDeclared && !cs.fontSizeExplicit && isMonospaceGeneric(comp.FontFamily()) {
			comp.SetFontSize(NewSize(SIZE_PIXEL, 13, 0))
		}
		cs.fontSizeExplicit = cs.fontSizeExplicit || sizeDeclared
	}
	// font-size 百分比按**父元素计算字号**解析为像素（CSS 2.1 §10.2）。
	// 必须在级联结束后做：百分比是相对值，布局期再解析拿不到父级的最终
	// 字号；这里落成像素后，度量/绘制沿用的都是同一份绝对值。
	if fs := comp.FontSize(); fs != nil && fs.Type() == SIZE_PERCENT {
		base := 16
		if parentBase != nil && parentBase.computed != nil {
			if p := resolveLen(parentBase.computed.FontSize(), 0); p > 0 {
				base = p
			}
		}
		px := int(float64(base)*float64(fs.Percent())/100 + 0.5)
		comp.SetFontSize(NewSize(SIZE_PIXEL, px, 0))
	}
	base.computed = comp

	// ::before/::after 生成盒：样式按宿主元素级联，盒按宿主元素挂载
	// （见 generated.go）。必须在递归子节点之前做完——生成盒不参与常规
	// 级联，由这里直接写入 computed。
	syncGeneratedContent(doc, base, author)

	for _, child := range base.children {
		cb := inner(child)
		if cb == nil || cb.generated {
			continue
		}
		resolveStylesTreeIn(doc, child, cb, base, author)
	}
}

// applyInheritedStyle 把父元素计算样式中可继承的属性写进 comp。
//
// 单独抽出来供伪元素复用：::before/::after 是宿主元素的子盒，继承口径
// 必须与普通子元素完全一致（否则伪元素的字号/颜色/行高会跟正文对不上）。
// 调用方负责在无父元素时写入 CSS 初始值。
func applyInheritedStyle(comp CSSStyleDeclaration, pc CSSStyleDeclaration) {
	if comp == nil || pc == nil {
		return
	}
	comp.SetColor(pc.Color())
	if fs := pc.FontSize(); fs != nil {
		comp.SetFontSize(fs)
	}
	comp.SetFontFamily(pc.FontFamily())
	comp.SetFontWeight(pc.FontWeight())
	// 数值字重随两档一起继承：bolder/lighter 相对它升降档，子元素不
	// 声明 font-weight 时也要保持父级的 600 语义（否则退化成 700）。
	setFontWeightNumOf(comp, fontWeightNumOf(pc))
	comp.SetFontStyle(pc.FontStyle())
	// visibility 与 cursor 都是继承属性：父级 hidden 的子树默认不绘制，
	// 子级声明 visible 才恢复；cursor 通常写在容器上（整块可拖拽区域）。
	comp.SetVisibility(pc.Visibility())
	comp.SetCursor(pc.Cursor())
	comp.SetTextAlign(pc.TextAlign())
	comp.SetTextDecoration(pc.TextDecoration())
	comp.SetWhiteSpace(pc.WhiteSpace())
	// pointer-events 是继承属性：占位块/分隔线声明 none 之后，它里面的
	// 内容（如果有）默认也跟着不可点，子级自己声明 auto 才恢复。
	comp.SetPointerEvents(pc.PointerEvents())
	// line-height 继承**原始形态**（倍数/绝对长度），不是解析后的像素：
	// 父级 line-height:1.5 + 子级 font-size:32px → 子级行高 48px，
	// 继承成像素就锁死在父级字号上了。
	comp.SetLineHeight(pc.LineHeight())
	// empty-cells / caption-side 是继承属性：作者写在 table 上（常见）
	// 或写在祖先上，靠继承落到单元格与题注；border-spacing 不继承，
	// 表格布局只读表格元素自身的声明。
	comp.SetEmptyCells(pc.EmptyCells())
	comp.SetCaptionSide(pc.CaptionSide())
	// list-style-type / list-style-position 是继承属性：作者通常写在 ul/ol
	// 上，标记由 li 读取；li 自己声明则覆盖。
	comp.SetListStyleType(pc.ListStyleType())
	comp.SetListStylePosition(pc.ListStylePosition())
	if pcs, ok := pc.(*cssStyleDeclaration); ok {
		if cs, ok := comp.(*cssStyleDeclaration); ok {
			cs.fontSizeExplicit = pcs.fontSizeExplicit
		}
	}
}

// isMonospaceGeneric 判断 font-family 的**首项**是否为等宽通用族。
// 只看首项：Chrome 的「默认等宽字号」只在最终用到的字体就是等宽通用族时
// 生效——"Courier New, monospace" 首项是具名字体，计算字号保持继承值
// （实测 16px）。引擎拿不到字体可用性，用首项近似。
func isMonospaceGeneric(family string) bool {
	first := strings.Split(family, ",")[0]
	name := strings.ToLower(strings.Trim(strings.TrimSpace(first), `"'`))
	return name == "monospace" || name == "ui-monospace"
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
// 顺序排好（同来源内）。doc 供伪类匹配查询动态状态。
//
// 规则经尾部复合选择器主键索引（sheet.ensureIndex）查桶：元素只可能命中
// 「id 相等 / class 包含 / tag 相等 / 无主键」四类桶里的规则，候选从
// 「全部规则」缩到少数几条，再做完整链匹配（祖先与组合符校验）。
// 15 万节点 × 数百条规则的页面，逐元素规则扫描因此从分钟级降到毫秒级。
func collectMatchingDecls(doc HTMLDocument, sheet *Stylesheet, e HTMLElement, origin int) []declCandidate {
	return collectDeclsIndexed(doc, sheet, e, origin,
		func(sel *Selector) bool { return matchSelectorChain(doc, sel, e) })
}

// collectPseudoDecls 收集命中 e 的 ::before/::after 规则的声明。匹配口径
// 是「宿主元素 + 尾部伪元素」（p:hover::before 的 :hover 按宿主状态判定）。
//
// 与元素自身匹配共用同一主键索引：伪元素选择器的尾部主键同样是宿主匹配
// 的必要条件（p::before 按 tag=p 入桶），多余的候选在 matchPseudoElementChain
// 里被伪元素名校验挡掉（普通规则 tail.pseudoElement 为空直接 false）。
func collectPseudoDecls(doc HTMLDocument, sheet *Stylesheet, e HTMLElement, kind string, origin int) []declCandidate {
	if !sheet.hasPseudoRules() {
		return nil
	}
	return collectDeclsIndexed(doc, sheet, e, origin,
		func(sel *Selector) bool { return matchPseudoElementChain(doc, sel, e, kind) })
}

// collectDeclsIndexed 按尾部主键查桶收集候选规则，再用 matches 做完整链
// 校验，返回按特异度与源码顺序排好的声明。两条级联路径（元素自身 /
// 伪元素）共用，差别只在 matches。
func collectDeclsIndexed(doc HTMLDocument, sheet *Stylesheet, e HTMLElement, origin int,
	matches func(*Selector) bool) []declCandidate {
	if sheet == nil {
		return nil
	}
	ix := sheet.ensureIndex()
	type matchedRule struct {
		r    *cssRule
		spec int
	}
	var matched []matchedRule
	// 同一规则的多个选择器可能落在不同桶，按规则去重：多个选择器同时命中
	// 时取最高特异度（CSS 级联对同规则多选择器命中的口径，比旧实现按源码
	// 顺序取首个更确定）。
	add := func(r *cssRule, spec int) {
		for i := range matched {
			if matched[i].r == r {
				if spec > matched[i].spec {
					matched[i].spec = spec
				}
				return
			}
		}
		matched = append(matched, matchedRule{r: r, spec: spec})
	}
	visit := func(ents []indexEntry) {
		for _, en := range ents {
			if !matches(en.sel) {
				continue
			}
			add(&sheet.rules[en.ruleIdx], en.sel.Specificity())
		}
	}
	visit(ix.all)
	if id := e.GetAttribute("id"); id != "" {
		visit(ix.byID[id])
	}
	if cls := e.GetAttribute("class"); cls != "" {
		for _, w := range strings.Fields(cls) {
			visit(ix.byClass[w])
		}
	}
	if tag := strings.ToLower(e.TagName()); tag != "" {
		visit(ix.byTag[tag])
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
