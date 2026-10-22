//go:build windows

package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-comm/ghromex/browser"
)

// 无头端到端：假桌面 → 选区页 → 预览页，两张图都要能解码、尺寸对、
// 关键像素对（背景图真的画出来了、遮罩真的变暗、工具条在正确位置）。
func TestRenderShots(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	t.Cleanup(func() { os.Unsetenv("SDL_VIDEO_DRIVER") })

	const (
		w, h = 320, 200
		sr   = 10
		sg   = 120
		sb   = 200
	)
	desktop := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(desktop, desktop.Bounds(),
		&image.Uniform{C: color.RGBA{sr, sg, sb, 255}}, image.Point{}, draw.Src)

	win, err := browser.NewWithOptions(browser.Options{
		Width: w, Height: h, Title: "snip-test",
		X: 0, Y: 0,
		Borderless: true, AlwaysOnTop: true, SkipTaskbar: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	a := newApp(win, desktop)
	overlayPath := filepath.Join(t.TempDir(), "overlay.png")
	previewPath := filepath.Join(t.TempDir(), "preview.png")
	if err := a.renderShots(overlayPath, previewPath); err != nil {
		t.Fatal(err)
	}

	ov := decodePNG(t, overlayPath)
	if got := ov.Bounds().Size(); got != (image.Point{w, h}) {
		t.Fatalf("选区页尺寸 = %v，期望 %v", got, image.Pt(w, h))
	}

	r := demoRect(w, h)
	barTop, winH := previewLayout(r.Dy(), h)
	pv := decodePNG(t, previewPath)
	if got := pv.Bounds().Size(); got != (image.Point{r.Dx(), winH}) {
		t.Fatalf("预览页尺寸 = %v，期望 %v", got, image.Pt(r.Dx(), winH))
	}

	// 选区内部保持原色，外部被暗化。
	if !closeRGB(rgbAt(ov, r.Min.X+10, r.Min.Y+10), sr, sg, sb) {
		t.Fatalf("选区内部颜色 = %v，期望 (%d,%d,%d)", rgbAt(ov, r.Min.X+10, r.Min.Y+10), sr, sg, sb)
	}
	outside := rgbAt(ov, 5, 5)
	if outside.G >= sg || outside.B >= sb {
		t.Fatalf("选区外没有被暗化: %v", outside)
	}

	// 预览：图片区 1:1，工具条是深色。
	if !closeRGB(rgbAt(pv, 5, 5), sr, sg, sb) {
		t.Fatalf("预览图片区颜色 = %v，期望 (%d,%d,%d)", rgbAt(pv, 5, 5), sr, sg, sb)
	}
	bar := rgbAt(pv, 5, barTop+8)
	if bar.R > 100 || bar.G > 100 || bar.B > 100 {
		t.Fatalf("工具条像素不够深: %v", bar)
	}
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func rgbAt(img image.Image, x, y int) color.RGBA {
	r, g, b, _ := img.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

func closeRGB(c color.RGBA, r, g, b int) bool {
	d := func(a uint8, b int) int {
		n := int(a) - b
		if n < 0 {
			n = -n
		}
		return n
	}
	return d(c.R, r) <= 2 && d(c.G, g) <= 2 && d(c.B, b) <= 2
}
