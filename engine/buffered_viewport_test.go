package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// NewBufferedViewport：视口与缓冲同尺寸，且缓冲就是视口的绘制后端——
// 绘制指令落进缓冲，才能逐像素断言。
func TestNewBufferedViewportWiresBuffer(t *testing.T) {
	vp, buf := engine.NewBufferedViewport(120, 80)
	if vp.ViewportWidth() != 120 || vp.ViewportHeight() != 80 {
		t.Fatalf("视口尺寸 = %dx%d, want 120x80", vp.ViewportWidth(), vp.ViewportHeight())
	}
	if buf.W != 120 || buf.H != 80 {
		t.Fatalf("缓冲尺寸 = %dx%d, want 120x80", buf.W, buf.H)
	}
	if vp.Graphics() != engine.Graphics(buf) {
		t.Fatal("视口的绘制后端不是返回的缓冲")
	}
}

// OpenBufferedDocument：一次建好「视口 + 缓冲」并完成首次绘制，
// 像素断言能直接读到画上去的颜色。
func TestOpenBufferedDocumentRendersIntoBuffer(t *testing.T) {
	buf, doc, err := engine.OpenBufferedDocument(120, 80,
		`<!doctype html><html><body style="margin:0">`+
			`<div style="width:40px;height:20px;background-color:#FF0000"></div>`+
			`</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		t.Fatal("文档为 nil")
	}
	r, g, b, _, ok := buf.ColorAt(10, 10)
	if !ok {
		t.Fatal("ColorAt 越界")
	}
	if r != 255 || g != 0 || b != 0 {
		t.Fatalf("像素 = %d,%d,%d, want 255,0,0", r, g, b)
	}
}
