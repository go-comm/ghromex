//go:build windows

package sdl2

import (
	"os"
	"runtime"
	"testing"
	"unsafe"
)

// ---------------------------------------------------------------------------
// A+B 文本渲染改进的环境变量控制（GHROMEX_HINT / GHROMEX_CONTRAST）
//
//  A. tone 曲线：blended 覆盖通道的对比度映射（text_contrast.go）；
//  B. hinting 档位：face 建立时的 TTF_SetFontHinting 取值（graphics.go）。
// ---------------------------------------------------------------------------

// TestContrastAlphaTable tone 曲线数学表：枢轴 0.5、k≤1 恒等、两端钳位、单调。
func TestContrastAlphaTable(t *testing.T) {
	// k≤1 恒等
	for _, k := range []float64{0, 0.5, 1} {
		for a := 0; a <= 255; a++ {
			if got := contrastAlpha(byte(a), k); got != byte(a) {
				t.Fatalf("k=%v 时 contrastAlpha(%d)=%d，应为恒等", k, a, got)
			}
		}
	}
	for _, k := range []float64{1.2, 1.6, 3} {
		// 极端值钳位
		if contrastAlpha(0, k) != 0 || contrastAlpha(255, k) != 255 {
			t.Fatalf("k=%v 端点未钳位: 0→%d 255→%d", k, contrastAlpha(0, k), contrastAlpha(255, k))
		}
		prev := contrastAlpha(0, k)
		for a := 1; a <= 255; a++ {
			v := contrastAlpha(byte(a), k)
			if v < prev {
				t.Fatalf("k=%v 非单调: a=%d→%d 但 a-1→%d", k, a, v, prev)
			}
			prev = v
			// 枢轴两侧：高覆盖更实（不降）、低覆盖更净（不升）
			if a > 127 && v < byte(a) {
				t.Fatalf("k=%v a=%d: %d 应 ≥ a（高覆盖被推实）", k, a, v)
			}
			if a < 127 && v > byte(a) {
				t.Fatalf("k=%v a=%d: %d 应 ≤ a（低覆盖被推净）", k, a, v)
			}
		}
	}
}

// TestTextContrastEnv GHROMEX_CONTRAST 解析：off/空/非法/≤1 关闭，上限 3。
func TestTextContrastEnv(t *testing.T) {
	orig := os.Getenv("GHROMEX_CONTRAST")
	defer os.Setenv("GHROMEX_CONTRAST", orig)
	cases := map[string]float64{
		"":       0,
		"off":    0,
		"OFF":    0,
		"0":      0,
		"1":      0,
		"0.9":    0,
		"abc":    0,
		"1.5":    1.5,
		" 1.35 ": 1.35,
		"3":      3,
		"9":      3, // 钳位
	}
	for in, want := range cases {
		os.Setenv("GHROMEX_CONTRAST", in)
		if got := textContrast(); got != want {
			t.Errorf("GHROMEX_CONTRAST=%q: textContrast()=%v, want %v", in, got, want)
		}
	}
}

// TestFontHintingEnv GHROMEX_HINT 档位映射（灰度档）+ LCD 强制 subpixel 覆盖。
func TestFontHintingEnv(t *testing.T) {
	origH, origL := os.Getenv("GHROMEX_HINT"), os.Getenv("GHROMEX_LCD")
	defer func() {
		os.Setenv("GHROMEX_HINT", origH)
		os.Setenv("GHROMEX_LCD", origL)
	}()
	os.Setenv("GHROMEX_LCD", "0")
	cases := map[string]uintptr{
		"":       hintNormal, // 默认 normal（对齐 Chrome 灰度轮廓）
		"normal": hintNormal,
		"NORMAL": hintNormal,
		"light":  hintLight,
		"LIGHT":  hintLight,
		"none":   hintNone,
		"NONE":   hintNone,
		"mono":   hintMono,
		"weird":  hintNormal, // 未知值回退默认
	}
	for in, want := range cases {
		os.Setenv("GHROMEX_HINT", in)
		if got := fontHinting(); got != want {
			t.Errorf("GHROMEX_HINT=%q: fontHinting()=0x%x, want 0x%x", in, got, want)
		}
	}
	// LCD 打开时强制 LIGHT_SUBPIXEL（子像素分解依赖），GHROMEX_HINT 不生效。
	// hasTTFLCD 在 Load 之后才有值，此处与其余测试同进程共享已加载的 DLL。
	if hasTTFLCD {
		os.Setenv("GHROMEX_LCD", "1")
		os.Setenv("GHROMEX_HINT", "none")
		if got := fontHinting(); got != hintLightSubpixel {
			t.Errorf("LCD=1 时 fontHinting()=0x%x, want LIGHT_SUBPIXEL(0x04)", got)
		}
	}
}

// TestApplyTextContrastSurface 真 surface 原地变换：RGB 恒定通道不动、覆盖
// 通道逐字节等于 contrastAlpha；k≤1 为严格 no-op。
func TestApplyTextContrastSurface(t *testing.T) {
	initSDLTTF(t)
	defer ttfQuit()

	const k = 1.6
	f := openTTF(t, CJKFontPath(), 16)
	defer ttfCloseFont(f)

	render := func() uintptr {
		b, p := cBytes("永 contrast")
		s := ttfRenderUTF8Blended(f, p, cmpPackColor(cmpProbeColor))
		runtime.KeepAlive(b)
		if s == 0 {
			t.Fatalf("RenderUTF8_Blended: %s", lastError())
		}
		return s
	}
	// 布局读取（与 fontcmp 同口径：x64 SDL_Surface w@16 h@20 pitch@24 pixels@32，
	// format@8 → BytesPerPixel@17）
	snap := func(s uintptr) (w, h, pitch, bpp, covPos int, rows [][]byte) {
		w = int(*(*int32)(ptrAt(s + 16)))
		h = int(*(*int32)(ptrAt(s + 20)))
		pitch = int(*(*int32)(ptrAt(s + 24)))
		format := *(*uintptr)(ptrAt(s + 8))
		bpp = int(*(*uint8)(ptrAt(format + 17)))
		if w <= 0 || h <= 0 || pitch < w*bpp || bpp < 1 || bpp > 4 {
			t.Fatalf("surface 布局异常: w=%d h=%d pitch=%d bpp=%d", w, h, pitch, bpp)
		}
		base := *(*uintptr)(ptrAt(s + 32))
		var mn [4]uint8 = [4]uint8{255, 255, 255, 255}
		var mx [4]uint8
		rows = make([][]byte, h)
		for y := 0; y < h; y++ {
			row := make([]byte, w*bpp)
			src := unsafe.Slice((*byte)(ptrAt(base+uintptr(y)*uintptr(pitch))), w*bpp)
			copy(row, src)
			rows[y] = row
			for x := 0; x < w; x++ {
				for i := 0; i < bpp; i++ {
					v := row[x*bpp+i]
					if v < mn[i] {
						mn[i] = v
					}
					if v > mx[i] {
						mx[i] = v
					}
				}
			}
		}
		covPos, covSpan := 0, int(mx[0])-int(mn[0])
		for i := 1; i < bpp; i++ {
			if v := int(mx[i]) - int(mn[i]); v > covSpan {
				covSpan, covPos = v, i
			}
		}
		if covSpan < 32 {
			t.Fatalf("未发现覆盖通道（跨度=%d）", covSpan)
		}
		return
	}

	// --- k≤1 严格 no-op ---
	s1 := render()
	defer sdlFreeSurface(s1)
	_, h, _, _, _, before := snap(s1)
	applyTextContrast(s1, 1.0)
	_, _, _, _, _, after := snap(s1)
	for y := 0; y < h; y++ {
		for i, b := range before[y] {
			if after[y][i] != b {
				t.Fatalf("k=1 应为 no-op: y=%d byte[%d] %d→%d", y, i, b, after[y][i])
			}
		}
	}

	// --- k>1：非覆盖通道恒为前景色且不变；覆盖通道 = contrastAlpha(原值) ---
	s2 := render()
	defer sdlFreeSurface(s2)
	w2, h2, _, bpp2, cov2, before2 := snap(s2)
	applyTextContrast(s2, k)
	_, _, _, _, _, after2 := snap(s2)
	fg := cmpProbeColor
	changed := 0
	// 非覆盖通道的顺序是像素格式相关的（不可硬编字节序）：只断言"各通道恒定
	// + 三个常量恰是前景 RGB 的一个排列"，覆盖通道逐点等于 contrastAlpha。
	nonCov := make(map[int]uint8) // 通道位 → 首次见到的值
	for y := 0; y < h2; y++ {
		for x := 0; x < w2; x++ {
			for i := 0; i < bpp2; i++ {
				o := x*bpp2 + i
				got, want := after2[y][o], before2[y][o]
				if i == cov2 {
					if e := contrastAlpha(want, k); got != e {
						t.Fatalf("覆盖通道 y=%d x=%d: %d→%d, want %d", y, x, want, got, e)
					}
					if got != want {
						changed++
					}
					continue
				}
				if got != want {
					t.Fatalf("非覆盖通道 y=%d x=%d byte[%d] 被改动: %d→%d", y, x, i, want, got)
				}
				if first, ok := nonCov[i]; !ok {
					nonCov[i] = want
				} else if first != want {
					t.Fatalf("直通 alpha 结构被打破: byte[%d] 非恒定 %d vs %d", i, first, want)
				}
			}
		}
	}
	if len(nonCov) != 3 {
		t.Fatalf("非覆盖通道数=%d, want 3（bpp=%d cov=%d）", len(nonCov), bpp2, cov2)
	}
	var gotVals, wantVals []uint8
	for _, v := range nonCov {
		gotVals = append(gotVals, v)
	}
	wantVals = append(wantVals, fg[0], fg[1], fg[2])
	sortBytes(gotVals)
	sortBytes(wantVals)
	for i := range gotVals {
		if gotVals[i] != wantVals[i] {
			t.Fatalf("前景 RGB 值集不匹配: got %v, want %v", gotVals, wantVals)
		}
	}
	if changed == 0 {
		t.Error("k=1.6 下覆盖通道一个字节都没变：曲线没有落到 surface 上")
	}
}

// sortBytes 插入排序（3 个元素，免引入 sort 依赖）。
func sortBytes(a []uint8) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
