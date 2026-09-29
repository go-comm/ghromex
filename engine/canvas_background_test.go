package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 传播规则：画布底色 = html 背景，html 无则退 body，均无则 nil（保持默认白）。
func TestCanvasBackgroundPropagation(t *testing.T) {
	cases := []struct {
		name string
		css  string
		want string // "#rrggbb"，空串表示期望 nil
	}{
		{"body only", `body { background-color: #EEF1F7; }`, "#eef1f7"},
		{"html wins", `html { background-color: #0F172A; } body { background-color: #EEF1F7; }`, "#0f172a"},
		{"none", `body { margin: 0; }`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `<!doctype html><html><head><style>` + tc.css +
				`</style></head><body><div>hi</div></body></html>`
			vp := engine.NewHeadlessViewport(100, 100)
			doc, err := engine.OpenDocument(vp, src)
			if err != nil {
				t.Fatal(err)
			}
			c := engine.CanvasBackground(doc)
			if tc.want == "" {
				if c != nil {
					t.Fatalf("CanvasBackground = non-nil, want nil")
				}
				return
			}
			if c == nil {
				t.Fatalf("CanvasBackground = nil, want %s", tc.want)
			}
			r, g, b, _ := c.RGBA()
			got := "#" + strings.ToLower(hex2(r)+hex2(g)+hex2(b))
			if got != tc.want {
				t.Fatalf("CanvasBackground = %s, want %s", got, tc.want)
			}
			// SVG 导出第一块底色必须与传播一致（浏览器预览同基准）
			svg := doc.DumpSVG()
			if !strings.Contains(svg, `<rect x="0" y="0" width="100" height="100" fill="`+tc.want+`"/>`) {
				head := svg
				if len(head) > 200 {
					head = head[:200]
				}
				t.Fatalf("DumpSVG canvas fill missing %s:\n%s", tc.want, head)
			}
		})
	}
}

func hex2(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&0x0f]})
}
