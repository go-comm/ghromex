// 本文件是 browser 门面的原生对话框转发；包文档见 doc.go。

package browser

import "github.com/go-comm/ghromex/backend/dialog"

// FileFilter 描述文件对话框里的一个类型过滤器（对应下拉框里的一项）。
type FileFilter struct {
	// Name 是显示名，如 "文本文件 (*.txt)"。
	Name string
	// Pattern 是扩展名模式，如 "*.txt"；多个模式用 ";" 分隔
	//（"*.txt;*.md"）。
	Pattern string
}

// DialogOptions 是文件对话框的公共选项。
type DialogOptions struct {
	// Title 是对话框标题；留空则用系统默认标题（打开/另存为）。
	Title string
	// DefaultDir 是初始目录；留空则用系统上次使用过的目录。
	DefaultDir string
	// DefaultName 是初始文件名（另存为用）。
	DefaultName string
	// Filters 是文件类型过滤器；留空则不过滤（显示全部文件）。
	Filters []FileFilter
}

// 消息框的类型直接复用 backend/dialog（类型别名），门面不另造一套。
type (
	// MessageOptions 是消息框的选项。
	MessageOptions = dialog.MessageOptions
	// MessageIcon 是消息框的图标。
	MessageIcon = dialog.MessageIcon
	// MessageButtons 是消息框的按钮组合。
	MessageButtons = dialog.MessageButtons
	// MessageResult 是用户在消息框里点击的按钮。
	MessageResult = dialog.MessageResult
)

const (
	// IconInfo 是信息图标（字母 i）。
	IconInfo = dialog.IconInfo
	// IconWarning 是警告图标（感叹号）。
	IconWarning = dialog.IconWarning
	// IconError 是错误图标（红叉）。
	IconError = dialog.IconError
	// IconQuestion 是询问图标（问号）。
	IconQuestion = dialog.IconQuestion

	// ButtonsOK 只有「确定」。
	ButtonsOK = dialog.ButtonsOK
	// ButtonsOKCancel 是「确定」「取消」。
	ButtonsOKCancel = dialog.ButtonsOKCancel
	// ButtonsYesNo 是「是」「否」。
	ButtonsYesNo = dialog.ButtonsYesNo
	// ButtonsYesNoCancel 是「是」「否」「取消」。
	ButtonsYesNoCancel = dialog.ButtonsYesNoCancel

	// ResultOK 是「确定」。
	ResultOK = dialog.ResultOK
	// ResultCancel 是「取消」。
	ResultCancel = dialog.ResultCancel
	// ResultYes 是「是」。
	ResultYes = dialog.ResultYes
	// ResultNo 是「否」。
	ResultNo = dialog.ResultNo
)

// OpenFileDialog 弹出原生「打开文件」对话框（单选），返回用户选中的文件路径。
//
// 用户取消返回 (nil, nil)——取消不是错误，调用方按「用户没选」处理即可。
// 平台不支持时返回 ErrUnsupported。
//
// 对话框是模态的：调用期间会阻塞当前线程（父窗口的 SDL 事件循环随之
// 暂停），因此应在按钮/菜单的回调里直接调用。
func (win *Window) OpenFileDialog(opts DialogOptions) ([]string, error) {
	if win == nil || win.sdl == nil {
		return nil, ErrUnsupported
	}
	parent, err := win.sdl.NativeHandle()
	if err != nil {
		return nil, err
	}
	return dialog.OpenFileDialog(parent, dialog.Options{
		Title:      opts.Title,
		DefaultDir: opts.DefaultDir,
		Filters:    toDialogFilters(opts.Filters),
	})
}

// OpenMultipleFilesDialog 弹出原生「打开文件」对话框（允许多选），返回
// 用户选中的多个文件路径。
//
// 用户取消返回 (nil, nil)；平台不支持时返回 ErrUnsupported。
func (win *Window) OpenMultipleFilesDialog(opts DialogOptions) ([]string, error) {
	if win == nil || win.sdl == nil {
		return nil, ErrUnsupported
	}
	parent, err := win.sdl.NativeHandle()
	if err != nil {
		return nil, err
	}
	return dialog.OpenMultipleFilesDialog(parent, dialog.Options{
		Title:      opts.Title,
		DefaultDir: opts.DefaultDir,
		Filters:    toDialogFilters(opts.Filters),
	})
}

// OpenDirectoryDialog 弹出原生「选择文件夹」对话框，返回用户选中的目录。
//
// 用户取消返回 ("", nil)；平台不支持时返回 ErrUnsupported。
func (win *Window) OpenDirectoryDialog(opts DialogOptions) (string, error) {
	if win == nil || win.sdl == nil {
		return "", ErrUnsupported
	}
	parent, err := win.sdl.NativeHandle()
	if err != nil {
		return "", err
	}
	return dialog.OpenDirectoryDialog(parent, dialog.Options{
		Title:      opts.Title,
		DefaultDir: opts.DefaultDir,
	})
}

// SaveFileDialog 弹出原生「另存为」对话框，返回用户输入的保存路径。
//
// 用户取消返回 ("", nil)；平台不支持时返回 ErrUnsupported。
func (win *Window) SaveFileDialog(opts DialogOptions) (string, error) {
	if win == nil || win.sdl == nil {
		return "", ErrUnsupported
	}
	parent, err := win.sdl.NativeHandle()
	if err != nil {
		return "", err
	}
	return dialog.SaveFileDialog(parent, dialog.Options{
		Title:       opts.Title,
		DefaultDir:  opts.DefaultDir,
		DefaultName: opts.DefaultName,
		Filters:     toDialogFilters(opts.Filters),
	})
}

// MessageDialog 弹一个原生消息框，返回用户点击的按钮。
//
// 例如「是否覆盖已存在的文件？」：
//
//	res, err := win.MessageDialog(browser.MessageOptions{
//		Title:   "确认",
//		Message: "文件已存在，要覆盖吗？",
//		Icon:    browser.IconQuestion,
//		Buttons: browser.ButtonsYesNo,
//	})
//	if err == nil && res == browser.ResultYes {
//		// 用户点了「是」
//	}
//
// 消息框是模态的：调用期间会阻塞当前线程。平台不支持时返回 ErrUnsupported。
func (win *Window) MessageDialog(opts MessageOptions) (MessageResult, error) {
	if win == nil || win.sdl == nil {
		return ResultOK, ErrUnsupported
	}
	parent, err := win.sdl.NativeHandle()
	if err != nil {
		return ResultOK, err
	}
	return dialog.MessageDialog(parent, opts)
}

// toDialogFilters 把门面的过滤器转成对话框包的类型（对话框包不认识
// browser 包）。
func toDialogFilters(filters []FileFilter) []dialog.FileFilter {
	if len(filters) == 0 {
		return nil
	}
	out := make([]dialog.FileFilter, 0, len(filters))
	for _, f := range filters {
		out = append(out, dialog.FileFilter{Name: f.Name, Pattern: f.Pattern})
	}
	return out
}
