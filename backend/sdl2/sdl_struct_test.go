//go:build windows

package sdl2

import (
	"runtime"
	"testing"
	"unsafe"
)

// 绑定层的结构体大小必须与**运行库实际写出的字节数**一致。
//
// SDL 的结构体是按值跨越 DLL 边界的：Go 侧申请一块内存交给 SDL 写，SDL 按
// 自己的结构体大小写。Go 侧估小了 → SDL 越界写坏调用者的栈（静默地改掉
// 相邻的局部变量，随后空指针崩溃）；估大了 → 只是浪费，不致命。
//
// 早先的实例：SDL_RendererInfo 被按「name + flags + num_texture_formats +
// 指针」估成 32/40 字节，而运行库写的是 88 字节（texture_formats 是**内联
// 数组** Uint32[16]，不是指针）——越界 48 字节，调用之后渲染器句柄变成 0，
// 随后空指针崩溃。
//
// 这里用「把缓冲区填成 0xAA，再看最后一个被改写的字节」量出运行库的真实
// 写入长度，并断言它不超过 Go 侧结构体的大小。缓冲区放在**堆**上：栈上会
// 有别的帧残留，量出来的「最后一个非 0xAA 字节」不可信。

// writeProbe 是一块「填了 0xAA 的堆内存」，用来量运行库写了多少字节。
type writeProbe struct {
	buf []byte
}

func newWriteProbe(n int) *writeProbe {
	p := &writeProbe{buf: make([]byte, n)}
	for i := range p.buf {
		p.buf[i] = 0xAA
	}
	return p
}

// written 返回运行库实际写到的最大字节偏移（+1）。
func (p *writeProbe) written() int {
	last := 0
	for i, b := range p.buf {
		if b != 0xAA {
			last = i + 1
		}
	}
	return last
}

// TestStructSizesMatchRuntime 逐个核对按值跨越 DLL 边界的结构体。
func TestStructSizesMatchRuntime(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	win, err := NewWindow(200, 200, "struct-audit")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	// 1) SDL_RendererInfo：Go 侧结构体大小 vs 运行库写入长度。
	p := newWriteProbe(256)
	if rc := sdlGetRendererInfo(win.renderer, uintptr(unsafe.Pointer(&p.buf[0]))); rc != 0 {
		t.Fatalf("SDL_GetRendererInfo rc=%d", rc)
	}
	goSize := int(unsafe.Sizeof(sdlRendererInfo{}))
	if w := p.written(); w > goSize {
		t.Errorf("SDL_GetRendererInfo 写了 %d 字节，Go 侧 SDL_RendererInfo 只有 %d 字节：越界写坏栈",
			w, goSize)
	} else {
		t.Logf("SDL_RendererInfo: 运行库写 %d 字节，Go 侧 %d 字节", w, goSize)
	}

	// 2) SDL_Event：轮询事件（仓库用 56 字节缓冲接）。
	p = newWriteProbe(256)
	if sdlPollEvent(uintptr(unsafe.Pointer(&p.buf[0]))) != 0 {
		t.Fatalf("SDL_PollEvent 失败")
	}
	if w := p.written(); w > eventBufferSize {
		t.Errorf("SDL_PollEvent 写了 %d 字节，事件缓冲只有 %d 字节：越界写坏栈", w, eventBufferSize)
	} else {
		t.Logf("SDL_Event: 运行库写 %d 字节，缓冲 %d 字节", w, eventBufferSize)
	}

	// 3) SDL_SysWMinfo：取原生窗口句柄（Windows 分支）。
	p = newWriteProbe(256)
	var ver [3]uint8
	sdlGetVersion(uintptr(unsafe.Pointer(&ver[0])))
	runtime.KeepAlive(&ver[0])
	// version 必须按 SDL_VERSION 宏填，否则 SDL_GetWindowWMInfo 直接失败。
	copy(p.buf[0:3], ver[:])
	if sdlGetWindowWMInfo(win.win, uintptr(unsafe.Pointer(&p.buf[0]))) == 0 {
		t.Errorf("SDL_GetWindowWMInfo 失败（version 填法不对？）")
	}
	goSize = int(unsafe.Sizeof(sdlSysWMinfo{}))
	if w := p.written(); w > goSize {
		t.Errorf("SDL_GetWindowWMInfo 写了 %d 字节，Go 侧 SDL_SysWMinfo 只有 %d 字节：越界写坏栈",
			w, goSize)
	} else {
		t.Logf("SDL_SysWMinfo: 运行库写 %d 字节，Go 侧 %d 字节", w, goSize)
	}
}

// TestSDLVersionRecord 记录运行库的版本号——结构体布局跟版本相关（不同
// 小版本的 SDL_SysWMinfo.version 从 SDL_version 改成了 Uint32 等），排查
// 布局问题时先对版本。
func TestSDLVersionRecord(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	var v [3]uint8
	sdlGetVersion(uintptr(unsafe.Pointer(&v[0])))
	runtime.KeepAlive(&v[0])
	t.Logf("SDL version = %d.%d.%d", v[0], v[1], v[2])
}
