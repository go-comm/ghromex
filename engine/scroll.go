package engine

import "strings"

// 滚动：overflow 滚动容器 + 文档级滚动的偏移施加与滚轮分发。
//
// 模型（与浏览器对齐的简化版）：
//   - 每个元素的滚动偏移存放在 htmlElement.scrollTop/scrollLeft 上，
//     由 applyScrollOffsets 在「常规流 + 定位」布局完成后统一夹紧并
//     平移其子树（布局每轮从头重算，偏移不会跨轮累积）；
//   - 根元素的滚动盒取视口（而非自身盒高），从而长页面可整页滚动；
//   - 滚轮从命中元素沿祖先链向上传递，对应轴滚到底才继续向上传
//     （scroll chaining），最后落到文档级滚动；textarea 例外：纵向
//     滚轮全量吸收（含撞边界后的剩余位移），光标停在框上时页面不动。
//
// 滚动条 UI 不做（README 已知限制），滚轮是唯一驱动源。

// clampInt 夹紧到 [lo, hi]（hi < lo 时取 lo）。
func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// axisScrollable 判断某轴在 overflow 语义下可滚：scroll/auto 仅在内容
// 确实溢出时可滚（visible/hidden 恒不可滚，hidden 只裁剪）。
func axisScrollable(o Overflow, content, box int) bool {
	switch o {
	case OverflowScroll, OverflowAuto:
		return content > box
	default:
		return false
	}
}

// scrollMax 返回元素两轴的最大滚动量（内容不溢出的轴为 0）。
// isRoot 为真时滚动盒取 (boxW, boxH)（视口），且不受 overflow 关键字
// 约束——文档级滚动在 html overflow:visible 下同样成立（浏览器语义）。
func scrollMax(el *htmlElement, boxW, boxH int, isRoot bool) (maxX, maxY int) {
	if el == nil || el.computed == nil {
		return 0, 0
	}
	if isAtomicFormControl(el) {
		// 原子控件的滚动由专用路径负责（textarea 按行数×行高，见
		// wheelScrollTextarea），常规流子尺寸不适用。
		return 0, 0
	}
	if isRoot {
		if el.contentW > boxW {
			maxX = el.contentW - boxW
		}
		if el.contentH > boxH {
			maxY = el.contentH - boxH
		}
		return maxX, maxY
	}
	comp := el.computed
	if axisScrollable(comp.OverflowX(), el.contentW, boxW) {
		maxX = el.contentW - boxW
	}
	if axisScrollable(comp.OverflowY(), el.contentH, boxH) {
		maxY = el.contentH - boxH
	}
	return maxX, maxY
}

// isClippingBox 判断元素绘制时是否需要裁剪（overflow 非 visible 即裁，
// hidden 只裁不滚）。根元素例外：文档级滚动由视口天然裁剪，自身不需要
// 额外裁剪盒。
func isClippingBox(el *htmlElement) bool {
	if el == nil || el.computed == nil {
		return false
	}
	return el.computed.OverflowX() != OverflowVisible ||
		el.computed.OverflowY() != OverflowVisible
}

// applyScrollOffsets 在布局完成后对滚动容器的子树施加滚动偏移，
// 并按当前内容/盒尺寸夹紧偏移量（内容变小后自动回弹到可滚范围）。
// 根元素以视口 (vw, vh) 为滚动盒，实现文档级页面滚动。
// 必须在 layoutPositioned 之后、layoutSelectPopups 之前调用：
// 前者产出的定位坐标一并平移，后者按平移后的 select 坐标定位浮层。
func applyScrollOffsets(g Graphics, root HTMLElement, vw, vh int) {
	var walk func(e HTMLElement, isRoot bool)
	walk = func(e HTMLElement, isRoot bool) {
		base := inner(e)
		if base == nil || base.computed == nil || base.computed.Display() == DisplayNone {
			return
		}
		if !isRoot && isAtomicFormControl(base) {
			// 原子控件（textarea/select）：滚动量由专用路径夹紧
			//（layoutTextarea 按行数×行高），内容不进常规流、无子树可平移。
			//若在此按 scrollMax（恒 0）夹紧会把光标跟随/滚轮滚出的
			// scrollTop 清零。
			return
		}
		boxW, boxH := base.width, base.height
		if isRoot {
			boxW, boxH = vw, vh
		}
		maxX, maxY := scrollMax(base, boxW, boxH, isRoot)
		ox := clampInt(base.scrollLeft, 0, maxX)
		oy := clampInt(base.scrollTop, 0, maxY)
		if ox != base.scrollLeft {
			base.scrollLeft = ox
		}
		if oy != base.scrollTop {
			base.scrollTop = oy
		}
		if ox != 0 || oy != 0 {
			// 平移全部子（含文本 run 与定位后代）：shiftBox 递归覆盖子树。
			for _, c := range base.children {
				shiftBox(c, -ox, -oy)
			}
		}
		for _, c := range base.children {
			walk(c, false)
		}
	}
	walk(root, true)
}

// scrollBy 按轴滚动一个普通容器，返回未能消费的剩余位移（0 = 全部消费）。
// 不可滚或已到边界时原样返回 delta，让事件继续向上传递。
func scrollBy(el *htmlElement, boxW, boxH int, axisX bool, delta int) int {
	if delta == 0 || el == nil {
		return delta
	}
	maxX, maxY := scrollMax(el, boxW, boxH, false)
	if axisX {
		if maxX <= 0 {
			return delta
		}
		cur := el.scrollLeft
		next := clampInt(cur+delta, 0, maxX)
		if next == cur {
			return delta
		}
		el.scrollLeft = next
		el.onChanged()
		return delta - (next - cur)
	}
	if maxY <= 0 {
		return delta
	}
	cur := el.scrollTop
	next := clampInt(cur+delta, 0, maxY)
	if next == cur {
		return delta
	}
	el.scrollTop = next
	el.onChanged()
	return delta - (next - cur)
}

// wheelScrollTextarea 滚动 textarea 的折行内容（纵向），返回剩余位移。
// 夹紧口径与 layoutTextarea 完全一致，保证下一轮布局不把它弹回。
func wheelScrollTextarea(doc HTMLDocument, el *htmlElement, delta int) int {
	if delta == 0 || el == nil {
		return delta
	}
	g := docGraphics(doc)
	l := layoutTextarea(g, el) // 内部已按行数×行高夹紧 el.scrollTop
	max := len(l.lines)*l.lh - el.height
	if max <= 0 {
		return delta
	}
	cur := el.scrollTop
	next := clampInt(cur+delta, 0, max)
	if next == cur {
		return delta
	}
	el.scrollTop = next
	el.onChanged()
	return delta - (next - cur)
}

// OnDocumentWheel 在文档上派发一次滚轮事件：坐标为视口坐标，
// dx/dy 为像素位移（dy > 0 = 向下滚，内容上移露出更下方内容，与浏览器
// WheelEvent.deltaY 同号）。返回是否有元素实际滚动（含 wheel 处理器被
// 触发时的 handled 由调用方另计——这里只回报滚动是否发生）。
//
// 流程：先沿祖先链冒泡 wheel 处理器（先于滚动，读到的是滚动前状态），
// 再从命中元素向上找第一个能消费位移的滚动目标（textarea / overflow
// 容器 / 文档），某轴滚到底继续尝试其祖先，最终落到根元素。
func OnDocumentWheel(doc HTMLDocument, x, y, dx, dy int) bool {
	if doc == nil {
		return false
	}
	target := ElementAt(doc, x, y)

	ev := &MouseEvent{Type: EventWheel, X: x, Y: y, DeltaX: dx, DeltaY: dy}
	for e := target; e != nil; e = e.ParentElement() {
		if base := inner(e); base != nil {
			base.dispatch(EventWheel, ev)
		}
	}

	remX, remY := dx, dy
	if remX == 0 && remY == 0 {
		return false
	}
	for e := target; e != nil && (remX != 0 || remY != 0); e = e.ParentElement() {
		base := inner(e)
		if base == nil || base.computed == nil {
			continue
		}
		// textarea 是原子盒：内容（value 折行）不进常规流，走专用滚动。
		// 纵向滚轮由 textarea 全量吸收（含滚到边界后的剩余位移），不做
		// scroll chaining——光标停在框上时页面跟着动会打断编辑（报告的
		// "滚 textarea 时 document 也滚"正是边界处剩余位移链给文档）。
		// 横向位移 textarea 无滚动能力，照常向祖先链传递。
		if strings.EqualFold(base.tagName, "textarea") && remY != 0 {
			wheelScrollTextarea(doc, base, remY)
			remY = 0
			continue
		}
		if isAtomicFormControl(base) {
			continue
		}
		if remX != 0 {
			remX = scrollBy(base, base.width, base.height, true, remX)
		}
		if remY != 0 {
			remY = scrollBy(base, base.width, base.height, false, remY)
		}
	}
	if remX == 0 && remY == 0 {
		return true
	}

	// 文档级滚动：根元素（html）以视口为滚动盒。
	root := inner(doc)
	if root == nil {
		return false
	}
	vw, vh := 0, 0
	if vp := doc.Viewport(); vp != nil {
		vw, vh = vp.ViewportWidth(), vp.ViewportHeight()
	} else {
		vw, vh = root.width, root.height
	}
	maxX, maxY := scrollMax(root, vw, vh, true)
	if remX != 0 && maxX > 0 {
		next := clampInt(root.scrollLeft+remX, 0, maxX)
		if next != root.scrollLeft {
			root.scrollLeft = next
			root.onChanged()
			remX = 0
		}
	}
	if remY != 0 && maxY > 0 {
		next := clampInt(root.scrollTop+remY, 0, maxY)
		if next != root.scrollTop {
			root.scrollTop = next
			root.onChanged()
			remY = 0
		}
	}
	return dx != remX || dy != remY
}
