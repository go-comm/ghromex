package engine

import (
	"os"
	"strings"
	"testing"
)

// 滚动/裁剪特性测试（白盒：需要读 scrollTop/scrollLeft/contentW 等未导出状态）。

// openScrollDoc 打开页面并完成首次布局 + 绘制。
func openScrollDoc(t *testing.T, w, h int, src string) (HTMLDocument, *BufferGraphics) {
	t.Helper()
	buf := NewBufferGraphics(w, h)
	vp := &HeadlessViewport{W: w, H: h, G: buf}
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)
	return doc, buf
}

func at(t *testing.T, buf *BufferGraphics, x, y int) (uint8, uint8, uint8, uint8) {
	t.Helper()
	r, g, b, a, ok := buf.ColorAt(x, y)
	if !ok {
		t.Fatalf("ColorAt(%d,%d) out of buffer", x, y)
	}
	return r, g, b, a
}

func isRed(r, g, b, a uint8) bool { return r == 255 && g == 0 && b == 0 && a == 255 }

func isNone(r, g, b, a uint8) bool { return a == 0 }

func TestParseOverflowKeywords(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body{margin:0}
#a{overflow:hidden}
#b{overflow:scroll}
#c{overflow:visible}
#d{overflow:auto}
#e{overflow-x:hidden;overflow-y:visible}
#f{overflow:clip}
#g{overflow:hidden auto}
</style></head><body>
<div id="a">a</div><div id="b">b</div><div id="c">c</div><div id="d">d</div>
<div id="e">e</div><div id="f">f</div><div id="g">g</div>
</body></html>`
	doc, _ := openScrollDoc(t, 200, 300, src)

	cases := []struct {
		id string
		x  Overflow
		y  Overflow
	}{
		{"a", OverflowHidden, OverflowHidden},
		{"b", OverflowScroll, OverflowScroll},
		{"c", OverflowVisible, OverflowVisible},
		{"d", OverflowAuto, OverflowAuto},
		{"e", OverflowHidden, OverflowVisible}, // overflow-x/overflow-y 分轴
		{"f", OverflowHidden, OverflowHidden},  // clip 按 hidden 处理
		{"g", OverflowHidden, OverflowAuto},    // 两值 = overflow-x overflow-y
	}
	for _, c := range cases {
		el := doc.QuerySelector("#" + c.id)
		if el == nil {
			t.Fatalf("#%s not found", c.id)
		}
		base := inner(el)
		if base == nil || base.computed == nil {
			t.Fatalf("#%s has no computed style", c.id)
		}
		if got := base.computed.OverflowX(); got != c.x {
			t.Errorf("#%s overflow-x = %v, want %v", c.id, got, c.x)
		}
		if got := base.computed.OverflowY(); got != c.y {
			t.Errorf("#%s overflow-y = %v, want %v", c.id, got, c.y)
		}
	}
}

func TestOverflowHiddenClipsPixels(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body{margin:0;background-color:#EDEEF2}
#box{width:80px;height:60px;overflow:hidden;background-color:#FFFFFF}
#tall{height:300px;background-color:#FF0000}
</style></head><body><div id="box"><div id="tall"></div></div></body></html>`
	_, buf := openScrollDoc(t, 200, 200, src)

	if r, g, b, a := at(t, buf, 40, 40); !isRed(r, g, b, a) {
		t.Fatalf("inside box (40,40) = %02X%02X%02X%02X, want red", r, g, b, a)
	}
	// 盒外：tall 高 300 会被裁掉，不许把红色画到 overflow:hidden 之外
	//（body 盒只到 y=60，盒外为未绘制的透明区）
	if r, g, b, a := at(t, buf, 40, 80); isRed(r, g, b, a) {
		t.Fatalf("outside box (40,80) is red — overflow:hidden 未裁剪")
	} else if a != 0 {
		t.Fatalf("outside box (40,80) = %02X%02X%02X%02X, want transparent (body ends at 60)", r, g, b, a)
	}
}

func TestScrollOffsetClampsAndShifts(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body{margin:0}
#box{width:100px;height:80px;overflow:auto;background-color:#FFFFFF}
#tall{height:400px;background-color:#FF0000}
</style></head><body><div id="box"><div id="tall"></div></div></body></html>`
	doc, buf := openScrollDoc(t, 200, 200, src)

	box := inner(doc.QuerySelector("#box"))
	tall := inner(doc.QuerySelector("#tall"))
	if box == nil || tall == nil {
		t.Fatal("elements not found")
	}
	if box.contentH != 400 {
		t.Fatalf("box.contentH = %d, want 400（未裁剪内容高应进 contentH）", box.contentH)
	}

	// 超量偏移：布局后夹紧到内容高 - 盒高
	box.scrollTop = 10000
	LayoutDocument(doc)
	if box.scrollTop != 320 {
		t.Fatalf("scrollTop = %d, want 320（400-80 夹紧失败）", box.scrollTop)
	}
	if tall.y != -320 {
		t.Fatalf("tall.y = %d, want -320（子树未随滚动平移）", tall.y)
	}
	if top := doc.QuerySelector("#tall").GetBoundingClientRect().Top().Pixel(); top != -320 {
		t.Fatalf("tall rect top = %d, want -320", top)
	}

	// 平移后的内容仍被裁剪盒包住：盒内尾部应是红色
	RenderNode(buf, doc)
	if r, g, b, a := at(t, buf, 40, 60); !isRed(r, g, b, a) {
		t.Fatalf("inside (40,60) = %02X%02X%02X%02X, want red after scroll", r, g, b, a)
	}
	// 盒外仍无红色
	if r, g, b, a := at(t, buf, 40, 100); isRed(r, g, b, a) {
		t.Fatalf("outside (40,100) is red — 滚动后裁剪丢失")
	}
}

func TestOverflowHiddenNotScrollable(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body{margin:0}
#box{width:100px;height:80px;overflow:hidden}
#tall{height:400px}
</style></head><body><div id="box"><div id="tall"></div></div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)

	box := inner(doc.QuerySelector("#box"))
	box.scrollTop = 500
	LayoutDocument(doc)
	if box.scrollTop != 0 {
		t.Fatalf("scrollTop = %d, want 0（overflow:hidden 只裁不滚）", box.scrollTop)
	}
	// 滚轮在该容器上也消费不了位移（短页面无文档级滚动）
	if OnDocumentWheel(doc, 50, 40, 0, 40) {
		t.Fatal("overflow:hidden 容器不应消费滚轮位移")
	}
}

func TestOnDocumentWheelDocumentScroll(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body{margin:0}
#page{height:1000px;background-color:#2563EB}
</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)

	root := inner(doc)
	if root.contentH != 1000 {
		t.Fatalf("root.contentH = %d, want 1000", root.contentH)
	}

	var got *MouseEvent
	doc.QuerySelector("#page").OnWheel(func(ev *MouseEvent) { got = ev })

	if !OnDocumentWheel(doc, 100, 100, 0, 40) {
		t.Fatal("长页面应发生文档级滚动")
	}
	if root.scrollTop != 40 {
		t.Fatalf("root.scrollTop = %d, want 40", root.scrollTop)
	}
	if got == nil {
		t.Fatal("wheel 处理器未触发")
	}
	if got.Type != EventWheel || got.DeltaY != 40 || got.DeltaX != 0 || got.X != 100 || got.Y != 100 {
		t.Fatalf("wheel ev = type=%s delta=(%d,%d) at (%d,%d), want wheel (0,40) at (100,100)",
			got.Type, got.DeltaX, got.DeltaY, got.X, got.Y)
	}

	LayoutDocument(doc)
	if top := doc.QuerySelector("#page").GetBoundingClientRect().Top().Pixel(); top != -40 {
		t.Fatalf("page rect top = %d, want -40（文档滚动未平移内容）", top)
	}

	// 超量：夹紧到 contentH - 视口高
	OnDocumentWheel(doc, 100, 100, 0, 100000)
	if root.scrollTop != 800 {
		t.Fatalf("clamped scrollTop = %d, want 800", root.scrollTop)
	}
	// 反向回滚
	OnDocumentWheel(doc, 100, 100, 0, -100000)
	if root.scrollTop != 0 {
		t.Fatalf("scrollTop after up-scroll = %d, want 0", root.scrollTop)
	}

	// 短页面：无可滚空间
	const short = `<!doctype html><html><head><style>body{margin:0}</style></head><body><div style="height:50px">x</div></body></html>`
	doc2, _ := openScrollDoc(t, 200, 200, short)
	if OnDocumentWheel(doc2, 50, 50, 0, 40) {
		t.Fatal("短页面不应滚动")
	}
	if inner(doc2).scrollTop != 0 {
		t.Fatal("短页面 scrollTop 应保持 0")
	}
	if OnDocumentWheel(nil, 0, 0, 0, 40) {
		t.Fatal("nil doc 应返回 false")
	}
}

func TestOnDocumentWheelChaining(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body{margin:0}
#wrap{height:600px;background-color:#EEEEEE}
#sc{width:120px;height:60px;overflow:auto;background-color:#FFFFFF}
#tall{height:300px;background-color:#00FF00}
</style></head><body>
<div id="wrap"><div id="sc"><div id="tall"></div></div></div>
</body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)

	root := inner(doc)
	sc := inner(doc.QuerySelector("#sc"))
	if sc == nil {
		t.Fatal("#sc not found")
	}

	// 一次超量滚轮：容器先吃满 240（300-60），余量 160 继续冒泡到文档级
	if !OnDocumentWheel(doc, 60, 30, 0, 400) {
		t.Fatal("expected scroll")
	}
	if sc.scrollTop != 240 {
		t.Fatalf("sc.scrollTop = %d, want 240（容器未吃满）", sc.scrollTop)
	}
	if root.scrollTop != 160 {
		t.Fatalf("root.scrollTop = %d, want 160（余量未滚动文档）", root.scrollTop)
	}

	// 容器已到边界：新位移全部落到文档级（scroll chaining）
	if !OnDocumentWheel(doc, 60, 30, 0, 1) {
		t.Fatal("expected doc scroll at container boundary")
	}
	if sc.scrollTop != 240 {
		t.Fatalf("sc.scrollTop = %d, want 240（边界外不应再动）", sc.scrollTop)
	}
	if root.scrollTop != 161 {
		t.Fatalf("root.scrollTop = %d, want 161", root.scrollTop)
	}

	// 两层滚动叠乘：tall 相对视口 = -容器滚动 - 文档滚动
	LayoutDocument(doc)
	if top := doc.QuerySelector("#tall").GetBoundingClientRect().Top().Pixel(); top != -401 {
		t.Fatalf("tall rect top = %d, want -401（两层滚动未叠加）", top)
	}
	if top := doc.QuerySelector("#wrap").GetBoundingClientRect().Top().Pixel(); top != -161 {
		t.Fatalf("wrap rect top = %d, want -161", top)
	}
}

func TestOnDocumentWheelHorizontal(t *testing.T) {
	// A) overflow-x 容器
	const srcA = `<!doctype html><html><head><style>
body{margin:0}
#hz{width:100px;height:50px;overflow-x:auto;overflow-y:hidden;background-color:#FFFFFF}
#wide{width:400px;height:30px;background-color:#FF0000}
</style></head><body><div id="hz"><div id="wide"></div></div></body></html>`
	docA, _ := openScrollDoc(t, 200, 200, srcA)
	hz := inner(docA.QuerySelector("#hz"))
	rootA := inner(docA)

	if !OnDocumentWheel(docA, 50, 25, 40, 0) {
		t.Fatal("overflow-x:auto 应消费横向滚轮")
	}
	if hz.scrollLeft != 40 {
		t.Fatalf("hz.scrollLeft = %d, want 40", hz.scrollLeft)
	}
	if rootA.scrollLeft != 0 {
		t.Fatalf("root.scrollLeft = %d, want 0（容器已消费，不应外溢）", rootA.scrollLeft)
	}
	// overflow-y:hidden：纵向位移消费不了
	if OnDocumentWheel(docA, 50, 25, 0, 40) {
		t.Fatal("overflow-y:hidden 不应消费纵向位移")
	}
	// 横向夹紧
	OnDocumentWheel(docA, 50, 25, 100000, 0)
	if hz.scrollLeft != 300 {
		t.Fatalf("hz.scrollLeft = %d, want 300（400-100 夹紧失败）", hz.scrollLeft)
	}
	LayoutDocument(docA)
	if left := docA.QuerySelector("#wide").GetBoundingClientRect().Left().Pixel(); left != -300 {
		t.Fatalf("wide rect left = %d, want -300（子树未随横向滚动平移）", left)
	}

	// B) 文档级横向滚动：宽块溢出视口，contentW 链式传播到根
	const srcB = `<!doctype html><html><head><style>
body{margin:0}
#wide{width:600px;height:100px;background-color:#0000FF}
</style></head><body><div id="wide"></div></body></html>`
	docB, _ := openScrollDoc(t, 200, 200, srcB)
	rootB := inner(docB)
	if rootB.contentW != 600 {
		t.Fatalf("root.contentW = %d, want 600（宽块内容未传播到根，文档级横滚失效）", rootB.contentW)
	}
	if !OnDocumentWheel(docB, 100, 50, 40, 0) {
		t.Fatal("宽页面应发生文档级横向滚动")
	}
	if rootB.scrollLeft != 40 {
		t.Fatalf("root.scrollLeft = %d, want 40", rootB.scrollLeft)
	}
	// 纵向无可滚（内容 100 < 视口 200）
	if OnDocumentWheel(docB, 100, 50, 0, 40) {
		t.Fatal("短高页面不应纵向滚动")
	}
}

func TestOnDocumentWheelTextarea(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString("row")
		b.WriteByte(byte('A' + i%26))
		b.WriteByte('\n')
	}
	const srcPrefix = `<!doctype html><html><head><style>
body{margin:0}
</style></head><body><textarea id="ta" style="width:120px;height:50px">`
	doc, _ := openScrollDoc(t, 200, 200, srcPrefix+b.String()+`</textarea></body></html>`)

	ta := doc.QuerySelector("#ta")
	base := inner(ta)
	if base == nil {
		t.Fatal("textarea not found")
	}
	r := ta.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	if !OnDocumentWheel(doc, cx, cy, 0, 40) {
		t.Fatal("textarea 应消费纵向滚轮")
	}
	if base.scrollTop != 40 {
		t.Fatalf("textarea scrollTop = %d, want 40", base.scrollTop)
	}

	// 夹紧口径 = 行数×行高 - 内容高（与 layoutTextarea 一致）
	l := layoutTextarea(docGraphics(doc), base)
	max := len(l.lines)*l.lh - base.height
	if max <= 40 {
		t.Fatalf("test fixture: max scroll %d should exceed 40", max)
	}
	OnDocumentWheel(doc, cx, cy, 0, 100000)
	if base.scrollTop != max {
		t.Fatalf("textarea scrollTop = %d, want %d（夹紧失败）", base.scrollTop, max)
	}
	// 布局不得清零原子控件的滚动量
	LayoutDocument(doc)
	if base.scrollTop != max {
		t.Fatalf("scrollTop after layout = %d, want %d（applyScrollOffsets 清掉了 textarea 滚动）", base.scrollTop, max)
	}
}

// TestWheelOverTextareaNeverScrollsDocument：可滚长页上滚 textarea，
// 位移必须全量被 textarea 吸收——含滚到边界之后（不链式传给文档）。
// 回归用户报告："滚动 textarea 时整个 document 也在滚动"（边界处剩余
// 位移经 scroll chaining 落到文档级滚动）。
func TestWheelOverTextareaNeverScrollsDocument(t *testing.T) {
	data, err := os.ReadFile("../demo/scroll.html")
	if err != nil {
		t.Fatalf("read demo/scroll.html: %v", err)
	}
	doc, _ := openScrollDoc(t, 1024, 720, string(data))
	root := inner(doc)
	if root.contentH <= 720 {
		t.Fatalf("fixture: root.contentH = %d, want > 720（页面应可滚动）", root.contentH)
	}
	ta := doc.QuerySelector("#ta")
	if ta == nil {
		t.Fatal("demo 页缺少 #ta textarea")
	}
	base := inner(ta)
	r := ta.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	// 远超 textarea 可滚上限的档位数：前段滚 textarea、后段撞边界，
	// 任何一档都不得推动文档。
	l := layoutTextarea(docGraphics(doc), base)
	max := len(l.lines)*l.lh - base.height
	notches := max/40 + 5
	for i := 1; i <= notches; i++ {
		OnDocumentWheel(doc, cx, cy, 0, 40)
		if root.scrollTop != 0 {
			t.Fatalf("第 %d 档后 root.scrollTop = %d, want 0（文档被链式滚动）；textarea.scrollTop = %d/%d",
				i, root.scrollTop, base.scrollTop, max)
		}
	}
	if base.scrollTop != max {
		t.Fatalf("textarea.scrollTop = %d, want 满滚 %d（撞边界前应全量消费）", base.scrollTop, max)
	}
}

func TestBufferClipStack(t *testing.T) {
	buf := NewBufferGraphics(20, 20)

	buf.PushClip(0, 0, 10, 20)
	buf.DrawColor(0, 0, 20, 20, NewColor(255, 0, 0, 255))
	if r, g, b, a := at(t, buf, 5, 5); !isRed(r, g, b, a) {
		t.Fatalf("(5,5) = %02X%02X%02X%02X, want red inside clip", r, g, b, a)
	}
	if r, g, b, a := at(t, buf, 15, 5); !isNone(r, g, b, a) {
		t.Fatalf("(15,5) = %02X%02X%02X%02X, want untouched outside clip", r, g, b, a)
	}

	// 嵌套求交：5..10
	buf.PushClip(5, 0, 20, 20)
	buf.DrawColor(0, 0, 20, 20, NewColor(0, 255, 0, 255))
	if r, g, b, a := at(t, buf, 7, 5); r != 0 || g != 255 || b != 0 || a != 255 {
		t.Fatalf("(7,5) = %02X%02X%02X%02X, want green in intersection", r, g, b, a)
	}
	if r, g, b, a := at(t, buf, 3, 5); !isRed(r, g, b, a) {
		t.Fatalf("(3,5) = %02X%02X%02X%02X, want red preserved (outside nested clip)", r, g, b, a)
	}
	if r, g, b, a := at(t, buf, 12, 5); !isNone(r, g, b, a) {
		t.Fatalf("(12,5) = %02X%02X%02X%02X, want untouched", r, g, b, a)
	}
	buf.PopClip()

	// 回到 0..10
	buf.DrawColor(0, 0, 20, 20, NewColor(0, 0, 255, 255))
	if r, g, b, a := at(t, buf, 7, 5); r != 0 || g != 0 || b != 255 || a != 255 {
		t.Fatalf("(7,5) = %02X%02X%02X%02X, want blue after pop", r, g, b, a)
	}
	buf.PopClip()

	// 栈空：不再裁剪
	buf.DrawColor(12, 12, 4, 4, NewColor(255, 255, 0, 255))
	if r, g, b, a := at(t, buf, 13, 13); r != 255 || g != 255 || b != 0 || a != 255 {
		t.Fatalf("(13,13) = %02X%02X%02X%02X, want yellow after pop to empty stack", r, g, b, a)
	}
	// 多余 PopClip 无害
	buf.PopClip()
	buf.DrawColor(14, 14, 2, 2, NewColor(0, 0, 0, 255))
	if r, g, b, a := at(t, buf, 15, 15); r != 0 || g != 0 || b != 0 || a != 255 {
		t.Fatalf("(15,15) = %02X%02X%02X%02X, want black", r, g, b, a)
	}
}

func TestElementAtClipsChildren(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body{margin:0}
#cp{width:60px;height:60px;border:5px solid #000000;overflow:hidden}
#wide{width:300px;height:60px;background-color:#00FF00}
</style></head><body><div id="cp"><div id="wide"></div></div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)

	// 边框环内：命中裁剪盒自身，子树（视觉已裁掉）不可命中
	if el := ElementAt(doc, 2, 2); el == nil || el.ID() != "cp" {
		t.Fatalf("ElementAt(2,2) = %v, want #cp（边框可点、子树应被裁剪拦截）", el)
	}
	// 裁剪区内：正常命中溢出子
	if el := ElementAt(doc, 30, 30); el == nil || el.ID() != "wide" {
		t.Fatalf("ElementAt(30,30) = %v, want #wide", el)
	}
	// 裁剪盒外：宽子虽在几何上覆盖该点，但已被裁掉，不可命中
	if el := ElementAt(doc, 150, 30); el == nil || el.TagName() != "body" {
		tag := "<nil>"
		if el != nil {
			tag = el.TagName() + "#" + el.ID()
		}
		t.Fatalf("ElementAt(150,30) = %s, want body（溢出子不应可点）", tag)
	}
}

func TestPositionedChildClippedByAncestor(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body{margin:0}
#box{position:relative;width:60px;height:60px;overflow:hidden;background-color:#FFFFFF}
#abs{position:absolute;left:40px;top:10px;width:40px;height:40px;background-color:#FF0000}
</style></head><body>
<div id="box"><div id="abs"></div></div>
</body></html>`
	_, buf := openScrollDoc(t, 200, 200, src)

	// 定位层在盒内部分可见
	if r, g, b, a := at(t, buf, 50, 30); !isRed(r, g, b, a) {
		t.Fatalf("(50,30) = %02X%02X%02X%02X, want red (inside clip)", r, g, b, a)
	}
	// 盒外部分必须被祖先裁剪（定位层若绕过裁剪会画出红块）
	if r, g, b, a := at(t, buf, 70, 30); isRed(r, g, b, a) {
		t.Fatalf("(70,30) is red — 定位层未受 overflow 祖先裁剪")
	}
}

func TestSVGClipPathForOverflow(t *testing.T) {
	const clipped = `<!doctype html><html><head><style>
body{margin:0}
#box{width:80px;height:60px;overflow:hidden}
#tall{height:300px}
</style></head><body><div id="box"><div id="tall"></div></div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, clipped)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, `<clipPath id="clip0"`) {
		t.Fatalf("SVG 缺少 clipPath 定义：\n%s", svg)
	}
	if !strings.Contains(svg, `clip-path="url(#clip0)"`) {
		t.Fatalf("SVG 缺少 clip-path 引用：\n%s", svg)
	}

	// 无 overflow 裁剪的页面不应产生任何 clipPath
	const plain = `<!doctype html><html><head><style>body{margin:0}</style></head><body><div style="height:50px">x</div></body></html>`
	doc2, _ := openScrollDoc(t, 200, 200, plain)
	if s := doc2.DumpSVG(); strings.Contains(s, "clipPath") {
		t.Fatalf("普通页面不应有 clipPath：\n%s", s)
	}
}

// TestRenderClipStackBalanced 渲染任意页面后裁剪栈必须回到 0：push 与
// pop 严格配对——非 Clipper 后端、退化盒（padding 盒 w/h ≤ 0）跳过压栈
// 时也不得出栈，否则会弹掉外层裁剪，滚动容器互相污染。
func TestRenderClipStackBalanced(t *testing.T) {
	cases := map[string]string{
		"嵌套滚动容器": `<!doctype html><html><head><style>
body{margin:0}
#outer{width:120px;height:80px;overflow:auto}
#inner{width:300px;height:200px;overflow:hidden}
#deep{position:absolute;left:50px;top:50px;width:400px;height:400px}
</style></head><body>
<div id="outer"><div id="inner"><div id="deep">x</div></div></div>
</body></html>`,
		// 内容盒 0×0 + 边框：padding 盒 w/h = 0，压栈必须整体跳过
		//（pushClip 返回 false → 不得配对 popClip）。
		"零尺寸裁剪盒": `<!doctype html><html><head><style>
body{margin:0}
#z{width:0;height:0;overflow:hidden;border:2px solid #000000}
#z>div{width:100px;height:100px}
</style></head><body><div id="z"><div>x</div></div></body></html>`,
		// textarea 走 UA overflow:hidden，自身是裁剪盒（含光标分支）
		"textarea控件": `<!doctype html><html><head><style>body{margin:0}
#t{overflow:visible}</style></head><body>
<textarea id="t" style="width:120px;height:50px">line1
line2
line3</textarea>
</body></html>`,
		// overflow:visible 的祖先里嵌裁剪盒 + 定位层：可见盒不压栈
		"可见盒套裁剪盒": `<!doctype html><html><head><style>
body{margin:0}
#v{width:200px;height:150px;overflow:visible}
#c{width:100px;height:60px;overflow:auto}
#p{position:absolute;left:10px;top:10px;width:500px;height:8px;background-color:#FF0000}
</style></head><body>
<div id="v"><div id="c"><div id="p"></div><div style="height:400px">tall</div></div></div>
</body></html>`,
	}
	for name, src := range cases {
		_, buf := openScrollDoc(t, 300, 300, src)
		if n := len(buf.clips); n != 0 {
			t.Errorf("%s：渲染后裁剪栈剩 %d 层（push/pop 失衡），栈顶=%+v",
				name, n, buf.clips[n-1])
		}
	}
}
