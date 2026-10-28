package engine

import (
	"fmt"
	"strings"
	"testing"
)

// 滚动期的布局快路径回归测试。
//
// # 为什么需要它
//
// 滚动本身要 +changed（宿主据此重绘），但必须同时 +scrollMark：两者自上次
// 布局以来的**增量相等**，LayoutDocument 才认得出「只发生过滚动」，跳过级联
// 与盒模型、只平移已布局的盒子（见 markScrollChanged / LayoutDocument）。
//
// 于是任何一笔**非滚动**的 changed 落在两次布局之间，下一档滚轮就是整棵树
// 重排。实测代价（1024×720、42 万元素、14.9MB 页面的压力页）：
//
//	纯平移        40ms   ← 快路径
//	全量重排   1750ms   ← 快路径失效
//
// 这个量级差意味着：快路径失效不是「慢一点」，而是滚第一轮卡 1.75 秒。历史
// 上它被一个极隐蔽的缺陷废掉过——视口尺寸变化在记账**之后**才 +changed，留下
// 永久 +1 错位，于是开页后与每次窗口缩放后的第一档滚轮都走全量。
//
// # 判定手段
//
// 用**文本测量次数**而不是计时：快路径一次都不测（不动级联、不排盒），全量
// 重排会把整棵树的行片段重测一遍。计时不可复现，不能写成断言（见 bench.go
// 的同款说明）。

// measureCounter 是计数桩：透传 FakeGraphics，另外数出文本测量次数。
type measureCounter struct {
	Graphics
	measures int
}

func newMeasureCounter(w, h int) (*measureCounter, *HeadlessViewport) {
	c := &measureCounter{Graphics: NewFakeGraphics()}
	return c, &HeadlessViewport{W: w, H: h, G: c}
}

func (c *measureCounter) MeasureText(s string, size int, bold bool, family string) (int, int) {
	c.measures++
	return c.Graphics.MeasureText(s, size, bold, family)
}

func (c *measureCounter) FontMetrics(s string, size int, bold bool, family string) (int, int) {
	c.measures++
	return c.Graphics.FontMetrics(s, size, bold, family)
}

// 加权测量也要透传：不实现的话后端类型断言会失败，布局走的是另一条代码路径，
// 数出来的量就不再是「整棵树重测」的真实规模。
func (c *measureCounter) MeasureTextWeight(s string, size, weightNum int, family string) (int, int) {
	c.measures++
	if wm, ok := c.Graphics.(WeightedMeasurer); ok {
		return wm.MeasureTextWeight(s, size, weightNum, family)
	}
	return c.Graphics.MeasureText(s, size, weightNum >= 550, family)
}

func (c *measureCounter) FontMetricsWeight(s string, size, weightNum int, family string) (int, int) {
	c.measures++
	if wm, ok := c.Graphics.(WeightedMeasurer); ok {
		return wm.FontMetricsWeight(s, size, weightNum, family)
	}
	return c.Graphics.FontMetrics(s, size, weightNum >= 550, family)
}

// longPage 造一个必然可滚的页面：paras 段文字，每段带 id 便于取深处元素。
func longPage(paras int) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><style>
		p { font-size: 14px; margin: 0 0 8px; }
	</style></head><body style="margin:0">`)
	for i := 0; i < paras; i++ {
		fmt.Fprintf(&b, `<p id="p%d">第 %d 段：滚动只该平移盒子，不该重排整棵树。</p>`, i, i)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

// openLong 打开可滚页并返回（文档, 内部实现, 计数桩, 视口）。
// 返回的计数桩已清过零：首次全量布局的测量数不算进后续断言。
func openLong(tb testing.TB, paras, w, h int) (HTMLDocument, *htmlDocument, *measureCounter, *HeadlessViewport) {
	tb.Helper()
	c, vp := newMeasureCounter(w, h)
	doc, err := OpenDocument(vp, longPage(paras))
	if err != nil {
		tb.Fatal(err)
	}
	c.measures = 0
	return doc, doc.(*htmlDocument), c, vp
}

// layoutAndMeasures 跑一次 LayoutDocument，返回本次消耗的文本测量数（>0 即
// 走了全量重排，==0 即走了纯平移快路径）。
func layoutAndMeasures(doc HTMLDocument, c *measureCounter) int {
	before := c.measures
	LayoutDocument(doc)
	return c.measures - before
}

// 开页后的第一档滚轮必须走快路径。
//
// 这是最容易破的一条：任何在布局末尾追加 changed 的收尾动作（同步视口尺寸、
// 补记首屏状态之类）都会留下 +1 错位，把这一帧变成全量重排。
func TestFirstWheelAfterOpenTakesFastPath(t *testing.T) {
	doc, d, c, _ := openLong(t, 120, 400, 300)

	// 前置条件：布局刚结束，账必须是平的，否则下面测的是别的东西。
	if skew := d.changed - d.lastLayoutChange; skew != 0 {
		t.Fatalf("开页后 changed 比 lastLayoutChange 多 %d：布局的记账被布局之外的收尾动作污染，"+
			"滚动快路径会被永久废掉（见 notifyChanged）", skew)
	}

	if !OnDocumentWheel(doc, 10, 10, 0, 100) {
		t.Fatal("滚轮未产生滚动：测试页不可滚，请加大段落数或缩小视口")
	}
	if got := layoutAndMeasures(doc, c); got != 0 {
		t.Fatalf("开页后第一档滚轮重排了整棵树（文本测量 %d 次），快路径失效", got)
	}
}

// 窗口缩放后的第一档滚轮也必须走快路径：缩放那一次布局要全量（尺寸变了，
// 平移改不了盒宽），但**只能全量这一次**。
func TestWheelAfterResizeTakesFastPath(t *testing.T) {
	doc, _, c, vp := openLong(t, 120, 400, 300)

	vp.W = 460 // 后端只改视口、不改文档内容：变更计数不会动
	if got := layoutAndMeasures(doc, c); got == 0 {
		t.Fatal("视口变宽后没有重排：缩放必须走全量（平移改不了盒宽）")
	}

	if !OnDocumentWheel(doc, 10, 10, 0, 100) {
		t.Fatal("滚轮未产生滚动")
	}
	if got := layoutAndMeasures(doc, c); got != 0 {
		t.Fatalf("缩放后第一档滚轮重排了整棵树（文本测量 %d 次）：尺寸这笔账没落在记账之前", got)
	}
}

// 缩放不得复用滚动快路径——即便同帧既有滚动又有缩放，也必须全量。
func TestResizeWithScrollForcesFullLayout(t *testing.T) {
	doc, _, c, vp := openLong(t, 120, 400, 300)

	if !OnDocumentWheel(doc, 10, 10, 0, 100) {
		t.Fatal("滚轮未产生滚动")
	}
	if got := layoutAndMeasures(doc, c); got != 0 {
		t.Fatalf("前置条件不成立：纯滚动却重排了（测量 %d 次）", got)
	}

	vp.H = 260 // 滚而未排的偏移还在，此时缩放：只平移会算错可视高度
	if got := layoutAndMeasures(doc, c); got == 0 {
		t.Fatal("带滚动的缩放走了平移快路径：盒宽/可视高都变了，必须全量")
	}

	if !OnDocumentWheel(doc, 10, 10, 0, 100) {
		t.Fatal("滚轮未产生滚动")
	}
	if got := layoutAndMeasures(doc, c); got != 0 {
		t.Fatalf("缩放后的第二档滚轮仍在重排（测量 %d 次）", got)
	}
}

// 滚动动画推进的每一帧都不得产生「非滚动」变更，也不得触发布局：动画帧只
// 平移（TickScroll 自己完成），宿主连 LayoutDocument 都不该调。
func TestScrollAnimationFramesStayOutOfLayout(t *testing.T) {
	doc, d, c, _ := openLong(t, 120, 400, 300)
	if !OnDocumentWheel(doc, 10, 10, 0, 300) {
		t.Fatal("滚轮未产生滚动")
	}
	LayoutDocument(doc)

	base := inner(doc)
	lastChange := d.changed // 宿主口径：ChangeCount 没变就不布局
	relayouts := 0
	for f := 0; f < 12; f++ {
		// 真实流逝一帧：动画时间戳回拨（与 scroll_anim_test.go 同手法），
		// 否则同一微秒内推进量为 0，量不到东西。
		base.scrollAnimLastTopNs -= int64(16 * 1000 * 1000)
		ch0, sc0 := d.changed, d.scrollMark
		if !TickScroll(doc) {
			continue
		}
		// 动画帧若要标记，只能标滚动类：非滚动的那一笔会废掉下一帧的快路径。
		if extra := (d.changed - ch0) - (d.scrollMark - sc0); extra != 0 {
			t.Fatalf("动画帧混入 %d 次非滚动变更：每一处都会让下一帧全量重排", extra)
		}
		if d.changed != lastChange {
			relayouts++
			lastChange = d.changed
			if got := layoutAndMeasures(doc, c); got != 0 {
				t.Fatalf("动画帧触发的布局重排了整棵树（测量 %d 次）", got)
			}
		}
	}
	if relayouts > 1 {
		t.Errorf("一次滚轮动画引发 %d 次布局，期望 0～1 次（滚动只平移，不该反复重排）", relayouts)
	}
}

// 量化两种布局的规模差，给「滚动偏移现算」那步（省掉每帧平移遍历）留对照：
// 快路径的耗时全部来自遍历，与页面元素数线性相关。

func BenchmarkScrollFastPathLayout(b *testing.B) {
	doc, d, _, _ := openLong(b, 2000, 400, 300)
	if !OnDocumentWheel(doc, 10, 10, 0, 100) {
		b.Fatal("滚轮未产生滚动")
	}
	LayoutDocument(doc)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.markScrollChanged() // 只标滚动：LayoutDocument 必须走平移
		LayoutDocument(doc)
	}
}

func BenchmarkFullRelayout(b *testing.B) {
	doc, d, _, _ := openLong(b, 2000, 400, 300)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.markChanged() // 造一次非滚动变更：强制走全量
		LayoutDocument(doc)
	}
}
