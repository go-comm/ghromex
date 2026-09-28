package engine_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// text-align：纯文本行按容器内容宽整体偏移（FakeGraphics 字宽估算确定）。
func TestTextAlignTextLine(t *testing.T) {
	// body margin 0；div 固定宽 200；默认字号 16px（Chrome 基线）；
	// "AB" 两个拉丁字符 = 2×(16*3/5)=18px
	for _, tc := range []struct {
		align string
		wantX int
	}{
		{"left", 0},
		{"center", (200 - 18) / 2},
		{"right", 200 - 18},
	} {
		src := fmt.Sprintf(
			`<html><body><div id="c" style="width:200px;text-align:%s">AB</div></body></html>`, tc.align)
		vp := engine.NewHeadlessViewport(400, 300)
		doc, err := engine.OpenDocument(vp, src)
		if err != nil {
			t.Fatal(err)
		}
		svg := doc.DumpSVG()
		want := fmt.Sprintf(`x="%d"`, tc.wantX)
		if !strings.Contains(svg, want) {
			t.Fatalf("align=%s: run x=%d not found in svg:\n%s", tc.align, tc.wantX, svg)
		}
	}
}

// text-align：含 inline-block 原子盒的行，原子盒及其子树（含内部文本）整体平移；
// 内部文本还叠加了 text-align 继承在原子盒内部的居中。
func TestTextAlignAtomicBox(t *testing.T) {
	src := `<html><body><div id="w" style="width:200px;text-align:center">` +
		`<span id="s" style="display:inline-block;width:40px;height:10px;background-color:#FF0000">Q</span>` +
		`</div></body></html>`
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, src)
	if err != nil {
		t.Fatal(err)
	}
	s := doc.QuerySelector("#s")
	if s == nil {
		t.Fatal("#s not found")
	}
	r := s.GetBoundingClientRect()
	// 原子盒：dx = (200-40)/2 = 80
	if got := r.Left().Pixel(); got != 80 {
		t.Fatalf("atomic box left = %d, want 80", got)
	}
	// 内部文本 "Q"(16*3/5=9px) 在 span 内再居中：80 + (40-9)/2 = 95
	if !strings.Contains(doc.DumpSVG(), `x="95"`) {
		t.Fatalf("inner run not shifted to 95:\n%s", doc.DumpSVG())
	}
}

// border-radius：扫描线圆角——外侧角像素不填充，边缘中点填充。
func TestBorderRadiusPixels(t *testing.T) {
	src := `<html><body><div style="width:100px;height:40px;` +
		`background-color:#FF0000;border-radius:8px"></div></body></html>`
	buf, _ := openBuffered(t, 120, 60, src)

	// 左上角在圆弧外（row1 inset=3）→ 透明
	if _, _, _, a := colorAt(t, buf, 1, 1); a != 0 {
		t.Fatalf("corner (1,1) alpha = %d, want 0（圆角未生效）", a)
	}
	// 左边中点（非角带行，inset=0）→ 红
	if r, g, b, a := colorAt(t, buf, 0, 20); a != 255 || r != 0xFF || g != 0 || b != 0 {
		t.Fatalf("mid-left (0,20) = %02X%02X%02X%02X, want FF0000FF", r, g, b, a)
	}
	// 中心 → 红
	if r, _, _, a := colorAt(t, buf, 50, 20); a != 255 || r != 0xFF {
		t.Fatalf("center (50,20) alpha = %d, want 红", a)
	}
}

// border-radius + border：边框环沿圆角走；r=0 时保持平面四边。
func TestBorderRadiusRing(t *testing.T) {
	src := `<html><body><div style="width:60px;height:40px;border:2px solid #00FF00;` +
		`background-color:#FF0000;border-radius:6px"></div></body></html>`
	buf, _ := openBuffered(t, 80, 60, src)

	// 角外（row0 inset=6）→ 透明
	if _, _, _, a := colorAt(t, buf, 0, 0); a != 0 {
		t.Fatalf("corner (0,0) alpha = %d, want 0", a)
	}
	// 顶边中点 → 绿（边框）
	if r, g, b, a := colorAt(t, buf, 30, 0); a != 255 || g != 0xFF || r != 0 {
		t.Fatalf("top edge (30,0) = %02X%02X%02X%02X, want 00FF00FF", r, g, b, a)
	}
	// 左边中点外圈 → 绿，内圈 → 红背景
	if _, g, _, _ := colorAt(t, buf, 0, 20); g != 0xFF {
		t.Fatalf("left ring (0,20) want green")
	}
	if r2, g2, _, _ := colorAt(t, buf, 6, 20); r2 == 0 || g2 != 0 {
		t.Fatalf("inner (6,20) = %02X%02X, want red background", r2, g2)
	}
}

// SVG 导出：带圆角时输出 rx，无圆角不输出。
func TestBorderRadiusSVG(t *testing.T) {
	vp := engine.NewHeadlessViewport(200, 100)
	doc, err := engine.OpenDocument(vp,
		`<html><body><div style="width:100px;height:40px;background-color:#FF0000;border-radius:8px"></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.DumpSVG(), `rx="8"`) {
		t.Fatalf("svg missing rx:\n%s", doc.DumpSVG())
	}
	doc2, _ := engine.OpenDocument(vp,
		`<html><body><div style="width:100px;height:40px;background-color:#FF0000"></div></body></html>`)
	if strings.Contains(doc2.DumpSVG(), `rx=`) {
		t.Fatalf("r=0 must not emit rx:\n%s", doc2.DumpSVG())
	}
}
