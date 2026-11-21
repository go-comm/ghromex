package engine_test

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/go-comm/ghromex/engine"
)

// bgColorPNG 生成一张纯色 PNG（w×h）。
func bgColorPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	return testPNG(t, w, h, func(x, y int) color.NRGBA { return c })
}

// fourColorPNG 生成 2×2 的四色图：左上红、右上绿、左下蓝、右下白。
func fourColorPNG(t *testing.T) []byte {
	t.Helper()
	return testPNG(t, 2, 2, func(x, y int) color.NRGBA {
		switch {
		case x == 0 && y == 0:
			return color.NRGBA{R: 255, A: 255}
		case x == 1 && y == 0:
			return color.NRGBA{G: 255, A: 255}
		case x == 0 && y == 1:
			return color.NRGBA{B: 255, A: 255}
		default:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		}
	})
}

// bgPixel 读一个像素，返回 r,g,b,a 与是否在缓冲区内。
func bgPixel(t *testing.T, buf *engine.BufferGraphics, x, y int) (uint8, uint8, uint8, uint8) {
	t.Helper()
	r, g, b, a, ok := buf.ColorAt(x, y)
	if !ok {
		t.Fatalf("ColorAt(%d,%d) 越界", x, y)
	}
	return r, g, b, a
}

// assertPainted 断言像素是期望的颜色（不透明）。
func assertPainted(t *testing.T, buf *engine.BufferGraphics, x, y int, r, g, b uint8) {
	t.Helper()
	gr, gg, gb, ga := bgPixel(t, buf, x, y)
	if gr != r || gg != g || gb != b || ga != 255 {
		t.Errorf("pixel(%d,%d) = %d,%d,%d,%d，期望 %d,%d,%d,255", x, y, gr, gg, gb, ga, r, g, b)
	}
}

// assertBlank 断言像素没落墨（alpha=0）。
func assertBlank(t *testing.T, buf *engine.BufferGraphics, x, y int) {
	t.Helper()
	_, _, _, ga := bgPixel(t, buf, x, y)
	if ga != 0 {
		t.Errorf("pixel(%d,%d) alpha=%d，期望 0（没落墨）", x, y, ga)
	}
}

// TestBackgroundImagePaintsPixels 断言背景图真的落到盒子上：固有尺寸、
// 默认位置（0% 0%）、不平铺时只画一张。
func TestBackgroundImagePaintsPixels(t *testing.T) {
	url := pngDataURL(bgColorPNG(t, 2, 2, color.NRGBA{R: 255, A: 255}))
	buf, _, err := engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:6px;height:6px;background-image:url(`+url+`);background-repeat:no-repeat"></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	assertPainted(t, buf, 0, 0, 255, 0, 0)
	assertPainted(t, buf, 1, 1, 255, 0, 0)
	assertBlank(t, buf, 2, 2) // 固有尺寸 2×2，画完就没了
	assertBlank(t, buf, 5, 5)
}

// TestBackgroundSizeStretch 断言 background-size 百分比相对定位盒。
func TestBackgroundSizeStretch(t *testing.T) {
	url := pngDataURL(bgColorPNG(t, 2, 2, color.NRGBA{R: 255, A: 255}))
	buf, _, err := engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:6px;height:4px;background-image:url(`+url+`);background-size:100% 100%"></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	assertPainted(t, buf, 0, 0, 255, 0, 0)
	assertPainted(t, buf, 5, 3, 255, 0, 0) // 铺满 6×4
	assertBlank(t, buf, 6, 0)
}

// TestBackgroundRepeatTiles 断言平铺：网格对齐，边缘那块只画可见的那一列
// （源子矩形映射，不是整图拉伸）。
func TestBackgroundRepeatTiles(t *testing.T) {
	url := pngDataURL(fourColorPNG(t))
	buf, _, err := engine.OpenBufferedDocument(10, 10, `<!doctype html><html><body style="margin:0">
		<div style="width:5px;height:5px;background-image:url(`+url+`)"></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	assertPainted(t, buf, 0, 0, 255, 0, 0) // 第一块左上：红
	assertPainted(t, buf, 1, 0, 0, 255, 0) // 绿
	assertPainted(t, buf, 0, 1, 0, 0, 255) // 蓝
	assertPainted(t, buf, 1, 1, 255, 255, 255)
	assertPainted(t, buf, 2, 0, 255, 0, 0) // 第二块起点 x=2
	assertPainted(t, buf, 4, 0, 255, 0, 0) // 第三块只露一列：源第 0 列
	assertPainted(t, buf, 4, 1, 0, 0, 255) // 源第 0 列第 1 行
	assertPainted(t, buf, 0, 4, 255, 0, 0) // 竖向第三块只露一行：源第 0 行
}

// TestBackgroundPositionOffsets 断言 background-position：百分比与关键字
// 的基数都是「定位盒 − 图片尺寸」。
func TestBackgroundPositionOffsets(t *testing.T) {
	url := pngDataURL(fourColorPNG(t))
	mk := func(pos string) *engine.BufferGraphics {
		buf, _, err := engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
			<div style="width:10px;height:10px;background-image:url(`+url+`);background-repeat:no-repeat;background-position:`+pos+`"></div>
		</body></html>`)
		if err != nil {
			t.Fatalf("OpenBufferedDocument: %v", err)
		}
		return buf
	}
	// 50% 50% → 偏移 (10-2)/2 = 4
	buf := mk("50% 50%")
	assertPainted(t, buf, 4, 4, 255, 0, 0)
	assertPainted(t, buf, 5, 5, 255, 255, 255)
	assertBlank(t, buf, 3, 3)
	assertBlank(t, buf, 6, 6)
	// right bottom → 贴右下角
	buf = mk("right bottom")
	assertPainted(t, buf, 8, 8, 255, 0, 0)
	assertPainted(t, buf, 9, 9, 255, 255, 255)
	assertBlank(t, buf, 7, 7)
	// left top → 初始位置
	buf = mk("left top")
	assertPainted(t, buf, 0, 0, 255, 0, 0)
	assertBlank(t, buf, 2, 2)
}

// TestBackgroundPositionEdgeOffset 断言「贴边再偏移」的写法：
// right 1px bottom 1px → 贴右下角再往里挪 1px。
func TestBackgroundPositionEdgeOffset(t *testing.T) {
	url := pngDataURL(bgColorPNG(t, 2, 2, color.NRGBA{G: 255, A: 255}))
	buf, _, err := engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:10px;height:10px;background-image:url(`+url+`);background-repeat:no-repeat;background-position:right 1px bottom 1px"></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	// 贴边偏移 = 定位盒 − 图片尺寸 − 1 = 7
	assertPainted(t, buf, 7, 7, 0, 255, 0)
	assertPainted(t, buf, 8, 8, 0, 255, 0)
	assertBlank(t, buf, 6, 6)
}

// TestBackgroundCoverContain 断言 cover / contain 的缩放口径。
func TestBackgroundCoverContain(t *testing.T) {
	url := pngDataURL(fourColorPNG(t))
	mk := func(size string) *engine.BufferGraphics {
		buf, _, err := engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
			<div style="width:8px;height:4px;background-image:url(`+url+`);background-repeat:no-repeat;background-size:`+size+`"></div>
		</body></html>`)
		if err != nil {
			t.Fatalf("OpenBufferedDocument: %v", err)
		}
		return buf
	}
	// cover：缩放到填满 8×4（按 8/2=4 倍），纵向溢出被裁掉
	buf := mk("cover")
	assertPainted(t, buf, 0, 0, 255, 0, 0)
	assertPainted(t, buf, 7, 0, 0, 255, 0)
	assertBlank(t, buf, 8, 0)
	// contain：装进 8×4（按 4/2=2 倍）→ 4×4，靠左上
	buf = mk("contain")
	assertPainted(t, buf, 0, 0, 255, 0, 0)
	assertPainted(t, buf, 3, 3, 255, 255, 255)
	assertBlank(t, buf, 4, 0)
}

// TestBackgroundMultipleLayers 断言多层叠放顺序：CSS 里首层画在最上面。
func TestBackgroundMultipleLayers(t *testing.T) {
	red := pngDataURL(bgColorPNG(t, 2, 2, color.NRGBA{R: 255, A: 255}))
	blue := pngDataURL(bgColorPNG(t, 2, 2, color.NRGBA{B: 255, A: 255}))
	buf, _, err := engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:2px;height:2px;background-image:url(`+red+`), url(`+blue+`)"></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	assertPainted(t, buf, 0, 0, 255, 0, 0) // 首层压住第二层
	assertPainted(t, buf, 1, 1, 255, 0, 0)

	buf, _, err = engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:2px;height:2px;background-image:url(`+blue+`), url(`+red+`)"></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	assertPainted(t, buf, 0, 0, 0, 0, 255) // 顺序反过来，首层是蓝
}

// TestBackgroundClipAndOrigin 断言 origin（定位盒）与 clip（裁剪盒）是两套
// 口径：origin 决定图从哪儿画起，clip 决定画到哪儿为止。
//
// 盒子：width/height 10px + padding 4px → 内容盒 (4,4,10,10)、
// padding 盒 = 边框盒 (0,0,18,18)。图 20×20，比所有盒子都大。
func TestBackgroundClipAndOrigin(t *testing.T) {
	red := pngDataURL(bgColorPNG(t, 20, 20, color.NRGBA{R: 255, A: 255}))
	mk := func(style string) *engine.BufferGraphics {
		buf, _, err := engine.OpenBufferedDocument(24, 24, `<!doctype html><html><body style="margin:0">
			<div style="width:10px;height:10px;padding:4px;background-image:url(`+red+`);background-repeat:no-repeat;`+style+`"></div>
		</body></html>`)
		if err != nil {
			t.Fatalf("OpenBufferedDocument: %v", err)
		}
		return buf
	}
	// 默认：定位盒 = padding 盒（0,0），裁剪盒 = 边框盒（= padding 盒）
	buf := mk("")
	assertPainted(t, buf, 0, 0, 255, 0, 0)
	assertPainted(t, buf, 17, 17, 255, 0, 0)
	assertBlank(t, buf, 18, 18)
	// origin: content-box → 从内容盒 (4,4) 画起。
	buf = mk("background-origin:content-box")
	assertBlank(t, buf, 0, 0)
	assertBlank(t, buf, 3, 3)
	assertPainted(t, buf, 4, 4, 255, 0, 0)
	// clip: content-box → 只画到内容盒 (4,4,10,10) 为止。
	buf = mk("background-clip:content-box")
	assertBlank(t, buf, 0, 0)
	assertBlank(t, buf, 3, 3)
	assertPainted(t, buf, 4, 4, 255, 0, 0)
	assertPainted(t, buf, 13, 13, 255, 0, 0)
	assertBlank(t, buf, 14, 14)
}

// TestBackgroundShorthandPaint 断言 background 简写：能带颜色、能带图、
// 颜色与图一起用时颜色在图下面。
func TestBackgroundShorthandPaint(t *testing.T) {
	url := pngDataURL(bgColorPNG(t, 2, 2, color.NRGBA{R: 255, A: 255}))
	// 简写里的颜色是背景色，图盖在它上面。
	buf, _, err := engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:4px;height:4px;background:#0000ff url(`+url+`) no-repeat"></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	assertPainted(t, buf, 0, 0, 255, 0, 0) // 图压住背景色
	assertPainted(t, buf, 3, 3, 0, 0, 255) // 没图的地方是背景色
	// 只有颜色：简写等价于 background-color。
	buf, _, err = engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:4px;height:4px;background:#0000ff"></div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	assertPainted(t, buf, 0, 0, 0, 0, 255)
}

// TestBackgroundNotInherited 断言背景不继承：子元素不画父元素的背景图。
func TestBackgroundNotInherited(t *testing.T) {
	url := pngDataURL(bgColorPNG(t, 2, 2, color.NRGBA{R: 255, A: 255}))
	buf, doc, err := engine.OpenBufferedDocument(20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:2px;height:2px;background-image:url(`+url+`)">
			<div style="width:2px;height:2px"></div>
		</div>
	</body></html>`)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	assertPainted(t, buf, 0, 0, 255, 0, 0)
	assertBlank(t, buf, 0, 2) // 子元素在自己的位置，不继承父元素的图
	// SVG 导出同样只导出父元素那一层。
	if n := strings.Count(doc.DumpSVG(), "<image "); n != 1 {
		t.Errorf("SVG 里的 <image> 数量 = %d，期望 1（背景不继承）", n)
	}
}

// TestBackgroundImageSVG 断言 SVG 导出用 <pattern> 表达平铺。
func TestBackgroundImageSVG(t *testing.T) {
	url := pngDataURL(bgColorPNG(t, 2, 2, color.NRGBA{R: 255, A: 255}))
	doc := openDoc(t, 20, 20, `<!doctype html><html><body style="margin:0">
		<div style="width:6px;height:6px;background-image:url(`+url+`)"></div>
	</body></html>`)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, "<pattern") || !strings.Contains(svg, `fill="url(#bgs`) {
		t.Fatalf("DumpSVG 缺少背景图 pattern:\n%s", svg)
	}
	if !strings.Contains(svg, "data:image/png;base64,") {
		t.Errorf("背景图没有内嵌 data URI:\n%s", svg)
	}
}

// TestBackgroundImageArrivesAsPaintOnly 断言背景图异步到货只算「重绘」，
// 不算「重排」——背景不影响盒模型。
func TestBackgroundImageArrivesAsPaintOnly(t *testing.T) {
	png := bgColorPNG(t, 2, 2, color.NRGBA{R: 255, A: 255})
	release := make(chan struct{})
	loader := func(src string) ([]byte, error) {
		<-release // 等到测试放行再返回，模拟「图晚到」
		return png, nil
	}
	src := `<!doctype html><html><body style="margin:0">
		<div id="a" style="width:4px;height:4px;background-image:url(file:///tmp/bg.png)"></div>
	</body></html>`
	vp, buf := engine.NewBufferedViewport(20, 20)
	doc, err := engine.OpenDocumentWith(vp, src, engine.DocumentOptions{LoadImage: loader})
	if err != nil {
		t.Fatalf("OpenDocumentWith: %v", err)
	}
	defer doc.Close()

	// 第一帧：图还没到，什么都不画（但登记已经发生）。
	engine.LayoutDocument(doc)
	engine.RenderNode(buf, doc)
	assertBlank(t, buf, 0, 0)

	base := doc.ChangeCount()
	paintBase := engine.PaintCount(doc)
	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for {
		level, _, _ := engine.PendingChange(doc, base, paintBase)
		if level != engine.ChangeNone {
			if level != engine.ChangePaintOnly {
				t.Errorf("背景图到货的变更档位 = %v，期望 ChangePaintOnly（不该重排）", level)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("等待背景图到货超时")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := doc.ChangeCount(); got != base {
		t.Errorf("背景图到货后 ChangeCount = %d，期望 %d（不该触发重排）", got, base)
	}

	engine.LayoutDocument(doc)
	engine.RenderNode(buf, doc)
	assertPainted(t, buf, 0, 0, 255, 0, 0)
}

// TestBackgroundDoesNotAffectLayout 断言背景图不影响盒模型：有无背景图，
// 元素的尺寸都一样。
func TestBackgroundDoesNotAffectLayout(t *testing.T) {
	url := pngDataURL(bgColorPNG(t, 40, 40, color.NRGBA{R: 255, A: 255}))
	doc := openDoc(t, 100, 100, `<!doctype html><html><body style="margin:0">
		<div id="a" style="width:10px">文字</div>
		<div id="b" style="width:10px;background-image:url(`+url+`)">文字</div>
	</body></html>`)
	a := mustEl(t, doc, "#a")
	b := mustEl(t, doc, "#b")
	if a.Width() != b.Width() || a.Height() != b.Height() {
		t.Errorf("有背景图的盒子 = %dx%d，无背景图 = %dx%d，尺寸应当一致",
			b.Width(), b.Height(), a.Width(), a.Height())
	}
}

// TestSetBackgroundImageAPI 断言公开 API 的往返：SetBackgroundImage 造单层
// 铺满的图层，BackgroundImage 取回首层位图，传 nil 清空。
//
// 只测 API 本身：级联读的是 style 属性解析出的声明，不是 Style() 这个活对象
// （见 html_element.go 的注释），所以这里不断言渲染结果。
func TestSetBackgroundImageAPI(t *testing.T) {
	doc := openDoc(t, 20, 20, `<!doctype html><html><body style="margin:0">
		<div id="a" style="width:4px;height:4px"></div>
	</body></html>`)
	st := mustEl(t, doc, "#a").Style()
	if st.BackgroundImage() != nil {
		t.Fatalf("初始应有背景图")
	}
	pix := make([]byte, 2*2*4)
	for i := range pix {
		pix[i] = 255
	}
	bmp := &engine.Bitmap{W: 2, H: 2, Pix: pix}
	if st.SetBackgroundImage(bmp) != st {
		t.Errorf("SetBackgroundImage 没有返回自身（链式调用）")
	}
	if got := st.BackgroundImage(); got != bmp {
		t.Errorf("BackgroundImage() = %v，期望拿回刚设的位图", got)
	}
	if st.SetBackgroundImage(nil).BackgroundImage() != nil {
		t.Errorf("传 nil 没清空背景图层")
	}
	// 非 *Bitmap 的 Image 实现按「没有背景图」处理（引擎只认位图）。
	st.SetBackgroundImage(bmp)
	if st.BackgroundImage() == nil {
		t.Errorf("位图没绑上")
	}
}
