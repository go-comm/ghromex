package engine

import "testing"

// 下划线必须从基线往下画，而不是从片段盒底往上找位置。SDL_ttf 的 descent
// 较小（Noto 16px 只有 3px），按盒底定位会让下划线贴到基线上甚至更高。
func TestUnderlineBelowBaseline(t *testing.T) {
	g := NewBufferGraphics(60, 40)
	// ascent 15、行片段高 18、字号 16：线厚 2，下划线顶边应在基线 + 2 = 17。
	drawDecoration(g, TextDecorationUnderline, 0, 0, 40, 18, 15, 16, nil)

	for _, y := range []int{17, 18} {
		_, _, _, a, ok := g.ColorAt(0, y)
		if !ok || a == 0 {
			t.Fatalf("下划线行 y=%d 未绘制（应在基线 15 下方）", y)
		}
	}
	for _, y := range []int{15, 16} {
		if _, _, _, a, _ := g.ColorAt(0, y); a != 0 {
			t.Fatalf("下划线画到了基线/基线以上（y=%d）", y)
		}
	}
}
