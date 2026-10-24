//go:build windows

package browser

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// SavePNG 重排 → 渲染 → 存盘一把过：文件存在、尺寸对得上、颜色来自这一帧。
// dummy 驱动没有真实字体，文字是占位方块，断言只看色块。
func TestWindowSavePNG(t *testing.T) {
	if os.Getenv("GHROMEX_REAL") != "1" {
		os.Setenv("SDL_VIDEO_DRIVER", "dummy")
		defer os.Unsetenv("SDL_VIDEO_DRIVER")
	}

	win, err := New(60, 40, "savepng-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer win.Close()
	if _, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">` +
		`<div style="width:60px;height:40px;background:#00FF00"></div>` +
		`</body></html>`); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "shot.png")
	if err := win.SavePNG(out); err != nil {
		t.Fatalf("SavePNG: %v", err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 60 || b.Dy() != 40 {
		t.Fatalf("PNG 尺寸 = %dx%d, want 60x40", b.Dx(), b.Dy())
	}
	r, g, bb, _ := img.At(30, 20).RGBA()
	if r>>8 != 0 || g>>8 != 255 || bb>>8 != 0 {
		t.Fatalf("中心像素 = (%d,%d,%d), want (0,255,0)（绿块没画出来）", r>>8, g>>8, bb>>8)
	}
}

// SavePNG 自带重排：改完文档不手动 Refresh，截到的也必须是被改后的布局。
func TestSavePNGRefreshesBeforeCapture(t *testing.T) {
	if os.Getenv("GHROMEX_REAL") != "1" {
		os.Setenv("SDL_VIDEO_DRIVER", "dummy")
		defer os.Unsetenv("SDL_VIDEO_DRIVER")
	}

	win, err := New(60, 40, "savepng-relayout")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer win.Close()
	doc, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">` +
		`<div id="a" style="width:60px;height:40px;background:#FF0000"></div>` +
		`</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	// 首帧渲染过一遍红底后改色：没有内部 Refresh 的话 PNG 仍是红。
	win.RenderFrame()
	doc.QuerySelector("#a").SetAttribute("style", "width:60px;height:40px;background:#0000FF")

	out := filepath.Join(t.TempDir(), "shot.png")
	if err := win.SavePNG(out); err != nil {
		t.Fatalf("SavePNG: %v", err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	r, g, bb, _ := img.At(30, 20).RGBA()
	if r>>8 != 0 || g>>8 != 0 || bb>>8 != 255 {
		t.Fatalf("变更后像素 = (%d,%d,%d), want (0,0,255)（SavePNG 没有先重排）", r>>8, g>>8, bb>>8)
	}
}
