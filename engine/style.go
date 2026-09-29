package engine

type BorderStyle uint8

const (
	BorderStyleNone BorderStyle = iota
	BorderStyleSolid
)

type Display uint8

const (
	DisplayNone Display = iota
	DisplayInline
	DisplayInlineBlock
	DisplayBlock
)

type Position uint8

const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
)

// isOutFlow 判断脱流定位（absolute/fixed 不参与常规流，二次 pass 布局）。
func (p Position) isOutFlow() bool {
	return p == PositionAbsolute || p == PositionFixed
}

type FontWeight uint8

const (
	FontWeightNormal FontWeight = iota
	FontWeightBold
)

type TextAlign uint8

const (
	TextAlignLeft TextAlign = iota // 默认（等同 CSS start）
	TextAlignCenter
	TextAlignRight
)

type BoxSizing uint8

const (
	BoxSizingContentBox BoxSizing = iota // width/height 仅为内容盒（CSS 默认）
	BoxSizingBorderBox                   // width/height 含 padding+border（不含 margin）
)

type OnCSSStyleDeclarationChanged interface {
	OnChanged()
}

// CSSStyleDeclaration 是可级联应用的样式对象。
// Width/Height 默认为 auto；盒模型四边默认 0px。
type CSSStyleDeclaration interface {
	Display() Display
	SetDisplay(display Display) CSSStyleDeclaration
	Width() Size
	SetWidth(w Size) CSSStyleDeclaration
	Height() Size
	SetHeight(h Size) CSSStyleDeclaration
	BackgroundColor() Color
	SetBackgroundColor(color Color) CSSStyleDeclaration
	BackgroundImage() Image
	SetBackgroundImage(image Image) CSSStyleDeclaration
	Padding() Rect
	SetPadding(r Rect) CSSStyleDeclaration
	Margin() Rect
	SetMargin(r Rect) CSSStyleDeclaration
	BorderStyleWidth() Rect
	SetBorderStyleWidth(r Rect) CSSStyleDeclaration
	BorderColor() Color
	SetBorderColor(color Color) CSSStyleDeclaration
	BorderImage() Image
	SetBorderImage(image Image) CSSStyleDeclaration
	BorderStyle() BorderStyle
	SetBorderStyle(style BorderStyle) CSSStyleDeclaration
	Color() Color
	SetColor(color Color) CSSStyleDeclaration
	FontSize() Size
	SetFontSize(size Size) CSSStyleDeclaration
	FontFamily() string
	SetFontFamily(family string) CSSStyleDeclaration
	FontWeight() FontWeight
	SetFontWeight(weight FontWeight) CSSStyleDeclaration
	TextAlign() TextAlign
	SetTextAlign(align TextAlign) CSSStyleDeclaration
	BorderRadius() Size
	SetBorderRadius(radius Size) CSSStyleDeclaration
	Position() Position
	SetPosition(pos Position) CSSStyleDeclaration
	// Inset 是 top/right/bottom/left 四边偏移（默认全 auto）：
	// relative 为偏移量，absolute/fixed 为相对包含块的锚距。
	Inset() Rect
	SetInset(r Rect) CSSStyleDeclaration
	ZIndex() int
	SetZIndex(z int) CSSStyleDeclaration
	// BoxSizing 决定 width/height 是否含 padding+border。Chrome UA 样式表将
	// 表单控件默认置为 border-box（见 style_ua.go），引擎按此对齐。
	BoxSizing() BoxSizing
	SetBoxSizing(b BoxSizing) CSSStyleDeclaration
}

func newStyle() CSSStyleDeclaration {
	style := &cssStyleDeclaration{}
	style.display = DisplayInlineBlock
	style.width = NewAutoSize()
	style.height = NewAutoSize()
	style.margin = NewZeroRect()
	style.padding = NewZeroRect()
	style.borderW = NewZeroRect()
	style.borderStyle = BorderStyleSolid
	style.borderRadius = NewZeroSize()
	style.inset = newAutoRect()
	return style
}

// newAutoRect 构造四边均为 auto 的 Rect（inset 初始值）。
func newAutoRect() Rect {
	a := NewAutoSize()
	return NewRect(a, a, a, a)
}

type cssStyleDeclaration struct {
	CSSStyleDeclaration

	display         Display
	width           Size
	height          Size
	margin          Rect
	padding         Rect
	borderW         Rect
	borderStyle     BorderStyle
	borderColor     Color
	backgroundColor Color
	backgroundImage Image
	color           Color
	fontSize        Size
	fontFamily      string
	fontWeight      FontWeight
	textAlign       TextAlign
	borderRadius    Size
	position        Position
	inset           Rect
	zindex          int
	boxSizing       BoxSizing
}

// copyStyle 复制样式值（不含继承字段之外的引用语义差异）。
func copyStyle(s CSSStyleDeclaration) CSSStyleDeclaration {
	c := newStyle()
	c.SetDisplay(s.Display())
	c.SetWidth(s.Width())
	c.SetHeight(s.Height())
	c.SetMargin(s.Margin())
	c.SetPadding(s.Padding())
	c.SetBorderStyleWidth(s.BorderStyleWidth())
	c.SetBorderColor(s.BorderColor())
	c.SetBorderStyle(s.BorderStyle())
	c.SetBackgroundColor(s.BackgroundColor())
	c.SetBackgroundImage(s.BackgroundImage())
	c.SetColor(s.Color())
	c.SetFontSize(s.FontSize())
	c.SetFontFamily(s.FontFamily())
	c.SetFontWeight(s.FontWeight())
	c.SetTextAlign(s.TextAlign())
	c.SetBorderRadius(s.BorderRadius())
	c.SetPosition(s.Position())
	c.SetInset(s.Inset())
	c.SetZIndex(s.ZIndex())
	c.SetBoxSizing(s.BoxSizing())
	return c
}

func (style *cssStyleDeclaration) Display() Display {
	return style.display
}

func (style *cssStyleDeclaration) SetDisplay(display Display) CSSStyleDeclaration {
	style.display = display
	return style
}

func (style *cssStyleDeclaration) Width() Size {
	return style.width
}

func (style *cssStyleDeclaration) SetWidth(w Size) CSSStyleDeclaration {
	if w != nil {
		style.width = w
	}
	return style
}

func (style *cssStyleDeclaration) Height() Size {
	return style.height
}

func (style *cssStyleDeclaration) SetHeight(h Size) CSSStyleDeclaration {
	if h != nil {
		style.height = h
	}
	return style
}

func (style *cssStyleDeclaration) Padding() Rect {
	return style.padding
}

func (style *cssStyleDeclaration) SetPadding(r Rect) CSSStyleDeclaration {
	if r != nil {
		style.padding = r
	}
	return style
}

func (style *cssStyleDeclaration) Margin() Rect {
	return style.margin
}

func (style *cssStyleDeclaration) SetMargin(r Rect) CSSStyleDeclaration {
	if r != nil {
		style.margin = r
	}
	return style
}

func (style *cssStyleDeclaration) BorderStyleWidth() Rect {
	return style.borderW
}

func (style *cssStyleDeclaration) SetBorderStyleWidth(r Rect) CSSStyleDeclaration {
	if r != nil {
		style.borderW = r
	}
	return style
}

func (style *cssStyleDeclaration) BorderColor() Color {
	return style.borderColor
}

func (style *cssStyleDeclaration) SetBorderColor(color Color) CSSStyleDeclaration {
	style.borderColor = color
	return style
}

func (style *cssStyleDeclaration) BorderImage() Image {
	return nil
}

func (style *cssStyleDeclaration) SetBorderImage(image Image) CSSStyleDeclaration {
	return style
}

func (style *cssStyleDeclaration) BorderStyle() BorderStyle {
	return style.borderStyle
}

func (style *cssStyleDeclaration) SetBorderStyle(s BorderStyle) CSSStyleDeclaration {
	style.borderStyle = s
	return style
}

func (style *cssStyleDeclaration) BackgroundColor() Color {
	return style.backgroundColor
}

func (style *cssStyleDeclaration) SetBackgroundColor(color Color) CSSStyleDeclaration {
	style.backgroundColor = color
	return style
}

func (style *cssStyleDeclaration) BackgroundImage() Image {
	return style.backgroundImage
}

func (style *cssStyleDeclaration) SetBackgroundImage(image Image) CSSStyleDeclaration {
	style.backgroundImage = image
	return style
}

func (style *cssStyleDeclaration) Color() Color {
	return style.color
}

func (style *cssStyleDeclaration) SetColor(color Color) CSSStyleDeclaration {
	style.color = color
	return style
}

func (style *cssStyleDeclaration) FontSize() Size {
	return style.fontSize
}

func (style *cssStyleDeclaration) SetFontSize(size Size) CSSStyleDeclaration {
	if size != nil {
		style.fontSize = size
	}
	return style
}

func (style *cssStyleDeclaration) FontFamily() string {
	return style.fontFamily
}

func (style *cssStyleDeclaration) SetFontFamily(family string) CSSStyleDeclaration {
	style.fontFamily = family
	return style
}

func (style *cssStyleDeclaration) FontWeight() FontWeight {
	return style.fontWeight
}

func (style *cssStyleDeclaration) SetFontWeight(weight FontWeight) CSSStyleDeclaration {
	style.fontWeight = weight
	return style
}

func (style *cssStyleDeclaration) TextAlign() TextAlign {
	return style.textAlign
}

func (style *cssStyleDeclaration) SetTextAlign(align TextAlign) CSSStyleDeclaration {
	style.textAlign = align
	return style
}

func (style *cssStyleDeclaration) BorderRadius() Size {
	return style.borderRadius
}

func (style *cssStyleDeclaration) SetBorderRadius(radius Size) CSSStyleDeclaration {
	if radius != nil {
		style.borderRadius = radius
	}
	return style
}

func (style *cssStyleDeclaration) Position() Position {
	return style.position
}

func (style *cssStyleDeclaration) SetPosition(pos Position) CSSStyleDeclaration {
	style.position = pos
	return style
}

func (style *cssStyleDeclaration) Inset() Rect {
	return style.inset
}

func (style *cssStyleDeclaration) SetInset(r Rect) CSSStyleDeclaration {
	if r != nil {
		style.inset = r
	}
	return style
}

func (style *cssStyleDeclaration) ZIndex() int {
	return style.zindex
}

func (style *cssStyleDeclaration) SetZIndex(z int) CSSStyleDeclaration {
	style.zindex = z
	return style
}

func (style *cssStyleDeclaration) BoxSizing() BoxSizing {
	return style.boxSizing
}

func (style *cssStyleDeclaration) SetBoxSizing(b BoxSizing) CSSStyleDeclaration {
	style.boxSizing = b
	return style
}
