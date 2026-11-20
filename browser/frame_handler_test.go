//go:build windows

package browser

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 每帧回调（SetFrameHandler）：返回 true 时主循环用 1ms 轮询，否则 10ms。
// 休眠粒度直接决定拖动窗口是否跟手，所以单独钉住这条策略。
func TestIdleDelay(t *testing.T) {
	if got := idleDelay(true); got != 1 {
		t.Errorf("idleDelay(true) = %d, want 1（拖动时要用 1ms 轮询）", got)
	}
	if got := idleDelay(false); got != 10 {
		t.Errorf("idleDelay(false) = %d, want 10（空闲时省 CPU）", got)
	}
}

// runFrameLoop 建窗、把空闲休眠换成记录器，跑到采够 want 次「请求睡几毫秒」
// 后请求关闭，返回采到的毫秒序列。
//
// 断言「请求值」而不是「回调次数」：Windows 的 Sleep(1) 并不保证 1ms（默认
// 计时器粒度 15.6ms，负载下 1ms 会被拉成约 15ms），200ms 里只跑 12~14 帧
// ——与 10ms 一档的次数区间重叠。按次数判断既测不出真实档位，机器一忙还会
// 假失败（v0.6.0 前 TestFrameHandlerKeepsLoopBusy 就这么红过：单独跑 190
// 次通过、整包 `go test ./...` 只跑 14 次）。
//
// 退出也不靠计时：采够样本后在回调里 RequestClose，主循环下一轮收到
// SDL_QUIT 就退出；SetAutoExit 只作兜底，回归时不至于把测试挂死。
func runFrameLoop(t *testing.T, busy bool, want int) []int {
	t.Helper()
	win := newTestWindow(t, 200, 120, "frame-test")
	if _, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">x</body></html>`); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var ms []int
	win.delay = func(d int) {
		mu.Lock()
		ms = append(ms, d)
		mu.Unlock()
	}

	var calls int32
	win.SetFrameHandler(func() bool {
		atomic.AddInt32(&calls, 1)
		mu.Lock()
		enough := len(ms) >= want
		mu.Unlock()
		if enough {
			win.RequestClose()
		}
		return busy
	})
	win.SetAutoExit(5 * time.Second)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	if n := atomic.LoadInt32(&calls); n < 2 {
		t.Fatalf("每帧回调只跑了 %d 次：回调没有每帧调用", n)
	}
	if len(ms) < want {
		t.Fatalf("只采到 %d 次空闲休眠请求（期望 ≥%d）", len(ms), want)
	}
	return append([]int(nil), ms...)
}

// 每帧回调在事件队列排空后、布局之前每轮调用一次；返回 true 期间主循环
// 按 1ms 一档休眠（档位由 idleDelay 给出，这里验证主循环确实用了它）。
func TestFrameHandlerKeepsLoopBusy(t *testing.T) {
	for i, d := range runFrameLoop(t, true, 5) {
		if d != 1 {
			t.Fatalf("第 %d 次空闲休眠请求 %dms，期望 1ms：busy 时主循环没有改用 1ms 轮询", i+1, d)
		}
	}
}

// 返回 false 的回调不能改变原有空闲行为（10ms 休眠），否则静态页面会
// 白白空转。
func TestFrameHandlerIdleKeepsSleeping(t *testing.T) {
	for i, d := range runFrameLoop(t, false, 5) {
		if d != 10 {
			t.Fatalf("第 %d 次空闲休眠请求 %dms，期望 10ms：空闲时不该改用 1ms 轮询", i+1, d)
		}
	}
}
