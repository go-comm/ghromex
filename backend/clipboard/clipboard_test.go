//go:build windows

package clipboard

import (
	"os"
	"testing"

	"github.com/go-comm/ghromex/backend/sdl2"
)

// 剪贴板要能写能读：应用层读写的与引擎 Ctrl+C / Ctrl+V 用的是同一份
// 内容（两边都走 SDL2 的剪贴板接口）。
func TestClipboardRoundTrip(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	// 建窗口顺带初始化 SDL（剪贴板接口要 video 子系统）。
	win, err := sdl2.NewWindow(320, 200, "clipboard-test")
	if err != nil {
		t.Skipf("创建窗口失败: %v", err)
	}
	defer win.Close()

	prev := Text()
	defer SetText(prev) // 测完把用户剪贴板还原

	const want = "ghromex 剪贴板测试"
	SetText(want)
	if got := Text(); got != want {
		t.Errorf("读回剪贴板 = %q, want %q", got, want)
	}
}
