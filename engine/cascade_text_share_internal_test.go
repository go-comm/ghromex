package engine

import (
	"testing"
)

// ---------------------------------------------------------------------------
// 文本节点共享父元素 computed（见 style_cascade.go）的回归锁定：
// 共享省下每文本节点 ~440B 样式对象与一轮规则匹配，但代价是父元素的
// 非继承属性（display/position/padding/opacity）也会从 #text 的 computed
// 读出来——以下用例锁定所有「按非继承属性分支」的场合都把 #text 排除在外，
// 行为必须与独立级联时完全一致。
// ---------------------------------------------------------------------------

// shareTestDoc 解析 + 级联 + 布局（探针同款口径，无头）。
func shareTestDoc(t *testing.T, g Graphics, w, h int, src string) HTMLDocument {
	t.Helper()
	doc := NewDocument()
	doc.(*htmlDocument).viewport = NewHeadlessViewport(w, h)
	parseInto(doc, doc.Head(), doc.Body(), src, doc)
	resolveStylesTree(doc, nil, doc.Stylesheet())
	root := inner(doc)
	layoutBox(g, root, w, h, 0, 0, w, h, nil)
	layoutPositioned(g, root, w, h)
	return doc
}

// findText 返回树里第一个文本节点（包装器）。
func findText(e HTMLElement) HTMLElement {
	base := inner(e)
	if base == nil {
		return nil
	}
	for _, c := range base.children {
		if tn, ok := c.(*textNode); ok {
			return tn
		}
		if t := findText(c); t != nil {
			return t
		}
	}
	return nil
}

// findID 按 id 找元素。
func findID(e HTMLElement, id string) *htmlElement {
	base := inner(e)
	if base == nil {
		return nil
	}
	if base.id == id {
		return base
	}
	for _, c := range base.children {
		if r := findID(c, id); r != nil {
			return r
		}
	}
	return nil
}

// firstRun 返回该元素子树里第一个文本 run（run.x/run.y 是绝对坐标）。
func firstRun(e *htmlElement) (textRun, bool) {
	var found bool
	var out textRun
	var walk func(*htmlElement)
	walk = func(el *htmlElement) {
		if el == nil || found {
			return
		}
		if el.tagName == "#text" && len(el.runs) > 0 {
			out, found = el.runs[0], true
			return
		}
		for _, c := range el.children {
			walk(inner(c))
		}
	}
	walk(e)
	return out, found
}

// paintRecorder 记录文字绘制的最终 Paint（穿透 opacity 包装之后的值）。
type paintRecorder struct {
	Graphics
	paints []Paint
	texts  []string
}

func (r *paintRecorder) DrawText(x, y, w, h int, p Paint, s string) {
	r.paints = append(r.paints, p)
	r.texts = append(r.texts, s)
	r.Graphics.DrawText(x, y, w, h, p, s)
}

// TestTextComputedSharesParentPointer 级联后文本节点的 computed 与父元素
// 是同一个对象（这正是省内存的来源）；文本读到的继承属性不变。
func TestTextComputedSharesParentPointer(t *testing.T) {
	src := `<html><head><style>
	body { margin: 0 }
	#box { color: #FF0000; padding: 12px; font-size: 18px }
	</style></head><body><div id="box">hi</div></body></html>`
	doc := shareTestDoc(t, NewFakeGraphics(), 200, 100, src)
	box := findID(inner(doc), "box")
	if box == nil {
		t.Fatal("找不到 #box")
	}
	tn := findText(box.self)
	if tn == nil {
		t.Fatal("找不到文本节点")
	}
	tb := inner(tn)
	if tb.computed != box.computed {
		t.Fatal("文本节点仍在独立级联（应共享父 computed 指针）")
	}
	// 继承属性从共享对象读出的值正确：font-size 18、color 红。
	if fs := resolveLen(tb.computed.FontSize(), 0); fs != 18 {
		t.Errorf("文本字号 = %d, want 18", fs)
	}
	if c := tb.computed.Color(); c == nil {
		t.Fatal("文本颜色为空")
	} else if r, g, b, _ := c.RGBA(); r != 255 || g != 0 || b != 0 {
		t.Errorf("文本颜色 = (%d,%d,%d), want (255,0,0)", r, g, b)
	}
}

// TestRelativeParentShiftsTextOnce relative 父元素下的文本只偏移一次：
// 布局第二遍若把 #text 当独立定位盒（共享的 Position 读出父的值），
// applyRelativeOffset 会把它再平移一遍。对照静态布局：文本相对盒的
// 偏移量在两种布局下必须完全一致。
func TestRelativeParentShiftsTextOnce(t *testing.T) {
	staticSrc := `<html><head><style>
	body { margin: 0 }
	#r { width: 100px; height: 20px }
	</style></head><body><div id="r">hi</div></body></html>`
	relSrc := `<html><head><style>
	body { margin: 0 }
	#r { position: relative; top: 10px; left: 20px; width: 100px; height: 20px }
	</style></head><body><div id="r">hi</div></body></html>`

	docA := shareTestDoc(t, NewFakeGraphics(), 300, 120, staticSrc)
	rA := findID(inner(docA), "r")
	runA, okA := firstRun(rA)
	docB := shareTestDoc(t, NewFakeGraphics(), 300, 120, relSrc)
	rB := findID(inner(docB), "r")
	runB, okB := firstRun(rB)
	if !okA || !okB {
		t.Fatal("没有文本 run")
	}
	// 盒本身偏移恰好一次。
	if rB.x != rA.x+20 || rB.y != rA.y+10 {
		t.Errorf("盒偏移 = (%d,%d), want (%d,%d)", rB.x, rB.y, rA.x+20, rA.y+10)
	}
	// 文本相对盒的落点与静态布局一致——不一致即双重偏移。
	if runB.x-rB.x != runA.x-rA.x {
		t.Errorf("文本相对盒 dx = %d, want %d（relative 偏移被重复施加到文本）",
			runB.x-rB.x, runA.x-rA.x)
	}
	if runB.y-rB.y != runA.y-rA.y {
		t.Errorf("文本相对盒 dy = %d, want %d（relative 偏移被重复施加到文本）",
			runB.y-rB.y, runA.y-rA.y)
	}
}

// TestFixedAnchorPushbackKeepsText fixed 盒按 right/bottom 锚定时用
// shiftBox 回推整盒；共享父 computed 后文本的 Position 读出来是 fixed，
// shiftBox 若不排除 #text 就会把文本落在原地（脱离盒）。
func TestFixedAnchorPushbackKeepsText(t *testing.T) {
	src := `<html><head><style>
	body { margin: 0 }
	#f { position: fixed; top: 0; right: 0 }
	</style></head><body><div id="f">AB</div></body></html>`
	doc := shareTestDoc(t, NewFakeGraphics(), 300, 120, src)
	f := findID(inner(doc), "f")
	run, ok := firstRun(f)
	if !ok {
		t.Fatal("没有文本 run")
	}
	// 右锚定后盒贴右缘；文本 run 必须随盒平移（未加守卫时留在 x≈0）。
	if f.x <= 0 {
		t.Fatalf("盒未右移：#f.x = %d", f.x)
	}
	if run.x < f.x-1 || run.x > f.x+f.width {
		t.Errorf("run.x = %d, want 落在右移后的盒 [%d,%d]", run.x, f.x, f.x+f.width)
	}
}

// TestTextOpacityAppliedOnce 父元素 opacity:0.5 时文本 alpha 只乘一次：
// 文本共享 computed 后若自己也包一层 opacity，绘制色的 alpha 会变成 63
// （0.5×0.5）而不是 127。
func TestTextOpacityAppliedOnce(t *testing.T) {
	src := `<html><head><style>
	body { margin: 0 }
	#o { opacity: 0.5; color: #FF0000 }
	</style></head><body><div id="o">hi</div></body></html>`
	doc := shareTestDoc(t, NewFakeGraphics(), 200, 100, src)
	rec := &paintRecorder{Graphics: NewFakeGraphics()}
	RenderNode(rec, doc)
	var found bool
	for i, s := range rec.texts {
		if s != "hi" {
			continue
		}
		found = true
		c := rec.paints[i].Color()
		if c == nil {
			t.Fatal("文字颜色为空")
		}
		if _, _, _, a := c.RGBA(); a < 100 {
			t.Errorf("文字 alpha = %d, want ≈127（opacity 被文本层重复乘算）", a)
		}
	}
	if !found {
		t.Error("未绘制文本 hi")
	}
}

// TestTextNotCollectedAsPositioned relative+z-index 的父元素下的文本不得
// 进入定位层收集，也不得因「文本被当成定位盒」在主 pass 被漏画。
func TestTextNotCollectedAsPositioned(t *testing.T) {
	src := `<html><head><style>
	body { margin: 0 }
	#z { position: relative; z-index: 5 }
	</style></head><body><div id="z">hi</div></body></html>`
	doc := shareTestDoc(t, NewFakeGraphics(), 200, 100, src)
	for _, pb := range collectPositioned(doc) {
		if b := inner(pb.e); b != nil && b.tagName == "#text" {
			t.Fatal("#text 被当成了定位盒收进定位层")
		}
	}
	rec := &paintRecorder{Graphics: NewFakeGraphics()}
	RenderNode(rec, doc)
	n := 0
	for _, s := range rec.texts {
		if s == "hi" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("文本绘制次数 = %d, want 1（漏画或双画都说明文本被误判为定位盒）", n)
	}
}

// TestTextRectStaysZero 文本没有自己的盒：父元素的 padding 不得通过共享
// computed 把文本的 GetBoundingClientRect 撑开（零盒口径不变）。
func TestTextRectStaysZero(t *testing.T) {
	src := `<html><head><style>
	body { margin: 0 }
	#b { padding: 20px }
	</style></head><body><div id="b">hi</div></body></html>`
	doc := shareTestDoc(t, NewFakeGraphics(), 200, 100, src)
	tn := findText(doc)
	if tn == nil {
		t.Fatal("找不到文本节点")
	}
	r := tn.GetBoundingClientRect()
	if w := r.Right().Pixel() - r.Left().Pixel(); w != 0 {
		t.Errorf("文本 rect 宽 = %d, want 0（父 padding 泄漏）", w)
	}
	if h := r.Bottom().Pixel() - r.Top().Pixel(); h != 0 {
		t.Errorf("文本 rect 高 = %d, want 0（父 padding 泄漏）", h)
	}
}
