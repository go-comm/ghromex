//go:build windows

package main

import (
	"image"
	"image/color"
	"image/draw"
	"os"
	"testing"
	"time"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

// 工具条区域的鼠标事件必须放行给引擎，否则“保存/关闭”按钮永远点不到；
// 图片区的事件才由拖动逻辑接管。这里用真实注入事件走一遍完整链路。
func TestPreviewToolbarClickPassesThrough(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	t.Cleanup(func() { os.Unsetenv("SDL_VIDEO_DRIVER") })

	desktop := image.NewRGBA(image.Rect(0, 0, 320, 200))
	draw.Draw(desktop, desktop.Bounds(),
		&image.Uniform{C: color.RGBA{40, 80, 120, 255}}, image.Point{}, draw.Src)

	win, err := browser.NewWithOptions(browser.Options{
		Width: 320, Height: 200, Title: "snip-preview-click",
		X: 0, Y: 0,
		Borderless: true, AlwaysOnTop: true, SkipTaskbar: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	a := newApp(win, desktop)
	if err := a.startOverlay(); err != nil {
		t.Fatal(err)
	}
	if err := a.startPreview(image.Rect(20, 20, 220, 160)); err != nil {
		t.Fatal(err)
	}
	a.doc.Refresh()

	save := a.doc.GetElementById("save")
	if save == nil {
		t.Fatal("预览页缺少 #save")
	}
	// 替换成计数器：这里只验证事件到达了按钮，不弹真实保存对话框。
	clicks := 0
	save.OnClick(func(*engine.MouseEvent) { clicks++ })
	r := save.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2
	if cx <= 0 || cy <= 0 {
		t.Fatalf("保存按钮几何异常: %+v", r)
	}

	win.SetKeyHandler(a.onKey)
	win.SetMouseHandler(a.onMouse)
	win.InjectEvent(sdl2.Event{
		Kind: sdl2.EventMouseButtonDown, Button: sdl2.MouseButtonLeft, X: cx, Y: cy,
	})
	win.InjectEvent(sdl2.Event{
		Kind: sdl2.EventMouseButtonUp, Button: sdl2.MouseButtonLeft, X: cx, Y: cy,
	})

	win.SetAutoExit(300 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatalf("保存按钮点击次数 = %d，期望 1（工具条事件被拖动逻辑吃掉了）", clicks)
	}
}
