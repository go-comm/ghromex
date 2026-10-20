package engine

import "testing"

// TestNoGapAfterLeadingSpaceInlineLabel 行内标签里「控件 + 空格 + 文字」的
// 空格只能占控件与首字之间的位置。此前待定空格在 addItem 内部才冲刷，而
// 字符的 x 已按冲刷前的游标算好 → 首字之后凭空多出一段空隙（"不接收" 渲染成
// "不 接收"）。此处锁定 run 之间首尾相接。
func TestNoGapAfterLeadingSpaceInlineLabel(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<p><label><input type="radio" name="g"> 不接收</label></p>
	</body></html>`
	buf := NewBufferGraphics(300, 200)
	vp := &HeadlessViewport{W: 300, H: 200, G: buf}
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)

	var runs []textRun
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		b := inner(e)
		if b == nil {
			return
		}
		runs = append(runs, b.runs...)
		for _, c := range b.children {
			walk(c)
		}
	}
	walk(doc)

	// 找到空格 run 与其后的首个文字 run：两者必须首尾相接
	spaceIdx := -1
	for i, r := range runs {
		if r.text == " " {
			spaceIdx = i
			break
		}
	}
	if spaceIdx < 0 {
		t.Fatalf("未找到空格 run：runs=%v", runs)
	}
	if spaceIdx+1 >= len(runs) {
		t.Fatalf("空格 run 之后没有文字 run：runs=%v", runs)
	}
	sp, next := runs[spaceIdx], runs[spaceIdx+1]
	if next.x != sp.x+sp.w {
		t.Errorf("空格与后一个文字 run 之间有空隙：空格 x=%d w=%d，下一个 x=%d（应 = %d）",
			sp.x, sp.w, next.x, sp.x+sp.w)
	}
}
