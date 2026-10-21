package engine

import (
	"image"
	"image/png"
	"os"
	"strings"

	"github.com/go-comm/ghromex/core"
)

// 跨层契约（接口与它们签名里用到的值类型）放在 core 包：平台后端只依赖
// core，不依赖 engine。这里的类型别名让对外 API 一字不变，引擎与后端
// 实现的仍是同一个类型，接口照样互相满足。
type (
	// SizeType 是带单位尺寸的种类。
	SizeType = core.SizeType
	// Size 表示一个带单位的尺寸值。
	Size = core.Size
	// Color 表示 RGBA 颜色。
	Color = core.Color
	// Paint 是一次绘制动作的属性集合。
	Paint = core.Paint
	// Image 是绘制目标里的一张位图。
	Image = core.Image
	// Sprite 是一块可 blit 的位图。
	Sprite = core.Sprite
	// SpriteDrawer 是 Graphics 的可选扩展：支持位图 blit。
	SpriteDrawer = core.SpriteDrawer
	// Graphics 是渲染后端的抽象。
	Graphics = core.Graphics
	// Clipper 是 Graphics 的可选扩展：矩形裁剪栈。
	Clipper = core.Clipper
)

// 尺寸单位（与 core 同名常量，类型一致）。
const (
	SIZE_PIXEL       = core.SIZE_PIXEL
	SIZE_POINT       = core.SIZE_POINT
	SIZE_DISPLAYPORT = core.SIZE_DISPLAYPORT
	SIZE_PERCENT     = core.SIZE_PERCENT
	SIZE_AUTO        = core.SIZE_AUTO
)

// 构造器转发到 core（值类型与实现同处 core，跨层共用同一份）。
func NewZeroSize() Size { return core.NewZeroSize() }

// NewAutoSize 表示 CSS 中的 auto。
func NewAutoSize() Size { return core.NewAutoSize() }

// NewSize 造一个带单位的尺寸。
func NewSize(tp SizeType, intVal int, float32Val float32) Size {
	return core.NewSize(tp, intVal, float32Val)
}

// resolveLen 将带单位的 Size 解析为像素值；auto/nil 返回 -1。
func resolveLen(s Size, base int) int {
	if s == nil {
		return -1
	}
	switch s.Type() {
	case SIZE_PIXEL:
		return s.Pixel()
	case SIZE_POINT:
		return s.Point() * 96 / 72
	case SIZE_DISPLAYPORT:
		return s.Displayport() * 16
	case SIZE_PERCENT:
		return int(s.Percent() * float32(base) / 100)
	default:
		return -1
	}
}

// NewColor 造一个 RGBA 颜色。
func NewColor(r, g, b, a uint8) Color { return core.NewColor(r, g, b, a) }

// Rect 在这里表示盒模型四边的尺寸集合（left/right/top/bottom）。
type Rect interface {
	Left() Size
	Right() Size
	Top() Size
	Bottom() Size
}

type rect struct {
	left   Size
	right  Size
	top    Size
	bottom Size
}

func NewZeroRect() Rect {
	return &rect{NewZeroSize(), NewZeroSize(), NewZeroSize(), NewZeroSize()}
}

func NewRect(left, right, top, botoom Size) Rect {
	return &rect{left, right, top, botoom}
}

func (r *rect) Left() Size {
	return r.left
}

func (r *rect) Right() Size {
	return r.right
}

func (r *rect) Top() Size {
	return r.top
}

func (r *rect) Bottom() Size {
	return r.bottom
}

// NewPaint 造一份默认绘制属性（16px 常规字重、黑色）。
func NewPaint() Paint { return core.NewPaint() }

// Graphics 是渲染后端的抽象。布局引擎依赖 MeasureText 获取文本度量，
// 渲染阶段通过 DrawText/DrawColor/DrawImage 落笔。
//
// 接口本体在 core 包（见 core.Graphics）：平台后端只依赖 core，不反向
// 依赖 engine。这里的别名保证对外 API 不变、两边实现的是同一类型。

// pushClip 对支持裁剪的后端压栈，返回是否真的压了栈——调用方必须据此
// 配对 popClip，否则非 Clipper 后端/退化矩形（w 或 h ≤ 0）漏压却出栈会
// 弹掉外层的裁剪，破坏栈平衡。其余后端静默跳过。
func pushClip(g Graphics, x, y, w, h int) bool {
	if c, ok := g.(Clipper); ok && w > 0 && h > 0 {
		c.PushClip(x, y, w, h)
		return true
	}
	return false
}

func popClip(g Graphics) {
	if c, ok := g.(Clipper); ok {
		c.PopClip()
	}
}

var (
	_ Clipper = (*FakeGraphics)(nil)
	_ Clipper = (*BufferGraphics)(nil)
)

// FakeGraphics 供测试与无显卡环境使用的空后端。
type FakeGraphics struct {
	DrawCalls int
	TextCalls int
}

func NewFakeGraphics() Graphics {
	return &FakeGraphics{}
}

func (g *FakeGraphics) DrawText(x, y, w, h int, paint Paint, s string) {
	g.DrawCalls++
	g.TextCalls++
}

func (g *FakeGraphics) DrawColor(x, y, w, h int, color Color) {
	g.DrawCalls++
}

func (g *FakeGraphics) DrawImage(x, y, w, h int, image Image) {
	g.DrawCalls++
}

// MeasureText 给出粗略估算：CJK 按全宽、拉丁按半宽，行高 1.3 倍字号。
func (g *FakeGraphics) MeasureText(text string, fontSize int, bold bool, family string) (w, h int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	for _, r := range text {
		if r > 0x2E80 {
			w += fontSize
		} else if r == ' ' {
			w += fontSize / 2
		} else {
			w += fontSize * 3 / 5
		}
	}
	h = fontSize * 13 / 10
	return
}

// FontMetrics 实现 Graphics：给出估算的行度量（与 MeasureText 的估算
// 行高同口径拆成基线上下两半），供布局按基线对齐行内盒。
func (g *FakeGraphics) FontMetrics(text string, fontSize int, bold bool, family string) (int, int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	h := fontSize * 13 / 10
	return h / 2, h - h/2
}

// PushClip/PopClip 实现 Clipper（空后端无需真裁剪，但保持接口一致，
// 让"后端是否支持裁剪"不因测试替身而改变）。
func (g *FakeGraphics) PushClip(x, y, w, h int) {}
func (g *FakeGraphics) PopClip()                {}

// ---------- BufferGraphics ----------

// BufferGraphics 把绘制指令落在一块二维 RGBA 缓冲区上：
// DrawColor 填充矩形、DrawText 把每个非空白字符画成一个色块（近似字形占位）、
// DrawImage 填充品红占位块。它不依赖任何图形后端，适合单元测试断言像素，
// 也能 SavePNG 出图供人工查看。缓冲区初始为全透明。
//
// 混合规则做了简化：半透明填充按 src*a + dst*(255-a) 直通混合（不预乘），
// 仅用于测试与可视化，不追求色彩学精确。
type BufferGraphics struct {
	W, H int
	pix  []uint8 // RGBA 行主序，len = W*H*4

	// clips 为裁剪矩形栈（每层与上一层求交集），空栈 = 不裁剪。
	clips []clipRect

	DrawCalls int
	TextCalls int
	Texts     []string // 按绘制顺序记录的文本内容
}

// clipRect 是一个闭区间裁剪矩形（左上含、右下不含）。
type clipRect struct{ x, y, w, h int }

func intersectClip(a, b clipRect) clipRect {
	x0, y0 := maxInt(a.x, b.x), maxInt(a.y, b.y)
	x1 := minInt(a.x+a.w, b.x+b.w)
	y1 := minInt(a.y+a.h, b.y+b.h)
	if x1 <= x0 || y1 <= y0 {
		return clipRect{x: x0, y: y0, w: 0, h: 0}
	}
	return clipRect{x: x0, y: y0, w: x1 - x0, h: y1 - y0}
}

func NewBufferGraphics(w, h int) *BufferGraphics {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &BufferGraphics{W: w, H: h, pix: make([]uint8, w*h*4)}
}

// ColorAt 读取像素点，越界时 ok=false。
func (g *BufferGraphics) ColorAt(x, y int) (r, gg, b, a uint8, ok bool) {
	if x < 0 || y < 0 || x >= g.W || y >= g.H {
		return 0, 0, 0, 0, false
	}
	i := (y*g.W + x) * 4
	return g.pix[i], g.pix[i+1], g.pix[i+2], g.pix[i+3], true
}

// PushClip 实现 Clipper：压入一个与当前栈顶求交集的裁剪矩形。
func (g *BufferGraphics) PushClip(x, y, w, h int) {
	c := clipRect{x: x, y: y, w: w, h: h}
	if n := len(g.clips); n > 0 {
		c = intersectClip(g.clips[n-1], c)
	}
	g.clips = append(g.clips, c)
}

// PopClip 实现 Clipper：弹出栈顶；栈空时为无害的多余调用。
func (g *BufferGraphics) PopClip() {
	if n := len(g.clips); n > 0 {
		g.clips = g.clips[:n-1]
	}
}

// clipBounds 返回当前裁剪栈顶（空栈 = 整个缓冲区）。
func (g *BufferGraphics) clipBounds() clipRect {
	if n := len(g.clips); n > 0 {
		return g.clips[n-1]
	}
	return clipRect{x: 0, y: 0, w: g.W, h: g.H}
}

// FillRect 填充一个整数矩形（自动裁剪到缓冲区与当前裁剪栈）。
func (g *BufferGraphics) FillRect(x, y, w, h int, r, gg, b, a uint8) {
	if w <= 0 || h <= 0 || a == 0 {
		return
	}
	if cl := g.clipBounds(); len(g.clips) > 0 {
		if cl.w <= 0 || cl.h <= 0 {
			return
		}
		x0, y0 := maxInt(x, cl.x), maxInt(y, cl.y)
		x1, y1 := minInt(x+w, cl.x+cl.w), minInt(y+h, cl.y+cl.h)
		if x1 <= x0 || y1 <= y0 {
			return
		}
		x, y, w, h = x0, y0, x1-x0, y1-y0
	}
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > g.W {
		w = g.W - x
	}
	if y+h > g.H {
		h = g.H - y
	}
	if w <= 0 || h <= 0 {
		return
	}
	for yy := y; yy < y+h; yy++ {
		i := (yy*g.W + x) * 4
		for xx := 0; xx < w; xx++ {
			if a == 255 {
				g.pix[i], g.pix[i+1], g.pix[i+2], g.pix[i+3] = r, gg, b, 255
			} else {
				sa := uint32(a)
				da := uint32(255 - a)
				nr := (uint32(r)*sa + uint32(g.pix[i])*da) / 255
				ng := (uint32(gg)*sa + uint32(g.pix[i+1])*da) / 255
				nb := (uint32(b)*sa + uint32(g.pix[i+2])*da) / 255
				na := sa + uint32(g.pix[i+3])*da/255
				g.pix[i], g.pix[i+1], g.pix[i+2], g.pix[i+3] = uint8(nr), uint8(ng), uint8(nb), uint8(na)
			}
			i += 4
		}
	}
}

// DrawColor 实现 Graphics：颜色填充矩形。
func (g *BufferGraphics) DrawColor(x, y, w, h int, color Color) {
	g.DrawCalls++
	if color == nil {
		return
	}
	r, gg, b, a := color.RGBA()
	g.FillRect(x, y, w, h, r, gg, b, a)
}

// DrawText 实现 Graphics：整段空白只记录不绘制；其余每个字符画一个内缩
// 色块作为字形近似，保证"该有字的地方有颜色可断言"。
func (g *BufferGraphics) DrawText(x, y, w, h int, paint Paint, text string) {
	g.DrawCalls++
	g.TextCalls++
	g.Texts = append(g.Texts, text)
	if strings.TrimSpace(text) == "" || w <= 0 || h <= 0 {
		return
	}
	var r, gg, b, a uint8 = 0, 0, 0, 255
	if paint != nil {
		if c := paint.Color(); c != nil {
			r, gg, b, a = c.RGBA()
		}
	}
	runes := []rune(text)
	cell := w / len(runes)
	if cell <= 0 {
		cell = 1
	}
	inx := cell / 5
	iny := h / 5
	ih := h - 2*iny
	if ih <= 0 {
		ih = 1
	}
	iw := cell - 2*inx
	if iw <= 0 {
		iw = 1
	}
	for i := range runes {
		g.FillRect(x+i*cell+inx, y+iny, iw, ih, r, gg, b, a)
	}
}

// DrawImage 实现 Graphics：记录调用并填充品红占位块。
func (g *BufferGraphics) DrawImage(x, y, w, h int, image Image) {
	g.DrawCalls++
	g.FillRect(x, y, w, h, 255, 0, 255, 255)
}

// DrawSprite 实现 engine.SpriteDrawer：按 RGBA8888 逐像素混合精灵
// （alpha 0 的像素跳过，与逐像素路径的跳过语义一致）。
func (g *BufferGraphics) DrawSprite(x, y int, s *Sprite) {
	if s == nil || s.W <= 0 || s.H <= 0 {
		return
	}
	g.DrawCalls++
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
			g.FillRect(x+u, y+v, 1, 1, s.Pix[i], s.Pix[i+1], s.Pix[i+2], a)
		}
	}
}

// MeasureText 实现 Graphics：与 FakeGraphics 相同的粗略估算，
// 保证布局结果可与断言表对应。
func (g *BufferGraphics) MeasureText(text string, fontSize int, bold bool, family string) (w, h int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	for _, r := range text {
		if r > 0x2E80 {
			w += fontSize
		} else if r == ' ' {
			w += fontSize / 2
		} else {
			w += fontSize * 3 / 5
		}
	}
	h = fontSize * 13 / 10
	return
}

// FontMetrics 实现 Graphics：与 FakeGraphics 相同的估算口径。
func (g *BufferGraphics) FontMetrics(text string, fontSize int, bold bool, family string) (int, int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	h := fontSize * 13 / 10
	return h / 2, h - h/2
}

// SavePNG 把缓冲区内容导出为 PNG 文件（供人工查看渲染结果）。
func (g *BufferGraphics) SavePNG(path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, g.W, g.H))
	for y := 0; y < g.H; y++ {
		copy(img.Pix[y*img.Stride:y*img.Stride+g.W*4], g.pix[y*g.W*4:(y+1)*g.W*4])
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

var _ Graphics = (*BufferGraphics)(nil)
