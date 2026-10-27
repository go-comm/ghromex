package engine

import (
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// 表格布局（display 驱动的完整表格模型，CSS 2.1 §17）
//
// 表格盒由 display 值驱动而非标签名：UA 样式表把 HTML <table>/<caption>/
// <tr>/<td> 等落位到表格 display 家族，任意元素写 display: table 同样成立
// （Chrome 同款）。本文件实现：
//
//   - 网格模型：单元格按 colspan/rowspan 落入行列（HTML5 标准放置算法）；
//     rowspan=0 按「延伸到网格末行」处理，≥9999 钳制为无限（3000，网格向下
//     延长隐式行，Chrome 同款）；
//   - 匿名盒：表格直属单元格 → 隐式行；表格/行组里的块级、行内容 → 占满
//     整行的匿名行；行（tr）里的非单元格子内容 → 按连续段聚成匿名格。
//     匿名载体不进 DOM（绘制/命中没有它的盒），只作为格的载体排布内容；
//   - 列宽：单列格的天然外宽定列；跨列格的宽度差按比例补进所辖列；
//     col/colgroup 宽度作列下限；格的百分比宽按**列外宽**解析（含格的
//     padding/border，同 Chrome 表格语义）；表格显式宽按比例拉伸/压缩列宽
//     （不低于列下限）；auto 宽收缩并以可用宽为上限；
//   - 行高：行等高（取行内格的外盒最大）；tr 显式高度/遗留 height 属性作
//     行下限；跨行格的宽度差按比例分给所辖行；格盒拉伸填满行高，内容按
//     vertical-align 落位——格上的 baseline 按 middle 处理（§17.5.2.1），
//     遗留 valign 属性与 tr 的值作回退档；
//   - border-spacing：网格四边都留间距（含外缘，Chrome 同款）。UA 给
//     <table> 的默认是 2px，CSS 初始值是 0——display:table 的普通元素不带
//     这份 2px。collapse 模式间距恒为 0，保持「半边框」模型
//     （collapseCellBorders 留上/左，表格补画外缘右/下）；
//   - caption-side：题注排网格上方/下方两档（left/right 回退 top）；
//   - empty-cells: hide 只隐绘制（背景/边框跳过，见 noCellDeco），格位、
//     列宽与命中照旧——Chrome 同口径。
//
// v1 边界：collapse 下不做相邻边框的「宽者胜」冲突仲裁（取先声明格）；
// rowspan 跨行组（thead→tbody）不合并；格的百分比高在行高不确定时按
// auto 退化；嵌套表格的列宽独立计算（外层按内容宽收）。
// ---------------------------------------------------------------------------

// 行组的视觉档位：表头组 → 正文组（含表格直属行，DOM 顺序）→ 表尾组
// （CSS 2.1 §17.5.1.2，Chrome 同款：tfoot 写在 tbody 前也排最后）。
const (
	tblGroupHeader = iota
	tblGroupBody
	tblGroupFooter
)

// 表格语境下 vertical-align 的三档（baseline 及非表格值归入 middle）。
const (
	tblVTop = iota
	tblVMiddle
	tblVBottom
)

// tableCell 是网格里的一格：真实格（td/th/display:table-cell）或匿名格载体。
type tableCell struct {
	el       *htmlElement
	spanW    int  // 声明列宽（colspan；fullSpan 列建格后扩为全部列）
	spanH    int  // 声明行高（rowspan；9999→3000）
	infinite bool // rowspan=0：延伸到网格末行
	fullSpan bool // 匿名「占整行」格：spanW 建格完成后扩为列数
	row      int  // 落位后的起始行/列
	col      int
	natW     int  // 测量外宽（内容自然宽，边框盒）
	minW     int  // 最小内容外宽（1px 试排换行后的最宽不可断片段，边框盒）——
	// Chrome 压列宽的下限：列宽被表格可用宽压时不得低于它，否则 CJK 列
	// 被比例压成一字宽、"10.31" 这类不可断片段被裁出格（会议纪要表实测）。
	natH     int  // 槽宽重量后的外高（含拉伸前）
	va       int8 // tblV* 三档
	empty    bool // 空单元格（empty-cells 用，仅真实格）
}

// tableRow 是网格的一行：真实 tr 或隐式/匿名行（el=nil 不写盒）。
type tableRow struct {
	el    *htmlElement
	kind  int8
	minH  int
	cells []*tableCell
	y     int
	h     int
}

// tableGroup 是行组元素（thead/tbody/tfoot/display:*-group）及其收集到的行。
type tableGroup struct {
	el   *htmlElement
	rows []*tableRow
}

// tableGrid 是一次表格布局的网格全貌。
type tableGrid struct {
	rows     []*tableRow
	groups   []*tableGroup
	captions []*htmlElement
	colMin   []int // col/colgroup 给出的各列外宽下限
	colCount int   // col 声明的列数（可多于实际格列）
}

// addRow 把行归入 body（groupEl=nil 为表格直属），同时按视觉档记录。
func (g *tableGrid) addRow(row *tableRow, grp *tableGroup, kind int8) {
	row.kind = kind
	g.rows = append(g.rows, row)
	if grp != nil {
		grp.rows = append(grp.rows, row)
	}
}

// ---------------------------------------------------------------------------
// 遗留属性与小工具
// ---------------------------------------------------------------------------

// tableIntAttr 读整数遗留属性（colspan/rowspan/width/height/span）。
func tableIntAttr(el *htmlElement, key string) (int, bool) {
	if el == nil || el.attrs == nil {
		return 0, false
	}
	v, ok := el.attrs[key]
	if !ok {
		for k, s := range el.attrs {
			if strings.EqualFold(k, key) {
				v, ok = s, true
				break
			}
		}
	}
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

// tableColspan 读 colspan：非法/缺省按 1，钳制 1..1000（HTML5）。
func tableColspan(el *htmlElement) int {
	n, ok := tableIntAttr(el, "colspan")
	if !ok || n < 1 {
		return 1
	}
	if n > 1000 {
		n = 1000
	}
	return n
}

// tableRowspan 读 rowspan：缺省 1 行；0 = 延伸到网格末（infinite）；
// ≥9999 = 钳制为无限跨度（3000 行，网格向下延长隐式行，Chrome 同款）。
func tableRowspan(el *htmlElement) (int, bool) {
	n, ok := tableIntAttr(el, "rowspan")
	if !ok || n == 1 {
		return 1, false
	}
	if n <= 0 {
		return 1, true
	}
	if n >= 9999 {
		return 3000, false
	}
	return n, false
}

// valignAttr 解析遗留 valign 属性（top/middle/center/bottom）。
func valignAttr(el *htmlElement) (int8, bool) {
	if el == nil || el.attrs == nil {
		return 0, false
	}
	v, ok := el.attrs["valign"]
	if !ok {
		for k, s := range el.attrs {
			if strings.EqualFold(k, "valign") {
				v, ok = s, true
				break
			}
		}
	}
	if !ok {
		return 0, false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "top":
		return tblVTop, true
	case "middle", "center":
		return tblVMiddle, true
	case "bottom":
		return tblVBottom, true
	}
	return 0, false
}

// resolveCellVA 定格的落位档：自身显式 top/middle/bottom 最优先；格上的
// baseline（含 UA 默认）按 middle 处理（§17.5.2.1），但先过两级回退：
// 格自身的遗留 valign → tr 的 computed/遗留 valign。
func resolveCellVA(cell, tr *htmlElement) int8 {
	if cell.computed != nil {
		switch cell.computed.VerticalAlign() {
		case VerticalAlignTop:
			return tblVTop
		case VerticalAlignBottom:
			return tblVBottom
		case VerticalAlignMiddle:
			return tblVMiddle
		}
	}
	if v, ok := valignAttr(cell); ok {
		return v
	}
	if tr != nil {
		if tr.computed != nil {
			switch tr.computed.VerticalAlign() {
			case VerticalAlignTop:
				return tblVTop
			case VerticalAlignBottom:
				return tblVBottom
			case VerticalAlignMiddle:
				return tblVMiddle
			}
		}
		if v, ok := valignAttr(tr); ok {
			return v
		}
	}
	return tblVMiddle
}

// rowDeclaredMinH 是 tr 的显式行高下限（computed height 或遗留 height
// 属性；百分比高在行高不确定时按 auto 退化）。
func rowDeclaredMinH(tr *htmlElement) int {
	if tr == nil || tr.computed == nil {
		return 0
	}
	if h := tr.computed.Height(); h != nil && h.Type() == SIZE_PIXEL && h.Pixel() > 0 {
		pdc, bdc := resolvedEdges(tr, tr.computed)
		return h.Pixel() + pdc[eTop] + pdc[eBottom] + bdc[eTop] + bdc[eBottom]
	}
	if n, ok := tableIntAttr(tr, "height"); ok && n > 0 {
		return n
	}
	return 0
}

// runHasContent 判断一段子节点里是否有非空白内容：纯空白文本不生成匿名盒
// （CSS 2.1 §17.5.3 的匿名化规则只包非白内容）。
func runHasContent(nodes []HTMLElement) bool {
	for _, n := range nodes {
		if tn, ok := n.(*textNode); ok {
			if strings.TrimSpace(tn.node.text) != "" {
				return true
			}
			continue
		}
		return true
	}
	return false
}

// tableCellEmpty 判空单元格：无元素子节点且无非空白文本（CSS 2.1 §17.5.1.1）。
// display:none 的子元素不算内容（Chrome 同款）。
func tableCellEmpty(el *htmlElement) bool {
	for _, c := range el.children {
		if tn, ok := c.(*textNode); ok {
			if strings.TrimSpace(tn.node.text) != "" {
				return false
			}
			continue
		}
		if cb := inner(c); cb != nil && cb.computed != nil && cb.computed.Display() == DisplayNone {
			continue
		}
		return false
	}
	return true
}

// newAnonCell 是 CSS 匿名盒的布局载体：不进 DOM（绘制/命中没有它的盒），
// 样式复制容器 computed（字体等继承随行），display 强制 table-cell 让
// auto 宽按内容测量；margin 清零（CSS：表格单元的 margin 被忽略）。
func newAnonCell(container *htmlElement, children []HTMLElement) *htmlElement {
	syn := &htmlElement{tagName: container.tagName}
	syn.self = syn
	syn.attrs = container.attrs
	var comp CSSStyleDeclaration
	if container.computed != nil {
		comp = copyStyle(container.computed)
	} else {
		comp = newStyle()
	}
	comp.SetDisplay(DisplayTableCell)
	comp.SetMargin(NewZeroRect())
	// 匿名盒没有声明尺寸：容器（表格/行）的显式宽高不能复制过来。
	if cs, ok := comp.(*cssStyleDeclaration); ok {
		cs.width, cs.height = nil, nil
	}
	syn.computed = comp
	syn.children = append([]HTMLElement(nil), children...)
	return syn
}

// ---------------------------------------------------------------------------
// 边框塌陷（collapse）——半边框模型
// ---------------------------------------------------------------------------

// tableCaption 返回表格第一个 display:table-caption 的题注盒（无则 nil）。
func tableCaption(tbl *htmlElement) *htmlElement {
	if tbl == nil {
		return nil
	}
	for _, c := range tbl.children {
		cb := inner(c)
		if cb != nil && cb.computed != nil && cb.computed.Display() == DisplayTableCaption {
			return cb
		}
	}
	return nil
}

// tableCellOuterW 返回单元格外宽（内容 + padding + border）。
func tableCellOuterW(cell *htmlElement) int {
	if cell == nil {
		return 0
	}
	pd, bd := resolvedEdges(cell, cell.computed)
	return cell.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
}

// cellOuterH 返回单元格外高（内容 + padding + border）。
func cellOuterH(cell *htmlElement) int {
	if cell == nil || cell.computed == nil {
		return 0
	}
	pd, bd := resolvedEdges(cell, cell.computed)
	return cell.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
}

// collapseCellBorders 把单元格边框改成「塌陷」归属：只保留上/左两条
// （相邻单元格共享的那条线由右边邻居的左边框、下一行的上边框承担），
// 外缘的右/下两条由表格自己补画（见 layoutTableBox 末尾）。
//
// 这是浏览器「半边框」模型在整数像素下的等价形式：Chrome 实测
// `border-collapse: collapse` + `border: 1px solid` + `padding: 4px 8px`
// 的单元格，外盒 = 内容 + 16 + 1（左右各摊 0.5px），行高 = 内容 + 8 + 1。
// 若照 separate 那样四边都画，单元格会宽/高各多 1px，相邻两格之间还会出现
// 2px 双线。
func collapseCellBorders(cell *htmlElement) {
	if cell == nil || cell.computed == nil {
		return
	}
	bd := resolveEdgeRect(cell.computed.BorderStyleWidth(), cell.width)
	zero := NewZeroSize()
	cell.computed.SetBorderStyleWidth(NewRect(
		NewSize(SIZE_PIXEL, bd[eLeft], 0), // left
		zero,                              // right
		NewSize(SIZE_PIXEL, bd[eTop], 0),  // top
		zero /* bottom */))
}

// collapsedOuterBorder 返回塌陷表格外缘边框的宽度与颜色：逐格逐边找第一条
// 非零边框（collapse 后格上只剩上/左；单边边框时按对应边取色）。表格自身
// 已声明边框时不调用——作者样式优先。
func collapsedOuterBorder(cells []*htmlElement) (int, Color) {
	for _, cell := range cells {
		if cell == nil || cell.computed == nil {
			continue
		}
		bd := resolveEdgeRect(cell.computed.BorderStyleWidth(), cell.width)
		for _, edge := range []boxEdge{eTop, eLeft, eBottom, eRight} {
			if bd[edge] <= 0 {
				continue
			}
			c := cell.computed.BorderColorFor(BorderSide(edge))
			if c == nil {
				c = cell.computed.BorderColor()
			}
			return bd[edge], c
		}
	}
	return 0, nil
}

// ---------------------------------------------------------------------------
// 网格构建：按 display 分类子树，收集行/格/列约束
// ---------------------------------------------------------------------------

// tableDispOf 读盒子的 display（未级联时按行内级）。
func tableDispOf(cb *htmlElement) Display {
	if cb == nil || cb.computed == nil {
		return DisplayInline
	}
	return cb.computed.Display()
}

// groupKindOf 把行组 display 映射到视觉档位。
func groupKindOf(d Display) int8 {
	switch d {
	case DisplayTableHeaderGroup:
		return tblGroupHeader
	case DisplayTableFooterGroup:
		return tblGroupFooter
	}
	return tblGroupBody
}

// gridBuilder 是一次建格遍历的状态。
type gridBuilder struct {
	grid *tableGrid
	pctW int // 列宽百分比的基数（表格声明的内容宽；-1 表示不确定，百分比列宽按 0 处理）
	// 当前层级上下文：直属表格时 grp=nil。
	container *htmlElement
	grp       *tableGroup
	kind      int8
	// implicit 是表格/行组直属单元格的隐式行（连续格共处一行）。
	implicit *tableRow
	// run 是连续的行内级内容段（文本 + 行内盒），成段生成匿名格。
	run []HTMLElement
}

// makeCell 由真实格元素建格（读 colspan/rowspan/vertical-align/空态）。
func makeCell(cell, tr *htmlElement) *tableCell {
	sh, inf := tableRowspan(cell)
	return &tableCell{
		el:       cell,
		spanW:    tableColspan(cell),
		spanH:    sh,
		infinite: inf,
		va:       resolveCellVA(cell, tr),
		empty:    tableCellEmpty(cell),
	}
}

// appendRow 按当前上下文把行记入网格（含行组）。
func (b *gridBuilder) appendRow(row *tableRow) {
	b.grid.addRow(row, b.grp, b.kind)
}

// flushImplicit 落定隐式行（直属单元格）。
func (b *gridBuilder) flushImplicit() {
	if b.implicit != nil && len(b.implicit.cells) > 0 {
		b.appendRow(b.implicit)
	}
	b.implicit = nil
}

// flushRun 把行内内容段落成「占整行」的匿名行。纯空白段不生成盒。
// 顺序：隐式行先落（它来自更早的格），匿名行随后——与 DOM 顺序一致。
func (b *gridBuilder) flushRun() {
	nodes := b.run
	b.run = nil
	if !runHasContent(nodes) {
		return
	}
	b.flushImplicit()
	b.appendRow(&tableRow{cells: []*tableCell{{
		el:       newAnonCell(b.container, nodes),
		spanW:    1,
		spanH:    1,
		fullSpan: true,
		va:       tblVMiddle,
	}}})
}

// flushAll 落定隐式行与行内段（层级切换/遍历结束）。
func (b *gridBuilder) flushAll() {
	b.flushRun()
	b.flushImplicit()
}

// cellRun 把当前行内段落成行内的一个匿名格（占 1 槽）。
func (b *gridBuilder) cellRun(tr *htmlElement, cells *[]*tableCell) {
	nodes := b.run
	b.run = nil
	if !runHasContent(nodes) {
		return
	}
	*cells = append(*cells, &tableCell{
		el:    newAnonCell(tr, nodes),
		spanW: 1,
		spanH: 1,
		va:    tblVMiddle,
	})
}

// enterRow 收集一行的格：真实格 + 行内段/块级内容生成的匿名格（各占 1 槽）。
func (b *gridBuilder) enterRow(tr *htmlElement) {
	row := &tableRow{el: tr, minH: rowDeclaredMinH(tr)}
	for _, c := range tr.children {
		if _, ok := c.(*textNode); ok {
			b.run = append(b.run, c)
			continue
		}
		cb := inner(c)
		if cb == nil {
			continue
		}
		d := tableDispOf(cb)
		switch {
		case d == DisplayNone:
			hideBox(cb)
		case cb.computed != nil && cb.computed.Position().isOutFlow():
			// 脱流盒由第二遍定位布局处理，不进网格。
		case d == DisplayTableCell:
			b.cellRun(tr, &row.cells)
			row.cells = append(row.cells, makeCell(cb, tr))
		case d == DisplayInline || d == DisplayInlineBlock:
			b.run = append(b.run, c)
		default:
			// 行内的块级/表格级内容：CSS 生成匿名格包住它（占 1 槽）。
			b.cellRun(tr, &row.cells)
			row.cells = append(row.cells, &tableCell{
				el:    newAnonCell(tr, []HTMLElement{c}),
				spanW: 1,
				spanH: 1,
				va:    tblVMiddle,
			})
		}
	}
	b.cellRun(tr, &row.cells)
	b.appendRow(row)
}

// walkTableLevel 遍历表格级（表格直属或行组直属）的子节点。
func (b *gridBuilder) walkTableLevel(parent *htmlElement) {
	b.container = parent
	for _, c := range parent.children {
		if _, ok := c.(*textNode); ok {
			b.run = append(b.run, c)
			continue
		}
		cb := inner(c)
		if cb == nil {
			continue
		}
		d := tableDispOf(cb)
		switch {
		case d == DisplayNone:
			hideBox(cb)
		case cb.computed != nil && cb.computed.Position().isOutFlow():
			// 脱流盒不进网格。
		case d == DisplayTableCaption:
			b.flushAll()
			b.grid.captions = append(b.grid.captions, cb)
		case d.isTableRowGroup():
			b.flushAll()
			sub := &tableGroup{el: cb}
			b.grid.groups = append(b.grid.groups, sub)
			savedGrp, savedKind := b.grp, b.kind
			b.grp, b.kind = sub, groupKindOf(d)
			b.walkTableLevel(cb)
			b.flushAll()
			b.grp, b.kind = savedGrp, savedKind
			b.container = parent
		case d == DisplayTableRow:
			b.flushAll()
			b.enterRow(cb)
		case d == DisplayTableCell:
			b.flushRun()
			if b.implicit == nil {
				b.implicit = &tableRow{}
			}
			b.implicit.cells = append(b.implicit.cells, makeCell(cb, nil))
		case d == DisplayTableColumn || d == DisplayTableColumnGroup:
			b.flushAll()
			b.collectCols(cb, d)
		case d == DisplayInline || d == DisplayInlineBlock:
			b.run = append(b.run, c)
		default:
			// 块级/表格级内容：先落行内段，再单独成「占整行」的匿名行。
			b.flushRun()
			b.flushImplicit()
			b.appendRow(&tableRow{cells: []*tableCell{{
				el:       newAnonCell(parent, []HTMLElement{c}),
				spanW:    1,
				spanH:    1,
				fullSpan: true,
				va:       tblVMiddle,
			}}})
		}
	}
	b.flushAll()
}

// colWidthOf 读 col/colgroup 的宽度约束（computed 优先，遗留 width 属性兜底）。
// 百分比按 pctW 解析；基数不确定时按 0（不约束）。computed 是 auto 时落到
// 遗留属性——width 的初始值就是 auto，不能当成「已声明」。
func (b *gridBuilder) colWidthOf(el *htmlElement) int {
	if el.computed != nil {
		if s := el.computed.Width(); s != nil {
			switch s.Type() {
			case SIZE_PIXEL:
				return s.Pixel()
			case SIZE_PERCENT:
				if b.pctW > 0 {
					return resolveLen(s, b.pctW)
				}
				return 0
			}
		}
	}
	if el.attrs != nil {
		v, ok := el.attrs["width"]
		if !ok {
			for k, s := range el.attrs {
				if strings.EqualFold(k, "width") {
					v, ok = s, true
					break
				}
			}
		}
		if ok {
			v = strings.TrimSpace(v)
			if strings.HasSuffix(v, "%") {
				if b.pctW > 0 {
					if n, err := strconv.Atoi(strings.TrimSuffix(v, "%")); err == nil && n > 0 {
						return b.pctW * n / 100
					}
				}
				return 0
			}
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// colSpanOf 读 col 的 span 属性（1..1000）。
func colSpanOf(el *htmlElement) int {
	n, ok := tableIntAttr(el, "span")
	if !ok || n < 1 {
		return 1
	}
	if n > 1000 {
		return 1000
	}
	return n
}

// appendCol 把一列的宽度下限写进 colMin（并推进列数）。
func (b *gridBuilder) appendCol(w int) {
	b.grid.colMin = append(b.grid.colMin, w)
	b.grid.colCount++
}

// collectCols 收集 col / colgroup 的列宽约束。colgroup 的宽度在无自身宽度的
// 列间均分（余数给末列）。
func (b *gridBuilder) collectCols(el *htmlElement, d Display) {
	if d == DisplayTableColumn {
		w := b.colWidthOf(el)
		span := colSpanOf(el)
		for i := 0; i < span; i++ {
			share := w
			if span > 1 {
				share = w / span
				if i == span-1 {
					share = w - (span-1)*(w/span)
				}
			}
			b.appendCol(share)
		}
		return
	}
	gw := b.colWidthOf(el)
	// 先数本组内 col 的列数（无 col 时按 span 属性，缺省 1）。
	total := 0
	var cols []*htmlElement
	for _, c := range el.children {
		cb := inner(c)
		if cb == nil || tableDispOf(cb) != DisplayTableColumn {
			continue
		}
		cols = append(cols, cb)
		total += colSpanOf(cb)
	}
	if len(cols) == 0 {
		span := colSpanOf(el)
		share := gw
		if span > 1 {
			share = gw / span
		}
		for i := 0; i < span; i++ {
			w := share
			if i == span-1 && span > 1 {
				w = gw - (span-1)*share
			}
			b.appendCol(w)
		}
		return
	}
	for _, cb := range cols {
		w := b.colWidthOf(cb)
		if w <= 0 && gw > 0 {
			w = gw / total
		}
		span := colSpanOf(cb)
		for i := 0; i < span; i++ {
			share := w
			if span > 1 {
				share = w / span
				if i == span-1 {
					share = w - (span-1)*(w/span)
				}
			}
			b.appendCol(share)
		}
	}
}

// buildTableGrid 遍历表格子树建网格（pctW 是列宽百分比的解析基数）。
func buildTableGrid(tbl *htmlElement, pctW int) *tableGrid {
	b := &gridBuilder{grid: &tableGrid{}, pctW: pctW, container: tbl, kind: tblGroupBody}
	b.walkTableLevel(tbl)
	b.grid.place()
	return b.grid
}

// ---------------------------------------------------------------------------
// 落位：按 HTML5 表格模型的槽位算法（colspan/rowspan 占位）
// ---------------------------------------------------------------------------

// place 把每行的格落进网格：按行序、列游标推进，跳过被上方 rowspan 占住的
// 槽位。rowspan=0 延伸到末行；跨度超出末行时按剩余行数钳制（Chrome 同口径，
// 不生成幽灵行）。落位后统一算列数，并把「占整行」的匿名格扩到全部列。
func (g *tableGrid) place() {
	nrows := len(g.rows)
	occ := make([][]bool, nrows)
	ncols := 0
	for ri, row := range g.rows {
		cursor := 0
		for _, c := range row.cells {
			sw := c.spanW
			if sw < 1 {
				sw = 1
			}
			sh := c.spanH
			if c.infinite || sh < 1 || ri+sh > nrows {
				sh = nrows - ri
			}
			if sh < 1 {
				sh = 1
			}
			col := cursor
			for {
				free := true
				for or := ri; or < ri+sh && free; or++ {
					for oc := col; oc < col+sw; oc++ {
						if oc < len(occ[or]) && occ[or][oc] {
							free = false
							break
						}
					}
				}
				if free {
					break
				}
				col++
			}
			for or := ri; or < ri+sh; or++ {
				for len(occ[or]) < col+sw {
					occ[or] = append(occ[or], false)
				}
				for oc := col; oc < col+sw; oc++ {
					occ[or][oc] = true
				}
			}
			c.row, c.col, c.spanW, c.spanH = ri, col, sw, sh
			cursor = col + sw
			if cursor > ncols {
				ncols = cursor
			}
		}
	}
	if g.colCount > ncols {
		ncols = g.colCount
	}
	g.colCount = ncols
	for _, row := range g.rows {
		for _, c := range row.cells {
			if c.fullSpan {
				c.spanW = ncols - c.col
				if c.spanW < 1 {
					c.spanW = 1
				}
			}
		}
	}
}

// spreadExtra 把宽度差 needed 按当前宽度比例补进 cols[from:from+span]；
// 总宽为 0 时均分，余数给末列。
func spreadExtra(cols []int, from, span, needed int) {
	if needed <= 0 || span <= 0 || from < 0 || from >= len(cols) {
		return
	}
	if from+span > len(cols) {
		span = len(cols) - from
	}
	tot := 0
	for i := from; i < from+span; i++ {
		tot += cols[i]
	}
	if tot <= 0 {
		each := needed / span
		for i := 0; i < span; i++ {
			cols[from+i] += each
		}
		for i := 0; i < needed-each*span; i++ {
			cols[from+i]++
		}
		return
	}
	done := 0
	for i := 0; i < span; i++ {
		share := needed * cols[from+i] / tot
		if i == span-1 {
			share = needed - done
		}
		cols[from+i] += share
		done += share
	}
}

// scaleTotal 把列宽整体缩放到 target（表格声明宽的拉伸/压缩）：按比例缩放，
// 被压到列下限（mins）以下的列钉住并重分剩余宽（最多 n 轮，必然收敛）。
// 返回新列宽切片。
func scaleTotal(cols, mins []int, target int) []int {
	n := len(cols)
	out := make([]int, n)
	if n == 0 {
		return out
	}
	mSum := 0
	for _, m := range mins {
		mSum += m
	}
	if target < mSum {
		target = mSum
	}
	pinned := make([]bool, n)
	rem := target
	for {
		tot := 0
		for i := range cols {
			if !pinned[i] {
				tot += cols[i]
			}
		}
		if tot <= 0 {
			cnt := 0
			for i := range cols {
				if !pinned[i] {
					cnt++
				}
			}
			if cnt == 0 {
				break
			}
			each := rem / cnt
			extra := rem - each*cnt
			for i := range cols {
				if pinned[i] {
					continue
				}
				out[i] = each
				if extra > 0 {
					out[i]++
					extra--
				}
			}
			break
		}
		violated := false
		done := 0
		next := rem
		for i := range cols {
			if pinned[i] {
				continue
			}
			var v int
			if i == n-1 {
				v = rem - done
			} else {
				v = cols[i] * rem / tot
			}
			done += v
			if i < len(mins) && v < mins[i] {
				pinned[i], out[i] = true, mins[i]
				next -= mins[i]
				violated = true
				continue
			}
			out[i] = v
		}
		if !violated {
			break
		}
		rem = next
		if rem <= 0 {
			for i := range cols {
				if !pinned[i] {
					out[i] = 0
				}
			}
			break
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 表格盒布局
// ---------------------------------------------------------------------------

// layoutTableBox 布局表格盒（display:table / inline-table）：网格、列宽、行高、
// 单元格拉伸与 vertical-align、题注、collapse 外缘补画。
//
// 口径与 layoutBox 对齐：x/y 写内容盒原点，width/height 写内容盒尺寸，
// rsvPD/rsvBD 缓存解析后的边（绘制/命中复用），layoutH 记外盒高（含 margin）。
// 返回：display:table 与块级同口径（outerW, 外盒高）；inline-table 返回外边距
// 盒宽高（行内原子盒用）。
func layoutTableBox(g Graphics, tbl *htmlElement, outerW, outerH, ox, oy, pctW, pctH int) (retW, retH int) {
	comp := tbl.computed
	if comp == nil {
		comp = newStyle()
		tbl.computed = comp
	}
	disp := comp.Display()

	mg := resolveEdgeRect(comp.Margin(), pctW)
	bd := resolveEdgeRect(comp.BorderStyleWidth(), pctW)
	pd := resolveEdgeRect(comp.Padding(), pctW)
	tbl.rsvPD, tbl.rsvBD, tbl.hasRsv = pd, bd, true

	// 内容宽：显式 width（border-box 时扣边）。
	declaredW := resolveLen(comp.Width(), pctW)
	if declaredW >= 0 && comp.BoxSizing() == BoxSizingBorderBox {
		if cw := declaredW - pd[eLeft] - pd[eRight] - bd[eLeft] - bd[eRight]; cw > 0 {
			declaredW = cw
		} else {
			declaredW = 0
		}
	}
	availW := outerW - mg[eLeft] - mg[eRight] - bd[eLeft] - bd[eRight] - pd[eLeft] - pd[eRight]
	if availW < 0 {
		availW = 0
	}
	// 格的百分比尺寸基数：表格有显式宽时按它，否则按可用宽（Chrome 同口径）。
	pctBase := availW
	if declaredW >= 0 {
		pctBase = declaredW
	}
	// 列宽百分比的基数：显式宽 → 按它；auto 宽 → 不确定（百分比列宽不约束）。
	colPctBase := -1
	if declaredW >= 0 {
		colPctBase = declaredW
	}

	// 间距：collapse 恒 0（半边框模型）；separate 取 border-spacing，网格四边都留。
	sh, sv := 0, 0
	collapse := comp.BorderCollapse() == BorderCollapseCollapse
	if !collapse {
		spH, spV := comp.BorderSpacing()
		if n := resolveLen(spH, pctW); n > 0 {
			sh = n
		}
		if n := resolveLen(spV, pctW); n > 0 {
			sv = n
		}
	}

	cx := ox + mg[eLeft] + bd[eLeft] + pd[eLeft]
	cy := oy + mg[eTop] + bd[eTop] + pd[eTop]

	grid := buildTableGrid(tbl, colPctBase)
	ncols := grid.colCount
	var cells []*tableCell
	for _, row := range grid.rows {
		cells = append(cells, row.cells...)
	}

	// 格的 margin 被忽略（CSS 2.1 §17.5.2）；collapse 下格只留上/左边框。
	for _, c := range cells {
		el := c.el
		if el.computed == nil {
			el.computed = newStyle()
		}
		el.computed.SetMargin(NewZeroRect())
		el.hasEff, el.mtAbsorbed, el.mbAbsorbed = false, false, false
		if collapse {
			collapseCellBorders(el)
		}
	}

	// 布局过程会把格与题注的 computed 宽/高/盒模型临时覆写成槽位值（第二、
	// 三遍按槽宽测量与定格）。函数退出前必须还原：否则同轮内的下一次布局
	// （行内原子的换行重排、remain=0 试排）或下一轮 Refresh 的第一遍天然宽
	// 测量会把上一轮槽宽当成作者声明——试排槽宽 1px 会让整表列塌等分、
	// 行高按 1px 换行测得虚胖（vm02 故障报告尾页 title-meta 495px 的根因）。
	type pinnedStyle struct {
		el        *htmlElement
		w, h      Size
		boxSizing BoxSizing
		disp      Display
	}
	var pinned []pinnedStyle
	pin := func(e *htmlElement) {
		if e == nil || e.computed == nil {
			return
		}
		pinned = append(pinned, pinnedStyle{e, e.computed.Width(), e.computed.Height(),
			e.computed.BoxSizing(), e.computed.Display()})
	}
	defer func() {
		for _, p := range pinned {
			p.el.computed.SetWidth(p.w)
			p.el.computed.SetHeight(p.h)
			p.el.computed.SetBoxSizing(p.boxSizing)
			p.el.computed.SetDisplay(p.disp)
		}
	}()
	for _, c := range cells {
		pin(c.el)
	}

	// 第一遍：天然尺寸（内容收缩测量，以可用宽为上限）；随后 1px 试排量
	// 最小内容外宽（内容按最窄换行后的最宽行——不可断片段/单词 + 边框内边距）。
	// 返回值不可用（fitW 被钳到试排宽），取内容盒流宽 + 自身边框内边距。
	for _, c := range cells {
		w, h := layoutBox(g, c.el, availW, 0, 0, 0, pctBase, -1, nil)
		c.natW, c.natH = w, h
		if c.el.computed != nil && c.el.computed.Display() == DisplayNone {
			c.minW = 0
			continue
		}
		layoutBox(g, c.el, 1, 0, 0, 0, pctBase, -1, nil)
		mw := c.el.contentW
		if mw > 0 && c.el.hasRsv {
			mw += c.el.rsvPD[eLeft] + c.el.rsvPD[eRight] +
				c.el.rsvBD[eLeft] + c.el.rsvBD[eRight]
		}
		// 显式声明宽分两档口径（Chrome 表格语义与 CSS 2.1 §17.5.2.2）：
		// - 百分比宽 = **列外宽**（含格自身 padding/border；第二遍槽宽同为
		//   border-box，同口径）。layoutBox 按 content-box 量出的是「内容=
		//   百分比再加 padding」，列宽虚胖 Σpadding——声明宽合计 100% 的表
		//   必然撑破容器（vm02 故障报告表格溢出 112px 的根因）。这里把
		//   natW/minW 都归位到百分比外宽本身：padding 由内容区吸收，合计
		//   超 100% 时按列宽之和撑宽表格（表格取 width 与列宽和的较大者），
		//   而不是把别的列压到声明宽之下。
		// - 像素宽是列最小宽：Chrome 不把作者声明的格宽压下去（表格随之变宽）。
		if comp := c.el.computed; comp != nil && comp.Width() != nil {
			if dw := resolveLen(comp.Width(), pctBase); dw >= 0 {
				if comp.Width().Type() == SIZE_PERCENT {
					if dw < mw {
						dw = mw // 不可断片段更宽时片段优先（列宽不低于最小内容）
					}
					c.natW, mw = dw, dw
				} else {
					mw = c.natW
				}
			}
		}
		if mw < 4 {
			mw = 4
		}
		if mw > c.natW {
			mw = c.natW
		}
		c.minW = mw
	}

	// 列宽：col 约束作下限 → 单列格定宽 → 跨列格补差（跨度从短到长）。
	cols := make([]int, ncols)
	mins := make([]int, ncols)
	for i := 0; i < ncols; i++ {
		if i < len(grid.colMin) && grid.colMin[i] > 0 {
			cols[i] = grid.colMin[i]
		}
		mins[i] = maxInt(cols[i], 1)
	}
	for _, c := range cells {
		if c.spanW != 1 || c.col >= ncols {
			continue
		}
		if c.natW > cols[c.col] {
			cols[c.col] = c.natW
		}
		if c.minW > mins[c.col] {
			mins[c.col] = c.minW
		}
	}
	for span := 2; span <= ncols; span++ {
		for _, c := range cells {
			if c.spanW != span {
				continue
			}
			covered := (span - 1) * sh
			for i := c.col; i < c.col+span && i < ncols; i++ {
				covered += cols[i]
			}
			if c.natW > covered {
				spreadExtra(cols, c.col, span, c.natW-covered)
			}
			covered = (span - 1) * sh
			for i := c.col; i < c.col+span && i < ncols; i++ {
				covered += mins[i]
			}
			if c.minW > covered {
				spreadExtra(mins, c.col, span, c.minW-covered)
			}
		}
	}
	// 传播：列宽不得低于内容最小宽（跨列分布后个别列可能仍超）。
	for i := range cols {
		if mins[i] > cols[i] {
			cols[i] = mins[i]
		}
	}
	// 表格宽：显式宽按比例缩放列（不低于列下限）；auto 宽收缩到可用宽。
	if ncols > 0 {
		if declaredW >= 0 {
			cols = scaleTotal(cols, mins, declaredW-(ncols+1)*sh)
		} else if availW > 0 {
			total := (ncols + 1) * sh
			for _, w := range cols {
				total += w
			}
			if total > availW {
				cols = scaleTotal(cols, mins, availW-(ncols+1)*sh)
			}
		}
	}
	gridW := 0
	if ncols > 0 {
		gridW = (ncols + 1) * sh
		for _, w := range cols {
			gridW += w
		}
	}

	cellX := func(c *tableCell) int {
		x := cx + sh
		for i := 0; i < c.col && i < ncols; i++ {
			x += cols[i] + sh
		}
		return x
	}
	slotW := func(c *tableCell) int {
		w := (c.spanW - 1) * sh
		for i := c.col; i < c.col+c.spanW && i < ncols; i++ {
			w += cols[i]
		}
		if w < 0 {
			w = 0
		}
		return w
	}

	// 第二遍：按槽宽重排（格宽 = 槽宽），得到换行后的行高。
	for _, c := range cells {
		w := slotW(c)
		c.el.computed.SetBoxSizing(BoxSizingBorderBox)
		c.el.computed.SetWidth(NewSize(SIZE_PIXEL, w, 0))
		_, h := layoutBox(g, c.el, w, 0, 0, 0, pctBase, -1, nil)
		c.natH = h
	}

	// 行高：tr 显式高作下限 → 单跨格定高 → 跨行格补差（跨度从短到长）。
	spreadRows := func(from, span, needed int) {
		tot := 0
		for i := from; i < from+span && i < len(grid.rows); i++ {
			tot += grid.rows[i].h
		}
		if tot <= 0 {
			each := needed / span
			for i := 0; i < span; i++ {
				grid.rows[from+i].h += each
			}
			grid.rows[from+span-1].h += needed - each*span
			return
		}
		done := 0
		for i := 0; i < span; i++ {
			share := needed * grid.rows[from+i].h / tot
			if i == span-1 {
				share = needed - done
			}
			grid.rows[from+i].h += share
			done += share
		}
	}
	for _, row := range grid.rows {
		if row.minH > row.h {
			row.h = row.minH
		}
	}
	for _, c := range cells {
		if c.spanH == 1 && c.row < len(grid.rows) && c.natH > grid.rows[c.row].h {
			grid.rows[c.row].h = c.natH
		}
	}
	for span := 2; span <= len(grid.rows); span++ {
		for _, c := range cells {
			if c.spanH != span {
				continue
			}
			covered := (span - 1) * sv
			for i := c.row; i < c.row+span && i < len(grid.rows); i++ {
				covered += grid.rows[i].h
			}
			if c.natH > covered {
				spreadRows(c.row, span, c.natH-covered)
			}
		}
	}

	// 题注：宽 = 表格边框盒宽（Chrome 同口径）。先按网格内容顶排，定出高度后
	// 再整体移到包装盒里的最终位置（题注在表格盒之外，见下方注释）。
	capH := 0
	capBottom := false
	caption := tableCaption(tbl)
	capOuterW := gridW + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
	if caption != nil && caption.computed != nil {
		capBottom = comp.CaptionSide() == CaptionSideBottom
		cmg := resolveEdgeRect(caption.computed.Margin(), pctW)
		caption.computed.SetBoxSizing(BoxSizingBorderBox)
		caption.computed.SetWidth(NewSize(SIZE_PIXEL, capOuterW, 0))
		_, h := layoutBox(g, caption, capOuterW+cmg[eLeft]+cmg[eRight], 0, cx, cy, capOuterW, -1, nil)
		capH = h
	}

	// 网格行带：题注上 → 每行前留纵向间距 → 末尾再留一次。
	gridTop := cy
	if !capBottom {
		gridTop += capH
	}
	cursor := gridTop
	for _, row := range grid.rows {
		row.y = cursor + sv
		cursor = row.y + row.h
	}
	gridH := 0
	if len(grid.rows) > 0 {
		cursor += sv
		gridH = cursor - gridTop
	}
	if caption != nil && capH > 0 {
		// 网格内容顶 → 包装盒题注位：横向退到表格边框盒左缘；纵向在网格上方
		// 或（题注在下时）网格底 + 下 padding/border 之后。
		dx := -(bd[eLeft] + pd[eLeft])
		dy := -(bd[eTop] + pd[eTop])
		if capBottom {
			dy = gridH + pd[eBottom] + bd[eBottom]
		}
		if dx != 0 || dy != 0 {
			shiftBox(caption, dx, dy)
		}
	}

	// 第三遍：定格到槽位与行带，内容按 vertical-align 落位。
	for _, c := range cells {
		el := c.el
		x, w := cellX(c), slotW(c)
		rowY, bandH := gridTop, (c.spanH-1)*sv
		if c.row < len(grid.rows) {
			rowY = grid.rows[c.row].y
			for i := c.row; i < c.row+c.spanH && i < len(grid.rows); i++ {
				bandH += grid.rows[i].h
			}
		}
		el.computed.SetBoxSizing(BoxSizingBorderBox)
		el.computed.SetWidth(NewSize(SIZE_PIXEL, w, 0))
		el.computed.SetHeight(NewSize(SIZE_PIXEL, bandH, 0))
		layoutBox(g, el, w, bandH, x, rowY, pctBase, bandH, nil)
		// 格盒已拉伸到行带，内容自然落在顶部；middle/bottom 把内容整体下移。
		pdC, bdC := resolvedEdges(el, el.computed)
		avail := bandH - pdC[eTop] - pdC[eBottom] - bdC[eTop] - bdC[eBottom]
		nat := c.natH - pdC[eTop] - pdC[eBottom] - bdC[eTop] - bdC[eBottom]
		if d := avail - nat; d > 0 {
			shift := 0
			switch c.va {
			case tblVMiddle:
				shift = d / 2
			case tblVBottom:
				shift = d
			}
			if shift > 0 {
				for i := range el.runs {
					el.runs[i].y += shift
				}
				for _, child := range el.children {
					shiftBox(child, 0, shift)
				}
			}
		}
		// empty-cells: hide 只隐绘制（几何/命中照旧，Chrome 同口径）。
		el.noCellDeco = c.empty && el.computed.EmptyCells() == EmptyCellsHide
	}

	// 行/行组盒：写几何（背景/边框与命中用）。
	for _, row := range grid.rows {
		if row.el == nil {
			continue
		}
		row.el.rsvPD, row.el.rsvBD, row.el.hasRsv = edges{}, edges{}, true
		row.el.setX(cx + sh)
		row.el.setY(row.y)
		row.el.setWidth(maxZero(gridW - 2*sh))
		row.el.setHeight(row.h)
		row.el.layoutH = row.h
		row.el.noCellDeco = false
	}
	for _, grp := range grid.groups {
		if grp.el == nil {
			continue
		}
		if len(grp.rows) == 0 {
			hideBox(grp.el)
			continue
		}
		first, last := grp.rows[0], grp.rows[len(grp.rows)-1]
		gh := last.y + last.h - first.y
		grp.el.rsvPD, grp.el.rsvBD, grp.el.hasRsv = edges{}, edges{}, true
		grp.el.setX(cx + sh)
		grp.el.setY(first.y)
		grp.el.setWidth(maxZero(gridW - 2*sh))
		grp.el.setHeight(gh)
		grp.el.layoutH = gh
	}

	// collapse 外缘：表格自身未声明右/下边框时用格的边框补画（半边框模型）。
	if collapse {
		if bw, col := collapsedOuterBorder(cellsEls(cells)); bw > 0 {
			tbd := comp.BorderStyleWidth()
			nb := tbd
			changed := false
			if resolveLen(tbd.Right(), pctW) <= 0 {
				nb = NewRect(nb.Left(), NewSize(SIZE_PIXEL, bw, 0), nb.Top(), nb.Bottom())
				comp.SetBorderStyleFor(BorderSideRight, BorderStyleSolid)
				if col != nil {
					comp.SetBorderColorFor(BorderSideRight, col)
				}
				changed = true
			}
			if resolveLen(tbd.Bottom(), pctW) <= 0 {
				nb = NewRect(nb.Left(), nb.Right(), nb.Top(), NewSize(SIZE_PIXEL, bw, 0))
				comp.SetBorderStyleFor(BorderSideBottom, BorderStyleSolid)
				if col != nil {
					comp.SetBorderColorFor(BorderSideBottom, col)
				}
				changed = true
			}
			if changed {
				comp.SetBorderStyleWidth(nb)
				bd = resolveEdgeRect(comp.BorderStyleWidth(), pctW)
				tbl.rsvBD = bd
			}
		}
	}

	contentW := gridW
	// 题注在表格盒之外（CSS 2.1 §17.4：caption 属于表格包装盒）——表格盒
	// 只含网格，题注占包装盒的额外高度。否则 collapse 补画的外缘边框会从
	// 题注带穿过去（Chrome 里表格边框只包网格）。
	contentH := gridH
	tbl.setX(cx)
	tbl.setY(gridTop)
	tbl.setWidth(contentW)
	tbl.setHeight(contentH)
	tbl.contentW, tbl.contentH = contentW, contentH

	outerBoxW := contentW + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight] + mg[eLeft] + mg[eRight]
	outerBoxH := capH + contentH + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom] + mg[eTop] + mg[eBottom]
	tbl.layoutH = outerBoxH
	if disp == DisplayInlineTable {
		return outerBoxW, outerBoxH
	}
	return outerW, outerBoxH
}

// cellsEls 取出格的盒元素（collapse 外缘补画用）。
func cellsEls(cells []*tableCell) []*htmlElement {
	out := make([]*htmlElement, 0, len(cells))
	for _, c := range cells {
		if c.el != nil {
			out = append(out, c.el)
		}
	}
	return out
}
