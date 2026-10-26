package engine

import (
	"strings"
	"testing"
)

// ---- white-space 族与 <pre>/<code> 的对照测试 ----
//
// 口径来自 Chrome 实测（headless，1024 视口）：
//   - UA 只给 pre/code/kbd/samp/tt 设 monospace，**不设字号**：字号继承父级，
//     父级从未显式声明字号时落到 monospace 通用族的默认 13px；
//   - 首换行剥离只对 <pre>/<textarea>/<listing> 生效；
//   - pre 保留空白与换行、不软换行（长行溢出）；pre-wrap 额外在空格处折行
//     但不拆长词；pre-line 折叠空白（含行首空格）但保留换行；nowrap 折叠
//     空白且不换行。

// openPreDoc 无头打开一页并完成布局，返回文档（run 坐标在布局阶段写入）。
func openPreDoc(t *testing.T, src string) HTMLDocument {
	t.Helper()
	vp, buf := NewBufferedViewport(800, 600)
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)
	t.Cleanup(func() { doc.Close() })
	return doc
}

// mustBox 取选择器对应元素的内部盒。
func mustBox(t *testing.T, doc HTMLDocument, sel string) *htmlElement {
	t.Helper()
	el := doc.QuerySelector(sel)
	if el == nil {
		t.Fatalf("未找到 %s", sel)
	}
	b := inner(el)
	if b == nil {
		t.Fatalf("%s 无内部盒", sel)
	}
	return b
}

// runsOf 按文档序收集元素子树里的文本绘制片段。
func runsOf(t *testing.T, doc HTMLDocument, sel string) []textRun {
	t.Helper()
	var out []textRun
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		if b := inner(e); b != nil {
			out = append(out, b.runs...)
		}
		for _, c := range e.Children() {
			walk(c)
		}
	}
	walk(mustBox(t, doc, sel).self)
	return out
}

// distinctLines 统计 run 落在几个不同的行上（按基线 y+asc 去重，保持首次
// 出现顺序）。不能用 y 本身：同一行里混排不同字体时，run 的 y 按各自
// ascent 对齐到共享基线，y 值不同但基线相同。
func distinctLines(runs []textRun) []int {
	var ys []int
	for _, r := range runs {
		baseline := r.y + r.asc
		seen := false
		for _, y := range ys {
			if y == baseline {
				seen = true
				break
			}
		}
		if !seen {
			ys = append(ys, baseline)
		}
	}
	return ys
}

// boxSize 取元素外盒宽高。
func boxSize(t *testing.T, doc HTMLDocument, sel string) (w, h int) {
	r := doc.QuerySelector(sel).GetBoundingClientRect()
	return r.Right().Pixel() - r.Left().Pixel(), r.Bottom().Pixel() - r.Top().Pixel()
}

// TestParseWhiteSpace white-space 五值 + break-spaces 近似档的解析：
// 非法值不覆盖已有有效值（与其它属性「非法声明忽略」的口径一致）。
func TestParseWhiteSpace(t *testing.T) {
	st := newStyle()
	cases := []struct {
		value string
		want  WhiteSpace
	}{
		{"normal", WhiteSpaceNormal},
		{"nowrap", WhiteSpaceNowrap},
		{"pre", WhiteSpacePre},
		{"pre-wrap", WhiteSpacePreWrap},
		{"pre-line", WhiteSpacePreLine},
		{"break-spaces", WhiteSpacePreWrap}, // 近似档：区别只在行尾空格细节
		{"PRE-WRAP", WhiteSpacePreWrap},     // 大小写不敏感
		{"bogus", WhiteSpaceNormal},
	}
	for _, c := range cases {
		st.SetWhiteSpace(WhiteSpacePre) // 先置非默认值，验证每条落位
		applyDecl(st, "white-space", c.value)
		if got := st.WhiteSpace(); got != c.want {
			t.Errorf("white-space:%s = %v, want %v", c.value, got, c.want)
		}
	}
}

// TestPreCodeFontSizeInheritsParent UA 不给 pre/code/kbd/samp/tt 写死字号：
// 父级显式 font-size:20px 时它们是 20px（Chrome 实测），只有父级从未声明
// 字号时才落到 monospace 通用族默认 13px。
func TestPreCodeFontSizeInheritsParent(t *testing.T) {
	doc := openPreDoc(t, `<!doctype html><html><body style="margin:0">
		<p>x <code id="c13">c</code> <kbd id="k13">k</kbd> <samp id="s13">s</samp> <tt id="t13">t</tt></p>
		<pre id="pre13">x</pre>
		<div style="font-size:20px"><p>x <code id="c20">c</code></p><pre id="pre20">x</pre><tt id="t20">t</tt></div>
	</body></html>`)
	fontOf := func(sel string) int {
		b := mustBox(t, doc, sel)
		if b.computed == nil || b.computed.FontSize() == nil {
			t.Fatalf("%s 无计算字号", sel)
		}
		return resolveLen(b.computed.FontSize(), 0)
	}
	// body 未声明字号 → 通用族默认 13px（Chrome 同款 quirk）。
	for _, sel := range []string{"#c13", "#k13", "#s13", "#t13", "#pre13"} {
		if got := fontOf(sel); got != 13 {
			t.Errorf("%s 字号 = %d, want 13（父级未声明字号时的 monospace 默认档）", sel, got)
		}
	}
	// 父级显式 20px → 继承 20px（写死 13px 会让代码比正文小一档）。
	for _, sel := range []string{"#c20", "#pre20", "#t20"} {
		if got := fontOf(sel); got != 20 {
			t.Errorf("%s 字号 = %d, want 20（继承父级 font-size:20px）", sel, got)
		}
	}
}

// TestTTIsInlineMonospace tt 是 HTML4 遗留的等宽标签：Chrome 仍认它。
// 不识别时引擎按「未知标签 → inline-block」渲染，段落里会被原子化甚至断行。
func TestTTIsInlineMonospace(t *testing.T) {
	doc := openPreDoc(t, `<!doctype html><html><body style="margin:0">
		<p>前<tt id="tt">等宽</tt>后</p>
	</body></html>`)
	b := mustBox(t, doc, "#tt")
	if b.computed == nil {
		t.Fatal("tt 无计算样式")
	}
	if d := b.computed.Display(); d != DisplayInline {
		t.Errorf("tt display = %v, want inline（未知标签默认 inline-block 会把它原子化）", d)
	}
	if fam := strings.ToLower(b.computed.FontFamily()); !strings.Contains(fam, "monospace") {
		t.Errorf("tt font-family = %q, want monospace", b.computed.FontFamily())
	}
	// 行内 tt 与前后中文同一行。
	runs := runsOf(t, doc, "p")
	if len(distinctLines(runs)) != 1 {
		t.Errorf("含 tt 的段落排成 %d 行, want 1 行", len(distinctLines(runs)))
	}
}

// TestPrePreservesSpacesTabsAndBlankLines pre 里每个空格各占一份宽度、制表位
// 从行首起每 8 个空格宽对齐、连续换行之间的空行照占一行。
func TestPrePreservesSpacesTabsAndBlankLines(t *testing.T) {
	doc := openPreDoc(t, "<!doctype html><html><body style=\"margin:0\">\n"+
		"<pre id=\"one\" style=\"margin:0\"> a</pre>\n"+ // 1 个前导空格
		"<pre id=\"two\" style=\"margin:0\">  a</pre>\n"+ // 2 个前导空格
		"<pre id=\"tab\" style=\"margin:0\">\tx</pre>\n"+ // 行首制表符
		"<pre id=\"blank\" style=\"margin:0\">a\n\nb</pre>\n"+ // 中间空行
		"<pre id=\"single\" style=\"margin:0\">a</pre>\n"+ // 单行基准
		"</body></html>")

	// 「 a」里的空格 run 宽度即一个空格宽，用它推 2 空格与 1 空格（tab 停靠）。
	one := runsOf(t, doc, "#one")
	if len(one) < 2 {
		t.Fatalf("#one runs = %v, want 至少空格 + a 两段（前导空格不能折叠）", one)
	}
	spaceW := one[0].w
	if spaceW <= 0 {
		t.Fatalf("前导空格宽度 = %d, want > 0", spaceW)
	}
	if one[0].text != " " {
		t.Errorf("首段 = %q, want 单个空格", one[0].text)
	}
	two := runsOf(t, doc, "#two")
	if len(two) < 2 {
		t.Fatalf("#two runs = %v, want 两个空格 + a", two)
	}
	// 两个前导空格各占一份宽度（折叠成一个空格的话第二段就不是 2×spaceW）。
	if two[1].x != two[0].x+spaceW {
		t.Errorf("两个前导空格被折叠：第二个空格 x = %d, want %d", two[1].x, two[0].x+spaceW)
	}
	tab := runsOf(t, doc, "#tab")
	if len(tab) < 2 {
		t.Fatalf("#tab runs = %v, want 制表位 + x", tab)
	}
	if got, want := tab[1].x-tab[0].x, 8*spaceW; got != want {
		t.Errorf("行首制表符前进 %d, want %d（tab-size:8 个空格宽）", got, want)
	}
	// 空行不产生 run，只能看盒高：a\n\nb 是三行高（Chrome 实测 3 行）。
	_, hSingle := boxSize(t, doc, "#single")
	_, hBlank := boxSize(t, doc, "#blank")
	if hSingle <= 0 {
		t.Fatalf("单行 <pre> 高 = %d, want > 0", hSingle)
	}
	if hBlank != 3*hSingle {
		t.Errorf("a\\n\\nb 高 = %d, want %d（空行也要占一行，单行高 %d）",
			hBlank, 3*hSingle, hSingle)
	}
	if ys := distinctLines(runsOf(t, doc, "#blank")); len(ys) != 2 {
		t.Errorf("a\\n\\nb 的文字排成 %d 行, want 2 行（a 与 b 各一行）", len(ys))
	}
}

// TestFirstNewlineStrippedOnlyForPreTags HTML 规范只剥 <pre>/<textarea>/
// <listing> 开标签后的第一个换行；普通元素声明 white-space:pre 时首换行
// 照常断一行。
func TestFirstNewlineStrippedOnlyForPreTags(t *testing.T) {
	doc := openPreDoc(t, "<!doctype html><html><body style=\"margin:0\">\n"+
		"<pre id=\"pre\" style=\"margin:0\">\na</pre>\n"+
		"<div id=\"div\" style=\"margin:0;white-space:pre\">\na</div>\n"+
		"<div id=\"div1\" style=\"margin:0;white-space:pre\">a</div>\n"+
		"</body></html>")
	preRuns := runsOf(t, doc, "#pre")
	if n := len(distinctLines(preRuns)); n != 1 {
		t.Errorf("<pre> 首换行未剥离：排成 %d 行, want 1 行", n)
	}
	// div 首换行不剥：多出一个空行，盒高是同字体单行的两倍（空行不产生
	// run，只能从高度看；参照盒用同一元素类型，避免字体档不同）。
	_, hSingle := boxSize(t, doc, "#div1")
	_, hDiv := boxSize(t, doc, "#div")
	if hSingle <= 0 {
		t.Fatalf("单行 div[white-space:pre] 高 = %d, want > 0", hSingle)
	}
	if hDiv != 2*hSingle {
		t.Errorf("div[white-space:pre] 高 = %d, want %d（首换行占一行，单行高 %d）",
			hDiv, 2*hSingle, hSingle)
	}
}

// TestEmptyPreHasNoHeight 空 <pre> 与只含一个换行的 <pre> 都不生成行盒
// （首换行被剥离后内容为空），Chrome 实测高度 0。
func TestEmptyPreHasNoHeight(t *testing.T) {
	doc := openPreDoc(t, "<!doctype html><html><body style=\"margin:0\">\n"+
		"<pre id=\"e\" style=\"margin:0\"></pre>\n"+
		"<pre id=\"n\" style=\"margin:0\">\n</pre>\n"+
		"<pre id=\"a\" style=\"margin:0\">a</pre>\n"+
		"</body></html>")
	if _, h := boxSize(t, doc, "#e"); h != 0 {
		t.Errorf("空 <pre> 高 = %d, want 0", h)
	}
	if _, h := boxSize(t, doc, "#n"); h != 0 {
		t.Errorf("<pre>\\n</pre> 高 = %d, want 0（首换行剥离后无内容）", h)
	}
	if _, h := boxSize(t, doc, "#a"); h <= 0 {
		t.Errorf("<pre>a</pre> 高 = %d, want > 0", h)
	}
}

// TestPreDoesNotSoftWrapButPreWrapDoes 窄容器里：pre 的长行溢出（不折行），
// pre-wrap 在空格处折回，但两者都不拆长单词（Chrome 同款）。
func TestPreDoesNotSoftWrapButPreWrapDoes(t *testing.T) {
	const words = "aaa bbb ccc ddd eee fff ggg hhh iii jjj"
	const long = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	doc := openPreDoc(t, "<!doctype html><html><body style=\"margin:0\">\n"+
		"<div id=\"pre\" style=\"margin:0;width:120px;white-space:pre\">"+words+"</div>\n"+
		"<div id=\"pw\" style=\"margin:0;width:120px;white-space:pre-wrap\">"+words+"</div>\n"+
		"<div id=\"word\" style=\"margin:0;width:120px;white-space:pre-wrap\">"+long+"</div>\n"+
		"</body></html>")
	if n := len(distinctLines(runsOf(t, doc, "#pre"))); n != 1 {
		t.Errorf("white-space:pre 折成 %d 行, want 1 行（长行应溢出而不是软换行）", n)
	}
	if n := len(distinctLines(runsOf(t, doc, "#pw"))); n < 2 {
		t.Errorf("white-space:pre-wrap 排成 %d 行, want ≥2 行（空格处软换行）", n)
	}
	wordRuns := runsOf(t, doc, "#word")
	if n := len(distinctLines(wordRuns)); n != 1 {
		t.Errorf("pre-wrap 把长单词拆成 %d 行, want 1 行（Chrome 不拆词，直接溢出）", n)
	}
	if w := wordRuns[0].w; w <= 120 {
		t.Errorf("长单词 run 宽 = %d, want > 120（溢出容器）", w)
	}
}

// TestPreLineCollapsesSpacesKeepsBreaks pre-line 折叠连续空白（含行首空白）
// 但保留源换行；同一段文本在 normal 下换行会被折叠成空格。
func TestPreLineCollapsesSpacesKeepsBreaks(t *testing.T) {
	doc := openPreDoc(t, "<!doctype html><html><body style=\"margin:0\">\n"+
		"<div id=\"pl\" style=\"margin:0;width:400px;white-space:pre-line\">a   b\n   c   d</div>\n"+
		"<div id=\"nm\" style=\"margin:0;width:400px\">a   b\n   c   d</div>\n"+
		"<div id=\"ref\" style=\"margin:0;width:400px\">a b</div>\n"+
		"</body></html>")
	pl := runsOf(t, doc, "#pl")
	if n := len(distinctLines(pl)); n != 2 {
		t.Errorf("pre-line 排成 %d 行, want 2 行（源换行保留）", n)
	}
	if n := len(distinctLines(runsOf(t, doc, "#nm"))); n != 1 {
		t.Errorf("normal 排成 %d 行, want 1 行（换行折叠成空格）", n)
	}
	// 折叠口径与 normal 一致：连续空白只留一个空格宽。
	ref := runsOf(t, doc, "#ref")
	if len(ref) < 3 {
		t.Fatalf("#ref runs = %v, want a / 空格 / b", ref)
	}
	spaceW := ref[1].w
	for _, r := range pl {
		if r.text == " " && r.w != spaceW {
			t.Errorf("pre-line 的空格 run 宽 = %d, want %d（连续空白应折叠）", r.w, spaceW)
		}
	}
	// 行首空白被移除：第二行首项贴在行首。
	var secondFirstX = -1
	for _, r := range pl {
		if r.y != pl[0].y {
			secondFirstX = r.x
			break
		}
	}
	if secondFirstX >= 0 && secondFirstX != pl[0].x {
		t.Errorf("pre-line 第二行首项 x = %d, want %d（行首空白移除）", secondFirstX, pl[0].x)
	}
}

// TestNowrapKeepsSingleLine nowrap 折叠空白但不软换行：窄容器里仍是一行溢出。
func TestNowrapKeepsSingleLine(t *testing.T) {
	const words = "aaa bbb ccc ddd eee fff ggg hhh iii jjj"
	doc := openPreDoc(t, "<!doctype html><html><body style=\"margin:0\">\n"+
		"<div id=\"nw\" style=\"margin:0;width:100px;white-space:nowrap\">"+words+"</div>\n"+
		"<div id=\"nm\" style=\"margin:0;width:100px\">"+words+"</div>\n"+
		"</body></html>")
	if n := len(distinctLines(runsOf(t, doc, "#nw"))); n != 1 {
		t.Errorf("nowrap 排成 %d 行, want 1 行", n)
	}
	if n := len(distinctLines(runsOf(t, doc, "#nm"))); n < 2 {
		t.Errorf("normal 排成 %d 行, want ≥2 行（同文本应软换行，测试前提）", n)
	}
}

// TestInlineCodeCollapsesSpaces 行内 code 的 white-space 是 normal：源码里的
// 连续空白折叠为一个空格（只有 <pre> 才保留）。
func TestInlineCodeCollapsesSpaces(t *testing.T) {
	doc := openPreDoc(t, `<!doctype html><html><body style="margin:0">
		<p>前<code id="wide">x  y</code>后</p>
		<p>前<code id="narrow">x y</code>后</p>
	</body></html>`)
	b := mustBox(t, doc, "#wide")
	if b.computed != nil && b.computed.WhiteSpace() != WhiteSpaceNormal {
		t.Errorf("行内 code white-space = %v, want normal", b.computed.WhiteSpace())
	}
	w1, _ := boxSize(t, doc, "#wide")
	w2, _ := boxSize(t, doc, "#narrow")
	if w1 != w2 {
		t.Errorf("行内 code 里双空格宽 %d != 单空格宽 %d（应折叠）", w1, w2)
	}
}

// TestBrInsidePreBreaksLine <pre> 里的 <br> 与源换行同样强制断行。
func TestBrInsidePreBreaksLine(t *testing.T) {
	doc := openPreDoc(t, `<!doctype html><html><body style="margin:0">
		<pre id="p" style="margin:0">a<br>b</pre>
	</body></html>`)
	if n := len(distinctLines(runsOf(t, doc, "#p"))); n != 2 {
		t.Errorf("<pre>a<br>b</pre> 排成 %d 行, want 2 行", n)
	}
}
