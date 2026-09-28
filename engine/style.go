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

type FontWeight uint8

const (
	FontWeightNormal FontWeight = iota
	FontWeightBold
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
	return style
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
