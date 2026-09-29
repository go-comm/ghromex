package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// input 按 type 的 UA 外观（属性选择器不可用，代码级补齐于级联）：
// radio/checkbox 13x13 控件；submit/reset/button 按钮外观、宽随 value 文本。
func TestInputTypeAppearance(t *testing.T) {
	doc := openDoc(t, 400, 200, `<!doctype html><html><head></head><body style="margin:0">
		<input id="r" type="radio"><input id="c" type="checkbox">
		<input id="s" type="submit" value="提交">
		<input id="t" type="text">
		<input id="ro" type="radio" style="width:40px;height:40px">
	</body></html>`)

	// radio/checkbox：外盒 13x13（UA 默认尺寸，Chrome 实测一致）
	for _, id := range []string{"#r", "#c"} {
		r := mustEl(t, doc, id).GetBoundingClientRect()
		if w, h := r.Right().Pixel()-r.Left().Pixel(), r.Bottom().Pixel()-r.Top().Pixel(); w != 13 || h != 13 {
			t.Errorf("%s 外盒 = %dx%d, want 13x13", id, w, h)
		}
	}
	// radio 行内基线近似：margin-top 3px
	if y := mustEl(t, doc, "#r").GetBoundingClientRect().Top().Pixel(); y != 3 {
		t.Errorf("#r.Top = %d, want 3（radio 顶距）", y)
	}

	// submit：内容宽 = value 文本 26 + padding 1px 6px + border 1px = 40；
	// 高 = 25（Chrome button 外盒高对齐）。宽含余量：渲染层溢出截断以
	// base.width-4 为界，不留余量会砍掉首字（回归：曾只画“交”丢“提”）。
	s := mustEl(t, doc, "#s").GetBoundingClientRect()
	if w := s.Right().Pixel() - s.Left().Pixel(); w != 40 {
		t.Errorf("#s 外盒宽 = %d, want 40（submit 宽随 value 文本）", w)
	}
	if h := s.Bottom().Pixel() - s.Top().Pixel(); h != 25 {
		t.Errorf("#s 外盒高 = %d, want 25", h)
	}

	// text：保持输入框默认 177x21
	tr := mustEl(t, doc, "#t").GetBoundingClientRect()
	if w, h := tr.Right().Pixel()-tr.Left().Pixel(), tr.Bottom().Pixel()-tr.Top().Pixel(); w != 177 || h != 21 {
		t.Errorf("#t 外盒 = %dx%d, want 177x21", w, h)
	}

	// 作者 CSS 覆盖 type 默认（校正发生在 UA 级、作者规则之前）
	ro := mustEl(t, doc, "#ro").GetBoundingClientRect()
	if w, h := ro.Right().Pixel()-ro.Left().Pixel(), ro.Bottom().Pixel()-ro.Top().Pixel(); w != 40 || h != 40 {
		t.Errorf("#ro 外盒 = %dx%d, want 40x40（作者样式未覆盖 type 默认）", w, h)
	}
}

// submit/reset 的 value 文本必须完整绘制（渲染层溢出截断以 base.width-4
// 为界，auto 宽不留余量会砍掉首字：回归锁定“提交”曾只画“交”）。
func TestSubmitValueTextRendered(t *testing.T) {
	doc := openDoc(t, 400, 200, `<!doctype html><html><head></head><body style="margin:0">
		<input type="submit" id="s" value="提交">
		<input type="reset" id="r" value="重置">
	</body></html>`)
	buf := engine.NewBufferGraphics(400, 200)
	engine.RenderNode(buf, doc)
	found := map[string]bool{}
	for _, s := range buf.Texts {
		found[s] = true
	}
	for _, want := range []string{"提交", "重置"} {
		if !found[want] {
			t.Errorf("按钮 value %q 未完整绘制，Texts=%v", want, buf.Texts)
		}
	}
}

// form 为块级（Chrome UA 语义）：宽度撑满包含块，不再按 inline-block 收缩
// （曾因无 UA 规则回退 inline-block 且 shrink-wrap 漏算块子宽而收缩到 4px）。
func TestFormBlockDisplay(t *testing.T) {
	doc := openDoc(t, 400, 300, `<!doctype html><html><head></head><body style="margin:0">
		<form id="f"><p id="p">x</p></form>
	</body></html>`)
	f := mustEl(t, doc, "#f")
	if f.Width() != 400 {
		t.Errorf("form 宽 = %d, want 400（块级撑满包含块）", f.Width())
	}
	if p := mustEl(t, doc, "#p"); p.X() != 0 || p.Width() != 400 {
		t.Errorf("form 内 p = (%d, %dx%d), want (0, 400)", p.X(), p.Width(), p.Height())
	}
}

// inline-block 容器仅含块级子：块子宽度计入 shrink-wrap 统计
// （回归：曾收缩到游标宽 4px）。
func TestInlineBlockBlockChildrenWidth(t *testing.T) {
	doc := openDoc(t, 400, 200, `<!doctype html><html><head></head><body style="margin:0">
		<div id="ib" style="display:inline-block"><p>宽文本宽度</p></div>
	</body></html>`)
	r := mustEl(t, doc, "#ib").GetBoundingClientRect()
	if w := r.Right().Pixel() - r.Left().Pixel(); w < 100 {
		t.Errorf("#ib 宽 = %d, want ≥100（块子宽度未计入 shrink-wrap，曾收缩为 4px）", w)
	}
}
