//go:build windows

package sdl2

import (
	"os"
	"testing"
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

	// 首次 Present 之后窗口才可见。
	win.Present()
	if !win.shown {
		t.Fatal("首次 Present 后 shown 未置位")
	}
	if f := uint32(sdlGetWindowFlags(win.win)); f&windowShown == 0 {
		t.Fatalf("首次 Present 后窗口未显示：flags=0x%02X", f)
	}

	// 重复 Present 不得重复 ShowWindow/RaiseWindow（置位即止）。
	win.Present()
	if !win.shown {
		t.Fatal("二次 Present 后 shown 丢失")
	}
}
