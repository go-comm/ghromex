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
	n := len([]rune(b.GetAttribute("value")))
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

// SelectedText 返回聚焦输入当前选中的文本。第二个返回值为 false 表示
// 没有选区（或没有聚焦输入），后端据此决定是否写剪贴板——空选区不应
// 清空用户原有剪贴板。
//
// password 返回真实 value（浏览器允许选中并复制密码框内容），显示层
// 的 ● 掩码只在绘制/SVG 导出时生效。
func SelectedText(doc HTMLDocument) (string, bool) {
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
	r := []rune(b.GetAttribute("value"))
	return string(r[lo:hi]), true
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
		return false
	}
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
	}
	SetActiveElement(doc, nil)
}

// textPrefixWidth 返回 runes 前 n 个字符的像素宽（n 已由调用方夹紧）。
func textPrefixWidth(g Graphics, runes []rune, n, px, weightNum int, bold bool, family string) int {
	if n <= 0 {
		return 0
	}
	if n > len(runes) {
		n = len(runes)
	}
	w, _ := measureTextW(g, string(runes[:n]), px, weightNum, bold, family)
	return w
}

// inputSelectionRects 计算单行 input 的选区高亮矩形。val 是已经按
// inputVisibleWindow 舍头保尾后的可见文本，cut 是它在完整显示值中的
// 起始下标——高亮前缀宽必须与渲染文本同口径，否则超宽输入的选区会
// 整体偏移到被裁掉的前缀上。
func inputSelectionRects(g Graphics, val string, cut, tx, ty, th, px, weightNum int,
	bold bool, family string, lo, hi int) []clipRect {
	runes := []rune(val)
	from := lo - cut
	to := hi - cut
	if from < 0 {
		from = 0
	}
	if to > len(runes) {
		to = len(runes)
	}
	if from >= to {
		return nil
	}
	x1 := tx + textPrefixWidth(g, runes, from, px, weightNum, bold, family)
	x2 := tx + textPrefixWidth(g, runes, to, px, weightNum, bold, family)
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
	for i, ln := range l.lines {
		y := l.y + i*l.lh
		if clipOK {
			if y+l.lh <= b.y || y >= b.y+b.height {
				continue
			}
		} else if y < b.y || y+l.lh > b.y+b.height {
			continue
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
		runes := []rune(ln.text)
		x1 := l.x + textPrefixWidth(g, runes, from-ln.start, px, weightNum, bold, family)
		x2 := l.x + textPrefixWidth(g, runes, to-ln.start, px, weightNum, bold, family)
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
