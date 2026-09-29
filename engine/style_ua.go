package engine

import (
	"strings"
	"sync"
)

// uaCSS 是内置用户代理样式表（类比浏览器的 UA stylesheet），
// 优先级低于文档样式，由层叠引擎统一应用。
const uaCSS = `
html { display: block; }
head { display: none; }
body { display: block; margin: 0; }
div { display: block; }
form { display: block; margin: 0; } /* Chrome UA: form 为块级 */
p { display: block; margin: 16px 0; } /* Chrome UA: margin 1em，16px 基 */
h1 { display: block; font-size: 32px; font-weight: bold; margin: 21px 0; } /* 2em；margin 0.67em */
h2 { display: block; font-size: 24px; font-weight: bold; margin: 20px 0; } /* 1.5em；margin 0.83em */
h3 { display: block; font-size: 18px; font-weight: bold; margin: 19px 0; } /* 1.17em=18.72px；margin 1em */
span { display: inline; }
label { display: inline; }
strong { font-weight: bold; } /* Chrome UA: bolder(700) */
b { font-weight: bold; }
a { display: inline; color: #2563EB; text-decoration: underline; }
/* 表单控件默认值对齐 Chrome（Windows）UA 样式：
   button: buttonface=#EFEFEF、文字色 buttontext=#000（不继承）、居中文本、
           边框灰 #767676、字号 13px（≈浏览器 13.33px Arial）。
           垂直 padding 3px：Chrome 实测外盒高 25（Segoe 13.33 行高所致），
           引擎字形高 16 → 24，1px 差异属字体行高度量（见 README 已知限制）。
   input : field=#FFF、边框同上、padding 1px 2px、宽高 177x21（border-box
           外盒值，Chrome 实测 size=20 默认）。
   Chrome 的 border 实为 2px outset/inset，引擎目前只支持平面 solid，用 1px 近似。
   box-sizing:border-box 与 Chrome UA 样式表对表单控件的默认一致：
   width:100% 声明的控件不再叠加 padding/border 溢出容器。 */
button { display: inline-block; box-sizing: border-box; font-family: system-ui; font-size: 13px; color: #000000; text-align: center; background-color: #EFEFEF; border: 1px solid #767676; border-radius: 2px; padding: 3px 6px; }
/* input 无文字子节点，引擎不会为空元素预留行盒（浏览器靠字体撑出约 21px），
   故显式给 height 让外盒=21px。type 间的尺寸/外观差异见 applyInputTypeDefaults
   （选择器不支持属性选择器，无法用 input[type=...] 表达）。 */
input { display: inline-block; box-sizing: border-box; font-family: system-ui; font-size: 13px; color: #000000; background-color: #FFFFFF; border: 1px solid #767676; border-radius: 2px; padding: 1px 2px; width: 177px; height: 21px; }
/* select/textarea 同 input 口径：border-box + 13px system-ui + 177px 宽。
   select 高 21px（外盒），展开的选项浮层由 layoutSelectPopups 单独定位，
   option 为 block 以免进入常规流参与行盒排布；textarea 两行高。 */
select { display: inline-block; box-sizing: border-box; font-family: system-ui; font-size: 13px; color: #000000; background-color: #FFFFFF; border: 1px solid #767676; border-radius: 2px; padding: 1px 2px; width: 177px; height: 21px; }
option { display: block; box-sizing: border-box; font-family: system-ui; font-size: 13px; color: #000000; padding: 3px 6px; }
textarea { display: inline-block; box-sizing: border-box; font-family: system-ui; font-size: 13px; color: #000000; background-color: #FFFFFF; border: 1px solid #767676; border-radius: 2px; padding: 2px 4px; width: 177px; height: 44px; }
`

var (
	uaOnce  sync.Once
	uaSheet *Stylesheet
)

func userAgentStylesheet() *Stylesheet {
	uaOnce.Do(func() {
		uaSheet = ParseCSS(uaCSS)
	})
	return uaSheet
}

// applyInputTypeDefaults 按 input 的 type 校正 UA 默认尺寸与外观。
// 引擎选择器不支持属性选择器，input[type=...] 的 UA 差异在此以代码级补齐；
// 调用时机在 UA 规则之后、作者规则之前——作者 CSS 仍可覆盖此处全部默认。
// 数值为 Chrome（Windows）实测：radio/checkbox 13x13；
// submit/reset/button 外观同 button（外盒高 25，value 文本居中）。
// v1 边界：checked 选中标记与点击切换交互未实现。
func applyInputTypeDefaults(comp CSSStyleDeclaration, base *htmlElement) {
	if comp == nil || base == nil || !strings.EqualFold(base.tagName, "input") {
		return
	}
	px := func(n int) Size { return NewSize(SIZE_PIXEL, n, 0) }
	zero := NewZeroSize()
	switch typ := strings.ToLower(base.GetAttribute("type")); typ {
	case "radio", "checkbox":
		comp.SetWidth(px(13))
		comp.SetHeight(px(13))
		comp.SetPadding(NewZeroRect())
		// 行内基线视觉对齐：无 vertical-align 支持，用 3px 顶距近似
		comp.SetMargin(NewRect(zero, zero, px(3), zero))
		if typ == "radio" {
			comp.SetBorderRadius(px(7)) // ≥ 半宽 → 正圆
		} else {
			comp.SetBorderRadius(px(2))
		}
	case "submit", "reset", "button":
		comp.SetWidth(NewAutoSize())                             // 宽随 value 文本（layoutBox 特测）
		comp.SetHeight(px(25))                                   // 外盒高对齐 Chrome button
		comp.SetBackgroundColor(NewColor(0xEF, 0xEF, 0xEF, 255)) // buttonface
		comp.SetPadding(NewRect(px(6), px(6), px(1), px(1)))
	}
}
