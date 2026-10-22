//go:build windows

// 伪类状态的端到端链路测试：鼠标移动/按下/抬起/离开 → 引擎状态 →
// 主循环重排 → 样式更新。
//
// 这一层在 browser（主循环所在处）：后端只解码事件，事件→引擎动作的
// 翻译在 browser，所以「鼠标事件真的能驱动 :hover」也在这里测。
package browser

import (
	"testing"
	"time"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/engine"
)

// hoverPage 是一张会因伪类状态改变几何的页面。
const hoverPage = `<!doctype html><html><head><style>
	body{margin:0}
	.box{display:block;width:60px;height:40px;background:#EEEEEE}
	.box:hover{width:200px}
	.box:active{height:120px}
</style></head><body><div class="box" id="b"></div></body></html>`

// boxWidth 读盒子宽度。
func boxWidth(t *testing.T, e engine.HTMLElement) int {
	t.Helper()
	r := e.GetBoundingClientRect()
	return r.Right().Pixel() - r.Left().Pixel()
}

// boxHeight 读盒子高度。
func boxHeight(t *testing.T, e engine.HTMLElement) int {
	t.Helper()
	r := e.GetBoundingClientRect()
	return r.Bottom().Pixel() - r.Top().Pixel()
}

// 鼠标移动到元素上 → :hover 生效（宽度变化），移开后还原。
func TestRunUpdatesHoverState(t *testing.T) {
	win := newTestWindow(t, 300, 200, "hover-test")
	doc, err := win.OpenDocument(hoverPage)
	if err != nil {
		t.Fatal(err)
	}
	box := doc.QuerySelector("#b")
	if box == nil {
		t.Fatal("box not found")
	}
	if w := boxWidth(t, box); w != 60 {
		t.Fatalf("初始宽 = %d, want 60", w)
	}

	mouseMotion(win, 30, 20)
	win.SetAutoExit(200 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if w := boxWidth(t, box); w != 200 {
		t.Fatalf("悬停后宽 = %d, want 200（:hover 未生效）", w)
	}

	// 鼠标离开窗口 → 悬停态清除，样式还原。
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventMouseLeave})
	win.SetAutoExit(200 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if w := boxWidth(t, box); w != 60 {
		t.Fatalf("离开后宽 = %d, want 60（悬停态未清除）", w)
	}
}

// 鼠标按下 → :active 生效；抬起 → 还原。
func TestRunUpdatesActiveState(t *testing.T) {
	win := newTestWindow(t, 300, 200, "active-test")
	doc, err := win.OpenDocument(hoverPage)
	if err != nil {
		t.Fatal(err)
	}
	box := doc.QuerySelector("#b")
	if box == nil {
		t.Fatal("box not found")
	}
	if h := boxHeight(t, box); h != 40 {
		t.Fatalf("初始高 = %d, want 40", h)
	}

	mouseMotion(win, 30, 20)
	mouseButtonDown(win, 30, 20)
	win.SetAutoExit(200 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if h := boxHeight(t, box); h != 120 {
		t.Fatalf("按下后高 = %d, want 120（:active 未生效）", h)
	}

	mouseButtonUp(win, 30, 20)
	win.SetAutoExit(200 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if h := boxHeight(t, box); h != 40 {
		t.Fatalf("抬起后高 = %d, want 40（:active 未还原）", h)
	}
}

// 点击按钮 → 获得焦点 → :focus 生效。
func TestRunUpdatesFocusState(t *testing.T) {
	win := newTestWindow(t, 300, 200, "focus-test")
	const src = `<!doctype html><html><head><style>
		body{margin:0}
		button{display:block;width:60px;height:40px}
		button:focus{width:220px}
	</style></head><body><button id="b">go</button></body></html>`
	doc, err := win.OpenDocument(src)
	if err != nil {
		t.Fatal(err)
	}
	btn := doc.QuerySelector("#b")
	if btn == nil {
		t.Fatal("button not found")
	}
	r := btn.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	mouseMotion(win, cx, cy)
	mouseButtonDown(win, cx, cy)
	mouseButtonUp(win, cx, cy)
	win.SetAutoExit(200 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
	if f := engine.FocusedElement(doc); f == nil {
		t.Fatal("点击按钮未获得焦点")
	}
	if w := boxWidth(t, btn); w != 220 {
		t.Fatalf("聚焦后宽 = %d, want 220（:focus 未生效）", w)
	}
}
