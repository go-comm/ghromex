package engine

import "testing"

// TestTableBorderCollapse 锁定 border-collapse: collapse 的「半边框」模型。
//
// Chrome 实测（雅黑 16px，`border: 1px solid` + `padding: 4px 8px`）：
// 单元格边框盒 90.38x30（= 内容 73.38 + 16 + **1**，左右两层边框只摊到
// 1px），表 181.75x61，相邻单元格共享一条 1px 线。若照 separate 画四边，
// 单元格会变成 91.38x31（+1），格间还多出 2px 空隙——三轮对比里表格区
// 「宽 2px、每行高 1px」的偏差就来自这里。
//
// 本用例用 BufferGraphics 的估算度量（CJK = 字号、空格 = 字号/2、
// ASCII = 字号×3/5，行高 = 字号×13/10），故内容宽 74、行盒高 20：
// 单元格 91x29、行 182x29、表外盒 183x59。
func TestTableBorderCollapse(t *testing.T) {
	src := `<!doctype html><html><head><style>
		body { margin: 0; }
		table { border-collapse: collapse; }
		td { border: 1px solid #767676; padding: 4px 8px; }
	</style></head><body>
		<table id="t">
			<tr id="r1"><td id="a1">单元格 A1</td><td id="a2">单元格 A2</td></tr>
			<tr id="r2"><td id="b1">单元格 B1</td><td id="b2">单元格 B2</td></tr>
		</table>
	</body></html>`
	vp, buf := NewBufferedViewport(400, 200)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)

	el := func(id string) *htmlElement {
		e := doc.QuerySelector(id)
		if e == nil {
			t.Fatalf("no %s", id)
		}
		return inner(e)
	}
	// 边框盒（与浏览器 getBoundingClientRect 同口径）
	box := func(id string) (x, y, w, h int) {
		r := el(id).GetBoundingClientRect()
		return r.Left().Pixel(), r.Top().Pixel(),
			r.Right().Pixel() - r.Left().Pixel(),
			r.Bottom().Pixel() - r.Top().Pixel()
	}
	tbl := el("#t")
	a1x, a1y, a1w, a1h := box("#a1")
	a2x, _, _, _ := box("#a2")
	_, r1y, r1w, r1h := box("#r1")
	_, b1y, _, _ := box("#b1")
	tw, th := tbl.GetBoundingClientRect().Right().Pixel()-tbl.GetBoundingClientRect().Left().Pixel(),
		tbl.GetBoundingClientRect().Bottom().Pixel()-tbl.GetBoundingClientRect().Top().Pixel()

	// 解析：collapse 必须落到计算样式上（否则下面的布局分支不会生效）
	if bc := tbl.computed.BorderCollapse(); bc != BorderCollapseCollapse {
		t.Fatalf("table 的 border-collapse = %v, want collapse", bc)
	}
	// 单元格只保留上/左（相邻线由右边邻居的左边框、下一行的上边框承担），
	// 右/下交给表格补画。
	bd := el("#a1").computed.BorderStyleWidth()
	if bd.Left().Pixel() != 1 || bd.Top().Pixel() != 1 {
		t.Errorf("单元格上/左边框 = %d/%d, want 1/1", bd.Top().Pixel(), bd.Left().Pixel())
	}
	if bd.Right().Pixel() != 0 || bd.Bottom().Pixel() != 0 {
		t.Errorf("单元格右/下边框 = %d/%d, want 0/0（共享线只画一次）",
			bd.Right().Pixel(), bd.Bottom().Pixel())
	}
	// 表格外缘：右/下各 1px（Chrome 的 collapse 表边框盒含外缘半个边框）
	tbd := tbl.computed.BorderStyleWidth()
	if tbd.Right().Pixel() != 1 || tbd.Bottom().Pixel() != 1 {
		t.Errorf("表格右/下外缘边框 = %d/%d, want 1/1", tbd.Right().Pixel(), tbd.Bottom().Pixel())
	}

	// 相邻单元格无缝（separate 才有 border-spacing）
	if a2x != a1x+a1w {
		t.Errorf("第二列左缘 = %d, want 第一列右缘 %d（collapse 不应有间隙）", a2x, a1x+a1w)
	}
	// 行按行堆叠且行高 = 单元格盒高（每行只摊一条 1px 线，不再多 1px）
	if b1y != r1y+r1h {
		t.Errorf("第二行顶 = %d, want %d", b1y, r1y+r1h)
	}
	if r1h != a1h {
		t.Errorf("行高 = %d, want 单元格盒高 %d", r1h, a1h)
	}

	// 绝对尺寸（BufferGraphics 估算度量下的确定值）
	if a1w != 91 || a1h != 29 {
		t.Errorf("单元格外盒 = %dx%d, want 91x29（内容 74 + padding 16 + 边框 1）", a1w, a1h)
	}
	if r1w != 182 {
		t.Errorf("行宽 = %d, want 182（两列各 91、无间隙）", r1w)
	}
	// 表格盒子 = 单元格盒之和 + 外缘 1px
	if tw != 183 || th != 59 {
		t.Errorf("表格外盒 = %dx%d, want 183x59（182x58 + 外缘 1）", tw, th)
	}
	if a1y != r1y {
		t.Errorf("单元格顶 = %d, want 行顶 %d", a1y, r1y)
	}
}

// TestTableBorderCollapseSeparateDefault border-collapse 缺省/其他值必须回到
// separate（保持 2px 间隙的老行为），避免新解析把默认值带偏。
func TestTableBorderCollapseSeparateDefault(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t" style="border-collapse: separate"><tr><td id="a">x</td></tr></table>
	</body></html>`
	vp, buf := NewBufferedViewport(200, 100)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)
	tbl := inner(doc.QuerySelector("#t"))
	td := inner(doc.QuerySelector("#a"))
	if bc := tbl.computed.BorderCollapse(); bc != BorderCollapseSeparate {
		t.Fatalf("border-collapse = %v, want separate", bc)
	}
	// separate 下单元格边框不被裁剪（未声明边框则四边皆 0）
	bd := td.computed.BorderStyleWidth()
	if bd.Left().Pixel() != 0 || bd.Right().Pixel() != 0 {
		t.Fatalf("separate 下未声明边框却被改动：%d/%d", bd.Left().Pixel(), bd.Right().Pixel())
	}
}
