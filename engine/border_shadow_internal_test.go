package engine

import "testing"

// border 简写先写四边，单边简写再覆盖一条边；长写属性最后覆盖单个分量。
// 这里把级联后真正参与绘制的四个数组都断言一遍，避免只测到解析入口。
func TestParseBorderSides(t *testing.T) {
	st := newStyle()
	applyDecl(st, "border", "1px solid #111111")
	applyDecl(st, "border-top", "3px dashed #ff0000")
	applyDecl(st, "border-left-width", "5px")
	applyDecl(st, "border-right-color", "#00ff00")
	applyDecl(st, "border-bottom-style", "none")

	if got := resolveEdgeRect(st.BorderStyleWidth(), 0); got != (edges{3, 1, 1, 5}) {
		t.Fatalf("border widths = %v, want top3 right1 bottom1 left5", got)
	}
	if st.BorderStyleFor(BorderSideBottom) != BorderStyleNone {
		t.Fatal("border-bottom-style:none 未生效")
	}
	if st.BorderStyleFor(BorderSideTop) != BorderStyleSolid {
		t.Fatal("dashed 目前应按 solid 处理")
	}
	for side, want := range map[BorderSide]uint32{
		BorderSideTop:    0xff0000,
		BorderSideRight:  0x00ff00,
		BorderSideBottom: 0x111111,
		BorderSideLeft:   0x111111,
	} {
		c := st.BorderColorFor(side)
		if c == nil {
			t.Fatalf("side %d color = nil", side)
		}
		r, g, b, _ := c.RGBA()
		if got := uint32(r)<<16 | uint32(g)<<8 | uint32(b); got != want {
			t.Errorf("side %d color = %06x, want %06x", side, got, want)
		}
	}
}

// border-color / border-style 的 1~4 值展开必须按 CSS 顺序落到
// top/right/bottom/left，而不是全部写同一边。
func TestParseBorderColorStyleShorthand(t *testing.T) {
	st := newStyle()
	applyDecl(st, "border-width", "1px 2px 3px 4px")
	applyDecl(st, "border-color", "red green blue")
	applyDecl(st, "border-style", "none solid")

	if got := resolveEdgeRect(st.BorderStyleWidth(), 0); got != (edges{1, 2, 3, 4}) {
		t.Fatalf("widths = %v, want 1/2/3/4", got)
	}
	wantRGB := [4]uint32{0xdc2626, 0x16a34a, 0x2563eb, 0x16a34a}
	for i, want := range wantRGB {
		c := st.BorderColorFor(BorderSide(i))
		if c == nil {
			t.Fatalf("side %d color = nil", i)
		}
		r, g, b, _ := c.RGBA()
		if got := uint32(r)<<16 | uint32(g)<<8 | uint32(b); got != want {
			t.Errorf("side %d color = %06x, want %06x", i, got, want)
		}
	}
	if st.BorderStyleFor(BorderSideTop) != BorderStyleNone ||
		st.BorderStyleFor(BorderSideBottom) != BorderStyleNone {
		t.Error("border-style:none solid 未把上下边置为 none")
	}
	if st.BorderStyleFor(BorderSideLeft) != BorderStyleSolid ||
		st.BorderStyleFor(BorderSideRight) != BorderStyleSolid {
		t.Error("border-style:none solid 未把左右边置为 solid")
	}
}

// box-shadow 的颜色函数里带逗号，不能被当成多条阴影的分隔符；颜色位置
// 允许在长度前或后，inset 关键字先记录下来（本轮只画外阴影）。
func TestParseBoxShadow(t *testing.T) {
	st := newStyle()
	applyDecl(st, "box-shadow",
		"0 1px 2px 3px rgba(0, 0, 0, 0.25), inset 1px 2px 4px #ff0000")
	got := st.BoxShadow()
	if len(got) != 2 {
		t.Fatalf("shadows = %d, want 2", len(got))
	}
	first := got[0]
	if first.X != 0 || first.Y != 1 || first.Blur != 2 || first.Spread != 3 || first.Inset {
		t.Errorf("first = %+v, want x0 y1 blur2 spread3 outer", first)
	}
	r, g, b, a := first.Color.RGBA()
	if r != 0 || g != 0 || b != 0 || a != 63 {
		t.Errorf("first color = %d,%d,%d,%d, want 0,0,0,63", r, g, b, a)
	}
	second := got[1]
	if !second.Inset || second.X != 1 || second.Y != 2 || second.Blur != 4 || second.Spread != 0 {
		t.Errorf("second = %+v, want inset 1 2 4 0", second)
	}

	applyDecl(st, "box-shadow", "0 1px bogus")
	if len(st.BoxShadow()) != 2 {
		t.Error("非法 box-shadow 应整条忽略并保留上一条合法值")
	}
	applyDecl(st, "box-shadow", "none")
	if len(st.BoxShadow()) != 0 {
		t.Error("box-shadow:none 未清空")
	}
}

// color 的 currentColor 回退由绘制层负责；解析层只需保留 nil。
func TestBorderColorCurrentColorFallback(t *testing.T) {
	st := newStyle()
	applyDecl(st, "border-width", "1px")
	applyDecl(st, "border-style", "solid")
	if st.BorderColorFor(BorderSideTop) != nil {
		t.Fatal("未声明颜色时应保持 nil，由绘制层回退 currentColor")
	}
	applyDecl(st, "border-color", "currentColor")
	if st.BorderColorFor(BorderSideTop) != nil {
		t.Fatal("显式 currentColor 也应保持 nil")
	}
}

// 宽度为 0 的边不参与绘制，颜色必须是 nil：否则圆角角块会把该边的
// currentColor（通常是黑）当成邻边颜色，`border-bottom:0` 的底角会被
// 画成黑块。这是精灵路径和逐像素路径共用的口径。
func TestBorderPaintStateZeroWidthHasNoColor(t *testing.T) {
	st := newStyle()
	applyDecl(st, "border-top", "4px solid #ff0000")
	_, colors, has := borderPaintState(st, edges{4, 0, 0, 0})
	if !has {
		t.Fatal("top border should be painted")
	}
	if colors[eBottom] != nil || colors[eRight] != nil || colors[eLeft] != nil {
		t.Fatalf("zero-width sides must have nil color: %v", colors)
	}
}

// 逐像素路径的角块选色：横向边没有颜色（宽度 0）时要回退到纵向边，
// 不能返回 nil 留下透明缺口。
func TestCornerColorFallsBackToOtherSide(t *testing.T) {
	colors := [4]Color{
		NewColor(255, 0, 0, 255), // top
		NewColor(0, 255, 0, 255), // right
		nil,                      // bottom 宽度 0
		NewColor(0, 0, 255, 255), // left
	}
	for _, p := range [][2]int{{0, 0}, {7, 0}, {0, 7}, {7, 7}} {
		if c := cornerBandColor(colors, 2, 8, p[0], p[1]); c == nil {
			t.Fatalf("corner pixel (%d,%d) fell through to nil", p[0], p[1])
		}
	}
}

// 不支持 SpriteDrawer 的后端走「多层圆角矩形 + 四条裁剪带」退化路径。
// 这里直接调用该路径，断言阴影只在边框盒外落笔：盒内透明，盒外四条带
// 都有颜色。若没有裁剪，盒内会被整块阴影填满。
func TestShadowFallbackClipsBorderBox(t *testing.T) {
	g := NewBufferGraphics(120, 100)
	red := NewColor(255, 0, 0, 255)
	// 元素盒 (40,30) 40x30；阴影向外扩 4px：(36,26) 48x38。
	paintShadowLayersClipped(g, g,
		36, 26, 48, 38, // 阴影精灵包围盒
		40, 30, 40, 30, // 元素边框盒（洞）
		36, 26, 48, 38, // 阴影本体
		0, 0, red)

	for _, p := range []struct {
		x, y int
		what string
	}{
		{60, 28, "top"},
		{60, 62, "bottom"},
		{38, 45, "left"},
		{82, 45, "right"},
	} {
		if _, _, _, a, ok := g.ColorAt(p.x, p.y); !ok || a == 0 {
			t.Errorf("%s band at (%d,%d) not painted", p.what, p.x, p.y)
		}
	}
	if _, _, _, a, ok := g.ColorAt(60, 45); !ok || a != 0 {
		t.Errorf("shadow painted inside border box: alpha=%d", a)
	}
	if _, _, _, a, ok := g.ColorAt(34, 45); !ok || a != 0 {
		t.Errorf("shadow painted outside its bounds: alpha=%d", a)
	}
}
