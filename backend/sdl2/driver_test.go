//go:build windows

package sdl2

import (
	"testing"

	"github.com/go-comm/ghromex/core"
)

// 驱动名要能读出来（诊断用：软件路径的瓶颈是填充率，GPU 路径的瓶颈通常
// 在纹理上传/draw call/swap 间隔——排查方向不同，先确认走的是哪条路）。
//
// 同时钉住**没写坏栈**：SDL_GetRendererInfo 在本仓库随附的 SDL2.dll 上
// 实际写 88 字节（比 SDL2 头文件里的 32 字节多），缓冲区按 32/40 字节申请
// 就会越界——现场症状是调用之后渲染器句柄变成 0、随后空指针崩溃。
func TestRendererDriverNameReadable(t *testing.T) {
	win, err := NewWindow(200, 200, "driver-name")
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()

	name := RendererDriver(win.renderer)
	if name == "" {
		t.Fatal("驱动名为空：SDL_GetRendererInfo 没读到 name")
	}
	t.Logf("driver = %q", name)

	// 调用之后渲染器句柄必须还在（越界写会把它清零）。
	if win.renderer == 0 {
		t.Fatal("调用 RendererDriver 后渲染器句柄变成 0：缓冲区越界写坏了栈")
	}
	// 还能正常画：渲染器没被破坏。
	win.g.DrawColor(0, 0, 4, 4, core.NewColor(255, 0, 0, 255))
}
