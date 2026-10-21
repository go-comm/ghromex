// Package clipboard 是系统剪贴板的通用调用入口。
//
// 调用方只 import 这一个包：平台差异由它内部收着——Windows 走
// backend/clipboard 的实现，其它平台没有剪贴板（Text 返回空串、
// SetText 是空操作）。
//
// 剪贴板里只放文本：引擎的输入框只管选区文本，取选区、读写剪贴板是
// 应用层的事（engine 保持纯 Go、可移植）。
//
// 典型用法（记事本示例）：
//
//	clipboard.SetText(selected)   // 复制
//	txt := clipboard.Text()       // 粘贴
//
// 注意：剪贴板是进程间共享的，写入会覆盖用户剪贴板里的原内容。
package clipboard

import backendclipboard "github.com/go-comm/ghromex/backend/clipboard"

// Text 读取系统剪贴板中的文本（UTF-8）。
//
// 剪贴板为空、里面不是文本，或当前平台没有剪贴板实现时返回空串。
func Text() string { return backendclipboard.Text() }

// SetText 把 UTF-8 文本写入系统剪贴板。
//
// 当前平台没有剪贴板实现时是空操作。写空串等于不动剪贴板。
func SetText(text string) { backendclipboard.SetText(text) }
