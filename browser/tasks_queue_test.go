// 主线程任务队列的**纯队列**用例：不建窗口、不碰 SDL，任意平台可跑。
//
// 为什么单开一个文件：线程身份那组断言只能在 Windows（真窗口）上做，
// 而「停循环要结清任务」「panic 要收敛成错误」这两条是纯逻辑，
// 放在无标签文件里 CI 才会真的跑到它们。
package browser

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestStopLoopFailsQueuedTasks：Run 返回（stopLoop）时把队列里没执行的任务
// 结清成 ErrLoopStopped，等待方不必干等到超时才发现窗口没了。
//
// 这里用 startLoop/enqueue/stopLoop 直接造时序——走真实 Run 会撞上调度抖动，
// 「任务已排队但循环将退出」这个瞬间在真实循环里几乎抓不住。
func TestStopLoopFailsQueuedTasks(t *testing.T) {
	win := &Window{}
	win.startLoop()

	reply := make(chan taskOutcome, 1)
	if err := win.enqueue(mainTask{
		run:   func() taskOutcome { return taskOutcome{value: 1} },
		reply: reply,
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	win.stopLoop()

	select {
	case o := <-reply:
		if !errors.Is(o.err, ErrLoopStopped) {
			t.Errorf("结清结果 = %v，want ErrLoopStopped", o.err)
		}
		if o.value != nil {
			t.Errorf("结清结果带了值 %v，want nil", o.value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stopLoop 没有结清排队中的任务")
	}
	// 循环已停：后续投递直接被拒（不留「投进去没人管」的暗坑）。
	if err := win.Post(func() {}); !errors.Is(err, ErrLoopNotRunning) {
		t.Errorf("停循环后 Post = %v，want ErrLoopNotRunning", err)
	}
}

// TestTaskPanicCaptured：runTask 把 panic 变成 error——Post 侧只打一行诊断，
// Do 侧回给等待方。一个坏请求不该把宿主应用一起带走。
func TestTaskPanicCaptured(t *testing.T) {
	out := runTask(func() taskOutcome {
		var p *int
		_ = *p // 空指针：模拟外部脚本触发的坏动作
		return taskOutcome{}
	})
	if out.err == nil || !strings.Contains(out.err.Error(), "panic") {
		t.Errorf("runTask（panic）= %v，want 带 panic 字样的错误", out.err)
	}
	// 正常路径不受影响。
	if o := runTask(func() taskOutcome { return taskOutcome{value: 7} }); o.err != nil || o.value != 7 {
		t.Errorf("runTask（正常）= %+v，want value=7 err=nil", o)
	}
}

// TestDrainTasksRunsAllInOrder：排空到底且保持投递顺序（外部脚本按序执行
// 动作的前提），并确认排空后 hasTasks 归零。
func TestDrainTasksRunsAllInOrder(t *testing.T) {
	win := &Window{}
	win.startLoop()

	var got []int
	for i := 0; i < 5; i++ {
		n := i
		if err := win.Post(func() { got = append(got, n) }); err != nil {
			t.Fatal(err)
		}
	}
	if !win.hasTasks() {
		t.Fatal("Post 之后队列应为非空")
	}
	win.drainTasks()
	if len(got) != 5 || got[0] != 0 || got[4] != 4 {
		t.Errorf("执行结果 = %v，want [0 1 2 3 4]", got)
	}
	if win.hasTasks() {
		t.Error("drainTasks 没有排空队列")
	}
}
