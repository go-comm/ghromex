//go:build windows

package sdl2

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Noto CJK VF 静态字重实例（会议纪要页 CJK 与 Chrome 差三档的回归）
//
// 根因（CDP getPlatformFontsForNode + fonttools 实测）：Google 的
// NotoSansSC-VF.ttf fvar 默认字重是 Thin(100)，Chrome 经 DirectWrite 按
// 命名实例取面（正文 400、600/700 就近 700 Bold），而 SDL2_ttf 读不了 VF
// 实例——直接打开渲染 Thin 正文 + Thin 合成加粗，标题与正文都比 Chrome
// 细、灰。修复：tools/gen-cjk-instances.ps1 把 400/700 实例化成静态面放
// libs/fonts（gitignore），引擎侧 cjkNotoInstance 优先取静态 Regular、
// boldVariants 接粗体档。
// ---------------------------------------------------------------------------

// TestCJKStaticWeightInstances 锁三段选面链：正体档解析到静态 Regular、
// 半粗/粗档解析到静态 Bold（Chrome 对无 600 面的 VF 同样 600→700 就近）、
// 正体档不再打开 VF 本体（Thin）。未生成静态面时跳过（生成命令见报错）。
func TestCJKStaticWeightInstances(t *testing.T) {
	vf := fontPathOf("NotoSansSC-VF.ttf")
	g := NewGraphics(0, "")
	defer g.CloseFonts()
	if vf == "" {
		// 系统没有 Noto VF：本机 Chrome 的 CJK 不会落 Noto，引擎也不许
		// 因 libs/fonts 的捆绑面而改选（门控只看系统字体目录）。
		if p := g.cjkFallbackPath("sans"); strings.Contains(strings.ToLower(filepath.Base(p)), "noto") {
			t.Errorf("系统无 Noto VF 时 sans 回退解析到 %s（应为雅黑等 g.cjkFont）", filepath.Base(p))
		}
		return
	}
	reg := fontPathOf("NotoSansSC-Regular.ttf")
	if reg == "" {
		t.Skip("未生成静态字重面：powershell -ExecutionPolicy Bypass -File tools\\gen-cjk-instances.ps1")
	}
	if got := filepath.Base(g.cjkFallbackPath("sans")); got != "NotoSansSC-Regular.ttf" {
		t.Errorf("sans 回退正体解析到 %s, want NotoSansSC-Regular.ttf（VF 默认面是 Thin，不能直接用）", got)
	}
	for _, tier := range []int{tierSemibold, tierBold} {
		if got := filepath.Base(g.facePath(reg, 32, tier)); got != "NotoSansSC-Bold.ttf" {
			t.Errorf("静态 Regular tier=%d 解析到 %s, want NotoSansSC-Bold.ttf", tier, got)
		}
		// 显式点名 VF 文件的老路径同样要接住粗体档
		if got := filepath.Base(g.facePath(vf, 32, tier)); got != "NotoSansSC-Bold.ttf" {
			t.Errorf("VF tier=%d 解析到 %s, want NotoSansSC-Bold.ttf", tier, got)
		}
	}
	if got := filepath.Base(g.facePath(reg, 32, tierRegular)); got != "NotoSansSC-Regular.ttf" {
		t.Errorf("正体档解析到 %s, want 保持 NotoSansSC-Regular.ttf", got)
	}
}

// TestMeasureCJKWeightRealFaces 端到端度量口径：同一 CJK 串在 400 与 700
// 档经 MeasureTextWeight 走生产选面链，700 档必须落到 Bold 面——度量与
// 绘制同口径（facePath 与 face 共用变体解析），且 legacy bold=true 与
// 700 档同宽。CJK advance 恒为 1em，宽度对字重不敏感，这里锁的是
// **高度/行盒**：Bold 面的 ascent+descent 与 Regular 相同（同源实例化），
// 因此真正的断言是「两次调用都成功且非零」+ 选面链由上一个测试钉住。
func TestMeasureCJKWeightRealFaces(t *testing.T) {
	if err := Load(); err != nil {
		t.Skip(err)
	}
	if ttfInit() != 0 {
		t.Skip("TTF_Init 失败")
	}
	defer ttfQuit()
	if fontPathOf("NotoSansSC-Regular.ttf") == "" {
		t.Skip("未生成静态字重面：powershell -ExecutionPolicy Bypass -File tools\\gen-cjk-instances.ps1")
	}
	g := NewGraphics(0, "")
	defer g.CloseFonts()
	const text = "会议纪要"
	w400, h400 := g.MeasureTextWeight(text, 32, 400, "")
	w700, h700 := g.MeasureTextWeight(text, 32, 700, "")
	if w400 <= 0 || w700 <= 0 {
		t.Fatalf("CJK 度量失败: w400=%d w700=%d", w400, w700)
	}
	if w400 != w700 || h400 != h700 {
		t.Errorf("CJK 同字号 400/700 档度量应同为 1em×行高（(%d,%d) vs (%d,%d)）——出现差异说明选面链变化需要复核", w400, h400, w700, h700)
	}
	if wb, _ := g.MeasureText(text, 32, true, ""); wb != w700 {
		t.Errorf("legacy bold 宽 %d ≠ 700 档 %d", wb, w700)
	}
}
