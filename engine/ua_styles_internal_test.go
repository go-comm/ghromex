package engine

import "testing"

// TestUAStylesForCommonTags UA 样式表里的常用标签规则必须真正落到计算样式上
// （dt 加粗、dd 缩进、ul 缩进、行内语义标签 inline）——规则写了但没生效，
// 对照页上会与浏览器差一截。
func TestUAStylesForCommonTags(t *testing.T) {
	vp := NewHeadlessViewport(300, 200)
	doc, err := OpenDocument(vp, `<!doctype html><html><head></head><body>
		<dl><dt id="dt">术语</dt><dd id="dd">说明</dd></dl>
		<ul id="u"><li>项</li></ul>
		<p>前<em id="em">中</em>后</p>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	styleOf := func(id string) CSSStyleDeclaration {
		el := doc.QuerySelector(id)
		if el == nil {
			t.Fatalf("no %s", id)
		}
		b := inner(el)
		if b == nil || b.computed == nil {
			t.Fatalf("%s 无计算样式", id)
		}
		return b.computed
	}
	if fw := styleOf("#dt").FontWeight(); fw != FontWeightBold {
		t.Errorf("dt 字体权重 = %v, want bold（Chrome UA: dt 加粗）", fw)
	}
	if ml := resolveEdgeRect(styleOf("#dd").Margin(), 300)[eLeft]; ml != 40 {
		t.Errorf("dd 左外边距 = %d, want 40（Chrome UA: dd margin-left 40px）", ml)
	}
	if pl := resolveEdgeRect(styleOf("#u").Padding(), 300)[eLeft]; pl != 40 {
		t.Errorf("ul 左内边距 = %d, want 40（Chrome UA: ul padding-left 40px）", pl)
	}
	if d := styleOf("#em").Display(); d != DisplayInline {
		t.Errorf("em display = %v, want inline（行内语义标签）", d)
	}
}
