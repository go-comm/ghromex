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
	// 像素上限：长文本纹理很占显存，条数没超也可能吃满。
	if g.texPixels > maxCachedTexPixels {
		t.Errorf("纹理像素数 = %d, want ≤ %d（显存上限失效）", g.texPixels, maxCachedTexPixels)
	}
}

// 缓存必须按"最近最少使用"淘汰：页面上的分隔符/价签等小文本被复用最多，
// FIFO 会先淘汰它们，工作集略大于上限时每帧全部重渲染（滚动发顿）。
func TestTextureCacheEvictsLeastRecentlyUsed(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(320, 200, "tex-lru")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g

	// 造一批纹理（超过上限的一半），其中 "hot" 被反复使用。
	mk := func(text string) {
		p := engine.NewPaint()
		p.SetSize(engine.NewSize(engine.SIZE_PIXEL, 13, 0))
		p.SetColor(engine.NewColor(0, 0, 0, 255))
		g.DrawText(8, 20, 280, 20, p, text)
	}
	for i := 0; i < maxCachedTextures; i++ {
		mk(fmt.Sprintf("t-%d", i))
	}
	hot, _ := g.lruTexture()
	// 让 hot 成为最近使用：再画一次即可刷新它的 LRU 时钟。
	mk(hot.text)

	// 再塞满一批新纹理，hot 不该被淘汰（它刚被用过）。
	for i := 0; i < maxCachedTextures/2; i++ {
		mk(fmt.Sprintf("n-%d", i))
	}
	if _, ok := g.textures[hot]; !ok {
		t.Errorf("最近使用的纹理被淘汰了（LRU 未生效）：%q", hot.text)
	}
}
