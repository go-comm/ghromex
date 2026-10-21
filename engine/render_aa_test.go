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
