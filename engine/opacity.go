package engine

// opacity 的近似实现：引擎没有图层合成（offscreen layer），无法把一棵子树
// 先画到离屏缓冲再整体按 alpha 混合。这里用「子树内每一次绘制动作的颜色
// alpha 都乘以 opacity」近似——对单色盒（背景/边框/文字/阴影）与浏览器
// 观感一致，对「半透明子元素叠在半透明父元素上」这类重叠场景会有差异
// （真 CSS 先合成整组再混合，引擎是逐个混合），README 已知限制里注明。

// withOpacity 用 alpha 缩放包装 g；k>=1 时原样返回（不引入额外分配）。
func withOpacity(g Graphics, k float64) Graphics {
	if g == nil || k >= 1 {
		return g
	}
	if k < 0 {
		k = 0
	}
	return &opacityGraphics{Graphics: g, k: k}
}

// opacityGraphics 把本子树的绘制颜色按 k 缩放 alpha。
type opacityGraphics struct {
	Graphics
	k float64
	// sprites 缓存本次子树内已缩放过的精灵（同一角标/阴影常被重复 blit）。
	sprites map[*Sprite]*Sprite
}

// scaleColor 把颜色的 alpha 乘以 k；k 为 1 或颜色为 nil 时原样返回。
func scaleColor(c Color, k float64) Color {
	if c == nil || k >= 1 {
		return c
	}
	r, g, b, a := c.RGBA()
	na := uint8(float64(a)*k + 0.5)
	if na == a {
		return c
	}
	return NewColor(r, g, b, na)
}

// scalePaint 复制一份 Paint 并缩放文字色与落点背景色（LCD 合成用）。
func scalePaint(p Paint, k float64) Paint {
	if p == nil {
		return nil
	}
	np := NewPaint()
	np.SetSize(p.Size())
	np.SetColor(scaleColor(p.Color(), k))
	np.SetFontFamily(p.FontFamily())
	np.SetBold(p.Bold())
	setPaintItalic(np, paintItalic(p))
	// 数值字重随 paint 一起缩放：透明层底下选字面不能退化成两档（否则
	// opacity 内的 600 文字会意外用上 700 Bold 面）。
	if wp, ok := np.(WeightedPaint); ok {
		if pp, ok2 := p.(WeightedPaint); ok2 {
			wp.SetFontWeightNum(pp.FontWeightNum())
		}
	}
	np.SetBackground(scaleColor(p.Background(), k))
	return np
}

func (o *opacityGraphics) DrawColor(x, y, w, h int, c Color) {
	o.Graphics.DrawColor(x, y, w, h, scaleColor(c, o.k))
}

func (o *opacityGraphics) DrawText(x, y, w, h int, p Paint, s string) {
	o.Graphics.DrawText(x, y, w, h, scalePaint(p, o.k), s)
}

// DrawImage 透传：位图本身没有可乘的颜色（引擎当前也不绘制图片）。
func (o *opacityGraphics) DrawImage(x, y, w, h int, img Image) {
	o.Graphics.DrawImage(x, y, w, h, img)
}

// PushClip/PopClip 委托给底层后端的裁剪栈；底层不支持时静默忽略
// （与 pushClip/popClip 的配对语义一致：包装层永远成对调用）。
func (o *opacityGraphics) PushClip(x, y, w, h int) {
	if c, ok := o.Graphics.(Clipper); ok {
		c.PushClip(x, y, w, h)
	}
}

func (o *opacityGraphics) PopClip() {
	if c, ok := o.Graphics.(Clipper); ok {
		c.PopClip()
	}
}

// DrawSprite 缩放精灵像素的 alpha。同一精灵在本次子树内只复制一次；
// 底层没有 SpriteDrawer 时退化为逐像素填充（与引擎的非精灵路径等价）。
func (o *opacityGraphics) DrawSprite(x, y int, s *Sprite) {
	if s == nil {
		return
	}
	if sd, ok := o.Graphics.(SpriteDrawer); ok {
		sd.DrawSprite(x, y, o.scaledSprite(s))
		return
	}
	for v := 0; v < s.H; v++ {
		for u := 0; u < s.W; u++ {
			i := (v*s.W + u) * 4
			if i+3 >= len(s.Pix) {
				break
			}
			a := s.Pix[i+3]
			if a == 0 {
				continue
			}
			na := uint8(float64(a)*o.k + 0.5)
			if na == 0 {
				continue
			}
			o.Graphics.DrawColor(x+u, y+v, 1, 1,
				NewColor(s.Pix[i], s.Pix[i+1], s.Pix[i+2], na))
		}
	}
}

func (o *opacityGraphics) scaledSprite(s *Sprite) *Sprite {
	if o.sprites == nil {
		o.sprites = make(map[*Sprite]*Sprite)
	}
	if cp, ok := o.sprites[s]; ok {
		return cp
	}
	pix := make([]byte, len(s.Pix))
	copy(pix, s.Pix)
	for i := 3; i < len(pix); i += 4 {
		pix[i] = uint8(float64(pix[i])*o.k + 0.5)
	}
	cp := &Sprite{W: s.W, H: s.H, Pix: pix}
	o.sprites[s] = cp
	return cp
}

var (
	_ Clipper      = (*opacityGraphics)(nil)
	_ SpriteDrawer = (*opacityGraphics)(nil)
)
