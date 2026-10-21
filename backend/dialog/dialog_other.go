//go:build !windows

// 本文件是非 Windows 平台的原生文件对话框空实现（与 Windows 实现同
// 签名）；包文档见 dialog.go。
//
// 原生对话框是 Windows Shell 的能力（IFileDialog），其它平台没有对应
// 实现，这里统一返回 ErrUnsupported：应用层据此提示「当前平台不支持
// 原生对话框」，或退回到自己实现的路径输入。

package dialog

// OpenFileDialog 在非 Windows 平台总是返回 ErrUnsupported。
func OpenFileDialog(parent uintptr, opts Options) ([]string, error) {
	return nil, ErrUnsupported
}

// OpenMultipleFilesDialog 在非 Windows 平台总是返回 ErrUnsupported。
func OpenMultipleFilesDialog(parent uintptr, opts Options) ([]string, error) {
	return nil, ErrUnsupported
}

// OpenDirectoryDialog 在非 Windows 平台总是返回 ErrUnsupported。
func OpenDirectoryDialog(parent uintptr, opts Options) (string, error) {
	return "", ErrUnsupported
}

// SaveFileDialog 在非 Windows 平台总是返回 ErrUnsupported。
func SaveFileDialog(parent uintptr, opts Options) (string, error) {
	return "", ErrUnsupported
}
