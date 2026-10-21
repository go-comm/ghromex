//go:build windows

// 本文件是 Windows 原生消息框（MessageBoxW）的实现；包文档见 dialog.go。
//
// 消息框与文件对话框不同：它是 user32 的 MessageBoxW，不是 Shell 的
// IFileDialog——不需要 COM、不需要 STA，任何线程都能弹（它自己带消息
// 循环），因此单独一个文件，也没有单独的包。
//
// 线程约束：MessageBoxW 是模态的，调用期间阻塞当前线程（父窗口的 SDL
// 事件循环随之暂停），与文件对话框的行为一致——在按钮/菜单的回调里
// 直接调用即可。

package dialog

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 系统 DLL 与入口点。
var (
	dllUser32       = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW = dllUser32.NewProc("MessageBoxW")
)

// MB_* 是 MessageBoxW 的 uType 位标志（winuser.h）。
const (
	mbOK          = 0x00000000 // 只有「确定」
	mbOKCancel    = 0x00000001 // 「确定」「取消」
	mbYesNo       = 0x00000004 // 「是」「否」
	mbYesNoCancel = 0x00000003 // 「是」「否」「取消」

	mbIconError    = 0x00000010 // MB_ICONHAND：红叉
	mbIconQuestion = 0x00000020 // MB_ICONQUESTION：问号
	mbIconWarning  = 0x00000030 // MB_ICONEXCLAMATION：感叹号
	mbIconInfo     = 0x00000040 // MB_ICONASTERISK：字母 i
)

// 消息框的返回值 = 被点击按钮的控件 ID（winuser.h）。
//
// 名字带 msg 前缀：dialog_ui_test.go 里也有 idOK（驱动对话框用的），
// 同包同名会撞车。
const (
	msgIDOK     = 1
	msgIDCancel = 2
	msgIDYes    = 6
	msgIDNo     = 7
)

// MessageDialog 弹一个模态消息框，返回用户点击的按钮。
//
// parent 是父窗口的原生 HWND（0 = 无父窗口，消息框照样能弹）。
// 用户点「取消」返回 ResultCancel；平台不支持时返回 ErrUnsupported。
//
// 消息框是模态的：调用期间阻塞当前线程（父窗口的 SDL 事件循环随之
// 暂停），因此应在按钮/菜单的回调里直接调用。
func MessageDialog(parent uintptr, opts MessageOptions) (MessageResult, error) {
	text, err := windows.UTF16PtrFromString(opts.Message)
	if err != nil {
		return ResultOK, fmt.Errorf("消息内容含非法字符: %w", err)
	}
	caption, err := windows.UTF16PtrFromString(opts.Title)
	if err != nil {
		return ResultOK, fmt.Errorf("消息框标题含非法字符: %w", err)
	}

	// MessageBoxW 返回被点击按钮的 ID；返回 0 是失败（资源不足等）。
	flags := messageBoxFlags(opts.Icon, opts.Buttons)
	ret, _, _ := procMessageBoxW.Call(
		parent,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(caption)),
		uintptr(flags),
	)
	// 宽串是 Go 堆上的内存：转成 uintptr 之后编译器不再认为它活着，
	// 不 KeepAlive 的话 GC 可能在调用期间回收它。
	runtime.KeepAlive(text)
	runtime.KeepAlive(caption)

	if ret == 0 {
		return ResultOK, fmt.Errorf("消息框显示失败")
	}
	result, ok := messageResult(ret)
	if !ok {
		return ResultOK, fmt.Errorf("消息框返回了未知的按钮 ID: %d", ret)
	}
	return result, nil
}

// messageBoxFlags 把图标与按钮组合翻译成 MessageBoxW 的 uType 位标志。
func messageBoxFlags(icon MessageIcon, buttons MessageButtons) uint32 {
	var flags uint32
	switch buttons {
	case ButtonsOKCancel:
		flags = mbOKCancel
	case ButtonsYesNo:
		flags = mbYesNo
	case ButtonsYesNoCancel:
		flags = mbYesNoCancel
	default:
		flags = mbOK
	}
	switch icon {
	case IconWarning:
		flags |= mbIconWarning
	case IconError:
		flags |= mbIconError
	case IconQuestion:
		flags |= mbIconQuestion
	default:
		flags |= mbIconInfo
	}
	return flags
}

// messageResult 把 MessageBoxW 的返回值翻译成 MessageResult。
func messageResult(id uintptr) (MessageResult, bool) {
	switch id {
	case msgIDOK:
		return ResultOK, true
	case msgIDCancel:
		return ResultCancel, true
	case msgIDYes:
		return ResultYes, true
	case msgIDNo:
		return ResultNo, true
	default:
		return ResultOK, false
	}
}
