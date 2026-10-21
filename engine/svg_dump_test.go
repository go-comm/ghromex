package engine_test

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// SVG 导出必须是合法 XML：font-family 常带引号
// （font-family="Microsoft YaHei", "Segoe UI"），引号不转义的话导出的就不是
// 合法 XML，浏览器直接拒绝渲染——无头调试（-dump-svg）会拿到一张白页。
func TestDumpSVGIsValidXMLWithQuotedFontFamily(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 200)
	doc, err := engine.OpenDocument(vp, `<!doctype html><html><head><style>`+
		`body{font-family:"Microsoft YaHei","Segoe UI",system-ui,sans-serif}`+
		`</style></head><body><p>hello</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	doc.Refresh()
	svg := doc.DumpSVG()
	if !strings.Contains(svg, "&quot;") {
		t.Fatalf("SVG 没有转义 font-family 里的引号:\n%s", svg)
	}
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := dec.Token(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("导出的 SVG 不是合法 XML: %v\n%s", err, svg)
		}
	}
}
