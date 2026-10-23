package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// pointer-events:none 的盒子自己不作为命中目标：为排版/装饰存在的元素
// （占位块、分隔线、覆盖层）不该吃掉压在它下面那份内容的点击——否则就是
// 「看得见点不到」。
func TestPointerEventsNoneFallsThrough(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, `<!doctype html><html><head><style>
		body { margin: 0; height: 300px; }
		.pad { pointer-events: none; }
	</style></head><body>
		<div id="under" style="position:absolute;left:20px;top:30px;width:100px;height:40px"></div>
		<div id="over" class="pad" style="position:absolute;left:0;top:0;width:400px;height:300px"></div>
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	under := doc.QuerySelector("#under")
	over := doc.QuerySelector("#over")
	if under == nil || over == nil {
		t.Fatal("元素缺失")
	}
	doc.Refresh()

	// 覆盖层按 DOM 序在后，声明 pointer-events:none 后命中要落到下面那层。
	if hit := engine.ElementAt(doc, 40, 40); hit != under {
		t.Fatalf("命中 = %v, want #under（覆盖层不该吃掉点击）", hit)
	}
	// 点击派发也必须落在真正的目标上：处理器收不到点击就是白改。
	clicks := 0
	under.OnClick(func(*engine.MouseEvent) { clicks++ })
	engine.OnDocumentClick(doc, 40, 40)
	if clicks != 1 {
		t.Fatalf("点击下方元素触发 %d 次点击，want 1", clicks)
	}
}

// pointer-events 沿父链继承：父级 none 时子内容默认也不可点，子级自己
// 声明 auto 能恢复（遮罩里留一个可点的按钮这类写法要靠这条）。
func TestPointerEventsInheritedAndReclaimed(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, `<!doctype html><html><head><style>
		body { margin: 0; height: 300px; }
		#veil { pointer-events: none; position: absolute; left: 0; top: 0;
			width: 400px; height: 200px; }
		#veil div { position: absolute; width: 60px; height: 30px; }
		#kid { left: 50px; top: 50px; pointer-events: auto; }
		#ghost { left: 200px; top: 50px; }
	</style></head><body>
		<div id="veil">
			<div id="kid"></div>
			<div id="ghost"></div>
		</div>
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	veil := doc.QuerySelector("#veil")
	kid := doc.QuerySelector("#kid")
	ghost := doc.QuerySelector("#ghost")
	if veil == nil || kid == nil || ghost == nil {
		t.Fatal("元素缺失")
	}
	doc.Refresh()

	if hit := engine.ElementAt(doc, 210, 60); hit == ghost || hit == veil {
		t.Fatalf("命中 = %v，want 继承来的 none 也该不可点", hit)
	}
	if hit := engine.ElementAt(doc, 60, 60); hit != kid {
		t.Fatalf("命中 = %v, want #kid（自己声明 pointer-events:auto 的子级）", hit)
	}
}
