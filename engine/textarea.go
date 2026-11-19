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
//
// 行文本不随行存储：由 textareaLayout 的 runes 按区间现切（零拷贝切片，
// 绘制/导出时才落成 string）。旧实现每折一行就 string(cur) 复制一份，
// 19.5MB 的 textarea 每帧要复制 8 遍全文，实测单帧分配 1.3GB。
type textareaLine struct {
	start int
	end   int
	w     int
}

// textareaWrap 是 textarea 的折行缓存（挂在 htmlElement.wrap）。
//
// 折行结果只由「值 + 可用宽 + 字体 + 度量后端」决定，而 layoutTextarea
// 一帧内会被上溯约 8 次（绘制、选区矩形、光标↔坐标映射、滚动条几何、
// 滚动夹紧、SVG 导出…）。不缓存时每次都从头贪心折行全文：大文本下单帧
// 分配上 GB、GC 追不上就变成几百 MB 常驻。值变化由 SetAttribute("value")
// 把整个缓存置 nil（见 html_element.go），其余维度在取用时比签名。
type textareaWrap struct {
	sig   textareaWrapSig
	runes []rune         // value 的 rune 视图（行区间的编号空间）
	lines []textareaLine // 折行结果
}

// textareaWrapSig 是折行结果的生效条件；任一不同就要重算 lines。
// g 参与比较是因为 SVG 导出走 FakeGraphics，度量口径与真实后端不同
// （所有 Graphics 实现都是指针类型，接口比较 O(1) 且安全）。
type textareaWrapSig struct {
	g                   Graphics
	maxW, px, weightNum int
	bold                bool
	family              string
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
	runes  []rune // lines 的区间编号空间（textarea 的值 / input 的显示值）
}

// lineRunes 返回一行的 rune 视图：直接切片，不复制。
func (l textareaLayout) lineRunes(ln textareaLine) []rune {
	if ln.start < 0 || ln.end > len(l.runes) || ln.end <= ln.start {
		return nil
	}
	return l.runes[ln.start:ln.end]
}

// lineText 返回一行的显示文本（绘制/SVG 导出用；空行返回空串）。
func (l textareaLayout) lineText(ln textareaLine) string {
	rn := l.lineRunes(ln)
	if rn == nil {
		return ""
	}
	return string(rn)
}

// wrapTextarea 按 maxW 贪心折行：'\n' 强制断行，行末优先在空格处回退
// 断行，超长单词硬切。始终至少返回一行（空值也返回空行，供光标落位）。
//
// runes 为调用方缓存的 value 的 rune 切片（本函数不持有副本）：大文本
// textarea 的值可达几十 MB，每次布局重新 []rune(value) 会把堆打爆。
// 行文本不在此生成——消费方按 rune 区间现切。
func wrapTextarea(g Graphics, runes []rune, maxW, fontPx, weightNum int, bold bool, family string) []textareaLine {
	if g == nil {
		g = NewFakeGraphics()
	}
	if maxW < 1 {
		maxW = 1
	}
	measure := func(s string) int {
		w, _ := measureTextW(g, s, fontPx, weightNum, bold, family)
		return w
	}
	// 单字符宽度备忘：折行要逐字问宽度，19.5MB 的值是 2000 万次查询。
	// 同一个字符的宽度与上下文无关，按 rune 记一次即可（ASCII 走定长
	// 数组，其余走 map），把度量次数从「字符总数」降到「不同字符数」。
	var asciiW [128]int
	var asciiSet [128]bool
	otherW := map[rune]int{}
	runeW := func(r rune) int {
		if r < 128 {
			if asciiSet[r] {
				return asciiW[r]
			}
			w := measure(string(r))
			asciiW[r] = w
			asciiSet[r] = true
			return w
		}
		if w, ok := otherW[r]; ok {
			return w
		}
		w := measure(string(r))
		otherW[r] = w
		return w
	}

	var lines []textareaLine
	start := 0
	curW := 0
	lastSpace := -1 // 当前行内最后一个空格的下标（绝对 rune 下标，断行回退点）

	// 行区间 [s,e) 的宽度：按区间现切字符串再度量，与旧实现（对整行字符串
	// 一次 measure）口径一致。大文本下每行只切一次短串，分配量与行数同阶
	// 而非与全文同阶。
	segW := func(s, e int) int {
		if e <= s {
			return 0
		}
		return measure(string(runes[s:e]))
	}

	// push 记录一行：文本区间 [s,e)，宽度 w。
	push := func(s, e, w int) {
		lines = append(lines, textareaLine{start: s, end: e, w: w})
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\r' {
			// 兜底：value 由 SetAttribute 直接写入时可能没经过归一（正常路径
			// SetValue/粘贴/解析都会先折成 '\n'）。CRLF 的 '\r' 交给后面的
			// '\n' 断行；孤立 CR 与 '\n' 同义。两种都不进行文本，避免把
			// 控制字符当字形画出来（行尾方块）。
			//
			// 行文本改由区间现切后，'\r' 必须从区间里排除，否则会画出
			// 行尾方块：区间收在 '\r' 之前，行尾符 '\n' 仍记在 end 处。
			if i+1 < len(runes) && runes[i+1] == '\n' {
				push(start, i, curW) // 区间收在 '\r' 之前：'\r'/'\n' 都不进行文本
				i++                  // 连 '\n' 一起消费掉，否则下一轮多出一个空行
				start = i + 1
				curW = 0
				lastSpace = -1
				continue
			}
			push(start, i, curW)
			start = i + 1
			curW = 0
			lastSpace = -1
			continue
		}
		if r == '\n' {
			// 行尾符记在本行 end 处；下一行从其后开始（循环的 i++ 消费它）
			push(start, i, curW)
			start = i + 1
			curW = 0
			lastSpace = -1
			continue
		}
		w := runeW(r)
		if curW+w > maxW && i > start {
			if lastSpace > start {
				// 在最后一个空格处断行：空格**留在上一行末尾**（含在 keep 内），
				// 下一行从空格之后开始。旧实现 keep=cur[:lastSpace] 把空格本身
				// 漏在两行之外——显示文本比 value 少字符，且该 rune 不属于任何
				// 行区间，光标映射随之错位。
				keepEnd := lastSpace + 1 // 空格下标 lastSpace，空格留在行末 → end=lastSpace+1
				push(start, keepEnd, segW(start, keepEnd))
				start = keepEnd
				curW = segW(start, i)
			} else {
				push(start, i, curW)
				start = i
				curW = 0
			}
			lastSpace = -1
		}
		if r == ' ' {
			lastSpace = i
		}
		curW += w
	}
	// 末行（值以 '\n' 结尾时是空行，光标可落位）
	push(start, len(runes), curW)
	return lines
}

// layoutTextarea 按元素当前尺寸/值/滚动量计算行布局。
// 滚动量在此静默夹紧（不触发 changed，避免布局期死循环）。
func layoutTextarea(g Graphics, el *htmlElement) textareaLayout {
	comp := el.computed
	if comp == nil {
		comp = newStyle()
		el.computed = comp
	}
	px, weightNum, bold, family := fontOf(comp)
	maxW := el.width
	if maxW < 1 {
		maxW = 1
	}
	// 折行结果缓存到 el.wrap：一帧内 layoutTextarea 被上溯约 8 次（绘制、
	// 选区、光标、滚动条、SVG…），长文本每次全量折行会打爆 GC。缓存由
	// 值、宽度、字体、度量后端共同决定，任一变化即重算。
	sig := textareaWrapSig{g: g, maxW: maxW, px: px, weightNum: weightNum,
		bold: bold, family: family}
	// 值变化时 SetAttribute("value") 已把 el.wrap 置 nil（见 html_element.go），
	// 这里只需在命中时直接复用 lines，未命中才重新 []rune(value)+折行——
	// 避免每帧对 19.5MB 值做 80MB 的 rune 转换。
	if el.wrap == nil || el.wrap.sig != sig {
		if el.wrap == nil {
			el.wrap = &textareaWrap{}
		}
		el.wrap.sig = sig
		el.wrap.runes = []rune(el.GetAttribute("value"))
		el.wrap.lines = wrapTextarea(g, el.wrap.runes, maxW, px, weightNum, bold, family)
	}
	lines := el.wrap.lines
	_, lh := measureTextW(g, "Mg", px, weightNum, bold, family)
	if lh <= 0 {
		lh = px * 13 / 10
	}
	// 声明的 line-height 优先：textarea 内部的行距、光标行高、选区高度与
	// 滚动范围都按 lh 算（行高变了这些必须一起变）。
	if decl := resolvedLineHeight(comp); decl >= 0 {
		lh = decl
	}
	asc, _ := fontMetricsW(g, "", px, weightNum, bold, family)
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
		lh: lh, asc: asc, scroll: scroll, lines: lines, runes: el.wrap.runes}
}

// visibleRows 返回绘制盒 [boxY, boxY+boxH) 可能用到的首尾行下标（闭区间）。
//
// 行等高于 l.lh，所以可视范围用除法直接定位，不必遍历全表：大文档折行后
// 可达上百万行，逐行判盒外裁剪是纯浪费（同屏可见行数十几行封顶）。
// 返回值含边缘的半截行，交由调用方按「是否支持裁剪」决定画还是丢。
// 盒高非正或行高异常时退回全表，保持旧行为。
func (l textareaLayout) visibleRows(boxY, boxH int) (first, last int) {
	if l.lh <= 0 || boxH <= 0 || len(l.lines) == 0 {
		return 0, len(l.lines) - 1
	}
	first = (boxY - l.y) / l.lh
	last = (boxY + boxH - 1 - l.y) / l.lh
	if first < 0 {
		first = 0
	}
	if last >= len(l.lines) {
		last = len(l.lines) - 1
	}
	if first > last {
		return 0, -1 // 盒子与内容完全不相交
	}
	return first, last
}

// caretRow 返回光标所在行下标（行区间 [start,end] 连续不重叠）。
//
// 行 end 随下标单调不减，用二分：大文档折行可达上百万行，线性扫描会让
// 每次光标移动、每帧滚动夹紧都走一遍全表。
func caretRow(lines []textareaLine, caret int) int {
	lo, hi := 0, len(lines)-1
	if hi < 0 {
		return 0
	}
	for lo < hi {
		mid := (lo + hi) / 2
		if caret <= lines[mid].end {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// caretXY 返回光标在内容区的落点（已扣滚动）。
func (l textareaLayout) caretXY(g Graphics, caret int, fontPx, weightNum int, bold bool, family string) (x, y int) {
	if len(l.lines) == 0 {
		return l.x, l.y
	}
	row := caretRow(l.lines, caret)
	ln := l.lines[row]
	txt := l.lineRunes(ln)
	col := caret - ln.start
	if col < 0 {
		col = 0
	}
	if col > len(txt) {
		col = len(txt)
	}
	w, _ := measureTextW(g, string(txt[:col]), fontPx, weightNum, bold, family)
	return l.x + w, l.y + row*l.lh
}

// caretFromPoint 由点击坐标反推光标 rune 下标：先按行高定行，再在行内
// 按字符中点定列（与浏览器点选一致的就近吸附）。
func (l textareaLayout) caretFromPoint(g Graphics, x, y int, fontPx, weightNum int, bold bool, family string) int {
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
	txt := l.lineRunes(ln)
	if len(txt) == 0 {
		return ln.start
	}
	prev := 0
	for k := 1; k <= len(txt); k++ {
		w, _ := measureTextW(g, string(txt[:k]), fontPx, weightNum, bold, family)
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
	n := valueRuneLen(el)
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
	px, weightNum, bold, family := fontOf(el.computed)
	full := []rune(displayValue(el))
	caret := clampCaret(el.caret, len(full))
	pre, _ := measureTextW(g, string(full[:caret]), px, weightNum, bold, family)
	fullW, _ := measureTextW(g, string(full), px, weightNum, bold, family)
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
		// 文本滚动窗口是**绘制层**状态（inputVisibleWindow 只在渲染/导出/
		// 光标定位处被读）：位置没变、只是首次定位也要记重排账的话，首次
		// 聚焦每个 input 都会拖一次整树重排（大页面 1.7 秒）。改记重绘账。
		el.onPaintChanged()
	}
}
