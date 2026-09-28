package engine_test

import (
	"fmt"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

func TestDocumentCreateElement(t *testing.T) {

	document := engine.NewDocument()

	div := document.CreateElement("div")

	label := document.CreateElement("label")
	label.SetText("User:")
	label.Style().SetWidth(engine.NewSize(engine.SIZE_PIXEL, 100, 0))
	label.Style().SetHeight(engine.NewSize(engine.SIZE_PIXEL, 40, 0))

	div.AppendChild(label)

	input := document.CreateElement("input")
	div.AppendChild(input)

	button := document.CreateElement("button")
	div.AppendChild(button)

	document.Body().AppendChild(div)

	fmt.Println(document.FormatJSON("  ", true))

	// 注册表克隆语义：两次 CreateElement 必须返回独立实例
	a := document.CreateElement("div")
	b := document.CreateElement("div")
	if a == b {
		t.Fatal("CreateElement must return a cloned instance")
	}
	a.SetText("x")
	if b.InnerText() != "" {
		t.Fatal("siblings must not share state")
	}
}
