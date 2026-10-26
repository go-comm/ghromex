package engine

import "testing"

// 形状几何：spread 向内收缩、偏移平移，半径随之收窄；收缩到空时 ok=false。
func TestInsetShadowShape(t *testing.T) {
	// padding 盒 (10,20) 100x60、内圆角 8；inset 3px 4px 0 5px。
	x, y, w, h, r, ok := insetShadowShape(10, 20, 100, 60, 8,
		Shadow{X: 3, Y: 4, Spread: 5, Inset: true})
	if !ok {
		t.Fatal("形状不应为空")
	}
	if x != 10+3+5 || y != 20+4+5 || w != 100-10 || h != 60-10 {
		t.Errorf("shape = (%d,%d) %dx%d, want (18,29) 90x50", x, y, w, h)
	}
	if r != 3 {
		t.Errorf("内圆角 = %d, want 3（8 − 5）", r)
	}

	// 负 spread 向外扩张，半径变大但不超过短边一半。
	x, y, w, h, r, ok = insetShadowShape(0, 0, 40, 20, 4,
		Shadow{Spread: -6, Inset: true})
	if !ok || x != -6 || y != -6 || w != 52 || h != 32 {
		t.Fatalf("负 spread shape = (%d,%d) %dx%d ok=%v, want (-6,-6) 52x32", x, y, w, h, ok)
	}
	if r != 10 {
		t.Errorf("负 spread 半径 = %d, want 10（4 + 6）", r)
	}

	// spread 收缩到空：整块 padding 盒都落在阴影里。
	if _, _, _, _, _, ok := insetShadowShape(0, 0, 10, 10, 0,
		Shadow{Spread: 5, Inset: true}); ok {
		t.Error("spread 收缩到空时 ok 应为 false")
	}
}

// 精灵掩码：形状之外 alpha 满、形状之内为 0；padding 盒外的模糊外扩
// 区域不落笔（裁切在精灵里完成）。
func TestInsetShadowSpriteMask(t *testing.T) {
	red := NewColor(255, 0, 0, 255)
	// padding 盒 20x20、圆角 0、无模糊；形状 = 收缩 4px 后的 12x12（居中）。
	s := insetShadowSprite(20, 20, 0, 0, red, 4, 4, 12, 12, 0)
	if s.W != 20 || s.H != 20 {
		t.Fatalf("sprite = %dx%d, want 20x20", s.W, s.H)
	}
	alphaAt := func(x, y int) uint8 { return s.Pix[(y*s.W+x)*4+3] }
	if a := alphaAt(1, 10); a != 255 {
		t.Errorf("ring alpha at (1,10) = %d, want 255", a)
	}
	if a := alphaAt(10, 10); a != 0 {
		t.Errorf("shape interior alpha at (10,10) = %d, want 0", a)
	}
	if a := alphaAt(10, 1); a != 255 {
		t.Errorf("ring alpha at (10,1) = %d, want 255", a)
	}
}

// 退化路径（后端无 SpriteDrawer）：裁到 padding 盒，形状之外四条带落笔，
// 形状内与盒外都不落笔。
func TestInsetShadowFallbackClipped(t *testing.T) {
	g := NewBufferGraphics(120, 100)
	red := NewColor(255, 0, 0, 255)
	// padding 盒 (20,20) 40x30；形状收缩 5px 后 (25,25) 30x20。
	paintInsetShadowFallback(g, 20, 20, 40, 30, 25, 25, 30, 20, red)

	for _, p := range []struct {
		x, y int
		what string
	}{
		{40, 22, "top band"},
		{40, 47, "bottom band"},
		{22, 35, "left band"},
		{57, 35, "right band"},
	} {
		if _, _, _, a, ok := g.ColorAt(p.x, p.y); !ok || a == 0 {
			t.Errorf("%s at (%d,%d) not painted", p.what, p.x, p.y)
		}
	}
	if _, _, _, a, ok := g.ColorAt(40, 35); !ok || a != 0 {
		t.Errorf("shape interior painted: alpha=%d", a)
	}
	if _, _, _, a, ok := g.ColorAt(18, 35); !ok || a != 0 {
		t.Errorf("painted outside padding box: alpha=%d", a)
	}
	if _, _, _, a, ok := g.ColorAt(40, 18); !ok || a != 0 {
		t.Errorf("painted above padding box: alpha=%d", a)
	}
}
