//go:build windows

// 端到端事件链测试：往事件队列注入真实平台事件（点击聚焦 →
// 文本输入上屏 → KEYDOWN Backspace 删除），由主循环完整消费。
//
// 补此前「探针直接调 engine API、绕过平台事件层」的盲区——真机
// 「有光标无法输入」正是这一层缺 StartTextInput。
//
// 这一层在 browser（主循环所在处）：后端只解码事件，事件→引擎动作的
// 翻译在 browser，所以端到端链路也在这里测。
package browser

import (
	"os"
	"testing"
	"time"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/runtime/clipboard"
)

// newTestWindow 建一个用 dummy 驱动的窗口（无显示器也能跑）。
func newTestWindow(t *testing.T, w, h int, title string) *Window {
	t.Helper()
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	t.Cleanup(func() { os.Unsetenv("SDL_VIDEO_DRIVER") })
	win, err := New(w, h, title)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(win.Close)
	return win
}

// mouseButtonUp 注入一次左键抬起（点击的抬起半边）。
func mouseButtonUp(win *Window, x, y int) {
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventMouseButtonUp, Button: sdl2.MouseButtonLeft, X: x, Y: y})
}

// mouseButtonDown 注入一次左键按下。
func mouseButtonDown(win *Window, x, y int) {
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventMouseButtonDown, Button: sdl2.MouseButtonLeft, X: x, Y: y})
}

// mouseMotion 注入一次鼠标移动。
func mouseMotion(win *Window, x, y int) {
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventMouseMotion, X: x, Y: y})
}

// mouseWheel 注入一档滚轮（wy 为负 = 朝用户）。
func mouseWheel(win *Window, x, y, wx, wy int) {
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventMouseWheel, X: x, Y: y, WheelX: wx, WheelY: wy})
}

// textInput 注入一段文本输入。
func textInput(win *Window, s string) {
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventTextInput, Text: s})
}

// keyDown 注入一次按键（可带修饰键）。
func keyDown(win *Window, key int, mod uint16) {
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventKeyDown, Key: key, Mod: mod})
}

func TestRunConsumesInjectedEvents(t *testing.T) {
	win := newTestWindow(t, 300, 200, "evt-test")

	const src = `<!doctype html><html><head><style>` +
		`body{margin:0}` +
		`.in{display:inline-block;width:180px;padding:6px;border:1px solid #CBD5E1}` +
		`</style></head><body><div><input class="in" id="a" type="text"></div></body></html>`
	doc, err := win.OpenDocument(src)
	if err != nil {
		t.Fatal(err)
	}
	input := doc.QuerySelector("#a")
	if input == nil {
		t.Fatal("input not found")
	}
	r := input.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	mouseButtonUp(win, cx, cy)
	textInput(win, "ab中")
	keyDown(win, 0x08, 0) // SDLK_BACKSPACE

	win.SetAutoExit(300 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	if f := engine.FocusedElement(doc); f == nil {
		t.Fatal("click event did not focus input（鼠标事件解码/分发断链）")
	}
	if v := input.GetAttribute("value"); v != "ab" {
		t.Fatalf("value = %q, want %q（TextInput/KeyDown 事件链断链）", v, "ab")
	}
}

func TestRunConsumesInjectedDragSelection(t *testing.T) {
	win := newTestWindow(t, 320, 200, "drag-test")

	const src = `<!doctype html><html><head><style>` +
		`body{margin:0}` +
		`.in{display:inline-block;width:260px;padding:6px;border:1px solid #CBD5E1;font-size:16px}` +
		`</style></head><body><div><input class="in" id="a" type="text" value="hello world"></div></body></html>`
	doc, err := win.OpenDocument(src)
	if err != nil {
		t.Fatal(err)
	}
	input := doc.QuerySelector("#a")
	if input == nil {
		t.Fatal("input not found")
	}
	r := input.GetBoundingClientRect()
	x1 := r.Left().Pixel() + 2
	x2 := r.Right().Pixel() - 2
	y := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	// 按下→移动（超过拖选阈值）→抬起：走完整事件链。
	mouseButtonDown(win, x1, y)
	mouseMotion(win, x2, y)
	mouseButtonUp(win, x2, y)

	win.SetAutoExit(250 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	got, ok := engine.SelectedText(doc)
	if !ok || got != "hello world" {
		t.Fatalf("拖选结果 = %q, ok=%v, want %q（MouseMotion 解码/分发断链）", got, ok, "hello world")
	}
}

func TestRunConsumesSelectionKeys(t *testing.T) {
	win := newTestWindow(t, 320, 200, "ctrl-a-test")

	skipIfClipboardUnavailable(t)

	const src = `<!doctype html><html><head><style>` +
		`body{margin:0}` +
		`.in{display:inline-block;width:260px;padding:6px;border:1px solid #CBD5E1;font-size:16px}` +
		`</style></head><body><div><input class="in" id="a" type="text" value="hello world"></div></body></html>`
	doc, err := win.OpenDocument(src)
	if err != nil {
		t.Fatal(err)
	}
	input := doc.QuerySelector("#a")
	if input == nil {
		t.Fatal("input not found")
	}
	r := input.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	mouseButtonUp(win, cx, cy) // 先聚焦
	keyDown(win, 'a', sdl2.KmodCtrl)
	keyDown(win, 'c', sdl2.KmodCtrl)

	win.SetAutoExit(250 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	got, ok := engine.SelectedText(doc)
	if !ok || got != "hello world" {
		t.Fatalf("Ctrl+A 选区 = %q, ok=%v（mod 位未解码或未传入引擎）", got, ok)
	}
	// 从系统剪贴板读回，验证 Ctrl+C 的完整链路（含系统剪贴板写入）。
	if clip := clipboard.Text(); clip != "hello world" {
		t.Fatalf("剪贴板 = %q, want %q（Ctrl+C 未写入或写入内容错误）", clip, "hello world")
	}
}

// 端到端：Ctrl+V 粘贴 —— 系统剪贴板 → 平台剪贴板读取 → 引擎
// OnDocumentPaste → value。覆盖「平台剪贴板读写」这一段引擎测不到的链路。
func TestRunConsumesInjectedPaste(t *testing.T) {
	win := newTestWindow(t, 320, 200, "paste-test")

	skipIfClipboardUnavailable(t)
	clipboard.SetText("pasted text")
	// 剪贴板是全局独占资源：本进程里别的用例窗口还开着时，写入可能失败
	// （写不进去就用例读到的还是上一次的内容）。写完读回来核对，读不到就
	// 跳过——否则会把「剪贴板写不进去」误报成「粘贴链路断了」。
	if clipboard.Text() != "pasted text" {
		t.Skip("系统剪贴板写入失败（被其它窗口占用），跳过粘贴用例")
	}

	const src = `<!doctype html><html><head><style>` +
		`body{margin:0}` +
		`.in{display:inline-block;width:260px;padding:6px;border:1px solid #CBD5E1;font-size:16px}` +
		`</style></head><body><div><input class="in" id="a" type="text"></div></body></html>`
	doc, err := win.OpenDocument(src)
	if err != nil {
		t.Fatal(err)
	}
	input := doc.QuerySelector("#a")
	if input == nil {
		t.Fatal("input not found")
	}
	r := input.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	mouseButtonUp(win, cx, cy) // 先聚焦
	keyDown(win, 'v', sdl2.KmodCtrl)

	win.SetAutoExit(250 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	if v := input.GetAttribute("value"); v != "pasted text" {
		t.Fatalf("粘贴后 value = %q, want %q（剪贴板读取/引擎粘贴链路断链）", v, "pasted text")
	}
}

// 端到端：Shift+方向键扩展选区（KEYDOWN 的 mod 位 → 引擎按键处理）。
func TestRunConsumesInjectedShiftArrow(t *testing.T) {
	win := newTestWindow(t, 320, 200, "shift-arrow-test")

	const src = `<!doctype html><html><head><style>` +
		`body{margin:0}` +
		`.in{display:inline-block;width:260px;padding:6px;border:1px solid #CBD5E1;font-size:16px}` +
		`</style></head><body><div><input class="in" id="a" type="text" value="hello"></div></body></html>`
	doc, err := win.OpenDocument(src)
	if err != nil {
		t.Fatal(err)
	}
	input := doc.QuerySelector("#a")
	if input == nil {
		t.Fatal("input not found")
	}
	r := input.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2

	mouseButtonUp(win, cx, cy)               // 聚焦
	keyDown(win, 'a', sdl2.KmodCtrl)         // 全选（建立选区）
	keyDown(win, 0x40000050, sdl2.KmodShift) // Shift+Left：SDLK_LEFT

	win.SetAutoExit(250 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	got, ok := engine.SelectedText(doc)
	if !ok || got != "hell" {
		t.Fatalf("Shift+Left 后选区 = %q, ok=%v, want %q（mod 位未解码或扩展逻辑未生效）", got, ok, "hell")
	}
}

// RequestClose：应用层「退出」菜单项要能关掉窗口——压入退出事件后，
// 主循环应当按「用户点关闭按钮」的同一条路径退出。
func TestRequestCloseStopsRun(t *testing.T) {
	win := newTestWindow(t, 200, 120, "close-test")
	const src = `<!doctype html><html><head><style>body{margin:0}</style></head>
	<body><div style="height:400px;background-color:#DBEAFE"></div></body></html>`
	if _, err := win.OpenDocument(src); err != nil {
		t.Fatal(err)
	}

	// 不设 autoExit：只靠 RequestClose 退出，若事件没被消费，主循环会一直
	// 跑下去（测试会超时失败）。
	go func() {
		time.Sleep(200 * time.Millisecond)
		win.RequestClose()
	}()
	start := time.Now()
	if err := win.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("RequestClose 后主循环仍跑了 %v 才退出（退出事件没进事件循环）", el)
	}
}

// 端到端滚轮事件链：注入滚轮事件 → 后端解码 → 主循环消费 →
// OnDocumentWheel 滚文档 → ChangeCount 变化触发重排 → 元素 rect 平移。
// dummy 驱动不会自发产生滚轮（无真实指针），必须注入验证整条链路。
func TestRunConsumesInjectedWheel(t *testing.T) {
	win := newTestWindow(t, 300, 200, "wheel-test")

	const src = `<!doctype html><html><head><style>body{margin:0}</style></head>` +
		`<body><div id="p" style="height:800px;background-color:#2563EB"></div></body></html>`
	doc, err := win.OpenDocument(src)
	if err != nil {
		t.Fatal(err)
	}
	p := doc.QuerySelector("#p")
	if p == nil {
		t.Fatal("#p not found")
	}
	before := p.GetBoundingClientRect().Top().Pixel()
	if before != 0 {
		t.Fatalf("initial top = %d, want 0", before)
	}

	// 滚轮向下（y 负 = 朝用户）→ 引擎 dy>0 → scrollTop +100（每档 100px，
	// 与 Chrome/Windows 一致）→ 内容上移 100px；平滑滚动 150ms 收敛。
	mouseWheel(win, 150, 100, 0, -1)

	win.SetAutoExit(500 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	after := p.GetBoundingClientRect().Top().Pixel()
	if after != before-wheelStepPx {
		t.Fatalf("rect top %d → %d, want %d（wheel 事件链断链：解码/消费/重排之一缺失）",
			before, after, before-wheelStepPx)
	}
}

// 端到端：滚轮落在「可滚长页上的 textarea」时，位移必须被 textarea 抢占，
// 文档级滚动不得同时发生。文档滚动的可观测面 = #tall 顶层 rect 平移；
// textarea 滚动的可观测面 = ChangeCount 变化（onChanged 触发）而外层
// rect 不动。补引擎白盒之外的盲区：真实事件坐标（mouseX/Y）→
// ElementAt 命中 textarea → 引擎的 textarea 滚轮消费整段位移。
func TestRunWheelOverTextareaKeepsDocumentStill(t *testing.T) {
	win := newTestWindow(t, 300, 200, "wheel-ta-test")

	var val []byte
	val = append(val, `<!doctype html><html><head><style>body{margin:0}</style></head><body>
<div id="tall" style="height:800px;background-color:#DBEAFE"></div>
<textarea id="ta" style="position:absolute;left:10px;top:10px;width:150px;height:60px">`...)
	for i := 0; i < 20; i++ {
		val = append(val, []byte("row\n")...)
	}
	val = append(val, `</textarea></body></html>`...)

	doc, err := win.OpenDocument(string(val))
	if err != nil {
		t.Fatal(err)
	}
	tall := doc.QuerySelector("#tall")
	ta := doc.QuerySelector("#ta")
	if tall == nil || ta == nil {
		t.Fatal("#tall/#ta not found")
	}
	if top := tall.GetBoundingClientRect().Top().Pixel(); top != 0 {
		t.Fatalf("initial top = %d, want 0", top)
	}
	r := ta.GetBoundingClientRect()
	cx := (r.Left().Pixel() + r.Right().Pixel()) / 2
	cy := (r.Top().Pixel() + r.Bottom().Pixel()) / 2
	cc0 := doc.ChangeCount()

	// 两档滚轮都落在 textarea 中心：长页 contentH=800 > 视口 200，
	// 若位移漏到根元素，#tall top 会变成 -40/-80。
	mouseWheel(win, cx, cy, 0, -1)
	mouseWheel(win, cx, cy, 0, -1)

	win.SetAutoExit(300 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	if top := tall.GetBoundingClientRect().Top().Pixel(); top != 0 {
		t.Fatalf("#tall top = %d, want 0（滚 textarea 时文档也在滚：坐标命中文档链/位移未被 textarea 消费）", top)
	}
	if doc.ChangeCount() == cc0 {
		t.Fatal("ChangeCount 未变：textarea 没有滚动（滚轮位移被丢弃）")
	}
}

// skipIfClipboardUnavailable 在系统剪贴板不可用时跳过测试。
//
// Windows 的剪贴板是全局独占资源：某个常驻程序打开它却没关闭时，别的
// 进程读写都会失败（无桌面会话的 CI 环境更是完全没有）。写一段再读回，
// 读不回来说明这会儿用不了剪贴板，剪贴板相关的用例直接跳过。
func skipIfClipboardUnavailable(t *testing.T) {
	t.Helper()
	clipboard.SetText("system-ui-clipboard-probe")
	if clipboard.Text() != "system-ui-clipboard-probe" {
		t.Skip("系统剪贴板不可用（被其它进程占用或没有桌面会话），跳过剪贴板测试")
	}
}
