//go:build windows

package browser

import (
	"testing"
	"time"

	"github.com/go-comm/ghromex/core"
	"github.com/go-comm/ghromex/engine"
)

// 背景图必须画在清屏之后、文档之前：不透明像素要原样出现在窗口里。
func TestSetBackdropDrawn(t *testing.T) {
	win := newTestWindow(t, 8, 6, "backdrop-test")
	doc, err := win.OpenDocument(`<!doctype html><html><body style="margin:0"></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	doc.Refresh()

	pix := make([]byte, 8*6*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i] = 10
		pix[i+1] = 200
		pix[i+2] = 30
		pix[i+3] = 255
	}
	win.SetBackdrop(&core.Sprite{W: 8, H: 6, Pix: pix})
	win.RenderFrame()

	raw, _, w, h, err := win.Screenshot()
	if err != nil {
		t.Fatal(err)
	}
	if w != 8 || h != 6 {
		t.Fatalf("截图尺寸 = %dx%d", w, h)
	}
	if got := raw[0]; got != 10 {
		t.Fatalf("R = %d，期望 10（背景图没画出来）", got)
	}
	if got := raw[1]; got != 200 {
		t.Fatalf("G = %d，期望 200", got)
	}
	if got := raw[2]; got != 30 {
		t.Fatalf("B = %d，期望 30", got)
	}
}

// 原始鼠标钩子先于引擎：返回 true 时页面收不到 click。
func TestMouseHandlerConsumesBeforeEngine(t *testing.T) {
	win := newTestWindow(t, 200, 120, "mouse-hook-test")
	doc, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">` +
		`<div id="a" style="width:200px;height:120px;background:#EEE">x</div>` +
		`</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	el := doc.QuerySelector("#a")
	if el == nil {
		t.Fatal("测试用的 div 不存在")
	}
	clicks := 0
	el.OnClick(func(*engine.MouseEvent) { clicks++ })

	// 先空跑一段把平台自身的首帧事件排空，再装钩子：dummy 驱动在窗口显示
	// （首帧 Present）时会把鼠标位置“瞬移”到窗口中心，那条 SDL_MOUSEMOTION
	// 会在注入的事件之后到达，钩子于是多收一个事件，下面的“恰好 3 个”断言
	// 随机失败（机器负载高时尤其明显，例如 go test ./... 并发跑包）。
	// 装钩子放在空跑之后，是为了不让这条平台事件混进 got——真实平台同样会
	// 在显示/聚焦时发这类事件，按真实时序消费掉即可，不需要在断言里容忍它。
	win.SetAutoExit(120 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	var got []MouseEvent
	win.SetMouseHandler(func(ev MouseEvent) bool {
		got = append(got, ev)
		return true
	})

	mouseButtonDown(win, 10, 10)
	mouseMotion(win, 30, 40)
	mouseButtonUp(win, 30, 40)

	win.SetAutoExit(200 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	want := []MouseEvent{
		{Kind: MouseDown, X: 10, Y: 10, Button: 1},
		{Kind: MouseMove, X: 30, Y: 40},
		{Kind: MouseUp, X: 30, Y: 40, Button: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("钩子收到 %d 个事件，期望 %d：%+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("事件[%d] = %+v，期望 %+v", i, got[i], want[i])
		}
	}
	if clicks != 0 {
		t.Fatalf("钩子消费后页面仍收到 %d 次 click", clicks)
	}
}

// nil 钩子保持原行为：点击照常到页面。
func TestNilMouseHandlerKeepsClicks(t *testing.T) {
	win := newTestWindow(t, 120, 80, "mouse-nil-test")
	doc, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">` +
		`<div id="a" style="width:120px;height:80px;background:#EEE">x</div>` +
		`</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	clicks := 0
	doc.QuerySelector("#a").OnClick(func(*engine.MouseEvent) { clicks++ })

	win.SetMouseHandler(nil)
	mouseButtonDown(win, 10, 10)
	mouseButtonUp(win, 10, 10)

	win.SetAutoExit(200 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatalf("点击次数 = %d，期望 1", clicks)
	}
}
