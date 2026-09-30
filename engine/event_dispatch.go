package engine

// ElementAt 命中测试：返回包含 (x, y) 的最深层元素，未命中返回 nil。
// 文本节点不可命中，事件目标为其父元素；零尺寸盒不参与命中。
// 展开的 select 选项浮层绘制在最上层，优先于常规树遍历判定。
// 命中受祖先 overflow 裁剪约束：点落在裁剪区外时整个子树不可命中
//（视觉上已被裁掉的内容不应可点，与渲染一致）。
func ElementAt(root HTMLElement, x, y int) HTMLElement {
	if opt := openOptionAt(root, x, y); opt != nil {
		return opt
	}
	var hit HTMLElement
	var walk func(e HTMLElement, clips []clipRect)
	walk = func(e HTMLElement, clips []clipRect) {
		for _, child := range e.Children() {
			if _, ok := child.(*textNode); ok {
				continue
			}
			if !clipsContain(clips, x, y) {
				return // 点在祖先裁剪区外：本层其余子树同样不可见
			}
			r := child.GetBoundingClientRect()
			l := r.Left().Pixel()
			t := r.Top().Pixel()
			wd := r.Right().Pixel() - l
			hh := r.Bottom().Pixel() - t
			if wd <= 0 || hh <= 0 {
				continue
			}
			if x >= l && x < l+wd && y >= t && y < t+hh {
				hit = child
				next := clips
				if cb := inner(child); cb != nil && isClippingBox(cb) {
					pd := resolveEdgeRect(cb.computed.Padding(), 0, 0)
					next = append(append([]clipRect{}, clips...), clipRect{
						x: cb.x - pd[eLeft], y: cb.y - pd[eTop],
						w: cb.width + pd[eLeft] + pd[eRight],
						h: cb.height + pd[eTop] + pd[eBottom],
					})
				}
				walk(child, next)
			}
		}
	}
	walk(root, nil)
	return hit
}

// clipsContain 判断点是否落在全部裁剪矩形内（空栈恒真；退化矩形恒假）。
func clipsContain(clips []clipRect, x, y int) bool {
	for _, c := range clips {
		if c.w <= 0 || c.h <= 0 {
			return false
		}
		if x < c.x || x >= c.x+c.w || y < c.y || y >= c.y+c.h {
			return false
		}
	}
	return true
}

// OnDocumentClick 在文档上派发一次点击事件：从最深命中元素向上冒泡，
// 依次调用各元素注册的 click 处理器；随后执行引擎默认动作
// （select 展开/收起/选中、radio/checkbox 勾选切换、<a> 导航），
// value/勾选状态变化补发 change。返回是否有处理器被触发。
func OnDocumentClick(doc HTMLDocument, x, y int) bool {
	ev := &MouseEvent{Type: EventClick, X: x, Y: y}
	// 展开的 select 浮层绘制在最上层，命中优先于常规树遍历（见 ElementAt）。
	target := ElementAt(doc, x, y)
	// 焦点/光标先于 click 处理器更新（处理器才能读到正确状态）。
	updateClickFocus(doc, target, x, y)
	changed := applySelectClick(doc, target)
	// 勾选切换同样在冒泡之前完成：click 处理器读到的是切换后的状态。
	toggled, didToggle := applyCheckClick(target)
	handled := false
	for e := target; e != nil; e = e.ParentElement() {
		base := inner(e)
		if base == nil {
			continue
		}
		if base.dispatch(EventClick, ev) {
			handled = true
		}
	}
	if changed {
		if sel := containingSelect(target); sel != nil {
			dispatchChange(inner(sel))
		}
	}
	if didToggle {
		dispatchChange(toggled)
	}
	navigate(doc, target)
	return handled
}
