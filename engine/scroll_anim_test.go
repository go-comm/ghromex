package engine

import (
	"testing"
)

// 连续滚动的运动必须连续：动画途中再来一档滚轮时，速度不能掉到 0 再跳回
// 高位。
//
// 旧模型（每档都从静止重新起步的固定曲线）实测每档的位移是
// 35/26/17/12/6/3/1/0px 的锯齿——滚起来一顿一顿的。弹簧模型只换目标、
// 保留速度，位移应当是一条连续下滑的曲线。
func TestScrollRetargetKeepsMotionContinuous(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	// 每 3 帧（≈50ms）来一档滚轮：真实快速滚动的节奏。
	var steps []int
	prev := 0
	for notch := 0; notch < 6; notch++ {
		OnDocumentWheel(doc, 100, 100, 0, 100)
		for f := 0; f < 3; f++ {
			TickScroll(doc)
			cur := root.scrollAnimTop
			steps = append(steps, cur-prev)
			prev = cur
		}
	}
	if len(steps) == 0 {
		t.Fatal("没有采到任何帧的位移")
	}
	// 相邻帧位移之比不能突变：旧模型在每档末尾是 1px、下一档开头是 35px
	// （比值 35），这里要求比值有界。
	for i := 1; i < len(steps); i++ {
		lo, hi := steps[i-1], steps[i]
		if lo <= 0 || hi <= 0 {
			continue
		}
		ratio := float64(hi) / float64(lo)
		if ratio > 4 {
			t.Errorf("第 %d 帧位移从 %d 跳到 %d（比值 %.1f）：滚动动画在档与档之间"+
				"速度跳变（应保留速度连续滑动）", i, lo, hi, ratio)
		}
	}
}

// 动画途中再来一档滚轮：速度必须保留（这是连续滑动的关键）。
func TestScrollRetargetKeepsVelocity(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	OnDocumentWheel(doc, 100, 100, 0, 100)
	TickScroll(doc)
	TickScroll(doc)
	vel := root.scrollAnimVelTop
	if vel == 0 {
		t.Fatal("动画推进两帧后速度仍为 0")
	}
	// 途中再来一档：目标变了，速度不能被清零。
	OnDocumentWheel(doc, 100, 100, 0, 100)
	if root.scrollAnimVelTop != vel {
		t.Errorf("重定向前后速度 = %v → %v（应保留）", vel, root.scrollAnimVelTop)
	}
}
