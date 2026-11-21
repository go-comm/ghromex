package engine_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-comm/ghromex/engine"
)

// testPNG 生成一张 w×h 的 NRGBA PNG；fill 为空时铺不透明红色。
func testPNG(t *testing.T, w, h int, fill func(x, y int) color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if fill != nil {
				c = fill(x, y)
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// pngDataURL 把 PNG 字节包成 data: URL（引擎内建解码，无需加载器）。
func pngDataURL(pngBytes []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
}

// TestImageIntrinsicAndDeclaredSize 覆盖替换元素的尺寸来源：
// 固有像素 → width/height 属性 → CSS 声明 → 单边已知时按纵横比补另一边。
func TestImageIntrinsicAndDeclaredSize(t *testing.T) {
	url := pngDataURL(testPNG(t, 4, 2, nil))
	src := `<!doctype html><html><body style="margin:0">
		<img id="a" src="` + url + `">
		<img id="b" src="` + url + `" width="8">
		<img id="c" src="` + url + `" height="8">
		<img id="d" src="` + url + `" width="10" height="6">
		<img id="e" src="` + url + `" style="width:6px">
		<img id="f" src="` + url + `" style="width:auto;height:10px">
		<img id="g" src="` + url + `" width="nope">
		<img id="h">
	</body></html>`
	doc := openDoc(t, 300, 300, src)

	cases := []struct {
		sel  string
		w, h int
	}{
		{"#a", 4, 2},   // 固有尺寸
		{"#b", 8, 4},   // 属性宽度 + 纵横比
		{"#c", 16, 8},  // 属性高度 + 纵横比
		{"#d", 10, 6},  // 两边都声明：拉伸
		{"#e", 6, 3},   // CSS 宽度优先 + 纵横比
		{"#f", 20, 10}, // CSS 高度 + 纵横比
		{"#g", 4, 2},   // 非法属性按未声明
		{"#h", 0, 0},   // 无 src、无声明：占 0×0
	}
	for _, c := range cases {
		el := mustEl(t, doc, c.sel)
		if el.Width() != c.w || el.Height() != c.h {
			t.Errorf("%s = %dx%d, want %dx%d", c.sel, el.Width(), el.Height(), c.w, c.h)
		}
	}
}

// TestImageRenderPixels 断言位图真的落到了内容盒上（含缩放与透明像素）。
func TestImageRenderPixels(t *testing.T) {
	raw := testPNG(t, 2, 2, func(x, y int) color.NRGBA {
		switch {
		case x == 0 && y == 0:
			return color.NRGBA{R: 255, A: 255}
		case x == 1 && y == 0:
			return color.NRGBA{G: 255, A: 255}
		case x == 0 && y == 1:
			return color.NRGBA{B: 255, A: 255}
		default:
			return color.NRGBA{} // alpha=0：完全透明
		}
	})
	src := `<!doctype html><html><body style="margin:0"><img id="i" width="4" height="4" src="` +
		pngDataURL(raw) + `" style="display:block"></body></html>`
	buf, _, err := engine.OpenBufferedDocument(20, 20, src)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}

	// 最近邻放大：2×2 → 4×4，每格 2×2。
	check := func(x, y int, r, g, b uint8, alphaOK bool) {
		t.Helper()
		gr, gg, gb, ga, ok := buf.ColorAt(x, y)
		if !ok {
			t.Fatalf("ColorAt(%d,%d) 越界", x, y)
		}
		if gr != r || gg != g || gb != b || (alphaOK && ga != 255) {
			t.Errorf("pixel(%d,%d) = %d,%d,%d,%d, want %d,%d,%d,255", x, y, gr, gg, gb, ga, r, g, b)
		}
	}
	check(0, 0, 255, 0, 0, true)
	check(1, 1, 255, 0, 0, true)
	check(2, 0, 0, 255, 0, true)
	check(3, 1, 0, 255, 0, true)
	check(0, 2, 0, 0, 255, true)
	check(1, 3, 0, 0, 255, true)

	// 透明像素不落墨：缓冲区仍全透明。
	if _, _, _, a, _ := buf.ColorAt(2, 2); a != 0 {
		t.Errorf("透明像素被画上去了：alpha=%d, want 0", a)
	}
}

// TestImageOverflowClip 断言图片能被父级 overflow:hidden 裁掉（与子元素同口径）。
func TestImageOverflowClip(t *testing.T) {
	raw := testPNG(t, 2, 2, nil)
	src := `<!doctype html><html><body style="margin:0">
		<div style="width:2px;height:2px;overflow:hidden">
			<img width="4" height="4" src="` + pngDataURL(raw) + `" style="display:block">
		</div>
	</body></html>`
	buf, _, err := engine.OpenBufferedDocument(20, 20, src)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	if r, g, b, a, _ := buf.ColorAt(0, 0); r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("裁剪区内 pixel(0,0) = %d,%d,%d,%d, want red", r, g, b, a)
	}
	if _, _, _, a, _ := buf.ColorAt(3, 3); a != 0 {
		t.Errorf("裁剪区外 pixel(3,3) alpha=%d, want 0（overflow:hidden 未生效）", a)
	}
}

// TestImageOpacity 断言 opacity 会乘到位图像素的 alpha 上。
func TestImageOpacity(t *testing.T) {
	raw := testPNG(t, 2, 2, nil)
	src := `<!doctype html><html><body style="margin:0"><img width="2" height="2" src="` +
		pngDataURL(raw) + `" style="display:block;opacity:0.5"></body></html>`
	buf, _, err := engine.OpenBufferedDocument(10, 10, src)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	// 缓冲区按直通 alpha 混合：50% 不透明红落在透明底上，结果是
	// 预乘后的 r≈128 / a≈128（不是 r=255 的“半透红”，那样再叠加会变亮）。
	if r, _, _, a, _ := buf.ColorAt(0, 0); r < 120 || r > 135 || a < 120 || a > 135 {
		t.Errorf("opacity=.5 pixel = r%d a%d, want r≈128 a≈128", r, a)
	}
}

// TestImageInlineBaselineMargin 锁死行内替换元素的基线口径：对齐的是
// **底外边距边**，所以 margin-bottom 会把图片整体上抬同样的量（Chrome 实测
// 同款），而不是在图片下方撑高行盒。同一条行盒里两张图共用基线：
// 高 2 + 下外边距 10 的那张，顶边比高 2 但无外边距的那张高 10px。
func TestImageInlineBaselineMargin(t *testing.T) {
	url := pngDataURL(testPNG(t, 4, 2, nil))
	src := `<!doctype html><html><body style="margin:0"><div>
		<img id="a" width="4" height="2" src="` + url + `">
		<img id="b" width="4" height="2" style="margin-bottom:10px" src="` + url + `">
	</div></body></html>`
	doc := openDoc(t, 100, 100, src)
	ya := mustEl(t, doc, "#a").GetBoundingClientRect().Top().Pixel()
	yb := mustEl(t, doc, "#b").GetBoundingClientRect().Top().Pixel()
	if ya-yb != 10 {
		t.Errorf("#a.Top - #b.Top = %d, want 10（基线应取底外边距边：%d / %d）", ya-yb, ya, yb)
	}
}

// TestImageAsyncLoader 验证 loader 注入：首帧 0×0、解码到货后 ChangeCount
// 增长并重排，同一 src 的多个 <img> 只加载一次。
func TestImageAsyncLoader(t *testing.T) {
	raw := testPNG(t, 4, 2, nil)
	var calls int32
	loader := func(src string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond) // 强制走异步路径
		return raw, nil
	}
	// display:block 让两张图各自成块（y=0、y=2）：行内 <img> 要落在行基线
	// 上，基线位置随字体度量变（无头 / 软件光栅 / 真实后端各不同），不适合
	// 做像素断言。
	src := `<!doctype html><html><body style="margin:0">
		<img id="a" style="display:block" src="file:///tmp/a.png">
		<img id="b" style="display:block" src="file:///tmp/a.png">
	</body></html>`
	vp, buf := engine.NewBufferedViewport(40, 40)
	doc, err := engine.OpenDocumentWith(vp, src, engine.DocumentOptions{LoadImage: loader})
	if err != nil {
		t.Fatalf("OpenDocumentWith: %v", err)
	}
	defer doc.Close()

	if w := mustEl(t, doc, "#a").Width(); w != 0 {
		t.Fatalf("加载完成前 #a.Width() = %d, want 0", w)
	}

	base := doc.ChangeCount()
	paintBase := engine.PaintCount(doc)
	deadline := time.Now().Add(2 * time.Second)
	for {
		level, _, _ := engine.PendingChange(doc, base, paintBase)
		if level == engine.ChangeReflow {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("等待图片解码超时")
		}
		time.Sleep(5 * time.Millisecond)
	}
	engine.LayoutDocument(doc)
	engine.RenderNode(buf, doc)

	if w, h := mustEl(t, doc, "#a").Width(), mustEl(t, doc, "#a").Height(); w != 4 || h != 2 {
		t.Errorf("#a = %dx%d, want 4x2", w, h)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("loader 调用 %d 次, want 1（同一 src 应共享缓存）", got)
	}
	if r, _, _, a, _ := buf.ColorAt(0, 0); r != 255 || a != 255 {
		t.Errorf("异步到货后 pixel(0,0) = r%d a%d, want red", r, a)
	}
}

// TestImageLoaderFailure 加载失败按空图处理：不占尺寸、不落墨、不 panic。
func TestImageLoaderFailure(t *testing.T) {
	loader := func(src string) ([]byte, error) {
		time.Sleep(5 * time.Millisecond)
		return nil, testErr("boom")
	}
	src := `<!doctype html><html><body style="margin:0"><img id="a" src="file:///tmp/missing.png"></body></html>`
	vp, _ := engine.NewBufferedViewport(20, 20)
	doc, err := engine.OpenDocumentWith(vp, src, engine.DocumentOptions{LoadImage: loader})
	if err != nil {
		t.Fatalf("OpenDocumentWith: %v", err)
	}
	defer doc.Close()

	base := doc.ChangeCount()
	paintBase := engine.PaintCount(doc)
	deadline := time.Now().Add(2 * time.Second)
	for {
		level, _, _ := engine.PendingChange(doc, base, paintBase)
		if level == engine.ChangeReflow {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("等待加载失败结果超时")
		}
		time.Sleep(5 * time.Millisecond)
	}
	engine.LayoutDocument(doc)
	if w, h := mustEl(t, doc, "#a").Width(), mustEl(t, doc, "#a").Height(); w != 0 || h != 0 {
		t.Errorf("失败图片 = %dx%d, want 0x0", w, h)
	}
}

// TestImageSVG 导出为内嵌 data URI 的 <image>；未加载/display:none 不导出。
func TestImageSVG(t *testing.T) {
	url := pngDataURL(testPNG(t, 3, 2, nil))
	doc := openDoc(t, 50, 50, `<!doctype html><html><body style="margin:0">
		<img id="a" src="`+url+`">
		<img id="b" style="display:none" src="`+url+`">
	</body></html>`)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, "<image ") || !strings.Contains(svg, "data:image/png;base64,") {
		t.Fatalf("DumpSVG 缺少 <image> data URI:\n%s", svg)
	}
	if strings.Count(svg, "<image ") != 1 {
		t.Errorf("<image> 数量 = %d, want 1（display:none 不应导出）", strings.Count(svg, "<image "))
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }
