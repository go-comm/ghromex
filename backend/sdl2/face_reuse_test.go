//go:build windows

package sdl2

import (
	"os"
	"runtime"
	"testing"
	"unsafe"
)

// ---------------------------------------------------------------------------
// face 复用回归锁
//
// TTF_SetFontSize 切换后，度量/像素必须与"直接按目标字号打开"完全一致。
// 注意：C 原型中 ptsize 是 float（x64 ABI 浮点应走 XMM 寄存器），而 Go 的
// syscall trampoline 只写 RCX/RDX/R8/R9 整数寄存器。若实现真的读浮点寄存器，
// 每次切换拿到的"值"就是此前 C 调用的残值——生产多字号交错时形同字号轮盘赌。
// 污染探针把参数通路钉死为确定性整数传递；任何失败都意味着复用 face 不可信，
// 必须回退按尺寸建 face 的路径。
// ---------------------------------------------------------------------------

const reuseProbeText = "登录"

// contamA/B 由 pid 派生，取值域避开测试字号（13~26）。
var (
	contamA = float32(os.Getpid()%97+3)*1.75 + 40.5
	contamB = float32(os.Getpid()%71+40)*2.25 + 41.25
)

//go:noinline
func contaminateXMM() float32 {
	// 两条独立浮点计算：返回时在 X0（及若干 XMM 中间值）留下与字号无关的残值
	x := contamA
	y := contamB
	return x*y/100 + (x - y)
}

func initSDLTTF(t *testing.T) {
	t.Helper()
	if err := Load(); err != nil {
		t.Skipf("SDL DLL 不可用: %v", err)
	}
	if ttfInit() != 0 {
		t.Skip("TTF_Init 失败")
	}
	if !hasTTFSetFontSize {
		t.Skip("该 DLL 无 TTF_SetFontSize，走多 face 回退路径")
	}
}

func openTTF(t *testing.T, path string, size int) uintptr {
	t.Helper()
	b, p := cBytes(path)
	h := ttfOpenFont(p, uintptr(size))
	runtime.KeepAlive(b)
	if h == 0 {
		t.Fatalf("TTF_OpenFont(%s,%d): %s", path, size, lastError())
	}
	return h
}

func sizeTTF(t *testing.T, font uintptr, text string) (int, int) {
	t.Helper()
	var w, h int32
	b, p := cBytes(text)
	r := ttfSizeUTF8(font, p, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)))
	runtime.KeepAlive(b)
	if r != 0 {
		t.Fatalf("TTF_SizeUTF8: %s", lastError())
	}
	return int(w), int(h)
}

// TestFaceReuseSwitchSize 验证：open(13)→SetFontSize(26) 与直接 open(26)
// 的度量、覆盖像素完全一致；bold 切换同理。
func TestFaceReuseSwitchSize(t *testing.T) {
	initSDLTTF(t)
	defer ttfQuit()

	path := CJKFontPath()
	ref := openTTF(t, path, 26)
	defer ttfCloseFont(ref)
	wRef, hRef := sizeTTF(t, ref, reuseProbeText)

	dyn := openTTF(t, path, 13)
	defer ttfCloseFont(dyn)
	if ret := ttfSetFontSize(dyn, 26); ret != 0 {
		t.Fatalf("TTF_SetFontSize(26) 失败 ret=%d err=%s（face 复用会渲染错字号）", ret, lastError())
	}
	if w, h := sizeTTF(t, dyn, reuseProbeText); w != wRef || h != hRef {
		t.Fatalf("动态切字号度量不一致: ref=(%d,%d) dyn=(%d,%d)", wRef, hRef, w, h)
	}

	// 用 cmpBlendedCoverage（fontcmp_test.go，32bpp surface 取真覆盖通道）。
	// 旧 readSurfacePixels 按"blended 是 8bpp"的假设每行只读前 w 字节，
	// 实测 blended 是 pitch≈4w 的 32bpp surface，旧读法实际只覆盖每行前 1/4
	// 像素——两侧一致所以对比仍成立，但已丢失后半行信息，不再保留。
	wr, hr, pRef := cmpBlendedCoverage(t, ref, reuseProbeText)
	wd, hd, pDyn := cmpBlendedCoverage(t, dyn, reuseProbeText)
	if wr != wd || hr != hd || len(pRef) != len(pDyn) {
		t.Fatalf("surface 尺寸不同 (%d,%d) vs (%d,%d)", wr, hr, wd, hd)
	}
	diff := 0
	for i := range pRef {
		if pRef[i] != pDyn[i] {
			diff++
		}
	}
	if diff > len(pRef)/50 { // 允许极少量抖动像素（缓存路径差异）
		t.Fatalf("像素差异过大: %d/%d", diff, len(pRef))
	}

	// bold 切换同理
	refB := openTTF(t, path, 13)
	defer ttfCloseFont(refB)
	ttfSetFontStyle(refB, ttfStyleBold)
	wB1, hB1 := sizeTTF(t, refB, reuseProbeText)
	if ret := ttfSetFontSize(dyn, 13); ret != 0 {
		t.Fatalf("TTF_SetFontSize(13) 失败: %s", lastError())
	}
	ttfSetFontStyle(dyn, ttfStyleBold)
	if w, h := sizeTTF(t, dyn, reuseProbeText); w != wB1 || h != hB1 {
		t.Fatalf("bold 切换度量不一致: ref=(%d,%d) dyn=(%d,%d)", wB1, hB1, w, h)
	}
}

// TestSetFontSizeUsesIntegerRegister 故意污染浮点寄存器后切换字号，
// 结果仍必须与整数参数一致。失败 = 该 DLL 的 ptsize 实际取自 XMM 残值
// （Go trampoline 无法正确传递），face 复用路径必须停用。
func TestSetFontSizeUsesIntegerRegister(t *testing.T) {
	initSDLTTF(t)
	defer ttfQuit()

	path := CJKFontPath()

	ref := openTTF(t, path, 26)
	defer ttfCloseFont(ref)
	wRef, hRef := sizeTTF(t, ref, reuseProbeText)

	dyn := openTTF(t, path, 13)
	defer ttfCloseFont(dyn)
	_ = contaminateXMM()
	if ret := ttfSetFontSize(dyn, 26); ret != 0 {
		t.Fatalf("污染寄存器后切 26 失败: %s", lastError())
	}
	if w, h := sizeTTF(t, dyn, reuseProbeText); w != wRef || h != hRef {
		t.Fatalf("浮点寄存器污染改变了切换结果 dyn=(%d,%d) ref=(%d,%d)："+
			"ptsize 实际取自 XMM 残值，face 复用不可信，必须按尺寸建 face", w, h, wRef, hRef)
	}

	// 生产式交错压测：每轮先污染，再经 SizeUTF8 等真实 C 调用换残值，
	// 多个字号来回切都要与直接打开一致
	for _, sz := range []int{14, 18, 26, 24, 16, 13, 26} {
		_ = contaminateXMM()
		if ret := ttfSetFontSize(dyn, uintptr(sz)); ret != 0 {
			t.Fatalf("TTF_SetFontSize(%d) 失败: %s", sz, lastError())
		}
		f := openTTF(t, path, sz)
		wWant, hWant := sizeTTF(t, f, reuseProbeText)
		ttfCloseFont(f)
		sizeTTF(t, dyn, reuseProbeText) // 模拟中间 C 调用刷新寄存器残值
		if w, h := sizeTTF(t, dyn, reuseProbeText); w != wWant || h != hWant {
			t.Fatalf("多字号压测在 %d 失败: dyn=(%d,%d) want=(%d,%d)", sz, w, h, wWant, hWant)
		}
	}
}

// TestSetFontSizePreservesHinting 验证生产路径的前提：建 face 时设一次
// hinting 后，TTF_SetFontSize / TTF_SetFontStyle 不会把它重置回默认 NORMAL。
// 若该前提被 DLL 升级打破，face 复用缓存会让大部分字号静默退回 NORMAL 渲染，
// LIGHT 选型失效——此测试把住这条缝。
func TestSetFontSizePreservesHinting(t *testing.T) {
	initSDLTTF(t)
	defer ttfQuit()
	if !hasTTFHinting {
		t.Skip("该 DLL 无 TTF_SetFontHinting")
	}
	path := CJKFontPath()

	// 基准：直接按 26 建 face 并设 LIGHT
	want := openTTF(t, path, 26)
	defer ttfCloseFont(want)
	ttfSetFontHinting(want, uintptr(hintLight))
	ww, wh, covWant := cmpBlendedCoverage(t, want, cmpProbeText)

	// 复用：13 建 face→设 LIGHT→切 26→切 bold 再切回，覆盖像素应与基准一致。
	// LIGHT/NORMAL 连文本宽都不同（163 vs 165），若切字号重置了 hinting，
	// 尺寸校验第一时间就能抓住。
	dyn := openTTF(t, path, 13)
	defer ttfCloseFont(dyn)
	ttfSetFontHinting(dyn, uintptr(hintLight))
	if ret := ttfSetFontSize(dyn, 26); ret != 0 {
		t.Fatalf("TTF_SetFontSize(26): %s", lastError())
	}
	ttfSetFontStyle(dyn, ttfStyleBold) // 走一次 style 切换，SDL_ttf 可能重建内部 face
	ttfSetFontStyle(dyn, 0)
	dw, dh, covDyn := cmpBlendedCoverage(t, dyn, cmpProbeText)
	if dw != ww || dh != wh || len(covDyn) != len(covWant) {
		t.Fatalf("切字号后 hinting 未保持：(%d,%d) vs 基准 (%d,%d)（生产会静默退回 NORMAL）",
			dw, dh, ww, wh)
	}
	diff := 0
	for i := range covWant {
		if covWant[i] != covDyn[i] {
			diff++
		}
	}
	if diff > len(covWant)/50 { // 与 TestFaceReuseSwitchSize 同容忍度（缓存路径微抖）
		t.Fatalf("切字号/style 后 hinting 像素偏差过大: %d/%d", diff, len(covWant))
	}
}

// readSurfacePixels 已删除：blended 实测是 32bpp（pitch≈4w），旧 8bpp 读法
// 只读到每行前 1/4 像素；统一用 fontcmp_test.go 的 cmpBlendedCoverage 取真
// 覆盖通道（含跨度自校准与 stride 行读取）。布局读取手法见 ptrAt。

// ptrAt 把局部 uintptr 变量的位模式按指针值读出（非 uintptr 运算直转，vet 友好）。
func ptrAt(u uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&u))
}
