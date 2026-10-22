package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// pseudoDoc 打开一个带样式的文档，返回文档与根元素。
func pseudoDoc(t *testing.T, src string) engine.HTMLDocument {
	t.Helper()
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	return doc
}

// widthOf 取元素的边框盒宽（样式断言用几何变化最直观）。
func widthOf(t *testing.T, e engine.HTMLElement) int {
	t.Helper()
	if e == nil {
		t.Fatal("元素不存在")
	}
	r := e.GetBoundingClientRect()
	return r.Right().Pixel() - r.Left().Pixel()
}

// hover 悬停某元素并重排（状态变化 → 变更通知 → 重排，与应用层同链路）。
func hover(t *testing.T, doc engine.HTMLDocument, e engine.HTMLElement) {
	t.Helper()
	engine.SetHoveredElement(doc, e)
	doc.Refresh()
}

// :hover —— 悬停态命中时套用对应声明。
func TestPseudoHoverAppliesStyle(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 50px; height: 10px; }
		div:hover { width: 200px; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	d := doc.QuerySelector("#d")
	if got := widthOf(t, d); got != 50 {
		t.Fatalf("悬停前宽 = %d, want 50", got)
	}
	hover(t, doc, d)
	if got := widthOf(t, d); got != 200 {
		t.Errorf("悬停后宽 = %d, want 200（:hover 未生效）", got)
	}
	// 移开悬停 → 样式还原
	hover(t, doc, nil)
	if got := widthOf(t, d); got != 50 {
		t.Errorf("移开后宽 = %d, want 50（:hover 未还原）", got)
	}
}

// :hover 沿父链继承：鼠标停在子元素上时，祖先同样匹配（Chrome 同款）。
func TestPseudoHoverMatchesAncestors(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 300px; height: 40px; }
		div:hover span { color: #FF0000; }
		span { color: #000000; }
	</style></head><body style="margin:0">
		<div id="d"><span id="s">x</span></div>
	</body></html>`)
	d := doc.QuerySelector("#d")
	s := doc.QuerySelector("#s")
	hover(t, doc, s)
	svg := doc.DumpSVG()
	if !contains(svg, "#ff0000") {
		t.Errorf("悬停子元素后祖先的 :hover 规则未生效:\n%s", svg)
	}
	_ = d
}

// 悬停态变化要触发变更通知（应用层据此重排重绘）。
func TestPseudoHoverChangeNotifies(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><body style="margin:0">
		<div id="d" style="width:50px;height:10px"></div></body></html>`)
	d := doc.QuerySelector("#d")
	before := doc.ChangeCount()
	engine.SetHoveredElement(doc, d)
	if doc.ChangeCount() == before {
		t.Error("悬停态变化未触发变更通知")
	}
	// 悬停目标没变 → 不通知（鼠标移动是最频繁的事件）
	mid := doc.ChangeCount()
	engine.SetHoveredElement(doc, d)
	if doc.ChangeCount() != mid {
		t.Error("悬停目标未变却触发了变更通知")
	}
}

// :focus —— 有焦点的元素套用对应声明（可编辑输入）。
func TestPseudoFocusAppliesStyle(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		input { width: 50px; }
		input:focus { width: 220px; }
	</style></head><body style="margin:0"><input id="i"></body></html>`)
	in := doc.QuerySelector("#i")
	if got := widthOf(t, in); got != 50 {
		t.Fatalf("聚焦前宽 = %d, want 50", got)
	}
	engine.SetFocusedElement(doc, in)
	doc.Refresh()
	if got := widthOf(t, in); got != 220 {
		t.Errorf("聚焦后宽 = %d, want 220（:focus 未生效）", got)
	}
	engine.SetFocusedElement(doc, nil)
	doc.Refresh()
	if got := widthOf(t, in); got != 50 {
		t.Errorf("失焦后宽 = %d, want 50（:focus 未还原）", got)
	}
}

// :focus 对按钮同样生效（点击按钮即可获得焦点）。
func TestPseudoFocusOnButton(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		button { width: 60px; }
		button:focus { width: 240px; }
	</style></head><body style="margin:0"><button id="b">go</button></body></html>`)
	b := doc.QuerySelector("#b")
	engine.SetFocusedElement(doc, b)
	doc.Refresh()
	if got := widthOf(t, b); got != 240 {
		t.Errorf("聚焦按钮后宽 = %d, want 240（:focus 未生效）", got)
	}
}

// :active —— 鼠标按下到抬起之间生效。
func TestPseudoActiveAppliesStyle(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 50px; height: 10px; }
		div:active { width: 180px; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	d := doc.QuerySelector("#d")
	engine.SetActiveElement(doc, d)
	doc.Refresh()
	if got := widthOf(t, d); got != 180 {
		t.Errorf("按下后宽 = %d, want 180（:active 未生效）", got)
	}
	engine.SetActiveElement(doc, nil)
	doc.Refresh()
	if got := widthOf(t, d); got != 50 {
		t.Errorf("抬起后宽 = %d, want 50（:active 未还原）", got)
	}
}

// :checked —— 勾选中的 radio/checkbox。
func TestPseudoCheckedAppliesStyle(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		input[type=checkbox] { width: 13px; }
		input:checked { width: 99px; }
	</style></head><body style="margin:0">
		<input id="a" type="checkbox"><input id="b" type="checkbox" checked>
	</body></html>`)
	a := doc.QuerySelector("#a")
	b := doc.QuerySelector("#b")
	if got := widthOf(t, a); got != 13 {
		t.Errorf("未勾选宽 = %d, want 13", got)
	}
	if got := widthOf(t, b); got != 99 {
		t.Errorf("已勾选宽 = %d, want 99（:checked 未生效）", got)
	}
}

// :disabled / :enabled —— 按 disabled 属性判定，且只对表单控件有意义。
func TestPseudoDisabledAndEnabled(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		input { width: 60px; }
		input:disabled { width: 120px; }
		input:enabled { width: 90px; }
		div { width: 70px; }
		div:disabled { width: 300px; }
	</style></head><body style="margin:0">
		<input id="on" type="text"><input id="off" type="text" disabled>
		<div id="d">x</div>
	</body></html>`)
	if got := widthOf(t, doc.QuerySelector("#on")); got != 90 {
		t.Errorf("可用控件宽 = %d, want 90（:enabled 未生效）", got)
	}
	if got := widthOf(t, doc.QuerySelector("#off")); got != 120 {
		t.Errorf("禁用控件宽 = %d, want 120（:disabled 未生效）", got)
	}
	// div 不是表单控件：:disabled/:enabled 都不该命中（宽度保持基础值）。
	if got := widthOf(t, doc.QuerySelector("#d")); got != 70 {
		t.Errorf("div 宽 = %d, want 70（div 不该命中 :disabled/:enabled）", got)
	}
}

// 结构伪类：:first-child / :last-child / :only-child。
func TestPseudoStructuralChildren(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		li { width: 40px; height: 10px; }
		li:first-child { width: 100px; }
		li:last-child { height: 30px; }
		li:only-child { width: 200px; }
	</style></head><body style="margin:0">
		<ul><li id="a">1</li><li id="b">2</li></ul>
		<ul><li id="c">3</li></ul>
	</body></html>`)
	a := doc.QuerySelector("#a")
	b := doc.QuerySelector("#b")
	c := doc.QuerySelector("#c")
	if got := widthOf(t, a); got != 100 {
		t.Errorf("首个 li 宽 = %d, want 100（:first-child 未生效）", got)
	}
	if got := widthOf(t, b); got != 40 {
		t.Errorf("末个 li 宽 = %d, want 40（:last-child 不该改宽）", got)
	}
	r := b.GetBoundingClientRect()
	if h := r.Bottom().Pixel() - r.Top().Pixel(); h != 30 {
		t.Errorf("末个 li 高 = %d, want 30（:last-child 未生效）", h)
	}
	// 独子：既是 first-child 也是 last-child，但 :only-child 规则特异度更高
	// （同为 class 档，但后声明的同特异度规则胜出）。
	if got := widthOf(t, c); got != 200 {
		t.Errorf("独子 li 宽 = %d, want 200（:only-child 未生效）", got)
	}
}

// :root —— 只命中文档根元素。
func TestPseudoRoot(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		html { width: 321px; }
		body { width: 100px; }
	</style></head><body style="margin:0">x</body></html>`)
	// 根元素不是 body：html 的宽度被 :root 规则改写。
	if got := widthOf(t, doc); got != 321 {
		t.Errorf("根元素宽 = %d, want 321（:root 未生效）", got)
	}
}

// :empty —— 没有子节点的元素。
func TestPseudoEmpty(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 60px; height: 10px; }
		div:empty { width: 150px; }
	</style></head><body style="margin:0">
		<div id="e"></div><div id="n">x</div>
	</body></html>`)
	if got := widthOf(t, doc.QuerySelector("#e")); got != 150 {
		t.Errorf("空 div 宽 = %d, want 150（:empty 未生效）", got)
	}
	if got := widthOf(t, doc.QuerySelector("#n")); got != 60 {
		t.Errorf("非空 div 宽 = %d, want 60（:empty 不该命中）", got)
	}
}

// 伪类按 class 档计特异度：.x 的规则压过同名标签规则，但压不过 id。
func TestPseudoSpecificity(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 10px; }
		div:hover { width: 20px; }
		#d { width: 30px; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	d := doc.QuerySelector("#d")
	if got := widthOf(t, d); got != 30 {
		t.Errorf("id 优先时宽 = %d, want 30", got)
	}
	hover(t, doc, d)
	if got := widthOf(t, d); got != 30 {
		t.Errorf("悬停后宽 = %d, want 30（id 特异度高于伪类）", got)
	}
}

// 伪类与标签/类/属性可写在同一个片段里（a.btn:hover）。
func TestPseudoCombinedWithOtherSelectors(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		a { width: 30px; height: 10px; display: inline-block; }
		a.btn:hover { width: 170px; }
	</style></head><body style="margin:0">
		<a id="hit" class="btn" href="#">x</a><a id="miss" class="other" href="#">y</a>
	</body></html>`)
	hit := doc.QuerySelector("#hit")
	miss := doc.QuerySelector("#miss")
	hover(t, doc, hit)
	if got := widthOf(t, hit); got != 170 {
		t.Errorf("命中元素宽 = %d, want 170（a.btn:hover 未生效）", got)
	}
	if got := widthOf(t, miss); got != 30 {
		t.Errorf("未命中元素宽 = %d, want 30", got)
	}
}

// 未知伪类不匹配（CSS：无法识别的伪类使整条选择器失效）。
func TestPseudoUnknownNeverMatches(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 40px; }
		div:nosuchpseudo { width: 300px; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	if got := widthOf(t, doc.QuerySelector("#d")); got != 40 {
		t.Errorf("宽 = %d, want 40（未知伪类不该命中）", got)
	}
}

// 伪元素未实现：含伪元素的片段永不匹配（不能退化成只匹配标签）。
func TestPseudoElementNeverMatches(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		div { width: 40px; }
		div::before { width: 300px; }
	</style></head><body style="margin:0"><div id="d">x</div></body></html>`)
	if got := widthOf(t, doc.QuerySelector("#d")); got != 40 {
		t.Errorf("宽 = %d, want 40（伪元素未实现，不该命中）", got)
	}
}

// QuerySelector 支持伪类选择器。
func TestQuerySelectorWithPseudo(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><body style="margin:0">
		<div id="a">1</div><div id="b">2</div></body></html>`)
	a := doc.QuerySelector("#a")
	hover(t, doc, a)
	got := doc.QuerySelector("div:hover")
	if got == nil {
		t.Fatal("QuerySelector(div:hover) 未命中")
	}
	if got.GetAttribute("id") != "a" {
		t.Errorf("命中的是 #%s, want #a", got.GetAttribute("id"))
	}
	if doc.QuerySelector("div:focus") != nil {
		t.Error("无焦点时 QuerySelector(div:focus) 不该命中")
	}
}

// contains 判断字符串是否包含子串（SVG 断言用）。
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
