//go:build windows

package sdl2

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"
)

// ---------------------------------------------------------------------------
// 字体渲染模式对比条（手动探针，GHROMEX_CMP=1 才运行）
//
// 背景：用户反馈小字号中文有"锯齿"。textaa 探针已量化：灰度过渡像素充足
// （AA 存在），但硬跳变占比偏高——根因是 FreeType NORMAL hinting 把笔画
// 吸附到像素网格，边缘锐、无 ClearType 式彩边。本测试对同一文本渲染 5 种
// 变体供人眼+数据对比，为最终选型（维持/LIGHT/超采样/LCD/加大字号）提供依据：
//
//	1 blended NORMAL   —— 现状：灰度 AA + 强 hinting（基线 hardV=161）
//	2 blended LIGHT    —— 弱网格吸附，边缘更柔
//	3 blended NONE     —— 无网格吸附，最柔但墨最虚（基线 hardV=8）
//	4 2x 超采样        —— 26px 渲染后 box 降采样到 13px
//	5 LCD 子像素       —— LIGHT_SUBPIXEL(4) + TTF_RenderUTF8_LCD，真子像素
//	                      分解（条纹守卫实测 fringe=1068）：白底上灰度合成的
//	                      亮度跨度被三通道拆开，横向硬跳变归零（hardH 12→0）
//	                      但代价是换非白背景合成会显彩边
//
// 输出：../../.temp/fontcmp_native.png（原始像素）与 fontcmp.png（4x 最近邻
// 放大，避免查看器双线性插值把锯齿抹平造成误判），并逐行打印与 textaa 同
// 口径的边缘指标（ink/bg/inter + 硬跳变/AA 边界计数）。
//
// 像素布局不靠记忆（两个旧假设均被实测推翻）：blended 并非 8bpp，实测
// pitch≈4w 的 32bpp surface；凭记忆硬编的 RGBA8888 枚举值实际是 UNKNOWN，
// SDL_ConvertSurface 方案废弃。覆盖通道用醒目色 (1,2,253) 渲染后取 4 个字
// 节位置中数值跨度最大者；LCD surface 用"墨体三指纹"判序——墨色 #0f172a=
// (15,23,42) 三分量互异，取前三字节全 ≤45 且极差 ≥15 的满覆盖像素与指纹
// 对齐锁定 R/G/B/A 字节位置（实测 BGRA/ARGB8888，任何布局统一成立）。
// 通道错位会改变彩边颜色，人眼对比必需顺序正确。
// ---------------------------------------------------------------------------

const (
	cmpProbeText = "登录工作台 Aa123 字体对比"
	cmpTextSize  = 13 // 正文小字号：锯齿问题的复现尺度
	cmpLabelSize = 12
)

// 墨色 #0f172a / 标签色 #334155 / 背景白，与 demo 正文配色一致。
var (
	cmpInk      = [4]uint8{15, 23, 42, 255}
	cmpLabelInk = [4]uint8{51, 65, 85, 255}
	cmpWhite    = [4]uint8{255, 255, 255, 255}
)

// 探针专用醒目色：R/G/B 互异且远离 0/255，用于"数值跨度判覆盖通道"。
var cmpProbeColor = [4]uint8{1, 2, 253, 255}

type cmpRow struct {
	name  string
	label string
	w, h  int
	rgba  []byte // 已合成白底的紧致 RGBA（w*h*4）
}

// TestHintingValueScan 扫描 TTF_SetFontHinting 取值 0..5：当前对比条里
// NONE(0x08) 输出与 NORMAL 逐字节相同、LIGHT(0x02) 反而更硬，怀疑常量集写错
// （把 FreeType FT_LOAD_TARGET 风格当成了 SDL2_ttf 的 TTF_HintingMode 枚举，
// 后者据传为 NORMAL=0 LIGHT=1 MONO=2 NONE=3 LIGHT_SUBPIXEL=4）。逐值渲染并
// 量化：MONO 的指纹是二值化（inter≈0）；NONE 应保留原始轮廓（与 NORMAL 不同
// 且 hard 更少）；结果用实测锁定，不凭文档记忆。
func TestHintingValueScan(t *testing.T) {
	if os.Getenv("GHROMEX_CMP") != "1" {
		t.Skip("hinting 取值扫描是手动探针，设置 GHROMEX_CMP=1 运行")
	}
	initSDLTTF(t)
	defer ttfQuit()
	if !hasTTFHinting {
		t.Skip("该 DLL 无 TTF_SetFontHinting")
	}
	cjk := CJKFontPath()
	var baseHash string
	for v := 0; v <= 5; v++ {
		f := openTTF(t, cjk, cmpTextSize)
		ttfSetFontHinting(f, uintptr(v))
		w, h, cov := cmpBlendedCoverage(t, f, cmpProbeText)
		rgba := cmpCompositeWhite(w, h, cov, cmpInk)
		hash := fmt.Sprintf("%x", sha256.Sum256(rgba))[:12]
		same := ""
		if v > 0 && hash == baseHash {
			same = " (=v0)"
		} else if v == 0 {
			baseHash = hash
		}
		t.Logf("[scan] v=%d %dx%d hash=%s%s | %s", v, w, h, hash, same, cmpMetricsLine(w, h, rgba))
		ttfCloseFont(f)
	}
}

// cmpMetricsLine 返回与 cmpAnalyze 同口径的单行指标文本（不打印）。
func cmpMetricsLine(w, h int, rgba []byte) string {
	if w <= 4 || h <= 4 {
		return "(too small)"
	}
	lum := func(x, y int) int {
		if x < 0 || y < 0 || x >= w || y >= h {
			return 255
		}
		o := (y*w + x) * 4
		return (int(rgba[o])*299 + int(rgba[o+1])*587 + int(rgba[o+2])*114) / 1000
	}
	cls := func(x, y int) int {
		l := lum(x, y)
		switch {
		case l <= 60:
			return 1
		case l >= 250:
			return -1
		default:
			return 0
		}
	}
	ink, bg, inter, hardV, aaV, hardH, aaH := 0, 0, 0, 0, 0, 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			switch cls(x, y) {
			case 0:
				inter++
			case 1:
				ink++
			case -1:
				bg++
			}
		}
	}
	hasX := func(x, y int) bool {
		return cls(x-1, y) == 0 || cls(x-2, y) == 0 || cls(x+1, y) == 0 || cls(x+2, y) == 0
	}
	hasY := func(x, y int) bool {
		return cls(x, y-1) == 0 || cls(x, y-2) == 0 || cls(x, y+1) == 0 || cls(x, y+2) == 0
	}
	for y := 2; y < h-2; y++ {
		for x := 2; x < w-2; x++ {
			if p, n := cls(x-1, y), cls(x, y); p != 0 && n != 0 && p != n {
				if hasX(x, y) {
					aaH++
				} else {
					hardH++
				}
			}
			if p, n := cls(x, y-1), cls(x, y); p != 0 && n != 0 && p != n {
				if hasY(x, y) {
					aaV++
				} else {
					hardV++
				}
			}
		}
	}
	return fmt.Sprintf("ink=%5d bg=%6d inter=%5d | hardH=%4d aaH=%4d | hardV=%4d aaV=%4d", ink, bg, inter, hardH, aaH, hardV, aaV)
}

func TestFontCompareStrip(t *testing.T) {
	if os.Getenv("GHROMEX_CMP") != "1" {
		t.Skip("字体渲染对比条是手动探针，设置 GHROMEX_CMP=1 运行")
	}
	initSDLTTF(t)
	defer ttfQuit()
	if !hasTTFHinting {
		t.Skip("该 DLL 无 TTF_SetFontHinting，无法对比 hinting")
	}
	if !hasTTFLCD {
		t.Skip("该 DLL 无 TTF_RenderUTF8_LCD，无法对比子像素")
	}

	cjk := CJKFontPath()
	t.Logf("[cmp] cjk=%s text=%q size=%dpx", filepath.Base(cjk), cmpProbeText, cmpTextSize)

	var rows []cmpRow

	font := openTTF(t, cjk, cmpTextSize)
	defer ttfCloseFont(font)

	// 1-3：blended，不同 hinting。face 复用合同下 hinting 是 face 状态，
	// 每次渲染前显式设置，不依赖初值。
	for _, hv := range []struct {
		name, label string
		hint        int
	}{
		{"blended-NORMAL", "1 blended NORMAL (current)", hintNormal},
		{"blended-LIGHT", "2 blended LIGHT", hintLight},
		{"blended-NONE", "3 blended NONE", hintNone},
	} {
		ttfSetFontHinting(font, uintptr(hv.hint))
		w, h, cov := cmpBlendedCoverage(t, font, cmpProbeText)
		rows = append(rows, cmpRow{name: hv.name, label: hv.label, w: w, h: h, rgba: cmpCompositeWhite(w, h, cov, cmpInk)})
	}

	// 4：2x 超采样。直接按 26px 建 face（SetFontSize 路径另有回归锁），
	// LIGHT hinting 下 2x 渲染再 box 降采样——alpha 线性，降采样即等效超采样。
	f26 := openTTF(t, cjk, cmpTextSize*2)
	defer ttfCloseFont(f26)
	ttfSetFontHinting(f26, uintptr(hintLight))
	w2, h2, cov2 := cmpBlendedCoverage(t, f26, cmpProbeText)
	dw, dh, covD := cmpDownscale2(w2, h2, cov2)
	rows = append(rows, cmpRow{name: "SS2x-LIGHT", label: "4 2x supersample (LIGHT)", w: dw, h: dh, rgba: cmpCompositeWhite(dw, dh, covD, cmpInk)})

	// 5：LCD 子像素。正确枚举（4）下为真子像素分解：横向硬跳变归零，
	// 但三通道独立，换非白背景合成会显彩边（条纹守卫把守灰化风险）。
	ttfSetFontHinting(font, uintptr(hintLightSubpixel))
	lw, lh, lrgba := cmpRenderLCD(t, font, cmpProbeText)
	cmpCheckLCDSubpixelFringe(t, lw, lh, lrgba)
	rows = append(rows, cmpRow{name: "LCD-subpixel", label: "5 LCD subpixel", w: lw, h: lh, rgba: lrgba})

	// 画布：左侧 12px arial 标签 + 右侧文本行，白底。
	lblFont := openTTF(t, DefaultFontPath(), cmpLabelSize)
	defer ttfCloseFont(lblFont)
	const margin, rowGap, colGap = 12, 14, 10
	labelW, labelH := 0, 0
	textW, textH := 0, 0
	for _, r := range rows {
		lw2, lh2, _ := cmpBlendedCoverage(t, lblFont, r.label)
		if lw2 > labelW {
			labelW = lw2
		}
		if lh2 > labelH {
			labelH = lh2
		}
		if r.w > textW {
			textW = r.w
		}
		if r.h > textH {
			textH = r.h
		}
		// 探针允许标签重复渲染一次（此处量尺寸、拼装时再画），不另存中间图
	}
	cw, ch := margin*2+labelW+colGap+textW, margin*2+len(rows)*textH+(len(rows)-1)*rowGap
	canvas := image.NewNRGBA(image.Rect(0, 0, cw, ch))
	for i := range canvas.Pix {
		canvas.Pix[i] = 0xFF
	}
	y := margin
	for _, r := range rows {
		lw2, lh2, lcov := cmpBlendedCoverage(t, lblFont, r.label)
		cmpBlit(canvas, margin, y+textH-lh2, lw2, lh2, cmpCompositeWhite(lw2, lh2, lcov, cmpLabelInk))
		cmpBlit(canvas, margin+labelW+colGap, y+textH-r.h, r.w, r.h, r.rgba)
		cmpAnalyze(t, r.name, r.w, r.h, r.rgba)
		y += textH + rowGap
	}

	outDir := filepath.Join("..", "..", ".temp")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}
	cmpSavePNG(t, filepath.Join(outDir, "fontcmp_native.png"), canvas)
	big := cmpUpscaleNearest(canvas, 4)
	cmpSavePNG(t, filepath.Join(outDir, "fontcmp.png"), big)
	if abs, err := filepath.Abs(filepath.Join(outDir, "fontcmp.png")); err == nil {
		t.Logf("[cmp] 对比条已输出: %s (native: fontcmp_native.png)", abs)
	}
}

// cmpCheckLCDSubpixelFringe 防止"LCD 被静默灰化"：真子像素渲染的墨边
// 像素三通道覆盖度不同（R≥G≥B 渐变）；若整图 R≈G≈B 则 DLL 根本没做子像素
// 分解，对比条里的"LCD 行"失去意义，直接 fail 提醒重扫。
func cmpCheckLCDSubpixelFringe(t *testing.T, w, h int, rgba []byte) {
	t.Helper()
	fringe := 0
	for y := 2; y < h-2; y++ {
		for x := 2; x < w-2; x++ {
			o := (y*w + x) * 4
			mx, mn := int(rgba[o]), int(rgba[o])
			for i := 1; i < 3; i++ {
				if v := int(rgba[o+i]); v > mx {
					mx = v
				} else if v < mn {
					mn = v
				}
			}
			if mx-mn >= 8 && mn < 240 {
				fringe++
			}
		}
	}
	if fringe < 8 {
		t.Fatalf("LCD 输出几乎无通道间差异（fringe=%d），子像素路径可能被 DLL 灰化，对比无效", fringe)
	}
	t.Logf("[cmp] LCD 子像素条纹像素=%d", fringe)
}

// ---------------------------------------------------------------------------
// 渲染原语
// ---------------------------------------------------------------------------

func cmpPackColor(c [4]uint8) uintptr {
	return uintptr(c[0]) | uintptr(c[1])<<8 | uintptr(c[2])<<16 | uintptr(c[3])<<24
}

// cmpSurfaceWH 取 surface 宽高（x64: w@16 h@20 pitch@24 pixels@32）。
func cmpSurfaceWH(s uintptr) (int, int) {
	return int(*(*int32)(ptrAt(s + 16))), int(*(*int32)(ptrAt(s + 20)))
}

// cmpBlendedCoverage 渲染覆盖 surface 并取紧致 w*h 覆盖字节。
// 实测 blended 为 32bpp ARGB8888 surface（白色也不例外）。每像素字节数直接
// 读 SDL_PixelFormat.BytesPerPixel——BitsPerPixel/BytesPerPixel 是相邻两个
// uint8（x64: @16/@17，实测把 @16 按 int32 读得 1056=32+4*256 即证），并与
// BitsPerPixel 互验。不可凭记忆硬编字节序，也不可拿 pitch/w 反推：窄 surface
// 行末填充会让商超出真实值（"登录" w=52 而 pitch=272=4×68，272/52=5）。
// 覆盖通道用跨度判：醒目色 (1,2,253) 渲染后，alpha=覆盖连续取 0..255 跨度
// 最大，RGB 至多 3 个离散值；跨度不足 32 则 fail 而非错位解码。
func cmpBlendedCoverage(t *testing.T, font uintptr, text string) (int, int, []byte) {
	t.Helper()
	b, p := cBytes(text)
	s := ttfRenderUTF8Blended(font, p, cmpPackColor(cmpProbeColor))
	runtime.KeepAlive(b)
	if s == 0 {
		t.Fatalf("RenderUTF8_Blended(%q): %s", text, lastError())
	}
	defer sdlFreeSurface(s)
	w, h := cmpSurfaceWH(s)
	pitch := int(*(*int32)(ptrAt(s + 24)))
	base := *(*uintptr)(ptrAt(s + 32))
	formatPtr := *(*uintptr)(ptrAt(s + 8))
	bits := int(*(*uint8)(ptrAt(formatPtr + 16)))
	bpp := int(*(*uint8)(ptrAt(formatPtr + 17)))
	if w <= 0 || h <= 0 || bpp < 1 || bpp > 4 || bpp*w > pitch || (bits%8 == 0 && bits/8 != bpp) {
		t.Fatalf("blended surface 布局假设被打破: w=%d h=%d pitch=%d bits=%d bytes=%d (%s)",
			w, h, pitch, bits, bpp, cmpFormatName(t, s))
	}
	rowAt := func(y int) []byte {
		return unsafe.Slice((*byte)(ptrAt(base+uintptr(y)*uintptr(pitch))), pitch)
	}
	// 通道跨度判定覆盖位置
	var mn [4]uint8 = [4]uint8{255, 255, 255, 255}
	var mx [4]uint8
	for y := 0; y < h; y++ {
		row := rowAt(y)
		for x := 0; x < w; x++ {
			p := row[x*bpp:][:bpp]
			for i := 0; i < bpp; i++ {
				if p[i] < mn[i] {
					mn[i] = p[i]
				}
				if p[i] > mx[i] {
					mx[i] = p[i]
				}
			}
		}
	}
	covPos, covSpan := 0, int(mx[0])-int(mn[0])
	if bpp == 1 {
		covPos, covSpan = 0, 256 // 8bpp 灰度：唯一通道即覆盖
	} else {
		for i := 1; i < bpp; i++ {
			if v := int(mx[i]) - int(mn[i]); v > covSpan {
				covSpan, covPos = v, i
			}
		}
	}
	if covSpan < 32 {
		t.Fatalf("未发现连续覆盖通道（跨度=%d，位置=%d，%s w=%d pitch=%d），surface 布局假设被打破",
			covSpan, covPos, cmpFormatName(t, s), w, pitch)
	}
	out := make([]byte, w*h)
	for y := 0; y < h; y++ {
		row := rowAt(y)
		for x := 0; x < w; x++ {
			out[y*w+x] = row[x*bpp+covPos]
		}
	}
	return w, h, out
}

// cmpCompositeWhite 把覆盖 alpha 合成到白底：out = 255 + (ink-255)*c/255。
func cmpCompositeWhite(w, h int, cov []byte, ink [4]uint8) []byte {
	out := make([]byte, w*h*4)
	for i, c := range cov {
		o := i * 4
		out[o] = 255 - uint8((255-int(ink[0]))*int(c)/255)
		out[o+1] = 255 - uint8((255-int(ink[1]))*int(c)/255)
		out[o+2] = 255 - uint8((255-int(ink[2]))*int(c)/255)
		out[o+3] = 255
	}
	return out
}

// cmpDownscale2 对覆盖图 box 2x 降采样（alpha 线性域平均 = 超采样本质）。
func cmpDownscale2(w, h int, cov []byte) (int, int, []byte) {
	dw, dh := w/2, h/2
	out := make([]byte, dw*dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			s := int(cov[(2*y)*w+2*x]) + int(cov[(2*y)*w+2*x+1]) +
				int(cov[(2*y+1)*w+2*x]) + int(cov[(2*y+1)*w+2*x+1])
			out[y*dw+x] = uint8(s / 4)
		}
	}
	return dw, dh, out
}

// cmpFormatName 经 SDL_GetPixelFormatName 读 surface 的格式名（仅记录；
// 解码不依赖它——SDL_PixelFormat.format 是首成员，偏移 0 无疑问）。
func cmpFormatName(t *testing.T, s uintptr) string {
	t.Helper()
	proc := dllSDL.NewProc("SDL_GetPixelFormatName")
	if err := proc.Find(); err != nil {
		return "?"
	}
	formatPtr := *(*uintptr)(ptrAt(s + 8))
	enum := *(*uint32)(ptrAt(formatPtr))
	r, _, _ := proc.Call(uintptr(enum))
	return goString(r)
}

// cmpRenderLCD 渲染 LCD surface（DLL 已按 bg=白合成）并解码为紧致 RGBA。
func cmpRenderLCD(t *testing.T, font uintptr, text string) (int, int, []byte) {
	t.Helper()
	b, p := cBytes(text)
	s := ttfRenderUTF8LCD(font, p, cmpPackColor(cmpInk), cmpPackColor(cmpWhite))
	runtime.KeepAlive(b)
	if s == 0 {
		t.Fatalf("RenderUTF8_LCD(%q): %s", text, lastError())
	}
	defer sdlFreeSurface(s)
	w, h, rgba, order := cmpDecodeSurfaceRGBA(t, s)
	t.Logf("[cmp] LCD surface format=%s %dx%d 通道字节序=%s", cmpFormatName(t, s), w, h, order)
	return w, h, rgba
}

// cmpDecodeSurfaceRGBA 不假设 SDL_PixelFormat 布局的 surface 解码：
//  1. bpp 与位深读 SDL_PixelFormat 的两个 uint8（Bits@16 Bytes@17），
//     再以 bpp*w<=pitch 守卫——与 cmpBlendedCoverage 同源，禁止 pitch/w 反推；
//  2. 墨体像素（≥3 字节 ≤60，对应 #0f172a=15,23,42）各字节位置取均值，
//     与三指纹贪心指派 → R/G/B 字节位置；4bpp 余下位置是 alpha（均值高=255
//     有效，低=X 字节未定义）；
//  3. 按判出的通道序统一输出紧致 RGBA（alpha 置 255）。
func cmpDecodeSurfaceRGBA(t *testing.T, s uintptr) (int, int, []byte, string) {
	t.Helper()
	w, h := cmpSurfaceWH(s)
	pitch := int(*(*int32)(ptrAt(s + 24)))
	base := *(*uintptr)(ptrAt(s + 32))
	formatPtr := *(*uintptr)(ptrAt(s + 8))
	bits := int(*(*uint8)(ptrAt(formatPtr + 16)))
	bpp := int(*(*uint8)(ptrAt(formatPtr + 17)))
	if w <= 0 || h <= 0 || pitch <= 0 || bpp < 3 || bpp > 4 || bpp*w > pitch || (bits%8 == 0 && bits/8 != bpp) {
		t.Fatalf("LCD surface 布局假设被打破: w=%d h=%d pitch=%d bits=%d bytes=%d", w, h, pitch, bits, bpp)
	}
	at := func(y, x int) []byte {
		return unsafe.Slice((*byte)(ptrAt(base+uintptr(y)*uintptr(pitch))), pitch)[x*bpp:][:bpp]
	}
	var sum, cnt [4]int
	samples := 0
	// 样本只取"满覆盖墨体"：前 3 字节（跳过 alpha/X）都 ≤45 且极差 ≥15。
	// 实测发现欠覆盖子像素像素（如 [52,44,48]）三通道互向均值收敛、极差小，
	// 混入会把指纹均值带偏；纯墨体（[42,23,15]=#0f172a 的 BGR 序）极差=27。
ingk:
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := at(y, x)
			mx, mn := 0, 255
			for i := 0; i < 3; i++ {
				if int(p[i]) > mx {
					mx = int(p[i])
				}
				if int(p[i]) < mn {
					mn = int(p[i])
				}
			}
			if mx > 45 || mx-mn < 15 {
				continue
			}
			for i := 0; i < bpp; i++ {
				sum[i] += int(p[i])
				cnt[i]++
			}
			if samples++; samples >= 256 {
				break ingk
			}
		}
	}
	if cnt[0] < 8 {
		t.Fatalf("满覆盖墨体样本不足(%d)，无法判定通道序", samples)
	}
	var mean [4]float64
	for i := 0; i < bpp; i++ {
		mean[i] = float64(sum[i]) / float64(cnt[i])
	}
	chName := [4]byte{'x', 'x', 'x', 'x'}
	used := [4]bool{}
	for _, f := range []struct {
		ch byte
		v  float64
	}{{'R', 15}, {'G', 23}, {'B', 42}} {
		best, bestErr := -1, 1e9
		for p := 0; p < bpp; p++ {
			if used[p] {
				continue
			}
			e := mean[p] - f.v
			if e < 0 {
				e = -e
			}
			if e < bestErr {
				bestErr, best = e, p
			}
		}
		if best < 0 || bestErr > 8 {
			t.Fatalf("指纹 %c=%v 无对应字节位置（err=%.1f），通道序判定失败", f.ch, f.v, bestErr)
		}
		chName[best] = f.ch
		used[best] = true
	}
	if bpp == 4 {
		for p := 0; p < 4; p++ {
			if !used[p] {
				if mean[p] > 127 {
					chName[p] = 'A'
				} else {
					chName[p] = 'X' // XRGB/XBGR 的未定义字节
				}
			}
		}
	}
	var rP, gP, bP int
	for p := 0; p < bpp; p++ {
		switch chName[p] {
		case 'R':
			rP = p
		case 'G':
			gP = p
		case 'B':
			bP = p
		}
	}
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := at(y, x)
			o := (y*w + x) * 4
			out[o], out[o+1], out[o+2] = p[rP], p[gP], p[bP]
			out[o+3] = 255
		}
	}
	return w, h, out, string(chName[:bpp])
}

// ---------------------------------------------------------------------------
// 画布与输出
// ---------------------------------------------------------------------------

// cmpBlit 把紧致 RGBA 贴到画布 (x,y)。
func cmpBlit(dst *image.NRGBA, x, y, w, h int, rgba []byte) {
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			o := (yy*w + xx) * 4
			dst.SetNRGBA(x+xx, y+yy, color.NRGBA{rgba[o], rgba[o+1], rgba[o+2], 255})
		}
	}
}

// cmpUpscaleNearest 最近邻放大 k 倍（保持原始像素结构，防查看器插值误判）。
func cmpUpscaleNearest(src *image.NRGBA, k int) *image.NRGBA {
	sb := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, sb.Dx()*k, sb.Dy()*k))
	for y := 0; y < sb.Dy(); y++ {
		for x := 0; x < sb.Dx(); x++ {
			c := src.NRGBAAt(x, y)
			for dy := 0; dy < k; dy++ {
				for dx := 0; dx < k; dx++ {
					dst.SetNRGBA(x*k+dx, y*k+dy, c)
				}
			}
		}
	}
	return dst
}

func cmpSavePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("png encode %s: %v", path, err)
	}
}

// ---------------------------------------------------------------------------
// 边缘量化（口径与 .temp/textaa 完全一致，数据可直接对照）
// ---------------------------------------------------------------------------

// cmpAnalyze 按"墨≤60 / 底≥250 / 其余=过渡像素"分类，统计硬跳变 vs AA 边界。
func cmpAnalyze(t *testing.T, name string, w, h int, rgba []byte) {
	t.Helper()
	line := fmt.Sprintf("[cmp] %-16s %s", name, cmpMetricsLine(w, h, rgba))
	t.Log(line)
	fmt.Println(line)
}
