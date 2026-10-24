package engine

import (
	"fmt"
	"strings"
	"testing"
)

// openTextareaLong 开一个 200×100 的 textarea 并填入 nLines 行文本，
// 滚动条几何/拖拽测试都以它为现场。
func openTextareaLong(t *testing.T, nLines int) (HTMLDocument, *htmlElement) {
	t.Helper()
	vp := NewHeadlessViewport(400, 300)
	doc, err := OpenDocument(vp, `<textarea id="t" style="width:200px;height:100px"></textarea>`)
	if err != nil {
		t.Fatal(err)
	}
	b := inner(doc.GetElementById("t"))
	var sb strings.Builder
	for i := 0; i < nLines; i++ {
		fmt.Fprintf(&sb, "line %02d\n", i)
	}
	b.SetAttribute("value", sb.String())
	doc.Refresh()
	return doc, b
}

// 内容溢出才有滚动条；几何贴着内容盒右缘，滑块短于轨道。
func TestTextareaScrollbarGeometry(t *testing.T) {
	g := NewFakeGraphics()
	_, b := openTextareaLong(t, 60)
	tx, ty, tw, th, thumbY, thumbH, ok := textareaScrollbar(g, b)
	if !ok {
		t.Fatal("60 行内容应溢出出滚动条")
	}
	if want := b.x + b.width - textareaScrollbarWidth; tx != want {
		t.Errorf("轨道 x = %d, want %d", tx, want)
	}
	if tw != textareaScrollbarWidth || th != b.height {
		t.Errorf("轨道 = %dx%d, want %dx%d", tw, th, textareaScrollbarWidth, b.height)
	}
	if thumbH >= th {
		t.Errorf("滑块高 = %d, 应短于轨道 %d", thumbH, th)
	}
	if thumbY != ty {
		t.Errorf("未滚动时滑块顶 = %d, want %d（轨道顶）", thumbY, ty)
	}

	// 短内容不溢出：没有滚动条，轨道区域点击照常映射光标。
	_, short := openTextareaLong(t, 2)
	if _, _, _, _, _, _, ok := textareaScrollbar(g, short); ok {
		t.Error("2 行内容不应出滚动条")
	}
}

// 轨道按下即跳转、拖动跟随、抬起结束；全程不动光标、不改滚动焦点。
func TestTextareaScrollbarDrag(t *testing.T) {
	doc, b := openTextareaLong(t, 60)
	g := docGraphics(doc)
	tx, ty, _, th, _, _, ok := textareaScrollbar(g, b)
	if !ok {
		t.Fatal("无滚动条")
	}
	l := layoutTextarea(g, b)
	maxScroll := len(l.lines)*l.lh - b.height

	// 按在轨道下部：消费事件、scrollTop 跳进、光标仍在原位、未进入拖选。
	if !OnDocumentMouseDown(doc, tx+textareaScrollbarWidth/2, ty+th-10) {
		t.Fatal("轨道按下未被消费")
	}
	if b.scrollTop == 0 {
		t.Error("轨道按下后应跳转到点击位置（scrollTop 仍为 0）")
	}
	if b.caret != 0 {
		t.Errorf("滚动条按下不应移动光标：caret = %d", b.caret)
	}
	if d := doc.(*htmlDocument); d.dragInput != nil || d.dragScroll != b {
		t.Errorf("按下态：dragScroll=%v dragInput=%v", d.dragScroll != nil, d.dragInput != nil)
	}

	// 拖到轨道底：到底（maxScroll）；拖回顶部之上：到 0。
	if !OnDocumentMouseDrag(doc, tx+textareaScrollbarWidth/2, ty+th-1) {
		t.Fatal("拖动未被消费")
	}
	if b.scrollTop != maxScroll {
		t.Errorf("拖到底 scrollTop = %d, want %d", b.scrollTop, maxScroll)
	}
	OnDocumentMouseDrag(doc, tx+textareaScrollbarWidth/2, ty-50)
	if b.scrollTop != 0 {
		t.Errorf("拖过顶 scrollTop = %d, want 0", b.scrollTop)
	}
	OnDocumentMouseUp(doc)
	if d := doc.(*htmlDocument); d.dragScroll != nil {
		t.Error("抬起后 dragScroll 应清空")
	}

	// 滑块几何跟着 scrollTop 走：滚到底时滑块贴轨道底。
	b.scrollTop = maxScroll
	tx2, ty2, _, th2, thumbY2, thumbH2, _ := textareaScrollbar(g, b)
	if got := ty2 + th2; thumbY2+thumbH2 != got {
		t.Errorf("到底时滑块底 = %d, want 轨道底 %d（x=%d）", thumbY2+thumbH2, got, tx2)
	}
}

// 回归：点文本区仍走光标映射；点滚动条的 click 不再二次移动光标。
func TestTextareaScrollbarKeepsCaretPath(t *testing.T) {
	doc, b := openTextareaLong(t, 60)
	g := docGraphics(doc)
	tx, ty, _, th, _, _, _ := textareaScrollbar(g, b)

	// 文本区内部点击：光标映射、不碰滚动。
	if !OnDocumentMouseDown(doc, b.x+10, b.y+10) {
		t.Fatal("文本区点击未被消费")
	}
	if d := doc.(*htmlDocument); d.dragInput == nil || d.dragScroll != nil {
		t.Error("文本区按下应进拖选态而非滚动态")
	}
	OnDocumentMouseUp(doc)
	if b.scrollTop != 0 {
		t.Errorf("文本区点击不应滚动：scrollTop = %d", b.scrollTop)
	}

	// 滚动条位置的 click（updateClickFocus 路径）：光标不动。
	caretBefore := b.caret
	if OnDocumentClick(doc, tx+textareaScrollbarWidth/2, ty+th-5) {
		if b.caret != caretBefore {
			t.Errorf("滚动条上的 click 移动了光标：%d → %d", caretBefore, b.caret)
		}
	}
}

// 首次在溢出 textarea 的顶部行点击：光标落在点击行，视口不先跳末尾
// （聚焦会先把光标设到值末尾，点击落位必须按点击前的视口算）。
func TestTextareaFirstClickLandsAtPoint(t *testing.T) {
	doc, b := openTextareaLong(t, 60)
	if !OnDocumentMouseDown(doc, b.x+10, b.y+5) {
		t.Fatal("点击未被消费")
	}
	OnDocumentMouseUp(doc)
	l := layoutTextarea(docGraphics(doc), b)
	if row := caretRow(l.lines, b.caret); row != 0 {
		t.Errorf("点击顶部第 1 行后光标应在第 1 行，实际第 %d 行（caret=%d）", row+1, b.caret)
	}
	if b.scrollTop != 0 {
		t.Errorf("点击顶部行不应把视口跳到底：scrollTop = %d", b.scrollTop)
	}
}
