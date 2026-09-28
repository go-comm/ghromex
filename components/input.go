package components

import "github.com/go-comm/ghromex/engine"

func CreateInput(document engine.HTMLDocument, props interface{}) engine.HTMLElement {
	node := document.CreateElement("input")
	node.SetAttribute("type", "text")
	return node
}
