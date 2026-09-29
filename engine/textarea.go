package engine

import "strings"

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
				// 在最后一个空格处断行：空格留在上一行，不作下一行行首
				keep := cur[:lastSpace]
				rest := append([]rune(nil), cur[lastSpace+1:]...)
				push(string(keep), start, start+lastSpace, measure(string(keep)))
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
	scroll := el.scrollTop
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
		lh: lh, scroll: scroll, lines: lines}
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
func setCaret(doc HTMLDocument, el *htmlElement, caret int) {
	if el == nil {
		return
	}
	if n := len([]rune(el.GetAttribute("value"))); caret > n {
		caret = n
	}
	if caret < 0 {
		caret = 0
	}
	if el.caret == caret {
		return
	}
	el.caret = caret
	el.onChanged()
	ensureCaretVisible(doc, el)
}

// ensureCaretVisible 为 textarea 调整 scrollTop 使光标行可见。
func ensureCaretVisible(doc HTMLDocument, el *htmlElement) {
	if el == nil || !strings.EqualFold(el.tagName, "textarea") {
		return
	}
	g := docGraphics(doc)
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
}
