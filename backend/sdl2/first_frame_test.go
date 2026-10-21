//go:build windows

package sdl2

import (
	"testing"
)

// ---------------------------------------------------------------------------
// 首帧稳定性回归（后端这一半）
//
// 用户反馈：examples/ua/index.html 点"城市" select，顶部"用户注册"标题有细微位移。
// 实测定位：布局盒不变，差异只在 h2 四个字的 DrawText 宽度/位置
// （0,24,48,73 → 0,26,51,78）——首帧与"首次重排后"的帧不一致，点击只是
// 触发了那次重排。
//
// 根因（graphics.go face）：f.bold 原先记请求值而非 handle 实际样式。
// TTF_OpenFont 初始恒为 regular，首个请求即 bold 时样式切换被
// "f.bold == bold" 短路跳过，handle 一直按 regular 度量/绘制，直到某次非
// bold 请求翻转标记、再来一次 bold 才纠正——那次翻转通常就在首次点击触发的
// 重排里。页面首段文字是粗体标题（本页 h2）时必现。
//
// 两条锁：
//  1. TestFreshFaceBoldAppliedImmediately（在本文件）：face 首个请求即 bold，
//     度量必须与"显式 TTF_SetFontStyle(BOLD) 的参照"一致（钉住 face 的
//     记账语义）——后端层，不涉及文档；
//  2. TestFirstFrameMatchesRelayout（在 browser/first_frame_test.go）：真实
//     字体下首帧与纯重排帧逐字调用、逐像素一致；点 select 展开浮层不得改动
//     顶部标题区（原始反馈场景）——那一半要有文档与主循环，归 browser 层。
// ---------------------------------------------------------------------------

// TestFreshFaceBoldAppliedImmediately 见文件头注释。
func TestFreshFaceBoldAppliedImmediately(t *testing.T) {
	initSDLTTF(t)
	defer ttfQuit()

	const size = 24
	// 单字而非整串：TTF_SizeUTF8 对整串与逐字的舍入口径不同，且 style 翻转
	// 在单字步进上最敏感（regular 24 → bold 26，实测 msyh@24px），
	// 整串值首测即稳定、抓不住该回归。
	text := "用"

	g := NewGraphics(0, "") // MeasureText 不用 renderer，0 即可
	defer g.CloseFonts()

	// 参照：直接按目标字号打开、按生产同序设置 hinting 与 bold——正确答案。
	// hinting 必须与 face() 一致（fontHinting()：各档位改变字形步进，
	// 见 TestSetFontSizePreservesHinting 的 163 vs 165）。
	// 文件与粗细也必须与生产同源，且由生产的同一套选择逻辑给出
	// （cjkFontPathFor + facePath）：family 为空时生产把 CJK 段落到引擎
	// standard（Arial）的 CJK 回退字体（实测 Noto Sans SC，而非雅黑），照旧钉
	// CJKFontPath()（雅黑 + msyhbd）会让参照与生产取到不同字体，测出来的是
	// "字体不同"而不是"bold 没落到 handle 上"。生产对粗体优先用**真粗体字
	// 文件**且不再叠加 TTF_STYLE_BOLD；无真粗体文件时才用常规文件 + 合成加粗
	// （facePath 返回与 base 相同即意味着走合成加粗）。
	base := g.cjkFontPathFor("")
	refPath := g.facePath(base, size, true)
	syntheticBold := refPath == base
	ref := openTTF(t, refPath, size)
	defer ttfCloseFont(ref)
	if hasTTFHinting {
		ttfSetFontHinting(ref, fontHinting())
	}
	if syntheticBold {
		ttfSetFontStyle(ref, ttfStyleBold)
	}
	wRef, hRef := sizeTTF(t, ref, text)

	// 新 face 的首个请求就是 bold：必须直接拿到 bold 口径
	wFirst, hFirst := g.MeasureText(text, size, true, "")
	// 再经 非bold→bold 往返后复测（旧 bug 下此时才是 bold 口径）
	g.MeasureText(text, size, false, "")
	wSteady, hSteady := g.MeasureText(text, size, true, "")

	if wFirst != wRef || hFirst != hRef {
		t.Fatalf("首个 bold 请求度量 (%d,%d) ≠ 显式 bold 参照 (%d,%d)："+
			"face 建立时 bold 没落到 handle 上（首帧按 regular 度量）",
			wFirst, hFirst, wRef, hRef)
	}
	if wFirst != wSteady || hFirst != hSteady {
		t.Fatalf("首个 bold 请求 (%d,%d) ≠ 样式往返后 (%d,%d)："+
			"度量依赖样式切换历史，首帧与重排帧必然不一致",
			wFirst, hFirst, wSteady, hSteady)
	}
}
