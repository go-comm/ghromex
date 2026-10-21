//go:build !windows

// 本文件是非 Windows 平台的原生消息框空实现（与 Windows 实现同签名）；
// 包文档见 dialog.go。
//
// 消息框在其它平台没有系统级对应物（各家的 GTK/Qt 方案都要额外依赖），
// 这里统一返回 ErrUnsupported：应用层据此提示「当前平台不支持原生
// 消息框」，或退回到自己实现的页面内提示。

package dialog

// MessageDialog 在非 Windows 平台总是返回 ErrUnsupported。
func MessageDialog(parent uintptr, opts MessageOptions) (MessageResult, error) {
	return ResultOK, ErrUnsupported
}
