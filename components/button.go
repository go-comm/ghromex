package components

import "github.com/go-comm/ghromex/engine"

func CreateButton(document engine.HTMLDocument, props interface{}) engine.HTMLElement {
	node := document.CreateElement("button")
	node.SetAttribute("class", "btn")
	return node
}

func CreateButtonGroup(document engine.HTMLDocument, props interface{}) engine.HTMLElement {
	node := document.CreateElement("div")
	node.SetAttribute("class", "btn-group")
	return node
}
