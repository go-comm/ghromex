package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 左浮动：后续行盒按浮动右缘收缩（文字绕排），父级 auto 高度不含浮动。
func TestFloatLeftShiftsLineAndExcludesHeight(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; }
		#box { width: 300px; }
		#f { float: left; width: 100px; height: 40px; }
		#p { margin: 0; padding: 0; font-size: 16px; line-height: 20px; }
	</style></head><body><div id="box"><div id="f"></div><p id="p"><span id="s">hello</span></p></div></body></html>`
	_, doc := openBuffered(t, 400, 300, src)
	box := mustEl(t, doc, "#box").GetBoundingClientRect()
	f := mustEl(t, doc, "#f").GetBoundingClientRect()
	s := mustEl(t, doc, "#s").GetBoundingClientRect()

	if got := f.Right().Pixel() - box.Left().Pixel(); got != 100 {
		t.Fatalf("浮动宽 = %d, want 100（shrink-to-fit 失败）", got)
	}
	if s.Left().Pixel() < box.Left().Pixel()+100 {
		t.Fatalf("行盒起点 x = %d, want >= %d（未避让左浮动）",
			s.Left().Pixel(), box.Left().Pixel()+100)
	}
	// 父级 auto 高度只算流内内容（p 的一行 20px），浮动不撑高（Chrome 口径）。
	if got := box.Bottom().Pixel() - box.Top().Pixel(); got != 20 {
		t.Fatalf("父盒高 = %d, want 20（浮动不应计入 auto 高度）", got)
	}
}

// 右浮动：行盒右边界收缩，放不下的行内盒换行。
func TestFloatRightShrinksLine(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; }
		#box { width: 300px; }
		#f { float: right; width: 80px; height: 40px; }
		#p { margin: 0; padding: 0; font-size: 16px; line-height: 20px; }
		.blk { display: inline-block; width: 150px; height: 10px; }
	</style></head><body><div id="box"><div id="f"></div>
		<p id="p"><span id="a" class="blk"></span><span id="b" class="blk"></span></p></div></body></html>`
	_, doc := openBuffered(t, 400, 300, src)
	box := mustEl(t, doc, "#box").GetBoundingClientRect()
	a := mustEl(t, doc, "#a").GetBoundingClientRect()
	b := mustEl(t, doc, "#b").GetBoundingClientRect()

	if a.Left().Pixel() != box.Left().Pixel() {
		t.Fatalf("首个行内盒 x = %d, want %d", a.Left().Pixel(), box.Left().Pixel())
	}
	// 可用行宽 = 300-80 = 220：第二个 150px 盒放不下，必须换行回到左缘。
	if b.Left().Pixel() != box.Left().Pixel() {
		t.Fatalf("换行后 x = %d, want %d（未按右浮动收缩行宽）",
			b.Left().Pixel(), box.Left().Pixel())
	}
	if b.Top().Pixel() <= a.Top().Pixel() {
		t.Fatalf("第二个行内盒未换行: a.top=%d b.top=%d", a.Top().Pixel(), b.Top().Pixel())
	}
}

// clear:both 把块盒下推到浮动下缘之下。
func TestClearBothMovesBelowFloat(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; }
		#box { width: 300px; }
		#f { float: left; width: 50px; height: 60px; }
		#after { clear: both; height: 10px; }
	</style></head><body><div id="box"><div id="f"></div><div id="after"></div></div></body></html>`
	_, doc := openBuffered(t, 400, 300, src)
	box := mustEl(t, doc, "#box").GetBoundingClientRect()
	f := mustEl(t, doc, "#f").GetBoundingClientRect()
	after := mustEl(t, doc, "#after").GetBoundingClientRect()

	if after.Top().Pixel() < f.Bottom().Pixel() {
		t.Fatalf("clear:both 元素 top=%d, 浮动底=%d（未下移）",
			after.Top().Pixel(), f.Bottom().Pixel())
	}
	if after.Top().Pixel() != box.Top().Pixel()+60 {
		t.Fatalf("clear 后 top=%d, want %d", after.Top().Pixel(), box.Top().Pixel()+60)
	}
}

// visibility:hidden 的盒子照常占位（兄弟位置不变），但自身不命中；
// 子级显式 visible 可以恢复显示与命中。
func TestVisibilityHiddenOccupiesSpaceButNotHittable(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; }
		#h { visibility: hidden; width: 100px; height: 40px; }
		#next { height: 40px; }
		#inner { visibility: visible; width: 50px; height: 20px; }
	</style></head><body>
		<div id="h"><div id="inner"></div></div>
		<div id="next"></div>
	</body></html>`
	_, doc := openBuffered(t, 400, 300, src)
	h := mustEl(t, doc, "#h").GetBoundingClientRect()
	next := mustEl(t, doc, "#next").GetBoundingClientRect()
	inner := mustEl(t, doc, "#inner")

	if next.Top().Pixel() != h.Bottom().Pixel() {
		t.Fatalf("hidden 盒未占位: h.bottom=%d next.top=%d",
			h.Bottom().Pixel(), next.Top().Pixel())
	}
	if hit := engine.ElementAt(doc, h.Left().Pixel()+5, h.Top().Pixel()+30); hit == mustEl(t, doc, "#h") {
		t.Fatal("visibility:hidden 的元素不应作为命中目标")
	}
	// 子级显式 visible：它自己的盒仍可命中。
	if hit := engine.ElementAt(doc, h.Left().Pixel()+5, h.Top().Pixel()+5); hit != inner {
		t.Fatalf("子级 visibility:visible 命中 = %v, want #inner", hit)
	}
}

// CursorAt 跟随命中元素的计算值 cursor（UA 给 a 设 pointer、input 设 text）。
func TestCursorAtFollowsHitElement(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; }
		#link { display: block; width: 100px; height: 30px; }
		#inp { display: block; width: 100px; height: 20px; }
		#move { width: 100px; height: 30px; cursor: move; }
	</style></head><body>
		<a id="link" href="#">link</a>
		<input id="inp">
		<div id="move"></div>
	</body></html>`
	_, doc := openBuffered(t, 400, 300, src)
	link := mustEl(t, doc, "#link").GetBoundingClientRect()
	inp := mustEl(t, doc, "#inp").GetBoundingClientRect()
	move := mustEl(t, doc, "#move").GetBoundingClientRect()

	if got := engine.CursorAt(doc, link.Left().Pixel()+5, link.Top().Pixel()+5); got != engine.CursorPointer {
		t.Fatalf("a 上 cursor = %v, want pointer", got)
	}
	if got := engine.CursorAt(doc, inp.Left().Pixel()+5, inp.Top().Pixel()+5); got != engine.CursorText {
		t.Fatalf("input 上 cursor = %v, want text", got)
	}
	if got := engine.CursorAt(doc, move.Left().Pixel()+5, move.Top().Pixel()+5); got != engine.CursorMove {
		t.Fatalf("cursor:move 上 = %v, want move", got)
	}
}

// opacity 按子树绘制颜色的 alpha 缩放：0.5 的红底压在白底上 ≈ 粉色。
func TestOpacityScalesDrawnAlpha(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; background: #ffffff; }
		#a { width: 40px; height: 40px; background: #ff0000; opacity: 0.5; }
		#b { width: 40px; height: 40px; background: #ff0000; }
	</style></head><body><div id="a"></div><div id="b"></div></body></html>`
	buf, doc := openBuffered(t, 200, 200, src)
	a := mustEl(t, doc, "#a").GetBoundingClientRect()
	b := mustEl(t, doc, "#b").GetBoundingClientRect()

	r, g, bl, _ := colorAt(t, buf, a.Left().Pixel()+5, a.Top().Pixel()+5)
	if !(r >= 250 && g >= 110 && g <= 145 && bl >= 110 && bl <= 145) {
		t.Fatalf("opacity:0.5 像素 = (%d,%d,%d), want ≈(255,128,128)", r, g, bl)
	}
	// 对照：不透明盒仍是纯红。
	if r2, g2, b2, _ := colorAt(t, buf, b.Left().Pixel()+5, b.Top().Pixel()+5); r2 != 255 || g2 != 0 || b2 != 0 {
		t.Fatalf("不透明盒像素 = (%d,%d,%d), want (255,0,0)", r2, g2, b2)
	}
}

// 行盒排到右边界后，后续原子盒必须换行回到行首：旧实现里间隙空格
// 把游标顶出右界后，换行判断（right > it.x）永久失效，整段粘成一行溢出。
func TestInlineAfterFullLineWrapsToNextLine(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; }
		#p { width: 300px; font-size: 16px; }
		#a { display: inline-block; width: 300px; height: 10px; }
		#b { display: inline-block; width: 20px; height: 10px; }
	</style></head><body><p id="p"><span id="a"></span> <span id="b"></span></p></body></html>`
	_, doc := openBuffered(t, 400, 200, src)
	p := mustEl(t, doc, "#p").GetBoundingClientRect()
	a := mustEl(t, doc, "#a").GetBoundingClientRect()
	b := mustEl(t, doc, "#b").GetBoundingClientRect()

	if b.Left().Pixel() != p.Left().Pixel() {
		t.Fatalf("#b 左缘 = %d, want %d（满行后的原子盒未换行，溢出到行外）",
			b.Left().Pixel(), p.Left().Pixel())
	}
	if b.Top().Pixel() <= a.Top().Pixel() {
		t.Fatalf("#b 未换行: a.top=%d b.top=%d", a.Top().Pixel(), b.Top().Pixel())
	}
}

// 超宽原子盒本身溢出后，后续原子盒仍要回到下一行行首（不能因为游标已在
// 右界之外就放弃换行，否则剩余内容全部溢出）。
func TestInlineAfterOverwideBoxWrapsToNextLine(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; }
		#p { width: 300px; font-size: 16px; }
		#wide { display: inline-block; width: 400px; height: 10px; }
		#b { display: inline-block; width: 20px; height: 10px; }
	</style></head><body><p id="p"><span id="wide"></span><span id="b"></span></p></body></html>`
	_, doc := openBuffered(t, 500, 200, src)
	p := mustEl(t, doc, "#p").GetBoundingClientRect()
	wide := mustEl(t, doc, "#wide").GetBoundingClientRect()
	b := mustEl(t, doc, "#b").GetBoundingClientRect()

	if b.Left().Pixel() != p.Left().Pixel() {
		t.Fatalf("#b 左缘 = %d, want %d（超宽盒后的原子盒未换行）",
			b.Left().Pixel(), p.Left().Pixel())
	}
	if b.Top().Pixel() <= wide.Top().Pixel() {
		t.Fatalf("#b 未换行: wide.top=%d b.top=%d", wide.Top().Pixel(), b.Top().Pixel())
	}
}

// SVG 导出与屏幕渲染同构：斜体带 font-style、hidden 不导出、opacity 包组。
func TestDumpSVGReflectsItalicHiddenOpacity(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; }
		#i { font-style: italic; }
		#hid { visibility: hidden; }
		#op { opacity: 0.4; width: 20px; height: 20px; background: #00ff00; }
	</style></head><body>
		<p id="i">italic</p>
		<p id="hid">hidden-text</p>
		<div id="op"></div>
	</body></html>`
	vp := engine.NewHeadlessViewport(200, 200)
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatal(err)
	}
	doc.Refresh()
	svg := doc.DumpSVG()

	if !strings.Contains(svg, `font-style="italic"`) {
		t.Fatalf("SVG 未导出斜体:\n%s", svg)
	}
	if strings.Contains(svg, "hidden-text") {
		t.Fatalf("visibility:hidden 的文字被导出:\n%s", svg)
	}
	if !strings.Contains(svg, `<g opacity="0.40">`) {
		t.Fatalf("SVG 未按 opacity 包组:\n%s", svg)
	}
}
