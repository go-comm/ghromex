//go:build windows

package sdl2

import (
	"os"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// ---------------------------------------------------------------------------
// 首帧稳定性回归
//
// 用户反馈：demo/form-ua.html 点"城市" select，顶部"用户注册"标题有细微位移。
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
//  1. TestFreshFaceBoldAppliedImmediately：face 首个请求即 bold，度量必须与
//     "显式 TTF_SetFontStyle(BOLD) 的参照"一致（钉住 face 的记账语义）；
//  2. TestFirstFrameMatchesRelayout：真实字体下首帧与纯重排帧逐字调用、
//     逐像素一致；点 select 展开浮层不得改动顶部标题区（原始反馈场景）。
// ---------------------------------------------------------------------------

// firstFramePage 复现最小页：首段文字即粗体 CJK 标题 + 非粗体正文（翻转
// 标记的触发者）+ 可点击的 select。
const firstFramePage = `<!doctype html><html><head><style>
body { margin: 0; }
</style></head><body>
<h2>用户注册</h2>
<p>说明文字：正文非粗体，会让 bold 标记翻转。</p>
<form><label>城市 <select id="city"><option value="sh">上海</option><option value="bj">北京</option></select></label></form>
</body></html>`

// TestFreshFaceBoldAppliedImmediately 见文件头注释。
func TestFreshFaceBoldAppliedImmediately(t *testing.T) {
	initSDLTTF(t)
	defer ttfQuit()

	const size = 24
	// 单字而非整串：TTF_SizeUTF8 对整串与逐字的舍入口径不同，且 style 翻转
	// 在单字步进上最敏感（regular 24 → bold 26，实测 msyh@24px），
	// 整串值首测即稳定、抓不住该回归。
	text := "用"

	// 参照：直接按目标字号打开、按生产同序设置 hinting 与 bold——正确答案。
	// hinting 必须与 face() 一致（LIGHT/LIGHT_SUBPIXEL 各自改变字形步进，
	// 见 TestSetFontSizePreservesHinting 的 163 vs 165）。
	ref := openTTF(t, CJKFontPath(), size)
	defer ttfCloseFont(ref)
	if hasTTFHinting {
		hint := uintptr(hintLight)
		if lcdText() {
			hint = uintptr(hintLightSubpixel)
		}
		ttfSetFontHinting(ref, hint)
	}
	ttfSetFontStyle(ref, ttfStyleBold)
	wRef, hRef := sizeTTF(t, ref, text)

	g := NewGraphics(0, "") // MeasureText 不用 renderer，0 即可
	defer g.CloseFonts()

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

// TestFirstFrameMatchesRelayout 见文件头注释。
func TestFirstFrameMatchesRelayout(t *testing.T) {
	if os.Getenv("GHROMEX_REAL") != "1" {
		os.Setenv("SDL_VIDEO_DRIVER", "dummy")
		defer os.Unsetenv("SDL_VIDEO_DRIVER")
	}

	win, err := NewWindow(1024, 720, "first-frame")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	doc, err := win.OpenDocument(firstFramePage)
	if err != nil {
		t.Fatal(err)
	}
	sel := doc.QuerySelector("#city")
	if sel == nil {
		t.Fatal("select not found")
	}

	// 渲染一帧并记录 DrawText 调用序列（x,y,w,h,size,bold,fam,text）
	capture := func() ([]byte, []string) {
		r := &seqG{inner: win.g}
		win.clearCanvas()
		engine.RenderNode(r, doc)
		win.present()
		buf, _, vw, vh, err := win.Screenshot()
		if err != nil {
			t.Fatalf("Screenshot: %v", err)
		}
		if vw != 1024 || vh != 720 {
			t.Fatalf("截图尺寸 %dx%d", vw, vh)
		}
		return buf, r.texts
	}
	diffCount := func(a, b []byte, y0, y1 int) int {
		n := 0
		for y := y0; y < y1; y++ {
			for x := 0; x < 1024; x++ {
				i := (y*1024 + x) * 4
				if a[i] != b[i] || a[i+1] != b[i+1] ||
					a[i+2] != b[i+2] || a[i+3] != b[i+3] {
					n++
				}
			}
		}
		return n
	}

	// 1) 首帧 vs 纯重排帧：标题随度量口径位移就在这里暴露
	shot1, seq1 := capture()
	engine.LayoutDocument(doc)
	shot2, seq2 := capture()
	if n := diffCount(shot1, shot2, 0, 720); n != 0 {
		t.Errorf("首帧与纯重排帧像素不一致: %d 个像素（标题会“点一下才挪正”）", n)
	}
	if len(seq1) != len(seq2) {
		t.Fatalf("DrawText 调用数 %d → %d", len(seq1), len(seq2))
	}
	for i := range seq1 {
		if seq1[i] != seq2[i] {
			t.Errorf("DrawText #%d 变化:\n  首帧: %s\n  重排: %s", i, seq1[i], seq2[i])
			break
		}
	}

	// 2) 原始反馈场景：点"城市"展开浮层 → 标题带（h2 在 y<60）必须不动，
	//    且浮层确实画出来了（否则"顶部没变"是空过的）
	r := sel.GetBoundingClientRect()
	engine.OnDocumentClick(doc,
		(r.Left().Pixel()+r.Right().Pixel())/2,
		(r.Top().Pixel()+r.Bottom().Pixel())/2)
	engine.LayoutDocument(doc)
	shot3, _ := capture()
	if n := diffCount(shot2, shot3, 0, 60); n != 0 {
		t.Errorf("点击 select 后顶部 60 行（标题带）变化 %d 像素", n)
	}
	if n := diffCount(shot2, shot3, 0, 720); n == 0 {
		t.Error("全页零差异：select 浮层没画出来，顶部断言空过")
	}
}

// seqG 记录 DrawText 调用参数（比像素更直接的诊断）。
type seqG struct {
	inner engine.Graphics
	texts []string
}

func (r *seqG) DrawText(x, y, w, h int, p engine.Paint, s string) {
	size := 0
	bold := false
	fam := ""
	if p != nil {
		if sz := p.Size(); sz != nil {
			size = sz.Pixel()
		}
		bold = p.Bold()
		fam = p.FontFamily()
	}
	r.texts = append(r.texts, itoa(x)+","+itoa(y)+","+itoa(w)+","+itoa(h)+
		" size="+itoa(size)+" bold="+boolStr(bold)+" fam="+fam+" "+s)
	r.inner.DrawText(x, y, w, h, p, s)
}

func (r *seqG) DrawColor(x, y, w, h int, c engine.Color) {
	r.inner.DrawColor(x, y, w, h, c)
}

func (r *seqG) DrawImage(x, y, w, h int, img engine.Image) {
	r.inner.DrawImage(x, y, w, h, img)
}

func (r *seqG) MeasureText(s string, size int, bold bool, family string) (int, int) {
	return r.inner.MeasureText(s, size, bold, family)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
