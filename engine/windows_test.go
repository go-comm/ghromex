package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

const demoPage = `<!doctype html>
<html>
<head>
	<title>测试页</title>
	<style>
		body { margin: 0; }
		.card { width: 200px; padding: 10px; background-color: #FFFFFF; border: 1px solid #CCCCCC; }
		.btn-primary { background-color: #2563EB; color: #FFFFFF; }
		#status { color: #666666; }
	</style>
</head>
<body>
	<div class="card">
		<span class="title">登录</span>
		<div><button class="btn-primary" id="login">登录</button></div>
		<span id="status">Ready.</span>
	</div>
</body>
</html>`

func TestParseAndLayout(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, demoPage)
	if err != nil {
		t.Fatal(err)
	}

	if got := doc.Title(); got != "测试页" {
		t.Fatalf("title = %q", got)
	}

	card := doc.QuerySelector(".card")
	if card == nil {
		t.Fatal(".card not found")
	}
	// 200px 内容宽 + 20px padding + 2px border = 222 边框盒
	r := card.GetBoundingClientRect()
	if w := r.Right().Pixel() - r.Left().Pixel(); w != 222 {
		t.Fatalf("card border-box width = %d, want 222", w)
	}

	btn := doc.QuerySelector("#login")
	if btn == nil {
		t.Fatal("#login not found")
	}
	br := btn.GetBoundingClientRect()
	if br.Bottom().Pixel()-br.Top().Pixel() <= 0 {
		t.Fatal("button must have layout height")
	}

	// 渲染走一遍不应 panic
	engine.RenderNode(vp.Graphics(), doc)

	svg := doc.DumpSVG()
	// CJK 文本按单字断行，每个字是独立 <text> run，故分别断言而非整词
	if !strings.Contains(svg, "登") || !strings.Contains(svg, "录") {
		t.Fatalf("svg missing text: %s", svg)
	}
}

func TestClickDispatch(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, demoPage)
	if err != nil {
		t.Fatal(err)
	}

	fired := 0
	status := doc.QuerySelector("#status")
	doc.QuerySelector("#login").OnClick(func(ev *engine.MouseEvent) {
		fired++
		status.SetText("clicked")
	})

	btn := doc.QuerySelector("#login").GetBoundingClientRect()
	cx := (btn.Left().Pixel() + btn.Right().Pixel()) / 2
	cy := (btn.Top().Pixel() + btn.Bottom().Pixel()) / 2
	if !engine.OnDocumentClick(doc, cx, cy) {
		t.Fatal("click not dispatched")
	}
	if fired != 1 {
		t.Fatalf("handler fired %d times", fired)
	}
	if status.InnerText() != "clicked" {
		t.Fatalf("status = %q", status.InnerText())
	}

	// 空白处不触发
	before := doc.ChangeCount()
	engine.OnDocumentClick(doc, 390, 290)
	if doc.ChangeCount() != before {
		t.Fatal("empty click should not mutate document")
	}
}

func TestCSSSpecificity(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	src := `<html><head><style>
		#only { width: 100px; }
		div.wide { width: 300px; }
		div { width: 50px; }
	</style></head><body><div class="wide" id="only"></div></body></html>`
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatal(err)
	}
	r := doc.QuerySelector("div").GetBoundingClientRect()
	if w := r.Right().Pixel() - r.Left().Pixel(); w != 100 {
		t.Fatalf("id selector should win, width = %d", w)
	}
}

func TestTokenizeAndWrap(t *testing.T) {
	vp := engine.NewHeadlessViewport(120, 300)
	src := `<html><body><div>hello world 你好世界 hello</div></body></html>`
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatal(err)
	}
	div := doc.QuerySelector("div")
	if div == nil {
		t.Fatal("div not found")
	}
	r := div.GetBoundingClientRect()
	h := r.Bottom().Pixel() - r.Top().Pixel()
	if h <= 24 {
		t.Fatalf("text should wrap into multiple lines, div height = %d", h)
	}
}
