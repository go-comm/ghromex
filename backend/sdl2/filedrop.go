//go:build windows

// 本文件是「文件拖放」的接收端（拖入窗口）：Win32 WM_DROPFILES + HDROP。
//
// 为什么不用 SDL 自带的 SDL_DROPFILE：SDL 把一个 HDROP 拆成「逐文件」事件
// （多选拖放进来 N 条），上层还要自己判断一批何时结束；而 HDROP 本身一次
// 就能取全整批路径。这里直接子类化窗口过程截获 WM_DROPFILES，得到的语义
// 正好是「一次拖放 = 一个事件、带一批路径」。
//
// 注意方向：本文件只做「拖入（接收）」。把窗口里的文件拖出去是 OLE 源端
// （IDropSource + DoDragDrop），与 WM_DROPFILES 无关，别混在一起。

package sdl2

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// wmDropFiles 是「文件被拖放到窗口」的窗口消息（Winuser.h）；wParam 是 HDROP。
	wmDropFiles = 0x0233
	// gwlpWndProc 是窗口过程指针的索引（Winuser.h：GWLP_WNDPROC = -4）。
	// SetWindowLongPtr 的索引参数按 uintptr 传，-4 要写成 ^uintptr(3)；
	// 直接写 uintptr(-4) 在 64 位下常量溢出，编译不过。
	gwlpWndProc = ^uintptr(3)
	// dragQueryAllFiles 是 DragQueryFileW 的「问文件总数」特殊索引（Winuser.h）。
	dragQueryAllFiles = 0xFFFFFFFF
)

var (
	dllUser32Drop  = windows.NewLazySystemDLL("user32.dll")
	dllShell32Drop = windows.NewLazySystemDLL("shell32.dll")

	procDragAcceptFiles = dllShell32Drop.NewProc("DragAcceptFiles")
	procDragQueryFileW  = dllShell32Drop.NewProc("DragQueryFileW")
	procDragFinish      = dllShell32Drop.NewProc("DragFinish")
	procCallWindowProcW = dllUser32Drop.NewProc("CallWindowProcW")
	procDefWindowProcW  = dllUser32Drop.NewProc("DefWindowProcW")
	procSetWindowPtr64  = dllUser32Drop.NewProc("SetWindowLongPtrW")
	procSetWindowPtr32  = dllUser32Drop.NewProc("SetWindowLongW")

	// dropProc 是 syscall.NewCallback 生成的窗口过程。NewCallback 的槽位
	// 是有限的，且同一个函数全局只需要一个指针，所以只建一次。
	dropProcOnce sync.Once
	dropProc     uintptr

	// dropWindows 把 HWND 映射到 *Window：窗口过程回调只拿得到 HWND，
	// 得先找回是哪个窗口才能往它的待处理队列里放路径。
	dropWindows sync.Map // map[uintptr]*Window
)

// dropWndProc 是子类化后的窗口过程：拦 WM_DROPFILES，其余消息原样转发给
// SDL 装的窗口过程。
//
// 回调跑在窗口所属线程的消息泵里（即 SDL_PollEvent → PeekMessage 内部），
// 与主循环同一个 goroutine，所以直接改 Window 字段是安全的；pendingDrops
// 仍加锁，是为了让 InjectEvent（测试/其它 goroutine）也走同一条路。
func dropWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if v, ok := dropWindows.Load(hwnd); ok {
		if win, _ := v.(*Window); win != nil {
			if msg == wmDropFiles {
				if files := queryDropPaths(wParam); len(files) > 0 {
					win.queueDrops(files)
				}
				// HDROP 由接收方释放：不 DragFinish 会泄漏一份拖放数据
				// （资源管理器还会一直挂着这次拖放）。
				procDragFinish.Call(wParam)
				return 0
			}
			if win.dropOldProc != 0 {
				r, _, _ := procCallWindowProcW.Call(win.dropOldProc, hwnd, msg, wParam, lParam)
				return r
			}
		}
	}
	// 未登记的窗口（正常不会到：只子类化 enableFileDrop 登记过的 HWND）：
	// 交给系统默认过程，避免把消息吞掉后窗口行为异常。
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// enableFileDrop 打开窗口的文件拖放接收：DragAcceptFiles + 子类化窗口过程。
//
// 返回 false 表示这个后端拿不到原生窗口（dummy 驱动等）或系统缺符号：
// 拖放静默不可用，不影响窗口其它功能。
func enableFileDrop(win *Window) bool {
	if win == nil {
		return false
	}
	hwnd := win.hwnd()
	if hwnd == 0 {
		return false
	}
	if err := procDragAcceptFiles.Find(); err != nil {
		return false
	}
	dropProcOnce.Do(func() { dropProc = syscall.NewCallback(dropWndProc) })
	if dropProc == 0 {
		return false
	}
	// 先登记再换过程：过程一换上就可能立刻收到 WM_DROPFILES。
	dropWindows.Store(hwnd, win)
	old, ok := setWindowProc(hwnd, dropProc)
	if !ok || old == 0 {
		dropWindows.Delete(hwnd)
		return false
	}
	win.dropOldProc = old
	win.dropHWND = hwnd
	procDragAcceptFiles.Call(hwnd, 1)
	return true
}

// disableFileDrop 关闭拖放接收并恢复原来的窗口过程。
//
// 必须在 SDL_DestroyWindow **之前**调用：窗口销毁后 HWND 失效，再恢复过程
// 会打到空句柄；而留给 SDL 的「原过程」也已经随着窗口一起作废。
func disableFileDrop(win *Window) {
	if win == nil || win.dropOldProc == 0 {
		return
	}
	if hwnd := win.dropHWND; hwnd != 0 {
		procDragAcceptFiles.Call(hwnd, 0)
		setWindowProc(hwnd, win.dropOldProc)
		dropWindows.Delete(hwnd)
	}
	win.dropHWND = 0
	win.dropOldProc = 0
}

// setWindowProc 把窗口过程换成 proc，返回原来的过程。
//
// 64 位系统用 SetWindowLongPtrW；32 位 Windows 不导出该符号（它是宏，
// 映射到 SetWindowLongW），按后者回退，保证 windows/386 也能构建运行。
func setWindowProc(hwnd, proc uintptr) (old uintptr, ok bool) {
	if err := procSetWindowPtr64.Find(); err == nil {
		old, _, _ = procSetWindowPtr64.Call(hwnd, gwlpWndProc, proc)
		return old, old != 0
	}
	if err := procSetWindowPtr32.Find(); err == nil {
		old, _, _ = procSetWindowPtr32.Call(hwnd, gwlpWndProc, proc)
		return old, old != 0
	}
	return 0, false
}

// queryDropPaths 从 HDROP 取出整批绝对路径（宽字符 → UTF-8）。
func queryDropPaths(hdrop uintptr) []string {
	if err := procDragQueryFileW.Find(); err != nil {
		return nil
	}
	n, _, _ := procDragQueryFileW.Call(hdrop, dragQueryAllFiles, 0, 0)
	if n == 0 {
		return nil
	}
	files := make([]string, 0, n)
	for i := uintptr(0); i < n; i++ {
		// 先问长度（不含结尾 NUL），再按长度 +1 取内容。
		size, _, _ := procDragQueryFileW.Call(hdrop, i, 0, 0)
		if size == 0 {
			continue
		}
		buf := make([]uint16, size+1)
		got, _, _ := procDragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), size+1)
		runtime.KeepAlive(buf) // 调用期间 buf 只以 uintptr 形式存在，防 GC 提前回收
		if got == 0 {
			continue
		}
		files = append(files, windows.UTF16ToString(buf[:got]))
	}
	return files
}

// queueDrops 把一批拖入路径挂到窗口上，等 PollEvent 交出去。
func (win *Window) queueDrops(files []string) {
	if win == nil || len(files) == 0 {
		return
	}
	win.dropMu.Lock()
	win.pendingDrops = append(win.pendingDrops, files...)
	win.dropMu.Unlock()
}

// takeDrops 取走待处理的拖放路径；没有则返回 nil。
func (win *Window) takeDrops() []string {
	if win == nil {
		return nil
	}
	win.dropMu.Lock()
	files := win.pendingDrops
	win.pendingDrops = nil
	win.dropMu.Unlock()
	return files
}
