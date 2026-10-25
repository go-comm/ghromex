package engine

import "testing"

// 本文件锁定 display 驱动的完整表格模型（CSS 2.1 §17）。度量口径沿用
// BufferGraphics 的估算：16px 字号下 CJK = 16、ASCII = 9、空格 = 8、
// 行高 = 20；UA 给 td/th 的是 padding: 1px，<table> 的 border-spacing 是 2px，
// display:table 的普通元素则按 CSS 初始值 0（Chrome 同口径）。

// tableEl 取元素底层结构。
func tableEl(t *testing.T, doc HTMLDocument, id string) *htmlElement {
	t.Helper()
	e := doc.QuerySelector("#" + id)
	if e == nil {
		t.Fatalf("no %s", id)
	}
	return inner(e)
}

// tableBox 返回边框盒（与浏览器 getBoundingClientRect 同口径）。
func tableBox(e *htmlElement) (x, y, w, h int) {
	r := e.GetBoundingClientRect()
	return r.Left().Pixel(), r.Top().Pixel(),
		r.Right().Pixel() - r.Left().Pixel(),
		r.Bottom().Pixel() - r.Top().Pixel()
}

// tableFirstRunY 返回元素内首个文本 run 的行顶 y（无 run 则失败）。
func tableFirstRunY(t *testing.T, el *htmlElement) int {
	t.Helper()
	for _, c := range el.children {
		if tn, ok := c.(*textNode); ok && len(tn.node.runs) > 0 {
			return tn.node.runs[0].y
		}
	}
	t.Fatalf("%s 没有文本 run", el.tagName)
	return 0
}

// tableLayout 解析并布局一帧，返回文档与缓冲（像素断言用）。
func tableLayout(t *testing.T, src string, w, h int) (HTMLDocument, *BufferGraphics) {
	t.Helper()
	vp, buf := NewBufferedViewport(w, h)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)
	return doc, buf
}

// TestTableDisplayDivGrid 表格布局只由 display 驱动：普通 div 写
// display:table/table-row/table-cell 就能排成网格（与标签名无关），
// 且不带 UA 给 <table> 的 2px border-spacing（CSS 初始值是 0）。
func TestTableDisplayDivGrid(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<div id="t" style="display:table">
			<div style="display:table-row">
				<div id="a" style="display:table-cell">A</div>
				<div id="b" style="display:table-cell">BB</div>
			</div>
			<div style="display:table-row">
				<div id="c" style="display:table-cell">CCC</div>
				<div id="d" style="display:table-cell">D</div>
			</div>
		</div>
	</body></html>`
	doc, _ := tableLayout(t, src, 400, 200)
	a, b := tableEl(t, doc, "a"), tableEl(t, doc, "b")
	c, d := tableEl(t, doc, "c"), tableEl(t, doc, "d")
	tbl := tableEl(t, doc, "t")

	// 列宽取同列最宽格：col0 = CCC(27)，col1 = BB(18)；行高 = 行高 20。
	if _, _, w, h := tableBox(a); w != 27 || h != 20 {
		t.Errorf("a 盒 = %dx%d, want 27x20", w, h)
	}
	if _, _, w, _ := tableBox(b); w != 18 {
		t.Errorf("b 宽 = %d, want 18", w)
	}
	// 同列左缘对齐、第二列紧贴第一列（display:table 无 border-spacing）。
	if a.x != c.x || b.x != d.x {
		t.Errorf("同列左缘未对齐：a=%d c=%d b=%d d=%d", a.x, c.x, b.x, d.x)
	}
	ax, _, aw, _ := tableBox(a)
	bx, _, _, _ := tableBox(b)
	if bx != ax+aw {
		t.Errorf("列间有间隙：a 右缘 %d, b 左缘 %d（display:table 的 spacing 应为 0）", ax+aw, bx)
	}
	// 行堆叠、单元格拉伸到行高。
	if c.y != 20 || a.y != 0 {
		t.Errorf("行 y = a:%d c:%d, want 0/20", a.y, c.y)
	}
	if _, _, w, h := tableBox(tbl); w != 45 || h != 40 {
		t.Errorf("表格盒 = %dx%d, want 45x40", w, h)
	}
}

// TestTableUADefaultSpacing <table> 的 UA 默认 border-spacing: 2px（网格四边
// 都留，含外缘）；display:table 的 div 则没有这份间距。
func TestTableUADefaultSpacing(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t"><tr><td id="a">A</td></tr></table>
		<div id="d" style="display:table"><div id="dc" style="display:table-cell">A</div></div>
	</body></html>`
	doc, _ := tableLayout(t, src, 400, 200)
	a, tbl := tableEl(t, doc, "a"), tableEl(t, doc, "t")
	dc, dtbl := tableEl(t, doc, "dc"), tableEl(t, doc, "d")

	// <table>：格 11x22（内容 9 + padding 2），四周各 2px 间距。
	if x, y, w, h := tableBox(a); x != 2 || y != 2 || w != 11 || h != 22 {
		t.Errorf("td 盒 = %d,%d %dx%d, want 2,2 11x22", x, y, w, h)
	}
	if _, _, w, h := tableBox(tbl); w != 15 || h != 26 {
		t.Errorf("table 盒 = %dx%d, want 15x26（11x22 + 四周 2px）", w, h)
	}
	// display:table 的 div：无间距、无 UA padding → 9x20，表 = 格。
	// （它排在 <table> 之后，y 从上一块的 26 起算。）
	if x, y, w, h := tableBox(dc); x != 0 || y != 26 || w != 9 || h != 20 {
		t.Errorf("display:table-cell 盒 = %d,%d %dx%d, want 0,26 9x20", x, y, w, h)
	}
	if _, _, w, h := tableBox(dtbl); w != 9 || h != 20 {
		t.Errorf("display:table 盒 = %dx%d, want 9x20", w, h)
	}
}

// TestTableColspanRowspan 网格落位：colspan 跨列（槽宽 = 所辖列宽之和），
// rowspan 跨行（格盒拉伸到所辖行高之和），后续行的格跳过被占槽位。
func TestTableColspanRowspan(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:0">
			<tr><td id="a" colspan="2">AA</td><td id="c">C</td></tr>
			<tr><td id="d" rowspan="2">D</td><td id="e">E</td><td id="f">F</td></tr>
			<tr><td id="g">G</td><td id="h">H</td></tr>
		</table>
	</body></html>`
	doc, _ := tableLayout(t, src, 400, 200)
	el := func(id string) *htmlElement { return tableEl(t, doc, id) }

	// 三列各 11 宽（内容 9 + padding 2），行高各 22。
	if _, _, w, h := tableBox(el("a")); w != 22 || h != 22 {
		t.Errorf("colspan=2 的格 = %dx%d, want 22x22", w, h)
	}
	if x, _, _, _ := tableBox(el("c")); x != 22 {
		t.Errorf("第三列左缘 = %d, want 22", x)
	}
	// rowspan=2：格盒高 = 两行之和，第二行的格跳过第一列。
	if x, y, w, h := tableBox(el("d")); x != 0 || y != 22 || w != 11 || h != 44 {
		t.Errorf("rowspan=2 的格 = %d,%d %dx%d, want 0,22 11x44", x, y, w, h)
	}
	if x, y, _, _ := tableBox(el("e")); x != 11 || y != 22 {
		t.Errorf("e = %d,%d, want 11,22", x, y)
	}
	if x, y, _, _ := tableBox(el("g")); x != 11 || y != 44 {
		t.Errorf("g = %d,%d, want 11,44", x, y)
	}
	if x, _, _, _ := tableBox(el("h")); x != 22 {
		t.Errorf("h 左缘 = %d, want 22", x)
	}
	if _, _, w, h := tableBox(el("t")); w != 33 || h != 66 {
		t.Errorf("表格盒 = %dx%d, want 33x66", w, h)
	}
}

// TestTableRowspanClamp rowspan=0 延伸到末行；rowspan≥9999 钳制为无限跨度，
// 但**不生成幽灵行**（Chrome 同口径：表格高度仍由实际行决定）。
func TestTableRowspanClamp(t *testing.T) {
	body := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:0">
			<tr><td id="x" rowspan="%s">X</td><td id="y">Y</td></tr>
			<tr><td id="z">Z</td></tr>
		</table>
	</body></html>`
	for _, rs := range []string{"0", "9999"} {
		vp, buf := NewBufferedViewport(400, 200)
		d, err := OpenDocument(vp, sprintfSpan(body, rs))
		if err != nil {
			t.Fatalf("OpenDocument: %v", err)
		}
		RenderNode(buf, d)
		x, z, tbl := tableEl(t, d, "x"), tableEl(t, d, "z"), tableEl(t, d, "t")
		if _, _, _, h := tableBox(x); h != 44 {
			t.Errorf("rowspan=%s 的格高 = %d, want 44（两行之和）", rs, h)
		}
		if _, y, _, _ := tableBox(z); y != 22 {
			t.Errorf("rowspan=%s 下第二行 y = %d, want 22", rs, y)
		}
		if _, _, _, h := tableBox(tbl); h != 44 {
			t.Errorf("rowspan=%s 下表格高 = %d, want 44（不得生成幽灵行）", rs, h)
		}
	}
}

// TestTableBorderSpacing border-spacing 的两轴：单值四边同间距（含外缘），
// 双值 = 水平/垂直。
func TestTableBorderSpacing(t *testing.T) {
	one := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:6px">
			<tr><td id="a">A</td><td id="b">B</td></tr>
		</table>
	</body></html>`
	doc, _ := tableLayout(t, one, 400, 200)
	a, b, tbl := tableEl(t, doc, "a"), tableEl(t, doc, "b"), tableEl(t, doc, "t")
	if x, y, _, _ := tableBox(a); x != 6 || y != 6 {
		t.Errorf("6px 间距下首格 = %d,%d, want 6,6", x, y)
	}
	if x, _, _, _ := tableBox(b); x != 23 {
		t.Errorf("6px 间距下第二格左缘 = %d, want 23（6+11+6）", x)
	}
	if _, _, w, h := tableBox(tbl); w != 40 || h != 34 {
		t.Errorf("6px 间距的表格盒 = %dx%d, want 40x34", w, h)
	}

	two := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:4px 6px">
			<tr><td id="a">A</td><td id="b">B</td></tr>
		</table>
	</body></html>`
	doc2, _ := tableLayout(t, two, 400, 200)
	a2, b2, tbl2 := tableEl(t, doc2, "a"), tableEl(t, doc2, "b"), tableEl(t, doc2, "t")
	if x, y, _, _ := tableBox(a2); x != 4 || y != 6 {
		t.Errorf("4px 6px 间距下首格 = %d,%d, want 4,6", x, y)
	}
	if x, _, _, _ := tableBox(b2); x != 19 {
		t.Errorf("水平间距 4px 下第二格左缘 = %d, want 19（4+11+4）", x)
	}
	if _, _, w, h := tableBox(tbl2); w != 34 || h != 34 {
		t.Errorf("4px 6px 间距的表格盒 = %dx%d, want 34x34", w, h)
	}
}

// TestTableCellVerticalAlign 行等高（取行内最高格），格盒拉伸填满行带，
// 内容按 vertical-align 落位：top 贴顶、middle 居中、bottom 贴底。
func TestTableCellVerticalAlign(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:0">
			<tr>
				<td id="vtop" style="vertical-align:top">A</td>
				<td id="vmid" style="vertical-align:middle">A</td>
				<td id="vbot" style="vertical-align:bottom">A</td>
				<td id="tall">A<br>B</td>
			</tr>
		</table>
	</body></html>`
	doc, _ := tableLayout(t, src, 400, 200)
	el := func(id string) *htmlElement { return tableEl(t, doc, id) }

	// 行高 = 两行文本 40 + padding 2 = 42；三个单行格都拉伸到 42。
	for _, id := range []string{"vtop", "vmid", "vbot", "tall"} {
		if _, _, _, h := tableBox(el(id)); h != 42 {
			t.Errorf("%s 盒高 = %d, want 42（行等高）", id, h)
		}
	}
	// 内容 y：内容盒顶 = 1（padding），middle 下移 (40-20)/2 = 10，bottom 下移 20。
	if y := tableFirstRunY(t, el("vtop")); y != 1 {
		t.Errorf("vertical-align:top 文本 y = %d, want 1", y)
	}
	if y := tableFirstRunY(t, el("vmid")); y != 11 {
		t.Errorf("vertical-align:middle 文本 y = %d, want 11", y)
	}
	if y := tableFirstRunY(t, el("vbot")); y != 21 {
		t.Errorf("vertical-align:bottom 文本 y = %d, want 21", y)
	}
}

// TestTableCaptionSide 题注排在网格上方（默认）或下方（caption-side:bottom），
// 宽度随表格宽，计入表格盒高。
func TestTableCaptionSide(t *testing.T) {
	top := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:0">
			<caption id="cap">T</caption>
			<tr><td id="cell">A</td></tr>
		</table>
	</body></html>`
	doc, _ := tableLayout(t, top, 400, 200)
	cap, cell, tbl := tableEl(t, doc, "cap"), tableEl(t, doc, "cell"), tableEl(t, doc, "t")
	if _, cy, _, _ := tableBox(cap); cy >= cell.y {
		t.Errorf("默认题注未排在网格上方：cap.y=%d cell.y=%d", cy, cell.y)
	}
	// 题注在表格盒之外（CSS 2.1 §17.4）：表格盒只含网格，题注占包装盒额外高度。
	if _, _, _, h := tableBox(tbl); h != 22 {
		t.Errorf("表格盒高 = %d, want 22（题注不在表格盒内）", h)
	}
	if y := cell.y; y != 21 {
		t.Errorf("带题注的网格顶 = %d, want 21（题注 20 + padding 1）", y)
	}

	bottom := `<!doctype html><html><body style="margin:0">
		<table id="t" style="caption-side:bottom;border-spacing:0">
			<caption id="cap">T</caption>
			<tr><td id="cell">A</td></tr>
		</table>
	</body></html>`
	doc2, _ := tableLayout(t, bottom, 400, 200)
	cap2, cell2, tbl2 := tableEl(t, doc2, "cap"), tableEl(t, doc2, "cell"), tableEl(t, doc2, "t")
	if _, cy, _, _ := tableBox(cap2); cy <= cell2.y {
		t.Errorf("caption-side:bottom 未排在网格下方：cap.y=%d cell.y=%d", cy, cell2.y)
	}
	if _, _, _, h := tableBox(tbl2); h != 22 {
		t.Errorf("题注在下的表格盒高 = %d, want 22", h)
	}
}

// TestTableEmptyCellsHide empty-cells:hide 只隐绘制（背景/边框不画），
// 格位、尺寸与命中照旧——Chrome 同口径。
func TestTableEmptyCellsHide(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:0;empty-cells:hide">
			<tr>
				<td id="e1" style="width:20px;height:20px;background:#ff0000"></td>
				<td id="e2" style="background:#ff0000">X</td>
			</tr>
		</table>
	</body></html>`
	doc, buf := tableLayout(t, src, 200, 100)
	e1, e2 := tableEl(t, doc, "e1"), tableEl(t, doc, "e2")
	if !e1.noCellDeco {
		t.Errorf("空单元格未标记 noCellDeco（empty-cells:hide 应跳过装饰）")
	}
	if e2.noCellDeco {
		t.Errorf("非空单元格被误标记 noCellDeco")
	}
	// 几何不变：空格的 20px 显式宽高 + padding 2。
	if _, _, w, h := tableBox(e1); w != 22 || h != 22 {
		t.Errorf("空单元格盒 = %dx%d, want 22x22（几何不因 empty-cells 改变）", w, h)
	}
	// 像素：空格中心不被红色填充；非空格的边框盒内（文本 run 之外）是红色。
	if r, g, b, _, ok := buf.ColorAt(11, 11); !ok || (r == 255 && g == 0 && b == 0) {
		t.Errorf("空单元格背景被绘制了：r=%d g=%d b=%d", r, g, b)
	}
	if r, g, b, _, ok := buf.ColorAt(22, 0); !ok || !(r == 255 && g == 0 && b == 0) {
		t.Errorf("非空单元格背景未绘制：r=%d g=%d b=%d", r, g, b)
	}
}

// TestTableColWidth col/colgroup 的宽度作列下限（style 与遗留 width 属性）。
func TestTableColWidth(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:0">
			<colgroup><col style="width:50px"><col width="30"></colgroup>
			<tr><td id="a">A</td><td id="b">B</td></tr>
		</table>
	</body></html>`
	doc, _ := tableLayout(t, src, 400, 200)
	a, b, tbl := tableEl(t, doc, "a"), tableEl(t, doc, "b"), tableEl(t, doc, "t")
	if _, _, w, _ := tableBox(a); w != 50 {
		t.Errorf("col width:50px 的列宽 = %d, want 50", w)
	}
	if _, _, w, _ := tableBox(b); w != 30 {
		t.Errorf("col width=30 的列宽 = %d, want 30", w)
	}
	if _, _, w, _ := tableBox(tbl); w != 80 {
		t.Errorf("表格宽 = %d, want 80", w)
	}
}

// TestTableImpliedClose HTML 解析的隐式闭合：<td>a<td>b 是同一行的两格，
// <tr> 隐式结束上一行（浏览器解析同口径）。
func TestTableImpliedClose(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-spacing:0"><tr><td id="a">a<td id="b">b<tr><td id="c">c</table>
	</body></html>`
	doc, _ := tableLayout(t, src, 400, 200)
	a, b, c := tableEl(t, doc, "a"), tableEl(t, doc, "b"), tableEl(t, doc, "c")
	if a.y != b.y {
		t.Errorf("隐式闭合的两个格不在同一行：a.y=%d b.y=%d", a.y, b.y)
	}
	if x, _, _, _ := tableBox(b); x != 11 {
		t.Errorf("第二格左缘 = %d, want 11", x)
	}
	if y := c.y; y != 23 {
		t.Errorf("隐式闭合的下一行 y = %d, want 23（行顶 22 + padding 1）", y)
	}
}

// TestTableDeclaredWidth 表格声明宽是下限：列按比例拉伸填满声明宽
// （不低于列下限），格盒随之变宽。
func TestTableDeclaredWidth(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t" style="width:100px;border-spacing:0">
			<tr><td id="a" style="width:50%">A</td><td id="b">B</td></tr>
		</table>
	</body></html>`
	doc, _ := tableLayout(t, src, 400, 200)
	a, b, tbl := tableEl(t, doc, "a"), tableEl(t, doc, "b"), tableEl(t, doc, "t")
	if _, _, w, _ := tableBox(tbl); w != 100 {
		t.Fatalf("声明宽 100px 的表格宽 = %d, want 100", w)
	}
	aw := tableBox2W(a)
	bw := tableBox2W(b)
	if aw+bw != 100 {
		t.Errorf("两列宽之和 = %d, want 100（列按比例填满声明宽）", aw+bw)
	}
	if aw <= bw {
		t.Errorf("50%% 宽的列 = %d, 无声明宽的列 = %d（百分比列应更宽）", aw, bw)
	}
}

// tableBox2W 只取边框盒宽。
func tableBox2W(e *htmlElement) int {
	_, _, w, _ := tableBox(e)
	return w
}

// sprintfSpan 替换 rowspan 占位。
func sprintfSpan(tpl, v string) string {
	out := make([]byte, 0, len(tpl)+len(v))
	for i := 0; i < len(tpl); i++ {
		if tpl[i] == '%' && i+1 < len(tpl) && tpl[i+1] == 's' {
			out = append(out, v...)
			i++
			continue
		}
		out = append(out, tpl[i])
	}
	return string(out)
}
