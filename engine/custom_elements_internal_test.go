package engine

import "testing"

// TestDefineCustomElementRejectsTypedNil 原型是「非 nil 接口 + nil 指针」
// （typed nil）时必须注册失败：此前注册成功，随后 CreateElement → Clone 会
// 调到提升自 nil 指针的方法 → 空指针崩溃。
func TestDefineCustomElementRejectsTypedNil(t *testing.T) {
	var proto *htmlElement
	if err := DefineCustomElement("x-typed-nil", proto); err == nil {
		t.Fatal("typed nil 原型应注册失败（后续 Clone 会空指针崩溃）")
	}
	// 即便注册表里被塞进 typed nil（外部 API 可直连注册表），CreateElement
	// 也不得崩溃——退化为未知元素。
	CustomElements().Define("x-typed-nil-forced", proto)
	vp := NewHeadlessViewport(200, 100)
	doc, err := OpenDocument(vp, "<html><body>x</body></html>")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	if el := doc.CreateElement("x-typed-nil-forced"); el == nil {
		t.Fatal("CreateElement 应返回未知元素而非 nil")
	}
}
