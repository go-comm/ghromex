package sdl2

import (
	"strconv"
	"unsafe"
)

// ---------------------------------------------------------------------------
// 灰度文本 tone 曲线（GHROMEX_CONTRAST，方案 A）
//
// 背景（README「渲染段」的矩阵实测）：SDL_ttf 的 blended 输出是
// FreeType 覆盖度**线性直出**，没有任何 gamma/对比度后处理；Chrome（Skia/
// DirectWrite）在覆盖度之后还有一道 tone 调整（Skia SkMaskGamma 同类），把
// 过渡像素推向两端——同样覆盖率观感更"利"。本文件在字形 surface 上做该
// 调整的线性近似：
//
//	a' = clamp((a - 127.5) * k + 127.5)   （k = GHROMEX_CONTRAST，>1 生效）
//
// 以 0.5 为枢轴拉开两端：高覆盖更实（笔画边缘更黑）、低覆盖更净（毛边消失），
// 过渡区变窄。k≤1 为 no-op，k 上限 3（textContrast 解析）。
//
// 作用对象：仅 blended（灰度）路径的覆盖通道。SDL_ttf Blended 实测为
// "RGB = 前景色恒定 + alpha = 覆盖"的直通 alpha（fontcmp 覆盖通道跨度判定
// 同源），所以只需改 alpha 字节；通道位置按跨度判定、不硬编字节序。LCD
// 路径的纹理在 DLL 内已按前景×背景合成完，不适用（也不需要）该曲线。
// ---------------------------------------------------------------------------

// contrastAlpha 对单个覆盖值做以 0.5 为枢轴的线性对比度映射；k≤1 恒等。
func contrastAlpha(a byte, k float64) byte {
	if k <= 1 {
		return a
	}
	v := (float64(a)-127.5)*k + 127.5
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return byte(v + 0.5)
}

// surfPtr 把局部 uintptr 变量的位模式按指针值读出（非 uintptr 运算直转，
// vet 友好——与测试侧 ptrAt 同一手法）。
func surfPtr(u uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&u))
}

// applyTextContrast 对 blended 字形 surface 施加 tone 曲线（原地修改）。
// s=0、k≤1 或布局异常时为 no-op。
//
// 布局读取按 **64 位** SDL_Surface 写死（w@16 h@20 pitch@24 pixels@32，
// format@8 → BytesPerPixel@17）：32 位下这套偏移读出的是错位垃圾值，
// 现有校验未必拦得住——而校验一旦漏过，后续写像素就会破坏内存。故 32 位
// 直接不做曲线（安全降级），64 位再叠加尺寸/步幅合理性校验兜底。
func applyTextContrast(s uintptr, k float64) {
	if strconv.IntSize != 64 {
		return
	}
	if s == 0 || k <= 1 {
		return
	}
	w := int(*(*int32)(surfPtr(s + 16)))
	h := int(*(*int32)(surfPtr(s + 20)))
	pitch := int(*(*int32)(surfPtr(s + 24)))
	base := *(*uintptr)(surfPtr(s + 32))
	format := *(*uintptr)(surfPtr(s + 8))
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 || base == 0 || format == 0 {
		return
	}
	bpp := int(*(*uint8)(surfPtr(format + 17)))
	// 只认 32bpp：blended 实测恒为 32bpp 直通 alpha；8bpp 调色板 surface 的
	// "覆盖通道"实为调色板索引，改写会毁掉字形。
	if bpp != 4 {
		return
	}
	// 步幅合理性：行末填充通常 ≤ 数十字节；过大即错位垃圾值。
	if pitch < w*bpp || pitch > w*bpp+1024 {
		return
	}
	rowAt := func(y int) []byte {
		return unsafe.Slice((*byte)(surfPtr(base+uintptr(y)*uintptr(pitch))), pitch)
	}
	// 通道跨度判定覆盖位置：RGB 在整幅内恒定（前景色），alpha 承载 0..255
	// 覆盖渐变——跨度最大的即覆盖通道；跨度不足（无渐变可调）则原样返回。
	var mn [4]uint8 = [4]uint8{255, 255, 255, 255}
	var mx [4]uint8
	for y := 0; y < h; y++ {
		row := rowAt(y)
		for x := 0; x < w; x++ {
			p := row[x*bpp : x*bpp+bpp]
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
	for i := 1; i < bpp; i++ {
		if v := int(mx[i]) - int(mn[i]); v > covSpan {
			covSpan, covPos = v, i
		}
	}
	if covSpan < 32 {
		return
	}
	for y := 0; y < h; y++ {
		row := rowAt(y)
		for x := 0; x < w; x++ {
			o := x*bpp + covPos
			row[o] = contrastAlpha(row[o], k)
		}
	}
}
