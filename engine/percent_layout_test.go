package engine_test

import (
	"testing"
)

// ---------------------------------------------------------------------------
// 百分比解析基准回归（CSS 2.1）
//
//  1. margin/padding 的百分比一律相对包含块**宽度**（§8.3.2，纵向边也如此）；
//  2. height 的百分比相对包含块**高度**（§10.5），不是"剩余可用高度"；
//  3. 布局算出的盒与画出来的盒必须是同一个盒（百分比 padding 的背景/边框
//     不得画在内容盒上，命中框也须与绘制框一致）。
// ---------------------------------------------------------------------------

// TestPercentHeightAgainstContainingBlock 定高父盒内的 height:% 子元素按
// 包含块高度解析：两个 50% 各得 200px（此前按"剩余可用高"算成 300/150）。
func TestPercentHeightAgainstContainingBlock(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#p { width: 400px; height: 400px; }
	</style></head><body>
	<div id="p"><div id="a" style="height:50%"></div><div id="b" style="height:50%"></div></div>
	</body></html>`
	doc := openDoc(t, 500, 500, src)
	for _, sel := range []string{"#a", "#b"} {
		r := mustEl(t, doc, sel).GetBoundingClientRect()
		if h := r.Bottom().Pixel() - r.Top().Pixel(); h != 200 {
			t.Errorf("%s 高 = %d, want 200（height:50%% 相对包含块高度 400）", sel, h)
		}
	}
	// 第二个子盒紧接第一个：位置也须正确
	pa := mustEl(t, doc, "#a").GetBoundingClientRect()
	pb := mustEl(t, doc, "#b").GetBoundingClientRect()
	if pb.Top().Pixel()-pa.Top().Pixel() != 200 {
		t.Errorf("#b 顶部 = %d, #a 顶部 = %d（相差应 200）", pb.Top().Pixel(), pa.Top().Pixel())
	}
}

// TestPercentHeightIndefiniteContainer 包含块高度不确定（auto）时，百分比
// 高度按 auto 处理（CSS 2.1 §10.5），不得按 0 或剩余空间硬算。
func TestPercentHeightIndefiniteContainer(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#p { width: 400px; }
	#a { height: 50%; }
	</style></head><body>
	<div id="p"><div id="a"><div id="inner">x</div></div></div>
	</body></html>`
	doc := openDoc(t, 500, 500, src)
	r := mustEl(t, doc, "#a").GetBoundingClientRect()
	if h := r.Bottom().Pixel() - r.Top().Pixel(); h <= 0 {
		t.Errorf("auto 高度父盒内的 50%% 高度子盒高度 = %d，应按内容撑开（>0）", h)
	}
}

// TestPercentHeightForAbsolute 绝对定位元素的 height:% 按包含块高度解析，
// 与 top 无关（此前 top 不同会得到不同高度）。
func TestPercentHeightForAbsolute(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#cb { position: relative; width: 400px; height: 400px; }
	#a1 { position: absolute; top: 0; height: 50%; }
	#a2 { position: absolute; top: 100px; height: 50%; }
	</style></head><body>
	<div id="cb"><div id="a1"></div><div id="a2"></div></div>
	</body></html>`
	doc := openDoc(t, 500, 500, src)
	for _, sel := range []string{"#a1", "#a2"} {
		r := mustEl(t, doc, sel).GetBoundingClientRect()
		if h := r.Bottom().Pixel() - r.Top().Pixel(); h != 200 {
			t.Errorf("%s 高 = %d, want 200（50%% 相对包含块高度 400）", sel, h)
		}
	}
}

// TestPercentPaddingUsesWidth 纵向 padding 的百分比也按包含块宽度解析
// （CSS 2.1 §8.3.2）：400px 宽容器内的 padding-top:10% = 40px。
func TestPercentPaddingUsesWidth(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#p { width: 400px; }
	#q { padding-top: 10%; }
	</style></head><body>
	<div id="p"><div id="q"><div id="c">x</div></div></div>
	</body></html>`
	doc := openDoc(t, 500, 500, src)
	p := mustEl(t, doc, "#p").GetBoundingClientRect()
	c := mustEl(t, doc, "#c").GetBoundingClientRect()
	if d := c.Top().Pixel() - p.Top().Pixel(); d != 40 {
		t.Errorf("padding-top 解析 = %d, want 40（10%% × 包含块宽 400）", d)
	}
}

// TestPercentPaddingBackgroundMatchesLayout 百分比 padding 的背景必须画在
// 边框盒上（与布局算出的盒一致），命中框也须同宽——此前绘制期按 0 基数
// 解析 padding，背景只覆盖内容盒。
func TestPercentPaddingBackgroundMatchesLayout(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#p { width: 400px; }
	#q { padding: 5%; background-color: #FF0000; }
	</style></head><body>
	<div id="p"><div id="q">x</div></div>
	</body></html>`
	buf, doc := openBuffered(t, 600, 200, src)
	r := mustEl(t, doc, "#q").GetBoundingClientRect()
	wantW := r.Right().Pixel() - r.Left().Pixel()
	// width:auto 的 content-box 盒：边框盒宽 = 包含块宽 400（padding 在内容盒
	// 之内），内容盒 = 400 - 2×20 = 360。
	if wantW != 400 {
		t.Fatalf("边框盒宽 = %d, want 400（auto 宽填满包含块）", wantW)
	}
	// 背景必须铺满**边框盒**（400px）——此前绘制期按 0 基数解析 padding，
	// 背景只覆盖 360px 的内容盒。
	red := 0
	for x := 0; x < 600; x++ {
		rr, _, _, a := colorAt(t, buf, x, 2)
		if a == 255 && rr == 0xFF {
			red++
		}
	}
	if red != wantW {
		t.Errorf("背景绘制宽 = %d, want %d（百分比 padding 的背景须覆盖边框盒）", red, wantW)
	}
}

// TestRelativeLeftPercent 相对定位的 left:% 按包含块宽度解析（Chrome 口径）。
func TestRelativeLeftPercent(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#p { width: 400px; }
	#r { position: relative; left: 10%; }
	</style></head><body>
	<div id="p"><div id="r">x</div></div>
	</body></html>`
	doc := openDoc(t, 500, 200, src)
	p := mustEl(t, doc, "#p").GetBoundingClientRect()
	r := mustEl(t, doc, "#r").GetBoundingClientRect()
	if d := r.Left().Pixel() - p.Left().Pixel(); d != 40 {
		t.Errorf("relative left:10%% 位移 = %d, want 40（10%% × 包含块宽 400）", d)
	}
}

// TestAbsoluteRightAnchorPercentMargin 右锚回推时 margin 的百分比基数须与
// 布局期一致（此前按 0 解析，盒子右缘错位）。
func TestAbsoluteRightAnchorPercentMargin(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#cb { position: relative; width: 400px; height: 200px; }
	#ab { position: absolute; right: 0; width: 50px; margin-left: 10%; }
	</style></head><body>
	<div id="cb"><div id="ab"></div></div>
	</body></html>`
	doc := openDoc(t, 500, 300, src)
	cb := mustEl(t, doc, "#cb").GetBoundingClientRect()
	ab := mustEl(t, doc, "#ab").GetBoundingClientRect()
	// 右锚定位的是**外边距盒**右缘：margin 盒宽 = margin-left 40 + 边框盒 50
	// → 边框盒左缘 = 400 - 90 + 40 = 350。
	if d := ab.Left().Pixel() - cb.Left().Pixel(); d != 350 {
		t.Errorf("绝对定位盒左缘 = %d, want 350（右锚按外边距盒右缘对齐）", d)
	}
}
