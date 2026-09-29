package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// radio/checkbox 页面：第一行勾选控件（含 label 包裹与 for 指向两种关联），
// 第二行 radio 覆盖三组情形——同名 g、异名 h、无 name（实测各自独立）。
const checkPage = `<!doctype html><html><head><style>
body { margin: 0; }
#row2 { margin-top: 8px; }
</style></head><body>
<input type="checkbox" id="c">
<label id="lw" style="margin-left:8px"><input type="checkbox" id="cw"> 包裹</label>
<label for="cf" id="lf" style="margin-left:8px">指向</label><input type="checkbox" id="cf">
<div id="row2">
	<input type="radio" name="g" id="r1" checked><input type="radio" name="g" id="r2">
	<input type="radio" name="h" id="h1">
	<input type="radio" id="n1"><input type="radio" id="n2">
</div>
</body></html>`

// clickCenter 点击元素盒子中心。
func clickCenter(t *testing.T, doc engine.HTMLDocument, sel string) {
	t.Helper()
	x, y := focusXY(t, doc, sel)
	engine.OnDocumentClick(doc, x, y)
}

// 点击切换 checkbox：click 冒泡、change 在冒泡之后补发、状态可再翻回。
func TestCheckboxClickToggle(t *testing.T) {
	doc := openDoc(t, 400, 300, checkPage)
	c := mustEl(t, doc, "#c")
	if engine.IsChecked(c) {
		t.Fatal("初始为选中（无 checked 属性）")
	}

	var clicks, changes, bodyChanges int
	c.OnClick(func(*engine.MouseEvent) { clicks++ })
	c.OnChange(func(*engine.MouseEvent) { changes++ })
	doc.Body().OnChange(func(*engine.MouseEvent) { bodyChanges++ })

	clickCenter(t, doc, "#c")
	if !engine.IsChecked(c) {
		t.Fatal("点击后 IsChecked = false, want true（默认动作未执行）")
	}
	if clicks != 1 || changes != 1 || bodyChanges != 1 {
		t.Fatalf("click=%d change=%d bodyChange=%d, want 1/1/1（冒泡或 change 补发缺失）",
			clicks, changes, bodyChanges)
	}

	clickCenter(t, doc, "#c")
	if engine.IsChecked(c) {
		t.Fatal("再次点击仍选中（切换失效）")
	}
	if clicks != 2 || changes != 2 || bodyChanges != 2 {
		t.Fatalf("第二次点击 click=%d change=%d bodyChange=%d, want 2/2/2",
			clicks, changes, bodyChanges)
	}
}

// radio 按 name 分组互斥：同名互斥、已选中项点击不取消、异名与无 name 独立。
func TestRadioGroupMutualExclusion(t *testing.T) {
	doc := openDoc(t, 400, 300, checkPage)
	r1 := mustEl(t, doc, "#r1")
	r2 := mustEl(t, doc, "#r2")
	h1 := mustEl(t, doc, "#h1")
	n1 := mustEl(t, doc, "#n1")
	n2 := mustEl(t, doc, "#n2")

	if !engine.IsChecked(r1) || engine.IsChecked(r2) {
		t.Fatalf("初始 r1=%v r2=%v, want true/false（checked 属性未生效）",
			engine.IsChecked(r1), engine.IsChecked(r2))
	}

	var r2Changes int
	r2.OnChange(func(*engine.MouseEvent) { r2Changes++ })

	// 点同组另一项 → 我上他下
	clickCenter(t, doc, "#r2")
	if engine.IsChecked(r1) || !engine.IsChecked(r2) {
		t.Fatalf("点 r2 后 r1=%v r2=%v, want false/true（分组未互斥）",
			engine.IsChecked(r1), engine.IsChecked(r2))
	}
	if r2Changes != 1 {
		t.Fatalf("change = %d, want 1", r2Changes)
	}

	// 再点已选中项 → 状态不变、不补发 change（radio 不可点灭）
	clickCenter(t, doc, "#r2")
	if !engine.IsChecked(r2) {
		t.Fatal("已选中 radio 被点击取消（浏览器不可点灭）")
	}
	if r2Changes != 1 {
		t.Fatalf("change = %d, want 1（无变化不应补发 change）", r2Changes)
	}

	// 异名组互不影响
	clickCenter(t, doc, "#h1")
	if !engine.IsChecked(h1) || !engine.IsChecked(r2) {
		t.Fatalf("h1=%v r2=%v, want true/true（异名组被误互斥）",
			engine.IsChecked(h1), engine.IsChecked(r2))
	}

	// 无 name 的 radio 各自独立（Chrome 实测口径）
	clickCenter(t, doc, "#n1")
	clickCenter(t, doc, "#n2")
	if !engine.IsChecked(n1) || !engine.IsChecked(n2) {
		t.Fatalf("n1=%v n2=%v, want true/true（无 name 被误并入同组）",
			engine.IsChecked(n1), engine.IsChecked(n2))
	}
}

// label 关联激活：包裹型与 for 指向都等效点控件；直接点控件只触发一次切换
// （否则控件与外层 label 双重激活会互相抵消）。
func TestLabelClickActivatesControl(t *testing.T) {
	doc := openDoc(t, 400, 300, checkPage)
	cw := mustEl(t, doc, "#cw")
	cf := mustEl(t, doc, "#cf")

	// 包裹型：点 label 文本（控件盒之外的右端）
	lw := mustEl(t, doc, "#lw").GetBoundingClientRect()
	engine.OnDocumentClick(doc, lw.Right().Pixel()-2,
		(lw.Top().Pixel()+lw.Bottom().Pixel())/2)
	if !engine.IsChecked(cw) {
		t.Fatal("点包裹 label 文本未激活其控件")
	}

	// 直接点控件：只切换一次（点两次 = 先灭后亮，双激活会看不出差异）
	clickCenter(t, doc, "#cw")
	if engine.IsChecked(cw) {
		t.Fatal("直接点控件切换了 0 或 2 次（与 label 双重激活）")
	}
	clickCenter(t, doc, "#cw")
	if !engine.IsChecked(cw) {
		t.Fatal("直接点控件第二次未切回")
	}

	// for 指向：点 label 文本激活目标控件
	lf := mustEl(t, doc, "#lf").GetBoundingClientRect()
	engine.OnDocumentClick(doc, (lf.Left().Pixel()+lf.Right().Pixel())/2,
		(lf.Top().Pixel()+lf.Bottom().Pixel())/2)
	if !engine.IsChecked(cf) {
		t.Fatal("点 for=label 未激活目标控件")
	}
	clickCenter(t, doc, "#cf")
	if engine.IsChecked(cf) {
		t.Fatal("直接点 for 目标控件未切换（双重激活）")
	}
}

const checkPixelPage = `<!doctype html><html><head><style>
body { margin: 0; }
</style></head><body>
<input type="checkbox" id="c">
<input type="radio" name="g" id="r1" checked>
<input type="radio" name="g" id="r2">
</body></html>`

// 像素级外观：未选中 = UA 白底灰边；checkbox 选中 = 强调色方块 + 白色对勾；
// radio 选中 = 强调色圆环 + 留白 + 实心内圆（口径见 check.go）。
func TestCheckedRenderingPixels(t *testing.T) {
	buf, doc := openBuffered(t, 400, 200, checkPixelPage)
	c := mustEl(t, doc, "#c")
	r1 := mustEl(t, doc, "#r1")
	r2 := mustEl(t, doc, "#r2")

	box := func(el engine.HTMLElement) (int, int) {
		rc := el.GetBoundingClientRect()
		return rc.Left().Pixel(), rc.Top().Pixel()
	}
	accent := func(tag string, x, y int) {
		t.Helper()
		if r, g, b, a := colorAt(t, buf, x, y); r != 37 || g != 99 || b != 235 || a != 255 {
			t.Fatalf("%s 像素(%d,%d) = %02X%02X%02X%02X, want 2563EBFF（强调色未画出）",
				tag, x, y, r, g, b, a)
		}
	}
	white := func(tag string, x, y int) {
		t.Helper()
		if r, g, b, a := colorAt(t, buf, x, y); r != 255 || g != 255 || b != 255 || a != 255 {
			t.Fatalf("%s 像素(%d,%d) = %02X%02X%02X%02X, want FFFFFFFF",
				tag, x, y, r, g, b, a)
		}
	}

	// 未选中：白底 + #767676 边框
	cx, cy := box(c)
	white("未选中 checkbox 中心", cx+6, cy+6)
	if r, g, b, a := colorAt(t, buf, cx, cy+6); r != 0x76 || g != 0x76 || b != 0x76 || a != 255 {
		t.Fatalf("未选中 checkbox 左边框 = %02X%02X%02X%02X, want 767676FF", r, g, b, a)
	}

	// radio 初始选中：中心强调色、留白为纯白、外环强调色（轴向 4/5/6px）
	r1x, r1y := box(r1)
	accent("radio 中心", r1x+6, r1y+6)
	white("radio 留白", r1x+2, r1y+6) // 距中心 4px，落在留白带内
	if r, g, b, _ := colorAt(t, buf, r1x+1, r1y+6); b < 200 || r > 60 || g > 120 {
		t.Fatalf("radio 外环(5px) = %02X%02X%02X, want 蓝色实环", r, g, b)
	}
	if r, g, b, _ := colorAt(t, buf, r1x, r1y+6); r != 0x25 || g != 0x63 || b != 0xEB {
		t.Fatalf("radio 外环(6px) = %02X%02X%02X, want 2563EB（环覆盖 UA 灰边）", r, g, b)
	}

	// checkbox 勾选：铺强调色 + 对勾折线白点
	engine.SetChecked(c, true)
	engine.RenderNode(buf, doc)
	accent("选中 checkbox", cx+6, cy+6)
	white("对勾起点", cx+3, cy+7)
	white("对勾拐点", cx+5, cy+9)
	white("对勾终点", cx+9, cy+3)
	if r, g, b, _ := colorAt(t, buf, cx, cy+6); r != 0x25 || g != 0x63 || b != 0xEB {
		t.Fatalf("选中 checkbox 左边框 = %02X%02X%02X, want 2563EB（边框被强调色覆盖）", r, g, b)
	}

	// radio 分组：SetChecked 置位 r2 会取消同组 r1，再渲染即 r2 有环、r1 变白
	engine.SetChecked(r2, true)
	if engine.IsChecked(r1) {
		t.Fatal("SetChecked(r2,true) 未取消同组 r1")
	}
	engine.RenderNode(buf, doc)
	r2x, r2y := box(r2)
	accent("r2 中心", r2x+6, r2y+6)
	white("r1 中心已复位", r1x+6, r1y+6)
}

// SVG 导出与渲染路径同构：选中时导出对勾折线 / 圆环，未选中不导出。
func TestSVGExportsCheckedMark(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<input type="checkbox" id="c"><input type="radio" name="g" id="r">
	</body></html>`
	doc := openDoc(t, 200, 100, src)
	if svg := doc.DumpSVG(); strings.Contains(svg, `<path `) || strings.Contains(svg, `<circle `) {
		t.Fatalf("未选中导出不应含标记：\n%s", svg)
	}
	engine.SetChecked(mustEl(t, doc, "#c"), true)
	engine.SetChecked(mustEl(t, doc, "#r"), true)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, `<path d="M `) {
		t.Fatalf("选中 checkbox 未导出对勾折线：\n%s", svg)
	}
	if n := strings.Count(svg, `<circle `); n != 2 {
		t.Fatalf("选中 radio 的 circle 数 = %d, want 2（外环 + 内圆）：\n%s", n, svg)
	}
}
