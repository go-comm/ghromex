//go:build windows

package sdl2

import (
	"os"
	"testing"
	"time"
)

// 交换链在首次 Present 前不含有效画面：窗口若建窗即可见，从建窗到解析/
// 布局/首绘（含首次字体加载，可达数百毫秒）会一直显示黑底，表现为
// “先黑一下再闪出页面”。这里锁定新时序：建窗隐藏 → 首帧提交后才显示。
func TestWindowHiddenUntilFirstFrame(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(320, 240, "show-test")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	if f := uint32(sdlGetWindowFlags(win.win)); f&windowShown != 0 {
		t.Fatalf("建窗即可见：flags=0x%02X（应隐藏到首帧）", f)
	}
	if win.shown {
		t.Fatal("NewWindow 后 shown 应为 false")
	}

	// 无文档时 RenderFrame 早退：不能顺手把窗口显示出来（那等于回到黑闪）。
	win.RenderFrame()
	if f := uint32(sdlGetWindowFlags(win.win)); f&windowShown != 0 {
		t.Fatalf("无文档 RenderFrame 不应显示窗口：flags=0x%02X", f)
	}

	const src = `<!doctype html><html><body><p>首帧</p></body></html>`
	if _, err := win.OpenDocument(src); err != nil {
		t.Fatal(err)
	}
	// OpenDocument 只置 dirty；黑闪窗口期正是这里到首帧之间，期间不得显示。
	if f := uint32(sdlGetWindowFlags(win.win)); f&windowShown != 0 {
		t.Fatalf("OpenDocument 后、首帧前窗口已显示：flags=0x%02X", f)
	}

	win.RenderFrame()
	if !win.shown {
		t.Fatal("首帧 Present 后 shown 未置位")
	}
	if f := uint32(sdlGetWindowFlags(win.win)); f&windowShown == 0 {
		t.Fatalf("首帧提交后窗口未显示：flags=0x%02X", f)
	}

	// 重复 Present 不得重复 ShowWindow/RaiseWindow（置位即止）。
	win.RenderFrame()
	if !win.shown {
		t.Fatal("二次 Present 后 shown 丢失")
	}
}

// Run 的帧循环是 demo 的常规路径：首个循环迭代出首帧并显示窗口。
func TestRunShowsWindowOnFirstFrame(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(320, 240, "show-run-test")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	const src = `<!doctype html><html><body><p>run</p></body></html>`
	if _, err := win.OpenDocument(src); err != nil {
		t.Fatal(err)
	}
	win.SetAutoExit(150 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if !win.shown {
		t.Fatal("Run 首帧后 shown 未置位")
	}
	if f := uint32(sdlGetWindowFlags(win.win)); f&windowShown == 0 {
		t.Fatalf("Run 首帧后窗口未显示：flags=0x%02X", f)
	}
}
