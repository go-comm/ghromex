//go:build windows

package browser

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 清屏的颜色层：画布背景不透明时，白底那层是被完全盖住的重复填充
// （大窗体下就是一整屏的像素），必须跳过——否则每帧多填一屏。
func TestClearLayersSkipsRedundantWhite(t *testing.T) {
	// 无背景：白底一层。
	_, doc, _ := engine.OpenBufferedDocument(200, 200,
		`<!doctype html><html><body><div>x</div></body></html>`)
	if got := clearLayers(doc); len(got) != 1 {
		t.Errorf("无画布背景时清屏层数 = %d, want 1", len(got))
	}

	// 不透明背景：只有背景一层（白底被完全盖住）。
	_, doc, _ = engine.OpenBufferedDocument(200, 200,
		`<!doctype html><html><head><style>html{background-color:#f3f5f7}</style></head><body>x</body></html>`)
	layers := clearLayers(doc)
	if len(layers) != 1 {
		t.Fatalf("不透明画布背景时清屏层数 = %d, want 1（白底那层是重复填充）", len(layers))
	}
	if r, g, b, a := layers[0].RGBA(); r != 0xf3 || g != 0xf5 || b != 0xf7 || a != 255 {
		t.Errorf("清屏颜色 = %d,%d,%d,%d, want 243,245,247,255", r, g, b, a)
	}

	// 半透明背景：白底 + 背景两层（背景盖不住白底）。
	_, doc, _ = engine.OpenBufferedDocument(200, 200,
		`<!doctype html><html><head><style>html{background-color:rgba(0,0,0,0.5)}</style></head><body>x</body></html>`)
	if got := clearLayers(doc); len(got) != 2 {
		t.Errorf("半透明画布背景时清屏层数 = %d, want 2（白底不能省）", len(got))
	}
}
