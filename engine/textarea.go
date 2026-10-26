package engine

import "strings"

// textareaScrollbarWidth 是 textarea 覆盖式滚动条的宽度（px）：贴内容盒
// 右缘绘制，叠在文本之上，不占布局宽度（折行口径不变）。
const textareaScrollbarWidth = 8

// scrollbarTrack/scrollbarThumb 是覆盖滚动条的轨道与滑块配色（半透明黑，
// 在 textarea 白底与选区高亮上都能看清）。
var (
	scrollbarTrack = NewColor(0x00, 0x00, 0x00, 0x14)
	scrollbarThumb = NewColor(0x00, 0x00, 0x00, 0x5C)
)

// textareaScrollbar 返回 textarea 覆盖滚动条的几何：轨道 (tx,ty,tw,th)、
// 滑块 (tx,thumbY,tw,thumbH)；ok=false 表示内容未溢出、没有滚动条。
// 绘制与命中/拖拽共用本函数，保证「看到的就是能点的」。
func textareaScrollbar(g Graphics, b *htmlElement) (tx, ty, tw, th, thumbY, thumbH int, ok bool) {
	l := layoutTextarea(g, b)
	contentH := len(l.lines) * l.lh
	if contentH <= b.height {
		return 0, 0, 0, 0, 0, 0, false
	}
	tw = textareaScrollbarWidth
	tx = b.x + b.width - tw
	ty = b.y
	th = b.height
	thumbH = th * b.height / contentH
	if thumbH < 24 {
		thumbH = 24 // 最小可抓握高度
	}
	if thumbH > th {
		thumbH = th
	}
	maxScroll := contentH - b.height
	if maxScroll < 1 {
		maxScroll = 1
	}
	if th > thumbH {
		thumbY = ty + l.scroll*(th-thumbH)/maxScroll
	}
	return tx, ty, tw, th, thumbY, thumbH, true
}

// textareaHitScrollbar 判断 (x,y) 是否落在该元素滚动条轨道内（含滑块）。
// 非 textarea 或未溢出都返回 false。
func textareaHitScrollbar(g Graphics, b *htmlElement, x, y int) bool {
	if b == nil || !strings.EqualFold(b.tagName, "textarea") {
		return false
	}
	tx, ty, tw, th, _, _, ok := textareaScrollbar(g, b)
	return ok && x >= tx && x < tx+tw && y >= ty && y < ty+th
}

// scrollbarClick 判断点击 (x,y) 是否落在可编辑元素的覆盖滚动条上——
// 滚动条只管滚动，mousedown 与后续 click 都不应移动光标。
func scrollbarClick(doc HTMLDocument, el HTMLElement, x, y int) bool {
	return textareaHitScrollbar(docGraphics(doc), inner(el), x, y)
}

// textareaScrollTo 把轨道内的 y 坐标映射为滚动偏移（滑块中心对准指针），
// 变化时写回 scrollTop 并发文档变更通知（重排/状态栏随之刷新）。
func textareaScrollTo(doc HTMLDocument, b *htmlElement, y int) {
	g := docGraphics(doc)
	l := layoutTextarea(g, b)
	contentH := len(l.lines) * l.lh
	maxScroll := contentH - b.height
	if maxScroll <= 0 {
		return
	}
	_, ty, _, th, _, thumbH, ok := textareaScrollbar(g, b)
	if !ok || th <= thumbH {
		return
	}
	pos := clampInt(y-ty-thumbH/2, 0, th-thumbH)
	if next := pos * maxScroll / (th - thumbH); next != b.scrollTop {
		b.scrollTop = next
		b.onChanged()
	}
}

// textarea 多行编辑：折行、光标↔坐标映射与纵向滚动。
//
// textarea 的内容存在 value 属性（解析器按原始文本元素读入，见
// html_parser.go），布局期不为它建行盒——行片段在渲染/光标映射时按
// 当前字体现算，保证「编辑→绘制→点击回定位」三者口径一致。

// textareaLine 一行折行结果。start/end 为 value 的 rune 下标：
// [start,end) 即本行文本，行尾 '\n' 位于 end 处（属本行的行尾符）。
type textareaLine struct {
	text  string
	start int
	end   int
	w     int
}

// wrapTextarea 按 maxW 贪心折行：'\n' 强制断行，行末优先在空格处回退
// 断行，超长单词硬切。始终至少返回一行（空值也返回空行，供光标落位）。
func wrapTextarea(g Graphics, value string, maxW, fontPx int, bold bool, family string) []textareaLine {
	if g == nil {
		g = NewFakeGraphics()
	}
	if maxW < 1 {
		maxW = 1
	}
	runes := []rune(value)
	measure := func(s string) int {
		w, _ := g.MeasureText(s, fontPx, bold, family)
		return w
	}
	var lines []textareaLine
	var cur []rune
	start := 0
	curW := 0
	lastSpace := -1 // cur 内最后一个空格的下标（断行回退点）

	push := func(text string, s, e, w int) {
		lines = append(lines, textareaLine{text: text, start: s, end: e, w: w})
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\r' {
			// 兜底：value 由 SetAttribute 直接写入时可能没经过归一（正常路径
			// SetValue/粘贴/解析都会先折成 '\n'）。CRLF 的 '\r' 交给后面的
			// '\n' 断行；孤立 CR 与 '\n' 同义。两种都不进行文本，避免把
			// 控制字符当字形画出来（行尾方块）。
			if i+1 < len(runes) && runes[i+1] == '\n' {
				continue
			}
			push(string(cur), start, i, curW)
			start = i + 1
			cur = cur[:0]
			curW = 0
			lastSpace = -1
			continue
		}
		if r == '\n' {
			// 行尾符记在本行 end 处；下一行从其后开始（循环的 i++ 消费它）
			push(string(cur), start, i, curW)
			start = i + 1
			cur = cur[:0]
			curW = 0
			lastSpace = -1
			continue
		}
		w, _ := g.MeasureText(string(r), fontPx, bold, family)
		if curW+w > maxW && len(cur) > 0 {
			if lastSpace > 0 {
				// 在最后一个空格处断行：空格**留在上一行末尾**（含在 keep 内），
				// 下一行从空格之后开始。旧实现 keep=cur[:lastSpace] 把空格本身
				// 漏在两行之外——显示文本比 value 少字符，且该 rune 不属于任何
				// 行区间，光标映射随之错位。
				keep := cur[:lastSpace+1]
				rest := append([]rune(nil), cur[lastSpace+1:]...)
				push(string(keep), start, start+lastSpace+1, measure(string(keep)))
				start += lastSpace + 1
				cur = rest
				curW = measure(string(cur))
			} else {
				push(string(cur), start, start+len(cur), curW)
				start += len(cur)
				cur = cur[:0]
				curW = 0
			}
			lastSpace = -1
		}
		if r == ' ' {
			lastSpace = len(cur)
		}
		cur = append(cur, r)
		curW += w
	}
	// 末行（值以 '\n' 结尾时是空行，光标可落位）
	push(string(cur), start, len(runes), curW)
	return lines
}

// textareaLayout 是 textarea 内容区的排版结果；x/y 为内容盒原点，
// y 已扣除纵向滚动，可直接用于绘制。
type textareaLayout struct {
	x, y   int
	w, h   int
	lh     int
	asc    int // 基线以上高度（文本装饰定位用）
	scroll int
	lines  []textareaLine
}

// layoutTextarea 按元素当前尺寸/值/滚动量计算行布局。
// 滚动量在此静默夹紧（不触发 changed，避免布局期死循环）。
func layoutTextarea(g Graphics, el *htmlElement) textareaLayout {
	comp := el.computed
	if comp == nil {
		comp = newStyle()
		el.computed = comp
	}
	px, bold, family := fontOf(comp)
	maxW := el.width
	if maxW < 1 {
		maxW = 1
	}
	lines := wrapTextarea(g, el.GetAttribute("value"), maxW, px, bold, family)
	_, lh := g.MeasureText("Mg", px, bold, family)
	if lh <= 0 {
		lh = px * 13 / 10
	}
	// 声明的 line-height 优先：textarea 内部的行距、光标行高、选区高度与
	// 滚动范围都按 lh 算（行高变了这些必须一起变）。
	if decl := resolvedLineHeight(comp); decl >= 0 {
		lh = decl
	}
	asc, _ := g.FontMetrics("", px, bold, family)
	if asc <= 0 {
		asc = lh / 2
	}
	scroll := el.scrollTop
	if scroll < 0 {
		// 负值哨兵：跳到光标行（SetValue 设值后光标在末尾，见 form.go）。
		row := caretRow(lines, el.caret)
		top := row * lh
		scroll = top
		if top+lh > el.height {
			scroll = top + lh - el.height
		}
	}
	if scroll < 0 {
		scroll = 0
	}
	if maxScroll := len(lines)*lh - el.height; scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	el.scrollTop = scroll
	return textareaLayout{x: el.x, y: el.y - scroll, w: el.width, h: el.height,
		lh: lh, asc: asc, scroll: scroll, lines: lines}
}

// caretRow 返回光标所在行下标（行区间 [start,end] 连续不重叠）。
func caretRow(lines []textareaLine, caret int) int {
	for i, ln := range lines {
		if caret <= ln.end {
			return i
		}
	}
	if len(lines) == 0 {
		return 0
	}
	return len(lines) - 1
}

// caretXY 返回光标在内容区的落点（已扣滚动）。
func (l textareaLayout) caretXY(g Graphics, caret int, fontPx int, bold bool, family string) (x, y int) {
	if len(l.lines) == 0 {
		return l.x, l.y
	}
	row := caretRow(l.lines, caret)
	ln := l.lines[row]
	txt := []rune(ln.text)
	col := caret - ln.start
	if col < 0 {
		col = 0
	}
	if col > len(txt) {
		col = len(txt)
	}
	w, _ := g.MeasureText(string(txt[:col]), fontPx, bold, family)
	return l.x + w, l.y + row*l.lh
}

// caretFromPoint 由点击坐标反推光标 rune 下标：先按行高定行，再在行内
// 按字符中点定列（与浏览器点选一致的就近吸附）。
func (l textareaLayout) caretFromPoint(g Graphics, x, y int, fontPx int, bold bool, family string) int {
	if len(l.lines) == 0 {
		return 0
	}
	row := 0
	if l.lh > 0 {
		row = (y - l.y) / l.lh
	}
	if row < 0 {
		row = 0
	}
	if row >= len(l.lines) {
		row = len(l.lines) - 1
	}
	ln := l.lines[row]
	txt := []rune(ln.text)
	if len(txt) == 0 {
		return ln.start
	}
	prev := 0
	for k := 1; k <= len(txt); k++ {
		w, _ := g.MeasureText(string(txt[:k]), fontPx, bold, family)
		if x < l.x+(prev+w)/2 {
			return ln.start + k - 1
		}
		prev = w
	}
	return ln.end
}

// setCaret 设置光标并触发重绘；textarea 顺带保证光标在可视区内。
// 光标移动同时折叠选区（锚点跟到新光标处）——点击/方向键/输入后的
// 默认语义，拖选扩展请走 setSelection。
func setCaret(doc HTMLDocument, el *htmlElement, caret int) {
	setSelection(doc, el, caret, caret)
}

// setSelection 设置选区两端并触发重绘：anchor 为固定端、focus 为活动端
// （即 caret）。两端都夹到 value 的 rune 长度内；相等即无选区。
// 用「两端都没变」判断是否需要重绘，避免拖选时同一位置反复触发重排。
func setSelection(doc HTMLDocument, el *htmlElement, anchor, focus int) {
	if el == nil {
		return
	}
	n := len([]rune(el.GetAttribute("value")))
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > n {
			return n
		}
		return v
	}
	anchor, focus = clamp(anchor), clamp(focus)
	if el.caret == focus && el.selAnchor == anchor {
		return
	}
	el.caret = focus
	el.selAnchor = anchor
	el.onChanged()
	ensureCaretVisible(doc, el)
}

// ensureCaretVisible 保证光标在可视区内：textarea 调 scrollTop（纵向），
// 单行 input 调 textScrollX（横向，Chrome scroll-follows-caret 同口径）。
// 没有这一步，长值 input 的窗口会固定在末尾，光标索引回到 0 也永远滚不回
// 首字（examples/browser 反馈"光标移动不到首字"）。
func ensureCaretVisible(doc HTMLDocument, el *htmlElement) {
	if el == nil {
		return
	}
	g := docGraphics(doc)
	if strings.EqualFold(el.tagName, "textarea") {
		l := layoutTextarea(g, el)
		if len(l.lines) == 0 {
			return
		}
		row := caretRow(l.lines, el.caret)
		top := row * l.lh
		scroll := el.scrollTop
		if top < scroll {
			scroll = top
		}
		if top+l.lh > scroll+el.height {
			scroll = top + l.lh - el.height
		}
		if scroll < 0 {
			scroll = 0
		}
		if scroll != el.scrollTop {
			el.scrollTop = scroll
			el.onChanged()
		}
		return
	}
	// 单行可编辑 input：窗口横向跟随光标。按钮型不滚动（auto 宽不溢出）。
	if !strings.EqualFold(el.tagName, "input") || !editableInput(el) {
		return
	}
	switch strings.ToLower(el.GetAttribute("type")) {
	case "submit", "reset", "button":
		return
	}
	px, bold, family := fontOf(el.computed)
	full := []rune(displayValue(el))
	caret := clampCaret(el.caret, len(full))
	pre, _ := g.MeasureText(string(full[:caret]), px, bold, family)
	fullW, _ := g.MeasureText(string(full), px, bold, family)
	avail := el.width - 4
	if avail < 1 {
		avail = 1
	}
	scroll := el.textScrollX
	if pre < scroll {
		scroll = pre // 光标越出左缘：滚回（Home/左移/点击行首由此滚回首字）
	} else if pre > scroll+avail {
		scroll = pre - avail // 越出右缘：光标贴窗口右缘（输入/右移/End 推窗口）
	}
	if maxScroll := fullW - avail; scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	if scroll != el.textScrollX || !el.textScrollSet {
		el.textScrollX = scroll
		el.textScrollSet = true
		el.onChanged()
	}
}
