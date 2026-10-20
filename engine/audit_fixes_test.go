package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// ---------------------------------------------------------------------------
// 审计修复的回归用例
// ---------------------------------------------------------------------------

// TestInvalidLengthDeclarationNoCrash 非法长度声明（单位不支持）不得崩：
// 解析层把 nil Size 写进边集后，命中测试里的 GetBoundingClientRect 会对
// nil 调 .Pixel() → 空指针崩溃（页面上任意一次点击即崩）。
func TestInvalidLengthDeclarationNoCrash(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#bad { display: block; width: 100px; height: 50px; padding: 2em; margin: 3em; border-width: 2em; }
	</style></head><body><div id="bad">x</div></body></html>`
	doc := openDoc(t, 200, 120, src)
	el := mustEl(t, doc, "#bad")
	// 命中测试路径：边框盒计算不得崩
	_ = el.GetBoundingClientRect()
	// 点击派发路径（ElementAt 会对每个候选元素调 GetBoundingClientRect）
	engine.OnDocumentClick(doc, 10, 10)
}

// TestInvalidLonghandKeepsUAValue 非法单位的长写声明应被**忽略**（保留 UA
// 值），而不是把该边清零——清零会抹掉 UA 给出的 padding，盒模型随之变形。
func TestInvalidLonghandKeepsUAValue(t *testing.T) {
	withBad := `<!doctype html><html><body><input id="i" style="padding-left:2em"></body></html>`
	plain := `<!doctype html><html><body><input id="i"></body></html>`
	w1 := inputBoxW(t, withBad)
	w2 := inputBoxW(t, plain)
	if w1 != w2 {
		t.Errorf("非法长写声明改变了盒宽: %d vs 无该声明 %d（应忽略该声明）", w1, w2)
	}
}

func inputBoxW(t *testing.T, src string) int {
	t.Helper()
	doc := openDoc(t, 300, 200, src)
	r := mustEl(t, doc, "#i").GetBoundingClientRect()
	return r.Right().Pixel() - r.Left().Pixel()
}

// TestTextareaFocusRingVisible 聚焦的 textarea 必须能看到聚焦框：textarea
// 默认 overflow:hidden，裁剪区是 padding 盒，而聚焦框画在边框盒上——
// 聚焦框若在裁剪栈内绘制会被整圈裁掉（此前 textarea 永远没有聚焦指示）。
func TestTextareaFocusRingVisible(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#t { display: block; width: 100px; height: 60px; }
	</style></head><body>
	<textarea id="t">hello</textarea>
	</body></html>`
	buf, doc := openBuffered(t, 200, 120, src)
	const fr, fg, fb = 0x00, 0x78, 0xD4 // focusBlue（Windows 系统强调色默认）
	hasFocusBlue := func() bool {
		for y := 0; y < 120; y++ {
			for x := 0; x < 200; x++ {
				r, g, b, _ := colorAt(t, buf, x, y)
				if r == fr && g == fg && b == fb {
					return true
				}
			}
		}
		return false
	}
	if hasFocusBlue() {
		t.Fatal("未聚焦时不应出现聚焦框")
	}
	ta := mustEl(t, doc, "#t")
	engine.SetFocusedElement(doc, ta)
	engine.RenderNode(buf, doc)
	if !hasFocusBlue() {
		t.Fatal("聚焦 textarea 未见聚焦框（被自身 overflow:hidden 的裁剪盒裁掉）")
	}
}
