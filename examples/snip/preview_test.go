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

// 拖动合并：motion 事件只记目标位置，帧末 onFrame 才落成一次 MoveTo。
// 逐事件 MoveTo 会让窗口在一个事件批次里跳好几次（配合 10ms 空闲休眠就是
// 「拖动一顿一顿」），所以这里断言事件处理阶段窗口位置不动。
func TestPreviewDragAppliesOncePerFrame(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	t.Cleanup(func() { os.Unsetenv("SDL_VIDEO_DRIVER") })

	desktop := image.NewRGBA(image.Rect(0, 0, 320, 200))
	draw.Draw(desktop, desktop.Bounds(),
		&image.Uniform{C: color.RGBA{40, 80, 120, 255}}, image.Point{}, draw.Src)

	win, err := browser.NewWithOptions(browser.Options{
		Width: 320, Height: 200, Title: "snip-preview-drag",
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
	p := a.prev
	x0, y0 := p.x, p.y

	move := func(x, y int) {
		a.onMouse(browser.MouseEvent{Kind: browser.MouseMove, X: x, Y: y})
	}

	// 按下 (30,20)：抓取偏移 = (30,20)。
	if !a.onMouse(browser.MouseEvent{
		Kind: browser.MouseDown, Button: sdl2.MouseButtonLeft, X: 30, Y: 20,
	}) {
		t.Fatal("图片区按下应被拖动逻辑消费")
	}
	// 同一批次两条 motion：只累计目标，窗口不动。
	move(40, 26)
	move(55, 33)
	if p.x != x0 || p.y != y0 {
		t.Fatalf("鼠标事件里直接移动了窗口: (%d,%d) → (%d,%d)", x0, y0, p.x, p.y)
	}
	if !p.movePending || p.targetX != x0+25 || p.targetY != y0+13 {
		t.Fatalf("目标位置 = (%d,%d) pending=%v, want (%d,%d) true",
			p.targetX, p.targetY, p.movePending, x0+25, y0+13)
	}

	// 帧末落成一次。
	if !a.onFrame() {
		t.Fatal("拖动中 onFrame 应返回 true（主循环据此保持 1ms 轮询）")
	}
	if p.x != x0+25 || p.y != y0+13 || p.movePending {
		t.Fatalf("帧末未落成: (%d,%d) pending=%v", p.x, p.y, p.movePending)
	}

	// 窗口已跟上光标：相对坐标回到按下点，再移 (5,0) 只产生 5px 位移。
	move(35, 20)
	if p.targetX != x0+30 || p.targetY != y0+13 {
		t.Fatalf("第二帧目标 = (%d,%d), want (%d,%d)（窗口跟上后不应重复计算位移）",
			p.targetX, p.targetY, x0+30, y0+13)
	}

	// 抬起：本帧位移仍要落成，但不再报告 busy。
	a.onMouse(browser.MouseEvent{
		Kind: browser.MouseUp, Button: sdl2.MouseButtonLeft, X: 35, Y: 20,
	})
	if a.onFrame() {
		t.Fatal("抬起后 onFrame 不应再报告 busy")
	}
	if p.x != x0+30 || p.y != y0+13 || p.movePending {
		t.Fatalf("抬起时最后一帧位移丢失: (%d,%d) pending=%v", p.x, p.y, p.movePending)
	}
}
