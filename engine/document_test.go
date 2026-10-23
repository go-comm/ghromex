package engine_test

import (
	"fmt"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// RemoveAttribute 要真正删除属性并清掉 id/style 的缓存。
func TestRemoveAttribute(t *testing.T) {
	doc := engine.NewDocument()
	el := doc.CreateElement("button")
	el.SetAttribute("id", "ok")
	el.SetAttribute("style", "width:10px")
	el.SetAttribute("disabled", "")
	el.RemoveAttribute("id")
	el.RemoveAttribute("style")
	el.RemoveAttribute("disabled")

	if got := el.ID(); got != "" {
		t.Fatalf("删除 id 后 ID() = %q, want 空", got)
	}
	if got := el.GetAttribute("style"); got != "" {
		t.Fatalf("删除 style 后 = %q, want 空", got)
	}
	if got := el.GetAttribute("disabled"); got != "" {
		t.Fatalf("删除 disabled 后 = %q, want 空", got)
	}
}

// 用 :disabled 的布局结果验证属性真的从 attrs 里删掉了；只把值改成
// 空串仍然算「属性存在」，布局不会恢复。
func TestRemoveAttributeRestoresDisabledLayout(t *testing.T) {
	vp := engine.NewHeadlessViewport(200, 100)
	doc, err := engine.OpenDocument(vp, `<style>
		button { width: 40px; height: 20px; }
		button:disabled { width: 99px; }
	</style><button id="b">x</button>`)
	if err != nil {
		t.Fatal(err)
	}
	btn := doc.GetElementById("b")
	if btn == nil {
		t.Fatal("缺少 #b")
	}
	btn.SetAttribute("disabled", "disabled")
	doc.Refresh()
	width := func() int {
		r := btn.GetBoundingClientRect()
		return r.Right().Pixel() - r.Left().Pixel()
	}
	if got := width(); got != 99 {
		t.Fatalf("禁用时宽度 = %d, want 99", got)
	}
	btn.RemoveAttribute("disabled")
	doc.Refresh()
	if got := width(); got != 40 {
		t.Fatalf("启用后宽度 = %d, want 40", got)
	}
}

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
