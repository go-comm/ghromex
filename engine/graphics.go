package engine

import (
	"image"
	"image/png"
	"os"
	"strings"
)

type SizeType uint8

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

func NewZeroSize() Size {
	return &size{tp: SIZE_PIXEL, intVal: 0, float32Val: 0}
}

// NewAutoSize 表示 CSS 中的 auto。
func NewAutoSize() Size {
	return &size{tp: SIZE_AUTO}
}

func NewSize(tp SizeType, intVal int, float32Val float32) Size {
	return &size{tp: tp, intVal: intVal, float32Val: float32Val}
}

type size struct {
	tp         SizeType
	intVal     int
	float32Val float32
}

func (s *size) Type() SizeType {
	return s.tp
}

func (s *size) Pixel() int {
	return s.intVal
}

func (s *size) SetPixel(v int) {
	s.tp = SIZE_PIXEL
	s.intVal = v
}

func (s *size) Point() int {
	return s.intVal
}

func (s *size) SetPoint(v int) {
	s.tp = SIZE_POINT
	s.intVal = v
}

func (s *size) Displayport() int {
	return s.intVal
}

func (s *size) SetDisplayport(v int) {
	s.tp = SIZE_DISPLAYPORT
	s.intVal = v
}

func (s *size) Percent() float32 {
	return s.float32Val
}

func (s *size) SetPercent(v float32) {
	s.tp = SIZE_PERCENT
	s.float32Val = v
}

func (s *size) Disc(int) int {
	return 0
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

// Color 表示 RGBA 颜色。
type Color interface {
	RGBA() (r, g, b, a uint8)
}

type colorVal struct {
	r, g, b, a uint8
}

func NewColor(r, g, b, a uint8) Color {
	return &colorVal{r: r, g: g, b: b, a: a}
}

func (c *colorVal) RGBA() (r, g, b, a uint8) {
	return c.r, c.g, c.b, c.a
}

type Image interface {
}

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

func NewPaint() Paint {
	p := &paint{size: NewSize(SIZE_PIXEL, 16, 0), color: NewColor(0, 0, 0, 255)}
	return p
}

type paint struct {
	size   Size
	color  Color
	family string
	bold   bool
	bg     Color
}

func (p *paint) Size() Size {
	return p.size
}

func (p *paint) SetSize(size Size) {
	if size != nil {
		p.size = size
	}
}

func (p *paint) Color() Color {
	return p.color
}

func (p *paint) SetColor(color Color) {
	if color != nil {
		p.color = color
	}
}

func (p *paint) FontFamily() string {
	return p.family
}

func (p *paint) SetFontFamily(family string) {
	p.family = family
}

func (p *paint) Bold() bool {
	return p.bold
}

func (p *paint) SetBold(bold bool) {
	p.bold = bold
}

func (p *paint) Background() Color {
	return p.bg
}

func (p *paint) SetBackground(color Color) {
	p.bg = color
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
}

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

	DrawCalls int
	TextCalls int
	Texts     []string // 按绘制顺序记录的文本内容
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

// FillRect 填充一个整数矩形（自动裁剪到缓冲区内）。
func (g *BufferGraphics) FillRect(x, y, w, h int, r, gg, b, a uint8) {
	if w <= 0 || h <= 0 || a == 0 {
		return
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
