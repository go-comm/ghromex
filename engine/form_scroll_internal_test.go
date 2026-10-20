package engine

import "testing"

// TestSetValueScrollsCaretIntoView SetValue 把光标放到值末尾时，textarea
// 必须滚动到光标行——此前 scrollTop 被清零，长文本设值后光标与末尾内容
// 都在可视区外。
func TestSetValueScrollsCaretIntoView(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#t { width: 100px; height: 40px; }
	</style></head><body>
	<textarea id="t">x</textarea>
	</body></html>`
	vp := NewHeadlessViewport(200, 200)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	ta := doc.QuerySelector("#t")
	if ta == nil {
		t.Fatal("no #t")
	}
	base := inner(ta)
	if base == nil {
		t.Fatal("no inner element")
	}
	// 长文本：足以撑出多行
	SetValue(ta, "line1\nline2\nline3\nline4\nline5\nline6")
	// 渲染一帧：textarea 的行布局与滚动量在渲染期计算（layoutTextarea），
	// 后端每帧都会走这一步。
	buf := NewBufferGraphics(200, 200)
	RenderNode(buf, doc)
	if base.scrollTop <= 0 {
		t.Errorf("SetValue 后 scrollTop = %d, want > 0（光标在末尾，应滚动到光标行）", base.scrollTop)
	}
}
