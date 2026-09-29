// Package browser 是面向应用层的装配门面：隐藏渲染后端
// （当前为 backend/sdl2）的选择与直接依赖，应用只需 import
// browser 与 engine，不再接触底层类型。
//
// API 按"可多次 New"塑形（每次返回独立窗口）；多窗口共存的
// 后端基础（SDL 子系统引用计数、windowID 事件路由、face 缓存
// 归属）待真实需求出现时在 backend 内解决，届时本层对外
// 签名不变。
package browser

import (
	"time"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/engine"
)

// Window 是一个可承载 HTML 文档的窗口，实现 engine.Viewport。
// 生命周期约定透传后端：New → OpenDocument → Run → Close，
// 且须在同一 goroutine（SDL 要求窗口/渲染器/事件循环同 OS 线程）。
type Window struct {
	sdl *sdl2.Window
}

// New 创建一个窗口。坐标体系 1:1（1 CSS px = 1 物理像素）。
func New(w, h int, title string) (*Window, error) {
	sdl, err := sdl2.NewWindow(w, h, title)
	if err != nil {
		return nil, err
	}
	return &Window{sdl: sdl}, nil
}

// ViewportWidth 实现 engine.Viewport。
func (win *Window) ViewportWidth() int { return win.sdl.ViewportWidth() }

// ViewportHeight 实现 engine.Viewport。
func (win *Window) ViewportHeight() int { return win.sdl.ViewportHeight() }

// Graphics 实现 engine.Viewport。
func (win *Window) Graphics() engine.Graphics { return win.sdl.Graphics() }

// OpenDocument 解析并布局 HTML 源码，绑定到本窗口。
func (win *Window) OpenDocument(src string) (engine.HTMLDocument, error) {
	return win.sdl.OpenDocument(src)
}

// SetAutoExit 让 Run 在窗口打开指定时长后自动返回（调试/CI 用）。
func (win *Window) SetAutoExit(d time.Duration) { win.sdl.SetAutoExit(d) }

// RenderFrame 手动绘制一帧（清屏+渲染+提交），供截图/无头校验使用。
func (win *Window) RenderFrame() { win.sdl.RenderFrame() }

// Screenshot 回读渲染目标当前内容（原生字节+格式+尺寸）。
func (win *Window) Screenshot() ([]byte, uint32, int, int, error) {
	return win.sdl.Screenshot()
}

// Run 进入事件循环，直到窗口关闭或 autoExit 触发。
func (win *Window) Run() error { return win.sdl.Run() }

// Close 释放窗口资源。
func (win *Window) Close() { win.sdl.Close() }

var _ engine.Viewport = (*Window)(nil)
