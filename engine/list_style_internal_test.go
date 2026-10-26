package engine

import (
	"strings"
	"testing"
)

// list-style 简写与长写：类型/位置按任意顺序识别，省略的分量重置为初始值；
// 非法类型保留继承值。
func TestParseListStyle(t *testing.T) {
	st := newStyle()
	applyDecl(st, "list-style", "upper-roman inside")
	if st.ListStyleType() != ListStyleUpperRoman || st.ListStylePosition() != ListStyleInside {
		t.Fatalf("list-style:upper-roman inside = (%v,%v), want (upper-roman,inside)",
			st.ListStyleType(), st.ListStylePosition())
	}
	// 顺序无关 + 位置在类型之前。
	applyDecl(st, "list-style", "outside square")
	if st.ListStyleType() != ListStyleSquare || st.ListStylePosition() != ListStyleOutside {
		t.Fatalf("list-style:outside square = (%v,%v), want (square,outside)",
			st.ListStyleType(), st.ListStylePosition())
	}
	// 简写重置省略的分量：type 回到 disc、position 回到 outside。
	applyDecl(st, "list-style", "inside")
	if st.ListStyleType() != ListStyleDisc || st.ListStylePosition() != ListStyleInside {
		t.Fatalf("list-style:inside = (%v,%v), want (disc,inside)",
			st.ListStyleType(), st.ListStylePosition())
	}
	applyDecl(st, "list-style-type", "lower-latin")
	if st.ListStyleType() != ListStyleLowerAlpha {
		t.Fatalf("list-style-type:lower-latin = %v, want lower-alpha", st.ListStyleType())
	}
	// 非法值忽略：保留前一次的有效值。
	applyDecl(st, "list-style-type", "cjk-ideographic")
	if st.ListStyleType() != ListStyleLowerAlpha {
		t.Errorf("非法 list-style-type 覆盖了有效值: %v", st.ListStyleType())
	}
	// list-style-image 接受但忽略（标记仍按 type 画）。
	applyDecl(st, "list-style-image", "url(dot.png)")
	if st.ListStyleType() != ListStyleLowerAlpha {
		t.Errorf("list-style-image 不应改变 type: %v", st.ListStyleType())
	}
	// 含未知关键字的简写整条非法：不重置已有值。
	applyDecl(st, "list-style", "bogus")
	if st.ListStyleType() != ListStyleLowerAlpha || st.ListStylePosition() != ListStyleInside {
		t.Errorf("非法简写改变了值: (%v,%v)", st.ListStyleType(), st.ListStylePosition())
	}
	// 带空格的 url() 不应截断其余分量。
	applyDecl(st, "list-style", "url(\"dot image.png\") inside")
	if st.ListStyleType() != ListStyleDisc || st.ListStylePosition() != ListStyleInside {
		t.Errorf("url()+inside 简写 = (%v,%v), want (disc,inside)",
			st.ListStyleType(), st.ListStylePosition())
	}
	applyDecl(st, "list-style", "none")
	if st.ListStyleType() != ListStyleNone {
		t.Errorf("list-style:none type = %v, want none", st.ListStyleType())
	}
	// 已有显式类型时，后面的 none 归图片（不覆盖 type）。
	applyDecl(st, "list-style", "square none")
	if st.ListStyleType() != ListStyleSquare {
		t.Errorf("list-style:square none type = %v, want square", st.ListStyleType())
	}
}

// 标记文本：三种符号、数字（含前导零）、字母（双射 26 进制）、罗马数字。
func TestListMarkerText(t *testing.T) {
	cases := []struct {
		typ  ListStyleType
		n    int
		want string
	}{
		{ListStyleDisc, 1, "•"},
		{ListStyleCircle, 1, "◦"},
		{ListStyleSquare, 3, "▪"},
		{ListStyleNone, 1, ""},
		{ListStyleDecimal, 7, "7."},
		{ListStyleDecimalLeadingZero, 7, "07."},
		{ListStyleDecimalLeadingZero, 12, "12."},
		{ListStyleLowerAlpha, 1, "a."},
		{ListStyleLowerAlpha, 26, "z."},
		{ListStyleLowerAlpha, 27, "aa."},
		{ListStyleLowerAlpha, 52, "az."},
		{ListStyleUpperAlpha, 28, "AB."},
		{ListStyleLowerRoman, 4, "iv."},
		{ListStyleLowerRoman, 9, "ix."},
		{ListStyleUpperRoman, 14, "XIV."},
		{ListStyleUpperRoman, 1994, "MCMXCIV."},
		{ListStyleLowerRoman, 4000, "4000."},
	}
	for _, c := range cases {
		if got := listMarkerText(c.typ, c.n); got != c.want {
			t.Errorf("listMarkerText(%v, %d) = %q, want %q", c.typ, c.n, got, c.want)
		}
	}
}

// 列表标记按 list-style-type 生成；ol 的 UA decimal 不被 ul 的继承值覆盖
// （ol 自身有 UA 声明），嵌套列表各自独立编号。
func TestListStyleMarkersInLayout(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<ul id="u" style="list-style-type:square">
			<li>甲</li><li>乙</li>
			<ol id="o"><li>一</li><li>二</li></ol>
		</ul>
		<ol id="r" style="list-style-type:upper-roman">
			<li>一</li><li>二</li><li>三</li><li>四</li>
		</ol>
		<ul id="n" style="list-style:none"><li>甲</li></ul>
	</body></html>`
	vp, buf := NewBufferedViewport(320, 300)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)

	markers := func(sel string) []string {
		var out []string
		for _, e := range doc.QuerySelectorAll(sel) {
			if b := inner(e); b != nil {
				out = append(out, b.listMarker)
			}
		}
		return out
	}
	if got := markers("#u > li"); len(got) != 2 || got[0] != "▪" || got[1] != "▪" {
		t.Errorf("square 标记 = %q, want 两个 ▪", got)
	}
	if got := markers("#o > li"); len(got) != 2 || got[0] != "1." || got[1] != "2." {
		t.Errorf("嵌套 ol 标记 = %q, want 1. 2.（ol 的 UA decimal 优先于继承）", got)
	}
	if got := markers("#r > li"); len(got) != 4 ||
		got[0] != "I." || got[1] != "II." || got[2] != "III." || got[3] != "IV." {
		t.Errorf("upper-roman 标记 = %q, want I. II. III. IV.", got)
	}
	if got := markers("#n > li"); len(got) != 1 || got[0] != "" {
		t.Errorf("list-style:none 标记 = %q, want 空", got)
	}

	svg := doc.DumpSVG()
	if !strings.Contains(svg, "▪") {
		t.Error("SVG 导出缺少 square 标记")
	}
	if !strings.Contains(svg, "IV.") {
		t.Error("SVG 导出缺少 upper-roman 标记")
	}
	if strings.Contains(svg, "•") {
		t.Error("SVG 导出了 list-style:none 的标记")
	}
}

// inside：标记画在内容盒内首行行首，首行文字整体右移（标记宽 + 0.5em）；
// outside（默认）：标记右对齐在内容左缘之外，文字不缩进。
func TestListStylePositionInside(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<ul style="margin:0;padding-left:40px">
			<li id="in" style="list-style-position:inside">内标记</li>
			<li id="out">外标记</li>
		</ul>
	</body></html>`
	vp, buf := NewBufferedViewport(320, 200)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)

	firstRunX := func(li *htmlElement) int {
		for _, c := range li.children {
			if tn, ok := c.(*textNode); ok && len(tn.node.runs) > 0 {
				return tn.node.runs[0].x
			}
		}
		return -1
	}

	in := inner(doc.QuerySelector("#in"))
	if in == nil || !in.listMarkerInside {
		t.Fatal("inside 标记未标记为盒内绘制")
	}
	mx, _, mw, _, ok := listMarkerBox(buf, in, in.computed)
	if !ok || mx != in.x {
		t.Errorf("inside 标记 x = %d (ok=%v), want 内容左缘 %d", mx, ok, in.x)
	}
	if runX := firstRunX(in); runX < in.x+mw {
		t.Errorf("inside 首行文字 x = %d, want ≥ 标记右缘 %d（应右移）", runX, in.x+mw)
	}

	out := inner(doc.QuerySelector("#out"))
	if out == nil || out.listMarkerInside {
		t.Fatal("默认 outside 标记不应画在盒内")
	}
	ox, _, ow, _, ok := listMarkerBox(buf, out, out.computed)
	if !ok || ox+ow > out.x {
		t.Errorf("outside 标记右缘 = %d (ok=%v), want ≤ 内容左缘 %d", ox+ow, ok, out.x)
	}
	if runX := firstRunX(out); runX != out.x {
		t.Errorf("outside 首行文字 x = %d, want 内容左缘 %d（不应缩进）", runX, out.x)
	}
}

// inside 但 li 首个子节点是块级：不做首行传播，标记退回 outside 绘制，
// 避免压在块内容上。
func TestListStyleInsideBlockChildFallsBackOutside(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<ul style="margin:0;padding-left:40px">
			<li id="b" style="list-style-position:inside"><p style="margin:0">块内容</p></li>
		</ul>
	</body></html>`
	vp, buf := NewBufferedViewport(320, 200)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)

	li := inner(doc.QuerySelector("#b"))
	if li == nil {
		t.Fatal("no li")
	}
	if li.listMarkerInside {
		t.Error("首个子节点是块级时不应启用盒内标记")
	}
	mx, _, mw, _, ok := listMarkerBox(buf, li, li.computed)
	if !ok {
		t.Fatal("列表项无标记")
	}
	if mx+mw > li.x {
		t.Errorf("标记右缘 %d 压进了内容盒（li.x=%d）", mx+mw, li.x)
	}
}
