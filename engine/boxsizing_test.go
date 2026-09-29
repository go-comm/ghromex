package engine_test

import "testing"

// box-sizing:border-box：显式 width/height 含 padding+border（不含 margin），
// 百分比解析基数与 content-box 一致；默认（作者 CSS）仍为 content-box。
func TestBoxSizingBorderBox(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div id="bb" style="box-sizing:border-box;width:100px;height:60px;padding:10px;border:5px solid #333"></div>
		<div id="cb" style="width:100px;height:60px;padding:10px;border:5px solid #333"></div>
		<div id="pct" style="box-sizing:border-box;width:50%;padding:0 10px"></div>
	</body></html>`
	doc := openDoc(t, 200, 300, src)

	// content = 100-20(pd)-10(bd) x 60-20-10；内容原点 = bd5+pd10
	bb := mustEl(t, doc, "#bb")
	if bb.Width() != 70 || bb.Height() != 30 || bb.X() != 15 || bb.Y() != 15 {
		t.Errorf("#bb = (%d,%d %dx%d), want origin (15,15) content (70,30)",
			bb.X(), bb.Y(), bb.Width(), bb.Height())
	}
	// CSS 默认 content-box：width 即内容宽
	cb := mustEl(t, doc, "#cb")
	if cb.Width() != 100 || cb.Height() != 60 {
		t.Errorf("#cb content = %dx%d, want 100x60（content-box 默认）", cb.Width(), cb.Height())
	}
	// 百分比相对包含块内容宽（200）解析后再减 padding：100-20 = 80
	if p := mustEl(t, doc, "#pct"); p.Width() != 80 {
		t.Errorf("#pct.Width() = %d, want 80（border-box 百分比解析）", p.Width())
	}
}

// 表单控件按 Chrome UA 默认 border-box：width 声明减 padding+border；
// 无声明的默认外盒 153x20（与切换语义前视觉等高，锁定不回归）。
func TestInputUABoxSizing(t *testing.T) {
	doc := openDoc(t, 300, 100, `<!doctype html><html><head></head><body style="margin:0">
		<input id="i" style="width:100px">
		<input id="d">
	</body></html>`)

	// UA: padding 1px 2px（左右 4）+ border 1px（左右 2）→ content = 94
	if w := mustEl(t, doc, "#i").Width(); w != 94 {
		t.Errorf("#i.Width() = %d, want 94（UA border-box 扣减）", w)
	}
	d := mustEl(t, doc, "#d")
	if ow, oh := d.Width()+4+2, d.Height()+2+2; ow != 177 || oh != 21 {
		t.Errorf("#d 外盒 = %dx%d, want 177x21（UA 默认外盒回归）", ow, oh)
	}
}

// 两列 inline-block 布局：input width:100% 在 UA border-box 下不再撑破列容器
// （示例页姓名两列曾因 content-box 溢出 26px 压歪第二列）。
func TestInputBorderBoxNoOverflow(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div id="cols" style="width:200px"><div class="c" style="display:inline-block;width:49%"><input class="in" style="width:100%"></div><div class="c" style="display:inline-block;width:49%"><input class="in" style="width:100%"></div>
		</div>
	</body></html>`
	doc := openDoc(t, 200, 60, src)
	inputs := doc.QuerySelectorAll(".in")
	if len(inputs) != 2 {
		t.Fatalf("want 2 inputs, got %d", len(inputs))
	}
	// 列宽 = 49% * 200 = 98；input 外盒应 = 98（不溢出），content = 98-6 = 92
	for k, in := range inputs {
		if ow := in.Width() + 6; ow != 98 {
			t.Errorf("input[%d] 外盒 = %d, want 98（width:100%% 溢出容器）", k, ow)
		}
	}
	// 第二列不被第一列溢出撑歪：起点 = 第一列之后（x=98）
	if c2 := doc.QuerySelectorAll(".c")[1]; c2.X() != 98 {
		t.Errorf("第二列 x = %d, want 98（列被 input 溢出撑歪）", c2.X())
	}
}
