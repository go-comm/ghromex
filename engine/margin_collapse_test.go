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

// 父-子穿越 margin 折叠（CSS 2.1 §8.3.1）：父块无上边框/上内边距时，父的
// 上外边距与首个子块的上外边距折叠（取大者），子块自身不再重复计入。
// 此前引擎只实现相邻兄弟折叠——<form> 内首个 <p> 会与前一个 <p> 的下外边距
// 叠加成 32px（Chrome 折叠为 16px），整块表单下移一行。
func TestMarginCollapseThroughParent(t *testing.T) {
	doc := openDoc(t, 200, 500, `<!doctype html><html><head></head><body style="margin:0">
		<p id="a" style="margin:16px 0;height:10px"></p>
		<form><p id="b" style="margin:16px 0;height:10px"></p></form>
	</body></html>`)
	a, b := mustEl(t, doc, "#a"), mustEl(t, doc, "#b")
	if gap := b.Y() - (a.Y() + a.Height()); gap != 16 {
		t.Errorf("嵌套 p 间隙 = %d, want 16（父-子穿越折叠：16+16 → max=16）", gap)
	}
}

// 父块有上边框/上内边距时不折叠：子块的上外边距留在父内容盒内。
func TestMarginNoCollapseThroughParentWithBorder(t *testing.T) {
	doc := openDoc(t, 200, 500, `<!doctype html><html><head></head><body style="margin:0">
		<p id="a" style="margin:0 0 16px;height:10px"></p>
		<div id="w" style="border:1px solid #000">
			<p id="b" style="margin:16px 0;height:10px"></p>
		</div>
	</body></html>`)
	a, b := mustEl(t, doc, "#a"), mustEl(t, doc, "#b")
	// 有边框隔开：间隙 = 前兄 mb16 + 父边框1 + 子 mt16 = 33
	if gap := b.Y() - (a.Y() + a.Height()); gap != 33 {
		t.Errorf("带边框父块的嵌套 p 间隙 = %d, want 33（边框隔开不折叠）", gap)
	}
}

// 父块的下外边距与末个子块的下外边距折叠：父块之后的间隙取大者。
func TestMarginCollapseThroughParentBottom(t *testing.T) {
	doc := openDoc(t, 200, 500, `<!doctype html><html><head></head><body style="margin:0">
		<div id="w" style="margin:0 0 10px">
			<p id="b" style="margin:0 0 30px;height:10px"></p>
		</div>
		<p id="c" style="margin:10px 0;height:10px"></p>
	</body></html>`)
	b, c := mustEl(t, doc, "#b"), mustEl(t, doc, "#c")
	// 父的 mb(10) 与末子 mb(30) 折叠 → 30；再与 c 的 mt(10) 折叠 → 30
	if gap := c.Y() - (b.Y() + b.Height()); gap != 30 {
		t.Errorf("父块底部折叠后间隙 = %d, want 30（max(10,30) 与 10 折叠）", gap)
	}
}
