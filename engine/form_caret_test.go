package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// ---------------------------------------------------------------------------
// 表单交互语义回归
// ---------------------------------------------------------------------------

// TestInputCaretInVisibleWindow 单行 input 的值超宽时渲染"舍头保尾"（只画
// 尾部），光标映射必须同口径——此前点击可见区却把光标定位到被裁掉的前缀
// 里，键入的字符落到错误位置。
func TestInputCaretInVisibleWindow(t *testing.T) {
	const long = "0123456789012345678901234567890123456789" // 40 字符
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#i { width: 180px; }
	</style></head><body><input id="i" value="` + long + `"></body></html>`
	doc := openDoc(t, 400, 200, src)
	in := mustEl(t, doc, "#i")
	r := in.GetBoundingClientRect()
	// 点击可见区右缘 → 光标应落在可见文本末尾
	engine.OnDocumentClick(doc, r.Right().Pixel()-2, (r.Top().Pixel()+r.Bottom().Pixel())/2)
	if !engine.OnDocumentTextInput(doc, "X") {
		t.Fatal("输入框未获得焦点，文本输入未被消费")
	}
	got := in.GetAttribute("value")
	if !strings.HasSuffix(got, "X") {
		t.Errorf("键入的字符未落在可见文本末尾: value=%q", got)
	}
	if len(got) != len(long)+1 {
		t.Errorf("值长度 = %d, want %d", len(got), len(long)+1)
	}
}

// TestInputCaretScrollsWindow 长值 input 的可见窗口跟随光标
// （Chrome scroll-follows-caret）：Home 后窗口滚回开头、首字可见；End 后
// 窗口回到末尾。回归：窗口曾固定"保尾"，光标索引能回到 0 但画面永远滚不
// 回首字（examples/browser 反馈"光标移动不到首字"）。
func TestInputCaretScrollsWindow(t *testing.T) {
	// 非周期字符集：首尾子串互不重叠，Contains 断言可靠。
	const long = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn" // 40 字母
	head, tail := long[:14], long[len(long)-14:]
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#i { width: 180px; }
	</style></head><body><input id="i" value="` + long + `"></body></html>`
	doc := openDoc(t, 400, 200, src)
	in := mustEl(t, doc, "#i")
	// 聚焦：光标放值末尾，窗口舍头保尾（末尾可见、开头不可见）。
	engine.SetFocusedElement(doc, in)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, tail) {
		t.Errorf("聚焦末尾时窗口应保尾，SVG 未含值末尾 %q:\n%s", tail, svg)
	}
	if strings.Contains(svg, head) {
		t.Errorf("聚焦末尾时不应显示值开头 %q:\n%s", head, svg)
	}
	// Home：光标回到 0，窗口必须滚回开头，首字可见。
	if !engine.OnDocumentKeyDown(doc, engine.KeyHome) {
		t.Fatal("Home 未被消费")
	}
	svg = doc.DumpSVG()
	if !strings.Contains(svg, head) {
		t.Errorf("Home 后窗口未滚回开头（首字不可见）:\n%s", svg)
	}
	if strings.Contains(svg, tail) {
		t.Errorf("Home 后窗口仍停在末尾:\n%s", svg)
	}
	// End：回到末尾，窗口重新保尾。
	if !engine.OnDocumentKeyDown(doc, engine.KeyEnd) {
		t.Fatal("End 未被消费")
	}
	svg = doc.DumpSVG()
	if !strings.Contains(svg, tail) {
		t.Errorf("End 后窗口未跟回末尾:\n%s", svg)
	}
	if strings.Contains(svg, head) {
		t.Errorf("End 后窗口不应停在开头:\n%s", svg)
	}
}

// TestInputCaretCharByCharLeftScroll 长值 input 逐字左移时光标窗口同步
// 左滚：从末尾一路回到开头，窗口最终展示首字（每步都保证光标不丢出
// 可视区——窗口跟随由 ensureCaretVisible 驱动，Chrome 同口径）。
func TestInputCaretCharByCharLeftScroll(t *testing.T) {
	const long = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn" // 40 字母
	head, tail := long[:14], long[len(long)-14:]
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#i { width: 180px; }
	</style></head><body><input id="i" value="` + long + `"></body></html>`
	doc := openDoc(t, 400, 200, src)
	in := mustEl(t, doc, "#i")
	engine.SetFocusedElement(doc, in)
	if svg := doc.DumpSVG(); !strings.Contains(svg, tail) {
		t.Fatalf("聚焦末尾窗口应保尾:\n%s", svg)
	}
	// 逐字左移整段值长：光标回到 0，窗口必须一路跟回开头。
	for i := 0; i < len(long); i++ {
		if !engine.OnDocumentKeyDown(doc, engine.KeyLeft) {
			t.Fatalf("第 %d 次左键未被消费", i+1)
		}
	}
	svg := doc.DumpSVG()
	if !strings.Contains(svg, head) {
		t.Errorf("左移到底后窗口未滚回开头（首字不可见）:\n%s", svg)
	}
	if strings.Contains(svg, tail) {
		t.Errorf("左移到底后窗口仍含值末尾:\n%s", svg)
	}
}

// TestSelectAutoWidthKeepsDisplayText auto 宽的 select 必须为右侧下拉箭头
// 预留位置，否则显示文本被自己的箭头挤掉（此前渲染出 "Alph"）。
func TestSelectAutoWidthKeepsDisplayText(t *testing.T) {
	src := `<!doctype html><html><body>
	<select id="s"><option value="a">Alpha</option></select>
	</body></html>`
	doc := openDoc(t, 400, 200, src)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, ">Alpha<") {
		t.Errorf("auto 宽 select 的显示文本被截断:\n%s", svg)
	}
}

// TestSelectSVGTextMatchesRender 窄 select 的显示文本在渲染与 SVG 导出两处
// 必须一致（都按"可用宽 = 内容宽 - 箭头位"截断）。
func TestSelectSVGTextMatchesRender(t *testing.T) {
	src := `<!doctype html><html><body>
	<select id="s" style="width:40px"><option value="a">Alpha</option></select>
	</body></html>`
	doc := openDoc(t, 400, 200, src)
	svg := doc.DumpSVG()
	if strings.Contains(svg, ">Alpha<") {
		t.Errorf("窄 select 的导出文本未截断（应与渲染同口径）:\n%s", svg)
	}
	if !strings.Contains(svg, ">Al") {
		t.Errorf("窄 select 应导出截断后的文本:\n%s", svg)
	}
}
