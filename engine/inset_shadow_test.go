package engine_test

import (
	"strings"
	"testing"
)

// inset 阴影：padding 盒内侧一圈实色（spread 4px），盒内中心与盒外都不画。
func TestInsetBoxShadowSpreadPixels(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:40px; height:30px; margin:20px;
		     background:#ffffff;
		     box-shadow:inset 0 0 0 4px #ff0000; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 120, 100, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by

	wantPixel(t, buf, bx+1, by+bh/2, "#ff0000", "inset shadow at left edge")
	wantPixel(t, buf, bx+bw/2, by+1, "#ff0000", "inset shadow at top edge")
	wantPixel(t, buf, bx+bw-2, by+bh/2, "#ff0000", "inset shadow at right edge")
	wantPixel(t, buf, bx+bw/2, by+bh-2, "#ff0000", "inset shadow at bottom edge")
	wantPixel(t, buf, bx+5, by+bh/2, "#ffffff", "inside the ring (past 4px)")
	wantPixel(t, buf, bx+bw/2, by+bh/2, "#ffffff", "center untouched")
	wantPixel(t, buf, bx-1, by+bh/2, "#ffffff", "outside border box untouched")
}

// inset 阴影按偏移移动「不投影形状」：6px 右移 → 左侧 6px 是阴影，右侧
// 形状被推到盒外，不再有阴影。
func TestInsetBoxShadowOffset(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:40px; height:30px; margin:20px;
		     background:#ffffff;
		     box-shadow:inset 6px 0 0 0 #ff0000; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 120, 100, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by

	wantPixel(t, buf, bx+2, by+bh/2, "#ff0000", "left band from offset")
	wantPixel(t, buf, bx+5, by+bh/2, "#ff0000", "left band ends at 6px")
	wantPixel(t, buf, bx+7, by+bh/2, "#ffffff", "past the offset band")
	wantPixel(t, buf, bx+bw-2, by+bh/2, "#ffffff", "right edge (shape pushed out)")
}

// inset 模糊：边缘红通道占优（半透明内发光），中心回到白底，盒外不落笔。
func TestInsetBoxShadowBlurSoftEdge(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:60px; height:44px; margin:20px;
		     background:#ffffff;
		     box-shadow:inset 0 0 8px #ff0000; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 140, 120, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by

	r, g, b, _ := colorAt(t, buf, bx+1, by+bh/2)
	if r <= g || r <= b {
		t.Fatalf("inset blur at edge = #%02x%02x%02x, want red-dominant", r, g, b)
	}
	if r < 200 {
		t.Fatalf("inset blur at edge too weak: #%02x%02x%02x", r, g, b)
	}
	wantPixel(t, buf, bx+bw/2, by+bh/2, "#ffffff", "center untouched")
	wantPixel(t, buf, bx-1, by+bh/2, "#ffffff", "outside border box untouched")
}

// 内阴影按 padding 盒裁切：边框区域保持边框色，阴影只出现在边框内侧。
func TestInsetBoxShadowClippedToPaddingBox(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:40px; height:30px; margin:20px;
		     border:4px solid #00ff00;
		     background:#ffffff;
		     box-shadow:inset 0 0 0 4px #ff0000; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 140, 120, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by

	wantPixel(t, buf, bx+1, by+bh/2, "#00ff00", "border keeps its color")
	wantPixel(t, buf, bx+5, by+bh/2, "#ff0000", "inset shadow inside border")
	wantPixel(t, buf, bx+bw/2, by+bh/2, "#ffffff", "content area untouched")
}

// 多条 inset 阴影：第一条在最上层（4px 红压住 8px 蓝）。
func TestInsetBoxShadowMultipleOrder(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:40px; height:30px; margin:20px;
		     background:#ffffff;
		     box-shadow:inset 0 0 0 4px #ff0000, inset 0 0 0 8px #0000ff; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 120, 100, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bh := br.Bottom().Pixel() - by

	wantPixel(t, buf, bx+2, by+bh/2, "#ff0000", "first inset shadow on top")
	wantPixel(t, buf, bx+6, by+bh/2, "#0000ff", "second inset shadow beyond first")
	wantPixel(t, buf, bx+10, by+bh/2, "#ffffff", "past both rings")
}

// 外阴影与内阴影可以同时存在：内阴影在盒内，外阴影在盒外，互不覆盖。
func TestInsetAndOuterShadowTogether(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:40px; height:30px; margin:20px;
		     background:#ffffff;
		     box-shadow:inset 0 0 0 3px #ff0000, 0 0 0 3px #0000ff; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 140, 120, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bh := br.Bottom().Pixel() - by

	wantPixel(t, buf, bx+1, by+bh/2, "#ff0000", "inset inside")
	wantPixel(t, buf, bx-1, by+bh/2, "#0000ff", "outer outside")
}

// inset 阴影要出现在 SVG 导出里：用 mask 挖形状 + clipPath 裁 padding 盒，
// 模糊走 feGaussianBlur（与渲染路径同口径）。
func TestInsetBoxShadowSVG(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:20px; height:20px; margin:10px; background:#ffffff;
		     box-shadow:inset 0 0 4px #112233; }
	</style></head><body><div id="s"></div></body></html>`
	doc := openDoc(t, 80, 80, src)
	svg := doc.DumpSVG()
	for _, want := range []string{
		`<clipPath`, `fill="#112233"`, `feGaussianBlur`,
	} {
		if !strings.Contains(svg, want) {
			t.Fatalf("SVG missing %s:\n%s", want, svg)
		}
	}
}
