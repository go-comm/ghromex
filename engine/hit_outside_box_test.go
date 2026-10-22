package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 零尺寸父盒里的绝对定位子元素：渲染层照画，命中层也必须能点到。
//
// 命中测试原来只在「点落在子盒内」时下钻，父盒是零尺寸（高度为 0 的
// body 之类）就整棵子树都跳过了——于是「看得见却点不到」。
//
// 这类场景在 Chrome 里是能点到的：overflow 默认 visible，父盒不裁剪子
// 内容，绝对定位的子元素画在父盒之外也照样能点。
func TestAbsoluteChildOfZeroHeightParentIsHit(t *testing.T) {
	const src = `<!doctype html><html><body>
		<div id="card" style="position:absolute;left:20px;top:30px;width:100px;height:40px;background-color:#FF0000"></div>
	</body></html>`

	buf, doc, err := engine.OpenBufferedDocument(400, 300, src)
	if err != nil {
		t.Fatal(err)
	}
	// body 高度为 0（唯一的子元素是绝对定位，不占常规流）——父盒是零尺寸。
	if h := doc.Body().GetBoundingClientRect().Bottom().Pixel() -
		doc.Body().GetBoundingClientRect().Top().Pixel(); h != 0 {
		t.Fatalf("body 高 = %d, want 0（用例前提）", h)
	}

	// 渲染层：绝对定位的子元素画在 body 之外，照样画出来。
	r, g, b, _, ok := buf.ColorAt(40, 40)
	if !ok {
		t.Fatal("像素越界")
	}
	if r != 255 || g != 0 || b != 0 {
		t.Fatalf("像素 = %d,%d,%d, want 255,0,0（绝对定位子元素应被画出）", r, g, b)
	}

	// 命中层：同一个位置必须能点到它。
	card := doc.QuerySelector("#card")
	if hit := engine.ElementAt(doc, 40, 40); hit != card {
		t.Fatalf("命中 = %v, want #card（看得见却点不到）", hit)
	}
}

// 子元素露在父盒之外（负边距/绝对定位）时也要能点到：
// overflow 可见的父盒不裁剪子内容，渲染层照画，命中层不能跳过。
func TestChildOutsideParentBoxIsHit(t *testing.T) {
	const src = `<!doctype html><html><head><style>
		body { margin: 0; height: 300px; }
		#wrap { position: relative; left: 20px; top: 30px; width: 100px; height: 40px; }
		#badge { position: absolute; left: 130px; top: 20px; width: 30px; height: 30px; background-color:#00FF00; }
	</style></head><body>
		<div id="wrap"><div id="badge"></div></div>
	</body></html>`

	buf, doc, err := engine.OpenBufferedDocument(400, 300, src)
	if err != nil {
		t.Fatal(err)
	}
	// 徽标在父盒右侧之外（父盒 x=20..120，徽标 x=150..180）。
	badge := doc.QuerySelector("#badge")
	br := badge.GetBoundingClientRect()
	x := (br.Left().Pixel() + br.Right().Pixel()) / 2
	y := (br.Top().Pixel() + br.Bottom().Pixel()) / 2

	r, g, b, _, ok := buf.ColorAt(x, y)
	if !ok {
		t.Fatal("像素越界")
	}
	if r != 0 || g != 255 || b != 0 {
		t.Fatalf("像素 = %d,%d,%d, want 0,255,0（徽标应被画出）", r, g, b)
	}
	if hit := engine.ElementAt(doc, x, y); hit != badge {
		t.Fatalf("命中 = %v, want #badge（露在父盒外也要能点到）", hit)
	}
}
