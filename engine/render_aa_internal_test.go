package engine

import (
	"testing"
)

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
		{0, 0, 7, 5, 3},     // 盒比直径还小（无中段）
		{0, 0, 5, 5, 2},     // 极小盒
		{10, 10, 30, 18, 9}, // r 接近半高
		{0, 0, 200, 12, 6},  // 扁盒
		{0, 0, 60, 60, 24},  // r 超查表上限（走逐点回退路径）
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
