package engine_test

import (
	"strings"
	"testing"
)

// fixed 元素建立层叠上下文（这里用 z:100 复现早期门户页的顶栏）。它内部的
// absolute 子元素必须在顶栏背景之后绘制；修复前 collectPositioned 把子元素
// 当成根上下文的 z:0 项，先于 z:100 的 fixed 顶栏绘制，结果被顶栏背景盖住。
func TestFixedStackingContextContainsPositionedChildren(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div style="width:150px;height:150px;background-color:#00FF00"></div>
		<div id="fixed" style="position:fixed;left:0;top:0;width:100px;height:100px;background-color:#FF0000;z-index:100">
			<div id="child" style="position:absolute;left:20px;top:20px;width:30px;height:30px;background-color:#0000FF"></div>
		</div>
	</body></html>`
	buf, doc := openBuffered(t, 150, 150, src)

	if r, g, b, _ := colorAt(t, buf, 25, 25); r != 0x00 || g != 0x00 || b != 0xFF {
		t.Errorf("fixed 内子元素 (25,25) = %02X%02X%02X, want 0000FF（子元素被顶栏背景盖住？）", r, g, b)
	}
	if r, g, b, _ := colorAt(t, buf, 5, 5); r != 0xFF || g != 0x00 || b != 0x00 {
		t.Errorf("fixed 背景 (5,5) = %02X%02X%02X, want FF0000", r, g, b)
	}
	if r, g, b, _ := colorAt(t, buf, 125, 5); r != 0x00 || g != 0xFF || b != 0x00 {
		t.Errorf("页面内容 (125,5) = %02X%02X%02X, want 00FF00（fixed 应压住页面）", r, g, b)
	}

	// SVG 导出与渲染共用同一层叠模型：子元素蓝色必须在顶栏红色之后。
	svg := doc.DumpSVG()
	red := strings.Index(svg, `fill="#ff0000"`)
	blue := strings.Index(svg, `fill="#0000ff"`)
	if red < 0 || blue < 0 {
		t.Fatalf("DumpSVG 缺少红/蓝矩形: red=%d blue=%d", red, blue)
	}
	if blue < red {
		t.Errorf("DumpSVG 蓝色子元素出现在红色顶栏之前（index red=%d blue=%d）", red, blue)
	}
}

// z-index 非 0 的定位元素是一个整体上下文：它的 z:auto 子元素不能跑到
// 外部去和祖先的兄弟比较。此处红色上下文 z:2 应连同蓝色子元素一起压过
// 绿色兄弟 z:1，即使绿色兄弟的 z 高于蓝色的 0。
func TestNestedStackingContextIsAtomic(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div id="ctx" style="position:absolute;left:0;top:0;width:120px;height:120px;background-color:#FF0000;z-index:2">
			<div style="position:absolute;left:60px;top:60px;width:50px;height:50px;background-color:#0000FF"></div>
		</div>
		<div id="sibling" style="position:absolute;left:40px;top:40px;width:80px;height:80px;background-color:#00FF00;z-index:1"></div>
	</body></html>`
	buf, _ := openBuffered(t, 150, 150, src)

	if r, g, b, _ := colorAt(t, buf, 70, 70); r != 0x00 || g != 0x00 || b != 0xFF {
		t.Errorf("上下文内部 (70,70) = %02X%02X%02X, want 0000FF（子元素被拆出上下文）", r, g, b)
	}
	if r, g, b, _ := colorAt(t, buf, 10, 10); r != 0xFF || g != 0x00 || b != 0x00 {
		t.Errorf("上下文背景 (10,10) = %02X%02X%02X, want FF0000", r, g, b)
	}
}
