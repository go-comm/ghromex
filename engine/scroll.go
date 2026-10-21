package engine

import (
	"math"
	"strings"
	"time"
)

// 滚动：overflow 滚动容器 + 文档级滚动的偏移施加与滚轮分发。
//
// 模型（与浏览器对齐的简化版）：
//   - 每个元素的滚动偏移存放在 htmlElement.scrollTop/scrollLeft 上，
//     由 applyScrollOffsets 在「常规流 + 定位」布局完成后统一夹紧并
//     平移其子树（布局每轮从头重算，偏移不会跨轮累积）；
//   - 根元素的滚动盒取视口（而非自身盒高），从而长页面可整页滚动；
//   - 滚轮从命中元素沿祖先链向上传递，对应轴滚到底才继续向上传
//     （scroll chaining），最后落到文档级滚动；textarea 例外：纵向
//     滚轮全量吸收（含撞边界后的剩余位移），光标停在框上时页面不动；
//   - 文档级滚动是**动画式**的（Chrome 同款）：滚轮只改目标，视觉位置由
//     弹簧（位置+速度）逐帧推进，见 advanceScrollAnim/startScrollAnim。
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

// scrollVisualOffset 返回容器当前应显示的偏移：动画在途取视觉位置
// （scrollAnimTop/Left），否则取滚动目标（scrollTop/Left）。
func scrollVisualOffset(base *htmlElement) (int, int) {
	if base == nil {
		return 0, 0
	}
	if base.scrollAnimOn {
		return base.scrollAnimLeft, base.scrollAnimTop
	}
	return base.scrollLeft, base.scrollTop
}

// applyScrollOffsets 在布局完成后对滚动容器的子树施加滚动偏移，
// 并按当前内容/盒尺寸夹紧偏移量（内容变小后自动回弹到可滚范围）。
// 根元素以视口 (vw, vh) 为滚动盒，实现文档级页面滚动。
// 必须在 layoutPositioned 之后、layoutSelectPopups 之前调用：
// 前者产出的定位坐标一并平移，后者按平移后的 select 坐标定位浮层。
//
// 这里按**绝对**偏移平移并记录已施加量（scrollShift*）；动画逐帧推进时
// 由 applyScrollDeltas 按增量平移，不必重排。
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
		// 夹紧写回的是**目标**（scrollLeft/scrollTop）；动画的视觉位置另算，
		// 两者不可混用（混用会把目标覆盖成中间值，动画随即停住）。
		if cx := clampInt(base.scrollLeft, 0, maxX); cx != base.scrollLeft {
			base.scrollLeft = cx
		}
		if cy := clampInt(base.scrollTop, 0, maxY); cy != base.scrollTop {
			base.scrollTop = cy
		}
		ox, oy := scrollVisualOffset(base)
		ox = clampInt(ox, 0, maxX)
		oy = clampInt(oy, 0, maxY)
		// 注意：此处**不再**把 ox/oy 写回 scrollLeft/scrollTop——那是滚动
		// **目标**（夹紧在上面已完成），写回视觉位置会让目标被中间值覆盖、
		// 动画随即停住。
		if ox != 0 || oy != 0 {
			// 平移全部子（含文本 run 与定位后代）：shiftBox 递归覆盖子树。
			for _, c := range base.children {
				shiftBox(c, -ox, -oy)
			}
		}
		base.scrollShiftLeft, base.scrollShiftTop = ox, oy
		for _, c := range base.children {
			walk(c, false)
		}
	}
	walk(root, true)
}

// applyScrollDeltas 把滚动视觉位置的**增量**平移到已布局的盒子上（不重排）。
//
// 滚动偏移只是布局之后的一次平移，与盒模型无关：动画逐帧推进时重跑全量
// 级联+布局是纯浪费（portal 页一帧 5ms+，滚动肉眼可见地发顿）。这里只
// 按"新视觉位置 - 已施加量"平移，返回是否有内容移动。
func applyScrollDeltas(g Graphics, root HTMLElement, vw, vh int) bool {
	moved := false
	var walk func(e HTMLElement, isRoot bool)
	walk = func(e HTMLElement, isRoot bool) {
		base := inner(e)
		if base == nil || base.computed == nil || base.computed.Display() == DisplayNone {
			return
		}
		if !isRoot && isAtomicFormControl(base) {
			return // 原子控件无子树可平移（见 applyScrollOffsets）
		}
		boxW, boxH := base.width, base.height
		if isRoot {
			boxW, boxH = vw, vh
		}
		maxX, maxY := scrollMax(base, boxW, boxH, isRoot)
		ox, oy := scrollVisualOffset(base)
		ox = clampInt(ox, 0, maxX)
		oy = clampInt(oy, 0, maxY)
		dx := ox - base.scrollShiftLeft
		dy := oy - base.scrollShiftTop
		if dx != 0 || dy != 0 {
			for _, c := range base.children {
				shiftBox(c, -dx, -dy)
			}
			base.scrollShiftLeft, base.scrollShiftTop = ox, oy
			moved = true
		}
		for _, c := range base.children {
			walk(c, false)
		}
	}
	walk(root, true)
	return moved
}

// TickScroll 推进文档级滚动动画一帧，并把视觉位置的增量平移到已布局的
// 盒子上。返回是否有内容移动（调用方需要重绘，但**不必重排**）。
//
// 动画按真实时间推进（见 scrollAnimDuration），故帧率波动不会拖慢滚动
// 速度；每帧只做一次平移，代价与页面元素数线性相关、与样式表规模无关。
//
// 引擎不自带帧循环：后端每帧调一次本函数即可（backend/sdl2 的事件循环
// 已接上）。不调则动画停在起点（滚动目标仍然生效，只是不做补间）。
func TickScroll(doc HTMLDocument) bool {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return false
	}
	base := inner(d)
	if base == nil || !base.scrollAnimOn {
		return false
	}
	vw, vh := 0, 0
	if v := doc.Viewport(); v != nil {
		vw, vh = v.ViewportWidth(), v.ViewportHeight()
	}
	g := docGraphics(doc)
	advanceScrollAnim(base)
	moved := applyScrollDeltas(g, doc, vw, vh)
	// select 浮层按平移后的坐标定位：滚动时展开的浮层要跟着走。
	layoutSelectPopups(g, base, vw, vh)
	return moved
}

// ScrollAnimating 报告文档里是否有滚动动画在途（调用方据此不休眠，
// 否则动画会被 10ms 的空闲休眠切成一段一段的台阶）。
func ScrollAnimating(doc HTMLDocument) bool {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return false
	}
	base := inner(d)
	return base != nil && base.scrollAnimOn
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

// 文档级平滑滚动的时基参数。
//
// 用**临界阻尼弹簧**（位置 + 速度）而不是固定时长的缓动曲线：固定曲线
// （此前的 easeOutCubic）在连续滚动时每档都从静止重新起步，速度在档与档
// 之间跳变——实测每档的位移是 35/26/17/12/6/3/1/0px 的锯齿，滚起来一顿
// 一顿。弹簧只换目标、保留速度，运动是连续的（Chrome 的滚动动画就是
// 连续重定向的），收尾也自然。
const (
	// scrollAnimOmega 是弹簧的固有角频率（1/s）：临界阻尼下位置偏差按
	// e^{-ωt} 衰减，约 4/ω 收敛。取 55 → 约 70~110ms 收敛，与 Chrome
	// 一格滚轮（≈100~150ms）同量级。
	scrollAnimOmega = 55.0
	// scrollAnimMinFrame 是连续布局（测试里没有 sleep，墙钟间隔≈0）时的
	// 名义帧步长：保证紧密循环按 60fps 节奏推进并在有限帧内收敛，不依赖
	// 墙钟（引擎布局要可重复）。真实运行时 dt 通常更大，取两者较大值。
	scrollAnimMinFrame = 16 * time.Millisecond
	// scrollAnimStepMax 是弹簧积分的最大子步长：显式积分在 ω·h 接近 2 时
	// 临界（会发散），按 4ms 切分保证稳定。
	scrollAnimStepMax = 4 * time.Millisecond
	// scrollAnimOmega2 是 ω²（积分时要反复用）。
	scrollAnimOmega2 = scrollAnimOmega * scrollAnimOmega
)

// advanceScrollAnim 把文档级滚动的视觉位置向目标推进一帧（弹簧积分）。
//
// 动画按真实时间推进（见 scrollAnimMinFrame），故帧率波动不会拖慢滚动
// 速度；每帧只做一次平移，代价与页面元素数线性相关、与样式表规模无关。
//
// 引擎不自带帧循环：后端每帧调一次本函数即可（browser 的主循环已接上）。
// 不调则动画停在起点（滚动目标仍然生效，只是不做补间）。
func advanceScrollAnim(el *htmlElement) {
	now := time.Now().UnixNano()
	dt := now - el.scrollAnimLastNs
	el.scrollAnimLastNs = now
	if min := int64(scrollAnimMinFrame); dt < min {
		dt = min
	}
	// 半隐式欧拉积分：子步 ≤ 4ms，保证 ω·h 足够小不发散。
	h := float64(dt) / float64(time.Second)
	steps := int(h/scrollAnimStepMax.Seconds()) + 1
	sh := h / float64(steps)
	for i := 0; i < steps; i++ {
		el.scrollAnimVelTop += (scrollAnimOmega2*(float64(el.scrollTop)-el.scrollAnimPosTop) -
			2*scrollAnimOmega*el.scrollAnimVelTop) * sh
		el.scrollAnimPosTop += el.scrollAnimVelTop * sh
		el.scrollAnimVelLeft += (scrollAnimOmega2*(float64(el.scrollLeft)-el.scrollAnimPosLeft) -
			2*scrollAnimOmega*el.scrollAnimVelLeft) * sh
		el.scrollAnimPosLeft += el.scrollAnimVelLeft * sh
	}
	// 收敛：位置贴到目标（差 < 0.5px）就吸附并结束该轴的动画。临界阻尼
	// 不会过冲，位置是单调逼近的，这一步就是收尾。
	topDone := math.Abs(float64(el.scrollTop)-el.scrollAnimPosTop) < 0.5
	leftDone := math.Abs(float64(el.scrollLeft)-el.scrollAnimPosLeft) < 0.5
	if topDone {
		el.scrollAnimPosTop = float64(el.scrollTop)
		el.scrollAnimVelTop = 0
	}
	if leftDone {
		el.scrollAnimPosLeft = float64(el.scrollLeft)
		el.scrollAnimVelLeft = 0
	}
	el.scrollAnimOn = !(topDone && leftDone)
	// 视觉位置取整（像素网格）：布局按整像素平移子树。
	el.scrollAnimTop = int(math.Round(el.scrollAnimPosTop))
	el.scrollAnimLeft = int(math.Round(el.scrollAnimPosLeft))
}

// startScrollAnim 启动（或重定向）文档级平滑滚动动画：目标由调用方随后
// 写入 scrollTop/scrollLeft。
//
// 首次启动：位置 = 当前视觉位置、速度 = 0（弹簧自己加速再减速到目标）。
//
// 重定向（滚动途中又来一档滚轮）：**只换目标，速度保留**——连续滚动于是
// 是一段连贯的滑动；每档都从静止重新起步的话，滚起来是一顿一顿的。
func startScrollAnim(el *htmlElement) {
	if el == nil {
		return
	}
	if !el.scrollAnimOn {
		el.scrollAnimPosTop = float64(el.scrollAnimTop)
		el.scrollAnimPosLeft = float64(el.scrollAnimLeft)
		el.scrollAnimVelTop = 0
		el.scrollAnimVelLeft = 0
	}
	el.scrollAnimLastNs = time.Now().UnixNano()
	el.scrollAnimOn = true
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
			startScrollAnim(root) // 平滑滚动：视觉位置从当前值起算
			root.scrollLeft = next
			root.onChanged()
			remX = 0
		}
	}
	if remY != 0 && maxY > 0 {
		next := clampInt(root.scrollTop+remY, 0, maxY)
		if next != root.scrollTop {
			startScrollAnim(root)
			root.scrollTop = next
			root.onChanged()
			remY = 0
		}
	}
	return dx != remX || dy != remY
}
