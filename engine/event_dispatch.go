package engine

// ElementAt 命中测试：返回包含 (x, y) 的最深层元素，未命中返回 nil。
// 文本节点不可命中，事件目标为其父元素；零尺寸盒不参与命中。
func ElementAt(root HTMLElement, x, y int) HTMLElement {
	var hit HTMLElement
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		for _, child := range e.Children() {
			if _, ok := child.(*textNode); ok {
				continue
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
				walk(child)
			}
		}
	}
	walk(root)
	return hit
}

// OnDocumentClick 在文档上派发一次点击事件：从最深命中元素向上冒泡，
// 依次调用各元素注册的 click 处理器。返回是否有处理器被触发。
func OnDocumentClick(doc HTMLDocument, x, y int) bool {
	ev := &MouseEvent{Type: EventClick, X: x, Y: y}
	target := ElementAt(doc, x, y)
	// 焦点先于 click 处理器更新（聚焦新目标后处理器才能读到正确状态）。
	updateClickFocus(doc, target)
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
	return handled
}
