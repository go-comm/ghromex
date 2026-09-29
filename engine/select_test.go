package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

const selectPage = `<!doctype html><html><head><style>
body { margin: 0; }
#s { display: block; width: 120px; height: 20px; }
.after { height: 60px; background: #eeeeee; }
</style></head><body>
<select id="s">
	<option value="a">Alpha</option>
	<option value="b" selected>Bravo</option>
	<option value="c">Charlie</option>
</select>
<div class="after">after</div>
</body></html>`

// 收起态：option 不进常规流（不占位、零尺寸不可命中），value 同步到 selected 项。
func TestSelectClosedKeepsFlowAndValue(t *testing.T) {
	doc := openDoc(t, 400, 300, selectPage)
	sel := mustEl(t, doc, "#s")

	for i, o := range doc.QuerySelectorAll("option") {
		if o.Width() != 0 || o.Height() != 0 {
			t.Fatalf("option[%d] 收起态盒 = %dx%d, want 0x0（选项进入了常规流？）",
				i, o.Width(), o.Height())
		}
	}
	if got := sel.GetBoundingClientRect().Bottom().Pixel(); got != 20 {
		t.Fatalf("select 底 = %d, want 20（高度被选项撑开）", got)
	}
	after := mustEl(t, doc, ".after")
	if got := after.GetBoundingClientRect().Top().Pixel(); got != 20 {
		t.Fatalf("后续块顶 = %d, want 20（选项占了位）", got)
	}

	if v := engine.SelectValue(sel); v != "b" {
		t.Fatalf("SelectValue = %q, want b（selected 未生效）", v)
	}
	if v := engine.GetValue(sel); v != "b" {
		t.Fatalf("GetValue = %q, want b", v)
	}
	// 收起态浮层区域应命中其下的内容，而不是 option
	if hit := engine.ElementAt(doc, 60, 30); hit.TagName() == "option" {
		t.Fatal("收起态 option 仍可命中")
	}
}

// 展开 → 浮层命中优先于其下流内容 → 点选项选中并收起 → change/click 冒泡 → 点空白收起。
func TestSelectOpenSelectAndClose(t *testing.T) {
	doc := openDoc(t, 400, 300, selectPage)
	sel := mustEl(t, doc, "#s")
	opts := doc.QuerySelectorAll("option")
	if len(opts) != 3 {
		t.Fatalf("option 数 = %d, want 3", len(opts))
	}

	var changes, clicks int
	sel.OnChange(func(ev *engine.MouseEvent) {
		changes++
		if v := engine.SelectValue(sel); v != "c" {
			t.Errorf("change 触发时 value = %q, want c", v)
		}
	})
	sel.OnClick(func(ev *engine.MouseEvent) { clicks++ })

	// 点 select 盒 → 展开
	engine.OnDocumentClick(doc, 60, 10)
	engine.LayoutDocument(doc)
	if opts[0].Height() == 0 {
		t.Fatal("展开后选项仍无盒")
	}
	if got := opts[0].GetBoundingClientRect().Top().Pixel(); got != 20 {
		t.Fatalf("首项顶 = %d, want 20（未排到 select 下缘）", got)
	}
	// 浮层压在其下的 .after 内容上，命中必须归 option
	hit := engine.ElementAt(doc, 60, 30)
	if hit.TagName() != "option" {
		t.Fatalf("浮层命中 = %s, want option（浮层未参与命中测试）", hit.TagName())
	}

	// 点第三项 → 收起 + 值变更 + change + click 冒泡
	engine.OnDocumentClick(doc, 60, 75)
	engine.LayoutDocument(doc)
	if v := engine.SelectValue(sel); v != "c" {
		t.Fatalf("选中后 value = %q, want c", v)
	}
	if changes != 1 {
		t.Fatalf("change 触发 %d 次, want 1", changes)
	}
	if clicks == 0 {
		t.Fatal("click 未冒泡到 select")
	}
	if opts[0].Height() != 0 {
		t.Fatal("选中后未收起")
	}
	if hit := engine.ElementAt(doc, 60, 30); hit.TagName() == "option" {
		t.Fatal("收起后浮层区域仍命中 option")
	}

	// 再次展开 → 点空白处收起
	engine.OnDocumentClick(doc, 60, 10)
	engine.LayoutDocument(doc)
	if opts[0].Height() == 0 {
		t.Fatal("再次展开失败")
	}
	engine.OnDocumentClick(doc, 300, 250)
	engine.LayoutDocument(doc)
	if opts[0].Height() != 0 {
		t.Fatal("点空白未收起")
	}
}

// 下方放不下且上方放得下 → 整组选项上移贴到 select 上缘。
func TestSelectPopupFlipsAbove(t *testing.T) {
	const src = `<!doctype html><html><head><style>
body { margin: 0; }
#sp { height: 100px; }
#s { position: absolute; top: 80px; left: 0; width: 100px; height: 20px; }
</style></head><body>
<div id="sp"></div>
<select id="s">
	<option value="a">Alpha</option>
	<option value="b">Bravo</option>
	<option value="c">Charlie</option>
</select>
</body></html>`
	doc := openDoc(t, 400, 100, src)
	if hit := engine.ElementAt(doc, 50, 90); hit == nil || hit.TagName() != "select" {
		t.Fatalf("hit(50,90) = %v, want select", hit)
	}
	engine.OnDocumentClick(doc, 50, 90) // select 盒 80..100
	engine.LayoutDocument(doc)

	opts := doc.QuerySelectorAll("option")
	if opts[0].Height() == 0 {
		t.Fatal("展开失败")
	}
	// 3 项 × 22px = 66 → 组顶 = 80-66 = 14
	top := opts[0].GetBoundingClientRect().Top().Pixel()
	bottom := opts[2].GetBoundingClientRect().Bottom().Pixel()
	if top != 14 || bottom != 80 {
		t.Fatalf("浮层 = [%d,%d], want [14,80]（未整体上移贴 select 上缘）", top, bottom)
	}
}

// 程序化赋值与 SVG 导出（浮层）与渲染保持同构。
func TestSetSelectValueAndSVGPopup(t *testing.T) {
	doc := openDoc(t, 400, 300, selectPage)
	sel := mustEl(t, doc, "#s")

	changes := 0
	sel.OnChange(func(ev *engine.MouseEvent) { changes++ })

	if !engine.SetSelectValue(sel, "c") {
		t.Fatal("SetSelectValue(c) 应成功")
	}
	if v := engine.SelectValue(sel); v != "c" {
		t.Fatalf("赋值后 value = %q, want c", v)
	}
	if changes != 1 {
		t.Fatalf("change 触发 %d 次, want 1", changes)
	}
	if engine.SetSelectValue(sel, "zzz") {
		t.Error("不存在的值应返回 false")
	}

	// 展开后 SVG 导出应含浮层容器与选项文字
	engine.OnDocumentClick(doc, 60, 10)
	engine.LayoutDocument(doc)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, `stroke="#767676"`) {
		t.Error("SVG 缺浮层容器边框")
	}
	for _, want := range []string{"Alpha", "Bravo", "Charlie"} {
		if !strings.Contains(svg, ">"+want+"</text>") {
			t.Errorf("SVG 缺选项文字 %s", want)
		}
	}
	// 收起态导出不应再含选项文字
	engine.OnDocumentClick(doc, 300, 250)
	engine.LayoutDocument(doc)
	if svg := doc.DumpSVG(); strings.Contains(svg, ">Alpha</text>") {
		t.Error("收起态 SVG 仍导出了选项文字")
	}
}

// 像素级校验：收起态的下拉箭头、展开浮层的底/高亮/描边（描边在高亮之上）。
func TestSelectPopupPixels(t *testing.T) {
	doc := openDoc(t, 400, 300, selectPage)
	buf := engine.NewBufferGraphics(400, 300)
	engine.RenderNode(buf, doc)

	check := func(x, y int, r, g, b uint8, what string) {
		t.Helper()
		pr, pg, pb, _, ok := buf.ColorAt(x, y)
		if !ok {
			t.Fatalf("%s: (%d,%d) 越界", what, x, y)
		}
		if pr != r || pg != g || pb != b {
			t.Errorf("%s: (%d,%d) = #%02x%02x%02x, want #%02x%02x%02x",
				what, x, y, pr, pg, pb, r, g, b)
		}
	}
	// 收起态：下拉箭头（倒三角首行 x110..116，取其行色），且不越出内容盒
	check(110, 8, 0x76, 0x76, 0x76, "箭头首行左端")
	check(113, 10, 0x76, 0x76, 0x76, "箭头末行")
	check(117, 9, 0xFF, 0xFF, 0xFF, "箭头右侧应为空白")
	check(119, 8, 0x76, 0x76, 0x76, "select 右边框不被箭头覆盖")

	// 展开浮层：容器底（Alpha 行）→ 选中项高亮（Bravo 行）→ 描边压在高亮上
	engine.OnDocumentClick(doc, 60, 10)
	engine.LayoutDocument(doc)
	engine.RenderNode(buf, doc)
	check(110, 30, 0xFF, 0xFF, 0xFF, "浮层容器底")
	check(110, 50, 0xCF, 0xE2, 0xFF, "选中项高亮")
	check(0, 50, 0x76, 0x76, 0x76, "浮层左边框（压在高亮之上）")
	check(119, 50, 0x76, 0x76, 0x76, "浮层右边框")
	check(0, 20, 0x76, 0x76, 0x76, "浮层上边框")
}

// 显示文本取选中项的文本内容，value 取 value 属性，两者可不同。
func TestSelectDisplayTextIsOptionText(t *testing.T) {
	doc := openDoc(t, 400, 300, selectPage)
	sel := mustEl(t, doc, "#s")
	if v := engine.SelectValue(sel); v != "b" {
		t.Fatalf("SelectValue = %q, want b", v)
	}
	buf := engine.NewBufferGraphics(400, 300)
	engine.RenderNode(buf, doc)
	joined := strings.Join(buf.Texts, "|")
	if !strings.Contains(joined, "Bravo") {
		t.Errorf("select 盒应显示选中项文本 Bravo, texts=%q", joined)
	}
	if strings.Contains(joined, "Alpha") {
		t.Errorf("收起态不应绘制选项文本: %q", joined)
	}
	if svg := doc.DumpSVG(); !strings.Contains(svg, ">Bravo</text>") {
		t.Errorf("SVG 未导出 select 显示文本: %s", svg[:200])
	}

	engine.SetSelectValue(sel, "c")
	buf = engine.NewBufferGraphics(400, 300)
	engine.RenderNode(buf, doc)
	if joined := strings.Join(buf.Texts, "|"); !strings.Contains(joined, "Charlie") {
		t.Errorf("换值后应显示 Charlie, texts=%q", joined)
	}
}
