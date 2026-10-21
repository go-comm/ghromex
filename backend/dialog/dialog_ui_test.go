//go:build windows

package dialog

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/go-comm/ghromex/backend/sdl2"
)

// 原生对话框的端到端验证：真弹一次「打开文件」对话框，在后台线程里
// 找到它、填好文件名、按下「打开」，然后断言拿回来的路径正确。
//
// 需要真实桌面交互，默认跳过；用下面的命令跑：
//
//	$env:GHROMEX_DIALOG_UI=1; go test ./backend/dialog -run TestOpenFileDialogUI -v
//
// 无头环境（CI）不要开这个开关：对话框弹不出来，测试会一直等到超时。
func TestOpenFileDialogUI(t *testing.T) {
	if os.Getenv("GHROMEX_DIALOG_UI") == "" {
		t.Skip("需要真实桌面交互：设置 GHROMEX_DIALOG_UI=1 运行")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	win, err := sdl2.NewWindow(400, 300, "dialog-ui-test")
	if err != nil {
		t.Fatalf("创建窗口失败: %v", err)
	}
	defer win.Close()
	hwnd, err := win.NativeHandle()
	if err != nil {
		t.Fatalf("取父窗口句柄失败: %v", err)
	}

	const title = "打开文件（UI 测试）"
	done := make(chan error, 1)
	go func() { done <- driveFileDialog(title, path) }()

	got, err := OpenFileDialog(hwnd, Options{
		Title:      title,
		DefaultDir: dir,
		Filters:    []FileFilter{{Name: "文本文件 (*.txt)", Pattern: "*.txt"}},
	})
	if err != nil {
		t.Fatalf("打开对话框失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("选中文件数 = %d, want 1", len(got))
	}
	if !samePath(got[0], path) {
		t.Errorf("选中路径 = %q, want %q", got[0], path)
	}
	if err := <-done; err != nil {
		t.Errorf("驱动对话框失败: %v", err)
	}
}

// 多选模式（MultiSelect=true）走的是另一条取值路径：GetResults 返回
// IShellItemArray，再逐项取路径。这条路径依赖 IFileOpenDialog::GetResults
// 的虚表下标——不同 Windows 版本上它落在不同的槽位，实现是运行时探测的，
// 这里验一遍（驱动脚本只选一个文件，够验证取值链路了）。
func TestOpenFilesDialogMultiUI(t *testing.T) {
	if os.Getenv("GHROMEX_DIALOG_UI") == "" {
		t.Skip("需要真实桌面交互：设置 GHROMEX_DIALOG_UI=1 运行")
	}

	dir := t.TempDir()
	first := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	win, err := sdl2.NewWindow(400, 300, "dialog-ui-multi")
	if err != nil {
		t.Fatalf("创建窗口失败: %v", err)
	}
	defer win.Close()
	hwnd, err := win.NativeHandle()
	if err != nil {
		t.Fatalf("取父窗口句柄失败: %v", err)
	}

	const title = "多选（UI 测试）"
	done := make(chan error, 1)
	go func() { done <- driveFileDialog(title, "a.txt") }()

	got, err := OpenFileDialog(hwnd, Options{
		Title:       title,
		DefaultDir:  dir,
		Filters:     []FileFilter{{Name: "文本文件 (*.txt)", Pattern: "*.txt"}},
		MultiSelect: true,
	})
	if err != nil {
		t.Fatalf("打开多选对话框失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("选中文件数 = %d (%v), want 1", len(got), got)
	}
	if !samePath(got[0], first) {
		t.Errorf("选中路径 = %q, want %q", got[0], first)
	}
	if err := <-done; err != nil {
		t.Errorf("驱动对话框失败: %v", err)
	}
}

// driveFileDialog 在后台线程里等对话框出现，填入文件名并按下「打开」。
func driveFileDialog(title, path string) error {
	dlg := waitForWindow(classDialog, title, 20*time.Second)
	if dlg == 0 {
		return errors.New("对话框窗口没有出现")
	}
	// 窗口一出现时对话框内部还没初始化完（文件名框、按钮都还没建好），
	// 这时候发消息会让 Shell 的控件回调崩掉。等它稳定下来再操作。
	time.Sleep(dialogSettleDelay)

	// 文件名输入框：ComboBoxEx32 → ComboBox → Edit（资源管理器式对话框）。
	edit := filenameEdit(dlg)
	if edit == 0 {
		edit = focusedControl(dlg) // 退化：用当前获得焦点的控件
	}
	if edit == 0 {
		return errors.New("找不到文件名输入框")
	}
	if !setWindowText(edit, path) {
		return errors.New("设置文件名失败")
	}

	// 「打开」按钮的控件 ID 是 IDOK(1)——按 ID 找，不受界面语言影响。
	btn := buttonByID(dlg, idOK)
	if btn == 0 {
		// 退化：直接回车（对话框的默认按钮）。
		postKey(edit, vkReturn)
		return nil
	}
	postMessage(btn, bmClick, 0, 0)
	return nil
}

// waitForWindow 等某个类名 + 标题的窗口出现，返回其句柄。
func waitForWindow(class, title string, timeout time.Duration) uintptr {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h := findWindow(class, title); h != 0 {
			return h
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

// Win32 消息与控件查找（驱动对话框用）。
const (
	wmSetText = 0x000C
	wmKeyDown = 0x0100
	bmClick   = 0x00F5
	vkReturn  = 0x0D
	idOK      = 1

	classDialog     = "#32770"
	classComboBoxEx = "ComboBoxEx32"
	classComboBox   = "ComboBox"
	classEdit       = "Edit"
	classButton     = "Button"

	// dialogSettleDelay 是「对话框窗口出现 → 内部控件可用」的等待时间。
	dialogSettleDelay = 700 * time.Millisecond
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procFindWindow      = user32.NewProc("FindWindowW")
	procFindWindowEx    = user32.NewProc("FindWindowExW")
	procSendMessage     = user32.NewProc("SendMessageW")
	procPostMessage     = user32.NewProc("PostMessageW")
	procGetWindowThread = user32.NewProc("GetWindowThreadProcessId")
	procGetGUIThread    = user32.NewProc("GetGUIThreadInfo")
	procGetDlgCtrlID    = user32.NewProc("GetDlgCtrlID")
	procGetClassName    = user32.NewProc("GetClassNameW")
)

// guiThreadInfo 是 GUITHREADINFO 的前几个字段（hwndFocus 在第 8 个）。
type guiThreadInfo struct {
	cbSize        uint32
	flags         uint32
	hwndActive    uintptr
	hwndFocus     uintptr
	hwndCapture   uintptr
	hwndMenuOwner uintptr
	hwndMoveSize  uintptr
	hwndCaret     uintptr
	rcCaret       int32
}

// findWindow 按窗口类名与标题找窗口。
func findWindow(class, title string) uintptr {
	c, err := windows.UTF16PtrFromString(class)
	if err != nil {
		return 0
	}
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	h, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)))
	// 宽串是 Go 堆上的内存：转成 uintptr 之后编译器不再认为它活着，
	// 不 KeepAlive 的话 GC 可能在调用期间回收它，句柄就随机了。
	runtime.KeepAlive(c)
	runtime.KeepAlive(t)
	return h
}

// findChild 找直接子控件（按类名；class 为空则不筛类名）。
func findChild(parent uintptr, class string) uintptr {
	var c uintptr
	var keep *uint16
	if class != "" {
		p, err := windows.UTF16PtrFromString(class)
		if err != nil {
			return 0
		}
		keep = p
		c = uintptr(unsafe.Pointer(p))
	}
	h, _, _ := procFindWindowEx.Call(parent, 0, c, 0)
	runtime.KeepAlive(keep)
	return h
}

// filenameEdit 取对话框里的文件名输入框（ComboBoxEx32 → ComboBox → Edit）。
func filenameEdit(dlg uintptr) uintptr {
	comboEx := findChild(dlg, classComboBoxEx)
	if comboEx == 0 {
		return 0
	}
	combo := findChild(comboEx, classComboBox)
	if combo == 0 {
		return 0
	}
	return findChild(combo, classEdit)
}

// buttonByID 找控件 ID 指定的按钮（IDOK=1 是「打开」）。
func buttonByID(dlg uintptr, id int) uintptr {
	child, _, _ := procFindWindowEx.Call(dlg, 0, 0, 0)
	for child != 0 {
		if className(child) == classButton {
			got, _, _ := procGetDlgCtrlID.Call(child)
			if int(got) == id {
				return child
			}
		}
		child, _, _ = procFindWindowEx.Call(dlg, child, 0, 0)
	}
	return 0
}

// className 取窗口类名。
func className(hwnd uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// focusedControl 取对话框线程里获得焦点的控件（文件名输入框）。
func focusedControl(dlg uintptr) uintptr {
	// GetWindowThreadProcessId 的返回值就是窗口所在线程 ID（第二个参数
	// 是可选的进程 ID 出参）。
	tid, _, _ := procGetWindowThread.Call(dlg, 0)
	if tid == 0 {
		return 0
	}
	info := guiThreadInfo{cbSize: uint32(unsafe.Sizeof(guiThreadInfo{}))}
	ok, _, _ := procGetGUIThread.Call(tid, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 0
	}
	return info.hwndFocus
}

// utf16Ptr 把 Go 字符串转成宽串指针（测试里驱动对话框用）。
func utf16Ptr(s string) (*uint16, error) { return windows.UTF16PtrFromString(s) }

// setWindowText 给控件设置文本（WM_SETTEXT）。
func setWindowText(hwnd uintptr, text string) bool {
	p, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return false
	}
	ret, _, _ := procSendMessage.Call(hwnd, wmSetText, 0, uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
	return ret != 0
}

// postMessage 给窗口投递一条消息。
func postMessage(hwnd uintptr, msg, wparam, lparam uintptr) {
	procPostMessage.Call(hwnd, msg, wparam, lparam)
}

// postKey 给控件投递一个按键（WM_KEYDOWN）。
func postKey(hwnd uintptr, vk uintptr) {
	postMessage(hwnd, wmKeyDown, vk, 0)
}
