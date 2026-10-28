package engine

import "strings"

// 交互状态（:hover / :active / :focus）变更的分级处理。
//
// # 问题
//
// 状态变化曾一律 markChanged → 主循环全量级联 + 布局。42 万元素的压力页上
// 鼠标滑入/滑出一个元素要重排整棵树（实测 1.72 秒、13,380,022 次文本测量），
// 而这些变更里绝大多数**只改颜色**——盒模型一个像素都不动，重排是纯浪费；
// 无 :hover 规则的页面（压力页就是）更是连样式都不变，1.7 秒什么都没买到。
//
// # 三档处理（判定顺序）
//
//  1. 没有任何受影响的元素（见 interactionRoots）→ **什么都不做**：不标记、
//     不通知、不重绘。无 :hover 规则且悬停目标不在表单控件上即此档——
//     UA 的悬停反馈（applyInteractionDefaults）只作用于 button/input/
//     select/textarea 这四种控件。
//  2. 状态相关规则的声明**全是纯绘制属性**（见 isPaintOnlyDecl）→ **只重绘**：
//     就地重算受影响子树的样式（markPaintChanged 记账，主循环跳过布局直接
//     重绘）。
//  3. 声明里出现任何可能改盒模型的属性（width/padding/font-…）→ **回退全量**：
//     markChanged()，走既有的 LayoutDocument 全量级联 + 布局。
//
// 档 2 的静态判定是**保守的**：白名单只收「CSS 语义上永远不进布局」的属性
// （颜色族、背景族、outline 那些），未收录/引擎未实现的属性一律回退档 3——
// 判错的方向只会是「多排一次」，不会是「漏排」。
//
// # 档 2 的级联范围（为什么可以只算一棵子树）
//
// 状态变化只可能改变两类东西：
//
//	a) **命中该状态的元素自身**：选择器里含 :hover 的复合片段，绑定的元素
//	   必然处于新/旧状态链上（matchHoverState 沿父链继承 → 链上元素）；
//	b) **其子树的继承值**：color/font-weight 等继承属性从 (a) 往下传播。
//
// 引擎的选择器只有后代/子代组合符（selector.go 只有 combDescendant/
// combChild，没有兄弟组合符），含状态的复合片段右侧只会**向下**展开，
// 所以所有「命中会翻转」的元素都落在**状态链元素的子树内**。于是只需对
// 链上真正被状态规则命中的元素（roots）逐个 resolveStylesTreeIn——
// 父链上未受影响的祖先不重算，子树外的 computed 原样保留。
//
// root 取「最靠近根的那个」：链上多代同时被命中时，浅的那个子树覆盖深的。
//
// # 与滚动快路径的关系
//
// 档 2 只动 paintChanged，不进 changed/scrollMark 两本账，滚动快路径的
// 「增量相等」判据（markScrollChanged）不受影响；且样式在标记前已就地算完，
// 即便下一帧走滚动快路径跳过级联，悬停颜色也已经是对的。

// 交互状态档位（内部枚举：规则扫描、binder 判定、root 收集共用一套口径）。
const (
	stateHover = iota
	stateActive
	stateFocus
)

// statePseudoName 返回状态对应的伪类名（selector 里 ':' 之后的小写名）。
func statePseudoName(state int) string {
	switch state {
	case stateActive:
		return pseudoActive
	case stateFocus:
		return pseudoFocus
	}
	return pseudoHover
}

// applyStateChange 处理一次交互状态变更：按包注释的三档判定决定「不动 /
// 就地重算受影响子树 + 记重绘账 / 回退全量重排」。
//
// changed 是本次状态可能波及的候选元素：hover/active 沿父链继承，传
// 新旧目标的整条祖先链；focus 不继承，传新旧焦点自身。候选可以有重叠与
// 祖先覆盖，roots 收集时统一去重（见 appendSubtreeRoot）。
func (d *htmlDocument) applyStateChange(state int, changed []HTMLElement) {
	roots := d.interactionRoots(state, changed)
	if len(roots) == 0 {
		// 没有任何样式会变：不标记、不通知——与「悬停目标没变就不通知」
		// 同一口径（SetHoveredElement）。无 :hover 规则的页面上鼠标移动
		// 因此完全免费。
		return
	}
	if d.interactionNeedsReflow(state) {
		// 有规则会改盒模型：回退全量，让 LayoutDocument 级联 + 布局。
		d.markChanged()
		return
	}
	// 纯绘制档：就地重算受影响子树的样式（盒模型不变，无需布局），记一笔
	// 重绘账，主循环按 PaintCount 跳过 LayoutDocument 直接重绘。
	author := d.Stylesheet()
	for _, r := range roots {
		rb := inner(r)
		if rb == nil {
			continue
		}
		resolveStylesTreeIn(d, r, rb, inner(r.ParentElement()), author)
	}
	d.markPaintChanged()
}

// isInteractionControl 判断元素是否参与 UA 的悬停/按下反馈
// （applyInteractionDefaults：button/input/select/textarea 这四种会改底色
// 或边框色，其余标签直接返回）。与那处 switch 的覆盖范围保持一致——漏一个
// 就会漏掉「没有 :hover 规则也要重算」的控件。
func isInteractionControl(el HTMLElement) bool {
	if el == nil {
		return false
	}
	switch strings.ToLower(el.TagName()) {
	case "button", "input", "select", "textarea":
		return true
	}
	return false
}

// interactionRoots 收集本次状态变更真正会波及的**子树根**（见包注释 b 档
// 判定与 root 去重）。空切片 = 没有任何样式会变。
func (d *htmlDocument) interactionRoots(state int, changed []HTMLElement) []HTMLElement {
	var roots []HTMLElement
	for _, el := range changed {
		if el == nil {
			continue
		}
		switch {
		case state == stateFocus:
			// 焦点不沿父链继承（matchFocusState 只比自身），但聚焦框/光标/
			// 选区由渲染层按 focus 绘制，即使没有 :focus 规则也要重绘——
			// 焦点恒进重绘档，候选就是焦点元素自己。
			roots = appendSubtreeRoot(roots, el)
		case isInteractionControl(el):
			// UA 悬停/按下反馈（applyInteractionDefaults）按**元素自身**
			// 的状态改底色/边框色，只作用于这四种控件，与作者规则无关——
			// 规则一条没有也得重算它。
			roots = appendSubtreeRoot(roots, el)
		case d.stateBindsElement(state, el):
			roots = appendSubtreeRoot(roots, el)
		}
	}
	return roots
}

// interactionNeedsReflow 判断该状态的规则里是否有**非纯绘制**声明（见
// isPaintOnlyDecl）。作者表与 UA 表都要看：任何一边越界即回退全量。
func (d *htmlDocument) interactionNeedsReflow(state int) bool {
	for _, sheet := range []*Stylesheet{d.Stylesheet(), userAgentStylesheet()} {
		st := sheet.interactions().of(state)
		if st.exists && !st.paintOnly {
			return true
		}
	}
	return false
}

// stateBindsElement 判断元素是否**可能**被该状态的某个复合片段绑定
// （存在一种状态赋值使其成立即可）——是则它就是状态样式的子树根。
//
// 只需看状态链上的元素（changed）：匹配翻转要求被绑定者恰好处于新/旧
// 状态，而状态只在链上成立。
func (d *htmlDocument) stateBindsElement(state int, el HTMLElement) bool {
	return sheetStateBindsDoc(d, d.Stylesheet(), state, el) ||
		sheetStateBindsDoc(d, userAgentStylesheet(), state, el)
}

// sheetStateBindsDoc 在单张样式表里找「提及该状态、且其状态复合片段能绑定
// el」的规则。规则子集由 interactions() 扫描期预切好（只有提及该状态的
// 规则才可能翻转命中）。
func sheetStateBindsDoc(doc HTMLDocument, sheet *Stylesheet, state int, el HTMLElement) bool {
	st := sheet.interactions().of(state)
	if !st.exists {
		return false
	}
	for _, r := range st.rules {
		for _, sel := range r.sels {
			for _, cp := range sel.compounds {
				if compoundMentions(cp, state) && compoundBindable(doc, cp, el) {
					return true
				}
			}
		}
	}
	return false
}

// appendSubtreeRoot 往 root 列表里加一个子树根，保持「互不覆盖」：
//   - el 已被现有 root 覆盖（同元素或其子树内）→ 跳过；
//   - el 是某个现有 root 的祖先 → 用 el 替换它（祖先的子树更大，覆盖后者）。
func appendSubtreeRoot(roots []HTMLElement, el HTMLElement) []HTMLElement {
	for _, r := range roots {
		if sameElement(r, el) || isDescendantOf(el, r) {
			return roots // el 的子树已被 r 覆盖
		}
	}
	out := roots[:0]
	for _, r := range roots {
		if isDescendantOf(r, el) {
			continue // r 是 el 的后代，被 el 的子树覆盖
		}
		out = append(out, r)
	}
	return append(out, el)
}

// isDescendantOf 判断 child 是否是 ancestor 的**严格**后代。
func isDescendantOf(child, ancestor HTMLElement) bool {
	ia, ib := inner(child), inner(ancestor)
	if ia == nil || ib == nil {
		return false
	}
	for cur := child; cur != nil; cur = cur.ParentElement() {
		if inner(cur) == ib {
			return true
		}
	}
	return false
}

// interactionScan 是一张样式表的交互状态静态扫描结果（惰性缓存，规则
// 变更时随索引一起失效，见 invalidateIndex）。
type interactionScan struct {
	hover  stateRules
	active stateRules
	focus  stateRules
}

// of 返回指定状态的扫描结果。
func (s *interactionScan) of(state int) stateRules {
	if s == nil {
		return stateRules{}
	}
	switch state {
	case stateActive:
		return s.active
	case stateFocus:
		return s.focus
	}
	return s.hover
}

// stateRules 是「提及某状态的规则」的静态分析：
//   - exists：有没有这种规则（没有 → 该状态在这张表上无样式影响）；
//   - paintOnly：这些规则的声明是否**全部**为纯绘制属性（exists=false 时
//     无意义）；
//   - rules：提及该状态的规则子集，binder 判定只扫这里。
type stateRules struct {
	exists    bool
	paintOnly bool
	rules     []*cssRule
}

// interactions 返回样式的交互状态扫描结果（nil 表按无规则处理）。
//
// 扫描按**规则**聚合：一条规则的声明对它提及的每个状态都算数（逗号列表
// 选择器共享同一份声明，`a:hover, div { … }` 的声明也会作用在 hover 命中
// 上）。
func (s *Stylesheet) interactions() *interactionScan {
	if s == nil {
		return nil
	}
	if s.interact != nil {
		return s.interact
	}
	sc := &interactionScan{
		hover:  stateRules{paintOnly: true},
		active: stateRules{paintOnly: true},
		focus:  stateRules{paintOnly: true},
	}
	for i := range s.rules {
		r := &s.rules[i]
		var (
			mentionsHover, mentionsActive, mentionsFocus bool
		)
		for _, sel := range r.sels {
			for _, cp := range sel.compounds {
				if compoundMentions(cp, stateHover) { // 复合片段递归看 :not 参数
					mentionsHover = true
				}
				if compoundMentions(cp, stateActive) {
					mentionsActive = true
				}
				if compoundMentions(cp, stateFocus) {
					mentionsFocus = true
				}
			}
		}
		if !mentionsHover && !mentionsActive && !mentionsFocus {
			continue
		}
		declsPaint := true
		for _, decl := range r.decls {
			if !isPaintOnlyDecl(decl.prop) {
				declsPaint = false
				break
			}
		}
		add := func(st *stateRules, mentioned bool) {
			if !mentioned {
				return
			}
			st.exists = true
			st.paintOnly = st.paintOnly && declsPaint
			st.rules = append(st.rules, r)
		}
		add(&sc.hover, mentionsHover)
		add(&sc.active, mentionsActive)
		add(&sc.focus, mentionsFocus)
	}
	s.interact = sc
	return sc
}

// isPaintOnlyDecl 判断一条声明是否**永远不进布局**（只影响绘制/命中）。
//
// 白名单方向刻意保守：只收 CSS 语义上与盒模型无关、且引擎的布局路径确实
// 不读的属性（颜色从不在布局里参与几何，背景/outline/阴影按 CSS 定义都画
// 在盒外不撑盒）。**未收录的属性（含引擎未实现的 transform/filter 之类）
// 一律按会改布局处理**——判错的方向只能是多排一次全量，不会漏排。
//
// 与 LayoutDocument 的关系：进入 paint 档后样式就地重算、布局不跑，所以
// 白名单的正确性要求是「重算后的 computed 不会让已算好的几何过期」。
// 文本片段（textRun）只存文本与几何、颜色渲染期现读 computed（见
// text_node.go / renderNodeTextColor），颜色类变更因此天然不需重排。
func isPaintOnlyDecl(prop string) bool {
	switch prop {
	case "color", "background", "visibility", "opacity", "cursor",
		"text-decoration", "pointer-events", "border-radius":
		return true
	}
	// background-*（color/size/position/repeat/clip…）都不撑盒；
	// -color 族（border-*-color、outline-*-color、caret-color、
	// text-decoration-color…）只改描边/填充的颜色，宽度另有其属性。
	//（border / border-width 这类不带 -color 后缀，不在此列 → 回退全量。）
	if len(prop) > len("background-") && strings.HasPrefix(prop, "background-") {
		return true
	}
	if strings.HasSuffix(prop, "-color") && len(prop) > len("-color") {
		return true
	}
	return false
}

// compoundMentions 判断复合片段是否提及某状态（递归展开 :not() 的参数
// 选择器——`div:not(:hover)` 的命中同样随 hover 翻转）。
func compoundMentions(c *compoundSelector, state int) bool {
	if c == nil {
		return false
	}
	name := statePseudoName(state)
	for _, ps := range c.pseudos {
		if ps.name == name {
			return true
		}
		for _, sub := range ps.sels {
			for _, scp := range sub.compounds {
				if compoundMentions(scp, state) {
					return true
				}
			}
		}
	}
	return false
}

// compoundBindable 判断元素是否**可能**匹配该复合片段：静态约束照比，
// 但 hover/active 这类**本次正在变化**的状态与含它们的 :not(...) 视为可
// 满足（存在一种状态赋值即可），其余伪类按当前状态与 DOM 形状正常判。
//
// 松约束是刻意的超集：多判 = 多级联一棵子树（浪费一次小重算），漏判 =
// 样式不更新（视觉错误）。
func compoundBindable(doc HTMLDocument, c *compoundSelector, e HTMLElement) bool {
	return matchCompoundBy(doc, c, e, bindablePseudo)
}

// bindablePseudo 是 compoundBindable 的伪类判定（见 matchCompoundBy）。
func bindablePseudo(doc HTMLDocument, pc pseudoClass, e HTMLElement) bool {
	if pc.bad {
		return false
	}
	if pc.name == pseudoHover || pc.name == pseudoActive {
		return true // 交互态由状态链决定，存在可满足的赋值
	}
	if pc.name == pseudoNot {
		for _, sub := range pc.sels {
			for _, scp := range sub.compounds {
				if compoundMentions(scp, stateHover) || compoundMentions(scp, stateActive) {
					return true // :not(…:hover…) 同样存在可满足的赋值
				}
			}
		}
	}
	return matchPseudo(doc, pc, e)
}
