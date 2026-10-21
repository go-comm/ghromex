// Package clipboard 是系统剪贴板的平台实现。
//
// 目前的实现复用 SDL2 的剪贴板接口（SDL_GetClipboardText /
// SDL_SetClipboardText）——它们操作的正是 Windows 系统剪贴板
// （CF_UNICODETEXT），引擎内部处理 Ctrl+C / Ctrl+V 时走的也是同一条路径，
// 所以应用层读写的与引擎粘贴进来的是同一份内容。
//
// 非 Windows 平台没有剪贴板实现：Text 返回空串、SetText 是空操作。
//
// 调用方用 runtime/clipboard（通用入口），别直接依赖本包。
package clipboard

import "github.com/go-comm/ghromex/backend/sdl2"

// Text 读取系统剪贴板中的文本（UTF-8）。
//
// 剪贴板为空、里面不是文本，或没有剪贴板实现时返回空串。
func Text() string { return sdl2.ClipboardText() }

// SetText 把 UTF-8 文本写入系统剪贴板。
//
// 没有剪贴板实现时是空操作。写空串等于不动剪贴板（SDL 的接口如此）。
func SetText(text string) { sdl2.SetClipboardText(text) }
