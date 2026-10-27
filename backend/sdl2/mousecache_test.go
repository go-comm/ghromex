//go:build windows

package sdl2

import (
	"encoding/binary"
	"testing"
)

// SDL2 的按钮事件不读 WM_LBUTTONDOWN 的 lParam，直接用内部缓存
// mouse->x/y，而缓存只在 motion 里刷新（.temp/mousecache 用 SendMessage
// 实测：motion(400,300) → MoveTo(350,250) → 按下事件带 (400,300)，即使
// lParam 给的是新窗口系的 (50,50)）。于是「窗口移动后、下一条 motion
// 之前」的按下坐标相对的是旧窗口位置——snip 截图后原地缩窗，第一次点击
// 因此落到错误的窗口坐标上（表现为「必须先点一下才拖得动」）。后端解码
// 层按「缓存原点相对当前位置」的位移把它平移回来。

func TestStaleButtonMovedToCurrentWindow(t *testing.T) {
	win := &Window{w: 200, h: 150, posX: 0, posY: 0, posKnown: true}

	// 旧位置 (0,0) 下的一条 motion：缓存原点 = (0,0)。
	win.noteMotion()

	// 窗口移到 (350,250)：SDL 不会补发 motion，缓存仍相对 (0,0)。
	// （用 applyMoved 走 MOVED 事件的同一条路径；MoveTo 在无 SDL 句柄时
	//  提前返回，单元测试不建窗。）
	win.applyMoved(350, 250)

	// 按钮带缓存坐标 (400,300)，应平移成新窗口系的 (50,50)。
	if x, y := win.fixStale(400, 300); x != 50 || y != 50 {
		t.Fatalf("移动后的首击坐标 = (%d,%d), want (50,50)（缓存没按窗口位移平移）", x, y)
	}

	// 真实 motion 到达：缓存原点跟着新位置，平移差归零。
	win.noteMotion()
	if x, y := win.fixStale(60, 61); x != 60 || y != 61 {
		t.Fatalf("motion 刷新后坐标 = (%d,%d), want (60,61)（新缓存又被平移了一次）", x, y)
	}
}

func TestStaleButtonUnchangedWithoutOrigin(t *testing.T) {
	// 还没观测到任何 motion：缓存相对的哪个位置无从推断，保持原样
	// （宁可不动，也不能猜错方向）。
	win := &Window{w: 100, h: 100, posKnown: true}
	if x, y := win.fixStale(7, 8); x != 7 || y != 8 {
		t.Fatalf("origin 未知时坐标被平移: (%d,%d)", x, y)
	}
}

func TestStaleButtonUnchangedWithoutPos(t *testing.T) {
	// Centered 建窗且 MOVED 还没到：位置未知，同样保持原样。
	win := &Window{w: 100, h: 100}
	win.noteMotion() // posKnown=false → originKnown 也应保持 false
	if win.originKnown {
		t.Fatal("位置未知时 noteMotion 不该把 originKnown 置真")
	}
	if x, y := win.fixStale(7, 8); x != 7 || y != 8 {
		t.Fatalf("pos 未知时坐标被平移: (%d,%d)", x, y)
	}
}

// 端到端走 decodeEvent+toPublicEvent：motion 写原点 → MOVED 更新位置 →
// 按钮坐标平移。注入路径（InjectEvent）不经过这条链，所以这里手搓字节。
func TestToPublicEventStaleButtonSequence(t *testing.T) {
	win := &Window{w: 800, h: 600, posX: 0, posY: 0, posKnown: true}

	// 1) motion(400,300)：相对旧位置 (0,0) 的缓存刷新。
	var buf [eventBufferSize]byte
	binary.LittleEndian.PutUint32(buf[0:4], eventMouseMotion)
	binary.LittleEndian.PutUint32(buf[20:24], 400)
	binary.LittleEndian.PutUint32(buf[24:28], 300)
	ev := win.toPublicEvent(decodeEvent(&buf))
	if ev.Kind != EventMouseMotion || ev.X != 400 || ev.Y != 300 {
		t.Fatalf("motion 解码 = kind%d (%d,%d), want %d (400,300)",
			ev.Kind, ev.X, ev.Y, EventMouseMotion)
	}

	// 2) SDL_WINDOWEVENT_MOVED(350,250)：位置基准更新，对外仍是未知事件。
	buf = [eventBufferSize]byte{}
	binary.LittleEndian.PutUint32(buf[0:4], eventWindow)
	buf[12] = windowEventMoved
	binary.LittleEndian.PutUint32(buf[16:20], 350)
	binary.LittleEndian.PutUint32(buf[20:24], 250)
	if ev = win.toPublicEvent(decodeEvent(&buf)); ev.Kind != EventUnknown {
		t.Fatalf("MOVED 应返回 EventUnknown（不能误当退出），got kind%d", ev.Kind)
	}
	if win.posX != 350 || win.posY != 250 || !win.posKnown {
		t.Fatalf("MOVED 没更新位置基准: (%d,%d) known=%v", win.posX, win.posY, win.posKnown)
	}

	// 3) 按下：事件体里的 x/y 是缓存值 (400,300)（相对旧位置），
	//    解码出来必须是新窗口系的 (50,50)。
	buf = [eventBufferSize]byte{}
	binary.LittleEndian.PutUint32(buf[0:4], eventMouseButtonDown)
	buf[16] = MouseButtonLeft
	buf[17] = 1
	binary.LittleEndian.PutUint32(buf[20:24], 400)
	binary.LittleEndian.PutUint32(buf[24:28], 300)
	ev = win.toPublicEvent(decodeEvent(&buf))
	if ev.Kind != EventMouseButtonDown || ev.X != 50 || ev.Y != 50 {
		t.Fatalf("首击解码 = kind%d (%d,%d), want %d (50,50)", ev.Kind, ev.X, ev.Y, EventMouseButtonDown)
	}
}
