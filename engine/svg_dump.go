package engine

import (
	"fmt"
	"strings"
)

// DumpSVG 将布局结果导出为 SVG 文本：背景/边框为 rect，文本片段为 text。
// 用于无显示器环境验证布局与绘制意图。
func (document *htmlDocument) DumpSVG() string {
	v := document.viewport
	w, h := 0, 0
	if v != nil {
		w, h = v.ViewportWidth(), v.ViewportHeight()
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", w, h, w, h)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" fill="#ffffff"/>`+"\n", w, h)
	dumpSVGNode(&b, document)
	b.WriteString("</svg>\n")
	return b.String()
}

func dumpSVGNode(b *strings.Builder, e HTMLElement) {
	if e == nil {
		return
	}
	base := inner(e)
	if base == nil {
		return
	}
	comp := base.computed
	if comp != nil && comp.Display() == DisplayNone {
		return
	}

	if tn, ok := e.(*textNode); ok {
		fs := 14
		fill := "#000000"
		fw := ""
		if base.computed != nil {
			if s := resolveLen(base.computed.FontSize(), 0); s > 0 {
				fs = s
			}
			if c := base.computed.Color(); c != nil {
				r, g, bl, _ := c.RGBA()
				fill = fmt.Sprintf("#%02x%02x%02x", r, g, bl)
			}
			if base.computed.FontWeight() == FontWeightBold {
				fw = ` font-weight="bold"`
			}
		}
		for _, run := range tn.node.runs {
			fmt.Fprintf(b, `<text x="%d" y="%d" font-size="%d"%s fill="%s">%s</text>`+"\n",
				run.x, run.y+run.h, fs, fw, fill, escapeXML(run.text))
		}
		return
	}

	if comp != nil {
		bd := resolveEdgeRect(comp.BorderStyleWidth(), 0, 0)
		pd := resolveEdgeRect(comp.Padding(), 0, 0)
		bx := base.x - pd[eLeft] - bd[eLeft]
		by := base.y - pd[eTop] - bd[eTop]
		bw := base.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
		bh := base.height + pd[eTop] + pd[eBottom] + bd[eTop] + bd[eBottom]
		br := resolveRadius(comp, bw, bh)
		rxAttr := ""
		if br > 0 {
			rxAttr = fmt.Sprintf(` rx="%d"`, br)
		}
		if bg := comp.BackgroundColor(); bg != nil && bw > 0 && bh > 0 {
			r, g, bl, al := bg.RGBA()
			if al > 0 {
				fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" fill-opacity="%.2f"%s/>`+"\n",
					bx, by, bw, bh, hexColor(r, g, bl), float64(al)/255, rxAttr)
			}
		}
		if bc := comp.BorderColor(); bc != nil && comp.BorderStyle() != BorderStyleNone && bw > 0 && bh > 0 {
			lw := bd[eLeft]
			if lw < bd[eTop] {
				lw = bd[eTop]
			}
			if lw > 0 {
				r, g, bl, _ := bc.RGBA()
				srx := ""
				if br > 0 { // 描边路径中心线内缩 lw/2，rx 相应修正
					srx = fmt.Sprintf(` rx="%d"`, maxZero(br-lw/2))
				}
				fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="%s" stroke-width="%d"%s/>`+"\n",
					bx+lw/2, by+lw/2, bw-lw, bh-lw, hexColor(r, g, bl), lw, srx)
			}
		}
	}

	for _, child := range base.children {
		dumpSVGNode(b, child)
	}
}

func hexColor(r, g, b uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
