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
	// 画布底色同渲染路径：CSS canvas propagation（html/body 背景），无则白。
	fill := "#ffffff"
	if bg := CanvasBackground(document); bg != nil {
		r, g, bl, _ := bg.RGBA()
		fill = fmt.Sprintf("#%02x%02x%02x", r, g, bl)
	}
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" fill="%s"/>`+"\n", w, h, fill)
	dumpSVGNode(&b, document)
	// 定位层：与 RenderNode 同一层叠模型（主 pass 已跳过 positioned 子树）。
	for _, pb := range collectPositioned(document) {
		dumpSVGNode(&b, pb.e)
	}
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
		fs := 16
		fill := "#000000"
		fw := ""
		fam := ""
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
			fam = base.computed.FontFamily()
		}
		ff := fmt.Sprintf(` font-family="%s"`, escapeXML(svgFontStack(fam)))
		for _, run := range tn.node.runs {
			fmt.Fprintf(b, `<text x="%d" y="%d" font-size="%d"%s%s fill="%s">%s</text>`+"\n",
				run.x, run.y+run.h, fs, ff, fw, fill, escapeXML(run.text))
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
		if isPositioned(child) {
			continue // 由 DumpSVG 的定位层统一追加
		}
		dumpSVGNode(b, child)
	}
}

func hexColor(r, g, b uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// svgFontStack 为导出的 SVG 文本补全字体栈，让浏览器预览与引擎实际落字一致：
// 未指定 family 时按引擎 standard（sans-serif → Arial）；
// 末尾始终附加 CJK per-script fallback（微软雅黑）。浏览器按"字形覆盖"逐字回退，
// 拉丁命中前者、汉字落到后者，与引擎的分段落字等价。缺省不附加时浏览器会用自己的
// 默认中文字体（常为宋体），预览观感就与引擎不符了。仅调试导出用途。
func svgFontStack(cssFamily string) string {
	cssFamily = strings.TrimSpace(cssFamily)
	if cssFamily == "" {
		cssFamily = "Arial" // 引擎 standard = sans-serif（Arial）
	}
	return cssFamily + ", 'Microsoft YaHei'"
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
