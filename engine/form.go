package engine

import "strings"

// 表单控件通用的取值与运行环境辅助。

// GetValue 返回控件当前值：
//   - select：选中项的值（渲染同口径）；
//   - input/textarea：value 属性；
//   - 其他元素：value 属性（无则空串）。
func GetValue(el HTMLElement) string {
	if el == nil {
		return ""
	}
	if tagIs(el, "select") {
		return SelectValue(el)
	}
	return el.GetAttribute("value")
}

// SetValue 设置控件当前值并触发重排重绘：
//   - select：按值选中对应 option（未匹配则忽略）；
//   - input/textarea：写 value 属性，光标移到值末尾。
func SetValue(el HTMLElement, value string) {
	if el == nil {
		return
	}
	if tagIs(el, "select") {
		SetSelectValue(el, value)
		return
	}
	b := inner(el)
	if b == nil {
		return
	}
	b.SetAttribute("value", value)
	b.caret = len([]rune(value))
	if strings.EqualFold(b.tagName, "textarea") {
		b.scrollTop = 0
	}
}

// docGraphics 取文档视口的 Graphics（用于按当前字体度量做光标映射）；
// 无视口时退回估算口径，保证无头环境行为一致。
func docGraphics(doc HTMLDocument) Graphics {
	if doc != nil {
		if v := doc.Viewport(); v != nil {
			if g := v.Graphics(); g != nil {
				return g
			}
		}
	}
	return NewFakeGraphics()
}

// fontOf 返回样式中的字号/粗体/字族（表单控件 UA 口径缺省 13px）。
func fontOf(comp CSSStyleDeclaration) (fontPx int, bold bool, family string) {
	fontPx = 13
	if comp == nil {
		return
	}
	if s := resolveLen(comp.FontSize(), 0); s > 0 {
		fontPx = s
	}
	bold = comp.FontWeight() == FontWeightBold
	family = comp.FontFamily()
	return
}
