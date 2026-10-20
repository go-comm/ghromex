package components

import "github.com/go-comm/ghromex/engine"

func CreateInput(document engine.HTMLDocument, props interface{}) engine.HTMLElement {
	node := document.CreateElement("input")
	node.SetAttribute("type", "text")
	// 与 CreateButton 同口径：挂上默认类，样式由 InjectDefaultClasses 注入。
	node.SetAttribute("class", "input")
	return node
}
