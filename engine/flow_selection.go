package engine

import "strings"

// 文档级文本选区（普通 HTML 文本，不含 input/textarea）。
//
// 输入框选区（selection.go）只覆盖可编辑控件：光标与锚点是控件 value 的
// rune 下标。普通页面文本没有 value，绘制单位是布局期生成的 textRun（按行/
// 词切分的片段），因此这里另建一套「位置」模型：
//
//	flowPosition = (文本节点, 该节点流文本的 rune 偏移)
//
// 「流文本」= 节点 runs 的 text 顺序拼接（即 tokenize 折叠空白后的显示
// 文本）。runs 每轮重排都会重建，但文本内容不变时拼接结果稳定，所以偏移量
// 跨重排仍然有效；若按 run 下标存，重排后就会错位。
//
// 选区状态存在文档上（htmlDocument.flowAnchor/flowFocus），与输入框选区
// 互不影响。绘制高亮不读选区两端，而是由 setFlowSelection 预先算好每个
// 文本节点被覆盖的区间（htmlElement.flowSelLo/Hi）：渲染路径拿不到文档
// 指针，也不该为了一次高亮再做一遍全文档序遍历。

// flowPosition 是文档级选区的一端。node 为文本节点的底层 htmlElement，
// off 是该节点流文本的 rune 偏移（0..流文本长度）。
type flowPosition struct {
	node *htmlElement
	off  int
}

// flowTextNodes 返回文档内所有「有文本片段」的文本节点，按 DOM 前序排列。
// 结果缓存在文档上，doc.changed 变化（DOM/布局变更）后重建；拖选期间只涨
// paintChanged，不会让缓存失效。
func (d *htmlDocument) flowTextNodes() []*htmlElement {
	if d.flowNodes != nil && d.flowNodesMark == d.changed {
		return d.flowNodes
	}
	d.flowNodes = d.flowNodes[:0]
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		for _, child := range e.Children() {
			cb := inner(child)
			if cb == nil {
				continue
			}
			if cb.isText() {
				if len(cb.runs) > 0 {
					d.flowNodes = append(d.flowNodes, cb)
				}
				continue
			}
			// 与渲染同口径：display:none 的子树不落墨，也不该被选中。
			if cb.computed != nil && cb.computed.Display() == DisplayNone {
				continue
			}
			walk(child)
		}
	}
	walk(d)
	// 空页面也要留一个非 nil 切片，否则每次都会因为 nil 判据重新走一遍全树。
	if d.flowNodes == nil {
		d.flowNodes = []*htmlElement{}
	}
	d.flowNodesMark = d.changed
	return d.flowNodes
}

// flowNodeLen 返回文本节点流文本的 rune 长度。
func flowNodeLen(b *htmlElement) int {
	n := 0
	for i := range b.runs {
		n += runeLen(b.runs[i].text)
	}
	return n
}

// flowNodeIndex 在有序文本节点表里定位节点；找不到返回 -1。
func flowNodeIndex(nodes []*htmlElement, node *htmlElement) int {
	if node == nil {
		return -1
	}
	for i, n := range nodes {
		if n == node {
			return i
		}
	}
	return -1
}

// clearFlowSelection 清掉文档级选区与所有节点上的高亮区间。
func clearFlowSelection(d *htmlDocument) {
	had := d.flowAnchor.node != nil || d.flowFocus.node != nil || len(d.flowMarked) > 0
	for _, n := range d.flowMarked {
		n.flowSelLo, n.flowSelHi = 0, 0
	}
	d.flowMarked = d.flowMarked[:0]
	d.flowAnchor, d.flowFocus = flowPosition{}, flowPosition{}
	if had {
		// 选区只是绘制状态：不触发重排，只记重绘账。
		d.markPaintChanged()
	}
}

// setFlowSelection 更新文档级选区（anchor 为固定端、focus 为活动端），
// 并重算受影响文本节点的高亮区间。两端会被夹到节点流文本长度内；相等
// 即无选区。返回是否形成非空选区。
func setFlowSelection(d *htmlDocument, anchor, focus flowPosition) bool {
	nodes := d.flowTextNodes()
	ai := flowNodeIndex(nodes, anchor.node)
	fi := flowNodeIndex(nodes, focus.node)
	if ai < 0 || fi < 0 {
		clearFlowSelection(d)
		return false
	}
	anchor.off = clampInt(anchor.off, 0, flowNodeLen(anchor.node))
	focus.off = clampInt(focus.off, 0, flowNodeLen(focus.node))
	// 活动端可以在锚点任一侧，统一按文档顺序归一化。
	if ai > fi || (ai == fi && anchor.off > focus.off) {
		anchor, focus = focus, anchor
		ai, fi = fi, ai
	}
	if ai == fi && anchor.off == focus.off {
		clearFlowSelection(d)
		return false
	}
	if d.flowAnchor == anchor && d.flowFocus == focus {
		return true
	}
	d.flowAnchor, d.flowFocus = anchor, focus
	d.markFlowNodes(nodes, ai, anchor.off, fi, focus.off)
	d.markPaintChanged()
	return true
}

// markFlowNodes 把 [ai,aoff) 到 [fi,foff) 的区间写到各文本节点上：先清掉
// 上一轮标记过的节点，再逐个写入本轮覆盖区间。
func (d *htmlDocument) markFlowNodes(nodes []*htmlElement, ai, aoff, fi, foff int) {
	for _, n := range d.flowMarked {
		n.flowSelLo, n.flowSelHi = 0, 0
	}
	d.flowMarked = d.flowMarked[:0]
	for i := ai; i <= fi; i++ {
		n := nodes[i]
		lo := 0
		if i == ai {
			lo = aoff
		}
		hi := flowNodeLen(n)
		if i == fi {
			hi = foff
		}
		if hi <= lo {
			continue
		}
		n.flowSelLo = int32(lo)
		n.flowSelHi = int32(hi)
		d.flowMarked = append(d.flowMarked, n)
	}
}

// flowSelectionBounds 归一化文档级选区：返回有序的节点表、两端所在的节点
// 下标与节点内偏移（已夹紧）。没有有效选区返回 ok=false。
func flowSelectionBounds(d *htmlDocument) (nodes []*htmlElement, ai, aoff, fi, foff int, ok bool) {
	nodes = d.flowTextNodes()
	a, f := d.flowAnchor, d.flowFocus
	ai = flowNodeIndex(nodes, a.node)
	fi = flowNodeIndex(nodes, f.node)
	if ai < 0 || fi < 0 {
		return nil, 0, 0, 0, 0, false
	}
	aoff = clampInt(a.off, 0, flowNodeLen(a.node))
	foff = clampInt(f.off, 0, flowNodeLen(f.node))
	if ai > fi || (ai == fi && aoff > foff) {
		ai, fi = fi, ai
		aoff, foff = foff, aoff
	}
	return nodes, ai, aoff, fi, foff, true
}

// flowSelectedText 返回文档级选区文本；没有非空选区返回 false。
//
// 跨块复制时插入换行（浏览器把段落/列表项/表格单元格视为换行边界），
// 同一块内的行内节点直接拼接——空白本身已是文本节点里的 run，不需要
// 再补空格。
func flowSelectedText(d *htmlDocument) (string, bool) {
	nodes, ai, aoff, fi, foff, ok := flowSelectionBounds(d)
	if !ok {
		return "", false
	}
	var b strings.Builder
	var prevBlock *htmlElement
	for i := ai; i <= fi; i++ {
		n := nodes[i]
		lo := 0
		if i == ai {
			lo = aoff
		}
		hi := flowNodeLen(n)
		if i == fi {
			hi = foff
		}
		if hi <= lo {
			continue
		}
		block := flowBlockAncestor(n)
		if b.Len() > 0 && block != prevBlock {
			b.WriteByte('\n')
		}
		flowTextInto(&b, n, lo, hi)
		prevBlock = block
	}
	if b.Len() == 0 {
		return "", false
	}
	return b.String(), true
}

// flowSelectedLength 返回文档级选区的 rune 数。与 flowSelectedText 同口径
// （含跨块换行），但不复制文本——状态栏每帧刷新时用它。
func flowSelectedLength(d *htmlDocument) (int, bool) {
	nodes, ai, aoff, fi, foff, ok := flowSelectionBounds(d)
	if !ok {
		return 0, false
	}
	total, chunks := 0, 0
	var prevBlock *htmlElement
	for i := ai; i <= fi; i++ {
		n := nodes[i]
		lo := 0
		if i == ai {
			lo = aoff
		}
		hi := flowNodeLen(n)
		if i == fi {
			hi = foff
		}
		if hi <= lo {
			continue
		}
		block := flowBlockAncestor(n)
		if chunks > 0 && block != prevBlock {
			total++
		}
		total += hi - lo
		chunks++
		prevBlock = block
	}
	if chunks == 0 {
		return 0, false
	}
	return total, true
}

// flowTextInto 把文本节点流文本的 [lo,hi) 区间写进 b，零拷贝切片。
func flowTextInto(b *strings.Builder, n *htmlElement, lo, hi int) {
	off := 0
	for i := range n.runs {
		r := &n.runs[i]
		l := runeLen(r.text)
		start, end := off, off+l
		off = end
		if end <= lo {
			continue
		}
		if start >= hi {
			break
		}
		from := 0
		if lo > start {
			from = lo - start
		}
		to := l
		if hi < end {
			to = hi - start
		}
		if from >= to {
			continue
		}
		b.WriteString(r.text[runeByteIndex(r.text, from):runeByteIndex(r.text, to)])
	}
}

// flowBlockAncestor 返回文本节点最近的块级祖先（跨块复制时据此补换行）。
// 表格单元格也算块级边界：不同单元格的文本不该黏成一句。
func flowBlockAncestor(n *htmlElement) *htmlElement {
	for p := n.parentElement; p != nil; p = p.ParentElement() {
		b := inner(p)
		if b == nil || b.isText() || b.computed == nil {
			continue
		}
		switch b.computed.Display() {
		case DisplayBlock, DisplayTable, DisplayTableCaption,
			DisplayTableRow, DisplayTableCell:
			return b
		}
	}
	return nil
}

// flowPositionAt 把视口坐标映射为文档文本位置。文本节点不作为命中目标
// （ElementAt 返回其父元素），所以在命中元素的子树里找最近的文本片段；
// 指针拖到页面内容之外（ElementAt 为 nil）或命中元素内没有文本、且
// allowFallback 为真时，退回全文档扫描——拖选要能延伸到首/末行。
func flowPositionAt(doc HTMLDocument, x, y int, allowFallback bool) (flowPosition, bool) {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return flowPosition{}, false
	}
	nodes := d.flowTextNodes()
	if len(nodes) == 0 {
		return flowPosition{}, false
	}
	var best flowPosition
	bestScore := int64(1) << 62
	found := false
	consider := func(n *htmlElement) {
		idx, score, ok := flowNearestRun(n, x, y)
		if !ok || score >= bestScore {
			return
		}
		bestScore = score
		best.node = n
		best.off = flowRunOffset(doc, n, idx, x)
		found = true
	}
	target := ElementAt(doc, x, y)
	if target != nil {
		walkFlowTextNodes(target, consider)
	}
	if !found && (allowFallback || target == nil) {
		for _, n := range nodes {
			consider(n)
		}
	}
	return best, found
}

// walkFlowTextNodes 前序遍历子树，把有文本片段的文本节点交给 visit。
func walkFlowTextNodes(e HTMLElement, visit func(*htmlElement)) {
	for _, child := range e.Children() {
		b := inner(child)
		if b == nil {
			continue
		}
		if b.isText() {
			if len(b.runs) > 0 {
				visit(b)
			}
			continue
		}
		if b.computed != nil && b.computed.Display() == DisplayNone {
			continue
		}
		walkFlowTextNodes(child, visit)
	}
}

// flowNearestRun 返回文本节点内离 (x,y) 最近的 run 下标。逐 run 比距离：
// 命中时只扫「命中元素子树」里的片段（通常是段落的几个词），长 pre 的
// 几十万 run 也只在该 pre 自身被命中时才扫——比按 y 二分更稳，mixed
// font-size 的行内文本 y 本来就不是严格单调的。
func flowNearestRun(n *htmlElement, x, y int) (idx int, score int64, ok bool) {
	runs := n.runs
	if len(runs) == 0 {
		return 0, 0, false
	}
	best := -1
	bestScore := int64(1) << 62
	for i := range runs {
		sc := flowRunScore(&runs[i], x, y)
		if best < 0 || sc < bestScore {
			best, bestScore = i, sc
		}
	}
	return best, bestScore, true
}

// flowRunScore 是点到 run 的距离：竖向距离优先（同一行内再比横向）。
// 1000000 的权重保证「行内很远的 run」也优于「相邻行的 run」。
func flowRunScore(r *textRun, x, y int) int64 {
	dy := 0
	if y < r.y {
		dy = r.y - y
	} else if y >= r.y+r.h {
		dy = y - (r.y + r.h) + 1
	}
	dx := 0
	if x < r.x {
		dx = r.x - x
	} else if x > r.x+r.w {
		dx = x - (r.x + r.w) + 1
	}
	return int64(dy)*1000000 + int64(dx)
}

// flowRunOffset 把点 x 映射为 run 内的 rune 偏移，再加上该 run 之前的
// 流文本长度，得到节点级偏移。按字符中点就近吸附（与浏览器点选一致）。
func flowRunOffset(doc HTMLDocument, n *htmlElement, idx, x int) int {
	g := docGraphics(doc)
	px, weightNum, bold, family := fontOf(n.computed)
	r := &n.runs[idx]
	off := flowRuneAtX(g, r, x, px, weightNum, bold, family)
	for i := 0; i < idx; i++ {
		off += runeLen(n.runs[i].text)
	}
	return off
}

// flowRuneAtX 返回 x 落在 run.text 的哪个 rune 边界（中点吸附）。
func flowRuneAtX(g Graphics, r *textRun, x, px, weightNum int, bold bool, family string) int {
	n := runeLen(r.text)
	if n == 0 || x <= r.x {
		return 0
	}
	if x >= r.x+r.w {
		return n
	}
	prev := 0
	for k := 1; k <= n; k++ {
		w := prefixWidth(g, r.text[:runeByteIndex(r.text, k)], px, weightNum, bold, family)
		if x < r.x+(prev+w)/2 {
			return k - 1
		}
		prev = w
	}
	return n
}

// flowRunSelectionRect 返回 run 内被选中部分的像素矩形（x 与宽）。
// measure 与绘制路径同口径（渲染传真实后端度量，SVG 导出传 Fake 度量）。
func flowRunSelectionRect(b *htmlElement, r *textRun, runStart int, measure func(string) int) (x, w int, ok bool) {
	lo, hi := int(b.flowSelLo), int(b.flowSelHi)
	if hi <= lo {
		return 0, 0, false
	}
	n := runeLen(r.text)
	if n == 0 {
		return 0, 0, false
	}
	end := runStart + n
	if end <= lo || runStart >= hi {
		return 0, 0, false
	}
	from, to := 0, n
	if lo > runStart {
		from = lo - runStart
	}
	if hi < end {
		to = hi - runStart
	}
	if from >= to {
		return 0, 0, false
	}
	x1 := r.x + measure(r.text[:runeByteIndex(r.text, from)])
	x2 := r.x + measure(r.text[:runeByteIndex(r.text, to)])
	if x2 <= x1 {
		// 子像素/零宽字符：给一个最小可见宽度，避免高亮整段消失。
		x2 = x1 + 1
	}
	return x1, x2 - x1, true
}
