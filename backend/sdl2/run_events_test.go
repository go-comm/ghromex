//go:build windows

package sdl2

import (
	"fmt"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/go-comm/ghromex/engine"
)

// 端到端事件链测试：SDL_PushEvent 注入真实 SDL 事件（点击聚焦 →
// SDL_TEXTINPUT 上屏 → KEYDOWN Backspace 删除），由 Run 的完整事件
// 循环消费。补此前"探针直接调 engine API、绕过 SDL 事件层"的盲区——
// 真机"有光标无法输入"正是这一层缺 SDL_StartTextInput。
func TestRunConsumesInjectedEvents(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(300, 200, "evt-test")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

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

	pushMouseButtonUp(cx, cy)
	pushTextInput("ab中")
	pushKeyDown(0x08) // SDLK_BACKSPACE

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

func pushEvent(buf *[eventBufferSize]byte) {
	// SDL_PushEvent（SDL_events.h 文档）：返回 1=成功、0=被事件过滤器丢弃、
	// 负数=失败。Windows x64 下 int 返回值只保证低 32 位有效，先截 int32 判。
	if r := int32(sdlPushEvent(uintptr(unsafe.Pointer(&buf[0])))); r < 0 {
		panic(fmt.Sprintf("SDL_PushEvent failed: ret=%d err=%s", r, lastError()))
	} else if r == 0 {
		panic("SDL_PushEvent filtered: 事件被过滤，测试前提不成立")
	}
}

func pushMouseButtonUp(x, y int) {
	var buf [eventBufferSize]byte
	put32(buf[0:], eventMouseButtonUp)
	buf[16] = mouseButtonLeft
	buf[17] = 1 // SDL_RELEASED
	put32(buf[20:], uint32(x))
	put32(buf[24:], uint32(y))
	pushEvent(&buf)
}

func pushTextInput(s string) {
	var buf [eventBufferSize]byte
	put32(buf[0:], eventTextInput)
	copy(buf[12:], s) // char text[32]，尾部天然 NUL
	pushEvent(&buf)
}

func pushKeyDown(sym uint32) {
	var buf [eventBufferSize]byte
	put32(buf[0:], eventKeyDown)
	buf[12] = 1 // SDL_PRESSED
	put32(buf[20:], sym)
	pushEvent(&buf)
}

// 端到端滚轮事件链：注入 SDL_MOUSEWHEEL → decodeEvent → Run 消费 →
// OnDocumentWheel 滚文档 → ChangeCount 变化触发重排 → 元素 rect 平移。
// dummy 驱动不会自发产生滚轮（无真实指针），必须注入验证整条链路。
func TestRunConsumesInjectedWheel(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(300, 200, "wheel-test")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

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

	// 滚轮向下（y 负 = 朝用户）→ 引擎 dy>0 → scrollTop +40 → 内容上移 40px
	pushMouseWheel(150, 100, 0, -1)

	win.SetAutoExit(300 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	after := p.GetBoundingClientRect().Top().Pixel()
	if after != before-40 {
		t.Fatalf("rect top %d → %d, want %d（wheel 事件链断链：解码/消费/重排之一缺失）",
			before, after, before-40)
	}
}

func pushMouseWheel(x, y int, wx, wy int32) {
	var buf [eventBufferSize]byte
	put32(buf[0:], eventMouseWheel)
	put32(buf[16:], uint32(wx))
	put32(buf[20:], uint32(wy))
	put32(buf[36:], uint32(x)) // mouseX/mouseY（SDL ≥2.26，DLL 2.28 ✓）
	put32(buf[40:], uint32(y))
	pushEvent(&buf)
}

// 端到端：滚轮落在「可滚长页上的 textarea」时，位移必须被 textarea 抢占，
// 文档级滚动不得同时发生。文档滚动的可观测面 = #tall 顶层 rect 平移；
// textarea 滚动的可观测面 = ChangeCount 变化（onChanged 触发）而外层
// rect 不动。补引擎白盒之外的盲区：真实 SDL 事件坐标（mouseX/Y@36/40）
// → ElementAt 命中 textarea → wheelScrollTextarea 消费整段位移。
func TestRunWheelOverTextareaKeepsDocumentStill(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(300, 200, "wheel-ta-test")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

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
	pushMouseWheel(cx, cy, 0, -1)
	pushMouseWheel(cx, cy, 0, -1)

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
