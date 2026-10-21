//go:build !windows

// 本文件是非 Windows 平台的空后端（与 Windows 实现同签名）；包文档见 doc.go。
// NewWindow 返回 ErrUnsupported，其余方法不应被调用。
//
// 本后端依赖 Windows 的 SDL2.dll / SDL2_ttf.dll 与系统字体目录
// （backend/sdl2 下的实现文件均为 //go:build windows）。把平台差异收在
// 这一层后，browser 与 examples 都无需构建标签；engine 本身是纯 Go，
// 其它平台可自行实现 engine.Graphics 后端。

package sdl2

import (
	"time"

	"github.com/go-comm/ghromex/engine"
)

// Window 在非 Windows 平台不可用；NewWindow 总是失败。
type Window struct{}

// NewWindow 在非 Windows 平台总是返回 ErrUnsupported。
func NewWindow(w, h int, title string) (*Window, error) { return nil, ErrUnsupported }

// ViewportWidth 实现 engine.Viewport（仅在 Window 可用时有效）。
func (*Window) ViewportWidth() int { return 0 }

// ViewportHeight 实现 engine.Viewport（仅在 Window 可用时有效）。
func (*Window) ViewportHeight() int { return 0 }

// Graphics 实现 engine.Viewport（仅在 Window 可用时有效）。
func (*Window) Graphics() engine.Graphics { return nil }

// OpenDocument 在非 Windows 平台总是返回 ErrUnsupported。
func (*Window) OpenDocument(src string) (engine.HTMLDocument, error) { return nil, ErrUnsupported }

// SetAutoExit 在非 Windows 平台为空操作。
func (*Window) SetAutoExit(d time.Duration) {}

// RenderFrame 在非 Windows 平台为空操作。
func (*Window) RenderFrame() {}

// Screenshot 在非 Windows 平台总是返回 ErrUnsupported。
func (*Window) Screenshot() ([]byte, uint32, int, int, error) { return nil, 0, 0, 0, ErrUnsupported }

// Run 在非 Windows 平台总是返回 ErrUnsupported。
func (*Window) Run() error { return ErrUnsupported }

// RequestClose 在非 Windows 平台为空操作。
func (*Window) RequestClose() {}

// SetTitle 在非 Windows 平台为空操作。
func (*Window) SetTitle(string) {}

// ClipboardText 在非 Windows 平台返回空串（无剪贴板实现）。
func ClipboardText() string { return "" }

// SetClipboardText 在非 Windows 平台为空操作（无剪贴板实现）。
func SetClipboardText(string) {}

// Close 在非 Windows 平台为空操作。
func (*Window) Close() {}

// NativeHandle 在非 Windows 平台返回 0（无原生句柄）。
func (*Window) NativeHandle() (uintptr, error) { return 0, ErrUnsupported }

// 平台无关地保持与 Windows 实现一致的接口实现。
var _ engine.Viewport = (*Window)(nil)
