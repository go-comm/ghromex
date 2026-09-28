package sdl2

import (
	"path/filepath"
	"strings"
	"testing"
)

// family 解析对齐 Chrome Windows 默认映射：通用族、具名别名、逗号列表回退、未知回退 standard。
func TestResolveFontPathMatchesChrome(t *testing.T) {
	g := NewGraphics(0, "") // 不触 native；仅建映射表与路径回退
	cases := []struct{ family, wantSuffix string }{
		{"serif", `times.ttf`},
		{"SANS-SERIF", `arial.ttf`}, // 大小写不敏感
		{"monospace", `cour.ttf`},
		{"cursive", `comic.ttf`},
		{"fantasy", `impact.ttf`},
		{"system-ui", `segoeui.ttf`},
		{`"Times New Roman"`, `times.ttf`}, // 带引号具名
		{"Microsoft YaHei", `msyh.ttc`},
		{"宋体", `simsun.ttc`},
		// 不存在的第一个候选 → 继续列表 → 通用族兜底
		{"NoSuchFont, serif", `times.ttf`},
		// 全部未知 → standard（sans-serif Arial，或本机缺失时 DefaultFontPath 回退）
		{"NoSuchFont", filepath.Base(g.standardFont)},
	}
	for _, c := range cases {
		got := strings.ToLower(g.resolveFontPath(c.family))
		if !strings.HasSuffix(got, strings.ToLower(c.wantSuffix)) {
			t.Errorf("resolveFontPath(%q) = %q, want suffix %q", c.family, got, c.wantSuffix)
		}
	}
}

// 字符级脚本分段：测量与绘制共用，段拼接必须还原原文。
func TestSplitScripts(t *testing.T) {
	text := "Ghromex 登录 Page 2页"
	segs := splitScripts(text)
	var joined strings.Builder
	cjkCount := 0
	for _, s := range segs {
		joined.WriteString(s.text)
		if s.cjk {
			cjkCount++
		}
	}
	if joined.String() != text {
		t.Fatalf("segments rejoin mismatch: %q", joined.String())
	}
	// 期望 4 段：["Ghromex "]["登录"][" Page 2"]["页"]，其中 2 段 CJK
	if len(segs) != 4 {
		t.Fatalf("want 4 segments, got %d: %+v", len(segs), segs)
	}
	if cjkCount != 2 {
		t.Fatalf("want 2 cjk segments, got %d", cjkCount)
	}
	if !segs[1].cjk || segs[1].text != "登录" {
		t.Fatalf("segs[1] = %+v, want {登录, cjk}", segs[1])
	}
}

// 纯 CJK / 纯拉丁不应被切分；全角标点与汉字同段。
func TestSplitScriptsHomogeneous(t *testing.T) {
	if segs := splitScripts("登录"); len(segs) != 1 || !segs[0].cjk {
		t.Fatalf("pure cjk: %+v", segs)
	}
	if segs := splitScripts("Hello"); len(segs) != 1 || segs[0].cjk {
		t.Fatalf("pure latin: %+v", segs)
	}
	if segs := splitScripts("好，世界"); len(segs) != 1 || !segs[0].cjk {
		t.Fatalf("cjk punctuation should stay in cjk seg: %+v", segs)
	}
}

// 字体未加载（无 native 符号）时 MeasureText 走估算回退，不 panic。
func TestMeasureTextFallbackWithoutDLL(t *testing.T) {
	g := NewGraphics(0, "")
	w, h := g.MeasureText("AB", 16, false, "serif")
	// 估算：2×(16*3/5)=18 宽，16×13/10=20 高
	if w != 18 || h != 20 {
		t.Fatalf("estimate = (%d,%d), want (18,20)", w, h)
	}
}
