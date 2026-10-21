// 本文件是 browser 门面的剪贴板转发；包文档见 doc.go。

package browser

import "github.com/go-comm/ghromex/runtime/clipboard"

// ClipboardText 读取系统剪贴板文本（UTF-8）。剪贴板属平台能力：引擎只
// 给出选区文本，读写由后端完成。非 Windows 平台返回空串。
func ClipboardText() string { return clipboard.Text() }

// SetClipboardText 把 UTF-8 文本写入系统剪贴板。非 Windows 平台为空操作。
func SetClipboardText(text string) { clipboard.SetText(text) }
