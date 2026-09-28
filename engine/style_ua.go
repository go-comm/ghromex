package engine

import "sync"

// uaCSS 是内置用户代理样式表（类比浏览器的 UA stylesheet），
// 优先级低于文档样式，由层叠引擎统一应用。
const uaCSS = `
html { display: block; }
head { display: none; }
body { display: block; margin: 0; }
div { display: block; }
p { display: block; margin: 12px 0; }
h1 { display: block; font-size: 28px; font-weight: bold; margin: 10px 0; }
h2 { display: block; font-size: 22px; font-weight: bold; margin: 8px 0; }
h3 { display: block; font-size: 18px; font-weight: bold; margin: 8px 0; }
span { display: inline; }
label { display: inline; }
a { display: inline; color: #2563EB; }
/* 表单控件默认值对齐 Chrome（Windows）UA 样式：
   button: buttonface=#EFEFEF、文字色 buttontext=#000（不继承）、居中文本、
           边框灰 #767676、字号 13px（≈浏览器 13.33px Arial）、padding 1px 6px。
   input : field=#FFF、边框同上、padding 1px 2px、宽度≈size=20 的 147px。
   Chrome 的 border 实为 2px outset/inset，引擎目前只支持平面 solid，用 1px 近似。 */
button { display: inline-block; font-size: 13px; color: #000000; text-align: center; background-color: #EFEFEF; border: 1px solid #767676; border-radius: 2px; padding: 1px 6px; }
/* input 无文字子节点，引擎不会为空元素预留行盒（浏览器靠字体撑出约 21px），
   故显式给 height 让外盒≈20px，与 button 视觉等高。 */
input { display: inline-block; font-size: 13px; color: #000000; background-color: #FFFFFF; border: 1px solid #767676; border-radius: 2px; padding: 1px 2px; width: 147px; height: 16px; }
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
