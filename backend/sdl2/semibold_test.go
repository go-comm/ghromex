//go:build windows

package sdl2

import (
	"path/filepath"
	"testing"
)

// TestFontTierOf 数值字重归档：550~649 落半粗档、650+ 落粗体档、
// 550 以下正体；未携带数值（weightNum=0，UA 直接 SetBold 的 legacy
// 路径）按两档布尔退化为 0/2。
func TestFontTierOf(t *testing.T) {
	cases := []struct {
		bold bool
		num  int
		want int
	}{
		{false, 0, tierRegular},
		{true, 0, tierBold},
		{false, 400, tierRegular},
		{false, 500, tierRegular},
		{true, 550, tierSemibold},
		{true, 600, tierSemibold},
		{true, 649, tierSemibold},
		{true, 650, tierBold},
		{true, 700, tierBold},
		{true, 900, tierBold},
	}
	for _, c := range cases {
		if got := fontTierOf(c.bold, c.num); got != c.want {
			t.Errorf("fontTierOf(%v,%d) = %d, want %d", c.bold, c.num, got, c.want)
		}
	}
}

// TestSemiboldFacePath Segoe UI 半粗档必须解析到 seguisb.ttf（真
// Semibold 面，Chrome 同口径），粗体档仍是 segoeuib.ttf；无半粗面的
// 族（雅黑）半粗档落粗体面——Chrome 对 {400,700} 族 600→700 就近选面。
func TestSemiboldFacePath(t *testing.T) {
	regular := fontPathOf("segoeui.ttf")
	if regular == "" {
		t.Skip("本机无 segoeui.ttf")
	}
	g := NewGraphics(0, "")
	defer g.CloseFonts()
	if fontPathOf("seguisb.ttf") == "" {
		t.Skip("本机无 seguisb.ttf（Segoe UI Semibold）")
	}
	if got := filepath.Base(g.facePath(regular, 32, tierSemibold)); got != "seguisb.ttf" {
		t.Errorf("Segoe UI 半粗档解析为 %s, want seguisb.ttf", got)
	}
	if got := filepath.Base(g.facePath(regular, 32, tierBold)); got != "segoeuib.ttf" {
		t.Errorf("Segoe UI 粗体档解析为 %s, want segoeuib.ttf", got)
	}
	if got := filepath.Base(g.facePath(regular, 32, tierRegular)); got != "segoeui.ttf" {
		t.Errorf("Segoe UI 正体档解析为 %s, want segoeui.ttf", got)
	}
	if msyh := fontPathOf("msyh.ttc"); msyh != "" {
		if got := filepath.Base(g.facePath(msyh, 32, tierSemibold)); got != "msyhbd.ttc" {
			t.Errorf("雅黑半粗档（族无 Semibold 面）应落粗体面 msyhbd.ttc，实际 %s", got)
		}
	}
}

// TestMeasureTextWeightSemibold 同一拉丁串 600 档与 700 档宽度必须
// 不同（Chrome 实测 32px "2026-10-23 AI"：Regular 201.5 / Semibold
// 202.7 / Bold 214.6）。若 600 档与 700 档同宽，说明档位归档或变体
// 解析失效（会议纪要页标题曾整体粗一圈即此 bug）。
func TestMeasureTextWeightSemibold(t *testing.T) {
	if err := Load(); err != nil {
		t.Skip(err)
	}
	if ttfInit() != 0 {
		t.Skip("TTF_Init 失败")
	}
	defer ttfQuit()
	g := NewGraphics(0, "")
	defer g.CloseFonts()
	if fontPathOf("seguisb.ttf") == "" {
		t.Skip("本机无 seguisb.ttf（Segoe UI Semibold）")
	}
	const text = "2026-10-23 AI"
	w400, _ := g.MeasureTextWeight(text, 32, 400, "Segoe UI")
	w600, _ := g.MeasureTextWeight(text, 32, 600, "Segoe UI")
	w700, _ := g.MeasureTextWeight(text, 32, 700, "Segoe UI")
	if w600 >= w700 {
		t.Errorf("600 档宽 %d 应窄于 700 档 %d（Semibold vs Bold 面）", w600, w700)
	}
	// 600 档应贴近 400 档（Semibold 与 Regular advance 差 ~1%），
	// 若偏移超过 700 档差值的一半，说明没选中 Semibold 面。
	if d6, d7 := w600-w400, w700-w400; d7 > 0 && d6*2 > d7 {
		t.Errorf("600 档偏移过大（d6=%d, d7=%d），疑似落到 Bold 面", d6, d7)
	}
	// legacy 两档口径不受扰：MeasureText(bold=true) 必须与 700 档同宽。
	if wBold, _ := g.MeasureText(text, 32, true, "Segoe UI"); wBold != w700 {
		t.Errorf("legacy bold 宽 %d 与 700 档 %d 不一致（两档归档回归）", wBold, w700)
	}
}
