// 本文件是 browser 门面的原生文件对话框转发；包文档见 doc.go。

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
	// MultiSelect 允许一次选择多个文件（仅「打开文件」有效）。
	MultiSelect bool
}

// OpenFileDialog 弹出原生「打开文件」对话框，返回用户选中的文件路径。
//
// 多选时返回多个路径。用户取消返回 (nil, nil)——取消不是错误，调用方
// 按「用户没选」处理即可。平台不支持时返回 ErrUnsupported。
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
		Title:       opts.Title,
		DefaultDir:  opts.DefaultDir,
		Filters:     toDialogFilters(opts.Filters),
		MultiSelect: opts.MultiSelect,
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
