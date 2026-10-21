// Package dialog 是原生对话框（文件对话框 + 消息框）的跨平台接口。
//
// Windows 上文件对话框接 Shell 的 IFileDialog（Vista 起的现代对话框），
// 纯 Win32 COM 调用、无 CGO；消息框接 user32 的 MessageBoxW（不需要
// COM）。其它平台一律返回 ErrUnsupported。
//
// 它与渲染后端（backend/sdl2）是两件事：对话框是操作系统的能力，不依赖
// SDL。父窗口句柄由调用方传入（backend/sdl2 的 Window.NativeHandle 给出
// 真实 HWND）——SDL_CreateWindow 返回的是 SDL_Window* 而非 HWND，原生对话框
// 要拿真正的 HWND 当父窗口（否则 Show 会回 E_INVALIDARG）。
//
// 线程约束：IFileDialog 必须在 STA（单线程套间）上创建并 Show，本包内部
// 会 LockOSThread 并用 CoInitializeEx(COINIT_APARTMENTTHREADED) 把当前线程
// 初始化成 STA；Show 期间阻塞该线程（对话框自带消息循环，父窗口的 SDL
// 事件循环随之暂停，这与原生应用「模态对话框期间父窗口不响应」一致）。
//
// 本包提供五个对话框：打开文件、打开多个文件、选择文件夹、另存为，以及
// 消息框（提示/警告/询问）。前四个走 Shell 的 IFileDialog，消息框走
// user32 的 MessageBoxW（不需要 COM、不需要 STA）。
//
// 对话框是**模态**的：应在按钮/菜单的回调里直接调用。
package dialog

import "errors"

// ErrUnsupported 表示当前平台没有原生文件对话框实现。
var ErrUnsupported = errors.New("ghromex 原生文件对话框仅支持 Windows")

// FileFilter 描述文件对话框里的一个类型过滤器（对应下拉框里的一项）。
type FileFilter struct {
	// Name 是显示名，如 "文本文件 (*.txt)"。
	Name string
	// Pattern 是扩展名模式，如 "*.txt"；多个模式用 ";" 分隔
	//（"*.txt;*.md"）。
	Pattern string
}

// Options 是文件对话框的公共选项。
type Options struct {
	// Title 是对话框标题；留空则用系统默认标题（打开/另存为）。
	Title string
	// DefaultDir 是初始目录；留空则用系统上次使用过的目录。
	DefaultDir string
	// DefaultName 是初始文件名（另存为用）。
	DefaultName string
	// Filters 是文件类型过滤器；留空则不过滤（显示全部文件）。
	Filters []FileFilter
}

// MessageIcon 是消息框的图标。
type MessageIcon int

const (
	// IconInfo 是信息图标（字母 i）。
	IconInfo MessageIcon = iota
	// IconWarning 是警告图标（感叹号）。
	IconWarning
	// IconError 是错误图标（红叉）。
	IconError
	// IconQuestion 是询问图标（问号）。
	IconQuestion
)

// MessageButtons 是消息框的按钮组合。
type MessageButtons int

const (
	// ButtonsOK 只有「确定」。
	ButtonsOK MessageButtons = iota
	// ButtonsOKCancel 是「确定」「取消」。
	ButtonsOKCancel
	// ButtonsYesNo 是「是」「否」。
	ButtonsYesNo
	// ButtonsYesNoCancel 是「是」「否」「取消」。
	ButtonsYesNoCancel
)

// MessageResult 是用户在消息框里点击的按钮。
type MessageResult int

const (
	// ResultOK 是「确定」。
	ResultOK MessageResult = iota
	// ResultCancel 是「取消」。
	ResultCancel
	// ResultYes 是「是」。
	ResultYes
	// ResultNo 是「否」。
	ResultNo
)

// MessageOptions 是消息框的选项。
type MessageOptions struct {
	// Title 是标题栏文字。
	Title string
	// Message 是正文。
	Message string
	// Icon 是图标；零值是 IconInfo。
	Icon MessageIcon
	// Buttons 是按钮组合；零值是 ButtonsOK。
	Buttons MessageButtons
}
