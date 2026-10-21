package engine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// textCounter 统计 DrawText 的次数与文字内容。
type textCounter struct {
	engine.Graphics
	calls int
	texts []string
}

func (c *textCounter) DrawText(x, y, w, h int, paint engine.Paint, text string) {
	c.calls++
	c.texts = append(c.texts, text)
	c.Graphics.DrawText(x, y, w, h, paint, text)
}

// 视口裁剪：完全落在视口外的文字不画。
//
// 滚动动画每帧都要重画整页，不裁剪的话长页的下半部分也在白画——实测
// portal 页一帧 1277 次 DrawText，裁掉后只剩 433 次（视口内的部分）。
// 帧时间顶到 vsync 之外就是掉帧，所以这条要钉住。
func TestRenderSkipsOffscreenText(t *testing.T) {
	// 10 屏高的页面：每屏一行文字（内容用 ASCII，避免按字符切分干扰计数）。
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body style="margin:0">`)
	for i := 0; i < 10; i++ {
		b.WriteString(`<div style="height:200px">row`)
		b.WriteString(string(rune('A' + i)))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</body></html>`)

	const vh = 200
	vp := engine.NewHeadlessViewport(200, vh)
	doc, err := engine.OpenDocument(vp, b.String())
	if err != nil {
		t.Fatal(err)
	}
	buf := engine.NewBufferGraphics(200, vh)
	counter := &textCounter{Graphics: buf}
	engine.RenderNode(counter, doc)

	// 视口 200px 高：只有第一行在视口内，其余 9 行在视口外。
	for _, got := range counter.texts {
		if strings.Contains(got, "row") && !strings.Contains(got, "rowA") {
			t.Errorf("画出了视口外的文字 %q（视口内只该有 rowA）", got)
		}
	}
	if len(counter.texts) == 0 {
		t.Error("视口内的文字没画出来")
	}
	// 视口外的 9 行不画：绘制调用数应远小于整页（10 行 × 每行若干片段）。
	if counter.calls > 8 {
		t.Errorf("DrawText 次数 = %d，want ≤ 8（视口外的 9 行不该画）", counter.calls)
	}
}

// 视口内的文字必须照画：裁剪不能误伤可见内容。
func TestRenderKeepsOnscreenText(t *testing.T) {
	const src = `<!doctype html><html><body style="margin:0">
		<div style="height:100px">up</div>
		<div style="height:100px">down</div>
	</body></html>`
	vp := engine.NewHeadlessViewport(200, 200)
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatal(err)
	}
	buf := engine.NewBufferGraphics(200, 200)
	counter := &textCounter{Graphics: buf}
	engine.RenderNode(counter, doc)

	if counter.calls != 2 {
		t.Errorf("DrawText 次数 = %d，want 2（两行都在视口内）", counter.calls)
	}
}

// SVG 导出不受视口裁剪影响：它要的是整页（导出的图与窗口大小无关）。
func TestSVGDumpKeepsOffscreenContent(t *testing.T) {
	const src = `<!doctype html><html><body style="margin:0">
		<div style="height:100px">visible</div>
		<div style="height:900px">hidden</div>
	</body></html>`
	vp := engine.NewHeadlessViewport(200, 200)
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatal(err)
	}
	svg := doc.DumpSVG()
	if !strings.Contains(svg, "visible") || !strings.Contains(svg, "hidden") {
		t.Errorf("SVG 导出缺少内容（应含整页，与视口裁剪无关）:\n%s", svg)
	}
}

// 裁剪后的画面必须与「不裁剪」逐像素相同：裁错一个盒子，元素就会在滚动
// 时闪进闪出（视口边缘的判断一旦算错，元素会忽隐忽现）。
//
// 对照组用「足够高的视口」渲染同一页面（不触发裁剪），再逐像素比。
func TestRenderCullKeepsVisiblePixels(t *testing.T) {
	src, err := os.ReadFile("../examples/portal/index.html")
	if err != nil {
		t.Fatal(err)
	}
	const w = 1280
	vp := engine.NewHeadlessViewport(w, 820)
	doc, err := engine.OpenDocument(vp, string(src))
	if err != nil {
		t.Fatal(err)
	}
	culled := engine.NewBufferGraphics(w, 820)
	engine.RenderNode(culled, doc)

	vp2 := engine.NewHeadlessViewport(w, 6000)
	doc2, err := engine.OpenDocument(vp2, string(src))
	if err != nil {
		t.Fatal(err)
	}
	full := engine.NewBufferGraphics(w, 6000)
	engine.RenderNode(full, doc2)

	diff, firstX, firstY := 0, -1, -1
	for y := 0; y < 820; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1, _, _ := culled.ColorAt(x, y)
			r2, g2, b2, _, _ := full.ColorAt(x, y)
			if r1 != r2 || g1 != g2 || b1 != b2 {
				diff++
				if firstX < 0 {
					firstX, firstY = x, y
				}
			}
		}
	}
	if diff != 0 {
		t.Errorf("裁剪后画面内 %d 个像素与不裁剪时不同（首个差异在 %d,%d）：裁剪画错了内容",
			diff, firstX, firstY)
	}
}
