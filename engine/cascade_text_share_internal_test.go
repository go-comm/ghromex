package engine

import (
	"testing"
)

// ---------------------------------------------------------------------------
// 文本节点共享父元素 computed（见 style_cascade.go）的回归锁定：
// 共享省下每文本节点 400B 样式对象与一轮规则匹配，但代价是父元素的
// 非继承属性（display/position/padding/opacity）也会从文本节点的 computed
// 读出来——以下用例锁定所有「按非继承属性分支」的场合都用 isText() 把文本
// 排除在外，行为必须与独立级联时完全一致。
//
// 判定点只有一处（htmlElement.isText()，看 nodeType 字段）：节点类型是类型
// 信息，不靠标签名字符串 #text 判断，否则「标签名恰好等于 #text 的元素」
// 会被误伤，且拼错的字符串编译期拦不住。
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
		if el.isText() && len(el.runs) > 0 {
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

// boxGeo 是盒几何的可比较快照（htmlElement 整体含切片，不能直接 ==）。
type boxGeo struct{ x, y, w, h, layoutH int }

func snapGeo(e *htmlElement) boxGeo {
	return boxGeo{x: e.x, y: e.y, w: e.width, h: e.height, layoutH: e.layoutH}
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
		if b := inner(pb.e); b != nil && b.isText() {
			t.Fatal("文本节点被当成了定位盒收进定位层")
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

// shareColor 断言颜色分量（带场合标签，失败时能一眼看出是哪一轮）。
func shareColor(t *testing.T, when string, c Color, r, g, b uint8) {
	t.Helper()
	if c == nil {
		t.Fatalf("%s：颜色为空", when)
	}
	if cr, cg, cb, _ := c.RGBA(); cr != r || cg != g || cb != b {
		t.Errorf("%s：颜色 = (%d,%d,%d), want (%d,%d,%d)", when, cr, cg, cb, r, g, b)
	}
}

// TestElementNamedTextIsNotTextNode 标签名恰好是 "#text" 的元素必须按**元素**
// 处理：有自己的 computed 对象、自己的盒。这是硬编码 tagName 比较留下的真实
// 误伤点——旧口径下这种元素会被当成文本节点，于是不独立级联、共享父样式、
// 零盒（内容画不出来）。判据换成 nodeType 后此处应当正常。
func TestElementNamedTextIsNotTextNode(t *testing.T) {
	src := `<html><head><style>
	body { margin: 0 }
	</style></head><body><div id="b">hi</div></body></html>`
	doc := shareTestDoc(t, NewFakeGraphics(), 200, 100, src)
	body := inner(doc.Body())

	// 手工构造：一个标签名叫 "#text" 的元素，里面放真正的文本节点。
	weird := newElement("#text")
	weird.AppendChild(NewTextNode("x"))
	weird.SetAttribute("style", "position: relative; top: 5px")
	body.AppendChild(weird)
	wb := inner(weird)
	resolveStylesTree(doc, nil, doc.Stylesheet())
	layoutBox(NewFakeGraphics(), body, 200, 100, 0, 0, 200, 100, nil)
	layoutPositioned(NewFakeGraphics(), body, 200, 100)

	if wb.isText() {
		t.Error("标签名为 #text 的元素被当成了文本节点")
	}
	if wb.computed == nil || wb.computed == body.computed {
		t.Fatal("该元素没有独立 computed（被按文本节点跳过级联）")
	}
	// 它内部的文本节点照旧共享它自己的 computed。
	tn := findText(weird)
	if tn == nil {
		t.Fatal("找不到内部文本节点")
	}
	if inner(tn).computed != wb.computed {
		t.Error("元素内部的文本节点未共享它的 computed")
	}
	// 定位层收集按 nodeType 判断：这个元素要能进（它是盒），
	// 它内部的文本节点不能进（旧口径按标签名比较，两边都是错的）。
	if !isPositioned(weird) {
		t.Error("标签名为 #text 的元素未进入定位层（零盒口径误伤）")
	}
	if isPositioned(tn) {
		t.Error("文本节点被当成定位盒")
	}
	for _, pb := range collectPositioned(doc) {
		if b := inner(pb.e); b != nil && b.isText() {
			t.Error("文本节点被收进定位层")
		}
	}
	// 零盒口径只属于文本节点：元素有盒、文本没有。
	if wr := weird.GetBoundingClientRect(); wr.Right().Pixel()-wr.Left().Pixel() <= 0 {
		t.Errorf("该元素 rect 宽 = %d, want >0（被按文本节点的零盒处理）",
			wr.Right().Pixel()-wr.Left().Pixel())
	}
	if tr := tn.GetBoundingClientRect(); tr.Right().Pixel()-tr.Left().Pixel() != 0 {
		t.Errorf("文本节点 rect 宽 = %d, want 0", tr.Right().Pixel()-tr.Left().Pixel())
	}
}

// TestHoverSharedTextFollowsParentColor 交互状态分级（change_level.go /
// hover_change.go）与文本共享 computed 的交叉点。
//
// :hover 只改 color 时走**纯绘制档**：就地重算受影响子树、跳过布局。文本节点
// 没有自己的样式对象，父元素改的是**同一份** computed，于是共享必须自动跟上
// 新颜色；反过来，若重算把 computed 换成新对象（而非就地改写），文本就会永远
// 停在旧颜色上——这是共享方案唯一会被静默改坏的地方，故在此钉住：
//
//  1. 悬停档位仍是 ChangePaintOnly（共享省下的级联不得被全量重排吃掉）；
//  2. 重算前后文本的 computed 指针仍等于父元素的那一份；
//  3. 文本读到的颜色、以及跳过布局直接重绘时的绘制色，都随父更新；
//  4. 盒几何与文本落点不变（布局被跳过的前提）。
func TestHoverSharedTextFollowsParentColor(t *testing.T) {
	src := `<html><head><style>
	body { margin: 0 }
	#h { color: #0000FF }
	#h:hover { color: #FF0000 }
	</style></head><body><div id="h">hi</div></body></html>`
	doc := shareTestDoc(t, NewFakeGraphics(), 200, 100, src)
	box := findID(inner(doc), "h")
	if box == nil {
		t.Fatal("找不到 #h")
	}
	tn := findText(box.self)
	tb := inner(tn)
	if tb == nil {
		t.Fatal("找不到文本节点")
	}
	if tb.computed != box.computed {
		t.Fatal("前置条件不成立：文本未共享父 computed")
	}
	shareColor(t, "悬停前", tb.computed.Color(), 0, 0, 255)
	geo := snapGeo(box) // 盒几何快照
	run0, ok := firstRun(box)
	if !ok {
		t.Fatal("没有文本 run")
	}

	reflowBase, paintBase := doc.ChangeCount(), PaintCount(doc)
	SetHoveredElement(doc, box.self)

	if level, _, _ := PendingChange(doc, reflowBase, paintBase); level != ChangePaintOnly {
		t.Errorf("悬停档位 = %d, want ChangePaintOnly(%d)（共享文本的存在让悬停退化成全量重排？）",
			level, ChangePaintOnly)
	}
	if tb.computed != box.computed {
		t.Fatal("悬停重算换掉了 computed 对象，文本共享断裂（文本会停在旧颜色）")
	}
	shareColor(t, "悬停后", tb.computed.Color(), 255, 0, 0)

	// 端到端：按主循环口径跳过布局、直接重绘，绘制层的文字必须是新色。
	rec := &paintRecorder{Graphics: NewFakeGraphics()}
	RenderNode(rec, doc)
	var drawn bool
	for i, s := range rec.texts {
		if s != "hi" {
			continue
		}
		drawn = true
		shareColor(t, "悬停后绘制色", rec.paints[i].Color(), 255, 0, 0)
	}
	if !drawn {
		t.Error("未绘制文本 hi")
	}

	// 取消悬停：同样只重绘，颜色回蓝，几何与文本落点一步未动。
	reflowBase, paintBase = doc.ChangeCount(), PaintCount(doc)
	SetHoveredElement(doc, nil)
	if level, _, _ := PendingChange(doc, reflowBase, paintBase); level != ChangePaintOnly {
		t.Errorf("取消悬停档位 = %d, want ChangePaintOnly(%d)", level, ChangePaintOnly)
	}
	shareColor(t, "取消悬停后", tb.computed.Color(), 0, 0, 255)
	if snapGeo(box) != geo {
		t.Errorf("纯绘制档改动了盒几何：%+v → %+v", geo, snapGeo(box))
	}
	if run1, ok := firstRun(box); !ok || run1 != run0 {
		t.Errorf("纯绘制档改动了文本落点：%+v → %+v", run0, run1)
	}
}
