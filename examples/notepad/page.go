// 本文件用 Go 构造记事本的页面 DOM，不再依赖 index.html。
//
// 引擎的文档模型本身就是 DOM：CreateElement 建节点、AppendChild 装树、
// AddStylesheetText 追加样式。页面结构很小，放在 Go 里比维护一份 HTML
// 更直接，也不会出现「源码改了但二进制里还是旧页面」的问题。
package main

import "github.com/go-comm/ghromex/engine"

// notepadCSS 是记事本自己的页面样式；外壳样式仍由 ui.Install 注入。
const notepadCSS = `
/* ui 主题会追加 html 的底色；记事本要白色纸面，这里用 important 盖掉。 */
html.system-ui { height: 100%; background-color: #FFFFFF !important; }
body { margin: 0; padding: 0; height: 100%; color: #1F1F1F;
	font-family: "Microsoft YaHei", "Segoe UI", system-ui, sans-serif; font-size: 13px; }

/* ── 查找替换栏（默认隐藏，显示时挤在工具栏下面） ───────────── */
#findbar { position: absolute; left: 0; width: 100%; height: 32px;
	display: none; box-sizing: border-box; background-color: #F4F8FE;
	border-bottom: 1px solid #D8E3F5; }
#findbar input { position: absolute; top: 6px; height: 20px; width: 170px;
	box-sizing: border-box; padding: 0 6px; font-size: 12px; }
#findbar button { position: absolute; top: 5px; height: 22px; padding: 0 10px;
	font-size: 12px; }
#find-text { left: 12px; }
#btn-find-next { left: 190px; }
#replace-text { left: 296px; }
#btn-replace { left: 474px; }
#btn-replace-all { left: 546px; }
#btn-find-close { left: 634px; }

/* ── 编辑区（几何由示例程序按窗口尺寸写入内联样式） ─────────── */
#editor { position: absolute; left: 0; width: 100%; height: 100%;
	box-sizing: border-box; border: none; padding: 6px 8px; background-color: #FFFFFF;
	color: #1F1F1F; font-family: Consolas, "Courier New", monospace; font-size: 14px; }
`

// buildPage 在空文档上搭出记事本页面：查找栏 + 编辑区。
//
// 元素 id 与 actions.go / main.go 里的查询保持一致；页面外观由
// notepadCSS 负责，编辑区几何稍后由 syncLayout 按视口尺寸写内联样式。
func buildPage(doc engine.HTMLDocument) {
	doc.Head().Title().SetText(appName)
	doc.AddStylesheetText(notepadCSS)

	body := doc.Body()

	findBar := pageElement(doc, "div")
	findBar.SetID("findbar")

	findIn := pageElement(doc, "input")
	findIn.SetID("find-text")
	findIn.SetAttribute("type", "text")

	findNext := pageElement(doc, "button")
	findNext.SetID("btn-find-next")
	findNext.SetAttribute("type", "button")
	findNext.SetText("查找下一个")

	replaceIn := pageElement(doc, "input")
	replaceIn.SetID("replace-text")
	replaceIn.SetAttribute("type", "text")

	replace := pageElement(doc, "button")
	replace.SetID("btn-replace")
	replace.SetAttribute("type", "button")
	replace.SetText("替换")

	replaceAll := pageElement(doc, "button")
	replaceAll.SetID("btn-replace-all")
	replaceAll.SetAttribute("type", "button")
	replaceAll.SetText("全部替换")

	findClose := pageElement(doc, "button")
	findClose.SetID("btn-find-close")
	findClose.SetAttribute("type", "button")
	findClose.SetText("关闭")

	findBar.AppendChild(findIn, findNext, replaceIn, replace, replaceAll, findClose)

	editor := pageElement(doc, "textarea")
	editor.SetID("editor")
	editor.SetAttribute("spellcheck", "false")

	body.AppendChild(findBar, editor)
}

// pageElement 是 CreateElement 的薄包装：标签拼错时尽早暴露，而不是在后面
// 的空指针或布局异常里猜原因。
func pageElement(doc engine.HTMLDocument, tag string) engine.HTMLElement {
	el := doc.CreateElement(tag)
	if el == nil {
		panic("notepad: 无法创建元素 " + tag)
	}
	return el
}
