package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// hasInk 判断矩形区域内是否有非白像素（用于「标记是否真的画出来」）。
func hasInk(t *testing.T, buf *engine.BufferGraphics, x0, y0, x1, y1 int) bool {
	t.Helper()
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := colorAt(t, buf, x, y)
			if r != 255 || g != 255 || b != 255 {
				return true
			}
		}
	}
	return false
}

// list-style-type 真的影响绘制：none 不画标记，square 在内容左缘之外落墨。
func TestListStyleTypePixels(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		ul { margin:0; padding-left:40px; color:#ff0000; }
		#sq { list-style-type:square; }
		#no { list-style:none; }
	</style></head><body>
		<ul id="sq"><li>方块</li></ul>
		<ul id="no"><li>无标记</li></ul>
	</body></html>`
	buf, doc := openBuffered(t, 260, 180, src)

	// square：内容左缘左侧（标记区）应有红色像素。
	sq := mustEl(t, doc, "#sq li").GetBoundingClientRect()
	sx, sy := sq.Left().Pixel(), sq.Top().Pixel()
	if !hasInk(t, buf, sx-24, sy, sx-1, sy+18) {
		t.Error("list-style-type:square 没有画出标记")
	}
	// none：同一区域应保持纯白。
	no := mustEl(t, doc, "#no li").GetBoundingClientRect()
	nx, ny := no.Left().Pixel(), no.Top().Pixel()
	if hasInk(t, buf, nx-24, ny, nx-1, ny+18) {
		t.Error("list-style:none 仍画出了标记")
	}
}

// list-style-position: inside：标记落在内容盒内首行行首，文字被右移；
// outside（默认）标记在内容盒外，文字贴内容左缘。
func TestListStylePositionPixels(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin:0; background:#ffffff; }
		ul { margin:0; padding-left:40px; color:#ff0000; }
		#in li { list-style-position:inside; }
	</style></head><body>
		<ul id="in"><li>内标记</li></ul>
		<ul id="out"><li>外标记</li></ul>
	</body></html>`
	buf, doc := openBuffered(t, 260, 180, src)

	in := mustEl(t, doc, "#in li").GetBoundingClientRect()
	ix, iy := in.Left().Pixel(), in.Top().Pixel()
	if !hasInk(t, buf, ix, iy, ix+8, iy+18) {
		t.Error("inside 标记未画在内容盒内（首行行首）")
	}
	// 内容盒外（inside 不应有标记）。
	if hasInk(t, buf, ix-24, iy, ix-1, iy+18) {
		t.Error("inside 标记仍画在内容盒外")
	}

	out := mustEl(t, doc, "#out li").GetBoundingClientRect()
	ox, oy := out.Left().Pixel(), out.Top().Pixel()
	if !hasInk(t, buf, ox-24, oy, ox-1, oy+18) {
		t.Error("outside 标记未画在内容盒外")
	}
}
