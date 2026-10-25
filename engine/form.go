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
	if strings.EqualFold(b.tagName, "textarea") {
		// 值里的换行统一成 '\n'：Windows 文本文件/剪贴板是 CRLF，'\r'
		// 留着会被 textarea 当字形画出来（每行末尾一个方块），光标与
		// 折行区间的 rune 下标也会和显示对不上。
		value = normalizeNewlines(value)
	}
	b.SetAttribute("value", value)
	b.caret = len([]rune(value))
	b.selAnchor = b.caret // 程序化设值后无选区，光标在末尾
	if strings.EqualFold(b.tagName, "textarea") {
		// 光标落在值末尾：滚动到光标行（scrollTop 负值 = 哨兵，由布局期
		// 解析成实际偏移）——否则长文本设值后光标与末尾内容都在可视区外。
		b.scrollTop = -1
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
