package engine_test

import "testing"

// 相邻块级兄弟 margin 折叠（CSS 2.1 §8.3.1）：
// 间隙 = max(前兄 mb, 后子 mt)；行内内容隔开则不折叠；脱流盒不打断折叠。
// 对照浏览器实测：两个 margin:16px 0 的相邻 p，间隙为 16（非 32）。
func TestMarginCollapsing(t *testing.T) {
	doc := openDoc(t, 200, 500, `<!doctype html><html><head></head><body style="margin:0">
		<p id="a" style="margin:16px 0;height:10px"></p>
		<p id="b" style="margin:16px 0;height:10px"></p>
	</body></html>`)
	a, b := mustEl(t, doc, "#a"), mustEl(t, doc, "#b")
	if gap := b.Y() - (a.Y() + a.Height()); gap != 16 {
		t.Errorf("相邻 p 间隙 = %d, want 16（margin 折叠：16+16 → max=16）", gap)
	}
}

// 不同值的折叠取较大者；inline-block 前兄的 margin 不折叠。
func TestMarginCollapsingMax(t *testing.T) {
	doc := openDoc(t, 200, 500, `<!doctype html><html><head></head><body style="margin:0">
		<div id="a" style="margin:0 0 20px;height:10px"></div>
		<div id="b" style="margin:10px 0;height:10px"></div>
		<div id="c" style="display:inline-block;margin:0 0 12px;width:10px;height:10px"></div>
		<div id="d" style="margin:10px 0;height:10px"></div>
	</body></html>`)
	a, b, d := mustEl(t, doc, "#a"), mustEl(t, doc, "#b"), mustEl(t, doc, "#d")
	if gap := b.Y() - (a.Y() + a.Height()); gap != 20 {
		t.Errorf("mb=20/mt=10 折叠间隙 = %d, want 20（取较大者）", gap)
	}
	// inline-block（行内内容）隔开：不折叠，后块顶 = 行盒底 + 前兄 mb(12) + mt(10)
	// c 高 10 + 前兄 mb 12 + mt 10 = 32 > 折叠值 12，只要明显大于折叠值即未折叠
	if gap := d.Y() - (b.Y() + b.Height()); gap <= 12 {
		t.Errorf("inline-block 后间隙 = %d, want >12（行内内容隔开不折叠）", gap)
	}
}

// 行内文本隔开两个块：margin 不折叠（CSS：行盒不算相邻）。
func TestMarginNoCollapseAcrossText(t *testing.T) {
	doc := openDoc(t, 200, 500, `<!doctype html><html><head></head><body style="margin:0">
		<p id="a" style="margin:16px 0;height:10px"></p>
		中间有文本
		<p id="b" style="margin:16px 0;height:10px"></p>
	</body></html>`)
	a, b := mustEl(t, doc, "#a"), mustEl(t, doc, "#b")
	// 间隙 = mb16 + 行盒高 + mt16 ≥ 32（未折叠），折叠版应为 16+行盒高-16=行盒高
	if gap := b.Y() - (a.Y() + a.Height()); gap < 32 {
		t.Errorf("文本隔开的块间隙 = %d, want ≥32（不折叠）", gap)
	}
}

// 脱流盒（absolute）不打断两侧常规流兄弟的折叠。
func TestMarginCollapseAcrossOutOfFlow(t *testing.T) {
	doc := openDoc(t, 200, 500, `<!doctype html><html><head></head><body style="margin:0">
		<div id="a" style="margin:0 0 16px;height:10px"></div>
		<div id="abs" style="position:absolute;left:0;top:0;width:5px;height:5px"></div>
		<div id="b" style="margin:16px 0;height:10px"></div>
	</body></html>`)
	a, b := mustEl(t, doc, "#a"), mustEl(t, doc, "#b")
	if gap := b.Y() - (a.Y() + a.Height()); gap != 16 {
		t.Errorf("脱流盒隔开的兄弟间隙 = %d, want 16（折叠不被打断）", gap)
	}
}
