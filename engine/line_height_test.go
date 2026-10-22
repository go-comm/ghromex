package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// lineHeightDoc 打开一个只有一段文字的文档，返回文档与段落元素。
func lineHeightDoc(t *testing.T, style string) (engine.HTMLDocument, engine.HTMLElement) {
	t.Helper()
	src := `<!doctype html><html><head></head><body style="margin:0">
		<p id="p" style="margin:0;` + style + `">Hi</p>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	return doc, mustEl(t, doc, "#p")
}

// FakeGraphics 的行度量：16px 字号 -> ascent 10 / descent 10（自然行高 20）。
const (
	fakeAsc16  = 10
	fakeDesc16 = 10
)

// line-height:40px -> 行盒高 40（行距 20 一半在上、一半在下）。
// 基线落在距行顶 20 处（asc = 10 + half = 10 + 10）。
func TestLineHeightLengthSetsLineBoxHeight(t *testing.T) {
	_, p := lineHeightDoc(t, "line-height:40px")
	if p.Height() != 40 {
		t.Errorf("段落高 = %d, want 40（line-height:40px）", p.Height())
	}
}

// line-height:2 -> 行高 = 2 倍字号 = 32（倍数相对**元素自身**字号）。
func TestLineHeightFactorSetsLineBoxHeight(t *testing.T) {
	_, p := lineHeightDoc(t, "line-height:2")
	if p.Height() != 32 {
		t.Errorf("段落高 = %d, want 32（line-height:2 乘 16px 字号）", p.Height())
	}
}

// line-height:150% 与 line-height:1.5 同义（都相对自身字号）。
func TestLineHeightPercentEqualsFactor(t *testing.T) {
	_, pct := lineHeightDoc(t, "line-height:150%")
	_, factor := lineHeightDoc(t, "line-height:1.5")
	if pct.Height() != 24 {
		t.Errorf("段落高 = %d, want 24（line-height:150%% 乘 16px）", pct.Height())
	}
	if pct.Height() != factor.Height() {
		t.Errorf("150%% 得 %d，1.5 得 %d——两者应同义", pct.Height(), factor.Height())
	}
}

// 倍数继承的是**倍数本身**：父级 line-height:1.5 + 子级 font-size:32px
// -> 子级行高 48px（继承成像素就锁死在父级字号上了）。
func TestLineHeightInheritedFactorUsesOwnFontSize(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div style="line-height:1.5;font-size:16px">
			<span id="s" style="font-size:32px">x</span>
		</div>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	s := mustEl(t, doc, "#s")
	if s.Height() != 48 {
		t.Errorf("span 高 = %d, want 48（1.5 乘自身 32px 字号）", s.Height())
	}
}

// 绝对长度行高按声明值继承（不随字号变化）。
func TestLineHeightInheritedLengthIsAbsolute(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div style="line-height:40px;font-size:16px">
			<span id="s" style="font-size:32px">x</span>
		</div>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	s := mustEl(t, doc, "#s")
	if s.Height() != 40 {
		t.Errorf("span 高 = %d, want 40（绝对行高不随字号变）", s.Height())
	}
}

// 未声明 line-height 时行为不变：行盒高 = 字体自然行高（16px 字号 -> 20）。
func TestLineHeightNormalKeepsFontLineHeight(t *testing.T) {
	_, p := lineHeightDoc(t, "")
	if p.Height() != fakeAsc16+fakeDesc16 {
		t.Errorf("段落高 = %d, want %d（normal 取字体自然行高）",
			p.Height(), fakeAsc16+fakeDesc16)
	}
}

// line-height:normal 显式声明等同不声明。
func TestLineHeightNormalKeyword(t *testing.T) {
	_, p := lineHeightDoc(t, "line-height:normal")
	if p.Height() != fakeAsc16+fakeDesc16 {
		t.Errorf("段落高 = %d, want %d（line-height:normal）",
			p.Height(), fakeAsc16+fakeDesc16)
	}
}

// 非法值整条声明被忽略（保留继承来的值），不是当成 0 行高。
func TestLineHeightInvalidValueIgnored(t *testing.T) {
	_, p := lineHeightDoc(t, "line-height:-3")
	if p.Height() != fakeAsc16+fakeDesc16 {
		t.Errorf("段落高 = %d, want %d（负行高应整条忽略）",
			p.Height(), fakeAsc16+fakeDesc16)
	}
	_, p2 := lineHeightDoc(t, "line-height:abc")
	if p2.Height() != fakeAsc16+fakeDesc16 {
		t.Errorf("段落高 = %d, want %d（非法值应整条忽略）",
			p2.Height(), fakeAsc16+fakeDesc16)
	}
}

// 行高变大时基线随之下移（文字在行盒内垂直居中），行盒高度严格等于
// 声明的 line-height。基线位置从 SVG 导出里读。
func TestLineHeightMovesBaselineWithinLineBox(t *testing.T) {
	doc, _ := lineHeightDoc(t, "line-height:40px")
	svg := doc.DumpSVG()
	y := svgTextBaselineY(t, svg)
	// 基线 = 行顶 + ascent = 10 + half(10) = 20
	if y != 20 {
		t.Errorf("基线 y = %d, want 20（行盒 40 高，文字垂直居中）", y)
	}
}

// 多行文本：每行都按声明的行高排（行盒高度累加）。
func TestLineHeightAppliesToEveryLine(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<p id="p" style="margin:0;line-height:20px;width:40px">Hi</p>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	p := mustEl(t, doc, "#p")
	// "Hi" 在 40px 宽里放不下（FakeGraphics 每字符 16*3/5=9px，两字符 18px），
	// 这里只断言行高是 20 的整数倍。
	if p.Height() <= 0 || p.Height()%20 != 0 {
		t.Errorf("段落高 = %d，不是 20 的整数倍（每行 20px）", p.Height())
	}
}

// line-height 小于字体自然行高时：文字溢出行盒（descent 为负），
// 行盒高度仍等于声明值。
func TestLineHeightSmallerThanFontOverflows(t *testing.T) {
	_, p := lineHeightDoc(t, "line-height:10px")
	if p.Height() != 10 {
		t.Errorf("段落高 = %d, want 10（行高小于字体自然行高时行盒仍是 10）", p.Height())
	}
}

// line-height:0 是合法值（行盒高度 0，文字全溢出）。
func TestLineHeightZero(t *testing.T) {
	_, p := lineHeightDoc(t, "line-height:0")
	if p.Height() != 0 {
		t.Errorf("段落高 = %d, want 0（line-height:0）", p.Height())
	}
}

// svgTextBaselineY 从 SVG 导出里取第一个 <text> 的基线 y。
func svgTextBaselineY(t *testing.T, svg string) int {
	t.Helper()
	all := svgTextBaselines(t, svg)
	if len(all) == 0 {
		t.Fatalf("SVG 里没有 <text>:\n%s", svg)
	}
	return all[0]
}

// svgTextBaselines 从 SVG 导出里取所有 <text> 的基线 y（按出现顺序）。
func svgTextBaselines(t *testing.T, svg string) []int {
	t.Helper()
	var out []int
	rest := svg
	for {
		i := strings.Index(rest, "<text x=\"")
		if i < 0 {
			return out
		}
		rest = rest[i+len("<text x=\""):]
		j := strings.Index(rest, " y=\"")
		if j < 0 {
			t.Fatalf("SVG 的 <text> 没有 y:\n%s", svg)
		}
		rest = rest[j+len(" y=\""):]
		k := strings.IndexByte(rest, '"')
		if k < 0 {
			t.Fatalf("SVG 的 <text> 的 y 没有闭合:\n%s", svg)
		}
		n := 0
		for _, c := range rest[:k] {
			if c < '0' || c > '9' {
				t.Fatalf("SVG 的 y 不是整数: %q", rest[:k])
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
		rest = rest[k:]
	}
}
