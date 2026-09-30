//go:build windows

package sdl2

import (
	"math"
	"testing"
)

// 回归测试：decodeEvent 的字段偏移以 .temp/eventprobe 对 SDL 2.28.4
// （dummy 驱动真实产生的事件）实测为准，防止再次按记忆/文档改错。
// 键盘/文本事件 dummy 不会产生（需 OS 输入），偏移按 SDL2 头文件
// SDL_keyboard.h / SDL_events.h 结构定义锁定（ABI 自 2.0.0 未变）。
// 事件类型值同样经 SDL_events.h 原文核对：SDL_TEXTINPUT=0x303（曾凭
// 记忆错标 0x400=SDL_MOUSEMOTION，decode 单测与常量共用错值是自闭环、
// 抓不出来——由 run_events_test.go 端到端注入 + 真机敲字双重兜底）。

func TestDecodeKeyDownEvent(t *testing.T) {
	var buf [eventBufferSize]byte
	// SDL_KeyboardEvent: type@0 ts@4 windowID@8 state@12 repeat@13
	// keysym{scancode@16, sym@20, mod@24}
	put32(buf[0:], eventKeyDown)
	put32(buf[4:], 789)
	put32(buf[8:], 1)
	buf[12] = 1           // SDL_PRESSED
	buf[13] = 1           // repeat
	put32(buf[16:], 42)   // scancode
	put32(buf[20:], 0x08) // SDLK_BACKSPACE
	put32(buf[24:], 0)    // mod+unused

	ev := decodeEvent(&buf)
	if ev.sym != 0x08 {
		t.Fatalf("sym = 0x%X, want 0x8（读错 keysym.sym 偏移）", ev.sym)
	}
}

func TestDecodeTextInputEvent(t *testing.T) {
	var buf [eventBufferSize]byte
	// SDL_TextInputEvent: type@0 ts@4 windowID@8 text[32]@12（UTF-8 NUL 结尾）
	put32(buf[0:], eventTextInput)
	put32(buf[4:], 999)
	put32(buf[8:], 1)
	copy(buf[12:], []byte("A中")) // 9 字节 + NUL

	ev := decodeEvent(&buf)
	if ev.text != "A中" {
		t.Fatalf("text = %q, want A中", ev.text)
	}
}

func TestDecodeMouseButtonEvent(t *testing.T) {
	var buf [eventBufferSize]byte
	// SDL_MouseButtonEvent: type@0 ts@4 windowID@8 which@12 button@16 state@17 x@20 y@24
	put32(buf[0:], eventMouseButtonUp)
	put32(buf[4:], 1234) // timestamp（若误当 padding 读 x 会偏移 4 字节）
	put32(buf[8:], 1)    // windowID
	put32(buf[12:], 0)   // which（Uint32，真机默认鼠标为 0）
	buf[16] = mouseButtonLeft
	buf[17] = 1 // SDL_RELEASED
	put32(buf[20:], 432)
	put32(buf[24:], 190)

	ev := decodeEvent(&buf)
	if ev.typ != eventMouseButtonUp {
		t.Fatalf("typ = 0x%X", ev.typ)
	}
	if ev.button != mouseButtonLeft {
		t.Fatalf("button = %d, want 1（把 which 当成了 button）", ev.button)
	}
	if ev.state != 1 {
		t.Fatalf("state = %d", ev.state)
	}
	if ev.x != 432 || ev.y != 190 {
		t.Fatalf("x,y = %d,%d want 432,190", ev.x, ev.y)
	}
}

func TestDecodeWindowEvent(t *testing.T) {
	var buf [eventBufferSize]byte
	// SDL_WindowEvent: type@0 ts@4 windowID@8 event@12 data1@16 data2@20
	put32(buf[0:], eventWindow)
	put32(buf[4:], 275)
	put32(buf[8:], 1)
	buf[12] = windowEventSizeChanged
	put32(buf[16:], 250) // 实测：SetWindowSize(250,180) 后 data1=250
	put32(buf[20:], 180)

	ev := decodeEvent(&buf)
	if ev.windowEvent != windowEventSizeChanged {
		t.Fatalf("windowEvent = %d, want 6（实测 SIZE_CHANGED=6 在偏移 12）", ev.windowEvent)
	}
	if ev.data1 != 250 || ev.data2 != 180 {
		t.Fatalf("data = %d,%d want 250,180", ev.data1, ev.data2)
	}
}

func TestDecodeMouseWheelEvent(t *testing.T) {
	var buf [eventBufferSize]byte
	// SDL_MouseWheelEvent（SDL_events.h，44 字节）：type@0 ts@4 windowID@8
	// which@12 x@16 y@20 direction@24 preciseX@28 preciseY@32 mouseX@36 mouseY@40
	put32(buf[0:], eventMouseWheel)
	put32(buf[4:], 777)
	put32(buf[8:], 1)
	put32(buf[12:], 0)
	put32(buf[16:], 1) // x 正 = 向右
	down := int32(-2)
	put32(buf[20:], uint32(down)) // y 负 = 滚轮朝用户（向下）
	put32(buf[24:], 0)            // SDL_MOUSEWHEEL_NORMAL
	put32(buf[36:], 111)          // mouseX（SDL ≥2.26）
	put32(buf[40:], 222)          // mouseY

	ev := decodeEvent(&buf)
	if ev.typ != eventMouseWheel {
		t.Fatalf("typ = 0x%X, want 0x%X（0x403 读错）", ev.typ, eventMouseWheel)
	}
	if ev.wheelX != 1 || ev.wheelY != -2 {
		t.Fatalf("wheel = (%d,%d), want (1,-2)（读错 x/y 或符号）", ev.wheelX, ev.wheelY)
	}
	if ev.x != 111 || ev.y != 222 {
		t.Fatalf("mouse = (%d,%d), want (111,222)（读错 mouseX/mouseY 偏移）", ev.x, ev.y)
	}
}

func TestDecodeMouseWheelFlipped(t *testing.T) {
	// SDL_MOUSEWHEEL_FLIPPED（Windows"自然滚动"）：x/y 取反即还原
	var buf [eventBufferSize]byte
	put32(buf[0:], eventMouseWheel)
	put32(buf[16:], 1)
	three := int32(3)
	put32(buf[20:], uint32(three))
	put32(buf[24:], wheelDirectionFlipped)

	ev := decodeEvent(&buf)
	if ev.wheelX != -1 || ev.wheelY != -3 {
		t.Fatalf("wheel = (%d,%d), want (-1,-3)（FLIPPED 未翻转）", ev.wheelX, ev.wheelY)
	}
}

func TestDecodeMouseWheelPreciseFallback(t *testing.T) {
	// 整数档位为 0（触控板平滑滚动只给 precise float）时回退 preciseX/preciseY
	var buf [eventBufferSize]byte
	put32(buf[0:], eventMouseWheel)
	put32(buf[28:], math.Float32bits(0.4))
	put32(buf[32:], math.Float32bits(-1.5))

	ev := decodeEvent(&buf)
	// round(0.4)=0 → 保持 0；round(-1.5)=-2（Go math.Round 半数远离零）
	if ev.wheelX != 0 || ev.wheelY != -2 {
		t.Fatalf("wheel = (%d,%d), want (0,-2)（precise 回退失败）", ev.wheelX, ev.wheelY)
	}
}

func put32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
