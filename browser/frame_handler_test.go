//go:build windows

package browser

import (
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

// 每帧回调在事件队列排空后、布局之前每轮调用一次；返回 true 期间主循环
// 不休眠 10ms（实测 SDL_Delay(1) 约 1ms），回调次数因此远多于 10ms 一档。
func TestFrameHandlerKeepsLoopBusy(t *testing.T) {
	win := newTestWindow(t, 200, 120, "frame-test")
	if _, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">x</body></html>`); err != nil {
		t.Fatal(err)
	}

	var calls int32
	win.SetFrameHandler(func() bool {
		atomic.AddInt32(&calls, 1)
		return true
	})

	const run = 200 * time.Millisecond
	win.SetAutoExit(run)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	// 1ms 轮询下 200ms 约 190 次；若退回 10ms 空闲休眠只有约 20 次。
	if n := atomic.LoadInt32(&calls); n < 30 {
		t.Fatalf("每帧回调只跑了 %d 次（%v），期望 ≥30：busy 时主循环没有改用 1ms 轮询", n, run)
	}
}

// 返回 false 的回调不能改变原有空闲行为（10ms 休眠），否则静态页面会
// 白白空转。
func TestFrameHandlerIdleKeepsSleeping(t *testing.T) {
	win := newTestWindow(t, 200, 120, "frame-idle-test")
	if _, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">x</body></html>`); err != nil {
		t.Fatal(err)
	}

	var calls int32
	win.SetFrameHandler(func() bool {
		atomic.AddInt32(&calls, 1)
		return false
	})

	const run = 120 * time.Millisecond
	win.SetAutoExit(run)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	// 10ms 一档：约 12 次；即使机器慢也不该到 40 次（那是 1ms 的量级）。
	if n := atomic.LoadInt32(&calls); n > 40 {
		t.Fatalf("空闲回调跑了 %d 次（%v），期望按 10ms 休眠（约 12 次）", n, run)
	}
	if n := atomic.LoadInt32(&calls); n == 0 {
		t.Fatal("每帧回调一次都没跑")
	}
}
