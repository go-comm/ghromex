// 本文件是「把活儿交回主循环」的任务队列。
//
// # 为什么必须有这一层
//
// SDL 的窗口与渲染器绑在 `NewWindow→Run→Close` 所在的那个 OS 线程
// （`LockOSThread`），引擎文档树也没有内部锁——它假定单线程访问。跨线程
// 直接碰 `doc` / `Graphics` / `Screenshot` 不是「偶发崩溃」，而是必然的数据
// 竞争。可是宿主应用里总有人在别的 goroutine 上干活：HTTP 调试服务收到一个
// 「截图」请求、后台 worker 下载完一张图想插进 DOM、定时器想改状态栏文本。
//
// 规则就定成一条：**跨线程只能投递闭包，执行永远发生在主循环里**。
// 输入注入是例外——`Window.InjectEvent` 底层是 `SDL_PushEvent`，本身线程安全，
// 事件自然会在主循环下一帧被消费，所以它不必走这条队列。
//
// # 与帧节拍的关系
//
// 队列在主循环里的排空点：平台事件排空之后、`SetFrameHandler` 与布局之前。
// 于是任务里改的 DOM 在同一帧就被重排重绘，不需要多等一拍。队列非空时
// 主循环不走 10ms 空闲休眠（见 Run 里的 `hasTasks`），一次请求的往返
// 因此受帧节拍而非休眠档位支配。

package browser

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// 队列相关错误。
var (
	// ErrLoopNotRunning 表示主循环没在跑：`Run` 还没开始或已经返回。
	// 此时没有消费者，投递任务只会让等待方白等，所以直接拒绝。
	ErrLoopNotRunning = errors.New("browser: 主循环未在运行（Run 未开始或已退出）")
	// ErrTaskTimeout 表示任务已投递但主循环在超时前没有执行它。
	// 常见原因：主循环正卡在一个长布局/长绘制上，或 `Run` 在等待期间退出了。
	ErrTaskTimeout = errors.New("browser: 等待主循环执行任务超时")
	// ErrLoopStopped 表示任务还没执行，主循环就先退出了（等待方拿到的是错误，
	// 而不是干等一场超时）。
	ErrLoopStopped = errors.New("browser: 主循环已退出，任务未执行")
)

// taskOutcome 是一个主线程任务的执行结果。
type taskOutcome struct {
	value any
	err   error
}

// mainTask 是队列里的一个任务。reply 为 nil 表示「不等结果」（Post）；
// 缓冲为 1，排空侧永远不会因为没人收而卡在主循环里。
type mainTask struct {
	run   func() taskOutcome
	reply chan taskOutcome
}

// Post 把一个闭包投进主线程任务队列，不等结果。
//
// 用途：别的 goroutine 上拿到了数据，想让主循环下一帧用它改 DOM / 换标题 /
// 开文档。执行点见本文件头说明。主循环没在跑时返回 ErrLoopNotRunning
// （不排队、不吞掉，免得调用方以为生效了）。
//
// 闭包里 panic 不会带走宿主应用：异常被转成一行 stderr 诊断。
// 想要结果或错误请用 Do。
//
// 不要在主循环的 goroutine 上调 Post 并指望「本帧就执行」——那要看排空点
// 是否已经过去；同 goroutine 直接写代码就行，队列是给跨线程用的。
func (win *Window) Post(f func()) error {
	if win == nil || f == nil {
		return errors.New("browser: Post 需要非空任务")
	}
	t := mainTask{run: func() taskOutcome {
		f()
		return taskOutcome{}
	}}
	return win.enqueue(t)
}

// Do 把一个函数投进主线程任务队列，等它在主循环里跑完并拿回结果。
//
// timeout <= 0 表示不限时等待（谨慎：主循环可能永远不回来）。返回的 error
// 有三类来源：任务自身的 error、超时（ErrTaskTimeout）、主循环提前退出
// （ErrLoopStopped）；任务 panic 也被转成 error（ErrLoopNotRunning 之外的
// 另一层保护：一个坏请求不该把宿主应用一起带走）。
//
// 超时后任务可能仍会被执行（它已经在队列里，主循环只是慢）——所以传给 Do 的
// 闭包要么幂等，要么只做「读」，别做「只能做一次」的事。
//
// 必须从**非主循环**的 goroutine 调用：同 goroutine 调用等于让主循环等自己，
// 只会稳定拿到超时。跨线程读文档、截图、绘制都靠它。
func (win *Window) Do(f func() (any, error), timeout time.Duration) (any, error) {
	if win == nil || f == nil {
		return nil, errors.New("browser: Do 需要非空任务")
	}
	reply := make(chan taskOutcome, 1)
	t := mainTask{
		run: func() taskOutcome {
			v, err := f()
			return taskOutcome{value: v, err: err}
		},
		reply: reply,
	}
	if err := win.enqueue(t); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		o := <-reply
		return o.value, o.err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case o := <-reply:
		return o.value, o.err
	case <-timer.C:
		return nil, fmt.Errorf("%w: %s", ErrTaskTimeout, timeout)
	}
}

// enqueue 在锁内检查「主循环是否在跑」再排队。
//
// 检查必须在锁里：停循环的一侧同样是「先置标志、再清队列」并持锁，
// 两边各自加锁才有全序——否则可能夹进一个「已无人消费」的任务。
func (win *Window) enqueue(t mainTask) error {
	win.taskMu.Lock()
	if !win.loopRunning {
		win.taskMu.Unlock()
		return ErrLoopNotRunning
	}
	win.tasks = append(win.tasks, t)
	win.taskMu.Unlock()
	return nil
}

// hasTasks 报告队列里是否还有任务：主循环据此决定要不要跳过空闲休眠。
func (win *Window) hasTasks() bool {
	win.taskMu.Lock()
	n := len(win.tasks)
	win.taskMu.Unlock()
	return n > 0
}

// Running 报告主循环是否正在 Run 里跑（返回后即可能已经变化，只适合做观测）。
func (win *Window) Running() bool {
	win.taskMu.Lock()
	r := win.loopRunning
	win.taskMu.Unlock()
	return r
}

// startLoop 标记主循环开始（Run 的循环体之前）。
func (win *Window) startLoop() {
	win.taskMu.Lock()
	win.loopRunning = true
	win.taskMu.Unlock()
}

// stopLoop 标记主循环结束，并把队列里还没执行的任务全部报错结清，
// 免得等待方一直等到超时才发现窗口关了。
func (win *Window) stopLoop() {
	win.taskMu.Lock()
	win.loopRunning = false
	pending := win.tasks
	win.tasks = nil
	win.taskMu.Unlock()
	for _, t := range pending {
		if t.reply != nil {
			t.reply <- taskOutcome{err: ErrLoopStopped}
		}
	}
}

// drainTasks 执行队列里的全部任务，直到队列为空。只由主循环调用。
//
// 这里用「排空到底」而不是「每帧最多执行 N 个」：任务本身是宿主递交的短动作，
// 排队积压意味着外部脚本在等结果，限流只会把延迟摊到更多帧上。
// 真要把长任务切片的宿主，自己在闭包里分批 Post。
func (win *Window) drainTasks() {
	for {
		win.taskMu.Lock()
		if len(win.tasks) == 0 {
			win.taskMu.Unlock()
			return
		}
		t := win.tasks[0]
		win.tasks = win.tasks[1:]
		if len(win.tasks) == 0 {
			win.tasks = nil // 队列空了就放掉底层数组，避免长期占着一段积压
		}
		win.taskMu.Unlock()

		out := runTask(t.run)
		if t.reply != nil {
			t.reply <- out
		} else if out.err != nil {
			fmt.Fprintf(os.Stderr, "ghromex: 主线程任务失败: %v\n", out.err)
		}
	}
}

// runTask 执行一个任务，并把 panic 收敛成 error（named return + recover）。
func runTask(run func() taskOutcome) (out taskOutcome) {
	defer func() {
		if r := recover(); r != nil {
			out = taskOutcome{err: fmt.Errorf("主线程任务 panic: %v", r)}
		}
	}()
	return run()
}
