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
