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
// textarea 提供覆盖式滚动条 UI（绘制+拖拽见 textarea.go 的
// textareaScrollbar）；文档级滚动也有覆盖式滚动条（视口右缘/下缘，绘制+
// 命中+拖拽见 viewport_scrollbar.go，拖拽立即到位不缓动；带域可按宿主
// 固定栏收窄、横条可关）；通用容器
// （overflow 滚动）仍无滚动条，滚轮是唯一驱动源，也仍无 PageUp/PageDown
// 翻页与触控板惯性。

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
		el.onScrollChanged()
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
	el.onScrollChanged()
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
	el.onScrollChanged()
	return delta - (next - cur)
}

// 文档级平滑滚动的时基参数。
//
// 模型是**基于时间的缓动 + 目标位置累加**（Chrome 同款）：
//   - 滚轮只改目标（scrollTop/scrollLeft，目标累加），不直接改视觉位置；
//   - 每帧按「已推进的动画时间 / 总时长」求进度 t∈[0,1]，视觉位置 =
//     起点 + (目标 − 起点) × easeOutQuad(t)；
//   - 动画途中再来一档滚轮：目标继续累加，缓动段从**当前视觉位置**重新
//     起算 → 位置连续、不跳。
//
// 为什么不用「固定帧数每帧加 N px」：那样在高刷屏上滚得太快、掉帧时又
// 变速（60Hz 下一档 100px 是 9 帧，240Hz 下同样的 9 帧就变成 37ms 滚完）。
// 为什么不用固定时长的曲线 + 每档从静止起步：每档都从静止重新起步，速度
// 在档与档之间跳变（实测每档位移是 35/26/17/12/6/3/1/0px 的锯齿），滚起来
// 一顿一顿。
const (
	// scrollAnimDuration 是一档滚轮（100px）的动画总时长。
	//
	// 取 150ms：与 Chrome 的平滑滚动同量级（Chrome 一档滚轮约 150ms 到位）。
	// 60Hz 下约 9 帧走完 100px——**均速** 11px/帧，但缓动是「起步快、收尾慢」：
	// 首帧约 21px、末帧约 1px（见 scrollAnimEase）。
	//
	// 时长用**墙钟时间**算（不是帧计数）：高刷屏上不会滚得太快，掉帧时也
	// 不会变速。
	scrollAnimDuration = 150 * time.Millisecond
	// scrollAnimFrame 是单帧能推进的**动画时间**上限。
	//
	// 动画按真实时间推进，遇到卡顿的一帧（新内容进视口、要现建纹理/字模，
	// 帧时间可能上百毫秒）就会「跳过」一大段距离——内容一格一格地跳过去，
	// 看起来就是滚动时一闪一闪。单帧推进量限制在一帧内：慢帧时滚动慢一点
	//（比跳一大段更顺），但运动始终连续。
	//
	// 取 20ms：60Hz 一帧 16.7ms 不受限制 → 时长精确 150ms；卡顿的一帧最多
	// 推进 20ms 的进度（约 1/8 段），不会一帧跳到位。
	scrollAnimFrame = 20 * time.Millisecond
)

// advanceScrollAnim 把文档级滚动的视觉位置向目标推进一帧。
//
// 进度按**已推进的动画时间**算（墙钟时间，不是帧计数），但单帧推进量有
// 上限（scrollAnimFrame）：卡顿的一帧不会让内容跳过一整段距离。每帧只做
// 一次平移，代价与页面元素数线性相关、与样式表规模无关。
//
// 引擎不自带帧循环：后端每帧调一次本函数即可（browser 的主循环已接上）。
// 不调则动画停在起点（滚动目标仍然生效，只是不做补间）。
func advanceScrollAnim(el *htmlElement) {
	now := time.Now().UnixNano()
	// 两轴**各自**算帧间隔：wheel 事件只重置对应轴的时间戳（见
	// startScrollAnim），共用一份的话，高频 wheel 会把另一轴的帧间隔
	// 截断（触控板 momentum 时 wheel 比帧还密 → 另一轴的动画几乎停住）。
	dtTop := now - el.scrollAnimLastTopNs
	dtLeft := now - el.scrollAnimLastLeftNs
	el.scrollAnimLastTopNs, el.scrollAnimLastLeftNs = now, now
	if max := int64(scrollAnimFrame); dtTop > max {
		dtTop = max // 卡顿的一帧不推进太多：运动保持连续（见 scrollAnimFrame）
	}
	if max := int64(scrollAnimFrame); dtLeft > max {
		dtLeft = max
	}
	if dtTop < 0 {
		dtTop = 0
	}
	if dtLeft < 0 {
		dtLeft = 0
	}
	// 两轴各自推进「动画时间」：进度只看时间，不看帧数。走满时长后停加
	// （进度已封顶为 1，再加只是让数值无意义地变大）。
	if el.scrollAnimElapsedTopNs < int64(scrollAnimDuration) {
		el.scrollAnimElapsedTopNs += dtTop
	}
	if el.scrollAnimElapsedLeftNs < int64(scrollAnimDuration) {
		el.scrollAnimElapsedLeftNs += dtLeft
	}
	tTop := scrollAnimProgress(el.scrollAnimElapsedTopNs)
	tLeft := scrollAnimProgress(el.scrollAnimElapsedLeftNs)

	// 缓动真值：起点 + (目标 − 起点) × eased(t)。
	posTop := el.scrollAnimBaseTop + (float64(el.scrollTop)-el.scrollAnimBaseTop)*scrollAnimEase(tTop)
	posLeft := el.scrollAnimBaseLeft + (float64(el.scrollLeft)-el.scrollAnimBaseLeft)*scrollAnimEase(tLeft)

	// 可见位置：把这一帧的**位移**取整后累加，而不是对累积位置取整。
	// 后者在速度变化时会在 floor/ceil 之间来回跳（实测 8→7→8、3→2→3），
	// 看着像抖；缓动曲线单调，位移取整后仍单调。
	el.scrollAnimTop = scrollAnimVisible(el.scrollAnimTop, posTop-el.scrollAnimPosTop, el.scrollTop, posTop)
	el.scrollAnimLeft = scrollAnimVisible(el.scrollAnimLeft, posLeft-el.scrollAnimPosLeft, el.scrollLeft, posLeft)
	el.scrollAnimPosTop, el.scrollAnimPosLeft = posTop, posLeft

	// 收尾：动画时间走满 → 精确贴到目标（量化误差在这一帧补齐），动画结束。
	if tTop >= 1 {
		el.scrollAnimPosTop = float64(el.scrollTop)
		el.scrollAnimTop = el.scrollTop
	}
	if tLeft >= 1 {
		el.scrollAnimPosLeft = float64(el.scrollLeft)
		el.scrollAnimLeft = el.scrollLeft
	}
	el.scrollAnimOn = tTop < 1 || tLeft < 1
}

// scrollAnimEase 是滚动动画的缓动函数（easeOutQuad）：起步快、收尾慢。
//
// 不用 easeOutCubic：它第一帧就走掉 30%（100px 里 30px），看着像被弹出去；
// easeOutQuad 第一帧 21%，更贴手。也不用线性：线性一眼就看出是「假动画」。
//
// 150ms / 9 帧下各帧的累计位移：21.0、39.5、55.6、69.1、80.2、88.9、
// 95.1、98.8、100（每帧增量 21、19、16、13、11、9、6、4、1，单调下降）。
func scrollAnimEase(t float64) float64 {
	return 1 - (1-t)*(1-t)
}

// scrollAnimProgress 把已推进的动画时间换算成进度 t∈[0,1]。
func scrollAnimProgress(elapsedNs int64) float64 {
	t := float64(elapsedNs) / float64(scrollAnimDuration)
	if t > 1 {
		t = 1
	}
	return t
}

// scrollAnimVisible 把这一帧的位移量化成可见的整像素位移。
//
// 位移取整（而不是对累积位置取整）：速度变化时后者会在 floor/ceil 之间
// 来回跳（实测 8→7→8、3→2→3），看着像抖；缓动曲线单调，位移取整后仍单调。
//
// 收尾保底 1px：真值已接近目标（或可见位置已只差 1~2px）却因本帧位移
// 不足 0.5px 而取整成 0 时，仍要再走 1px——否则内容停在离目标几像素的
// 地方不动（"到位了还在磨蹭"）。方向由 **target − vis** 决定，向上滚
// 同样是 -1。
//
// 真值侧的阈值取 8px 而不是 2px：高帧率下每帧位移不足 1px，取整误差会
// 同号累积（实测 1000fps 可累积到 8px），尾段要多留几帧才吃得完，否则
// 收尾要一次性跳几像素。
//
// 量化误差不能把内容推到目标之外（那就要再跳回来）；越界时直接贴到目标。
func scrollAnimVisible(vis int, delta float64, target int, truth float64) int {
	step := int(math.Round(delta))
	if step == 0 && vis != target {
		if math.Abs(float64(target)-truth) <= 8 || math.Abs(float64(target)-float64(vis)) <= 2 {
			step = 1
			if target < vis {
				step = -1
			}
		}
	}
	next := vis + step
	if next > target && target >= vis {
		next = target
	}
	if next < target && target <= vis {
		next = target
	}
	return next
}

// startScrollAnim 启动（或重定向）文档级平滑滚动动画：目标由调用方写入
// scrollTop/scrollLeft（滚轮累加，见 OnDocumentWheel）。
//
// 缓动段从**当前视觉位置**起算：位置连续，不会跳。
//
// 连续滚动（动画途中又来一档滚轮）：目标继续累加，缓动段从当前视觉位置
// 重新起算——运动是连贯的一段滑动；每档都从静止重新起步的话，滚起来是
// 一顿一顿的。
//
// 两轴各自计时（vertical 决定重置哪一轴）：只滚纵向时横向的动画不能被
// 重新计时，否则横向会「跳回起点」再重新走一遍；横向的**帧间隔**也不能
// 被纵向的 wheel 截断（时间戳同样分轴，见 advanceScrollAnim）。
//
// 不变量：起点取的就是「上一帧的真值」（scrollAnimPosTop/PosLeft），
// 因此本帧的位移仍按真值差算——重定向不会让某一帧多走或少走一段。
func startScrollAnim(el *htmlElement, vertical bool) {
	if el == nil {
		return
	}
	now := time.Now().UnixNano()
	if vertical {
		el.scrollAnimBaseTop = el.scrollAnimPosTop
		el.scrollAnimElapsedTopNs = 0
		el.scrollAnimLastTopNs = now
	} else {
		el.scrollAnimBaseLeft = el.scrollAnimPosLeft
		el.scrollAnimElapsedLeftNs = 0
		el.scrollAnimLastLeftNs = now
	}
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
		// 元素链吃下了全部位移：确有内容移动，气泡锚点已经指错位置。
		dismissTooltip(doc)
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
			// 目标累加（连续滚轮接着上一档走），缓动段从当前视觉位置起算。
			root.scrollLeft = next
			startScrollAnim(root, false) // 平滑滚动：横向
			root.onScrollChanged()
			remX = 0
		}
	}
	if remY != 0 && maxY > 0 {
		next := clampInt(root.scrollTop+remY, 0, maxY)
		if next != root.scrollTop {
			root.scrollTop = next
			startScrollAnim(root, true) // 平滑滚动：纵向
			root.onScrollChanged()
			remY = 0
		}
	}
	moved := dx != remX || dy != remY
	if moved {
		dismissTooltip(doc) // 内容移动了：锚在旧位置的气泡收起（浏览器同行为）
	}
	return moved
}
