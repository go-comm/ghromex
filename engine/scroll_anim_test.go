package engine

import (
	"testing"
	"time"
)

// 连续滚动的运动必须连续：每一帧的可见位移都要**同向且非零**——不能出现
// 「某帧完全不动」的卡顿，也不能出现反向的小台阶（那正是之前 8→7→8、
// 3→2→3 那种抖动）。
//
// 旧模型（每档都从静止重新起步的固定曲线）实测每档的位移是
// 35/26/17/12/6/3/1/0px 的锯齿——滚起来一顿一顿。
//
// （旧断言是「相邻帧位移比 ≤4」，那对滚轮节奏太敏感：换一种节奏就会误报。
// 方向与非零才是真正的手感不变量。）
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
			tickScroll(t, doc)
			cur := root.scrollAnimTop
			steps = append(steps, cur-prev)
			prev = cur
		}
	}
	if len(steps) == 0 {
		t.Fatal("没有采到任何帧的位移")
	}
	for i, d := range steps {
		if d <= 0 {
			t.Errorf("第 %d 帧位移 = %dpx：连续滚动中位移必须为正"+
				"（不能整帧不动，也不能反向抖）", i+1, d)
			break
		}
	}
}

// 纵向滚轮不得截断横向动画的**帧间隔**。
//
// 帧时间戳是分轴的（scrollAnimLastTopNs / scrollAnimLastLeftNs）：共用
// 一份的话，一次 wheel 会把时间戳重置到 wheel 时刻，下一帧算出的帧间隔
// 就只剩「wheel 到这一帧」的一点点——触控板 momentum（wheel 比帧还密）
// 时，另一轴的动画几乎停住。
func TestScrollAxesDoNotShareFrameClock(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{width:4000px;height:400px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	// 横向滚一档 → 横向动画在途。
	OnDocumentWheel(doc, 100, 100, 100, 0)
	if root.scrollLeft != 100 {
		t.Fatalf("横向目标 = %d, want 100", root.scrollLeft)
	}
	feedScrollClock(root, scrollAnimFrame)
	TickScroll(doc)
	elapsedLeft := root.scrollAnimElapsedLeftNs
	if elapsedLeft != int64(scrollAnimFrame) {
		t.Fatalf("横向进度 = %v, want %v", elapsedLeft, scrollAnimFrame)
	}

	// 纵向滚一档：只重置纵向的时间戳/起点/进度。
	// 横向的时间戳在 wheel **之前**就该是「离上一帧 20ms」——共享时间戳
	// 的写法会被这次 wheel 重置掉，下一帧横向的帧间隔就≈0。
	root.scrollAnimLastLeftNs = time.Now().UnixNano() - int64(scrollAnimFrame)
	OnDocumentWheel(doc, 100, 100, 0, 100)
	TickScroll(doc)
	if got := root.scrollAnimElapsedLeftNs - elapsedLeft; got != int64(scrollAnimFrame) {
		t.Errorf("横向动画进度只推进了 %v（应为一帧 %v）：纵向滚轮截断了横向的帧间隔",
			got, scrollAnimFrame)
	}
}

// 动画途中再来一档滚轮：视觉位置必须连续（不能跳）。
//
// 目标继续累加（scrollTop 累加），缓动段从**当前视觉位置**重新起算，
// 所以位置既不跳回起点，也不顿挫。
func TestScrollRetargetKeepsPositionContinuous(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	OnDocumentWheel(doc, 100, 100, 0, 100)
	tickScroll(t, doc)
	tickScroll(t, doc)
	before := root.scrollAnimTop
	if before == 0 {
		t.Fatal("动画推进两帧后视觉位置仍为 0")
	}

	// 途中再来一档：目标累加到 200。
	OnDocumentWheel(doc, 100, 100, 0, 100)
	if root.scrollTop != 200 {
		t.Errorf("重定向后目标 = %d, want 200（目标应累加）", root.scrollTop)
	}
	if root.scrollAnimTop != before {
		t.Errorf("重定向前后视觉位置 = %d → %d（应保持，不能跳）", before, root.scrollAnimTop)
	}

	// 继续推进：从 before 接着往新目标走（不回起点）。
	tickScroll(t, doc)
	if root.scrollAnimTop <= before {
		t.Errorf("重定向后应继续前进：%d → %d", before, root.scrollAnimTop)
	}
}

// 向上滚：位移为负、绝对值单调不增、落点精确。
//
// 收尾保底 1px 的方向必须由「目标 − 可见位置」决定（向上滚是 -1），
// 规则 3 也不能让内容越过目标。
func TestScrollUpwardStepsAreMonotone(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	// 先向下滚两档并等它停下。
	OnDocumentWheel(doc, 100, 100, 0, 100)
	OnDocumentWheel(doc, 100, 100, 0, 100)
	for i := 0; i < 20; i++ {
		tickScroll(t, doc)
	}
	if root.scrollAnimTop != 200 {
		t.Fatalf("向下滚停稳后位置 = %d, want 200", root.scrollAnimTop)
	}

	// 向上滚一档：目标 100。
	OnDocumentWheel(doc, 100, 100, 0, -100)
	if root.scrollTop != 100 {
		t.Fatalf("向上滚后目标 = %d, want 100", root.scrollTop)
	}
	prev := 200
	var mags []int
	for f := 0; f < 20; f++ {
		feedScrollClock(root, 16666667*time.Nanosecond)
		TickScroll(doc)
		cur := root.scrollAnimTop
		if cur != prev {
			d := cur - prev
			if d > 0 {
				t.Fatalf("向上滚却向下走了 %dpx（位置 %d → %d）", d, prev, cur)
			}
			if cur < root.scrollTop {
				t.Fatalf("向上滚越过了目标：位置 %d < 目标 %d", cur, root.scrollTop)
			}
			mags = append(mags, -d)
			prev = cur
		}
	}
	if len(mags) == 0 {
		t.Fatal("向上滚没有采到任何帧的位移")
	}
	for i := 1; i < len(mags); i++ {
		if mags[i] > mags[i-1] {
			t.Errorf("向上滚第 %d 帧位移从 %d 涨到 %d：绝对值应单调不增",
				i+1, mags[i-1], mags[i])
			break
		}
	}
	if root.scrollAnimTop != 100 {
		t.Errorf("向上滚落点 = %d, want 100", root.scrollAnimTop)
	}

	// 继续向上滚过头：目标被夹到 0，动画要收敛到 0 且不越过。
	OnDocumentWheel(doc, 100, 100, 0, -500)
	if root.scrollTop != 0 {
		t.Fatalf("越界向上滚后目标 = %d, want 0（应被夹紧）", root.scrollTop)
	}
	for i := 0; i < 20; i++ {
		tickScroll(t, doc)
		if root.scrollAnimTop < 0 {
			t.Fatalf("越界向上滚后位置 = %d（<0，越过了目标）", root.scrollAnimTop)
		}
	}
	if root.scrollAnimTop != 0 {
		t.Errorf("越界向上滚落点 = %d, want 0", root.scrollAnimTop)
	}
}

// 高帧率 + 向上滚：每帧位移不足 1px，取整误差会累积；落点仍要精确。
//
// 这是收尾保底 1px 的**边际**情形：帧率越高，每帧位移越小，取整误差
// 同号累积得越多（1000fps 实测可累积到 8px），收尾必须吃得完。
func TestScrollHighHzUpwardLandsExactly(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	for _, hz := range []int{60, 240, 480, 1000} {
		doc, _ := openScrollDoc(t, 200, 200, src)
		root := inner(doc)
		frame := time.Duration(int64(time.Second) / int64(hz))

		// 向下滚两档，停下。
		OnDocumentWheel(doc, 100, 100, 0, 100)
		OnDocumentWheel(doc, 100, 100, 0, 100)
		for i := 0; i < 2000; i++ {
			feedScrollClock(root, frame)
			TickScroll(doc)
			if root.scrollAnimTop >= 200 && !root.scrollAnimOn {
				break
			}
		}
		if root.scrollAnimTop != 200 {
			t.Fatalf("%dHz：向下滚停稳后位置 = %d, want 200", hz, root.scrollAnimTop)
		}

		// 向上滚一档：位移为负、落点精确。
		OnDocumentWheel(doc, 100, 100, 0, -100)
		prev := root.scrollAnimTop
		for i := 0; i < 4000; i++ {
			feedScrollClock(root, frame)
			TickScroll(doc)
			cur := root.scrollAnimTop
			if cur > prev {
				t.Fatalf("%dHz：向上滚却向下走（%d → %d）", hz, prev, cur)
			}
			prev = cur
			if cur <= 100 && !root.scrollAnimOn {
				break
			}
		}
		if root.scrollAnimTop != 100 {
			t.Errorf("%dHz：向上滚落点 = %d, want 100（收尾没吃掉落后的累积误差）",
				hz, root.scrollAnimTop)
		}
	}
}

// 动画途中内容变矮 → 布局把目标夹到可滚范围 → 动画要收敛到**夹紧后的**
// 目标（不是原来的目标），也不能越过它。
func TestScrollTargetClampedMidAnimation(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	OnDocumentWheel(doc, 100, 100, 0, 500)
	if root.scrollTop != 500 {
		t.Fatalf("目标 = %d, want 500", root.scrollTop)
	}
	tickScroll(t, doc)
	tickScroll(t, doc)

	// 内容变矮：视口 200 高，内容 300 高 → 可滚范围 100 → 目标被夹到 100。
	doc.QuerySelector("#page").SetStyle("height:300px")
	LayoutDocument(doc)
	if root.scrollTop != 100 {
		t.Fatalf("夹紧后目标 = %d, want 100", root.scrollTop)
	}

	// 动画收敛到夹紧后的目标，且不越过。
	for i := 0; i < 20; i++ {
		tickScroll(t, doc)
		if root.scrollAnimTop > root.scrollTop {
			t.Fatalf("位置 %d 越过了夹紧后的目标 %d", root.scrollAnimTop, root.scrollTop)
		}
	}
	if root.scrollAnimTop != 100 {
		t.Errorf("夹紧后落点 = %d, want 100", root.scrollAnimTop)
	}
}

// 卡顿的一帧不能把内容「跳过」一大段。
//
// 动画按真实时间推进：遇到很慢的一帧（新内容进视口、要现建纹理/字模，
// 帧时间可能上百毫秒），内容就会一格一格跳过去——滚动时看起来一闪一闪。
// 单帧推进量因此有上限（scrollAnimFrame）：慢帧时滚动慢一点，但运动连续。
func TestScrollSlowFrameDoesNotJump(t *testing.T) {
	const src = `<!doctype html><html><head><style>
	body{margin:0}
	#page{height:4000px;background-color:#2563EB}
	</style></head><body><div id="page">content</div></body></html>`
	doc, _ := openScrollDoc(t, 200, 200, src)
	root := inner(doc)

	OnDocumentWheel(doc, 100, 100, 0, 400)
	// 假装上一帧是很久以前（模拟一次百毫秒级的卡顿）。
	root.scrollAnimLastTopNs -= int64(500 * time.Millisecond)
	root.scrollAnimLastLeftNs -= int64(500 * time.Millisecond)
	before := root.scrollAnimTop
	tickScroll(t, doc)
	step := root.scrollAnimTop - before
	// 单帧最多推进 20ms 的动画进度（约全程的 1/8）→ 远小于目标距离。
	if step > 300 {
		t.Errorf("卡顿的一帧把内容推进了 %dpx（>300）：单帧推进量应有上限，否则滚动是一格一格跳的", step)
	}
}
