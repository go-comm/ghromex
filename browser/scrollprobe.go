// 本文件是 GHROMEX_SCROLLPROBE 滚动探针：在真实主循环里**持续注入滚轮事件**，
// 把「滚动一帧到底花多少」拆开量出来。
//
// # 为什么 GHROMEX_BENCH 不够
//
// bench 强制每帧重绘，但页面是**静止**的：它量到的是「把当前这一屏重画一遍」
// 的成本，滚动专属的那部分开销（TickScroll 遍历子树平移盒子、滚到新位置后
// 可见集合变化带来的调用数变化）完全没进统计——TickScroll 在主循环里甚至是
// 裸调用、没计时。而「离屏纹理 / 瓦片缓存」这类优化的收益，恰恰只在滚动时
// 才存在。所以本探针做三件 bench 不做的事：
//
//  1. 每帧注入滚轮（走真实事件队列：onWheel → 滚动动画 → TickScroll），
//     量的就是用户狂滚滚轮时的那条路径，不是人造的静止重绘；
//  2. 单独计时 TickScroll（引擎每帧在滚动期唯一的遍历开销）；
//  3. 记录每帧绘制调用的**构成**（文字段 vs 色块）与「有位移的帧」比例，
//     用来判断视口裁剪是否真的生效、以及滚到底之后数据是否已经失真。
//
// # 读法
//
//   - 「平移 p50」很大（几 ms）→ 瓶颈在**引擎遍历子树**，离屏纹理救不了它
//     （瓦片同样要更新位置），该优化的是滚动偏移的表达方式（偏移不进盒子坐标，
//     改在绘制期现算）；
//   - 「平移」很小而「重绘/呈现」大 → 瓶颈在**绘制与填充**，脏矩形/分层纹理
//     才有肉；
//   - 「文字/帧」随滚动位置大幅波动 → 视口裁剪没起作用（或页面本身在视口外
//     还有大量片段），先修裁剪再谈缓存；
//   - 「有位移帧」比例低 → 注入没换来位移（页面压根不可滚、或滚轮落点是
//     不可滚动元素），这批重绘/呈现数字量的其实是静止页，**作废**。
//     探针自带「到界掉头」（连续几次无位移就反向滚），所以正常长页面
//     跑不出低比例；真跑出来了就说明不是「滚到底」的问题，是滚动没发生。
//
// # 环境变量
//
//	GHROMEX_SCROLLPROBE=1                 开探针（隐含 bench 的分段采集）
//	GHROMEX_SCROLLPROBE_NOTCH=n           每次注入滚 n 格（一格 100px，默认 1）
//	GHROMEX_SCROLLPROBE_EVERY=n           每 n 帧注入一次（默认 1＝持续滚）
//	GHROMEX_SCROLLPROBE_STALL=n           连续 n 次无位移判为到界并掉头（默认 4）
//	GHROMEX_BENCH_FRAMES=n                批次样本数（与 bench 共用，默认 300）
//
// # 不进 CI
//
// 与 bench 同一层：计时不可复现，只做本地诊断。
//
// # 用法
//
//	GHROMEX_SCROLLPROBE=1 go run ./examples/portal
//	GHROMEX_SCROLLPROBE=1 GHROMEX_BENCH_FRAMES=600 go run ./examples/portal
//	GHROMEX_SCROLLPROBE=1 GHROMEX_SCROLLPROBE_NOTCH=2 GHROMEX_SCROLLPROBE_EVERY=2 \
//	    go run ./examples/portal        # 每 2 帧滚 2 格（更接近真实滚轮节奏）
//	GHROMEX_GPU=1 GHROMEX_SCROLLPROBE=1 go run ./examples/portal   # 量加速路径
//
// 探针隐含 bench 的采集（分段计时 + 每帧重绘），因为持续滚动的每一帧本来
// 就要重绘——「强制每帧重绘」在这里不是失真，正是被测对象。
package browser

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/go-comm/ghromex/backend/sdl2"
)

// scrollProbeEnabled 读 GHROMEX_SCROLLPROBE（按布尔解析，与 benchEnabled 同口径）。
func scrollProbeEnabled() bool {
	switch os.Getenv("GHROMEX_SCROLLPROBE") {
	case "1", "true", "yes", "on", "ON", "True":
		return true
	}
	return false
}

// scrollProbeNotch 读 GHROMEX_SCROLLPROBE_NOTCH：每次注入的滚轮格数
// （一格 = wheelStepPx 像素，默认 1）。格数只能是整数，因为 Event.WheelY
// 是整数格——要「每帧滚 30px」请改 EVERY（拉大间隔等于放慢滚动）。
func scrollProbeNotch() int {
	if s := os.Getenv("GHROMEX_SCROLLPROBE_NOTCH"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 {
			return n
		}
	}
	return 1
}

// scrollProbeEvery 读 GHROMEX_SCROLLPROBE_EVERY：每隔多少帧注入一次（默认每帧）。
// 每帧注入 = 持续重定向滚动动画，模拟「滚轮一直滚」的最坏情况。
func scrollProbeEvery() int {
	if s := os.Getenv("GHROMEX_SCROLLPROBE_EVERY"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 {
			return n
		}
	}
	return 1
}

// scrollProbeStallFrames 读 GHROMEX_SCROLLPROBE_STALL：连续多少次「注入了但
// 内容没位移」判定为碰到滚动边界，掉头。默认 4——动画每帧都在推进，真在滚
// 就不可能连续几帧零位移；设太大都会在边界浪费样本，设 1 则会被缓动曲线起点
// （第一帧位移不足 1px 被取整为 0）误判。
func scrollProbeStallFrames() int {
	if s := os.Getenv("GHROMEX_SCROLLPROBE_STALL"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 {
			return n
		}
	}
	return 4
}

// scrollProbe 收集滚动期的专项样本（分段耗时仍由 bench 采，这里只补滚动
// 专属的三项：TickScroll 耗时、调用构成、有位移帧比例）。
type scrollProbe struct {
	win        *Window
	notch      int // 每次注入的滚轮格数
	every      int // 每隔多少帧注入一次
	stallLimit int // 连续多少次无位移判为到界（掉头阈值）

	frames   int // 已处理的帧数（决定注入节奏与批次边界）
	injected int // 已注入的滚轮事件数
	moved    int // TickScroll 报「内容真的动了」的帧数

	// 到界自动掉头：一直往一个方向滚，长页面几百帧就到底，剩下的样本量的
	// 是静止页（数字看着很漂亮但全是假的）。连续 stallLimit 次「注入了却没
	// 位移」判定为碰到边界，翻转方向，让整批样本都处在真实滚动中。
	dir     int  // +1 向下、-1 向上
	stall   int  // 连续「注入后无位移」的注入次数
	pending bool // 本帧是否真的注入了（every>1 时有些帧不注入）
	revs    int  // 本批掉头次数

	ticks  []time.Duration // TickScroll 每帧耗时
	texts  []int           // 每帧文字绘制调用
	colors []int           // 每帧色块绘制调用
	calls  []int           // 每帧总调用

	// 布局取证：滚动期**本一帧都不该重排**（滚动只平移，见 markScrollChanged
	// 与 LayoutDocument 的快路径）。分位数看不见偶发的一次全量重排——实测
	// 104 帧里那唯一一帧 1.75 秒的重排，p95/p99 读出来仍是 43/44ms，而用户
	// 的体感就是「打开页面后第一下滚轮卡住」。所以这里记的是**次数与帧号**。
	layouts int             // 本批调过 LayoutDocument 的帧数
	spikes  []time.Duration // 布局耗时超过 layoutSpike 的样本
	spikeAt []int           // 这些尖峰各发生在第几帧（判断是启动期还是滚动期）

	// 缩放事件次数：主循环收到尺寸变化会把 lastChange 置 -1 **强制重排**，
	// 于是一次性尖峰可能根本不是滚动引起的（开页时视口尺寸还没落定，就会在
	// 第 1 帧再排一遍整棵树）。把它和尖峰帧号并排看，才知道该找谁。
	resizes  int
	resizeAt []int // 前几次缩放事件发生的帧号（与尖峰帧号对上即是「启动重排」）

	// 鼠标移动次数与发生帧号：悬停态变化是**非滚动**变更（:hover 要改样式），
	// 一笔就能让下一帧全量重排。滚动探针若不知道鼠标动没动，就会把 hover 引起
	// 的尖峰误读成「滚动引起的尖峰」。
	motions  int
	motionAt []int
}

// noteMotion 记一次鼠标移动事件（见 motions 字段说明）。
func (p *scrollProbe) noteMotion() {
	if p == nil {
		return
	}
	p.motions++
	if len(p.motionAt) < 8 {
		p.motionAt = append(p.motionAt, p.frames)
	}
}

// noteResize 记一次窗口尺寸变化事件（主循环会因此强制重排，见 Run）。
func (p *scrollProbe) noteResize() {
	if p == nil {
		return
	}
	p.resizes++
	if len(p.resizeAt) < 8 {
		p.resizeAt = append(p.resizeAt, p.frames)
	}
}

// layoutSpike 是「这次布局干了重活」的阈值：快路径纯平移在 42 万元素页面上
// 实测 40ms，全量重排同页 1.75 秒以上，两个数量级之间取个位。
const layoutSpike = 300 * time.Millisecond

// newScrollProbe 建探针（注入参数取环境变量）。初值 dir=+1 先往下滚。
func newScrollProbe(win *Window) *scrollProbe {
	return &scrollProbe{
		win: win, notch: scrollProbeNotch(), every: scrollProbeEvery(),
		stallLimit: scrollProbeStallFrames(), dir: 1,
	}
}

// inject 在帧首（事件队列排空之前）注入一次滚轮：光标压在视口正中，保证
// 命中可滚动内容而不是边缘的滚动条。WheelY 取负 = 向下滚（onWheel 的口径是
// dy = -WheelY*wheelStepPx，向上为正），再乘掉头方向。
func (p *scrollProbe) inject() {
	if p == nil || p.win.doc == nil {
		return
	}
	p.frames++
	if p.frames%p.every != 0 {
		return
	}
	// 到界判定要在注入前做：这一注入就换成能动的方向，省掉边界处的空转帧。
	if p.stall >= p.stallLimit {
		p.dir = -p.dir
		p.stall = 0
		p.revs++
	}
	vw, vh := p.win.sdl.ViewportWidth(), p.win.sdl.ViewportHeight()
	p.win.InjectEvent(sdl2.Event{
		Kind:   sdl2.EventMouseWheel,
		X:      vw / 2,
		Y:      vh / 2,
		WheelY: -p.notch * p.dir,
	})
	p.injected++
	p.pending = true
}

// addTick 收一帧的 TickScroll 耗时与「是否真有位移」，并维护到界计数。
func (p *scrollProbe) addTick(d time.Duration, moved bool) {
	if p == nil {
		return
	}
	p.ticks = append(p.ticks, d)
	if moved {
		p.moved++
	}
	// 只统计「注入之后」的帧：every>1 时没注入的帧本来就不该有位移。
	if p.pending {
		p.pending = false
		if moved {
			p.stall = 0
		} else {
			p.stall++
		}
	}
}

// addLayout 收一帧的布局耗时（d==0 表示本帧没调 LayoutDocument，正常滚动
// 期间应该几乎全是这种）。
func (p *scrollProbe) addLayout(d time.Duration) {
	if p == nil || d <= 0 {
		return
	}
	p.layouts++
	if d > layoutSpike {
		p.spikes = append(p.spikes, d)
		p.spikeAt = append(p.spikeAt, p.frames)
	}
}

// addDraw 收一帧的绘制调用构成（从计数 Graphics 里取）。
func (p *scrollProbe) addDraw(texts, colors, calls int) {
	if p == nil {
		return
	}
	p.texts = append(p.texts, texts)
	p.colors = append(p.colors, colors)
	p.calls = append(p.calls, calls)
}

// report 在批次满（与 bench 同批界）时出专项统计并清空样本。
func (p *scrollProbe) report() {
	if p == nil || p.frames == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "ghromex scrollprobe: 每帧滚 %d 格(每 %d 帧一次)  注入=%d 次  样本帧=%d\n",
		p.notch, p.every, p.injected, p.frames)
	fmt.Fprintf(os.Stderr, "  滚动平移(TickScroll) %s\n", fmtSeg(p.ticks))
	// 布局取证：滚动期不该有全量重排。尖峰落在第几帧很关键——帧号很小（启动
	// 头几帧）说明是开页/首个尺寸事件的一次性代价；帧号散布在整批里说明滚动
	// 本身在触发重排，那是必须修的。
	fmt.Fprintf(os.Stderr, "  布局帧 %d/%d", p.layouts, p.frames)
	if n := len(p.spikes); n > 0 {
		fmt.Fprintf(os.Stderr, "  重活(>%s) %d 次: ", fmtDur(layoutSpike), n)
		for i, d := range p.spikes {
			if i > 0 {
				fmt.Fprint(os.Stderr, ", ")
			}
			fmt.Fprintf(os.Stderr, "帧%d=%s", p.spikeAt[i], fmtDur(d))
		}
		// 尖峰帧号很小（开头几帧）→ 启动/缩放的一次性代价；散布在整批里 →
		// 滚动本身在触发重排，那是必须修的缺陷。
		if len(p.resizeAt) > 0 {
			fmt.Fprintf(os.Stderr, "  （缩放事件 %d 次，帧%v）", p.resizes, p.resizeAt)
		}
		if len(p.motionAt) > 0 {
			fmt.Fprintf(os.Stderr, "  （鼠标移动 %d 次，帧%v←hover 改样式即非滚动变更）", p.motions, p.motionAt)
		}
	} else if len(p.resizeAt) > 0 || len(p.motionAt) > 0 {
		fmt.Fprintf(os.Stderr, "  （缩放 %d 次/帧%v，鼠标移动 %d 次/帧%v）",
			p.resizes, p.resizeAt, p.motions, p.motionAt)
	}
	fmt.Fprintln(os.Stderr)
	// 有位移帧比例：低说明页面已滚到底或动画被反复重定向吃掉，此时后面的
	// 重绘/呈现数字量的是静止页，不能当滚动成本用。
	ratio := 0.0
	if p.frames > 0 {
		ratio = float64(p.moved) / float64(p.frames) * 100
	}
	warn := ""
	if ratio < 60 {
		warn = fmt.Sprintf("  ← 只有 %.0f%% 的帧真的动了：注入没换来位移（页面不可滚？滚轮打在不可滚动元素上？），"+
			"这批重绘/呈现数字按静止页理解", ratio)
	}
	fmt.Fprintf(os.Stderr, "  有位移帧 %d/%d（%.0f%%） 掉头=%d 次%s\n",
		p.moved, p.frames, ratio, p.revs, warn)
	fmt.Fprintf(os.Stderr, "  绘制调用/帧 %s\n", fmtIntSeg(p.calls))
	// 文字段与色块分开报：文字片段是逐 run 裁的（见 engine/render.go 的
	// offscreenBox），这两个数随滚动位置波动就说明裁剪在起作用。
	fmt.Fprintf(os.Stderr, "  其中文字 %s / 色块 %s\n", fmtIntSeg(p.texts), fmtIntSeg(p.colors))

	p.frames, p.injected, p.moved, p.stall, p.revs, p.layouts = 0, 0, 0, 0, 0, 0
	p.resizes, p.resizeAt = 0, p.resizeAt[:0]
	p.motions, p.motionAt = 0, p.motionAt[:0]
	p.ticks = p.ticks[:0]
	p.texts = p.texts[:0]
	p.colors = p.colors[:0]
	p.calls = p.calls[:0]
	p.spikes = p.spikes[:0]
	p.spikeAt = p.spikeAt[:0]
}

// fmtIntSeg 格式化一组整数的分位数（调用数没有单位，只报数）。
func fmtIntSeg(xs []int) string {
	if len(xs) == 0 {
		return "p50=0 p95=0 p99=0"
	}
	s := make([]int, len(xs))
	copy(s, xs)
	sort.Ints(s)
	at := func(p float64) int {
		i := int(p * float64(len(s)-1))
		if i < 0 {
			i = 0
		}
		if i >= len(s) {
			i = len(s) - 1
		}
		return s[i]
	}
	return fmt.Sprintf("p50=%d p95=%d p99=%d min=%d max=%d",
		at(0.50), at(0.95), at(0.99), s[0], s[len(s)-1])
}
