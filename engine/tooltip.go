package engine

import (
	"strings"
	"time"
)

// title 原生提示气泡。
//
// 鼠标停在带非空 title 的元素（或其子孙）上原地超过 tooltipDelay，引擎就在
// 鼠标右下方画一个小气泡——与浏览器的 title 提示同语义，画在文档顶层
// （select 浮层同层语义），应用一行代码都不用写。
//
// 为什么画在引擎而不是 ui 层做 DOM 浮层：
//   - 引擎没有 z-index，DOM 序的浮层会被其后绝对定位的内容盖住；绘制层
//     天然在最顶、豁免裁剪。
//   - 延时需要按真实时间逐帧推进——引擎已有 TickScroll 先例（后端每帧调
//     一次），气泡挂同一个帧循环，不新增定时器。
//   - 气泡是绘制层不是 DOM 元素，拿不到级联样式；配色由引擎给一套
//     默认值（深底浅字，浅色/深色页面都读得清），应用可用
//     SetTooltipColors 按自己的主题下发（ui.Workbench 会随主题自动配）。
//
// 交互语义（对齐浏览器）：悬停目标或文案变化重新计时；弹出后气泡停在原地
// 不再跟鼠标；按下/点击/实际滚动把气泡收起，直到鼠标换目标才重新计时。
//
// 不变量：可见性与计时只在 tick 与事件阶段推进（TickTooltip / 鼠标事件），
// 布局期不 markChanged；气泡弹出与收起只要重绘、不重排。

// titleAttr 是原生提示的属性名。
const titleAttr = "title"

// tooltipDelay 是鼠标原地停留多久后弹出气泡。
//
// 取 600ms：浏览器实测约 500–1000ms（Chrome ~700ms），取中间值——不迟钝
// 也不神经质。写成 var 不写 const：测试拨小它来推进计时（见 tooltip_test.go）。
var tooltipDelay = 600 * time.Millisecond

// 气泡的字号与盒度量（px）：参照 Windows 提示——小字号、紧 padding、小圆角；
// 偏移量是经典的「鼠标右下挪一档」：不能让光标压住气泡，也不能远得不像一对。
const (
	tooltipFontSize = 13
	tooltipPadX     = 8
	tooltipPadY     = 5
	tooltipRadius   = 3
	tooltipGapX     = 12
	tooltipGapY     = 18
)

// TooltipColors 是提示气泡的配色（浮在文档之上的绘制层，不是 DOM 元素，
// 所以拿不到级联样式，只能由应用显式下发）。空字段按引擎默认。
type TooltipColors struct {
	Background Color // 底色（默认深板岩蓝）
	Border     Color // 1px 描边（默认中间色）
	Text       Color // 文字（默认浅色）
}

// 默认配色：深底、浅字、中间色描边——浅色/深色页面上都能读清。
// 接入主题的应用（ui.Workbench）会用主题令牌覆盖它。
var (
	tooltipBackground = NewColor(0x0F, 0x17, 0x2A, 255)
	tooltipBorder     = NewColor(0x47, 0x55, 0x69, 255)
	tooltipTextColor  = NewColor(0xF1, 0xF5, 0xF9, 255)
)

// SetTooltipColors 设置文档的提示气泡配色，让气泡跟随应用主题而不是引擎
// 默认值。空字段保持默认。
//
// 纯绘制层状态、不触发变更通知：调用方自己保证会有一次重绘（换主题通常
// 伴随样式表追加，重排重绘自然发生）。
func SetTooltipColors(doc HTMLDocument, c TooltipColors) {
	if d, ok := doc.(*htmlDocument); ok {
		d.tooltipColors = c
	}
}

// TooltipColorsOf 返回文档当前的气泡配色（未设置的字段为 nil，表示用引擎
// 默认配色）。
func TooltipColorsOf(doc HTMLDocument) TooltipColors {
	if d, ok := doc.(*htmlDocument); ok {
		return d.tooltipColors
	}
	return TooltipColors{}
}

// TooltipText 返回当前弹出中的气泡文案（未弹出时为空串）。
func TooltipText(doc HTMLDocument) string {
	d, ok := doc.(*htmlDocument)
	if !ok || !d.tooltipShown {
		return ""
	}
	return d.tooltipText
}

// TooltipPending 报告当前悬停是否指向一个还没弹出的气泡（正悬停在带提示的
// 元素上、未到弹出点、也没被按下/滚动收掉）。调用方据此跳过空闲休眠——与
// ScrollAnimating 同理：休眠会让气泡比延时晚一拍才弹出。
//
// 按 hover 实时解析而不是读上一帧登记的状态：鼠标刚进入带提示的元素就该
// 不休眠，登记发生在下一次 TickTooltip，读状态会晚一帧。
func TooltipPending(doc HTMLDocument) bool {
	d, ok := doc.(*htmlDocument)
	if !ok || d.tooltipShown || d.tooltipDismissed {
		return false
	}
	_, text := tooltipFor(d)
	return text != ""
}

// tooltipFor 从悬停元素沿父链上溯，找最近一个带非空 title 的元素，返回它和
// 文案（浏览器语义：子元素继承祖先的 title；全空的 title 等于没有）。
func tooltipFor(d *htmlDocument) (HTMLElement, string) {
	for cur := d.hover; cur != nil; cur = cur.ParentElement() {
		if t := strings.TrimSpace(cur.GetAttribute(titleAttr)); t != "" {
			return cur, t
		}
	}
	return nil, ""
}

// TickTooltip 推进提示气泡的计时，后端每帧调一次（browser 主循环已接上，
// 与 TickScroll 并排）。返回是否需要重绘（气泡弹出或收起——纯绘制层变化，
// 不必重排）。
//
// 可见性变化全部集中在这里：目标或文案变化就重新计时（并把旧气泡收掉）；
// 目标没变但曾被按下/点击/滚动收掉过，就不追回，直到鼠标换目标；弹出后
// 原地不动，等目标变化。
func TickTooltip(doc HTMLDocument) bool {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return false
	}
	target, text := tooltipFor(d)
	redraw := false
	if !sameElement(d.tooltipTarget, target) || d.tooltipText != text {
		if d.tooltipShown {
			d.tooltipShown = false
			redraw = true // 旧气泡要在重绘里擦掉
		}
		d.tooltipTarget, d.tooltipText = target, text
		d.tooltipSinceNs = time.Now().UnixNano()
		d.tooltipDismissed = false // 换了目标：计时重新武装
	}
	if text == "" {
		return redraw
	}
	if d.tooltipShown || d.tooltipDismissed {
		return redraw
	}
	if time.Now().UnixNano()-d.tooltipSinceNs >= int64(tooltipDelay) {
		d.tooltipShown = true
		redraw = true
	}
	return redraw
}

// trackTooltip 在鼠标移动时更新气泡锚点：弹出前跟着鼠标走（弹出瞬间用的
// 就是最新位置）；弹出后不再挪——浏览器气泡出现后就不跟鼠标，等目标变化
// 再收起、重新计时。
func (d *htmlDocument) trackTooltip(x, y int) {
	if !d.tooltipShown {
		d.tooltipMouseX, d.tooltipMouseY = x, y
	}
}

// dismissTooltip 收起气泡并进入「不换目标不追回」状态（按下/点击/实际滚动
// 触发，与浏览器收起提示的行为一致）。
//
// 不主动 markChanged：触发方（按下改 :active、点击改焦点、滚动动内容）这
// 一帧本来就有重绘，气泡随那次重绘消失。
func (d *htmlDocument) dismissTooltip() {
	d.tooltipDismissed = true
	d.tooltipShown = false
}

// dismissTooltip 的文档接口版本（非文档实现时为空操作）。
func dismissTooltip(doc HTMLDocument) {
	if d, ok := doc.(*htmlDocument); ok {
		d.dismissTooltip()
	}
}

// renderTooltip 绘制气泡：圆角底 + 1px 描边 + 文字。在 RenderNode 尾部
// （select 浮层之后）调用：最顶层、豁免裁剪；贴住窗口边缘时向内夹取，
// 下方放不下就翻到鼠标上方。配色取文档级设置，空字段落回引擎默认。
func renderTooltip(g Graphics, d *htmlDocument, vpW, vpH int) {
	bg, bc, tc := d.tooltipColors.Background, d.tooltipColors.Border, d.tooltipColors.Text
	if bg == nil {
		bg = tooltipBackground
	}
	if bc == nil {
		bc = tooltipBorder
	}
	if tc == nil {
		tc = tooltipTextColor
	}
	text := d.tooltipText
	tw, th := g.MeasureText(text, tooltipFontSize, false, "")
	bw, bh := tw+2*tooltipPadX, th+2*tooltipPadY
	x, y := d.tooltipMouseX+tooltipGapX, d.tooltipMouseY+tooltipGapY
	if vpH > 0 && y+bh > vpH {
		y = d.tooltipMouseY - tooltipGapY - bh // 下方放不下：翻到鼠标上方
	}
	if vpW > 0 && x+bw > vpW {
		x = vpW - bw
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	fillRoundedRect(g, x, y, bw, bh, tooltipRadius, bg)
	strokeRoundedRect(g, x, y, bw, bh, tooltipRadius, edges{1, 1, 1, 1}, bc)
	p := NewPaint()
	p.SetSize(NewSize(SIZE_PIXEL, tooltipFontSize, 0))
	p.SetColor(tc)
	// 垫气泡底色给字体合成用：LCD 子像素渲染必须知道落点底色才能合成出
	// 正确的过渡色（与 select 浮层高亮行的 seed Paint 同理）。
	p.SetBackground(bg)
	g.DrawText(x+tooltipPadX, y+tooltipPadY, tw, th, p, text)
}
