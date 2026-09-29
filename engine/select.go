package engine

import "strings"

// select/option 控件：展开态、选项浮层布局、取值与点击默认动作。
//
// 设计要点：
//   - option 不参与常规流（layoutBox 对 select 子树跳过 layoutChildren），
//     展开时由 layoutSelectPopups 在第二遍布局里把选项排到 select 边框盒
//     下方（空间不足且上方放得下时整体上移），关闭时清零其盒使其不可命中；
//   - 浮层覆盖其后的流内容，故命中测试（ElementAt）优先查展开的 option；
//   - 绘制上浮层排在定位层之后，保证盖住 z-index 定位元素。

// tagIs 判断元素标签名（大小写不敏感；解析器已小写，CreateElement 不保证）。
func tagIs(e HTMLElement, tag string) bool {
	b := inner(e)
	return b != nil && strings.EqualFold(b.tagName, tag)
}

// isAtomicFormControl 判断不参与子节点流式布局的表单控件（select/textarea）。
func isAtomicFormControl(b *htmlElement) bool {
	if b == nil {
		return false
	}
	return strings.EqualFold(b.tagName, "select") || strings.EqualFold(b.tagName, "textarea")
}

// selectOptions 返回 select 的 option 直接子元素（保持文档序）。
func selectOptions(sel HTMLElement) []HTMLElement {
	var out []HTMLElement
	if sel == nil {
		return out
	}
	for _, c := range sel.Children() {
		if tagIs(c, "option") {
			out = append(out, c)
		}
	}
	return out
}

// SelectOptions 是 selectOptions 的导出版本。
func SelectOptions(sel HTMLElement) []HTMLElement { return selectOptions(sel) }

// optionValue 返回 option 的值：value 属性，缺省为其文本内容。
func optionValue(opt HTMLElement) string {
	b := inner(opt)
	if b == nil {
		return ""
	}
	if v, ok := b.attrs["value"]; ok {
		return v
	}
	return strings.TrimSpace(opt.InnerText())
}

// selectedOptionBase 按 select 的 value 属性定位选中项；无匹配时退回带
// selected 属性的项，再退回首个选项——与渲染取文本的口径完全一致。
func selectedOptionBase(sel *htmlElement) *htmlElement {
	opts := selectOptions(sel)
	if len(opts) == 0 {
		return nil
	}
	want, _ := sel.attrs["value"]
	for _, o := range opts {
		if optionValue(o) == want {
			return inner(o)
		}
	}
	for _, o := range opts {
		if _, ok := inner(o).attrs["selected"]; ok {
			return inner(o)
		}
	}
	return inner(opts[0])
}

// SelectValue 返回 select 当前选中项的值（渲染同口径）。
func SelectValue(sel HTMLElement) string {
	b := inner(sel)
	if b == nil {
		return ""
	}
	if o := selectedOptionBase(b); o != nil {
		return optionValue(o)
	}
	return ""
}

// selectDisplayText 返回 select 收起态显示的文本：选中项的文本内容，
// 无文本时才退回其 value（显示文本 ≠ 值，与浏览器一致）。
func selectDisplayText(sel *htmlElement) string {
	o := selectedOptionBase(sel)
	if o == nil {
		return ""
	}
	if t := strings.TrimSpace(o.InnerText()); t != "" {
		return t
	}
	return optionValue(o)
}

// SetSelectValue 按值选中选项并触发变更通知；未找到匹配项返回 false。
// 程序化赋值不改变展开态（与浏览器一致）。
func SetSelectValue(sel HTMLElement, value string) bool {
	b := inner(sel)
	if b == nil {
		return false
	}
	for _, o := range selectOptions(b) {
		if optionValue(o) == value {
			changed := applySelectValue(b, value)
			if !changed {
				return true // 已是该值，但值合法
			}
			dispatchChange(b)
			return true
		}
	}
	return false
}

// applySelectValue 写入 value 属性（静默路径供布局期同步用；写属性触发
// 文档变更通知 → 重排重绘）。返回值是否变化。
func applySelectValue(sel *htmlElement, value string) bool {
	if sel.attrs == nil {
		sel.attrs = make(map[string]string)
	}
	if old, ok := sel.attrs["value"]; ok && old == value {
		return false
	}
	sel.attrs["value"] = value
	sel.onChanged()
	return true
}

// syncSelectValue 在布局期把缺失的 value 属性补成选中项的值。
// 必须走静默路径：布局中触发 markChanged 会形成「重排→变更→重排」死循环。
func syncSelectValue(sel *htmlElement) {
	if sel == nil || sel.attrs == nil {
		return
	}
	if _, ok := sel.attrs["value"]; ok {
		return
	}
	if o := selectedOptionBase(sel); o != nil {
		sel.attrs["value"] = optionValue(o)
	}
}

// selectOpen 返回 select 展开态。
func selectOpen(sel HTMLElement) bool {
	b := inner(sel)
	return b != nil && b.open
}

// setSelectOpen 切换展开态并触发重排（选项浮层的布局与清零都在重排里完成）。
func setSelectOpen(sel HTMLElement, open bool) {
	b := inner(sel)
	if b == nil || b.open == open {
		return
	}
	b.open = open
	b.onChanged()
}

// dispatchChange 从元素起沿父链派发 change 事件（与 click 同一套冒泡）。
func dispatchChange(e *htmlElement) {
	if e == nil {
		return
	}
	ev := &MouseEvent{Type: EventChange}
	for cur := e.self; cur != nil; cur = cur.ParentElement() {
		if b := inner(cur); b != nil {
			b.dispatch(EventChange, ev)
		}
	}
}

// borderBoxRect 返回元素的边框盒（内容盒外扩 padding+border）。
func borderBoxRect(b *htmlElement) (x, y, w, h int) {
	if b == nil {
		return 0, 0, 0, 0
	}
	if b.computed == nil {
		return b.x, b.y, b.width, b.height
	}
	bd := resolveEdgeRect(b.computed.BorderStyleWidth(), 0, 0)
	pd := resolveEdgeRect(b.computed.Padding(), 0, 0)
	return b.x - pd[eLeft] - bd[eLeft], b.y - pd[eTop] - bd[eTop],
		b.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight],
		b.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
}

// clearSubtreeBox 清零元素子树的布局盒与文本片段：关闭的选项既不可命中
// 也不会被绘制（引擎没有 display:none 那样的统一「不占位」语义可用）。
func clearSubtreeBox(b *htmlElement) {
	if b == nil {
		return
	}
	b.x, b.y, b.width, b.height, b.layoutH = 0, 0, 0, 0, 0
	b.runs = nil
	for _, c := range b.children {
		cb := inner(c)
		if cb == nil {
			if tn, ok := c.(*textNode); ok {
				tn.node.runs = nil
			}
			continue
		}
		clearSubtreeBox(cb)
	}
}

// layoutSelectPopups 是定位之后的第三遍布局：为每个 select 的选项定位
// （展开）或清零（关闭）。所有流水线入口都必须调用它。
func layoutSelectPopups(g Graphics, root HTMLElement, vw, vh int) {
	if g == nil {
		g = NewFakeGraphics()
	}
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		b := inner(e)
		if b == nil {
			return
		}
		if strings.EqualFold(b.tagName, "select") {
			layoutSelectOptions(g, b, vw, vh)
		}
		for _, c := range b.children {
			walk(c)
		}
	}
	walk(root)
}

// layoutSelectOptions 为单个 select 的选项排版：展开时按边框盒宽逐项
// 堆叠到下缘（下方放不下且上方放得下时整组上移），关闭时清零。
func layoutSelectOptions(g Graphics, sel *htmlElement, vw, vh int) {
	syncSelectValue(sel)
	opts := selectOptions(sel)
	if len(opts) == 0 {
		return
	}
	if !sel.open {
		for _, o := range opts {
			clearSubtreeBox(inner(o))
		}
		return
	}

	bx, by, bw, bh := borderBoxRect(sel)
	y := by + bh
	for _, o := range opts {
		ob := inner(o)
		layoutBox(g, ob, bw, maxInt(vh, 0), bx, y, bw)
		y += ob.layoutH
	}
	total := y - (by + bh)
	if total > 0 && by+bh+total > vh && by-total >= 0 {
		// 下方放不下、上方放得下 → 整组上移到 select 上缘之上
		dy := by - total - (by + bh)
		for _, o := range opts {
			shiftBox(o, 0, dy)
		}
	}
}

// openOptionAt 返回展开浮层中包含 (x,y) 的 option，无则 nil。
// 浮层绘制在所有常规内容之上，命中测试必须优先于树遍历，否则会被
// 浮层下方的兄弟元素抢走点击。
func openOptionAt(root HTMLElement, x, y int) HTMLElement {
	var found HTMLElement
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		b := inner(e)
		if b == nil {
			return
		}
		if strings.EqualFold(b.tagName, "select") && b.open {
			for _, o := range selectOptions(b) {
				if boxContains(inner(o), x, y) {
					found = o
				}
			}
		}
		for _, c := range b.children {
			walk(c)
		}
	}
	walk(root)
	return found
}

// boxContains 与 ElementAt 同一套命中规则：边框盒、半开区间、零尺寸不命中。
func boxContains(b *htmlElement, x, y int) bool {
	if b == nil {
		return false
	}
	var bd, pd edges
	if b.computed != nil {
		bd = resolveEdgeRect(b.computed.BorderStyleWidth(), 0, 0)
		pd = resolveEdgeRect(b.computed.Padding(), 0, 0)
	}
	l := b.x - pd[eLeft] - bd[eLeft]
	t := b.y - pd[eTop] - bd[eTop]
	w := b.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
	h := b.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
	if w <= 0 || h <= 0 {
		return false
	}
	return x >= l && x < l+w && y >= t && y < t+h
}

// collectOpenSelects 收集文档内所有展开的 select（绘制浮层用）。
func collectOpenSelects(root HTMLElement) []*htmlElement {
	var out []*htmlElement
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		b := inner(e)
		if b == nil {
			return
		}
		if strings.EqualFold(b.tagName, "select") && b.open {
			out = append(out, b)
		}
		for _, c := range b.children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// selectPopupBounds 计算展开浮层的整体边界（各 option 边框盒的并集）。
func selectPopupBounds(sel *htmlElement) (x0, y0, x1, y1 int, ok bool) {
	for _, o := range selectOptions(sel.self) {
		ox, oy, ow, oh := borderBoxRect(inner(o))
		if ow <= 0 || oh <= 0 {
			continue
		}
		if !ok {
			x0, y0, x1, y1 = ox, oy, ox+ow, oy+oh
			ok = true
			continue
		}
		if ox < x0 {
			x0 = ox
		}
		if oy < y0 {
			y0 = oy
		}
		if ox+ow > x1 {
			x1 = ox + ow
		}
		if oy+oh > y1 {
			y1 = oy + oh
		}
	}
	return
}

// containingSelect 返回元素自身或其祖先中的 select，无则 nil。
func containingSelect(e HTMLElement) HTMLElement {
	for cur := e; cur != nil; cur = cur.ParentElement() {
		if tagIs(cur, "select") {
			return cur
		}
	}
	return nil
}

// optionOf 返回元素自身或祖先中的 option，无则 nil。
func optionOf(e HTMLElement) HTMLElement {
	for cur := e; cur != nil; cur = cur.ParentElement() {
		if tagIs(cur, "option") {
			return cur
		}
	}
	return nil
}

// applySelectClick 执行 select 的点击默认动作，返回 value 是否变化
// （调用方据此补发 change）。规则：
//   - 命中 option：选中该项并收起；
//   - 命中 select 盒：切换展开态；
//   - 命中浮层之外：收起所有展开的 select（点空白收起）。
func applySelectClick(doc HTMLDocument, target HTMLElement) bool {
	sel := containingSelect(target)
	if sel == nil {
		for _, s := range collectOpenSelects(doc) {
			setSelectOpen(s, false)
		}
		return false
	}
	// 同一时刻至多展开一个：点击另一个 select 时先收起其余展开项
	//（否则点 B 后 A、B 同时展开，浮层互相叠加）
	for _, s := range collectOpenSelects(doc) {
		if s != inner(sel) {
			setSelectOpen(s, false)
		}
	}
	if opt := optionOf(target); opt != nil && selectOpen(sel) {
		setSelectOpen(sel, false)
		return applySelectValue(inner(sel), optionValue(opt))
	}
	setSelectOpen(sel, !selectOpen(sel))
	return false
}
