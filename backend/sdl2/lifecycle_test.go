//go:build windows

package sdl2

import (
	"os"
	"testing"
)

// TestCloseIdempotent 重复 Close 不得二次销毁 SDL 对象（use-after-free）：
// 关闭后句柄必须置零，且第二次调用直接返回。
func TestCloseIdempotent(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(120, 80, "close-twice")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	win.Close()
	if win.renderer != 0 || win.win != 0 {
		t.Fatalf("Close 后句柄未置零: renderer=%x win=%x", win.renderer, win.win)
	}
	// 第二次 Close 必须是空操作（此前会二次 SDL_Destroy*）
	win.Close()
	if err := win.Run(); err == nil {
		t.Error("已关闭窗口再 Run 应报错")
	}
}
