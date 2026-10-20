package engine_test

import (
	"testing"
)

// 引擎 UA 样式表须与 Chrome（Windows）侧对齐的常用规则：
// 行内语义标签不另起一行、列表缩进 40px、定义列表 dd 缩进 40px。
func TestUATagInlineAndList(t *testing.T) {
	// 行内语义标签：em 与前后文字同一行
	doc := openDoc(t, 300, 200, `<!doctype html><html><head></head><body style="margin:0">
		<p id="p">前<em id="em">中</em>后</p>
	</body></html>`)
	p := mustEl(t, doc, "#p")
	em := mustEl(t, doc, "#em")
	if em.Y() != p.Y() {
		t.Errorf("em 另起一行（Y=%d vs 段落 Y=%d）：行内语义标签应为 inline", em.Y(), p.Y())
	}

	// 无序列表：li 缩进 40px（Chrome 的 ul padding-left）
	doc2 := openDoc(t, 300, 200, `<!doctype html><html><head></head><body style="margin:0">
		<ul id="u"><li id="li">项</li></ul>
	</body></html>`)
	li := mustEl(t, doc2, "#li")
	if x := li.X(); x != 40 {
		t.Errorf("li 左缘 = %d, want 40（ul 的 padding-left，与 Chrome 一致）", x)
	}

	// 定义列表：dd 缩进 40px（Chrome 的 dd margin-left）
	doc3 := openDoc(t, 300, 200, `<!doctype html><html><head></head><body style="margin:0">
		<dl id="d"><dt id="dt">术语</dt><dd id="dd">说明</dd></dl>
	</body></html>`)
	dd := mustEl(t, doc3, "#dd")
	if x := dd.X(); x != 40 {
		t.Errorf("dd 左缘 = %d, want 40（dd 的 margin-left，与 Chrome 一致）", x)
	}
}
