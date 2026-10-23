package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

func wantPixel(t *testing.T, buf *engine.BufferGraphics, x, y int, want, where string) {
	t.Helper()
	gotR, gotG, gotB, _ := colorAt(t, buf, x, y)
	wantR, wantG, wantB := hexToRGB3(t, want)
	if gotR != wantR || gotG != wantG || gotB != wantB {
		t.Fatalf("%s at (%d,%d) = #%02x%02x%02x, want %s",
			where, x, y, gotR, gotG, gotB, want)
	}
}

func hexToRGB3(t *testing.T, s string) (uint8, uint8, uint8) {
	t.Helper()
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		t.Fatalf("bad hex %q", s)
	}
	var out [3]uint8
	for i := 0; i < 3; i++ {
		var v uint8
		for _, c := range s[i*2 : i*2+2] {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= uint8(c - '0')
			case c >= 'a' && c <= 'f':
				v |= uint8(c-'a') + 10
			case c >= 'A' && c <= 'F':
				v |= uint8(c-'A') + 10
			default:
				t.Fatalf("bad hex %q", s)
			}
		}
		out[i] = v
	}
	return out[0], out[1], out[2]
}

// 单边简写只影响对应边：上红、左蓝，底/右没有边框（露出内容背景）。
func TestPerSideBorderPixels(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#b { width:20px; height:20px; margin:10px;
		     border-top:4px solid #ff0000;
		     border-left:4px solid #0000ff;
		     background:#00ff00; }
	</style></head><body><div id="b"></div></body></html>`
	buf, doc := openBuffered(t, 80, 80, src)
	br := mustEl(t, doc, "#b").GetBoundingClientRect()
	bx := br.Left().Pixel()
	by := br.Top().Pixel()
	bw := br.Right().Pixel() - bx
	bh := br.Bottom().Pixel() - by
	if bw != 24 || bh != 24 {
		t.Fatalf("border box = %dx%d, want 24x24", bw, bh)
	}

	wantPixel(t, buf, bx+10, by+1, "#ff0000", "top border")
	wantPixel(t, buf, bx+1, by+10, "#0000ff", "left border")
	wantPixel(t, buf, bx+10, by+10, "#00ff00", "content background")
	wantPixel(t, buf, bx+10, by+bh-1, "#00ff00", "bottom (no border)")
	wantPixel(t, buf, bx+bw-1, by+10, "#00ff00", "right (no border)")
	wantPixel(t, buf, bx-1, by+10, "#ffffff", "outside")
}

// 直角边框的角区按 CSS 的斜接（miter）分色：上边 4px、左边 2px 时，
// 斜线从外角走到内角，比例是 4:2；不能让上边整块盖住左边，也不能
// 简单按 45° 切。
func TestSquareCornerMiter(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#b { width:20px; height:20px; margin:10px;
		     border-top:4px solid #ff0000;
		     border-left:2px solid #0000ff;
		     background:#ffffff; }
	</style></head><body><div id="b"></div></body></html>`
	buf, doc := openBuffered(t, 80, 80, src)
	br := mustEl(t, doc, "#b").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()

	// 上边红、左边蓝；角区前两列属于左边，其余列属于上边。
	wantPixel(t, buf, bx, by, "#ff0000", "top-left corner, top row")
	wantPixel(t, buf, bx+1, by, "#ff0000", "top-left corner, top row col 1")
	wantPixel(t, buf, bx, by+3, "#0000ff", "top-left corner, left column")
	wantPixel(t, buf, bx+1, by+3, "#0000ff", "top-left corner, left column col 1")
	wantPixel(t, buf, bx+2, by+1, "#ff0000", "top edge past corner")
}

// 下侧角块要从外角（底行）往里扫，不能和上侧共用同一个 y 方向。
func TestSquareCornerMiterBottom(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#b { width:20px; height:20px; margin:10px;
		     border-bottom:4px solid #ff0000;
		     border-right:2px solid #0000ff;
		     background:#ffffff; }
	</style></head><body><div id="b"></div></body></html>`
	buf, doc := openBuffered(t, 80, 80, src)
	br := mustEl(t, doc, "#b").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by

	// 底行最外侧属于下边（红），角块顶部（贴着内容）属于右边（蓝）。
	wantPixel(t, buf, bx+bw-1, by+bh-1, "#ff0000", "bottom-right corner, bottom row")
	wantPixel(t, buf, bx+bw-1, by+bh-4, "#0000ff", "bottom-right corner, inner row")
}

// border-color 的三值展开：top red / left+right green / bottom blue。
func TestBorderColorShorthandPixels(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#b { width:20px; height:20px; margin:10px;
		     border-width:2px;
		     border-color:#ff0000 #00ff00 #0000ff;
		     background:#ffffff; }
	</style></head><body><div id="b"></div></body></html>`
	buf, doc := openBuffered(t, 80, 80, src)
	br := mustEl(t, doc, "#b").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by

	wantPixel(t, buf, bx+10, by+1, "#ff0000", "top border")
	wantPixel(t, buf, bx+bw-1, by+10, "#00ff00", "right border")
	wantPixel(t, buf, bx+10, by+bh-1, "#0000ff", "bottom border")
	wantPixel(t, buf, bx+1, by+10, "#00ff00", "left border")
	wantPixel(t, buf, bx+10, by+10, "#ffffff", "content background")
}

// border-*-style:none 的那条边不画；但布局仍为它保留了宽度。
func TestBorderStyleNoneSideNotPainted(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#b { width:20px; height:20px; margin:10px;
		     border:4px solid #ff0000;
		     border-left-style:none;
		     background:#00ff00; }
	</style></head><body><div id="b"></div></body></html>`
	buf, doc := openBuffered(t, 80, 80, src)
	br := mustEl(t, doc, "#b").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()

	wantPixel(t, buf, bx+10, by+1, "#ff0000", "top border")
	// 左边框宽度仍在布局盒里，但 style:none → 背景铺满该区域。
	wantPixel(t, buf, bx+1, by+10, "#00ff00", "left style:none")
}

// 无模糊的 spread 阴影：外向扩 4px，背景必须压在阴影上面。
func TestBoxShadowSpreadPixels(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:40px; height:30px; margin:20px;
		     background:#ffffff;
		     box-shadow:0 0 0 4px #ff0000; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 120, 100, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by

	wantPixel(t, buf, bx-1, by+bh/2, "#ff0000", "shadow just outside left")
	wantPixel(t, buf, bx-4, by+bh/2, "#ff0000", "shadow at spread edge")
	wantPixel(t, buf, bx-5, by+bh/2, "#ffffff", "outside spread")
	wantPixel(t, buf, bx+bw/2, by+bh/2, "#ffffff", "background on top")
	wantPixel(t, buf, bx+bw/2, by-1, "#ff0000", "shadow above box")
}

// 模糊阴影在盒外产生半透明红色：近处红通道占优，远处回到白底。
func TestBoxShadowBlurSoftEdge(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:40px; height:30px; margin:20px;
		     background:#ffffff;
		     box-shadow:0 0 8px 0 #ff0000; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 120, 100, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()
	bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by

	r, g, b, _ := colorAt(t, buf, bx-3, by+bh/2)
	if r <= g || r <= b {
		t.Fatalf("blur shadow = #%02x%02x%02x, want red-dominant near edge", r, g, b)
	}
	if g > 250 || b > 250 {
		t.Fatalf("blur shadow near edge too light: #%02x%02x%02x", r, g, b)
	}
	wantPixel(t, buf, bx+bw/2, by+bh/2, "#ffffff", "background on top")
	wantPixel(t, buf, bx-12, by+bh/2, "#ffffff", "far outside")
}

// 外阴影只存在于边框盒外侧。即使元素背景透明，盒内也不能透出阴影——
// CSS 把外阴影裁在 border-box 之外，而不是靠背景去盖。
func TestBoxShadowClippedInsideBorderBox(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#blur { width:40px; height:30px; margin:20px;
		        box-shadow:0 0 8px 0 #ff0000; }
		#sharp { width:40px; height:30px; margin:20px 20px 0 80px;
		         box-shadow:0 0 0 4px #ff0000; }
	</style></head><body><div id="blur"></div><div id="sharp"></div></body></html>`
	buf, doc := openBuffered(t, 160, 120, src)
	for _, sel := range []string{"#blur", "#sharp"} {
		br := mustEl(t, doc, sel).GetBoundingClientRect()
		bx, by := br.Left().Pixel(), br.Top().Pixel()
		bw, bh := br.Right().Pixel()-bx, br.Bottom().Pixel()-by
		wantPixel(t, buf, bx+bw/2, by+bh/2, "#ffffff", sel+" interior")
	}
	// 同时确认阴影确实画到了盒外，排除「整条阴影都没画」的假通过。
	br := mustEl(t, doc, "#blur").GetBoundingClientRect()
	r, g, b, _ := colorAt(t, buf, br.Left().Pixel()-3, br.Top().Pixel()+10)
	if r <= g || r <= b {
		t.Fatalf("blur outside = #%02x%02x%02x, want red-dominant", r, g, b)
	}
}

// 第一条阴影在顶层：红色 2px 压住蓝色 6px。
func TestBoxShadowMultipleOrder(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:40px; height:30px; margin:20px;
		     background:#ffffff;
		     box-shadow:0 0 0 2px #ff0000, 0 0 0 6px #0000ff; }
	</style></head><body><div id="s"></div></body></html>`
	buf, doc := openBuffered(t, 120, 100, src)
	br := mustEl(t, doc, "#s").GetBoundingClientRect()
	bx, by := br.Left().Pixel(), br.Top().Pixel()

	wantPixel(t, buf, bx-1, by+15, "#ff0000", "first shadow on top")
	wantPixel(t, buf, bx-4, by+15, "#0000ff", "second shadow beyond first")
}

// 单边边框与阴影都要出现在 SVG 导出里；四条边的颜色不能被合成一个 stroke。
func TestBorderShadowSVG(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		#s { width:20px; height:20px; margin:10px; background:#ffffff;
		     border-top:2px solid #123456;
		     border-right:2px solid #654321;
		     border-bottom:2px solid #abcdef;
		     border-left:2px solid #fedcba;
		     box-shadow:0 1px 4px #112233; }
	</style></head><body><div id="s"></div></body></html>`
	doc := openDoc(t, 80, 80, src)
	svg := doc.DumpSVG()
	for _, want := range []string{
		`fill="#123456"`, `fill="#654321"`, `fill="#abcdef"`, `fill="#fedcba"`,
		`feGaussianBlur`, `fill="#112233"`,
	} {
		if !strings.Contains(svg, want) {
			t.Fatalf("SVG missing %s:\n%s", want, svg)
		}
	}
}
