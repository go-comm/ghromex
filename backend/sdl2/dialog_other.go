//go:build !windows

// 本文件是非 Windows 平台的原生文件对话框空实现（与 Windows 实现同
// 签名）；包文档见 doc.go。
//
// 原生对话框是 Windows Shell 的能力（IFileDialog），其它平台没有对应
// 实现，这里统一返回 ErrUnsupported：应用层据此提示「当前平台不支持
// 原生对话框」，或退回到自己实现的路径输入。

package sdl2

// FileFilter 描述文件对话框里的一个类型过滤器（非 Windows 平台不可用）。
type FileFilter struct {
	// Name 是显示名，如 "文本文件 (*.txt)"。
	Name string
	// Pattern 是扩展名模式，如 "*.txt"；多个模式用 ";" 分隔。
	Pattern string
}

// OpenFileDialog 在非 Windows 平台总是返回 ErrUnsupported。
func (win *Window) OpenFileDialog(title, dir string, filters []FileFilter, multi bool) ([]string, error) {
	return nil, ErrUnsupported
}

// OpenDirectoryDialog 在非 Windows 平台总是返回 ErrUnsupported。
func (win *Window) OpenDirectoryDialog(title, dir string) (string, error) {
	return "", ErrUnsupported
}

// SaveFileDialog 在非 Windows 平台总是返回 ErrUnsupported。
func (win *Window) SaveFileDialog(title, dir, name string, filters []FileFilter) (string, error) {
	return "", ErrUnsupported
}
