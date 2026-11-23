//go:build windows

// 主线程任务队列（browser/tasks.go）的端到端用例：跨线程投递的动作必须
// 落在**主循环所在的那个 OS 线程**上执行。
//
// 为什么值得单独测：这条纪律是整个调试服务（以及任何「worker 改 DOM」的
// 宿主应用）能成立的地基。队列一旦把任务跑在投递方线程上，读文档/截图
// 就是跨线程访问 SDL 与引擎文档树——症状是偶发崩溃或状态错乱，测试很难
// 稳定复现，所以这里直接断言线程身份。
package browser

import (
	"errors"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// currentThreadID 取当前 OS 线程 ID（kernel32!GetCurrentThreadId）。
//
// 为什么不引 golang.org/x/sys/windows：本仓库零外部依赖，而 syscall 的
// LazyDLL 正是 backend/sdl2 加载 SDL 用的同一套办法。
var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCurThreadID = kernel32.NewProc("GetCurrentThreadId")
)

func currentThreadID() uint32 {
	tid, _, _ := procCurThreadID.Call()
	return uint32(tid)
}

// waitLoopRunning 等主循环进入 Run（Running() 为真）。
//
// 为什么要等：Run 之前调 Do/Post 会拿到 ErrLoopNotRunning（这是设计，不是
// bug）——测试里跨线程投递的时机不确定，只能等主循环真的起来了。
func waitLoopRunning(win *Window, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if win.Running() {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return errors.New("主循环没在限定时间内启动")
}

// TestDoRunsOnMainLoopThread：Do 里的闭包必须跑在主循环的 OS 线程上，
// 并且能在里面安全读到文档。
func TestDoRunsOnMainLoopThread(t *testing.T) {
	win := newTestWindow(t, 200, 120, "task-thread")
	if _, err := win.OpenDocument(`<!doctype html><html><body>x</body></html>`); err != nil {
		t.Fatal(err)
	}
	// 建窗时 runtime.LockOSThread 把当前 goroutine 钉在这个 OS 线程上，
	// Run 也在这里跑——所以测试函数所在线程就是「主线程」。
	mainThread := currentThreadID()

	type result struct {
		value  any
		err    error
		thread uint32
	}
	ch := make(chan result, 1)
	go func() {
		if err := waitLoopRunning(win, 2*time.Second); err != nil {
			ch <- result{err: err}
			win.RequestClose()
			return
		}
		var thread uint32
		v, err := win.Do(func() (any, error) {
			thread = currentThreadID()
			return win.Document() != nil, nil
		}, 3*time.Second)
		ch <- result{value: v, err: err, thread: thread}
		win.RequestClose()
	}()

	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatalf("Do 失败: %v", r.err)
	}
	if r.thread == 0 {
		t.Fatal("任务里的线程 ID 是 0：闭包没执行？")
	}
	if r.thread != mainThread {
		t.Errorf("任务跑在 OS 线程 %d，主循环在 %d：任务没落在主线程上", r.thread, mainThread)
	}
	if v, ok := r.value.(bool); !ok || !v {
		t.Errorf("Do 返回值 = %v，want true（任务里应能读到文档）", r.value)
	}
}

// TestDoRejectsWhenLoopNotRunning：Run 没跑（或已退出）时 Do 直接报错，
// 不排队、不让调用方干等一场超时。
func TestDoRejectsWhenLoopNotRunning(t *testing.T) {
	win := newTestWindow(t, 120, 80, "task-noloop")
	if _, err := win.Do(func() (any, error) { return 1, nil }, time.Second); !errors.Is(err, ErrLoopNotRunning) {
		t.Errorf("Do（未运行）= %v，want ErrLoopNotRunning", err)
	}
	if err := win.Post(func() {}); !errors.Is(err, ErrLoopNotRunning) {
		t.Errorf("Post（未运行）= %v，want ErrLoopNotRunning", err)
	}
}

// TestDoTimesOutWhenLoopBusy：主循环被长任务占住时，Do 到点报超时，
// 而不是无限等下去。
func TestDoTimesOutWhenLoopBusy(t *testing.T) {
	win := newTestWindow(t, 120, 80, "task-busy")
	if _, err := win.OpenDocument(`<!doctype html><html><body>x</body></html>`); err != nil {
		t.Fatal(err)
	}
	// 每帧睡 300ms：主循环被占住，任务排不上队。
	win.SetFrameHandler(func() bool {
		time.Sleep(300 * time.Millisecond)
		return true
	})
	ch := make(chan error, 1)
	go func() {
		if err := waitLoopRunning(win, 2*time.Second); err != nil {
			ch <- err
			win.RequestClose()
			return
		}
		_, err := win.Do(func() (any, error) { return 1, nil }, 50*time.Millisecond)
		ch <- err
		win.RequestClose()
	}()
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if err := <-ch; !errors.Is(err, ErrTaskTimeout) {
		t.Errorf("Do（主循环忙）= %v，want ErrTaskTimeout", err)
	}
}

// TestPostRunsOnMainLoop：Post 的闭包同样在主线程执行，且不阻塞投递方。
func TestPostRunsOnMainLoop(t *testing.T) {
	win := newTestWindow(t, 120, 80, "task-post")
	if _, err := win.OpenDocument(`<!doctype html><html><body>x</body></html>`); err != nil {
		t.Fatal(err)
	}
	mainThread := currentThreadID()

	var mu sync.Mutex
	var gotThread uint32
	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	go func() {
		if err := waitLoopRunning(win, 2*time.Second); err != nil {
			t.Errorf("等待主循环: %v", err)
			finish()
			return
		}
		// t.Errorf 允许在非测试 goroutine 上调（t.Fatal 不允许）。
		if err := win.Post(func() {
			mu.Lock()
			gotThread = currentThreadID()
			mu.Unlock()
			finish()
		}); err != nil {
			t.Errorf("Post: %v", err)
			finish()
		}
	}()

	// done 由任务本身关闭，所以不能在 Run 之前等它——另开一个 goroutine，
	// 拿到结果（或兜底超时）后请求关闭。
	go func() {
		select {
		case <-done:
		case <-time.After(4 * time.Second):
		}
		win.RequestClose()
	}()
	win.SetAutoExit(5 * time.Second)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	th := gotThread
	mu.Unlock()
	if th == 0 {
		t.Fatal("Post 的任务没被执行")
	}
	if th != mainThread {
		t.Errorf("Post 任务跑在 OS 线程 %d，主循环在 %d", th, mainThread)
	}
}

// TestTaskPanicBecomesError：任务里的 panic 被收敛成 error，不把宿主带走。
func TestTaskPanicBecomesError(t *testing.T) {
	win := newTestWindow(t, 120, 80, "task-panic")
	if _, err := win.OpenDocument(`<!doctype html><html><body>x</body></html>`); err != nil {
		t.Fatal(err)
	}
	ch := make(chan error, 1)
	go func() {
		if err := waitLoopRunning(win, 2*time.Second); err != nil {
			ch <- err
			win.RequestClose()
			return
		}
		_, err := win.Do(func() (any, error) {
			var p *int
			_ = *p // 故意的空指针：模拟外部脚本触发的坏请求
			return nil, nil
		}, 2*time.Second)
		ch <- err
		win.RequestClose()
	}()
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	err := <-ch
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("Do（任务 panic）= %v，want 带 panic 字样的错误", err)
	}
}
