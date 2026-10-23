// 本文件用 ui 控件拼装记事本的页面：查找替换栏 + 编辑区。
//
// 装配只用控件视图（NewBar/NewTextInput/NewButton/NewTextArea），页面里不再
// 出现手搭 DOM、id 往返查询和逐个控件的绝对坐标：类名、取值/设值、可用态、
// 点击绑定都由 ui 负责，条内控件的排布由主题的 .system-ui-bar（inline-block）
// 负责。
//
// 页面 CSS 只留主题给不出的那一条：body 的边距归零。颜色一律来自主题令牌，
// 所以切换浅色/深色时「纸张」和查找栏会一起变，不会留一块浅色底在深色界面上。
//
// 仍然由应用算的只有几何：引擎的 CSS 没有 calc，「视口高度 - 菜单栏 - 工具栏
// - 状态栏」这类尺寸只能写内联样式（见 main.go 的 syncLayout），栏高则从
// ui.Theme.BarHeight 读，和主题里的控件条一致。
package main

import (
	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/ui"
)

// pageCSS 是记事本自己的页面样式；外壳与控件样式由 ui 主题注入。
func pageCSS(th ui.Theme) string {
	return `
body { margin: 0; padding: 0; height: 100%; color: ` + th.Text + `;
	font-family: ` + th.Font + `; font-size: 13px; }
`
}

// findBarActions 是查找替换栏上四个按钮的动作，在装配时交给按钮。
type findBarActions struct {
	findNext   func()
	replace    func()
	replaceAll func()
	hide       func()
}

// page 是记事本页面上的控件集合。
//
// 持有的是控件视图而不是裸元素：读写值用 Value/SetValue，几何、焦点这类
// 需要元素的场合再取 Element()。
type page struct {
	findBar    *ui.BarView       // 查找替换栏（横向控件条）
	findIn     *ui.TextInputView // 查找内容
	findNext   *ui.ButtonView    // 查找下一个
	replaceIn  *ui.TextInputView // 替换内容
	replace    *ui.ButtonView    // 替换
	replaceAll *ui.ButtonView    // 全部替换
	findClose  *ui.ButtonView    // 关闭查找栏
	editor     *ui.TextAreaView  // 编辑区（纸面）
}

// buildPage 在空文档上搭出记事本页面。每个控件都留了 id：测试和
// GetElementById 查询用，程序内部一律直接持有视图，不再 QuerySelector 往返。
func buildPage(doc engine.HTMLDocument, th ui.Theme, acts findBarActions) *page {
	doc.Head().Title().SetText(appName)
	doc.AddStylesheetText(pageCSS(th))

	p := &page{
		findIn:    ui.NewTextInput(doc, ""),
		replaceIn: ui.NewTextInput(doc, ""),
		editor:    ui.NewTextArea(doc, ""),
	}

	// 「查找下一个」是这条栏的主操作，给它强调色；其余是中性按钮。
	p.findNext = ui.NewButton(doc, "查找下一个", acts.findNext).SetVariant("primary")
	p.replace = ui.NewButton(doc, "替换", acts.replace)
	p.replaceAll = ui.NewButton(doc, "全部替换", acts.replaceAll)
	p.findClose = ui.NewButton(doc, "关闭", acts.hide)

	p.findBar = ui.NewBar(doc,
		p.findIn, p.findNext, p.replaceIn, p.replace, p.replaceAll, p.findClose)

	setControlID(p.findBar, "findbar")
	setControlID(p.findIn, "find-text")
	setControlID(p.findNext, "btn-find-next")
	setControlID(p.replaceIn, "replace-text")
	setControlID(p.replace, "btn-replace")
	setControlID(p.replaceAll, "btn-replace-all")
	setControlID(p.findClose, "btn-find-close")
	setControlID(p.editor, "editor")

	// 编辑区是铺满内容区的纸：去掉描边与圆角，字号/字体族由 syncLayout 写。
	p.editor.SetVariant("plain")
	p.editor.Element().SetAttribute("spellcheck", "false")

	doc.Body().AppendChild(p.findBar.Element(), p.editor.Element())
	return p
}

// setControlID 给控件的底层元素设 id；控件为 nil 时跳过（按钮动作可能缺省）。
func setControlID(c ui.Control, id string) {
	if c == nil {
		return
	}
	if el := c.Element(); el != nil {
		el.SetID(id)
	}
}
