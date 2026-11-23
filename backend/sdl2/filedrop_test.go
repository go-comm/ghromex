//go:build windows

// 文件拖放（拖入窗口）后端测试。
//
// 真实拖放（Explorer → 窗口）要真人操作，无法自动化；但 WM_DROPFILES 的
// 载荷 HDROP 可以自己构造（DROPFILES 头 + 宽字符文件清单），PostMessage
// 给窗口后就是一条和真实拖放逐字节同构的消息。于是「窗口过程 → DragQueryFileW
// → 待处理队列 → PollEvent」这条链能完整跑通。
//
// 无原生窗口的环境（CI 用 SDL_VIDEO_DRIVER=dummy）跳过消息级用例，
// 只保留纯队列/降级用例。
package sdl2

import (
	"encoding/binary"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DROPFILES（ShlObj.h）：pFiles(4) pt(8) fNC(4) fWide(4) = 20 字节。
const dropFilesHeaderSize = 20

var (
	dllKernel32Drop   = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc   = dllKernel32Drop.NewProc("GlobalAlloc")
	procGlobalLock    = dllKernel32Drop.NewProc("GlobalLock")
	procGlobalUnlock  = dllKernel32Drop.NewProc("GlobalUnlock")
	procRtlMoveMemory = dllKernel32Drop.NewProc("RtlMoveMemory")
	procPostMessageW  = dllUser32Drop.NewProc("PostMessageW")
)

// makeHDROP 构造一个「宽字符文件清单」的 HDROP（GMEM_MOVEABLE）。
func makeHDROP(t *testing.T, paths []string) uintptr {
	t.Helper()
	var list []uint16
	for _, p := range paths {
		u, err := windows.UTF16FromString(p)
		if err != nil {
			t.Fatalf("路径含 NUL: %q", p)
		}
		list = append(list, u...) // u 已含结尾 NUL
	}
	list = append(list, 0) // 整份清单再以 NUL 收尾

	// 先在 Go 缓冲里拼好，再 RtlMoveMemory 拷进 GlobalAlloc 出来的内存：
	// GlobalLock 返回的是 uintptr，直接转 unsafe.Pointer 会被 go vet 判为
	// unsafeptr（无法证明来源）；拷贝则只需要 Pointer→uintptr 这一安全方向。
	buf := make([]byte, dropFilesHeaderSize+len(list)*2)
	binary.LittleEndian.PutUint32(buf[0:4], dropFilesHeaderSize) // pFiles：清单相对 HDROP 的偏移
	binary.LittleEndian.PutUint32(buf[16:20], 1)                 // fWide = TRUE
	for i, u := range list {
		binary.LittleEndian.PutUint16(buf[dropFilesHeaderSize+i*2:], u)
	}

	h, _, _ := procGlobalAlloc.Call(0x0042, uintptr(len(buf))) // GMEM_MOVEABLE|GMEM_ZEROINIT
	if h == 0 {
		t.Fatal("GlobalAlloc 失败")
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		t.Fatal("GlobalLock 失败")
	}
	procRtlMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	runtime.KeepAlive(buf)
	procGlobalUnlock.Call(h)
	return h
}

func TestFileDropQueueMergesBatches(t *testing.T) {
	win := &Window{}
	win.queueDrops([]string{`C:\a.txt`})
	win.queueDrops([]string{`C:\b.png`, `C:\c.jpg`})

	got := win.takeDrops()
	want := []string{`C:\a.txt`, `C:\b.png`, `C:\c.jpg`}
	if len(got) != len(want) {
		t.Fatalf("取到 %d 个路径，期望 %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 个路径 = %q, want %q", i, got[i], want[i])
		}
	}
	if rest := win.takeDrops(); rest != nil {
		t.Fatalf("取走后队列未清空：%v", rest)
	}
}

// 拖放路径必须经 PollEvent 交出去，且只交一次。
func TestPollEventDeliversFileDrop(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(120, 80, "filedrop-poll")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()

	files := []string{`C:\a.txt`, `C:\b.png`}
	win.InjectEvent(Event{Kind: EventFilesDropped, Files: files})

	ev, ok := win.PollEvent()
	if !ok {
		t.Fatal("注入的拖放事件没有交出来")
	}
	if ev.Kind != EventFilesDropped {
		t.Fatalf("事件种类 = %v, want EventFilesDropped", ev.Kind)
	}
	if len(ev.Files) != len(files) || ev.Files[0] != files[0] || ev.Files[1] != files[1] {
		t.Fatalf("路径 = %q, want %q", ev.Files, files)
	}

	// 一次拖放只投递一次：后面的轮询即便还有别的平台事件，也不该再冒出
	// 一个拖放事件（否则应用会把同一批文件打开两遍）。
	for {
		ev, ok := win.PollEvent()
		if !ok {
			break
		}
		if ev.Kind == EventFilesDropped {
			t.Fatalf("同一批路径被重复投递：%q", ev.Files)
		}
	}
}

// 端到端：WM_DROPFILES → 窗口过程 → DragQueryFileW（UTF-16 → UTF-8）→
// 待处理队列 → PollEvent。Close 时窗口过程必须已恢复。
func TestFileDropMessageDeliversPaths(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(120, 80, "filedrop-msg")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()

	hwnd := win.hwnd()
	if hwnd == 0 {
		t.Skip("当前环境拿不到原生窗口（无头/ dummy 驱动），跳过消息级用例")
	}
	if win.dropOldProc == 0 {
		t.Fatal("有原生窗口却没有子类化窗口过程：WM_DROPFILES 收不到")
	}

	want := []string{`C:\pics\a.png`, `C:\图片\测试 1.jpg`}
	hdrop := makeHDROP(t, want)
	if r, _, _ := procPostMessageW.Call(hwnd, wmDropFiles, hdrop, 0); r == 0 {
		t.Fatal("PostMessage(WM_DROPFILES) 失败")
	}

	// 消息要等窗口线程泵消息（PollEvent 内部）才被派发。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ev, ok := win.PollEvent(); ok && ev.Kind == EventFilesDropped {
			if len(ev.Files) != len(want) {
				t.Fatalf("收到 %d 个路径 (%q)，期望 %d 个", len(ev.Files), ev.Files, len(want))
			}
			for i := range want {
				if ev.Files[i] != want[i] {
					t.Fatalf("第 %d 个路径 = %q, want %q（UTF-16 → UTF-8 转换有误）", i, ev.Files[i], want[i])
				}
			}
			return
		}
		sdlDelay(2)
	}
	t.Fatal("超时：WM_DROPFILES 没有变成 EventFilesDropped")
}

// 重复关闭拖放接收必须是空操作（Close 与手动 disable 都走这里）。
func TestDisableFileDropIdempotent(t *testing.T) {
	win := &Window{}
	disableFileDrop(win) // 未启用
	if win.dropOldProc != 0 {
		t.Fatalf("未启用时 disable 改动了窗口过程：%#x", win.dropOldProc)
	}
}
