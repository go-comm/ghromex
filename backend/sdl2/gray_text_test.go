//go:build windows

package sdl2

import (
	"os"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// ---------------------------------------------------------------------------
// 灰度 AA 管线保真度探针
//
// 背景：用户反馈灰度路径（反馈时为 GHROMEX_LCD=0 回退，现为默认渲染方式）
// 下字体模糊。模糊有两个可能来源，必须先分离再谈优化：
//
//  1. 管线缺陷（可修）：纹理被缩放/坐标非整数/alpha 合成错误（如字形 surface
//     实为预乘 alpha 却按直通 alpha 贴图，边缘会系统性偏浅 = 观感发虚）；
//  2. 灰度 AA 固有：无横向子像素分解，竖笔边缘只能靠覆盖度过渡，
//     与 LCD/ClearType 的锐度差是物理性的（README 已注明）。
//
// 判据：把同一 face、同一 hinting 直出的 blended surface 合成到白底，与
// 经 Graphics.DrawText → SDL_RenderCopy → 回读得到的落屏像素逐个比对。
// 两者一致（±2 内）→ 管线忠实，发虚属灰度 AA 固有；显著偏大 → 找到了可修的
// 管线缺陷（此处会打印两边的边缘指标与偏差分布供定位）。
//
// 覆盖两条脚本路径：英文走 Arial（resolveFontPath），中文走 msyh 分段
// （splitScripts → segFontPath），界面实际显示以中文为主，缺一不可。
// ---------------------------------------------------------------------------

const (
	grayProbeTextEN = "Sharpness Aa12 Blurry"
	grayProbeTextZH = "灰度字形清晰度测试"
)

// TestGrayscalePipelineFidelity 见文件头注释。
func TestGrayscalePipelineFidelity(t *testing.T) {
	// 默认 dummy 驱动 = 软件渲染器（CI 稳定）；GHROMEX_REAL=1 时不接管驱动，
	// 用真机 GPU 渲染器复验——用户看到模糊的是 GPU 路径，软件路径忠实不代表
	// GPU 贴图（格式转换/过滤/混合）也忠实。
	if os.Getenv("GHROMEX_REAL") != "1" {
		os.Setenv("SDL_VIDEO_DRIVER", "dummy")
		defer os.Unsetenv("SDL_VIDEO_DRIVER")
	}
	// 显式钉住灰度路径：探针结论不随默认值/LCD 开关漂移。
	os.Setenv("GHROMEX_LCD", "0")
	defer os.Unsetenv("GHROMEX_LCD")

	cases := []struct {
		name string
		text string
		font string // "" = DefaultFontPath()
	}{
		{"en-arial", grayProbeTextEN, ""},
		{"zh-msyh", grayProbeTextZH, CJKFontPath()},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGrayFidelity(t, tc.text, tc.font)
		})
	}
}

// runGrayFidelity 单条文本的落屏比对，见文件头注释。
func runGrayFidelity(t *testing.T, text, fontPath string) {
	const (
		size    = 13
		originX = 8
		originY = 24
	)
	if fontPath == "" {
		fontPath = DefaultFontPath()
	}
	ink := [4]uint8{30, 30, 30, 255}

	win, err := NewWindow(300, 80, "gray-text")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()

	// --- 生产路径：Graphics.DrawText 落屏 ---
	g := win.g // 包内访问具体类型：Clear/Present 不在 engine.Graphics 接口上
	g.Clear(255, 255, 255, 255)
	p := engine.NewPaint()
	p.SetSize(engine.NewSize(engine.SIZE_PIXEL, size, 0))
	p.SetColor(engine.NewColor(ink[0], ink[1], ink[2], ink[3]))
	p.SetBackground(engine.NewColor(255, 255, 255, 255))
	g.DrawText(originX, originY, 280, 20, p, text)
	g.Present()

	buf, format, vw, vh, err := win.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	t.Logf("[gray] format=0x%08x %dx%d len=%d", format, vw, vh, len(buf))
	// 背板 alpha 通道未被写入（实测白底字节 [255 255 255 0]）：直接对 4 字节取
	// min 会把整屏判成墨迹。先在已知白底像素上定位 alpha 字节位置并排除之，
	// 剩下三字节为颜色（中性灰三通道同值，取 min 即灰度）。
	whiteIdx := 2 * 4 // 左上角 (2,2)：远离 originX/originY 处的文字
	alphaIdx := -1
	for i := 0; i < 4; i++ {
		if buf[whiteIdx+i] != 255 {
			alphaIdx = i
			break
		}
	}
	t.Logf("[gray] alpha 字节位=%d（-1 表示背板不透明）", alphaIdx)
	// 落屏像素取颜色字节最小值为灰度：底为白(255)、字为深灰(30)，
	// 对通道顺序不敏感（ARGB/ABGR/RGBA 布局未知）。
	grayAt := func(x, y int) int {
		o := (y*vw + x) * 4
		v := -1
		for i := 0; i < 4; i++ {
			if i == alphaIdx {
				continue
			}
			if v < 0 || int(buf[o+i]) < v {
				v = int(buf[o+i])
			}
		}
		return v
	}

	// --- 参照：同字体同字号同 hinting 直出 blended surface 合成白底 ---
	if err := Load(); err != nil {
		t.Skipf("SDL DLL 不可用: %v", err)
	}
	ref := openTTF(t, fontPath, size)
	defer ttfCloseFont(ref)
	// 生产 face 在 GHROMEX_LCD=0 下建 face 时设 LIGHT（见 Graphics.face），
	// 参照必须用同一 hinting，否则比的是 hinting 差异而非管线差异。
	ttfSetFontHinting(ref, uintptr(hintLight))
	rw, rh, cov := cmpBlendedCoverage(t, ref, text)
	rgba := cmpCompositeWhite(rw, rh, cov, ink)

	// --- 两边墨迹包围盒对齐（纹理含透明行高留白，包围盒才是字形本体） ---
	bBox := func(isInk func(x, y int) bool, w, h int) (x0, y0, x1, y1 int) {
		x0, y0, x1, y1 = w, h, -1, -1
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if isInk(x, y) {
					if x < x0 {
						x0 = x
					}
					if y < y0 {
						y0 = y
					}
					if x > x1 {
						x1 = x
					}
					if y > y1 {
						y1 = y
					}
				}
			}
		}
		return
	}
	const inkThresh = 250 // 墨<250 视为墨迹（灰 30 + AA 过渡，白底 255）
	p0x, p0y, p1x, p1y := bBox(func(x, y int) bool { return grayAt(x, y) < inkThresh }, vw, vh)
	refInk := func(x, y int) bool {
		o := (y*rw + x) * 4
		return int(rgba[o]) < inkThresh
	}
	r0x, r0y, r1x, r1y := bBox(refInk, rw, rh)
	if p1x < 0 || r1x < 0 {
		t.Fatal("某一边完全无墨迹：生产路径没画出来，或参照渲染失败")
	}
	pw, ph := p1x-p0x+1, p1y-p0y+1
	rwBox, rhBox := r1x-r0x+1, r1y-r0y+1
	t.Logf("[gray] 生产墨迹 %dx%d @(%d,%d) 参照墨迹 %dx%d @(%d,%d) 纹理 %dx%d",
		pw, ph, p0x, p0y, rwBox, rhBox, r0x, r0y, rw, rh)
	if pw != rwBox || ph != rhBox {
		t.Fatalf("墨迹尺寸不一致：生产 %dx%d vs 参照 %dx%d——两边字形本体不同，"+
			"比对失去意义（hinting/字号/字体不一致？）", pw, ph, rwBox, rhBox)
	}
	// 落屏包围盒必须落在窗口内，否则比对区非法
	if p0x < 0 || p0y < 0 || p0x+pw > vw || p0y+ph > vh {
		t.Fatalf("生产墨迹越出窗口：box=(%d,%d)-(%d,%d) 窗口 %dx%d",
			p0x, p0y, p1x, p1y, vw, vh)
	}

	// --- 逐像素比对 ---
	var (
		n        int
		sumAbs   int
		maxDiff  int
		bad      int // |Δ|>4 的像素数
		refCrop  = make([]byte, pw*ph*4)
		prodCrop = make([]byte, pw*ph*4)
	)
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			gp := grayAt(p0x+x, p0y+y)
			ro := ((r0y+y)*rw + (r0x + x)) * 4
			rv := int(rgba[ro])
			d := gp - rv
			if d < 0 {
				d = -d
			}
			sumAbs += d
			if d > maxDiff {
				maxDiff = d
			}
			if d > 4 {
				bad++
			}
			n++
			po := (y*pw + x) * 4
			prodCrop[po] = byte(gp)
			prodCrop[po+1] = byte(gp)
			prodCrop[po+2] = byte(gp)
			prodCrop[po+3] = 255
			copy(refCrop[po:po+4], rgba[ro:ro+4])
		}
	}
	mean := float64(sumAbs) / float64(n)
	badPct := 100 * float64(bad) / float64(n)
	t.Logf("[gray] 逐像素：mean|Δ|=%.2f max|Δ|=%d 超差(>4)=%.2f%% (%d/%d)",
		mean, maxDiff, badPct, bad, n)
	t.Logf("[gray] 生产边缘: %s", cmpMetricsLine(pw, ph, prodCrop))
	t.Logf("[gray] 参照边缘: %s", cmpMetricsLine(pw, ph, refCrop))

	// 判据：管线忠实 ⇔ 绝大多数像素 ±4 内、平均偏差 ~1 级（舍入差）。
	// 若系统性偏大（如预乘/直通错配会让整片边缘浅 10+），这里先炸出来。
	if mean > 2 {
		t.Errorf("平均偏差 %.2f 超阈值：灰度管线存在系统性合成偏差（发虚的可修来源）", mean)
	}
	if badPct > 2 {
		t.Errorf("超差像素占比 %.2f%% > 2%%：落屏结果与字形直出不一致，管线失真", badPct)
	}
}
