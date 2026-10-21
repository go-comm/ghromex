// Package dialog 是原生文件对话框（打开/另存为/选择文件夹）的跨平台接口。
//
// Windows 上接 Shell 的 IFileDialog（Vista 起的现代对话框），纯 Win32
// COM 调用、无 CGO；其它平台返回 ErrUnsupported。
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
	// MultiSelect 允许一次选择多个文件（仅「打开文件」有效）。
	MultiSelect bool
}
