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
button { display: inline-block; border: 1px solid #C8C8C8; border-radius: 4px; padding: 8px 16px; background-color: #F5F5F5; }
input { display: inline-block; border: 1px solid #333333; border-radius: 4px; padding: 6px 8px; background-color: #FFFFFF; width: 160px; }
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
