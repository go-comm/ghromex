//go:build windows

package sdl2

import "testing"

// 回归测试：decodeEvent 的字段偏移以 .temp/eventprobe 对 SDL 2.28.4
// （dummy 驱动真实产生的事件）实测为准，防止再次按记忆/文档改错。

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

func put32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
