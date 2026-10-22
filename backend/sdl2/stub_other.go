//go:build !windows

// 本文件是非 Windows 平台的空后端（与 Windows 实现同签名）；包文档见 doc.go。
// NewWindow 返回 ErrUnsupported，其余方法不应被调用。
//
// 本后端依赖 Windows 的 SDL2.dll / SDL2_ttf.dll 与系统字体目录
// （backend/sdl2 下的实现文件均为 //go:build windows）。把平台差异收在
// 这一层后，browser 与 examples 都无需构建标签；engine 本身是纯 Go，
// 其它平台可自行实现 core.Graphics 后端。

package sdl2

import (
	"github.com/go-comm/ghromex/core"
)

// Window 在非 Windows 平台不可用；NewWindow 总是失败。
type Window struct{}

// NewWindow 在非 Windows 平台总是返回 ErrUnsupported。
func NewWindow(w, h int, title string) (*Window, error) { return nil, ErrUnsupported }

// ViewportWidth 实现 core.Viewport（仅在 Window 可用时有效）。
func (*Window) ViewportWidth() int { return 0 }

// ViewportHeight 实现 core.Viewport（仅在 Window 可用时有效）。
func (*Window) ViewportHeight() int { return 0 }

// Graphics 实现 core.Viewport（仅在 Window 可用时有效）。
func (*Window) Graphics() core.Graphics { return nil }

// PollEvent 在非 Windows 平台没有事件源：永远返回 false。
func (*Window) PollEvent() (Event, bool) { return Event{}, false }

// InjectEvent 在非 Windows 平台是空操作。
func (*Window) InjectEvent(Event) {}

// Present 在非 Windows 平台是空操作。
func (*Window) Present() {}

// Delay 在非 Windows 平台是空操作。
func (*Window) Delay(int) {}

// Ticks 在非 Windows 平台恒为 0（没有事件循环，用不到）。
func (*Window) Ticks() uint32 { return 0 }

// ModState 在非 Windows 平台恒为 0（没有键盘事件）。
func (*Window) ModState() uint16 { return 0 }

// Screenshot 在非 Windows 平台总是返回 ErrUnsupported。
func (*Window) Screenshot() ([]byte, uint32, int, int, error) { return nil, 0, 0, 0, ErrUnsupported }

// RequestClose 在非 Windows 平台为空操作。
func (*Window) RequestClose() {}

// SetTitle 在非 Windows 平台为空操作。
func (*Window) SetTitle(string) {}

// NativeHandle 在非 Windows 平台总是返回 ErrUnsupported（没有原生句柄）。
func (*Window) NativeHandle() (uintptr, error) { return 0, ErrUnsupported }

// ClipboardText 在非 Windows 平台返回空串（无剪贴板实现）。
func ClipboardText() string { return "" }

// SetClipboardText 在非 Windows 平台为空操作（无剪贴板实现）。
func SetClipboardText(string) {}

// Close 在非 Windows 平台为空操作。
func (*Window) Close() {}

// Closed 在非 Windows 平台恒为 false（窗口从来没能创建）。
func (*Window) Closed() bool { return false }

// Shown 在非 Windows 平台恒为 false（窗口从来没能创建）。
func (*Window) Shown() bool { return false }

// RendererInfo 在非 Windows 平台没有渲染器，返回空串（诊断输出里一眼看出
// 「没跑在 SDL 上」）。
func (*Window) RendererInfo() string { return "" }

// SetVSync 在非 Windows 平台为空操作（没有渲染器可切换）。
func (*Window) SetVSync(bool) {}

// 平台无关地保持与 Windows 实现一致的接口实现。
var _ core.Viewport = (*Window)(nil)
