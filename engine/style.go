package engine

type BorderStyle uint8

const (
	BorderStyleNone BorderStyle = iota
	BorderStyleSolid
)

// BorderSide 是边框四边的下标。顺序与渲染内部的 eTop/eRight/eBottom/eLeft
// 一致：这样样式层、解析层与绘制层可以共用一份 [4] 数组，不必在中间反复
// 做名字到索引的映射。
type BorderSide uint8

const (
	BorderSideTop BorderSide = iota
	BorderSideRight
	BorderSideBottom
	BorderSideLeft
)

// Shadow 是一个 box-shadow 值。
//
// 长度在解析期就落成像素（box-shadow 的模糊/扩散半径不接受百分比；偏移
// 虽允许百分比，但 UI 与对照页只用 px，本轮不引入带单位的 Shadow，避免
// 在绘制期还要拿元素盒宽度做二次解析）。Color 为 nil 表示 currentColor：
// 级联期不知道元素最终文字色，留到绘制期回退。
type Shadow struct {
	X, Y   int // 偏移
	Blur   int // 模糊半径
	Spread int // 扩散半径（可为负）
	Color  Color
	// Inset 为真表示内阴影：阴影区是「padding 盒内、偏移并按 spread 收缩后
	// 的形状之外」，按 padding 盒（含圆角）裁切，画在背景之上、内容之下。
	Inset bool
}

// Display 是 display 计算值。前四个是常规流值；其余为表格家族（CSS 2.1
// 表格模型）：table/inline-table 生成表格盒（列对齐 + 行堆叠的专用布局），
// table-caption/table-header-group/table-row-group/table-footer-group/
// table-row/table-cell 分别生成题注、行组、行、单元格盒，由最近的表格
// 祖先收集进网格；table-column/table-column-group 不生成盒（只提供列宽
// 约束）。元素在表格中的角色只由 display 判定，与标签名无关——HTML
// <table> 靠 UA 样式表落位，任意元素用 display: table 同样能排成表格。
type Display uint8

const (
	DisplayNone Display = iota
	DisplayInline
	DisplayInlineBlock
	DisplayBlock
	// DisplayTable 生成块级表格盒（auto 宽度按网格收缩，不参与 margin 折叠）。
	DisplayTable
	// DisplayInlineTable 生成行内级表格盒：与 DisplayTable 同一套网格布局，
	// 差别只在参与行内排版（随文字流、可换行）。
	DisplayInlineTable
	// DisplayTableCaption 生成题注盒；在表格内排于网格上方或下方（见
	// CaptionSide），在表格外退化为普通块盒。
	DisplayTableCaption
	// DisplayTableColumn 生成列盒（CSS 里不产生几何），仅 col/colgroup
	// 的宽度在此充当列宽下限。
	DisplayTableColumn
	// DisplayTableColumnGroup 是 table-column-group：不产生几何，子 col
	// 的宽度约束照常收集。
	DisplayTableColumnGroup
	// DisplayTableHeaderGroup / DisplayTableFooterGroup 是表头/表尾行组：
	// 视觉顺序固定为「表头组 → 行组 → 表尾组」，与源码顺序无关（CSS 2.1
	// §17.5.1.2，Chrome 同款——tfoot 写在 tbody 前面也排最后）。
	DisplayTableHeaderGroup
	DisplayTableFooterGroup
	// DisplayTableRowGroup 是普通行组（tbody 的 UA 值）。
	DisplayTableRowGroup
	// DisplayTableRow 生成行盒：行等高（取行内最高单元格）。
	DisplayTableRow
	// DisplayTableCell 生成单元格盒：content 自动宽度按内容收缩测量，
	// 定列后拉伸到列宽并随单元格 vertical-align 落位。
	DisplayTableCell
)

// isTableRowGroup 判断是否为三种行组 display。
func (d Display) isTableRowGroup() bool {
	return d == DisplayTableRowGroup || d == DisplayTableHeaderGroup || d == DisplayTableFooterGroup
}

// CaptionSide 是 caption-side 的取值（继承属性，只被表格布局读取）。
// left/right 是竖排表格的取值，引擎按 top/bottom 两档处理（其余值回退 top）。
type CaptionSide uint8

const (
	CaptionSideTop    CaptionSide = iota // 默认：题注在网格上方
	CaptionSideBottom                    // 题注在网格下方
)

// EmptyCells 是 empty-cells 的取值（继承属性，只作用于表格单元格）。
// hide 时「空单元格」（无元素子节点、无非空白文本）的背景与边框不绘制
// （CSS 2.1 §17.5.1.1）——只隐绘制，格位/列宽/命中保留，Chrome 同口径。
type EmptyCells uint8

const (
	EmptyCellsShow EmptyCells = iota // 默认
	EmptyCellsHide
)

// ListStyleType 是 list-style-type 的计算值：列表标记的形状或编号方案
// （继承属性，只有 li 的标记绘制读取）。引擎没有 display:list-item，标记
// 仍按「ul/ol 的 li 子元素」生成，但标记形态改由该属性决定（含 none）。
type ListStyleType uint8

const (
	ListStyleDisc               ListStyleType = iota // 默认：实心圆点
	ListStyleCircle                                  // 空心圆圈
	ListStyleSquare                                  // 实心方块
	ListStyleDecimal                                 // 1. 2. 3.
	ListStyleDecimalLeadingZero                      // 01. 02. 03.
	ListStyleLowerAlpha                              // a. b. c.
	ListStyleUpperAlpha                              // A. B. C.
	ListStyleLowerRoman                              // i. ii. iii.
	ListStyleUpperRoman                              // I. II. III.
	ListStyleNone                                    // 不显示标记
)

// isBulletListStyle 判断是否为符号类标记（圆点/圆圈/方块）。符号类的绘制
// 间隙比编号类大（Chrome 实测 0.875em vs 0.6em）。
func isBulletListStyle(t ListStyleType) bool {
	return t == ListStyleDisc || t == ListStyleCircle || t == ListStyleSquare
}

// ListStylePosition 是 list-style-position 的计算值（继承属性）。
// outside 把标记右对齐到内容盒左侧；inside 把标记放在内容盒内首行之前
// （首行内容整体右移标记宽度 + 间隙）。
type ListStylePosition uint8

const (
	ListStyleOutside ListStylePosition = iota // 默认：标记在内容盒外
	ListStyleInside                           // 标记在内容盒内、首行之前
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

// FontStyle 是 font-style 的计算值。引擎只区分正体与斜体：italic 与
// oblique 都落到 FontStyleItalic，由绘制后端用**合成斜体**（SDL_ttf 的
// TTF_STYLE_ITALIC）呈现——引擎不带真正的斜体字面（italic face），
// 合成斜体的字形前进宽度与正体一致，因此布局度量不需要区分字形样式。
type FontStyle uint8

const (
	FontStyleNormal FontStyle = iota
	FontStyleItalic
)

// Visibility 是 visibility 的计算值。hidden/collapse 的盒子照常参与布局
// （占位、影响兄弟位置），只是自身与（默认继承的）子树不绘制、不参与
// 命中；子级显式声明 visible 可以重新显示（CSS 语义）。
type Visibility uint8

const (
	VisibilityVisible Visibility = iota
	VisibilityHidden
	// VisibilityCollapse 对非表格元素等同 hidden（引擎没有行/列折叠布局）。
	VisibilityCollapse
)

// FloatSide 是 float 的计算值：左浮动、右浮动或不分浮动。
// 浮动盒脱流：不推进游标、不占行高，但会影响同一块格式化上下文里
// 后续行盒的左右边界（文字绕排）与 clear 的下移。
type FloatSide uint8

const (
	FloatNone FloatSide = iota
	FloatLeft
	FloatRight
)

// ClearSide 是 clear 的计算值：块级元素的上外边距边被下推到指定方向上
// 所有浮动盒的下缘之下。
type ClearSide uint8

const (
	ClearNone ClearSide = iota
	ClearLeft
	ClearRight
	ClearBoth
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

// Overflow 控制盒子内容溢出时的处理。visible 不裁剪；hidden 裁剪但不可滚；
// scroll/auto 裁剪且可滚（auto 仅在内容确实溢出时才可滚）。
type Overflow uint8

const (
	OverflowVisible Overflow = iota // 默认：溢出可见（不裁剪不滚动）
	OverflowHidden                  // 裁剪，不可滚
	OverflowScroll                  // 裁剪，恒可滚（滚动条与否不影响滚轮）
	OverflowAuto                    // 裁剪，溢出时可滚
)

// TextDecoration 文本装饰（下划线/删除线）。CSS 该属性沿父链继承，
// <a> 的 UA 下划线正是靠继承落到其文本子节点上。
type TextDecoration uint8

const (
	TextDecorationNone TextDecoration = iota
	TextDecorationUnderline
	TextDecorationLineThrough
)

// WhiteSpace 控制空白折叠与换行（white-space）。CSS Text §4.1 用三条正交
// 行为描述这一族取值——「折叠空白」「保留换行」「软换行」：
//
//	值          折叠空白  保留换行  软换行
//	normal      是        否        是
//	nowrap      是        否        否
//	pre         否        是        否
//	pre-wrap    否        是        是
//	pre-line    是        是        是
//
// <pre> 的 UA 值是 pre：保留缩进/制表符、按换行强制断行、长行溢出而不是
// 折行。缺这一档时 <pre> 里的换行与缩进会被当成普通文本折叠掉，与浏览器
// 视觉差一整行。pre-wrap / pre-line / nowrap 是代码块与「不换行」标题的
// 常用写法（GitHub 的代码块就是 pre-wrap）。
type WhiteSpace uint8

const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpaceNowrap
	WhiteSpacePre
	WhiteSpacePreWrap
	WhiteSpacePreLine
)

// PointerEvents 决定元素是否参与命中测试（pointer-events）。只实现两档：
// auto 是 CSS 默认（正常接收点击）；none 让元素自己不再是被点中的目标。
//
// 为什么要有这一档：外壳与页面里存在「只为排版而存在」的盒子——ui 用来给
// fixed 菜单栏/状态栏让位的占位块、1px 分隔线。它们按 DOM 序排在页面内容
// 之后，于是盒子里层的内容虽然照样画得出来（渲染层按 DOM 序叠放），点击却
// 被这些透明盒子吃掉，表现为「看得见点不到」。真实 CSS 里这类装饰元素正是
// 用 pointer-events:none 交还交互的。
//
// 与 CSS 一致，该属性沿父链继承：父级 none 时子树默认也不可点，子级自己
// 声明 auto 可以恢复。
type PointerEvents uint8

const (
	PointerEventsAuto PointerEvents = iota
	PointerEventsNone
)

// LineHeightKind 是 line-height 的取值形态。
type LineHeightKind uint8

const (
	// LineHeightNormal 是 line-height: normal——行高取字体度量
	// （ascent + descent），不做额外行距调整。
	LineHeightNormal LineHeightKind = iota
	// LineHeightFactor 是相对**元素自身字号**的倍数：line-height:1.5 与
	// line-height:150% 同义（两者都按继承下来的倍数、相对各自元素的字号
	// 解析），故归为同一形态。
	LineHeightFactor
	// LineHeightLength 是绝对行高（px）。
	LineHeightLength
)

// LineHeight 是 line-height 的值：normal / 相对字号的倍数 / 绝对长度。
//
// 倍数与绝对长度都**继承原始形态**而非解析后的像素值——倍数要相对子元素
// 自己的字号解析（父级 line-height:1.5 + 子级 font-size:32px → 子级行高
// 48px），继承成像素就全错了。
type LineHeight struct {
	kind  LineHeightKind
	value float32
}

// NewLineHeightNormal 造 line-height: normal。
func NewLineHeightNormal() LineHeight { return LineHeight{kind: LineHeightNormal} }

// NewLineHeightFactor 造相对字号的倍数行高（1.5 表示 1.5em）。
func NewLineHeightFactor(f float32) LineHeight {
	return LineHeight{kind: LineHeightFactor, value: f}
}

// NewLineHeightLength 造绝对行高（px）。
func NewLineHeightLength(px float32) LineHeight {
	return LineHeight{kind: LineHeightLength, value: px}
}

// Kind 返回取值形态。
func (l LineHeight) Kind() LineHeightKind { return l.kind }

// Factor 返回倍数（仅 LineHeightFactor 有效）。
func (l LineHeight) Factor() float32 { return l.value }

// Length 返回绝对行高（仅 LineHeightLength 有效）。
func (l LineHeight) Length() float32 { return l.value }

// VerticalAlign 是 vertical-align 的取值（**不继承**）。只作用于行内级盒
// （引擎里行内元素都是"原子盒"，见 measure.go 的 inlineBoxBaseline）。
type VerticalAlign uint8

const (
	// VerticalAlignBaseline 是默认值：盒的基线与父级基线对齐。
	VerticalAlignBaseline VerticalAlign = iota
	// VerticalAlignTop：盒的顶边与行盒顶边对齐。
	VerticalAlignTop
	// VerticalAlignMiddle：盒的垂直中点与「父级基线 + 半个 x 高度」对齐。
	VerticalAlignMiddle
	// VerticalAlignBottom：盒的底边与行盒底边对齐。
	VerticalAlignBottom
	// VerticalAlignTextTop：盒的顶边与父级内容区上沿（strut 的 ascent）对齐。
	VerticalAlignTextTop
	// VerticalAlignTextBottom：盒的底边与父级内容区下沿（strut 的 descent）对齐。
	VerticalAlignTextBottom
	// VerticalAlignSub：相对基线下移（父级字号/5+1，与 Chrome 的 <sub> 同口径）。
	VerticalAlignSub
	// VerticalAlignSuper：相对基线上移（父级字号/3+1，与 Chrome 的 <sup> 同口径）。
	VerticalAlignSuper
	// VerticalAlignRaise：长度/百分比，相对基线上移指定距离。
	VerticalAlignRaise
)

// BorderCollapse 决定表格相邻单元格的边框是否合并（border-collapse），
// 只被表格布局读取。separate 为 CSS 默认（单元格各自画四边边框，相邻两条各
// 自 1px）；collapse 时相邻单元格共享一条线——浏览器把它实现成每个单元格只
// 摊到"半个边框"（实测 Chrome：`border: 1px` + `padding: 4px 8px` 的单元格
// 外盒 = 内容 + 16 + 1），外缘的半个边框归表格自己。
type BorderCollapse uint8

const (
	BorderCollapseSeparate BorderCollapse = iota // 默认
	BorderCollapseCollapse
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
	// BorderColor/SetBorderColor 是四边同色的兼容入口（读顶边、写四边）；
	// 单边颜色用 BorderColorFor/SetBorderColorFor。
	BorderColor() Color
	SetBorderColor(color Color) CSSStyleDeclaration
	BorderColorFor(side BorderSide) Color
	SetBorderColorFor(side BorderSide, color Color) CSSStyleDeclaration
	BorderImage() Image
	SetBorderImage(image Image) CSSStyleDeclaration
	// BorderStyle/SetBorderStyle 同样保留「读顶边、写四边」的旧语义。
	BorderStyle() BorderStyle
	SetBorderStyle(style BorderStyle) CSSStyleDeclaration
	BorderStyleFor(side BorderSide) BorderStyle
	SetBorderStyleFor(side BorderSide, style BorderStyle) CSSStyleDeclaration
	// BoxShadow 是 box-shadow 声明（逗号分隔的多条阴影），默认 nil。
	BoxShadow() []Shadow
	SetBoxShadow(shadows []Shadow) CSSStyleDeclaration
	Color() Color
	SetColor(color Color) CSSStyleDeclaration
	FontSize() Size
	SetFontSize(size Size) CSSStyleDeclaration
	FontFamily() string
	SetFontFamily(family string) CSSStyleDeclaration
	FontWeight() FontWeight
	SetFontWeight(weight FontWeight) CSSStyleDeclaration
	// FontStyle 是 font-style 的计算值（normal/italic），沿父链继承。
	// 斜体由绘制后端合成（见 FontStyle 的说明）。
	FontStyle() FontStyle
	SetFontStyle(style FontStyle) CSSStyleDeclaration
	// Visibility 决定元素自身是否绘制/可命中（hidden/collapse），沿父链
	// 继承；布局位置不受影响，子级可声明 visible 重新显示。
	Visibility() Visibility
	SetVisibility(v Visibility) CSSStyleDeclaration
	// FloatSide 是 float 的计算值（none/left/right），不继承。
	Float() FloatSide
	SetFloat(f FloatSide) CSSStyleDeclaration
	// Clear 是 clear 的计算值（none/left/right/both），不继承。
	Clear() ClearSide
	SetClear(c ClearSide) CSSStyleDeclaration
	// Opacity 是 opacity 的计算值（0~1），不继承。引擎没有图层合成，
	// 按「子树所有绘制颜色的 alpha 乘以该值」近似（见 README 已知限制）。
	Opacity() float32
	SetOpacity(o float32) CSSStyleDeclaration
	// Cursor 是 cursor 的计算值（命中时使用的系统指针形状），沿父链继承。
	Cursor() Cursor
	SetCursor(c Cursor) CSSStyleDeclaration
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
	// TextDecoration 为文本装饰（underline/line-through/none），随父链继承。
	TextDecoration() TextDecoration
	SetTextDecoration(d TextDecoration) CSSStyleDeclaration
	// OverflowX/OverflowY 为水平/纵向溢出处理（overflow-x / overflow-y，
	// 简写 overflow 两值同写）。默认 visible。
	OverflowX() Overflow
	SetOverflowX(o Overflow) CSSStyleDeclaration
	OverflowY() Overflow
	SetOverflowY(o Overflow) CSSStyleDeclaration
	// WhiteSpace 决定空白处理（normal/pre），沿父链继承。
	WhiteSpace() WhiteSpace
	SetWhiteSpace(w WhiteSpace) CSSStyleDeclaration
	// PointerEvents 决定元素是否参与命中测试（auto/none），沿父链继承；
	// 只有命中测试读它，渲染与布局不受影响。
	PointerEvents() PointerEvents
	SetPointerEvents(p PointerEvents) CSSStyleDeclaration
	// BorderCollapse 决定表格相邻单元格边框是否合并（border-collapse），
	// 不继承、仅表格布局读取。默认 separate。
	BorderCollapse() BorderCollapse
	SetBorderCollapse(b BorderCollapse) CSSStyleDeclaration
	// BorderSpacing 是 border-spacing 的两轴间距（不继承，仅表格布局读取）。
	// 初始值 0；HTML <table> 的 UA 样式表把它置为 2px（Chrome 同款——
	// 2px 是 UA 对表格元素的默认，不是 CSS 初始值，display:table 的普通
	// 元素不带）。语义为 separate 模式下单元格网格四周与相邻格之间的
	// 间隙；collapse 模式下恒无效。
	BorderSpacing() (h, v Size)
	SetBorderSpacing(h, v Size) CSSStyleDeclaration
	// EmptyCells 决定空单元格是否绘制背景与边框（empty-cells），沿父链
	// 继承、仅表格布局读取。默认 show。
	EmptyCells() EmptyCells
	SetEmptyCells(e EmptyCells) CSSStyleDeclaration
	// CaptionSide 决定题注排在网格上方还是下方（caption-side），沿父链
	// 继承、仅表格布局读取。默认 top。
	CaptionSide() CaptionSide
	SetCaptionSide(c CaptionSide) CSSStyleDeclaration
	// ListStyleType 是列表标记的形状/编号方案（list-style-type），沿父链
	// 继承，只有 li 的标记绘制读取。默认 disc（ol 的 UA 样式表置 decimal）。
	ListStyleType() ListStyleType
	SetListStyleType(t ListStyleType) CSSStyleDeclaration
	// ListStylePosition 决定标记画在内容盒外（默认）还是盒内首行之前
	// （list-style-position），沿父链继承。
	ListStylePosition() ListStylePosition
	SetListStylePosition(p ListStylePosition) CSSStyleDeclaration
	// LineHeight 是 line-height 的值（normal/倍数/绝对长度），沿父链继承
	// 的是**原始形态**（倍数相对子元素自己的字号解析）。
	LineHeight() LineHeight
	SetLineHeight(lh LineHeight) CSSStyleDeclaration
	// VerticalAlign 是 vertical-align 的值（不继承），只作用于行内级盒。
	VerticalAlign() VerticalAlign
	SetVerticalAlign(v VerticalAlign) CSSStyleDeclaration
	// VerticalAlignOffset 是 vertical-align 为长度/百分比时的抬升值
	// （百分比相对元素自身的 line-height）。
	VerticalAlignOffset() Size
	SetVerticalAlignOffset(s Size) CSSStyleDeclaration
}

func newStyle() CSSStyleDeclaration {
	style := &cssStyleDeclaration{}
	style.display = DisplayInlineBlock
	style.width = NewAutoSize()
	style.height = NewAutoSize()
	style.margin = NewZeroRect()
	style.padding = NewZeroRect()
	style.borderW = NewZeroRect()
	// 四边默认 solid（宽度为 0 时不画）：与旧版单一 borderStyle 的默认值
	// 一致，作者只写 border-width 时仍能看到边框。
	for i := range style.borderStyles {
		style.borderStyles[i] = BorderStyleSolid
	}
	style.borderRadius = NewZeroSize()
	style.inset = newAutoRect()
	style.borderSpacingH = NewZeroSize()
	style.borderSpacingV = NewZeroSize()
	// opacity 初始值为 1（完全不透明）：0 是合法声明值（全透明），不能
	// 用零值当默认。
	style.opacity = 1
	return style
}

// newAutoRect 构造四边均为 auto 的 Rect（inset 初始值）。
func newAutoRect() Rect {
	a := NewAutoSize()
	return NewRect(a, a, a, a)
}

// 与 htmlElement 同样的三段式排布：含指针的字段（Size/Rect/Color/Image 都是
// 接口，加上切片、map、string）集中在前，纯数值居中，22 个单字节枚举与标记
// 收尾。原顺序里每个枚举后面都被插进 1~7B 的对齐空洞，合计 42B；紧凑排列后
// sizeof 由 440B 降到 400B——每个节点都持有一个样式对象，六万节点约省 2.5MB。
type cssStyleDeclaration struct {
	// —— 第一段：含指针
	CSSStyleDeclaration
	width            Size
	height           Size
	margin           Rect
	padding          Rect
	borderW          Rect
	inset            Rect
	borderColors     [4]Color
	boxShadow        []Shadow
	backgroundColor  Color
	backgroundImage  Image
	color            Color
	fontSize         Size
	borderRadius     Size
	borderSpacingH   Size
	borderSpacingV   Size
	verticalAlignOff Size
	fontFamily       string

	// —— 第二段：纯数值
	// fontWeightNum 保留 font-weight 的**数值**原值（100~1000），0 表示
	// 声明未携带数字（UA 直接写 bold/normal、或外部实现 SetFontWeight）。
	// 公开接口仍是两档 FontWeight（外部实现不受影响）；后端选择中间字重
	// 字面（600 → Segoe UI Semibold）时用它，见 core.WeightedPaint。
	fontWeightNum int
	zindex        int
	lineHeight    LineHeight
	opacity       float32

	// —— 第三段：单字节枚举与标记
	display      Display
	borderStyles [4]BorderStyle
	// fontSizeExplicit 记录字号是否来自显式 font-size 声明（含祖先继承）。
	// Chrome 的 monospace 通用族只在字号仍是「默认值」时才用 13px 固定字号
	// （作者显式给过 16px 时保持 16），需要这个来源标记来判断。
	fontSizeExplicit bool
	fontWeight       FontWeight
	fontStyle        FontStyle
	visibility       Visibility
	floatSide        FloatSide
	clearSide        ClearSide
	cursor           Cursor
	textAlign        TextAlign
	position         Position
	boxSizing        BoxSizing
	textDecoration   TextDecoration
	overflowX        Overflow
	overflowY        Overflow
	whiteSpace       WhiteSpace
	pointerEvents    PointerEvents
	borderCollapse   BorderCollapse
	emptyCells       EmptyCells
	captionSide      CaptionSide
	listStyleType    ListStyleType
	listStylePos     ListStylePosition
	verticalAlign    VerticalAlign
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
	for side := BorderSideTop; side <= BorderSideLeft; side++ {
		c.SetBorderColorFor(side, s.BorderColorFor(side))
		c.SetBorderStyleFor(side, s.BorderStyleFor(side))
	}
	c.SetBoxShadow(s.BoxShadow())
	c.SetBackgroundColor(s.BackgroundColor())
	c.SetBackgroundImage(s.BackgroundImage())
	c.SetColor(s.Color())
	c.SetFontSize(s.FontSize())
	c.SetFontFamily(s.FontFamily())
	c.SetFontWeight(s.FontWeight())
	setFontWeightNumOf(c, fontWeightNumOf(s))
	c.SetFontStyle(s.FontStyle())
	c.SetVisibility(s.Visibility())
	c.SetFloat(s.Float())
	c.SetClear(s.Clear())
	c.SetOpacity(s.Opacity())
	c.SetCursor(s.Cursor())
	c.SetTextAlign(s.TextAlign())
	c.SetBorderRadius(s.BorderRadius())
	c.SetPosition(s.Position())
	c.SetInset(s.Inset())
	c.SetZIndex(s.ZIndex())
	c.SetBoxSizing(s.BoxSizing())
	c.SetTextDecoration(s.TextDecoration())
	c.SetOverflowX(s.OverflowX())
	c.SetOverflowY(s.OverflowY())
	c.SetWhiteSpace(s.WhiteSpace())
	c.SetPointerEvents(s.PointerEvents())
	c.SetBorderCollapse(s.BorderCollapse())
	sph, spv := s.BorderSpacing()
	c.SetBorderSpacing(sph, spv)
	c.SetEmptyCells(s.EmptyCells())
	c.SetCaptionSide(s.CaptionSide())
	c.SetListStyleType(s.ListStyleType())
	c.SetListStylePosition(s.ListStylePosition())
	c.SetLineHeight(s.LineHeight())
	c.SetVerticalAlign(s.VerticalAlign())
	c.SetVerticalAlignOffset(s.VerticalAlignOffset())
	if src, ok := s.(*cssStyleDeclaration); ok {
		if dst, ok := c.(*cssStyleDeclaration); ok {
			dst.fontSizeExplicit = src.fontSizeExplicit
		}
	}
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
	return style.borderColors[BorderSideTop]
}

func (style *cssStyleDeclaration) SetBorderColor(color Color) CSSStyleDeclaration {
	for side := BorderSideTop; side <= BorderSideLeft; side++ {
		style.SetBorderColorFor(side, color)
	}
	return style
}

func (style *cssStyleDeclaration) BorderColorFor(side BorderSide) Color {
	if side > BorderSideLeft {
		return nil
	}
	return style.borderColors[side]
}

func (style *cssStyleDeclaration) SetBorderColorFor(side BorderSide, color Color) CSSStyleDeclaration {
	if side > BorderSideLeft {
		return style
	}
	style.borderColors[side] = color
	return style
}

func (style *cssStyleDeclaration) BorderImage() Image {
	return nil
}

func (style *cssStyleDeclaration) SetBorderImage(image Image) CSSStyleDeclaration {
	return style
}

func (style *cssStyleDeclaration) BorderStyle() BorderStyle {
	return style.borderStyles[BorderSideTop]
}

func (style *cssStyleDeclaration) SetBorderStyle(s BorderStyle) CSSStyleDeclaration {
	for side := BorderSideTop; side <= BorderSideLeft; side++ {
		style.SetBorderStyleFor(side, s)
	}
	return style
}

func (style *cssStyleDeclaration) BorderStyleFor(side BorderSide) BorderStyle {
	if side > BorderSideLeft {
		return BorderStyleNone
	}
	return style.borderStyles[side]
}

func (style *cssStyleDeclaration) SetBorderStyleFor(side BorderSide, s BorderStyle) CSSStyleDeclaration {
	if side > BorderSideLeft {
		return style
	}
	style.borderStyles[side] = s
	return style
}

func (style *cssStyleDeclaration) BoxShadow() []Shadow {
	// 返回副本：调用方拿到切片后可能排序/修改，不应反向污染计算样式
	// （同一份 computed 会被绘制、SVG 导出和命中测试共享）。
	return append([]Shadow(nil), style.boxShadow...)
}

func (style *cssStyleDeclaration) SetBoxShadow(shadows []Shadow) CSSStyleDeclaration {
	style.boxShadow = append([]Shadow(nil), shadows...)
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

// fontWeightNumOf 读取计算样式的数值字重（未携带时返回 0）。公开的
// CSSStyleDeclaration 接口保持两档不动，内部用具体类型读取。
func fontWeightNumOf(s CSSStyleDeclaration) int {
	if c, ok := s.(*cssStyleDeclaration); ok {
		return c.fontWeightNum
	}
	return 0
}

// setFontWeightNumOf 写入数值字重（同 fontWeightNumOf 的接口口径）。
func setFontWeightNumOf(s CSSStyleDeclaration, n int) {
	if c, ok := s.(*cssStyleDeclaration); ok {
		c.fontWeightNum = n
	}
}

func (style *cssStyleDeclaration) FontStyle() FontStyle {
	return style.fontStyle
}

func (style *cssStyleDeclaration) SetFontStyle(s FontStyle) CSSStyleDeclaration {
	style.fontStyle = s
	return style
}

func (style *cssStyleDeclaration) Visibility() Visibility {
	return style.visibility
}

func (style *cssStyleDeclaration) SetVisibility(v Visibility) CSSStyleDeclaration {
	style.visibility = v
	return style
}

func (style *cssStyleDeclaration) Float() FloatSide {
	return style.floatSide
}

func (style *cssStyleDeclaration) SetFloat(f FloatSide) CSSStyleDeclaration {
	style.floatSide = f
	return style
}

func (style *cssStyleDeclaration) Clear() ClearSide {
	return style.clearSide
}

func (style *cssStyleDeclaration) SetClear(c ClearSide) CSSStyleDeclaration {
	style.clearSide = c
	return style
}

func (style *cssStyleDeclaration) Opacity() float32 {
	return style.opacity
}

func (style *cssStyleDeclaration) SetOpacity(o float32) CSSStyleDeclaration {
	style.opacity = o
	return style
}

func (style *cssStyleDeclaration) Cursor() Cursor {
	return style.cursor
}

func (style *cssStyleDeclaration) SetCursor(c Cursor) CSSStyleDeclaration {
	style.cursor = c
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

func (style *cssStyleDeclaration) TextDecoration() TextDecoration {
	return style.textDecoration
}

func (style *cssStyleDeclaration) SetTextDecoration(d TextDecoration) CSSStyleDeclaration {
	style.textDecoration = d
	return style
}

func (style *cssStyleDeclaration) OverflowX() Overflow {
	return style.overflowX
}

func (style *cssStyleDeclaration) SetOverflowX(o Overflow) CSSStyleDeclaration {
	style.overflowX = o
	return style
}

func (style *cssStyleDeclaration) OverflowY() Overflow {
	return style.overflowY
}

func (style *cssStyleDeclaration) SetOverflowY(o Overflow) CSSStyleDeclaration {
	style.overflowY = o
	return style
}

func (style *cssStyleDeclaration) WhiteSpace() WhiteSpace {
	return style.whiteSpace
}

func (style *cssStyleDeclaration) SetWhiteSpace(w WhiteSpace) CSSStyleDeclaration {
	style.whiteSpace = w
	return style
}

func (style *cssStyleDeclaration) PointerEvents() PointerEvents {
	return style.pointerEvents
}

func (style *cssStyleDeclaration) SetPointerEvents(p PointerEvents) CSSStyleDeclaration {
	style.pointerEvents = p
	return style
}

func (style *cssStyleDeclaration) BorderCollapse() BorderCollapse {
	return style.borderCollapse
}

func (style *cssStyleDeclaration) SetBorderCollapse(b BorderCollapse) CSSStyleDeclaration {
	style.borderCollapse = b
	return style
}

func (style *cssStyleDeclaration) BorderSpacing() (h, v Size) {
	return style.borderSpacingH, style.borderSpacingV
}

func (style *cssStyleDeclaration) SetBorderSpacing(h, v Size) CSSStyleDeclaration {
	if h != nil {
		style.borderSpacingH = h
	}
	if v != nil {
		style.borderSpacingV = v
	}
	return style
}

func (style *cssStyleDeclaration) EmptyCells() EmptyCells {
	return style.emptyCells
}

func (style *cssStyleDeclaration) SetEmptyCells(e EmptyCells) CSSStyleDeclaration {
	style.emptyCells = e
	return style
}

func (style *cssStyleDeclaration) CaptionSide() CaptionSide {
	return style.captionSide
}

func (style *cssStyleDeclaration) SetCaptionSide(c CaptionSide) CSSStyleDeclaration {
	style.captionSide = c
	return style
}

func (style *cssStyleDeclaration) ListStyleType() ListStyleType {
	return style.listStyleType
}

func (style *cssStyleDeclaration) SetListStyleType(t ListStyleType) CSSStyleDeclaration {
	style.listStyleType = t
	return style
}

func (style *cssStyleDeclaration) ListStylePosition() ListStylePosition {
	return style.listStylePos
}

func (style *cssStyleDeclaration) SetListStylePosition(p ListStylePosition) CSSStyleDeclaration {
	style.listStylePos = p
	return style
}

func (style *cssStyleDeclaration) LineHeight() LineHeight {
	return style.lineHeight
}

func (style *cssStyleDeclaration) SetLineHeight(lh LineHeight) CSSStyleDeclaration {
	style.lineHeight = lh
	return style
}

func (style *cssStyleDeclaration) VerticalAlign() VerticalAlign {
	return style.verticalAlign
}

func (style *cssStyleDeclaration) SetVerticalAlign(v VerticalAlign) CSSStyleDeclaration {
	style.verticalAlign = v
	return style
}

func (style *cssStyleDeclaration) VerticalAlignOffset() Size {
	return style.verticalAlignOff
}

func (style *cssStyleDeclaration) SetVerticalAlignOffset(s Size) CSSStyleDeclaration {
	style.verticalAlignOff = s
	return style
}
