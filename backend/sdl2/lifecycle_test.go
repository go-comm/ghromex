//go:build windows

package sdl2

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-comm/ghromex/engine"
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

// TestTextureCacheBounded 文本纹理缓存必须有上限：动态文本（计数/时间戳）
// 不断产生新的 (文本,字号,颜色) 组合，缓存只增不减必然耗尽显存。
func TestTextureCacheBounded(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(320, 200, "tex-cache")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g

	for i := 0; i < maxCachedTextures+64; i++ {
		g.Clear(255, 255, 255, 255)
		p := engine.NewPaint()
		p.SetSize(engine.NewSize(engine.SIZE_PIXEL, 13, 0))
		p.SetColor(engine.NewColor(0, 0, 0, 255))
		g.DrawText(8, 20, 280, 20, p, fmt.Sprintf("dyn-%d", i))
	}
	if n := len(g.textures); n > maxCachedTextures {
		t.Errorf("纹理缓存 = %d, want ≤ %d（无上限会让动态文本页面显存单调增长）", n, maxCachedTextures)
	}
	if n := len(g.texOrder); n > maxCachedTextures {
		t.Errorf("纹理插入序列表 = %d, want ≤ %d", n, maxCachedTextures)
	}
}
