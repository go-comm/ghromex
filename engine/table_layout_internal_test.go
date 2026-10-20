package engine

import "testing"

// TestTableLayoutColumns 表格布局：行按行堆叠、同列单元格左缘对齐
// （引擎此前无表格模型，单元格按默认 inline-block 挤在一行）。
func TestTableLayoutColumns(t *testing.T) {
	src := `<!doctype html><html><body style="margin:0">
		<table id="t">
			<caption>标题</caption>
			<thead><tr><th id="h1">表头一</th><th id="h2">表头二</th></tr></thead>
			<tbody>
				<tr><td id="a1">A1</td><td id="a2">A2</td></tr>
				<tr><td id="b1">B1</td><td id="b2">B2</td></tr>
			</tbody>
		</table>
	</body></html>`
	buf := NewBufferGraphics(400, 200)
	vp := &HeadlessViewport{W: 400, H: 200, G: buf}
	doc, err := OpenDocument(vp, src)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	RenderNode(buf, doc)

	el := func(id string) *htmlElement {
		e := doc.QuerySelector(id)
		if e == nil {
			t.Fatalf("no %s", id)
		}
		return inner(e)
	}
	h1, a1, b1 := el("#h1"), el("#a1"), el("#b1")
	a2 := el("#a2")

	// 同列左缘对齐
	if h1.x != a1.x || a1.x != b1.x {
		t.Errorf("第一列左缘未对齐：表头=%d A1=%d B1=%d", h1.x, a1.x, b1.x)
	}
	// 第二列在第一列右侧
	if a2.x <= a1.x {
		t.Errorf("第二列未排在第一列右侧：A1=%d A2=%d", a1.x, a2.x)
	}
	// 行按行堆叠（第二行在第一行下方）
	if b1.y <= a1.y {
		t.Errorf("第二行未排在第一行下方：A1.y=%d B1.y=%d", a1.y, b1.y)
	}
	// caption 在表格首行之上
	if cap := tableCaption(el("#t")); cap != nil && cap.y >= h1.y {
		t.Errorf("caption 未排在首行之上：caption.y=%d 表头.y=%d", cap.y, h1.y)
	}
	// 表头加粗（Chrome UA: th 居中加粗）
	if fw := h1.computed.FontWeight(); fw != FontWeightBold {
		t.Errorf("th 字体权重 = %v, want bold", fw)
	}
}
