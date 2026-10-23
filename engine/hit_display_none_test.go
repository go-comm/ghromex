package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// display:none 的元素不能被命中。
//
// 元素从可见变成 display:none 后若保留旧盒子，命中测试仍会命中它：明明
// 看不见，点上去却还能触发它的动作。真实症状：记事本的菜单项执行后菜单
// 收起，在原位置再点一次仍会命中那个已隐藏的菜单项，「时间/日期」就连插
// 两次日期。
//
// 命中测试按盒子判定（零尺寸盒不参与命中），所以隐藏时必须清盒——渲染层
// 按 display:none 跳过，命中层靠零尺寸盒跳过，两边同口径。
func TestDisplayNoneElementNotHit(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, `<!doctype html><html><head><style>
		body { margin: 0; height: 300px; }
	</style></head><body>
		<div id="under" style="position:absolute;left:20px;top:30px;width:100px;height:40px"></div>
		<div id="box" style="position:absolute;left:20px;top:30px;width:100px;height:40px"></div>
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	box := doc.QuerySelector("#box")
	under := doc.QuerySelector("#under")
	if box == nil || under == nil {
		t.Fatal("元素缺失")
	}
	doc.Refresh()

	// 可见时：命中后一个（DOM 顺序后者胜出）。
	if hit := engine.ElementAt(doc, 40, 40); hit != box {
		t.Fatalf("命中 = %v, want 后一个 div", hit)
	}

	// 隐藏后一个：命中应落到前一个，而不是被隐藏的那个。
	box.SetAttribute("style", "display:none")
	doc.Refresh()
	// 隐藏的元素连盒子也没了：应用层查矩形不该拿到旧盒子。
	if r := box.GetBoundingClientRect(); r.Right().Pixel()-r.Left().Pixel() != 0 ||
		r.Bottom().Pixel()-r.Top().Pixel() != 0 {
		t.Fatalf("隐藏后的盒 = %dx%d, want 0x0",
			r.Right().Pixel()-r.Left().Pixel(), r.Bottom().Pixel()-r.Top().Pixel())
	}
	hit := engine.ElementAt(doc, 40, 40)
	if hit == box {
		t.Fatal("display:none 的元素仍被命中（旧盒子没清）")
	}
	if hit != under {
		t.Fatalf("命中 = %v, want 前一个 div", hit)
	}
}

// display:none 的元素矩形要彻底空掉。
//
// GetBoundingClientRect 返回的是边框盒（内容盒 + padding + border），布局期
// 解析出的边长另有缓存；只清内容盒而留着边长，隐藏后的元素仍会报出一个
// padding 撑出来的空壳尺寸（记事本的查找栏就是这样：明明 display:none，
// 矩形还有 16x15），应用层按矩形判断「有没有占地方」就读到假数据。
func TestDisplayNonePaddedBoxReportsEmptyRect(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, `<!doctype html><html><head><style>
		body { margin: 0; height: 300px; }
	</style></head><body>
		<div id="pad" style="position:absolute;left:20px;top:30px;width:0px;height:1px;
			padding:7px 8px;border:1px solid #333333"></div>
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	pad := doc.QuerySelector("#pad")
	if pad == nil {
		t.Fatal("元素缺失")
	}
	doc.Refresh()

	// 可见时：边框盒把 padding 与 border 都算进来。
	r := pad.GetBoundingClientRect()
	if w, h := r.Right().Pixel()-r.Left().Pixel(), r.Bottom().Pixel()-r.Top().Pixel(); w != 18 || h != 17 {
		t.Fatalf("可见时边框盒 = %dx%d, want 18x17（内容 0x1 + padding 8+8/7+7 + border 1x4）", w, h)
	}

	pad.SetAttribute("style", "position:absolute;left:20px;top:30px;display:none")
	doc.Refresh()
	r = pad.GetBoundingClientRect()
	if w, h := r.Right().Pixel()-r.Left().Pixel(), r.Bottom().Pixel()-r.Top().Pixel(); w != 0 || h != 0 {
		t.Fatalf("隐藏后边框盒 = %dx%d, want 0x0", w, h)
	}
}
