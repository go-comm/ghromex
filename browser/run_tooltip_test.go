//go:build windows

// 悬停提示气泡的端到端链路：注入鼠标移动 → 主循环 TickTooltip 计时 → 超过
// 延时气泡弹出（engine.TooltipText 可读）→ 鼠标离开窗口悬停清除 → 收起。
//
// 这一层测的是主循环接线：TickTooltip 的每帧推进与「计时在途不休眠」都挂
// 在 Run 里，引擎白盒测试拨的是自己的计时字段，测不到帧循环真的在转。
package browser

import (
	"testing"
	"time"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/engine"
)

const tooltipRunPage = `<!doctype html><html><head><style>
	body{margin:0}
	#tool{display:block;width:120px;height:40px;background:#EEEEEE}
</style></head><body><div id="tool" title="保存  Ctrl+S"></div></body></html>`

func TestRunShowsTooltipOnHover(t *testing.T) {
	win := newTestWindow(t, 300, 200, "tooltip-test")
	doc, err := win.OpenDocument(tooltipRunPage)
	if err != nil {
		t.Fatal(err)
	}
	// 先空跑一段把平台自身的首帧事件排空：dummy 驱动在窗口显示时会把鼠标
	// 「瞬移」到窗口中心，那条鼠标移动会在注入事件之后到达，把悬停态清掉
	//（见 TestRunUpdatesHoverState 的同类处理）。
	win.SetAutoExit(120 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if got := engine.TooltipText(doc); got != "" {
		t.Fatalf("还没悬停就有气泡：%q", got)
	}

	mouseMotion(win, 60, 20) // #tool 中心
	win.SetAutoExit(900 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if got := engine.TooltipText(doc); got != "保存  Ctrl+S" {
		t.Fatalf("悬停 900ms（> 600ms 延时）后气泡应弹出，got %q", got)
	}

	// 鼠标离开窗口 → 悬停清除 → 气泡在计时里收起。
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventMouseLeave})
	win.SetAutoExit(200 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if got := engine.TooltipText(doc); got != "" {
		t.Fatalf("鼠标离开后气泡应收起，got %q", got)
	}
}
