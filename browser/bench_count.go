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

	// 分桶计数：总数说不清瓶颈。「文字调用」随**可见文本片段**涨（视口裁剪
	// 生效就应该稳定在可见行数），「色块调用」随可见盒子涨。滚动探针靠这两个
	// 数判断裁剪有没有真起作用（见 scrollprobe.go）。
	texts, colors, images, sprites int
}

// reset 把计数清零（每帧开始时调）。
func (c *countingGraphics) reset() {
	c.calls, c.texts, c.colors, c.images, c.sprites = 0, 0, 0, 0, 0
}

func (c *countingGraphics) DrawText(x, y, w, h int, p core.Paint, s string) {
	c.calls++
	c.texts++
	c.inner.DrawText(x, y, w, h, p, s)
}

func (c *countingGraphics) DrawColor(x, y, w, h int, col core.Color) {
	c.calls++
	c.colors++
	c.inner.DrawColor(x, y, w, h, col)
}

func (c *countingGraphics) DrawImage(x, y, w, h int, img core.Image) {
	c.calls++
	c.images++
	c.inner.DrawImage(x, y, w, h, img)
}

func (c *countingGraphics) DrawSprite(x, y int, s *core.Sprite) {
	c.calls++
	c.sprites++
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
