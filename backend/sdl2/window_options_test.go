//go:build windows

package sdl2

import (
	"os"
	"testing"
)

// 悬浮窗的窗口类型必须真的落到 SDL 标志上：无边框、置顶、不占任务栏，
// 且不能是可缩放窗（否则拖图会变成拖边框）。
func TestWindowOptionsFlags(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindowWithOptions(WindowOptions{
		Width: 160, Height: 120, Title: "opt-test",
		X: 10, Y: 20,
		Borderless: true, AlwaysOnTop: true, SkipTaskbar: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	flags := uint32(sdlGetWindowFlags(win.win))
	for _, c := range []struct {
		name string
		bit  uint32
	}{
		{"BORDERLESS", windowBorderless},
		{"ALWAYS_ON_TOP", windowAlwaysOnTop},
		{"SKIP_TASKBAR", windowSkipTaskbar},
	} {
		if flags&c.bit == 0 {
			t.Errorf("缺少 %s 标志，flags=0x%X", c.name, flags)
		}
	}
	if flags&windowResizable != 0 {
		t.Errorf("悬浮窗不该可缩放，flags=0x%X", flags)
	}
}

// SetSize 要立刻反映到视口：预览是“同一窗口原地缩成选区”，下一帧就要
// 按新尺寸布局，不能等 WINDOWEVENT。
func TestSetSizeUpdatesViewport(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindowWithOptions(WindowOptions{Width: 200, Height: 150, Title: "size-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	win.SetSize(80, 60)
	if w, h := win.ViewportWidth(), win.ViewportHeight(); w != 80 || h != 60 {
		t.Fatalf("视口 = %dx%d，期望 80x60", w, h)
	}
}
