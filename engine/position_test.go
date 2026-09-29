package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// openDoc 无头视口打开文档（FakeGraphics 布局，无需像素）。
func openDoc(t *testing.T, w, h int, src string) engine.HTMLDocument {
	t.Helper()
	vp := engine.NewHeadlessViewport(w, h)
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	return doc
}

func mustEl(t *testing.T, doc engine.HTMLDocument, sel string) engine.HTMLElement {
	t.Helper()
	e := doc.QuerySelector(sel)
	if e == nil {
		t.Fatalf("selector %s not found", sel)
	}
	return e
}

// 负值 inset（越界角标）：right/top/bottom/left 允许负值，锚定到包含块边缘之外；
// right/bottom 回推结果不得受 shrink 宽/盒当前原点影响。
func TestAbsoluteNegativeInset(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div class="card" style="position:relative;margin:14px 40px 0 40px;padding:20px 32px;border:1px solid #333;height:60px;background:#fff">
			<div id="br" style="position:absolute;right:-14px;top:-12px;padding:5px 10px 6px 10px;border:2px solid #fff;background:#d26;font-size:13px">13</div>
			<div id="bl" style="position:absolute;left:-10px;bottom:-8px;width:20px;height:10px;background:#08f"></div>
		</div>
	</body></html>`
	doc := openDoc(t, 200, 150, src)
	// CB（card padding box）= (40+1, 14+1, 120-2, 60+40) = (41,15,118,100)

	br := mustEl(t, doc, "#br")
	// top:-12 → border box 顶 = 15-12 = 3；content y = 3+bd2+pd5 = 10
	if br.Y() != 10 {
		t.Errorf("#br.Y() = %d, want 10（top:-12 未锚到 CB 顶外）", br.Y())
	}
	// right:-14 → border box 右缘 = 41+118+14 = 173，即 X()+Width()+pd10+bd2
	if got := br.X() + br.Width() + 12; got != 173 {
		t.Errorf("#br 右缘 = %d, want 173（right:-14 回推错，X=%d W=%d）", got, br.X(), br.Width())
	}

	bl := mustEl(t, doc, "#bl")
	if bl.X() != 31 {
		t.Errorf("#bl.X() = %d, want 31（left:-10）", bl.X())
	}
	if got := bl.Y() + bl.Height(); got != 123 {
		t.Errorf("#bl 底缘 = %d, want 123（bottom:-8）", got)
	}
}

// absolute 锚定：left/top 直锚，right/bottom 回推，fixed 以视口为包含块，
// 脱流盒不影响常规流兄弟。
func TestAbsolutePositioning(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div id="at" style="position:absolute;left:20px;top:30px;width:50px;height:10px"></div>
		<div id="rb" style="position:absolute;right:10px;bottom:8px;width:50px;height:10px"></div>
		<div id="fx" style="position:fixed;left:5px;bottom:0px;width:10px;height:5px"></div>
		<div id="flow" style="height:5px"></div>
	</body></html>`
	doc := openDoc(t, 200, 100, src)

	if e := mustEl(t, doc, "#at"); e.X() != 20 || e.Y() != 30 || e.Width() != 50 || e.Height() != 10 {
		t.Errorf("#at box = (%d,%d,%d,%d), want (20,30,50,10)", e.X(), e.Y(), e.Width(), e.Height())
	}
	// right/bottom：x = 200-10-50，y = 100-8-10
	if e := mustEl(t, doc, "#rb"); e.X() != 140 || e.Y() != 82 {
		t.Errorf("#rb = (%d,%d), want (140,82)", e.X(), e.Y())
	}
	// fixed：无视 static 父链，锚定视口
	if e := mustEl(t, doc, "#fx"); e.X() != 5 || e.Y() != 95 {
		t.Errorf("#fx = (%d,%d), want (5,95)", e.X(), e.Y())
	}
	// 脱流不占位：flow 兄弟仍在文档流原点
	if e := mustEl(t, doc, "#flow"); e.Y() != 0 {
		t.Errorf("#flow Y = %d, want 0（absolute/fixed 侵占了常规流）", e.Y())
	}
}

// relative 父级作为 absolute 的包含块（padding box 锚定）；
// absolute 的 auto 宽度按 shrink-to-fit（不同于块级的撑满）。
func TestAbsoluteContainingBlock(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div class="rel" style="position:relative;margin:40px 0 0 40px;width:100px;height:60px;padding:8px">
			<div id="inner" style="position:absolute;left:5px;top:6px;width:20px;height:10px"></div>
			<div id="rb2" style="position:absolute;right:6px;bottom:4px;width:20px;height:10px"></div>
			<div id="auto" style="position:absolute;left:0;top:0">HI</div>
		</div>
	</body></html>`
	doc := openDoc(t, 200, 150, src)

	// rel padding box 原点 = content(48,48) - padding(8,8) = (40,40)
	rel := mustEl(t, doc, ".rel")
	if rel.X() != 48 || rel.Y() != 48 {
		t.Fatalf(".rel content origin = (%d,%d), want (48,48)", rel.X(), rel.Y())
	}
	innerEl := mustEl(t, doc, "#inner")
	if x, y := innerEl.X(), innerEl.Y(); x != 45 || y != 46 {
		t.Errorf("#inner = (%d,%d), want (45,46)（包含块未取 relative 父 padding box）", x, y)
	}
	// 非零原点 CB 的 right/bottom 回推：目标右缘 = 40+116-6=150，底缘 = 40+76-4=112
	// （回归锁定：曾漏减盒子当前 ox/oy，导致偏移恰差一个 CB 原点）
	rb2 := mustEl(t, doc, "#rb2")
	if x, y := rb2.X(), rb2.Y(); x != 130 || y != 102 {
		t.Errorf("#rb2 = (%d,%d), want (130,102)（right/bottom 回推漏减盒子当前原点）", x, y)
	}
	// auto 宽收缩：应小于可用宽（116-padding 后仍远小于 200 满宽语义）
	if w := mustEl(t, doc, "#auto").Width(); w >= 100 {
		t.Errorf("#auto width = %d, want shrink-to-fit (<100)", w)
	}
}

// relative 偏移自身与子树，但不影响后续流兄弟的位置。
func TestRelativeOffset(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div style="height:20px"></div>
		<div id="r" style="position:relative;left:10px;top:5px;width:40px;height:10px">文字</div>
		<div id="sib" style="height:20px;width:5px"></div>
	</body></html>`
	doc := openDoc(t, 200, 100, src)

	r := mustEl(t, doc, "#r")
	// 流内位置 (0,20) + 偏移 (10,5)
	if x, y := r.X(), r.Y(); x != 10 || y != 25 {
		t.Errorf("#r = (%d,%d), want (10,25)", x, y)
	}
	if s := mustEl(t, doc, "#sib"); s.Y() != 30 {
		t.Errorf("#sib Y = %d, want 30（relative 偏移影响了常规流）", s.Y())
	}
}

// 层叠：定位层整体在静态内容之上；层内按 z-index 排序（文档序决胜）。
// 用 BufferGraphics 像素验证。
func TestPositionedStacking(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div id="staticbg" style="height:100px;background-color:#0000FF"></div>
		<div style="position:absolute;left:50px;top:50px;width:20px;height:20px;background-color:#000000"></div>
		<div id="red" style="position:absolute;left:20px;top:20px;width:60px;height:60px;background-color:#FF0000;z-index:1"></div>
		<div id="green" style="position:absolute;left:40px;top:40px;width:60px;height:60px;background-color:#00FF00;z-index:2"></div>
	</body></html>`
	buf, _ := openBuffered(t, 120, 120, src)

	// 静态蓝底未被覆盖处
	if r, g, b, _ := colorAt(t, buf, 5, 5); r != 0x00 || g != 0x00 || b != 0xFF {
		t.Errorf("static bg = %02X%02X%02X, want 0000FF", r, g, b)
	}
	// (25,25)：红 z1 覆盖蓝（定位层压静态层；黑块 z0 不在该区）
	if r, g, b, _ := colorAt(t, buf, 25, 25); r != 0xFF || g != 0x00 || b != 0x00 {
		t.Errorf("red zone = %02X%02X%02X, want FF0000（定位层未压住静态层？）", r, g, b)
	}
	// (55,55)：红/黑/绿三区叠加，绿 z 最高
	if r, g, b, _ := colorAt(t, buf, 55, 55); r != 0x00 || g != 0xFF || b != 0x00 {
		t.Errorf("overlap = %02X%02X%02X, want 00FF00（z-index 排序失效）", r, g, b)
	}

	// 交换 z-index 后叠区变红（文档序：红在前）
	const swap = `<!doctype html><html><head></head><body style="margin:0">
		<div id="red" style="position:absolute;left:20px;top:20px;width:60px;height:60px;background-color:#FF0000;z-index:2"></div>
		<div id="green" style="position:absolute;left:40px;top:40px;width:60px;height:60px;background-color:#00FF00;z-index:1"></div>
	</body></html>`
	buf2, _ := openBuffered(t, 120, 120, swap)
	if r, g, b, _ := colorAt(t, buf2, 55, 55); r != 0xFF || g != 0x00 || b != 0x00 {
		t.Errorf("swapped overlap = %02X%02X%02X, want FF0000", r, g, b)
	}

	// z 相同（默认 auto→0）：文档序后者覆盖前者
	const same = `<!doctype html><html><head></head><body style="margin:0">
		<div style="position:absolute;left:10px;top:10px;width:40px;height:40px;background-color:#FF0000"></div>
		<div style="position:absolute;left:20px;top:20px;width:40px;height:40px;background-color:#00FF00"></div>
	</body></html>`
	buf3, _ := openBuffered(t, 100, 100, same)
	if r, g, b, _ := colorAt(t, buf3, 30, 30); r != 0x00 || g != 0xFF || b != 0x00 {
		t.Errorf("same-z overlap = %02X%02X%02X, want 00FF00（文档序决胜失效）", r, g, b)
	}
}

// relative 带偏移的元素同样进定位层：盖在静态内容之上。
func TestRelativeStacksAboveStatic(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div style="height:40px;background-color:#0000FF"></div>
		<div id="rv" style="position:relative;top:-20px;width:40px;height:20px;background-color:#FF0000"></div>
	</body></html>`
	buf, doc := openBuffered(t, 120, 120, src)
	// rv 流内 y=40，偏移 -20 → y=20，盖在蓝带上
	if y := mustEl(t, doc, "#rv").Y(); y != 20 {
		t.Fatalf("rv Y = %d, want 20", y)
	}
	if r, g, b, _ := colorAt(t, buf, 10, 25); r != 0xFF || g != 0x00 || b != 0x00 {
		t.Errorf("relative over static = %02X%02X%02X, want FF0000", r, g, b)
	}
}
