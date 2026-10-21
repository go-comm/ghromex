//go:build windows

package browser

import (
	"os"
	"testing"

	"github.com/go-comm/ghromex/core"
	"github.com/go-comm/ghromex/engine"
)

// drawCounter 统计一帧里的绘制调用数。
//
// 必须实现 core.SpriteDrawer：否则引擎判定后端不支持位图 blit，圆角会
// 退化成逐像素填充（上万次 DrawColor），数出来的就不是真实路径了。
type drawCounter struct {
	inner   core.Graphics
	texts   int
	colors  int
	images  int
	sprites int
}

func (c *drawCounter) reset() { c.texts, c.colors, c.images, c.sprites = 0, 0, 0, 0 }
func (c *drawCounter) DrawText(x, y, w, h int, p core.Paint, s string) {
	c.texts++
	c.inner.DrawText(x, y, w, h, p, s)
}
func (c *drawCounter) DrawColor(x, y, w, h int, col core.Color) {
	c.colors++
	c.inner.DrawColor(x, y, w, h, col)
}
func (c *drawCounter) DrawImage(x, y, w, h int, img core.Image) {
	c.images++
	c.inner.DrawImage(x, y, w, h, img)
}
func (c *drawCounter) DrawSprite(x, y int, s *core.Sprite) {
	c.sprites++
	c.inner.(core.SpriteDrawer).DrawSprite(x, y, s)
}
func (c *drawCounter) MeasureText(s string, size int, bold bool, family string) (int, int) {
	return c.inner.MeasureText(s, size, bold, family)
}
func (c *drawCounter) FontMetrics(s string, size int, bold bool, family string) (int, int) {
	return c.inner.FontMetrics(s, size, bold, family)
}

// 门户页每帧的绘制调用要有上限。
//
// 滚动时整页都要重画（引擎没有合成器，滚动=逐帧重绘），调用数一多，
// 帧时间就顶到 vsync 之外——掉帧，滚动就不丝滑。绘制时按视口裁剪把视口
// 外的内容摘掉后，每帧调用数应当远小于整页（实测 2624 → 1014）。
func TestPortalFrameDrawCallsBounded(t *testing.T) {
	src, err := os.ReadFile("../examples/portal/index.html")
	if err != nil {
		t.Fatal(err)
	}
	win, err := New(1280, 820, "portal-perf")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()
	doc, err := win.OpenDocument(string(src))
	if err != nil {
		t.Fatal(err)
	}

	counter := &drawCounter{inner: win.sdl.Graphics()}
	counter.reset()
	engine.RenderNode(counter, doc)
	total := counter.texts + counter.colors + counter.images + counter.sprites
	if total == 0 {
		t.Fatal("没有采到任何绘制调用")
	}
	t.Logf("门户页一帧绘制调用：DrawText %d，DrawColor %d，DrawSprite %d，合计 %d",
		counter.texts, counter.colors, counter.sprites, total)
	// 视口 820px 高，页面约 3 屏高：视口外的部分不画，调用数应明显
	// 小于「整页都画」的量级（上限取 1500，实测 1014）。
	if total > 1500 {
		t.Errorf("门户页一帧绘制调用 = %d（>1500）：视口裁剪可能失效，滚动会掉帧", total)
	}
}
