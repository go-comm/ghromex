package engine

import "testing"

// TestParseImportRef 覆盖 @import 的写法：引号、url()、媒体查询后缀。
func TestParseImportRef(t *testing.T) {
	ok := map[string]string{
		`@import "x.css"`:                               "x.css",
		`@import 'x.css'`:                               "x.css",
		`@import url(x.css)`:                            "x.css",
		`@import url("x.css")`:                          "x.css",
		`@import url( 'x.css' )`:                        "x.css",
		`@IMPORT "x.css"`:                               "x.css",
		`@import url("x.css") screen`:                   "x.css",
		`@import "x.css" screen and (min-width: 600px)`: "x.css",
	}
	for stmt, want := range ok {
		got, found := parseImportRef(stmt)
		if !found || got != want {
			t.Errorf("parseImportRef(%q) = %q,%v, want %q,true", stmt, got, found, want)
		}
	}
	for _, stmt := range []string{
		`@charset "utf-8"`,
		`@import`,
		`@import url(`,
		`@import "x.css`,
		`body { color: red }`,
		``,
	} {
		if got, found := parseImportRef(stmt); found {
			t.Errorf("parseImportRef(%q) = %q,true, want false", stmt, got)
		}
	}
}

// TestParseCSSRulesKeepsRulesAfterImport 是扫描器的回归：@import 是一条
// 以分号结束的语句，必须单独收掉。旧实现把它和下一条规则拼成
// `@import url(x.css); body` 一起当 at-rule 跳过 → @import 之后的规则
// 全部不生效（页面看起来「样式只加载了一半」）。
func TestParseCSSRulesKeepsRulesAfterImport(t *testing.T) {
	rules, imports := parseCSSRules(`
		@import url("a.css");
		@import 'b.css' screen;
		body { color: red; }
		@media print { body { color: blue; } }
		p, span { margin: 0; }
	`)
	if len(imports) != 2 || imports[0] != "a.css" || imports[1] != "b.css" {
		t.Fatalf("imports = %v, want [a.css b.css]", imports)
	}
	// body + p + span（逗号列表拆成两条），@media 里的规则整块跳过。
	if len(rules) != 3 {
		t.Fatalf("规则数 = %d, want 3（@media 内的规则不应计入）", len(rules))
	}
	if rules[0].order != 0 || rules[1].order != 1 || rules[2].order != 2 {
		t.Errorf("order 未按源码顺序编号：%d/%d/%d", rules[0].order, rules[1].order, rules[2].order)
	}
}

// TestParseCSSRulesSkipsMultiRuleMediaBlock 回归（会议纪要页调试）：
// @media 块内含多条规则时，旧实现只跳到块内**第一个**「}」，第二条起的
// 规则被提升成顶层规则应用——prefers-color-scheme: dark 的暗色表格样式
// 漏进亮色页面渲染。带块 at-rule 必须按大括号深度配平整块跳过。
func TestParseCSSRulesSkipsMultiRuleMediaBlock(t *testing.T) {
	rules, _ := parseCSSRules(`
		body { color: black; }
		@media (prefers-color-scheme: dark) {
			table tr { background-color: #0d1117; }
			table td { border-color: #3d444d; }
			pre { color: #e6edf3; }
		}
		h1 { font-size: 32px; }
	`)
	if len(rules) != 2 {
		t.Fatalf("规则数 = %d, want 2（@media 内 3 条整块跳过，块后的 h1 保留）", len(rules))
	}
	if s := rules[1].sels[0]; len(s.compounds) != 1 || s.compounds[0].tag != "h1" {
		t.Errorf("块后规则不是 h1：%+v", s.compounds)
	}
}

// TestFontWeightMapping 回归（会议纪要页调试）：GitHub markdown 的标题与
// 表格 th 用 font-weight:600，旧解析只把 700/800/900 判为粗体，600 落成
// normal——作者 600 覆盖 UA 的 bold 后整页标题全画成细体。引擎只有
// 正/粗两档字面，按 Chrome 就近选面归档：550 及以上取粗，500 及以下取正；
// bolder/lighter 相对继承值升降（Normal→bolder=Bold，Bold→bolder=Bold，
// Bold→lighter=Normal，Normal→lighter=Normal；近似到无条件归档结果一致）。
func TestFontWeightMapping(t *testing.T) {
	cases := []struct {
		value string
		want  FontWeight
	}{
		{"normal", FontWeightNormal}, {"400", FontWeightNormal},
		{"500", FontWeightNormal}, {"lighter", FontWeightNormal},
		{"bold", FontWeightBold}, {"bolder", FontWeightBold},
		{"550", FontWeightBold}, {"600", FontWeightBold},
		{"700", FontWeightBold}, {"800", FontWeightBold}, {"900", FontWeightBold},
	}
	for _, c := range cases {
		st := newStyle()
		applyDecl(st, "font-weight", c.value)
		if got := st.FontWeight(); got != c.want {
			t.Errorf("font-weight:%s → %v, want %v", c.value, got, c.want)
		}
	}

	// bolder/lighter 是相对值：继承值不同，结果不同。
	st := newStyle()
	st.SetFontWeight(FontWeightBold)
	applyDecl(st, "font-weight", "lighter")
	if st.FontWeight() != FontWeightNormal {
		t.Errorf("Bold + lighter = %v, want Normal", st.FontWeight())
	}

	// 非法值（var() 未展开、拼写错误）按 CSS 语义丢弃，不覆盖继承值。
	st = newStyle()
	st.SetFontWeight(FontWeightBold)
	applyDecl(st, "font-weight", "var(--x)")
	if st.FontWeight() != FontWeightBold {
		t.Errorf("非法值应保留继承的 Bold，实际被覆盖为 %v", st.FontWeight())
	}
}

// TestFontWeightNumericStorage 回归（会议纪要页拉丁 600 修复）：两档
// FontWeight 归档之外必须把 font-weight 的**数值**原值存进
// fontWeightNum——后端要靠它区分 600/700（Semibold vs Bold 面）。
// bolder/lighter 同时给出数值（bolder：继承值 ≥650→900，否则→700，
// 0=UA 直接 bold 视作 400 起跳；lighter→400，与两档结论一致）。
func TestFontWeightNumericStorage(t *testing.T) {
	cases := []struct {
		value string
		want  int
	}{
		{"normal", 400}, {"400", 400}, {"500", 500}, {"lighter", 400},
		{"bold", 700}, {"550", 550}, {"600", 600}, {"650", 650},
		{"700", 700}, {"900", 900},
	}
	for _, c := range cases {
		st := newStyle()
		applyDecl(st, "font-weight", c.value)
		if got := fontWeightNumOf(st); got != c.want {
			t.Errorf("font-weight:%s 数值 = %d, want %d", c.value, got, c.want)
		}
	}

	// bolder 按继承数值升档。
	for _, c := range []struct{ from, want int }{
		{0, 700}, {400, 700}, {600, 700}, {650, 900}, {700, 900},
	} {
		st := newStyle()
		setFontWeightNumOf(st, c.from)
		applyDecl(st, "font-weight", "bolder")
		if got := fontWeightNumOf(st); got != c.want {
			t.Errorf("bolder 从 %d = %d, want %d", c.from, got, c.want)
		}
	}

	// 非法值同样不覆盖数值（与两档归档的丢弃语义一致）。
	st := newStyle()
	setFontWeightNumOf(st, 600)
	applyDecl(st, "font-weight", "var(--x)")
	if got := fontWeightNumOf(st); got != 600 {
		t.Errorf("非法值应保留继承的 600，实际 %d", got)
	}

	// font-weight 是继承属性：子元素未声明时数值随两档一起继承
	//（否则父级 600 在子级退化成初始档，Semibold 丢失）。
	parent := newStyle()
	applyDecl(parent, "font-weight", "600")
	child := newStyle()
	applyInheritedStyle(child, parent)
	if got := fontWeightNumOf(child); got != 600 {
		t.Errorf("继承后数值 = %d, want 600", got)
	}
	// 子级声明 normal 必须复位为 400（不能保留父级 600）。
	applyDecl(child, "font-weight", "normal")
	if got := fontWeightNumOf(child); got != 400 {
		t.Errorf("子级 normal 数值 = %d, want 400", got)
	}

	// 样式克隆也要带上数值（级联快照路径）。
	src := newStyle()
	applyDecl(src, "font-weight", "600")
	dst := copyStyle(src)
	if got := fontWeightNumOf(dst); got != 600 {
		t.Errorf("克隆后数值 = %d, want 600", got)
	}
}
