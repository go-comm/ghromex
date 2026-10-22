// 本文件是 GHROMEX_BENCH 分段基准工具：把主循环的各段分开计时，
// 累积足够样本后报 p50/p95/p99。
//
// # 为什么要分段
//
// 「整帧多少毫秒」这个数字几乎没有诊断价值：
//   - 加速路径的整帧恒为 ~16.6ms（vsync 间隔），**与实际工作量无关**；
//   - dummy 驱动下 Present 是空操作，纯绘制与含 present 差一个数量级
//     （2560×1440：纯绘制 ~1.5ms vs 含 present ~4ms）。
//
// 两种口径的数字并排会得出「快了 2.7 倍」之类的错误结论。分段计时
// 才能说清「时间花在哪儿」，也能把 vsync 等待从真实工作里剥出来。
//
// # 不进 CI
//
// 计时受机器负载、驱动、电源策略、温度影响，数值不可复现——把它写成
// 断言只会得到一个 flaky 的测试。它是本地诊断工具，与 GHROMEX_DEBUG
// 同一层：手动跑、看数字、不做断言。
//
// # 口径：软件路径的「重绘」是入队，不是光栅化
//
// SDL 的软件光栅器把 DrawColor/DrawText **记进命令表**，真正填像素发生在
// Present 时。所以软件路径上：
//
//	重绘段  = 命令入队 + 遍历（与面积几乎无关，与调用数相关）
//	呈现段  = 真正的像素填充 + 拷贝到窗口（与面积相关）
//
// 加速路径相反：命令提交给驱动就返回，填充在 GPU 上，Present 里等 vsync。
//
// 这也解释了「纯绘制 1.5ms」与「整帧 4ms」为什么能对上：后者是前两者之和。
//
// # 用法
//
//	GHROMEX_BENCH=1 go run ./examples/portal
//	GHROMEX_BENCH=1 GHROMEX_BENCH_FRAMES=600 go run ./examples/portal
//
// 输出示例（软件路径，dummy 驱动，1280×820）：
//
//	ghromex bench: renderer=software (driver=software; 强制软件：GHROMEX_SOFTWARE/dummy)
//	  窗口=1280x820  样本=400  帧周期 p50=3.04ms
//	  布局 p50=0.00ms p95=0.00ms p99=0.00ms
//	  清屏 p50=0.00ms p95=0.00ms p99=0.52ms
//	  重绘 p50=1.19ms p95=2.23ms p99=2.75ms  ← 软件光栅器：此处只是命令入队，像素在「呈现」段才填
//	  呈现 p50=1.87ms p95=3.34ms p99=3.90ms  ← 软件路径：只是拷贝，无 vsync
//	  空闲 p50=0.00ms p95=0.00ms p99=0.00ms  ← 基准模式不进入休眠（模拟持续动画，量最大吞吐）
//	  工作合计（布局+清屏+重绘）p50=1.20ms p95=2.28ms p99=2.85ms
//	  每帧分配 246KB（Go 侧累计分配）
//	  绘制调用 p50=1194 p95=1194 p99=1194（每帧）
//	  堆常驻 3.1MB（GC 后；开跑基线 3.0MB）
//
// 同一页面同一尺寸走加速路径时：
//
//	ghromex bench: renderer=accelerated(GPU, driver=direct3d)
//	  窗口=1280x820  样本=400  帧周期 p50=16.47ms
//	  重绘 p50=1.32ms p95=2.43ms p99=3.08ms
//	  呈现 p50=14.62ms p95=15.84ms p99=16.19ms  ← 含等 vsync≈14.05ms（关 vsync 实测纯拷贝 0.57ms）
//
// 注意两边的「帧周期」不可横比：软件路径按工作量跑（3ms），加速路径被
// vsync 锁在 16.6ms，其中 14ms 是纯等待。要比就比真实工作（重绘 + 拷贝）。
package browser

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// benchSamples 是每一批统计的样本数。300 帧 @60fps ≈ 5 秒——短于此的
// 样本会被偶发尖峰主导（p99 尤其）。
const benchSamples = 300

// benchEnabled 读 GHROMEX_BENCH（按布尔解析：1/true/yes/on 才算开）。
func benchEnabled() bool {
	switch os.Getenv("GHROMEX_BENCH") {
	case "1", "true", "yes", "on", "ON", "True":
		return true
	}
	return false
}

// benchFrames 读 GHROMEX_BENCH_FRAMES（每批样本数，默认 benchSamples）。
func benchFrames() int {
	if s := os.Getenv("GHROMEX_BENCH_FRAMES"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 10 {
			return n
		}
	}
	return benchSamples
}

// bench 收集主循环各段的耗时样本。
type bench struct {
	win *Window

	layout  []time.Duration
	clear   []time.Duration
	render  []time.Duration
	present []time.Duration
	idle    []time.Duration
	period  []time.Duration
	// calls 是每帧的绘制调用数（画调用是软件路径的主要 CPU 开销之一，
	// 与「时间随什么涨」对照着看才能定位瓶颈）。
	calls []int

	// heapStart 是开跑前的堆基线（GC 后读数），用来算「这段时间涨了多少」。
	heapStart uint64
	// allocStart 是本批开始时的累计分配量，与批末相减得每帧分配。
	allocStart uint64
	// presentNoVSync 是关掉 vsync 后实测的呈现耗时（纯拷贝）。
	presentNoVSync time.Duration
	hasCalib       bool
}

// newBench 建一个基准收集器（已开跑：读堆基线）。
func newBench(win *Window) *bench {
	b := &bench{win: win}
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	b.heapStart = m.HeapAlloc
	b.allocStart = m.TotalAlloc
	return b
}

// calibVsync 关掉 vsync 量一段纯拷贝耗时，再恢复。
//
// 加速路径的 Present 里混着「等 vsync」与「拷贝」两件事，只有拆开才能
// 说清 16.6ms 里有多少是真实工作、多少是等显示器。软件路径没有 vsync，
// 不做该校准（量出来的差值只是噪声）。
func (b *bench) calibVsync() {
	if b == nil || b.win == nil || b.win.sdl == nil {
		return
	}
	if !strings.HasPrefix(b.win.sdl.RendererInfo(), "accelerated") {
		b.hasCalib = false
		return
	}
	const frames = 30
	b.win.sdl.SetVSync(false)
	defer b.win.sdl.SetVSync(true)
	var total time.Duration
	for i := 0; i < frames; i++ {
		t0 := time.Now()
		b.win.sdl.Present()
		total += time.Since(t0)
	}
	b.presentNoVSync = total / frames
	b.hasCalib = true
}

// add 收一帧的各段耗时。
func (b *bench) add(period, layout, clear, render, present, idle time.Duration, calls int) {
	b.period = append(b.period, period)
	b.layout = append(b.layout, layout)
	b.clear = append(b.clear, clear)
	b.render = append(b.render, render)
	b.present = append(b.present, present)
	b.idle = append(b.idle, idle)
	b.calls = append(b.calls, calls)
}

// report 出一份统计并清空样本。
func (b *bench) report() {
	if b == nil || len(b.period) == 0 {
		return
	}
	win := b.win
	fmt.Fprintf(os.Stderr, "ghromex bench: renderer=%s\n", win.sdl.RendererInfo())
	fmt.Fprintf(os.Stderr, "  窗口=%dx%d  样本=%d  帧周期 p50=%s\n",
		win.sdl.ViewportWidth(), win.sdl.ViewportHeight(), len(b.period),
		fmtDur(pct(b.period, 0.50)))
	fmt.Fprintf(os.Stderr, "  布局 %s\n", fmtSeg(b.layout))
	fmt.Fprintf(os.Stderr, "  清屏 %s\n", fmtSeg(b.clear))
	// 「重绘」在软件路径上量的其实是**绘制命令入队**：SDL 的软件光栅器把
	// DrawColor/DrawText 记进命令表，真正填像素发生在 Present 时（见
	// 「呈现」段）。所以清屏看着是 0ms —— 填 viewport 那块大矩形也在
	// Present 里才发生。加速路径则相反：命令提交给驱动，填充在 GPU 上，
	// Present 只等 vsync。
	renderNote := ""
	if !strings.HasPrefix(win.sdl.RendererInfo(), "accelerated") {
		renderNote = "  ← 软件光栅器：此处只是命令入队，像素在「呈现」段才填"
	}
	fmt.Fprintf(os.Stderr, "  重绘 %s%s\n", fmtSeg(b.render), renderNote)
	present := fmtSeg(b.present)
	switch {
	case b.hasCalib:
		wait := b.presentNoVSync
		if p50 := pct(b.present, 0.50); p50 > wait {
			wait = p50 - wait
		} else {
			wait = 0
		}
		present += fmt.Sprintf("  ← 含等 vsync≈%s（关 vsync 实测纯拷贝 %s）",
			fmtDur(wait), fmtDur(b.presentNoVSync))
	case strings.HasPrefix(win.sdl.RendererInfo(), "accelerated"):
		present += "  ← 含等 vsync（加速路径）"
	default:
		present += "  ← 软件路径：只是拷贝，无 vsync"
	}
	fmt.Fprintf(os.Stderr, "  呈现 %s\n", present)
	fmt.Fprintf(os.Stderr, "  空闲 %s  ← 主循环的节流休眠，不是 vsync\n", fmtSeg(b.idle))

	// 工作合计 = 布局+清屏+重绘（不含呈现：呈现含 vsync 等待，混进去
	// 就没法说「真实工作多少」）。
	work := make([]time.Duration, len(b.period))
	for i := range work {
		work[i] = b.layout[i] + b.clear[i] + b.render[i]
	}
	fmt.Fprintf(os.Stderr, "  工作合计（布局+清屏+重绘）%s\n", fmtSeg(work))

	// 每帧分配：批次起止的 TotalAlloc 之差 / 帧数（Go 侧；原生侧的
	// 纹理/目标缓冲不在这个数里）。
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	n := uint64(len(b.period))
	if n > 0 && m.TotalAlloc > b.allocStart {
		fmt.Fprintf(os.Stderr, "  每帧分配 %s（Go 侧累计分配）\n",
			fmtBytes((m.TotalAlloc-b.allocStart)/n))
	}

	// 绘制调用数：与「时间随什么涨」对照着看才知道瓶颈在哪。
	if len(b.calls) > 0 {
		cs := make([]int, len(b.calls))
		copy(cs, b.calls)
		sort.Ints(cs)
		fmt.Fprintf(os.Stderr, "  绘制调用 p50=%d p95=%d p99=%d（每帧）\n",
			cs[len(cs)*50/100], cs[minInt(len(cs)-1, len(cs)*95/100)],
			cs[minInt(len(cs)-1, len(cs)*99/100)])
	}

	// 堆常驻：GC 后再读，否则测到的是没回收的垃圾（量级参考，非稳态）。
	runtime.GC()
	runtime.ReadMemStats(&m)
	fmt.Fprintf(os.Stderr, "  堆常驻 %s（GC 后；开跑基线 %s）\n",
		fmtBytes(m.HeapAlloc), fmtBytes(b.heapStart))
	b.allocStart = m.TotalAlloc

	b.period = b.period[:0]
	b.layout = b.layout[:0]
	b.clear = b.clear[:0]
	b.render = b.render[:0]
	b.present = b.present[:0]
	b.idle = b.idle[:0]
	b.calls = b.calls[:0]
}

// minInt 取两数之小。
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fmtBytes 把字节数格式化成 MB/KB。
func fmtBytes(n uint64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/1024/1024)
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// pct 取分位数（样本排序后按比例取；p=0.5 即中位数）。
func pct(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := make([]time.Duration, len(d))
	copy(s, d)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := int(float64(len(s)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(s) {
		idx = len(s) - 1
	}
	return s[idx]
}

// fmtSeg 格式化一段耗时的分位数。
func fmtSeg(d []time.Duration) string {
	return fmt.Sprintf("p50=%s p95=%s p99=%s",
		fmtDur(pct(d, 0.50)), fmtDur(pct(d, 0.95)), fmtDur(pct(d, 0.99)))
}

// fmtDur 把耗时格式化成毫秒（带一位小数）。
func fmtDur(d time.Duration) string {
	return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
}
