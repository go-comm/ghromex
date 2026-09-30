//go:build windows

package sdl2

import (
	"bytes"
	"os"
	"testing"
	"unsafe"

	"github.com/go-comm/ghromex/engine"
)

// 裁剪栈回归：PushClip/PopClip 与 SDL_RenderSetClipRect 的绑定语义
//（NULL=关闭、交集下推、栈空/空矩形=不裁剪），用 SDL_RenderGetClipRect
// 回读校验——绑定签名或偏移错误会在这里直接暴露。
func TestGraphicsClipStack(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(120, 80, "clip-test")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	getClip := bindSdl("SDL_RenderGetClipRect")
	read := func() sdlRect {
		var r sdlRect
		getClip(win.g.renderer, uintptr(unsafe.Pointer(&r)))
		return r
	}

	win.g.PushClip(10, 20, 30, 40)
	if got := read(); got != (sdlRect{10, 20, 30, 40}) {
		t.Fatalf("clip = %+v, want {10 20 30 40}", got)
	}

	// 嵌套求交：{0,10,15,60} ∩ {10,20,30,40} = {10,20,5,40}
	win.g.PushClip(0, 10, 15, 60)
	if got := read(); got != (sdlRect{10, 20, 5, 40}) {
		t.Fatalf("nested clip = %+v, want {10 20 5 40}（交集计算错误）", got)
	}

	win.g.PopClip()
	if got := read(); got != (sdlRect{10, 20, 30, 40}) {
		t.Fatalf("after pop = %+v, want {10 20 30 40}", got)
	}

	// 栈空：SDL_RenderSetClipRect(NULL) 关闭裁剪（GetClipRect 返回空矩形）
	win.g.PopClip()
	if got := read(); got.w != 0 || got.h != 0 {
		t.Fatalf("disabled clip = %+v, want empty (0×0)", got)
	}
	// 多余 PopClip 无害
	win.g.PopClip()
	if got := read(); got.w != 0 || got.h != 0 {
		t.Fatalf("clip after extra pop = %+v, want empty", got)
	}

	// Clear：清空裁剪栈并同步关闭（防止上一帧配对失衡污染后续绘制）
	win.g.PushClip(0, 0, 5, 5)
	win.g.Clear(0, 0, 0, 255)
	if got := read(); got.w != 0 || got.h != 0 {
		t.Fatalf("clip after Clear = %+v, want empty（Clear 未重置裁剪栈）", got)
	}
}

// 裁剪必须真正影响绘制结果：裁剪区内的像素落笔、区外同一次绘制的像素
// 不落笔。先无裁剪画红块校准当前像素格式下红/黑的字节序列（格式无关），
// 再对裁剪后的截图做逐字节比较——绑定到错误的 SDL 函数、或裁剪未应用到
// draw 时 GetClipRect 可能仍"看起来正确"，但此测试会失败。
func TestClipAffectsDrawing(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	const W, H = 120, 80
	win, err := NewWindow(W, H, "clip-draw-test")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	shot := func() []byte {
		buf, _, _, _, err := win.Screenshot()
		if err != nil {
			t.Fatalf("Screenshot: %v", err)
		}
		return buf
	}
	px := func(b []byte, x, y int) []byte {
		i := (y*W + x) * 4
		return b[i : i+4]
	}
	red := engine.NewColor(255, 0, 0, 255)

	// 校准：无裁剪画红块，记录红/黑两种像素在本格式下的字节序列
	win.g.Clear(0, 0, 0, 255)
	win.g.DrawColor(60, 10, 10, 10, red)
	redPat := append([]byte(nil), px(shot(), 65, 15)...)
	win.g.Clear(0, 0, 0, 255)
	blackPat := append([]byte(nil), px(shot(), 90, 40)...)
	if bytes.Equal(redPat, blackPat) {
		t.Fatalf("校准失败：红=%v 黑=%v，无法区分", redPat, blackPat)
	}

	// 裁剪 {10,10,20,20} 下画 50×50 红块
	win.g.Clear(0, 0, 0, 255)
	win.g.PushClip(10, 10, 20, 20)
	win.g.DrawColor(0, 0, 50, 50, red)
	win.g.PopClip()
	got := shot()

	if p := px(got, 15, 15); !bytes.Equal(p, redPat) {
		t.Errorf("裁剪区内 (15,15) = %v, want 红 %v", p, redPat)
	}
	// 区外两点都在本次 50×50 绘制范围内，但必须保持黑底
	if p := px(got, 45, 15); !bytes.Equal(p, blackPat) {
		t.Errorf("裁剪区外 (45,15) = %v, want 黑 %v（裁剪未影响绘制）", p, blackPat)
	}
	if p := px(got, 15, 45); !bytes.Equal(p, blackPat) {
		t.Errorf("裁剪区外 (15,45) = %v, want 黑 %v（裁剪未影响绘制）", p, blackPat)
	}
}
