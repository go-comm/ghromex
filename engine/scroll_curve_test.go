package engine

import (
	"testing"
	"time"
)

// 滚动动画的曲线形状（手感）要钉住：起步太慢、太急，收尾拖太长，都会让
// 滚动「不像 Chrome」。
//
// 模型是**基于时间的缓动**：进度 t = 已推进时间 / 150ms，位置 =
// 起点 + (目标 − 起点) × easeOutQuad(t)。这里的断言取的是曲线上的几个
// 特征点（而不是逐帧精确值），既挡住「曲线形状变了」，又不至于因为
// 像素取整而抖动。
func TestScrollAnimationCurveShape(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	OnDocumentWheel(doc, 100, 100, 0, 100)
	const target = 100

	// 按 1ms 步进，记录每毫秒的**连续位置**（缓动真值，不含像素取整）。
	pos := map[int]float64{}
	for ms := 1; ms <= 300; ms++ {
		feedScrollClock(root, time.Millisecond)
		TickScroll(doc)
		pos[ms] = root.scrollAnimPosTop
	}

	// 起步：第 16ms（60Hz 一帧）走完约 20%（easeOutQuad(16/150)=20%）。
	// 太慢会「粘」在原地，太快则一步窜出去。
	if p := pos[16]; p < 17 || p > 23 {
		t.Errorf("第 16ms 位置 = %.1fpx（want 17~23）：起步太慢或太急", p)
	}
	// 中段：75ms 过半（easeOutQuad(0.5)=75%）。
	if p := pos[75]; p < 70 || p > 80 {
		t.Errorf("第 75ms 位置 = %.1fpx（want 70~80）", p)
	}
	// 收尾：100ms 已到 89%（Chrome 同款是「很快出去、慢慢收」）。
	if p := pos[100]; p < 85 || p > 93 {
		t.Errorf("第 100ms 位置 = %.1fpx（want 85~93）", p)
	}
	// 时长：150ms 整（150ms / 9 帧 @60Hz，与 Chrome 同量级）。
	if p := pos[150]; p < target-1 || p > target {
		t.Errorf("第 150ms 位置 = %.1fpx（want %d）：时长不是 150ms", p, target)
	}
	// 过 150ms 之后必须停在目标上（不能还在爬）。
	if p := pos[300]; p != target {
		t.Errorf("第 300ms 位置 = %.1fpx，want %d：动画没有停在目标上", p, target)
	}
	// 别过头：缓动不该过冲。
	for ms := 1; ms <= 300; ms++ {
		if pos[ms] > target {
			t.Errorf("第 %dms 位置 = %.1fpx，超过目标 %d（过冲）", ms, pos[ms], target)
			break
		}
	}
}

// 一档滚轮（100px）在 60Hz 下 150ms 走完（≈9 帧）：时长用**墙钟时间**算，
// 与帧率无关——高刷屏上不会滚得更快、掉帧时也不会变速。
//
// （对照：固定帧数每帧加 11.11px 的做法，60Hz 下是 9 帧，但 240Hz 下同样
// 9 帧只花 37ms，滚得太快；掉帧时更会变速。）
func TestScrollDuration150msAt60Hz(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	OnDocumentWheel(doc, 100, 100, 0, 100)
	frames := 0
	for f := 0; f < 60; f++ {
		feedScrollClock(root, 16666667*time.Nanosecond) // 60Hz 一帧
		TickScroll(doc)
		frames++
		if root.scrollAnimTop >= 100 {
			break
		}
	}
	// 150ms / 16.67ms ≈ 9 帧（可见位置因取整可能早一两帧到位）。
	if frames < 7 || frames > 11 {
		t.Errorf("一档 100px 用了 %d 帧（want 7~11，约 150ms）", frames)
	}
	if root.scrollAnimTop != 100 {
		t.Errorf("动画结束位置 = %d, want 100", root.scrollAnimTop)
	}
}

// 可见位移（整像素）必须**单调不增**：缓动曲线的速度单调下降，取整后
// 仍是单调的。此前把「累积位置」取整，速度一变快就在 floor/ceil 之间
// 来回跳（实测 8→7→8、3→2→3），一帧前进一帧后退，看着像抖。
func TestScrollVisibleStepsAreMonotone(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	OnDocumentWheel(doc, 100, 100, 0, 100)
	prev := 0
	var steps []int
	for f := 0; f < 20; f++ {
		// 喂 60Hz 的帧间隔（16.67ms）。
		feedScrollClock(root, 16666667*time.Nanosecond)
		TickScroll(doc)
		cur := root.scrollAnimTop
		if cur != prev {
			steps = append(steps, cur-prev)
			prev = cur
		}
	}
	if len(steps) == 0 {
		t.Fatal("没有采到任何帧的位移")
	}
	for i := 1; i < len(steps); i++ {
		if steps[i] > steps[i-1] {
			t.Errorf("第 %d 帧位移从 %d 涨到 %d：可见位移应单调不增（有抖动）",
				i+1, steps[i-1], steps[i])
			break
		}
	}
	// 落点必须精确：动画结束时正好在目标上（量化误差在收尾那一帧补齐）。
	if root.scrollAnimTop != 100 {
		t.Errorf("动画结束位置 = %d, want 100", root.scrollAnimTop)
	}
}
