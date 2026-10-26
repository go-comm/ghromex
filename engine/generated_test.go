package engine_test

import (
	"testing"
)

// ::before / ::after 的 content 文本按源序绘制在宿主内容前后，颜色来自
// 伪元素自身的声明（不污染宿主）。
func TestPseudoContentRendersBeforeAfter(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		#d { font-size: 16px; }
		#d::before { content: "<<" ; color: #ff0000; }
		#d::after { content: ">>" ; color: #0000ff; }
	</style></head><body style="margin:0"><div id="d">X</div></body></html>`)
	svg := doc.DumpSVG()
	iBefore := indexOf(svg, "&lt;&lt;")
	iText := indexOf(svg, ">X<")
	iAfter := indexOf(svg, "&gt;&gt;")
	if iBefore < 0 || iText < 0 || iAfter < 0 {
		t.Fatalf("生成内容未绘制（before=%d text=%d after=%d）:\n%s", iBefore, iText, iAfter, svg)
	}
	if !(iBefore < iText && iText < iAfter) {
		t.Errorf("绘制顺序不对：before=%d text=%d after=%d（应为 before < text < after）", iBefore, iText, iAfter)
	}
	if !contains(svg, "#ff0000") {
		t.Errorf("::before 的 color 未生效:\n%s", svg)
	}
	if !contains(svg, "#0000ff") {
		t.Errorf("::after 的 color 未生效:\n%s", svg)
	}
}

// 生成内容不是 DOM：InnerText 与 Children 都看不到它。
func TestPseudoContentNotInDOM(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		#d::before { content: "前"; }
		#d::after { content: "后"; }
	</style></head><body style="margin:0"><div id="d">正文</div></body></html>`)
	d := doc.QuerySelector("#d")
	if got := d.InnerText(); got != "正文" {
		t.Errorf("InnerText = %q, want 正文（生成内容不该进 DOM）", got)
	}
	if n := len(d.Children()); n != 1 {
		t.Errorf("Children 数 = %d, want 1（生成盒应被过滤）", n)
	}
	if all := doc.QuerySelectorAll("div"); len(all) != 1 {
		t.Errorf("QuerySelectorAll(div) = %d 个, want 1", len(all))
	}
	// JSON 视图同样不应出现生成内容（调试/脚本读的是 DOM 快照）。
	if js := d.FormatJSON("  ", true); contains(js, "前") || contains(js, "后") {
		t.Errorf("JSON 视图里出现了生成内容:\n%s", js)
	}
}

// content: attr(name) 取宿主属性值；多个分量按顺序拼接。
func TestPseudoContentAttr(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		#d::before { content: "[" attr(data-k) "]"; }
	</style></head><body style="margin:0"><div id="d" data-k="A">x</div></body></html>`)
	if svg := doc.DumpSVG(); !contains(svg, "[A]") {
		t.Errorf("attr() 内容未渲染:\n%s", svg)
	}
}

// content: none 不生成盒子；伪元素上的 width 不该落到宿主。
func TestPseudoContentNoneGeneratesNothing(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 40px; }
		div::before { content: none; width: 300px; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	if got := widthOf(t, doc.QuerySelector("#d")); got != 40 {
		t.Errorf("宽 = %d, want 40（content:none 不该生成盒子）", got)
	}
}

// content: "" 仍然生成盒子：空内容 + 宽高/背景可以画成圆点、分隔条。
func TestPseudoEmptyContentStillGeneratesBox(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		#d::before { content: ""; display: inline-block; width: 24px; height: 8px;
		             background: #123456; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	svg := doc.DumpSVG()
	if !contains(svg, "#123456") {
		t.Errorf("空 content 的生成盒未绘制背景:\n%s", svg)
	}
	if !contains(svg, `width="24"`) {
		t.Errorf("生成盒宽度 24px 未生效:\n%s", svg)
	}
}

// 单冒号旧写法 :before/:after 与双冒号等价。
func TestPseudoSingleColonBeforeAfter(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		#d:before { content: "L"; }
		#d:after { content: "R"; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	svg := doc.DumpSVG()
	if !contains(svg, ">L<") || !contains(svg, ">R<") {
		t.Errorf("单冒号伪元素未生效:\n%s", svg)
	}
}

// :hover::before 按宿主的悬停状态匹配，状态变化后内容随之更新。
func TestPseudoHoverChangesContent(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		#d::before { content: "off"; }
		#d:hover::before { content: "on"; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	d := doc.QuerySelector("#d")
	if svg := doc.DumpSVG(); !contains(svg, ">off<") {
		t.Fatalf("悬停前内容不对:\n%s", svg)
	}
	hover(t, doc, d)
	svg := doc.DumpSVG()
	if !contains(svg, ">on<") {
		t.Errorf("悬停后 content 未更新:\n%s", svg)
	}
	if contains(svg, ">off<") {
		t.Errorf("悬停后旧内容仍在:\n%s", svg)
	}
}

// content 字符串里的分号与转义引号按值处理，不会被当成声明分隔符。
func TestPseudoContentStringEscapes(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		#d::before { content: "a;b\"c"; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	if svg := doc.DumpSVG(); !contains(svg, `a;b&quot;c`) {
		t.Errorf("content 字符串的分号/转义引号未正确解析:\n%s", svg)
	}
}

// 未支持的伪元素（::first-line）仍不匹配，声明不落到宿主。
func TestPseudoUnknownElementNeverMatches(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 40px; }
		div::first-line { width: 300px; content: "ZZ"; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	if got := widthOf(t, doc.QuerySelector("#d")); got != 40 {
		t.Errorf("宽 = %d, want 40（未知伪元素不该命中）", got)
	}
	if svg := doc.DumpSVG(); contains(svg, ">ZZ<") {
		t.Errorf("未知伪元素不该生成内容:\n%s", svg)
	}
}
