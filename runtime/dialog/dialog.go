// Package dialog 是原生对话框的通用调用入口。
//
// 调用方只 import 这一个包：平台差异由它内部收着——Windows 走
// backend/dialog 的 Win32 实现（Shell 的 IFileDialog 与 user32 的
// MessageBoxW，纯 Win32 调用、无 CGO），其它平台返回 ErrUnsupported。
//
// 五个对话框：打开文件、打开多个文件、选择文件夹、另存为、消息框。
//
// # 父窗口
//
// 原生对话框要挂在应用窗口下面，需要平台的原生窗口句柄——
// browser.Window.NativeHandle() 给出（Windows 上是真实 HWND）。用 Parent
// 包装它传给对话框：
//
//	paths, err := dialog.OpenFileDialog(dialog.Parent(hwnd), dialog.Options{
//		Title:      "打开",
//		DefaultDir: `C:\Users\me\Documents`,
//		Filters: []dialog.FileFilter{
//			{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
//			{Name: "所有文件 (*.*)", Pattern: "*.*"},
//		},
//	})
//
// Parent 为 0 表示没有父窗口：对话框照样能弹，只是不挂在主窗口下面。
//
// # 模态
//
// 对话框都是模态的：调用期间阻塞当前线程（父窗口的事件循环随之暂停），
// 这与原生应用里模态对话框的表现一致，应在按钮/菜单的回调里直接调用。
package dialog

import backenddialog "github.com/go-comm/ghromex/backend/dialog"

// 类型直接复用平台实现（类型别名）：调用方不用关心它们住在哪一层，
// 平台层换实现时这一层的名字不变。
type (
	// Options 是文件对话框的选项。
	Options = backenddialog.Options
	// FileFilter 描述文件对话框里的一个类型过滤器（对应下拉框里的一项）。
	FileFilter = backenddialog.FileFilter
	// MessageOptions 是消息框的选项。
	MessageOptions = backenddialog.MessageOptions
	// MessageIcon 是消息框的图标。
	MessageIcon = backenddialog.MessageIcon
	// MessageButtons 是消息框的按钮组合。
	MessageButtons = backenddialog.MessageButtons
	// MessageResult 是用户在消息框里点击的按钮。
	MessageResult = backenddialog.MessageResult
)

// ErrUnsupported 表示当前平台没有原生对话框实现。
var ErrUnsupported = backenddialog.ErrUnsupported

// Parent 是对话框的父窗口（平台相关的原生窗口句柄）。
//
// 取法：browser.Window.NativeHandle()。0 表示没有父窗口。
type Parent uintptr

// OpenFileDialog 弹出「打开文件」对话框（单选），返回用户选中的文件路径。
//
// 用户取消返回 (nil, nil)——取消不是错误，调用方按「用户没选」处理即可。
// 平台不支持时返回 ErrUnsupported。
func OpenFileDialog(parent Parent, opts Options) ([]string, error) {
	return backenddialog.OpenFileDialog(uintptr(parent), opts)
}

// OpenMultipleFilesDialog 弹出「打开文件」对话框（允许多选），返回用户
// 选中的多个文件路径。
//
// 用户取消返回 (nil, nil)；平台不支持时返回 ErrUnsupported。
func OpenMultipleFilesDialog(parent Parent, opts Options) ([]string, error) {
	return backenddialog.OpenMultipleFilesDialog(uintptr(parent), opts)
}

// OpenDirectoryDialog 弹出「选择文件夹」对话框，返回用户选中的目录。
//
// 用户取消返回 ("", nil)；平台不支持时返回 ErrUnsupported。
func OpenDirectoryDialog(parent Parent, opts Options) (string, error) {
	return backenddialog.OpenDirectoryDialog(uintptr(parent), opts)
}

// SaveFileDialog 弹出「另存为」对话框，返回用户输入的保存路径。
//
// 用户取消返回 ("", nil)；平台不支持时返回 ErrUnsupported。
func SaveFileDialog(parent Parent, opts Options) (string, error) {
	return backenddialog.SaveFileDialog(uintptr(parent), opts)
}

// MessageDialog 弹一个模态消息框，返回用户点击的按钮。
//
// 例如「是否覆盖已存在的文件？」：
//
//	res, err := dialog.MessageDialog(dialog.Parent(hwnd), dialog.MessageOptions{
//		Title:   "确认",
//		Message: "文件已存在，要覆盖吗？",
//		Icon:    dialog.IconQuestion,
//		Buttons: dialog.ButtonsYesNo,
//	})
//	if err == nil && res == dialog.ResultYes {
//		// 用户点了「是」
//	}
//
// 平台不支持时返回 ErrUnsupported。
func MessageDialog(parent Parent, opts MessageOptions) (MessageResult, error) {
	return backenddialog.MessageDialog(uintptr(parent), opts)
}
