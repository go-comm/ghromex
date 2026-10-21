package engine_test

import (
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
