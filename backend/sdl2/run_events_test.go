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
