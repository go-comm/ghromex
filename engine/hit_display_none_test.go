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
	hit := engine.ElementAt(doc, 40, 40)
	if hit == box {
		t.Fatal("display:none 的元素仍被命中（旧盒子没清）")
	}
	if hit != under {
		t.Fatalf("命中 = %v, want 前一个 div", hit)
	}
}
