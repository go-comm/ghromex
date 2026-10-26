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
/* body 的默认外边距取 Chrome 的 8px（而非 0）：body 外边距属于 UA 默认
   样式的一部分，页面作者未显式声明时浏览器就是 8px——引擎此前按"桌面 UI
   取向"取 0，整页内容比浏览器整体上移 8px，与 Chrome 对照时第一行标题/
   段落位置就不同。 */
body { display: block; margin: 8px; }
div { display: block; }
form { display: block; margin: 0; } /* Chrome UA: form 为块级 */
p { display: block; margin: 16px 0; } /* Chrome UA: margin 1em，16px 基 */
h1 { display: block; font-size: 32px; font-weight: bold; margin: 21px 0; } /* 2em；margin 0.67em */
h2 { display: block; font-size: 24px; font-weight: bold; margin: 20px 0; } /* 1.5em；margin 0.83em */
h3 { display: block; font-size: 19px; font-weight: bold; margin: 19px 0; } /* 1.17em=18.72px，引擎字号取整到 19：Chrome 行盒 25，18px 只有 24 */
 span { display: inline; }
 label { display: inline; }
  /* 其余行内语义标签统一 display:inline（CSS 的初始值就是 inline；未知标签
     默认块级会让 em/i/code/small/sub/sup 等在引擎里各占一行——与浏览器
     明显不符）。斜体/上标下标/小字号的字形差异属 v1 边界（无斜体渲染、
     无 vertical-align），此处只保证行内性。 */
  em, i, u, s, strike, code, kbd, samp, var, cite, dfn, abbr, q, mark, small,
  sub, sup, time, data, output, bdi, bdo, ins, del, ruby, rt, rp { display: inline; }
  /* 列表/定义列表/引用/预格式化：与 Chrome UA 同口径（缩进与外边距）。
     li 必须是块级（Chrome: display:list-item）——否则引擎默认的 inline-block
     会让多个 li 挤在同一行。列表标记（disc/decimal）引擎无 list-item 概念，
     改由布局/绘制层按 ul/ol 自行绘制（measure.go / render.go）。 */
  ul, ol { display: block; margin: 16px 0; padding-left: 40px; }
  /* 标记类型：Chrome UA 的 ul → disc、ol → decimal（CSS 初始值本身是
     disc；ol 必须显式置 decimal，否则作者写的 list-style-type 不生效时
     ol 也会退化成圆点）。list-style-type 继承给 li，作者写在 ul/ol 上
     即可覆盖。 */
  ul { list-style-type: disc; }
  ol { list-style-type: decimal; }
  /* 嵌套列表的外边距为 0（Chrome UA 实测：内层 ul 的 margin 计算值为 0，
     不是外层的 1em）——否则内层列表会额外多出 16px 上间距，嵌套列表的
     整块高度比浏览器高 34px。 */
  ul ul, ol ul, ul ol, ol ol { display: block; margin: 0; padding-left: 40px; }
  li { display: block; }
  /* 表格：display 家族按 HTML5 渲染规范的 UA 值落位，布局走 display 驱动的
     网格模型（见 table_layout.go）。border-spacing:2px 是 UA 给 <table>
     元素的默认（CSS 初始值是 0——display:table 的普通元素不带这份 2px，
     Chrome 同款）。单元格不写 UA 的 vertical-align（Chrome 也报 baseline）：
     表格布局把单元格上的 baseline 按 middle 处理（CSS 2.1 §17.5.2.1），
     作者显式 top/middle/bottom 则照办。 */
  table { display: table; border-spacing: 2px; }
  caption { display: table-caption; text-align: center; }
  colgroup { display: table-column-group; }
  col { display: table-column; }
  thead { display: table-header-group; }
  tbody { display: table-row-group; }
  tfoot { display: table-footer-group; }
  tr { display: table-row; }
  /* 单元格的宽度测量语义与旧 inline-block 口径一致：display:table 布局
     先按内容自然宽量列（table-cell 在 layoutBox 里按内容收缩），块级会
     撑满容器、列宽就量不出来了。padding:1px 沿用 HTML5 UA 的单元格默认。 */
  th, td { display: table-cell; padding: 1px; }
  th { text-align: center; font-weight: bold; }
  dl { display: block; margin: 16px 0; }
  /* dt 不加粗：HTML5 渲染规范里 dt 没有 font-weight（HTML4 的 UA 才加粗），
     Chrome 实测计算权重 400、与普通 span 同宽同墨量；写成 bold 会让 dt 行
     比浏览器重一档（对照页墨量约 1.8 倍）。 */
  dt { display: block; }
  dd { display: block; margin-left: 40px; }
  blockquote { display: block; margin: 16px 40px; }
  pre { display: block; font-family: monospace; font-size: 13px; margin: 16px 0; white-space: pre; }
  figure { display: block; margin: 16px 40px; }
  figcaption { display: block; }
  /* 块级语义容器（HTML5 渲染规范）：引擎对未收录标签按 CSS 初始值 inline
     处理——这些容器标签不落位 block 会与兄弟挤进行内原子布局，游标顶到右缘
     后按 remain=0 试排（换行重排），内部 display:table 的天然宽测量随即塌陷
     （列宽等分、行高虚胖）。address 同时补斜体（上文注释早已声明 Chrome 把
     address 置 italic，规则此前缺失）。h4~h6 的字号/外边距按 Chrome 的
     1em/0.83em/0.67em 与 1.33em/1.67em/2.33em（基 16px，引擎字号取整）。 */
  section, article, aside, header, footer, main, nav, hgroup, details, summary,
  fieldset, legend, dialog, center { display: block; }
  address { display: block; font-style: italic; }
  h4 { display: block; font-size: 16px; font-weight: bold; margin: 21px 0; }
  h5 { display: block; font-size: 13px; font-weight: bold; margin: 22px 0; }
  h6 { display: block; font-size: 11px; font-weight: bold; margin: 25px 0; }
  hr { display: block; margin: 8px 0; border: 1px solid #767676; }
  strong { font-weight: bold; } /* Chrome UA: bolder(700) */
b { font-weight: bold; }
/* 斜体语义标签：Chrome UA 把 i/cite/em/var/address/dfn 置为 italic。
   引擎用绘制后端的合成斜体呈现（见 FontStyle），字形前进宽度不变。 */
i, cite, em, var, dfn { font-style: italic; }
a { display: inline; color: #0000EE; text-decoration: underline; cursor: pointer; } /* Chrome 默认链接色 #0000EE */
/* 等宽与变小字号：Chrome UA 实测 code/kbd/samp 的计算字号为 13.33px、
   字体族为 monospace；small 为 smaller（同 13.33px）；pre 的计算字号为
   13px（其 monospace 通用族的默认字号）。引擎字号取整为 13px。 */
code, kbd, samp { font-family: monospace; font-size: 13px; }
small { font-size: 13px; }
/* 上下标：Chrome UA 的 smaller = 13.33px（引擎取整 13），并用
   vertical-align:super/sub 把基线分开（位移量按父级字号算，见
   measure.go 的 verticalAlignShift）。 */
sub, sup { font-size: 13px; }
sub { vertical-align: sub; }
sup { vertical-align: super; }
/* 表单控件默认值对齐 Chrome（Windows）UA 样式：
   button: buttonface=#EFEFEF、文字色 buttontext=#000（不继承）、居中文本、
           边框灰 #767676、字号 13px（≈浏览器 13.33px Arial）。
           字体统一 Arial（引擎默认字体，桌面 UI 取向；Chrome 控件用系统
           UI 字体 Segoe UI，见 README「字体默认语义」）。
           垂直 padding 3px：Chrome 实测外盒高 25（Segoe 13.33 行高所致），
           引擎字形高 16 → 24，1px 差异属字体行高度量（见 README 已知限制）。
   input : field=#FFF、边框同上、padding 1px 2px、宽高 177x21（border-box
           外盒值，Chrome 实测 size=20 默认）。
   Chrome 的 border 实为 2px outset/inset，引擎目前只支持平面 solid，用 1px 近似。
   box-sizing:border-box 与 Chrome UA 样式表对表单控件的默认一致：
   width:100% 声明的控件不再叠加 padding/border 溢出容器。 */
button { display: inline-block; box-sizing: border-box; font-family: Arial, system-ui, sans-serif; font-size: 13px; font-style: normal; color: #000000; text-align: center; background-color: #EFEFEF; border: 1px solid #767676; border-radius: 2px; padding: 3px 6px; }
/* input 无文字子节点，引擎不会为空元素预留行盒（浏览器靠字体撑出约 21px），
   故显式给 height 让外盒=21px。type 间的尺寸/外观差异见 applyInputTypeDefaults
   （选择器不支持属性选择器，无法用 input[type=...] 表达）。 */
input { display: inline-block; box-sizing: border-box; font-family: Arial, system-ui, sans-serif; font-size: 13px; font-style: normal; color: #000000; background-color: #FFFFFF; border: 1px solid #767676; border-radius: 2px; padding: 1px 2px; width: 177px; height: 21px; cursor: text; }
/* select/textarea 同 input 口径：border-box + 13px Arial + 177px 宽。
   select 高 21px（外盒），展开的选项浮层由 layoutSelectPopups 单独定位，
   option 为 block 以免进入常规流参与行盒排布；textarea 两行高。 */
 select { display: inline-block; box-sizing: border-box; font-family: Arial, system-ui, sans-serif; font-size: 13px; font-style: normal; color: #000000; background-color: #FFFFFF; border: 1px solid #767676; border-radius: 2px; padding: 1px 2px; height: 19px; }
option { display: block; box-sizing: border-box; font-family: Arial, system-ui, sans-serif; font-size: 13px; font-style: normal; color: #000000; padding: 3px 6px; }
 textarea { display: inline-block; box-sizing: border-box; font-family: Arial, system-ui, sans-serif; font-size: 13px; font-style: normal; color: #000000; background-color: #FFFFFF; border: 1px solid #767676; border-radius: 2px; padding: 2px 4px; width: 161px; height: 36px; overflow: hidden; cursor: text; }
/* textarea 的 overflow 走 hidden 而非浏览器的 auto：折行内容的滚动由
   textarea 自身 scrollTop 机制驱动（光标跟随 + 滚轮，见 scroll.go），
   通用 overflow 滚动对原子控件不生效（scrollMax 对原子控件恒 0），
   hidden 已提供绘制所需的裁剪盒——半截行由此被截断而非整行丢弃。 */
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
// checked 选中标记与点击切换/分组互斥见 check.go（radio 盒半径在渲染期
// 按正圆修正，见 renderNode）。
func applyInputTypeDefaults(comp CSSStyleDeclaration, base *htmlElement) {
	if comp == nil || base == nil || !strings.EqualFold(base.tagName, "input") {
		return
	}
	px := func(n int) Size { return NewSize(SIZE_PIXEL, n, 0) }
	switch typ := strings.ToLower(base.GetAttribute("type")); typ {
	case "radio", "checkbox":
		comp.SetWidth(px(13))
		comp.SetHeight(px(13))
		comp.SetPadding(NewZeroRect())
		// Chrome UA 的单选/复选外边距实测值：
		//   radio    : 3px 3px 0px 5px（上3 右3 下0 左5）
		//   checkbox : 3px 3px 3px 4px（上3 右3 下3 左4）
		// 左边距是原生控件的固有留白，缺它控件会比 Chrome 左移 4~5px。
		if typ == "radio" {
			comp.SetMargin(NewRect(px(5), px(3), px(3), px(0)))
		} else {
			comp.SetMargin(NewRect(px(4), px(3), px(3), px(3)))
		}
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

// UA 默认的交互反馈配色：悬停/按下时的底色与边框色。
var (
	uaHoverFace    = NewColor(0xE3, 0xE3, 0xE3, 255) // 悬停底色（比 buttonface 深一档）
	uaActiveFace   = NewColor(0xD4, 0xD4, 0xD4, 255) // 按下底色
	uaHoverBorder  = NewColor(0x6A, 0x6A, 0x6A, 255) // 悬停边框（比 #767676 深）
	uaActiveBorder = NewColor(0x4F, 0x4F, 0x4F, 255) // 按下边框
)

// applyInteractionDefaults 给交互控件补上 UA 默认的悬停/按下反馈。
//
// Chrome 的 UA 样式表**没有** hover 规则（按钮悬停不变色），但 Ghromex
// 定位是桌面 UI 框架：按钮与输入框悬停毫无反馈会显得「死了」。这里给一套
// 克制的默认反馈——按钮型控件（<button> 与 <input type=submit/reset/button>
// 同款）底色与边框一起加深，文本类控件只加深边框。
//
// 为什么放代码里而不是 UA 样式表：
//   - 按钮型外观按 type 分支，而属性选择器对 type 的取值是大小写敏感的
//     （<input type="SUBMIT"> 匹配不到 [type=submit]），代码分支按小写
//     比较更稳；
//   - 「哪些控件底色随悬停变、哪些只变边框」这类类型差异在代码里一处说
//     完，不必在样式表里堆叠选择器。
//
// 调用时机在 UA 规则之后、作者规则之前：作者 CSS 仍可覆盖（作者来源优先
// 于 UA 来源，与 CSS 的级联顺序一致）。
func applyInteractionDefaults(comp CSSStyleDeclaration, base *htmlElement, doc HTMLDocument) {
	if comp == nil || base == nil {
		return
	}
	hovered := matchHoverState(doc, base)
	active := matchActiveState(doc, base)
	if !hovered && !active {
		return
	}
	face, border := uaHoverFace, uaHoverBorder
	if active {
		face, border = uaActiveFace, uaActiveBorder
	}
	// buttonFace：底色随交互态变（按钮型控件）；其余控件只变边框。
	buttonFace := false
	switch strings.ToLower(base.tagName) {
	case "button":
		buttonFace = true
	case "input":
		switch strings.ToLower(base.GetAttribute("type")) {
		case "submit", "reset", "button":
			buttonFace = true
		case "radio", "checkbox":
			return // 勾选控件不参与悬停反馈（Chrome 也不变）
		}
	case "select", "textarea":
		// 只变边框
	default:
		return
	}
	if buttonFace {
		comp.SetBackgroundColor(face)
	}
	comp.SetBorderColor(border)
}
