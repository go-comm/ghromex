// Package core 是渲染引擎与平台后端之间的契约层。
//
// 它是叶子包：不依赖仓库内任何其它包。引擎与后端都只依赖它，于是
// 依赖方向是单向的：
//
//	core  ←  engine（实现契约，纯 Go）
//	core  ←  backend/sdl2（实现契约，平台相关）
//
// 没有它时，后端要「实现引擎的接口」就得 import 引擎，依赖方向就成了
// backend → engine：平台层反过来依赖它上面的层。Go 的接口是结构化的
// （只要方法集一致就满足），所以把接口与跨层用到的值类型搬到 core，
// 引擎用类型别名（type Color = core.Color）保持对外 API 不变，
// 后端则只 import core——两边实现的是同一个类型，接口照样满足。
//
// 这里只放**跨层契约**：接口，以及接口签名里用到的值类型。引擎内部的
// 类型（文档、DOM、样式）与平台内部的类型（纹理、窗口句柄）都不在此。
package core

// SizeType 是带单位尺寸的种类。
type SizeType uint8

// 尺寸单位。
const (
	SIZE_PIXEL SizeType = iota
	SIZE_POINT
	SIZE_DISPLAYPORT
	SIZE_PERCENT
	SIZE_AUTO
)

// Size 表示一个带单位的尺寸值。
type Size interface {
	Type() SizeType
	Pixel() int
	SetPixel(v int)
	Point() int
	SetPoint(v int)
	Displayport() int
	SetDisplayport(v int)
	Percent() float32
	SetPercent(v float32)
	Disc(int) int
}

// size 是 Size 的实现。
type size struct {
	tp         SizeType
	intVal     int
	float32Val float32
}

// NewZeroSize 造一个 0 像素的尺寸。
func NewZeroSize() Size {
	return &size{tp: SIZE_PIXEL, intVal: 0, float32Val: 0}
}

// NewAutoSize 表示 CSS 中的 auto。
func NewAutoSize() Size {
	return &size{tp: SIZE_AUTO}
}

// NewSize 造一个带单位的尺寸。
func NewSize(tp SizeType, intVal int, float32Val float32) Size {
	return &size{tp: tp, intVal: intVal, float32Val: float32Val}
}

func (s *size) Type() SizeType { return s.tp }
func (s *size) Pixel() int     { return s.intVal }
func (s *size) SetPixel(v int) {
	s.tp = SIZE_PIXEL
	s.intVal = v
}
func (s *size) Point() int { return s.intVal }
func (s *size) SetPoint(v int) {
	s.tp = SIZE_POINT
	s.intVal = v
}
func (s *size) Displayport() int { return s.intVal }
func (s *size) SetDisplayport(v int) {
	s.tp = SIZE_DISPLAYPORT
	s.intVal = v
}
func (s *size) Percent() float32 { return s.float32Val }
func (s *size) SetPercent(v float32) {
	s.tp = SIZE_PERCENT
	s.float32Val = v
}
func (s *size) Disc(int) int { return 0 }

// Color 表示 RGBA 颜色。
type Color interface {
	RGBA() (r, g, b, a uint8)
}

// colorVal 是 Color 的实现。
type colorVal struct {
	r, g, b, a uint8
}

// NewColor 造一个 RGBA 颜色。
func NewColor(r, g, b, a uint8) Color {
	return &colorVal{r: r, g: g, b: b, a: a}
}

func (c *colorVal) RGBA() (r, g, b, a uint8) {
	return c.r, c.g, c.b, c.a
}

// Paint 是一次绘制动作的属性集合（类比 Android Paint / Skia Paint）。
// Background 为文字落点的背景色（沿父链最近实底，可能 nil）：供 LCD
// 子像素合成使用——子像素渲染必须知道底色才能合成出正确过渡色。
type Paint interface {
	Size() Size
	SetSize(size Size)
	Color() Color
	SetColor(color Color)
	FontFamily() string
	SetFontFamily(family string)
	Bold() bool
	SetBold(bold bool)
	Background() Color
	SetBackground(color Color)
}

// paint 是 Paint 的实现。
type paint struct {
	size   Size
	color  Color
	family string
	bold   bool
	bg     Color
}

// NewPaint 造一份默认绘制属性（16px 常规字重、黑色）。
func NewPaint() Paint {
	return &paint{size: NewSize(SIZE_PIXEL, 16, 0), color: NewColor(0, 0, 0, 255)}
}

func (p *paint) Size() Size { return p.size }
func (p *paint) SetSize(size Size) {
	if size != nil {
		p.size = size
	}
}
func (p *paint) Color() Color { return p.color }
func (p *paint) SetColor(color Color) {
	if color != nil {
		p.color = color
	}
}
func (p *paint) FontFamily() string     { return p.family }
func (p *paint) SetFontFamily(f string) { p.family = f }
func (p *paint) Bold() bool             { return p.bold }
func (p *paint) SetBold(bold bool)      { p.bold = bold }
func (p *paint) Background() Color      { return p.bg }
func (p *paint) SetBackground(c Color)  { p.bg = c }

// Image 是绘制目标里的一张位图。
type Image interface {
}

// Sprite 是一块可 blit 的位图（RGBA8888，每像素 4 字节，行主序）。
//
// 目前只用于圆角抗锯齿的“角标精灵”：整块圆角一次 blit，代替逐像素的
// 半透明小方块。逐像素画圆角边缘的绘制调用极多——实测 portal 页一帧
// 18524 次 1×1 绘制，占全部绘制调用的 3/4；GPU 路径上每次都是驱动
// 调用，帧时间被拖到几十毫秒，120ms 的滚动动画被切成两三帧，看起来就是
// “一卡一卡、还会闪”。整角一次 blit 把这部分降到 4 次/盒。
//
// alpha 为 0 的像素表示“不着色”（blit 时不覆盖底色），与逐像素路径里
// scaledAlpha 返回 ok=false 的跳过语义一致。
type Sprite struct {
	W, H int
	Pix  []byte
}

// SpriteDrawer 是 Graphics 的可选扩展：支持位图 blit 的后端实现它。
// 不实现的后端回落到逐像素绘制（结果一致，只是慢）。
type SpriteDrawer interface {
	DrawSprite(x, y int, s *Sprite)
}

// Graphics 是渲染后端的抽象。布局引擎依赖 MeasureText 获取文本度量，
// 渲染阶段通过 DrawText/DrawColor/DrawImage 落笔。
type Graphics interface {
	DrawText(x, y, w, h int, paint Paint, text string)
	DrawColor(x, y, w, h int, color Color)
	DrawImage(x, y, w, h int, image Image)
	// MeasureText 返回文本在给定字号/粗体/字族下的像素宽高。
	// family 传 CSS font-family 原值（可含逗号列表）；度量必须与 DrawText
	// 使用同一字体解析规则，否则混排宽度会错位。
	MeasureText(text string, fontSize int, bold bool, family string) (w, h int)
	// FontMetrics 返回文本在给定字号/粗体/字族下的**行度量**：ascent 为
	// 基线以上的像素高度，descent 为基线以下的像素高度，二者之和即行盒
	// 高度。口径对齐浏览器 line-height:normal（Chrome 取字体 hhea 的
	// ascender+descender+lineGap），行盒按基线对齐各内联盒。
	// text 为空时返回主字体的度量（不含逐字回退）——用于行盒的 strut
	// （块级自身的字体）。
	FontMetrics(text string, fontSize int, bold bool, family string) (ascent, descent int)
}

// Clipper 是 Graphics 的可选扩展：矩形裁剪栈。overflow 非 visible 的
// 元素绘制其内容前 PushClip、画完 PopClip，超出裁剪区的像素不落笔。
// 实现方可自行选择不支持（不实现本接口时渲染层退化为不裁剪，内容
// 可能画出盒外——README 已知限制）。栈语义：每层与上一层求交集。
type Clipper interface {
	PushClip(x, y, w, h int)
	PopClip()
}

// Viewport 是渲染输出目标的抽象：一个有尺寸、可绘图的窗口/画布。
// 具体后端（如 SDL2）实现该接口；文档通过 OpenDocument 绑定到视口。
type Viewport interface {
	ViewportWidth() int
	ViewportHeight() int
	Graphics() Graphics
}
