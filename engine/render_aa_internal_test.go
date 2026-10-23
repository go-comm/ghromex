package engine

import (
	"testing"
)

// 圆角边框环的"精灵路径"（四条直边 + 四角精灵）必须与逐行逐像素路径
// 逐像素一致：环的覆盖率 = 外盒覆盖 - 内盒覆盖，边界取错会差半个 alpha。
func TestStrokeRoundedRectMatchesPixelPath(t *testing.T) {
	blue := NewColor(0x00, 0x00, 0xFF, 0xFF)

	type spec struct {
		bx, by, bw, bh, r int
		bd                edges
	}
	cases := []spec{
		{0, 0, 64, 44, 6, edges{1, 1, 1, 1}},
		{0, 0, 64, 44, 6, edges{2, 2, 2, 2}},
		{0, 0, 64, 44, 6, edges{1, 3, 2, 1}}, // 四侧宽不同
		{5, 3, 40, 30, 10, edges{1, 1, 1, 1}},
		{0, 0, 20, 20, 8, edges{1, 1, 1, 1}},  // 小盒
		{0, 0, 30, 30, 12, edges{3, 3, 3, 3}}, // 边框很厚
	}
	for _, cs := range cases {
		w := cs.bx + cs.bw + 8
		h := cs.by + cs.bh + 8
		got := NewBufferGraphics(w, h)
		want := NewBufferGraphics(w, h)
		strokeRoundedRect(got, cs.bx, cs.by, cs.bw, cs.bh, cs.r, cs.bd, blue)
		// 逐像素参照路径
		li, rt, tp, bt := maxZero(cs.bd[eLeft]), maxZero(cs.bd[eRight]),
			maxZero(cs.bd[eTop]), maxZero(cs.bd[eBottom])
		iw, ih := cs.bw-li-rt, cs.bh-tp-bt
		irs := innerCornerRadii(cs.r, li, rt, tp, bt, iw, ih)
		strokeRoundedRectPixels(want, cs.bx, cs.by, cs.bw, cs.bh, cs.r, li, rt, tp, bt,
			iw, ih, irs, blue)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				ar, ag, ab, aa, aok := got.ColorAt(x, y)
				br, bg, bb, ba, bok := want.ColorAt(x, y)
				if aok != bok || ar != br || ag != bg || ab != bb || aa != ba {
					t.Fatalf("盒 %+v：像素 (%d,%d) 精灵路径 (%d,%d,%d,%d,%v) ≠ 逐像素 (%d,%d,%d,%d,%v)",
						cs, x, y, ar, ag, ab, aa, aok, br, bg, bb, ba, bok)
				}
			}
		}
	}
}

// 底边宽度为 0 时，内圆角必须是与外圆同心的椭圆（纵向半轴仍为 r）：
// 旧实现用单一半径 ir = r - min(四侧宽) 的内圆，在底边角块的最后一列
// 会留下实色条带（视觉上像多了一条底边）。这里断言该列是渐隐的，
// 而角块内部仍有左边色。
func TestRoundedRingTapersOnZeroWidthBottom(t *testing.T) {
	g := NewBufferGraphics(100, 80)
	colors := [4]Color{
		NewColor(255, 0, 0, 255), // top
		NewColor(0, 255, 0, 255), // right
		nil,                      // bottom：宽度 0
		NewColor(0, 0, 255, 255), // left
	}
	// 盒 10,10 60×40，r=12；top/right/left 6px，bottom 0。
	strokeRoundedRectSides(g, 10, 10, 60, 40, 12, edges{6, 6, 0, 6}, colors)

	// 左下角块最后一列（x=21, y=49）：只应剩左边色向底边相切的渐隐。
	if _, _, _, a, ok := g.ColorAt(10+12-1, 10+40-1); !ok {
		t.Fatal("角块像素越界")
	} else if a >= 250 {
		t.Fatalf("底边为空时仍画出实色条带，alpha=%d", a)
	}
	// 角块内部（x=15, y=46）仍应有实心的左边色。
	if _, _, b, a, _ := g.ColorAt(10+5, 10+40-4); b < 200 || a < 200 {
		t.Fatalf("角块内部缺少左边色：b=%d a=%d", b, a)
	}
}

// 圆角填充的"只算弧边"优化必须与逐点算覆盖率的朴素实现**逐像素一致**：
// 优化把整盒扫描换成角上窄带（SDF 推导：中间行与角行中段必为全覆盖），
// 一旦边界取错就会差半个像素的 alpha。
func TestFillRoundedRectMatchesNaiveCoverage(t *testing.T) {
	red := NewColor(0xFF, 0x00, 0x00, 0xFF)

	// 朴素实现：整行逐点算覆盖率（优化前的做法）。
	naive := func(g Graphics, bx, by, bw, bh, r int, c Color) {
		for y := by; y < by+bh; y++ {
			x0, x1, ok := roundedSpanX(bx, by, bw, bh, r, y)
			if !ok {
				continue
			}
			py := float64(y) + 0.5
			lo := maxInt(bx, x0-2)
			hi := minInt(bx+bw-1, x1+2)
			paintCoverage(g, y, lo, hi, c, func(x int) float64 {
				return coverageAt(float64(x)+0.5, py, bx, by, bw, bh, r)
			})
		}
	}

	cases := []struct{ bx, by, bw, bh, r int }{
		{0, 0, 100, 40, 8},
		{0, 0, 100, 40, 1},
		{0, 0, 100, 40, 20}, // r = bh/2（半圆端）
		{3, 5, 64, 64, 12},  // 正方形
		{0, 0, 7, 5, 2},     // 盒比直径还小（无中段）
		{0, 0, 5, 5, 2},     // 极小盒
		{10, 10, 30, 18, 9}, // r 接近半高
		{0, 0, 200, 12, 6},  // 扁盒
		{0, 0, 60, 60, 24},  // r 超查表上限（走逐点回退路径）
		{40, 15, 40, 40, 9}, // portal 页的品牌角标（40×40 r=9）
	}
	for _, cs := range cases {
		w := cs.bx + cs.bw + 8
		h := cs.by + cs.bh + 8
		got := NewBufferGraphics(w, h)
		want := NewBufferGraphics(w, h)
		fillRoundedRect(got, cs.bx, cs.by, cs.bw, cs.bh, cs.r, red)
		naive(want, cs.bx, cs.by, cs.bw, cs.bh, cs.r, red)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				ar, ag, ab, aa, aok := got.ColorAt(x, y)
				br, bg, bb, ba, bok := want.ColorAt(x, y)
				if aok != bok || ar != br || ag != bg || ab != bb || aa != ba {
					t.Fatalf("盒 %+v：像素 (%d,%d) 优化后 (%d,%d,%d,%d,%v) ≠ 朴素 (%d,%d,%d,%d,%v)",
						cs, x, y, ar, ag, ab, aa, aok, br, bg, bb, ba, bok)
				}
			}
		}
	}
}
