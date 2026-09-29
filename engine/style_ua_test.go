package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// UA 表单控件默认样式回归保护（对齐 Chrome/Windows 观感）。
// 曾因在 uaCSS 字符串里写 `//` 注释（CSS 只支持 /* */），注释被并入
// 下一条规则的 selector 导致 input/a 规则静默丢失——用几何断言兜底。
func TestUAFormControlsBrowserLike(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 200)
	doc, err := engine.OpenDocument(vp, `<html><body><input id="i"><button id="b">OK</button></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	// input：宽 = 171 内容 + 2×2 padding + 2×1 border = 177；高 = 17 + 2 + 2 = 21
	//（Chrome/Windows 实测 size=20 默认 177x21）
	ir := doc.QuerySelector("#i").GetBoundingClientRect()
	if w := ir.Right().Pixel() - ir.Left().Pixel(); w != 177 {
		t.Fatalf("input 边框盒宽 = %d, want 177（input 的 UA 规则丢失？）", w)
	}
	if h := ir.Bottom().Pixel() - ir.Top().Pixel(); h != 21 {
		t.Fatalf("input 边框盒高 = %d, want 21", h)
	}

	// button：shrink-to-fit 包住 13px 文本，垂直 padding 3px → 外盒高 24
	//（Chrome 实测 25，1px 差异属字体行高度量）
	br := doc.QuerySelector("#b").GetBoundingClientRect()
	if h := br.Bottom().Pixel() - br.Top().Pixel(); h != 24 {
		t.Fatalf("button 边框盒高 = %d, want 24（button 的 UA 规则丢失？）", h)
	}
	if w := br.Right().Pixel() - br.Left().Pixel(); w < 20 || w > 40 {
		t.Fatalf("button 宽 = %d, want 20~40（文本+padding 1px 6px+border）", w)
	}
}
