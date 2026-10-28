package engine

import (
	"strings"
	"testing"
)

// 交互状态变更的档位回归（见 hover_change.go / change_level.go）。
//
// # 为什么需要它
//
// 状态变更曾一律 markChanged → 整棵树级联 + 布局：42 万元素的压力页上
// 鼠标滑入/滑出一个元素 1.72 秒（13,380,022 次文本测量），而这些变更里
// 绝大多数只改颜色、盒模型纹丝不动，无 :hover 规则的页面更是纯浪费。
// 分级后要钉住三件事：
//
//  1. 档位判定正确：无影响→零标记；纯绘制→只进重绘账；改盒模型→照旧全量；
//  2. 只重绘档的样式**已就地算完**：不跑 LayoutDocument 也必须拿到新值，
//     且下一帧若走滚动快路径（跳过级联），悬停颜色不能回退；
//  3. 级联范围收得住：受影响子树之外的 computed 一个字节都不动（否则
//     paint 档就退化成了「换个名目的全树级联」）。
//
// 判据沿用 scroll_layout_internal_test.go 的口径：**文本测量次数**而非计时
// （快路径 0 次、全量整树），计时不可复现，不能写成断言。

// hoverDoc 打开一个文档并返回全套句柄（文档、内部实现、计数桩、视口），
// 首次全量布局的测量数已清零。
func hoverDoc(tb testing.TB, src string, w, h int) (HTMLDocument, *htmlDocument, *measureCounter, *HeadlessViewport) {
	tb.Helper()
	c, vp := newMeasureCounter(w, h)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		tb.Fatal(err)
	}
	c.measures = 0
	return doc, doc.(*htmlDocument), c, vp
}

// colorHex 把 Color 拼成 "rrggbbaa" 便于断言与报错。
func colorHex(c Color) string {
	if c == nil {
		return "<nil>"
	}
	r, g, b, a := c.RGBA()
	return strings.ToLower(hex2(r) + hex2(g) + hex2(b) + hex2(a))
}

func hex2(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&0xf]})
}

// 纯绘制 :hover：不碰重排账、样式在标记当场就地生效、盒子纹丝不动。
func TestHoverPaintOnlyUpdatesStyleWithoutLayout(t *testing.T) {
	doc, d, c, _ := hoverDoc(t, `<!doctype html><html><head><style>
		a { color: #000000; } a:hover { color: #ff0000; }
	</style></head><body style="margin:0"><a id="a" href="#">x</a></body></html>`, 400, 300)
	a := doc.GetElementById("a")
	if a == nil {
		t.Fatal("元素缺失")
	}
	rb, pb := d.changed, d.paintChanged
	w0, h0 := widthOfBox(a), heightOfBox(a)

	SetHoveredElement(doc, a)

	level, rc, pc := PendingChange(doc, rb, pb)
	if level != ChangePaintOnly {
		t.Fatalf("档位 = %d，期望 ChangePaintOnly(%d)", level, ChangePaintOnly)
	}
	if rc != rb || pc != pb+1 {
		t.Fatalf("重排账 %d→%d、重绘账 %d→%d：纯绘制档只该动重绘账", rb, rc, pb, pc)
	}
	// 关键：没有跑过任何布局，样式必须已经是新值（标记前就地级联）。
	if got := colorHex(inner(a).computed.Color()); got != "ff0000ff" {
		t.Errorf("未布局时悬停色 = %s，期望 ff0000ff（样式没在标记前算完）", got)
	}
	// 盒子不变：这正是「跳过布局」的正当性来源。
	if w1, h1 := widthOfBox(a), heightOfBox(a); w1 != w0 || h1 != h0 {
		t.Errorf("纯绘制 :hover 改动了盒子：%dx%d → %dx%d", w0, h0, w1, h1)
	}
	if c.measures != 0 {
		t.Errorf("纯绘制 :hover 消耗了 %d 次文本测量（不该有任何布局动作）", c.measures)
	}
}

// 级联范围：受影响子树**之外**的 computed 必须原样保留。
//
// 手段是白盒哨兵——先往兄弟元素的 computed 里塞一个非默认颜色，悬停后
// 若它还在，说明这一档没有全树重算（全树级联会把 computed 重置回模板值，
// 哨兵必然被冲掉）。
func TestHoverPaintOnlyCascadeScopedToSubtree(t *testing.T) {
	doc, d, _, _ := hoverDoc(t, `<!doctype html><html><head><style>
		#a:hover { color: #ff0000; }
	</style></head><body style="margin:0">
		<div id="a">in</div><div id="b">out</div>
	</body></html>`, 400, 300)
	a, b := doc.GetElementById("a"), doc.GetElementById("b")
	sentinel := NewColor(1, 2, 3, 255)
	inner(b).computed.SetColor(sentinel)

	rb, pb := d.changed, d.paintChanged
	SetHoveredElement(doc, a)

	if level, _, _ := PendingChange(doc, rb, pb); level != ChangePaintOnly {
		t.Fatalf("档位 = %d，期望 ChangePaintOnly", level)
	}
	if got := colorHex(inner(a).computed.Color()); got != "ff0000ff" {
		t.Errorf("悬停目标色 = %s，期望 ff0000ff", got)
	}
	if got := colorHex(inner(b).computed.Color()); got != colorHex(sentinel) {
		t.Errorf("子树外元素的 computed 被重算了（%s → %s）：paint 档退化成全树级联", colorHex(sentinel), got)
	}
}

// 无 :hover 规则但悬停在表单控件上：UA 悬停反馈（底色/边框色）仍要生效——
// 纯绘制档不能连它一起吞掉。
func TestHoverFormControlWithoutAuthorRulesStillPaints(t *testing.T) {
	doc, d, _, _ := hoverDoc(t, `<!doctype html><html><body style="margin:0">
		<button id="b">go</button></body></html>`, 400, 300)
	b := doc.GetElementById("b")
	if b == nil {
		t.Fatal("按钮缺失")
	}
	before := colorHex(inner(b).computed.BackgroundColor())

	rb, pb := d.changed, d.paintChanged
	SetHoveredElement(doc, b)

	level, rc, pc := PendingChange(doc, rb, pb)
	if level != ChangePaintOnly || rc != rb || pc != pb+1 {
		t.Fatalf("档位/账本 = %d(%d,%d)，期望 ChangePaintOnly(%d,%d)", level, rc, pc, rb, pb+1)
	}
	after := colorHex(inner(b).computed.BackgroundColor())
	if after == before {
		t.Errorf("悬停按钮后底色没变（%s）：UA 悬停反馈没生效", after)
	}
	if want := colorHex(uaHoverFace); after != want {
		t.Errorf("悬停底色 = %s, 期望 UA 悬停色 %s", after, want)
	}
}

// :active 对按钮同样走纯绘制档（无 :active 规则时只有 UA 按下反馈）。
func TestActiveOnButtonWithoutRulesIsPaintOnly(t *testing.T) {
	doc, d, _, _ := hoverDoc(t, `<!doctype html><html><body style="margin:0">
		<button id="b">go</button></body></html>`, 400, 300)
	b := doc.GetElementById("b")
	rb, pb := d.changed, d.paintChanged
	SetActiveElement(doc, b)
	if level, rc, pc := PendingChange(doc, rb, pb); level != ChangePaintOnly || rc != rb || pc != pb+1 {
		t.Fatalf("按下按钮的档位/账本 = %d(%d,%d)，期望 ChangePaintOnly(%d,%d)", level, rc, pc, rb, pb+1)
	}
	if want := colorHex(uaActiveFace); colorHex(inner(b).computed.BackgroundColor()) != want {
		t.Errorf("按下底色 = %s, 期望 UA 按下色 %s", colorHex(inner(b).computed.BackgroundColor()), want)
	}
}

// 无 :focus 规则也要进重绘账：聚焦框/光标/选区由渲染层按 focus 现画，
// 不重绘就看不见；但不该惊动重排账。
// 账本允许 ≥2 笔：首次聚焦 input 除档位判定的一笔外，ensureCaretVisible
// 定位文本滚动窗口（textScrollSet 首次置位，窗口从「舍头保尾」切到
// 「跟随光标」）还会补一笔重绘账——两笔都是纯绘制级。
func TestFocusWithoutRulesStillRepaints(t *testing.T) {
	doc, d, _, _ := hoverDoc(t, `<!doctype html><html><body style="margin:0">
		<input id="i"></body></html>`, 400, 300)
	i := doc.GetElementById("i")
	rb, pb := d.changed, d.paintChanged
	SetFocusedElement(doc, i)
	level, rc, pc := PendingChange(doc, rb, pb)
	if level != ChangePaintOnly || rc != rb || pc < pb+1 {
		t.Fatalf("聚焦的档位/账本 = %d(%d,%d)，期望 ChangePaintOnly、至少 %d 笔重绘账", level, rc, pc, pb+1)
	}
}

// 改盒模型的 :hover 照旧回退全量：档位=重排、布局真的发生、几何跟着变。
func TestHoverReflowRuleFallsBackToFullLayout(t *testing.T) {
	doc, d, c, _ := hoverDoc(t, `<!doctype html><html><head><style>
		div { width: 60px; height: 20px; } div:hover { padding-left: 6px; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`, 400, 300)
	d0 := doc.GetElementById("d")
	w0 := widthOfBox(d0)

	rb, pb := d.changed, d.paintChanged
	SetHoveredElement(doc, d0)

	level, rc, pc := PendingChange(doc, rb, pb)
	if level != ChangeReflow || rc != rb+1 || pc != pb {
		t.Fatalf("档位/账本 = %d(%d,%d)，期望 ChangeReflow(%d,%d)", level, rc, pc, rb+1, pb)
	}
	// 主循环见 ChangeReflow 才调 LayoutDocument——布局必须真的把几何排出来。
	if measures := layoutAndMeasures(doc, c); measures == 0 {
		t.Fatal("重排档的布局走了纯平移快路径（测量 0 次）：盒模型变更被漏排")
	}
	if w1 := widthOfBox(d0); w1 != w0+6 {
		t.Errorf("悬停后宽 = %d，期望 %d（:hover 的 padding-left 未生效）", w1, w0+6)
	}
}

// 规则提及 :hover 但绑定的是别的标签：链上没有元素会被它命中 → 零标记。
// （档位判定在 roots 之后：没有受影响元素时连「重排档」都不该是。）
func TestHoverRuleOnUnmatchedTagMarksNothing(t *testing.T) {
	doc, d, _, _ := hoverDoc(t, `<!doctype html><html><head><style>
		a:hover { width: 300px; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`, 400, 300)
	rb, pb := d.changed, d.paintChanged
	SetHoveredElement(doc, doc.GetElementById("d"))
	if level, rc, pc := PendingChange(doc, rb, pb); level != ChangeNone || rc != rb || pc != pb {
		t.Fatalf("悬停非目标标签却标记了变更：%d(%d,%d)", level, rc, pc)
	}
}

// paint 档不进 changed/scrollMark 两本账 → 下一档滚轮必须照旧走快路径，
// 且快路径跳过级联时悬停颜色**不能回退**（样式在标记前已算完，这正是
// 「标记前就地级联」而非「布局时级联」的原因）。
func TestPaintMarkKeepsScrollFastPathAndStyle(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><style>
		a { color: #000000; } a:hover { color: #ff0000; }
		p { font-size: 14px; margin: 0 0 8px; }
	</style></head><body style="margin:0"><a id="a" href="#">link</a>`)
	for i := 0; i < 120; i++ {
		b.WriteString(`<p>段落，用来撑出可滚高度。</p>`)
	}
	b.WriteString(`</body></html>`)
	doc, d, c, _ := hoverDoc(t, b.String(), 400, 300)

	rb, pb := d.changed, d.paintChanged
	SetHoveredElement(doc, doc.GetElementById("a"))
	if level, _, _ := PendingChange(doc, rb, pb); level != ChangePaintOnly {
		t.Fatalf("前置条件：悬停应为重绘档，实际 %d", level)
	}

	if !OnDocumentWheel(doc, 10, 10, 0, 100) {
		t.Fatal("滚轮未产生滚动：测试页不可滚")
	}
	if measures := layoutAndMeasures(doc, c); measures != 0 {
		t.Fatalf("重绘档污染了滚动快路径：滚轮重排整棵树（测量 %d 次）", measures)
	}
	if got := colorHex(inner(doc.GetElementById("a")).computed.Color()); got != "ff0000ff" {
		t.Errorf("快路径后的悬停色 = %s，期望 ff0000ff（样式没在标记前算完，跳过级联就回退）", got)
	}
}

// 档位优先级：重排待处理时压过重绘；两本账都记平即 ChangeNone。
func TestPendingChangeLevelPrecedence(t *testing.T) {
	doc, d, _, _ := hoverDoc(t, `<!doctype html><html><head><style>
		a { color: #000000; } a:hover { color: #ff0000; }
	</style></head><body style="margin:0"><a id="a" href="#">x</a></body></html>`, 400, 300)
	rb, pb := d.changed, d.paintChanged

	SetHoveredElement(doc, doc.GetElementById("a"))
	if level, _, _ := PendingChange(doc, rb, pb); level != ChangePaintOnly {
		t.Fatalf("只有重绘变更时档位 = %d，期望 ChangePaintOnly", level)
	}
	d.markChanged() // 又来一笔重排级变更
	level, rc, pc := PendingChange(doc, rb, pb)
	if level != ChangeReflow {
		t.Fatalf("重排与重绘同时待处理时档位 = %d，期望 ChangeReflow（重排消化一切）", level)
	}
	if level, _, _ := PendingChange(doc, rc, pc); level != ChangeNone {
		t.Fatalf("两本账记平后档位 = %d，期望 ChangeNone", level)
	}
}

// widthOfBox 取元素边框盒宽（内部口径，省一次矩形换算的断言噪音）。
func widthOfBox(e HTMLElement) int {
	r := e.GetBoundingClientRect()
	return r.Right().Pixel() - r.Left().Pixel()
}

// heightOfBox 取元素边框盒高。
func heightOfBox(e HTMLElement) int {
	r := e.GetBoundingClientRect()
	return r.Bottom().Pixel() - r.Top().Pixel()
}
