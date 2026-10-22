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
