package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 填充圆角 AA：白底上弧边像素呈分数覆盖的半透明红，
// 弧外保持纯白、全覆盖区保持纯红（手算 SDF 覆盖率验证）。
func TestFillRadiusAntialiasing(t *testing.T) {
	// 盒 (0,0,100,40) r=8；像素中心 (x+0.5,y+0.5) 代入 rounded box SDF：
	//   (2,1): d≈+0.50 ≥0.5 → cov=0
	//   (3,1): d≈-0.08     → cov≈0.58（分数覆盖）
	//   (4,1): d≈-0.61     → cov=1
	//   (0,20): d=-0.5 恰在边界 → cov=1
	src := `<!doctype html><html><body style="background-color:#FFFFFF;margin:0">` +
		`<div style="width:100px;height:40px;background-color:#FF0000;border-radius:8px"></div>` +
		`</body></html>`
	buf, _ := openBuffered(t, 120, 60, src)

	if r, g, b, a := colorAt(t, buf, 2, 1); r != 255 || g != 255 || b != 255 || a != 255 {
		t.Fatalf("(2,1) = %02X%02X%02X%02X, want FFFFFFFF（弧外不应绘制）", r, g, b, a)
	}
	r, g, b, a := colorAt(t, buf, 3, 1)
	if a != 255 {
		t.Fatalf("(3,1) alpha = %d, want 255", a)
	}
	if r != 255 || g != b {
		t.Fatalf("(3,1) = %02X%02X%02X, want 红主导且 G=B", r, g, b)
	}
	if g <= 60 || g >= 240 {
		t.Fatalf("(3,1) gray = %d, want 介于纯白(255)与纯红(0)之间的分数覆盖", g)
	}
	if r, g, b, _ := colorAt(t, buf, 4, 1); r != 255 || g != 0 || b != 0 {
		t.Fatalf("(4,1) = %02X%02X%02X, want FF0000", r, g, b)
	}
	if r, g, b, _ := colorAt(t, buf, 0, 20); r != 255 || g != 0 || b != 0 {
		t.Fatalf("(0,20) = %02X%02X%02X, want FF0000", r, g, b)
	}
}

// 基线补偿（GHROMEX_BASELINE_ADJ / SetBaselineAdjust）：与 Chrome 对照的全局
// 微调——只挪基线，**不排行盒高度**，故行距与盒尺寸都不变。
func TestBaselineAdjustMovesTextOnly(t *testing.T) {
	const src = `<!doctype html><html><body style="margin:0">
	<p id="p" style="margin:0;font-size:16px">第一行<br>第二行</p>
	</body></html>`
	doc := openDoc(t, 200, 120, src)

	ys := func() []int {
		engine.LayoutDocument(doc) // 基线是布局期算的，重排后再生效
		rec := &posRecorder{Graphics: doc.Viewport().Graphics()}
		engine.RenderNode(rec, doc)
		return rec.ys
	}
	before := ys()
	if len(before) < 2 {
		t.Fatalf("只记到 %d 段文字", len(before))
	}
	engine.SetBaselineAdjust(2)
	defer engine.SetBaselineAdjust(0)
	after := ys()
	if len(after) != len(before) {
		t.Fatalf("补偿前后文字段数不同：%d → %d", len(before), len(after))
	}
	for i := range before {
		if after[i]-before[i] != 2 {
			t.Fatalf("第 %d 段文字基线移动 %dpx，want 2", i, after[i]-before[i])
		}
	}
	// 行距不变：两段之间的间距应保持不变。
	if d0, d1 := before[1]-before[0], after[1]-after[0]; d0 != d1 {
		t.Fatalf("行距被改了：%d → %d（补偿只应挪基线）", d0, d1)
	}
}

// posRecorder 记录 DrawText 的 y 坐标。
type posRecorder struct {
	engine.Graphics
	ys []int
}

func (p *posRecorder) DrawText(x, y, w, h int, paint engine.Paint, text string) {
	p.ys = append(p.ys, y)
	p.Graphics.DrawText(x, y, w, h, paint, text)
}

// <button> 的内容垂直居中（HTML 规范的 button 布局，Chrome/Edge 同款）：
// 定高按钮的文字不能贴在内容盒顶部（否则整排按钮的文字都偏上）。
func TestButtonContentVerticallyCentered(t *testing.T) {
	const src = `<!doctype html><html><body style="margin:0;background-color:#FFFFFF">
	<button id="b" style="display:inline-block;width:120px;height:40px;padding:0;
		border:0;background-color:#000000;color:#FFFFFF;font-size:14px">居中</button>
	</body></html>`
	doc := openDoc(t, 200, 80, src)
	buf := engine.NewBufferGraphics(200, 80)
	engine.RenderNode(buf, doc)

	// 按钮盒：x=0,y=0,120×40（body margin 0）。白字在黑底上，按白像素
	// 找文字的上下留白：上下应大致相等（居中），而不是上=0（贴顶）。
	first, last := -1, -1
	for y := 0; y < 40; y++ {
		for x := 0; x < 120; x++ {
			r, g, b, _ := colorAt(t, buf, x, y)
			if r > 200 && g > 200 && b > 200 {
				if first < 0 {
					first = y
				}
				last = y
				break
			}
		}
	}
	if first < 0 {
		t.Fatal("按钮内没找到白字")
	}
	top, bottom := first, 40-1-last
	if top < 4 {
		t.Errorf("文字上留白 = %dpx（贴顶了：button 内容未垂直居中）", top)
	}
	if diff := top - bottom; diff > 2 || diff < -2 {
		t.Errorf("上留白 %dpx / 下留白 %dpx 相差过大（未居中）", top, bottom)
	}
}

// 圆角裁剪盒的裁剪要按 border-radius 的形状（浏览器语义）：子元素溢出到
// 角外时，角上应透出盒背后的底色，而不是像矩形裁剪那样被溢出内容盖成
// 方角（portal 页的大黑卡右上角就是这样）。
func TestRoundedClipCornerMask(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body { margin: 0; background-color: #F1F5F9; }
	#box { position: relative; left: 20px; top: 20px; width: 100px; height: 60px;
		background-color: #0B1220; border-radius: 12px; overflow: hidden; }
	#orb { position: absolute; right: -30px; top: -30px; width: 60px; height: 60px;
		background-color: #123A6B; border-radius: 30px; }
	</style></head><body><div id="box"><div id="orb"></div></div></body></html>`
	doc := openDoc(t, 200, 120, src)
	buf := engine.NewBufferGraphics(200, 120)
	engine.RenderNode(buf, doc)

	// 盒的右上角（盒 = 20,20 起 100×60，r=12）：角上的最外像素应透出
	// 页面底色 #F1F5F9，而不是溢出到角外的蓝球 #123A6B。
	if r, g, b, _ := colorAt(t, buf, 119, 20); r != 0xF1 || g != 0xF5 || b != 0xF9 {
		t.Errorf("右上角 (119,20) = %02X%02X%02X，want F1F5F9（角外应透出底色）", r, g, b)
	}
	// 盒内（圆角形状以内）仍是蓝球：补盖只动角外那几块。
	if r, g, b, _ := colorAt(t, buf, 100, 40); r != 0x12 || g != 0x3A || b != 0x6B {
		t.Errorf("盒内 (100,40) = %02X%02X%02X，want 123A6B（补盖不应影响盒内）", r, g, b)
	}
	// 盒内左下的黑底仍在（盒自身背景不被补盖吃掉）。
	if r, g, b, _ := colorAt(t, buf, 30, 70); r != 0x0B || g != 0x12 || b != 0x20 {
		t.Errorf("盒内左下 (30,70) = %02X%02X%02X，want 0B1220", r, g, b)
	}
}

// 边框圆角环 AA：环外纯白、环中全覆盖纯蓝、角弧外圈像素按覆盖率掺白。
func TestStrokeRadiusAntialiasing(t *testing.T) {
	// 边框盒 (0,0,64,44) r=6，内盒 (2,2,60,40) ir=4：
	//   (0,0)  外盒 d≈1.78 → cov=0
	//   (30,0) 环全覆盖   → 纯蓝
	//   (1,1)  外盒 d≈0.36 → cov≈0.14（分数环覆盖，蓝淡掺白）
	src := `<html><body style="background-color:#FFFFFF;margin:0">` +
		`<div style="width:60px;height:40px;border:2px solid #0000FF;border-radius:6px"></div>` +
		`</body></html>`
	buf, _ := openBuffered(t, 80, 60, src)

	if r, g, b, _ := colorAt(t, buf, 0, 0); r != 255 || g != 255 || b != 255 {
		t.Fatalf("(0,0) = %02X%02X%02X, want FFFFFF（环外）", r, g, b)
	}
	if r, g, b, _ := colorAt(t, buf, 30, 0); r != 0 || g != 0 || b != 255 {
		t.Fatalf("(30,0) = %02X%02X%02X, want 0000FF（环全覆盖）", r, g, b)
	}
	r, g, b, _ := colorAt(t, buf, 1, 1)
	if b != 255 || r != g || r >= 250 || r <= 150 {
		t.Fatalf("(1,1) = %02X%02X%02X, want 淡蓝（分数覆盖掺白）", r, g, b)
	}
}
