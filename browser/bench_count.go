// 本文件是基准模式用的绘制调用计数器：套在真实 Graphics 外面数调用。
//
// 为什么要数：光看「重绘多少毫秒」不知道该优化什么。把调用数与耗时
// 并排看，才能判断时间随**调用数**涨还是随**面积**涨——两者的优化方向
// 完全不同（减调用 vs 减填充）。
package browser

import (
	"github.com/go-comm/ghromex/core"
)

// countingGraphics 统计每帧的绘制调用数，其余行为透传。
type countingGraphics struct {
	inner core.Graphics
	calls int
}

// reset 把计数清零（每帧开始时调）。
func (c *countingGraphics) reset() { c.calls = 0 }

func (c *countingGraphics) DrawText(x, y, w, h int, p core.Paint, s string) {
	c.calls++
	c.inner.DrawText(x, y, w, h, p, s)
}

func (c *countingGraphics) DrawColor(x, y, w, h int, col core.Color) {
	c.calls++
	c.inner.DrawColor(x, y, w, h, col)
}

func (c *countingGraphics) DrawImage(x, y, w, h int, img core.Image) {
	c.calls++
	c.inner.DrawImage(x, y, w, h, img)
}

func (c *countingGraphics) DrawSprite(x, y int, s *core.Sprite) {
	c.calls++
	if d, ok := c.inner.(core.SpriteDrawer); ok {
		d.DrawSprite(x, y, s)
	}
}

func (c *countingGraphics) MeasureText(s string, size int, bold bool, family string) (int, int) {
	return c.inner.MeasureText(s, size, bold, family)
}

func (c *countingGraphics) FontMetrics(s string, size int, bold bool, family string) (int, int) {
	return c.inner.FontMetrics(s, size, bold, family)
}

// PushClip/PopClip：后端支持裁剪时透传（不计数——裁剪不是绘制调用）。
func (c *countingGraphics) PushClip(x, y, w, h int) {
	if cl, ok := c.inner.(core.Clipper); ok {
		cl.PushClip(x, y, w, h)
	}
}

func (c *countingGraphics) PopClip() {
	if cl, ok := c.inner.(core.Clipper); ok {
		cl.PopClip()
	}
}
