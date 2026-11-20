package engine

import (
	"fmt"
	"strings"
	"testing"
)

// textarea 折行缓存的回归测试。
//
// # 为什么需要它
//
// layoutTextarea 一帧内会被上溯约 8 次（绘制、选区矩形、光标↔坐标映射、
// 滚动条几何、滚动夹紧、SVG 导出…）。旧实现每次都从头贪心折行全文：
// 19.5MB 的值每次折行要做 2000 万次单字宽度测量、分配 80MB 的 []rune
// 临时切片，再把每行文本复制一遍——实测首帧总分配 1.3GB，5 帧 7.8GB。
//
// 现在折行结果缓存在 htmlElement.wrap 上，键是「值 + 可用宽 + 字体 +
// 度量后端」，值变化由 SetAttribute("value") 置 nil 失效。
//
// # 判定手段
//
// 用**文本测量次数**而不是计时（计时不可复现，同 scroll_layout_internal_test.go
// 的口径）：缓存命中时不该再逐字测量，只有行高/基线那两次固定测量。

// wrapMeasureCounter 统计文本测量次数。折行每断一行要量一次行宽
// （segW），所以「一次布局的测量总数」与行数同阶；命中缓存时只剩
// 行高/基线那两三次固定测量，是与文本长度无关的常数。
type wrapMeasureCounter struct {
	Graphics
	measures int
}

func (c *wrapMeasureCounter) MeasureText(s string, size int, bold bool, family string) (int, int) {
	c.measures++
	return c.Graphics.MeasureText(s, size, bold, family)
}

func (c *wrapMeasureCounter) FontMetrics(s string, size int, bold bool, family string) (int, int) {
	c.measures++
	return c.Graphics.FontMetrics(s, size, bold, family)
}

func (c *wrapMeasureCounter) MeasureTextWeight(s string, size, weightNum int, family string) (int, int) {
	c.measures++
	if wm, ok := c.Graphics.(WeightedMeasurer); ok {
		return wm.MeasureTextWeight(s, size, weightNum, family)
	}
	return c.Graphics.MeasureText(s, size, weightNum >= 550, family)
}

func (c *wrapMeasureCounter) FontMetricsWeight(s string, size, weightNum int, family string) (int, int) {
	c.measures++
	if wm, ok := c.Graphics.(WeightedMeasurer); ok {
		return wm.FontMetricsWeight(s, size, weightNum, family)
	}
	return c.Graphics.FontMetrics(s, size, weightNum >= 550, family)
}

// 同一帧内多次 layoutTextarea 只折行一次：后续调用命中缓存，测量次数
// 掉到与文本长度无关的常数。这条断言是本次修复的核心防退化点——缓存
// 一旦失效（键算错、失效钩子漏挂），代码仍然正确，只是每帧重新折行全文，
// 19.5MB 的文档实测单帧分配 1.3GB。
func TestTextareaWrapCachedAcrossLayouts(t *testing.T) {
	c := &wrapMeasureCounter{Graphics: NewFakeGraphics()}
	vp := &HeadlessViewport{W: 400, H: 300, G: c}
	doc, err := OpenDocument(vp, `<textarea id="t" style="width:200px;height:100px"></textarea>`)
	if err != nil {
		t.Fatal(err)
	}
	b := inner(doc.GetElementById("t"))
	if b == nil {
		t.Fatal("textarea not found")
	}
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "line %03d of the textarea body text\n", i)
	}
	b.SetAttribute("value", sb.String())

	// 第一次布局：真的要折行，测量次数与行数同阶。
	c.measures = 0
	l1 := layoutTextarea(c, b)
	firstMeasures := c.measures
	if len(l1.lines) < 2 {
		t.Fatalf("应折成多行，实得 %d 行", len(l1.lines))
	}
	if firstMeasures < len(l1.lines) {
		t.Fatalf("第一次布局测量 %d 次，少于行数 %d（折行路径没走到？）",
			firstMeasures, len(l1.lines))
	}

	// 后续布局：必须命中缓存。上限 8 是给行高/基线/选区等的固定开销留余量，
	// 关键是与 firstMeasures（数百次）差一个数量级。
	for i := 0; i < 5; i++ {
		c.measures = 0
		l2 := layoutTextarea(c, b)
		if len(l2.lines) != len(l1.lines) {
			t.Fatalf("缓存命中后行数变了 = %d, want %d", len(l2.lines), len(l1.lines))
		}
		if c.measures > 8 {
			t.Errorf("第 %d 次布局测量 %d 次（折行没命中缓存；首次是 %d 次）",
				i+2, c.measures, firstMeasures)
		}
	}

	// 值变化必须让缓存失效：新的内容要重新折行。
	c.measures = 0
	b.SetAttribute("value", "changed value")
	l3 := layoutTextarea(c, b)
	if c.measures > 8 {
		t.Logf("注：值变化后测量 %d 次（重建缓存，含逐字宽度备忘的冷启动）", c.measures)
	}
	if len(l3.lines) != 1 || l3.lineText(l3.lines[0]) != "changed value" {
		t.Errorf("失效后行 = %#v, want 单行 \"changed value\"", l3.lines)
	}
}

// 宽度变化必须让缓存失效：可用宽变了，折行结果就不同。
func TestTextareaWrapInvalidatedByWidth(t *testing.T) {
	c := &wrapMeasureCounter{Graphics: NewFakeGraphics()}
	vp := &HeadlessViewport{W: 400, H: 300, G: c}
	doc, err := OpenDocument(vp, `<textarea id="t" style="width:200px;height:100px"></textarea>`)
	if err != nil {
		t.Fatal(err)
	}
	b := inner(doc.GetElementById("t"))
	b.SetAttribute("value", "aaaa bbbb cccc dddd eeee ffff gggg hhhh")

	l1 := layoutTextarea(c, b)
	if len(l1.lines) < 2 {
		t.Fatalf("200px 宽下应折成多行，实得 %d 行", len(l1.lines))
	}
	// 宽度加倍 → 行数必须变少（缓存键含 maxW）。
	b.width *= 2
	l2 := layoutTextarea(c, b)
	if len(l2.lines) >= len(l1.lines) {
		t.Errorf("宽度加倍后行数 = %d, want < %d（缓存键没含宽度）",
			len(l2.lines), len(l1.lines))
	}
}

// 行文本按区间现切，不随行存储：大文档不该为每行留一份文本副本。
func TestTextareaLineTextSlicedNotStored(t *testing.T) {
	c := &wrapMeasureCounter{Graphics: NewFakeGraphics()}
	vp := &HeadlessViewport{W: 400, H: 300, G: c}
	doc, err := OpenDocument(vp, `<textarea id="t" style="width:200px;height:100px"></textarea>`)
	if err != nil {
		t.Fatal(err)
	}
	b := inner(doc.GetElementById("t"))
	b.SetAttribute("value", "first line\nsecond line\nthird line")
	l := layoutTextarea(c, b)
	if len(l.lines) != 3 {
		t.Fatalf("行数 = %d, want 3", len(l.lines))
	}
	want := []string{"first line", "second line", "third line"}
	for i, ln := range l.lines {
		if got := l.lineText(ln); got != want[i] {
			t.Errorf("行[%d] 文本 = %q, want %q", i, got, want[i])
		}
	}
	// 行区间恢复的 rune 总数等于原文 rune 数（末行 end）。
	if got := l.lines[len(l.lines)-1].end; got != len("first line\nsecond line\nthird line") {
		t.Errorf("末行 end = %d, want %d", got, len("first line\nsecond line\nthird line"))
	}
}

// caretRow 走二分：行 end 单调，光标行查找不该线性扫全表。
// 用「二分结果 == 线性结果」断言正确性（大表上线性版本慢，只在小表上对照）。
func TestCaretRowMatchesLinearScan(t *testing.T) {
	g := NewFakeGraphics()
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&sb, "row %03d\n", i)
	}
	lines := wrapTextarea(g, sb.String(), 1000, 10, 0, false, "")
	linear := func(caret int) int {
		for i, ln := range lines {
			if caret <= ln.end {
				return i
			}
		}
		return len(lines) - 1
	}
	for caret := 0; caret <= len(sb.String())+2; caret++ {
		if got, want := caretRow(lines, caret), linear(caret); got != want {
			t.Fatalf("caretRow(%d) = %d, want %d", caret, got, want)
		}
	}
}
