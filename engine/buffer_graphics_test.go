package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 打开页面并把绘制指令落在 BufferGraphics 上，返回缓冲与文档。
func openBuffered(t *testing.T, w, h int, src string) (*engine.BufferGraphics, engine.HTMLDocument) {
	t.Helper()
	buf := engine.NewBufferGraphics(w, h)
	vp := &engine.HeadlessViewport{W: w, H: h, G: buf}
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	engine.RenderNode(buf, doc)
	return buf, doc
}

const bufferPage = `<!doctype html>
<html>
<head>
<style>
	body { margin: 0; padding: 24px; background-color: #EDEEF2; }
	.card { width: 320px; margin: 24px; padding: 20px; background-color: #FFFFFF; border: 1px solid #C8C8C8; }
	.title { font-size: 20px; font-weight: bold; color: #1F2937; }
	.btn-primary { background-color: #2563EB; color: #FFFFFF; }
</style>
</head>
<body>
	<div class="card">
		<span class="title">登录</span>
		<div><button class="btn-primary" id="login">登录</button></div>
	</div>
</body>
</html>`

func colorAt(t *testing.T, buf *engine.BufferGraphics, x, y int) (uint8, uint8, uint8, uint8) {
	t.Helper()
	r, g, b, a, ok := buf.ColorAt(x, y)
	if !ok {
		t.Fatalf("ColorAt(%d,%d) out of buffer", x, y)
	}
	return r, g, b, a
}

func TestBufferPaintsBackgroundAndBlocks(t *testing.T) {
	buf, doc := openBuffered(t, 400, 300, bufferPage)

	// 1. 页面背景：body background-color 覆盖 (2,2)
	if r, g, b, a := colorAt(t, buf, 2, 2); r != 0xED || g != 0xEE || b != 0xF2 || a != 0xFF {
		t.Fatalf("page bg = %02X%02X%02X%02X, want EDEEF2FF", r, g, b, a)
	}

	// 2. 卡片中心应为白底
	card := doc.QuerySelector(".card")
	if card == nil {
		t.Fatal(".card not found")
	}
	br := card.GetBoundingClientRect()
	cx := (br.Left().Pixel() + br.Right().Pixel()) / 2
	cy := (br.Top().Pixel() + br.Bottom().Pixel()) / 2
	if r, g, b, a := colorAt(t, buf, cx, cy); r != 0xFF || g != 0xFF || b != 0xFF || a != 0xFF {
		t.Fatalf("card center(%d,%d) = %02X%02X%02X%02X, want FFFFFFFF", cx, cy, r, g, b, a)
	}

	// 3. 按钮附近应能找到 #2563EB 蓝底像素（避开文字白色区，横向扫描一圈）
	btn := doc.QuerySelector("#login")
	if btn == nil {
		t.Fatal("#login not found")
	}
	nr := btn.GetBoundingClientRect()
	midY := (nr.Top().Pixel() + nr.Bottom().Pixel()) / 2
	blue := 0
	for x := nr.Left().Pixel() - 12; x <= nr.Right().Pixel()+12; x++ {
		if r, g, b, _ := colorAt(t, buf, x, midY); r == 0x25 && g == 0x63 && b == 0xEB {
			blue++
		}
	}
	if blue < 8 {
		t.Fatalf("expected blue pixels around button at y=%d, got %d", midY, blue)
	}

	// 4. 标题区域应出现深色文字块（#1F2937）
	title := doc.QuerySelector(".title")
	if title == nil {
		t.Fatal(".title not found")
	}
	tr := title.GetBoundingClientRect()
	dark := 0
	for y := tr.Top().Pixel(); y < tr.Bottom().Pixel(); y++ {
		for x := tr.Left().Pixel(); x < tr.Right().Pixel(); x++ {
			if r, g, b, a := colorAt(t, buf, x, y); a == 0xFF && r < 0x40 && g < 0x40 && b < 0x60 {
				dark++
			}
		}
	}
	if dark == 0 {
		t.Fatal("no dark title pixels inside .title box")
	}

	// 5. 文本调用记录（顺序即绘制顺序）
	joined := len(buf.Texts)
	if joined < 2 {
		t.Fatalf("expected >=2 DrawText calls, got %d", joined)
	}
}

func TestBufferClipsOutOfBounds(t *testing.T) {
	buf := engine.NewBufferGraphics(8, 8)
	buf.DrawColor(-4, -4, 8, 8, engine.NewColor(255, 0, 0, 255))
	buf.DrawColor(6, 6, 20, 20, engine.NewColor(0, 255, 0, 255))
	buf.DrawColor(100, 100, 10, 10, engine.NewColor(0, 0, 255, 255))

	if r, g, b, _ := colorAt(t, buf, 0, 0); r != 255 || g != 0 || b != 0 {
		t.Fatalf("clipped negative rect: (0,0) = %d,%d,%d want red", r, g, b)
	}
	if r, g, b, _ := colorAt(t, buf, 7, 7); g != 255 {
		t.Fatalf("clipped overflow rect: (7,7) = %d,%d,%d want green", r, g, b)
	}
	if _, _, _, _, ok := buf.ColorAt(8, 8); ok {
		t.Fatal("ColorAt(8,8) should be out of bounds")
	}
	if _, _, _, _, ok := buf.ColorAt(-1, 0); ok {
		t.Fatal("ColorAt(-1,0) should be out of bounds")
	}
}

func TestBufferAlphaBlend(t *testing.T) {
	buf := engine.NewBufferGraphics(4, 4)
	buf.DrawColor(0, 0, 4, 4, engine.NewColor(255, 255, 255, 255))
	buf.DrawColor(1, 1, 2, 2, engine.NewColor(255, 0, 0, 128))

	r, g, b, a := colorAt(t, buf, 2, 2)
	// 50% 红覆盖白：r=255，g/b≈127，a=255
	if r != 255 || g < 125 || g > 129 || b < 125 || b > 129 || a != 255 {
		t.Fatalf("blend = %d,%d,%d,%d, want ~255,127,127,255", r, g, b, a)
	}
	// 未覆盖处仍是纯白
	if r, g, b, _ := colorAt(t, buf, 0, 0); r != 255 || g != 255 || b != 255 {
		t.Fatalf("untouched = %d,%d,%d want white", r, g, b)
	}
}

func TestBufferDrawImagePlaceholder(t *testing.T) {
	buf := engine.NewBufferGraphics(4, 4)
	buf.DrawImage(1, 1, 2, 2, nil)
	if r, g, b, _ := colorAt(t, buf, 2, 2); r != 255 || b != 255 || g != 0 {
		t.Fatalf("image placeholder = %d,%d,%d want magenta", r, g, b)
	}
}
