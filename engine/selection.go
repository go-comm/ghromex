package engine

import "strings"

// 输入框选区（鼠标拖选 / Ctrl+A）与复制支持。
//
// 引擎只维护选区状态并提供 SelectedText，不直接接触系统剪贴板——剪贴板
// 属平台能力，由 SDL2 后端在 Ctrl+C 时调用 SDL_SetClipboardText 写入。
// 这样 engine 仍是纯 Go 且可在无显示/无 SDL 环境下测试。

// selectionColor 是选区高亮底色。半透明蓝在深色地址栏与浅色输入框上
// 都能看清，同时不改变文字颜色（引擎按整段文本一次绘制，逐字改色的
// 收益不抵复杂度）。
var selectionColor = NewColor(0x3B, 0x82, 0xF6, 0x82)

// selectionRange 返回元素的选区 [lo, hi)（rune 下标，已按 value 长度夹紧）。
// selAnchor 与 caret 相等表示无选区；拖选时活动端可在锚点任一侧，方向
// 不固定，所以统一按 min/max 归一化。
func selectionRange(b *htmlElement) (lo, hi int, ok bool) {
	if b == nil {
		return 0, 0, false
	}
	n := valueRuneLen(b)
	a, c := b.selAnchor, b.caret
	if a < 0 {
		a = 0
	} else if a > n {
		a = n
	}
	if c < 0 {
		c = 0
	} else if c > n {
		c = n
	}
	if a == c {
		return 0, 0, false
	}
	if a > c {
		a, c = c, a
	}
	return a, c, true
}

// valueText 返回 value 属性的 rune 区间 [lo,hi) 对应的文本（lo/hi 由调用方
// 夹紧）：textarea 布局后已有折行缓存，按行表把 rune 下标换算成字节下标
// 后切子串（不复制、也不必先 materialize 整篇 []rune——19.5MB 的值一次
// []rune 就是 80MB）；缓存不存在时退回旧口径 []rune 切片。
//
// 只要长度的调用方请改用 valueRuneLen（零分配）。
func valueText(b *htmlElement, lo, hi int) string {
	if w := b.wrap; w != nil {
		blo, bhi := w.byteIndexOf(lo), w.byteIndexOf(hi)
		if bhi <= blo {
			return ""
		}
		return w.value[blo:bhi]
	}
	r := []rune(b.GetAttribute("value"))
	if lo < 0 {
		lo = 0
	}
	if hi > len(r) {
		hi = len(r)
	}
	if hi <= lo {
		return ""
	}
	return string(r[lo:hi])
}

// SelectedText 返回当前选中的文本：优先返回文档级文本选区（普通页面
// 文本），其次返回聚焦输入内的选区。第二个返回值为 false 表示没有选区，
// 后端据此决定是否写剪贴板——空选区不应清空用户原有剪贴板。
//
// password 返回真实 value（浏览器允许选中并复制密码框内容），显示层
// 的 ● 掩码只在绘制/SVG 导出时生效。
func SelectedText(doc HTMLDocument) (string, bool) {
	if d, ok := doc.(*htmlDocument); ok {
		if text, ok := flowSelectedText(d); ok {
			return text, true
		}
	}
	f := FocusedElement(doc)
	if f == nil {
		return "", false
	}
	b := inner(f)
	if b == nil || !editableInput(f) {
		// 焦点不在可编辑元素上：没有选区可言。
		return "", false
	}
	lo, hi, ok := selectionRange(b)
	if !ok {
		return "", false
	}
	return valueText(b, lo, hi), true
}

// SelectionLength 返回当前选区的 rune 数，第二个返回值为 false 表示没有
// 选区。文档级文本选区优先（与 SelectedText 同口径）。
//
// 与 SelectedText 相比它不复制选区文本：状态栏这类每帧都要刷新、只关心
// 「选中多少字」的场景用它，避免为一句提示复制整个选区（全选一份 19.5MB
// 的文档就是 19.5MB 的临时分配）。
func SelectionLength(doc HTMLDocument) (int, bool) {
	if d, ok := doc.(*htmlDocument); ok {
		if n, ok := flowSelectedLength(d); ok {
			return n, true
		}
	}
	f := FocusedElement(doc)
	if f == nil {
		return 0, false
	}
	b := inner(f)
	if b == nil || !editableInput(f) {
		return 0, false
	}
	lo, hi, ok := selectionRange(b)
	if !ok {
		return 0, false
	}
	return hi - lo, true
}

// controlCaretFromPoint 把坐标反推为控件光标（rune 下标），不修改状态。
// textarea 走折行布局的行/列映射，单行 input 走可见窗口的前缀宽度映射。
func controlCaretFromPoint(doc HTMLDocument, b *htmlElement, x, y int) int {
	g := docGraphics(doc)
	px, weightNum, bold, family := fontOf(b.computed)
	if strings.EqualFold(b.tagName, "textarea") {
		return layoutTextarea(g, b).caretFromPoint(g, x, y, px, weightNum, bold, family)
	}
	return controlLayout(doc, b).caretFromPoint(g, x, y, px, weightNum, bold, family)
}

// OnDocumentMouseDown 在鼠标按下时准备潜在拖选：命中可编辑输入则聚焦、
// 按按下点落光标，并把锚点定在同一位置。返回是否落在可编辑输入上——
// 后端据此决定后续 MouseMotion 是否扩展选区，以及抬起时是否跳过 click
// （拖选不是点击，不应触发链接/按钮的默认动作）。
//
// 真正的选区在 MouseMotion 中才展开：按下即产生选区会让"单击定位光标"
// 也走选区路径，和浏览器行为不一致。
func OnDocumentMouseDown(doc HTMLDocument, x, y int) bool {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return false
	}
	// 按下即视为要操作：收起提示气泡（浏览器同行为，收起后不换目标不追回）。
	d.dismissTooltip()
	// 文档级覆盖滚动条优先于内容命中：滚动条只管滚动——不聚焦、不落光标、
	// 不进入拖选，按下即跳到点击位置，随后拖动跟随指针（见
	// viewportBarScrollTo）。返回 true 让后端进入拖动通道、抬起时跳过
	// click（结构同 textarea 滚动条）。
	vw, vh := d.viewportSize()
	if root := inner(doc); root != nil {
		if axisX, hit := viewportBarHit(root, vw, vh, x, y); hit {
			if axisX {
				d.dragViewAxis = -1
			} else {
				d.dragViewAxis = 1
			}
			viewportBarScrollTo(d, axisX, x, y)
			return true
		}
	}
	target := ElementAt(doc, x, y)
	// 按下态（:active）：命中什么就按在什么上（命中空白则无按下态）。
	// 状态变化要改样式，故走 SetActiveElement 的变更通知。
	SetActiveElement(doc, target)
	var next HTMLElement
	for e := target; e != nil; e = e.ParentElement() {
		if editableInput(e) {
			next = e
			break
		}
	}
	if next == nil {
		// 命中普通页面文本：进入文档级拖选准备态。锚点先落在按下点，选区
		// 在 MouseMotion 中才展开（与可编辑控件同语义：单击不产生选区）。
		pos, ok := flowPositionAt(doc, x, y, false)
		d.flowSelecting = false
		clearFlowSelection(d)
		if !ok {
			return false
		}
		// 浏览器点击页面文本会让输入框失焦：焦点留着的话后续打字还会
		// 改输入框，输入框选区高亮也会和页面选区同时出现。
		if d.focus != nil {
			SetFocusedElement(doc, nil)
		}
		d.flowAnchor, d.flowFocus = pos, pos
		d.flowSelecting = true
		return true
	}
	// 命中可编辑输入：输入框选区接管，先清掉文档级选区（两套高亮不能同时亮）。
	d.flowSelecting = false
	clearFlowSelection(d)
	b := inner(next)
	// 按在 textarea 覆盖滚动条上：只滚动——不移动光标、不改变焦点，
	// 按下即跳到点击位置，随后拖动跟随指针（OnDocumentMouseDrag）。
	if textareaHitScrollbar(docGraphics(doc), b, x, y) {
		d.dragScroll = b
		textareaScrollTo(doc, b, y)
		return true
	}
	// 可编辑控件先按「指针此刻看到的视口」算好目标光标，再改焦点：
	// 首次聚焦会把光标落到值末尾并滚动跟随（Tab 聚焦的合理行为），若
	// 聚焦之后才映射，点击点会按跳走后的视口反推出完全错误的行。
	var caretHint int
	clickCaret := b != nil && editableInput(next)
	if clickCaret {
		caretHint = controlCaretFromPoint(doc, b, x, y)
	}
	SetFocusedElement(doc, next)
	if b == nil {
		return false
	}
	if clickCaret {
		setSelection(doc, b, caretHint, caretHint)
	}
	b.selAnchor = b.caret
	d.dragInput = b
	return true
}

// OnDocumentMouseDrag 把拖选活动端移到指针位置，锚点保持不变。指针移出
// 控件后仍按原控件布局反推光标（并自然夹到行首/行尾/首末行），因此
// 可以拖到输入框外继续扩选。按住 textarea 覆盖滚动条时改为滚动。
func OnDocumentMouseDrag(doc HTMLDocument, x, y int) bool {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return false
	}
	if d.dragViewAxis != 0 {
		viewportBarScrollTo(d, d.dragViewAxis < 0, x, y)
		return true
	}
	if d.dragScroll != nil {
		textareaScrollTo(doc, d.dragScroll, y)
		return true
	}
	if d.dragInput == nil {
		if d.flowSelecting {
			if pos, ok := flowPositionAt(doc, x, y, true); ok {
				setFlowSelection(d, d.flowAnchor, pos)
			}
			// 指针即使暂时不在文本上（划过图片/空白）也保持拖选通道，
			// 后续移动仍能继续扩展选区。
			return true
		}
		return false
	}
	b := d.dragInput
	c := controlCaretFromPoint(doc, b, x, y)
	setSelection(doc, b, b.selAnchor, c)
	return true
}

// OnDocumentMouseUp 结束拖选。选区本身保留（浏览器同语义：松开鼠标后
// 高亮仍在，直到再次点击或按键）。
//
// 同时清掉按下态（:active）——鼠标已抬起。
func OnDocumentMouseUp(doc HTMLDocument) {
	if d, ok := doc.(*htmlDocument); ok {
		d.dragInput = nil
		d.dragScroll = nil
		d.dragViewAxis = 0
		d.flowSelecting = false
	}
	SetActiveElement(doc, nil)
}

// prefixWidth 返回一段前缀文本的像素宽（文本由调用方从 value 子串切好）。
func prefixWidth(g Graphics, s string, px, weightNum int, bold bool, family string) int {
	if s == "" {
		return 0
	}
	w, _ := measureTextW(g, s, px, weightNum, bold, family)
	return w
}

// inputSelectionRects 计算单行 input 的选区高亮矩形。val 是已经按
// inputVisibleWindow 舍头保尾后的可见文本，cut 是它在完整显示值中的
// 起始下标——高亮前缀宽必须与渲染文本同口径，否则超宽输入的选区会
// 整体偏移到被裁掉的前缀上。
func inputSelectionRects(g Graphics, val string, cut, tx, ty, th, px, weightNum int,
	bold bool, family string, lo, hi int) []clipRect {
	from := lo - cut
	to := hi - cut
	if from < 0 {
		from = 0
	}
	if n := runeLen(val); to > n {
		to = n
	}
	if from >= to {
		return nil
	}
	// 前缀按 rune 边界切字节再量（不 materialize []rune[val]）。
	x1 := tx + prefixWidth(g, val[:runeByteIndex(val, from)], px, weightNum, bold, family)
	x2 := tx + prefixWidth(g, val[:runeByteIndex(val, to)], px, weightNum, bold, family)
	if x2 <= x1 {
		return nil
	}
	return []clipRect{{x: x1, y: ty, w: x2 - x1, h: th}}
}

// textareaSelectionRects 计算 textarea 各行的选区高亮矩形。选区可跨行；
// 每行按 [start,end) 求交，选中换行符时把高亮延到行尾（hi > end）。
// 空行被选中时画一个窄块，否则“整段选中但空行没有高亮”会显得断裂。
// clipOK 与渲染路径同口径：后端支持裁剪时只丢弃与盒子完全不相交的行，
// 半截行交给裁剪；不支持裁剪时整行丢弃，避免高亮画到盒外。
func textareaSelectionRects(g Graphics, b *htmlElement, l textareaLayout,
	px, weightNum int, bold bool, family string, clipOK bool) []clipRect {
	lo, hi, ok := selectionRange(b)
	if !ok {
		return nil
	}
	var out []clipRect
	first, last := l.visibleRows(b.y, b.height)
	for i := first; i <= last; i++ {
		ln := l.lines[i]
		y := l.y + i*l.lh
		if !clipOK && (y < b.y || y+l.lh > b.y+b.height) {
			continue // 无裁剪原语：盒外整行丢弃，避免高亮画到盒外
		}
		// 行文本为 [start,end)，换行符位于 end。以下覆盖三种交集：
		// 选区在行首之前/行内到行尾/空行被整行选中。
		if hi <= ln.start {
			continue
		}
		if lo >= ln.end && ln.end > ln.start {
			continue
		}
		if ln.start == ln.end && !(lo <= ln.start && hi > ln.start) {
			continue
		}
		from := lo
		if from < ln.start {
			from = ln.start
		}
		to := hi
		if to > ln.end {
			to = ln.end
		}
		// 前缀宽与渲染同源：都从 value 的子串上量，不再 materialize 行 rune。
		x1 := l.x + prefixWidth(g, l.linePrefix(ln, from-ln.start), px, weightNum, bold, family)
		x2 := l.x + prefixWidth(g, l.linePrefix(ln, to-ln.start), px, weightNum, bold, family)
		if hi > ln.end {
			x2 = l.x + ln.w // 选区包含本行行尾（换行符/折行点）
		}
		w := x2 - x1
		if w <= 0 {
			// 空行或只选中换行符：给一个最小可见宽度
			w = px / 2
			if w < 2 {
				w = 2
			}
		}
		out = append(out, clipRect{x: x1, y: y, w: w, h: l.lh})
	}
	return out
}
