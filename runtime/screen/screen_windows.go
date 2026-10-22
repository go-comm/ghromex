//go:build windows

package screen

import (
	"fmt"
	"image"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 桌面抓屏的 Win32 绑定。
//
// 不引入 CGO：x/sys/windows 只用来拿 LazySystemDLL，函数地址在首次调用
// 时解析。gdi32 的调用约定与 syscall.SyscallN 一致（stdcall on 386 /
// Microsoft x64），Proc.Call 已经处理。
var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	// DPI 申报是进程级一次性动作（重复调用无害，但没必要）。
	dpiAwareOnce sync.Once

	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	procSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procBitBlt             = gdi32.NewProc("BitBlt")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
)

const (
	// GetSystemMetrics 的主屏指标（像素）。capture 进入时先申报
	// system-DPI-aware，所以这里拿到的是物理像素，与窗口坐标一致。
	smCXScreen = 0
	smCYScreen = 1

	// BITMAPINFOHEADER 的固定值。
	biRGB        = 0 // BI_RGB
	dibRGBColors = 0 // DIB_RGB_COLORS

	// BitBlt 光栅操作：直接拷贝；CAPTUREBLT 把分层窗口（部分游戏/播放器）
	// 也纳入拷贝，否则截出来会缺一块。
	srcCopy    = 0x00CC0020
	captureBlt = 0x40000000
)

// bitmapInfoHeader 对应 Win32 的 BITMAPINFOHEADER（40 字节）。
// 字段顺序与宽度必须逐字节对齐，不能随意改成更宽的 int。
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// bitmapInfo 是 BITMAPINFOHEADER + 一个颜色项。DIB_RGB_COLORS 下颜色项
// 实际不被读取，但结构体大小要够。
type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

// capture 用 GDI 把主屏 BitBlt 进一块 32bpp 的 DIB，再转成 RGBA。
//
// 选 DIB 而不是 CreateCompatibleBitmap + GetDIBits：DIB 的像素直接写进
// 进程内存，BitBlt 完就能读，少一次整屏拷贝。
func capture() (*image.RGBA, error) {
	// 必须在取屏幕尺寸**之前**申报 DPI 感知：进程默认 DPI-unaware 时，
	// GetSystemMetrics 返回的是被系统缩放后的逻辑尺寸（150% 下 2560 宽的
	// 屏会报 1706），而屏幕 DC 仍是物理像素——按逻辑尺寸 BitBlt 只会截到
	// 左上角一块，且之后 SDL 建窗（它自己会申报感知）尺寸对不上。
	// 与 backend/sdl2 的申报一致：系统级 DPI 感知，主屏 1:1。
	dpiAwareOnce.Do(func() { procSetProcessDPIAware.Call() })

	w, h := getSystemMetrics(smCXScreen), getSystemMetrics(smCYScreen)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("主屏尺寸非法: %dx%d", w, h)
	}

	// NULL 表示整个屏幕的 DC。
	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return nil, fmt.Errorf("GetDC(NULL) 失败")
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(hdcMem)

	bi := bitmapInfo{}
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = int32(w)
	// 负高度 = top-down DIB：内存第一行就是屏幕最上面一行，省掉翻转。
	bi.Header.Height = -int32(h)
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = biRGB

	var bits unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(
		hdcMem,
		uintptr(unsafe.Pointer(&bi)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hbmp == 0 || bits == nil {
		return nil, fmt.Errorf("CreateDIBSection 失败")
	}
	// 先恢复旧位图再删除：DIB 仍被 DC 选中时 DeleteObject 会失败。
	// defer 是后进先出，所以「删除」先注册、「恢复」后注册，实际执行
	// 顺序才是恢复 → 删除。
	old, _, _ := procSelectObject.Call(hdcMem, hbmp)
	defer procDeleteObject.Call(hbmp)
	defer procSelectObject.Call(hdcMem, old)

	ret, _, _ := procBitBlt.Call(
		hdcMem, 0, 0, uintptr(w), uintptr(h),
		hdcScreen, 0, 0,
		srcCopy|captureBlt,
	)
	if ret == 0 {
		// CAPTUREBLT 在少数驱动/会话里不被支持；退回普通 SRCCOPY，
		// 代价只是可能漏掉分层窗口（大部分桌面不受影响）。
		ret, _, _ = procBitBlt.Call(
			hdcMem, 0, 0, uintptr(w), uintptr(h),
			hdcScreen, 0, 0,
			srcCopy,
		)
	}
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt 失败")
	}

	// bits 指向 w*h*4 字节，直到 hbmp 被删除都有效（defer 在函数返回后
	// 才执行，这里读是安全的）。
	src := unsafe.Slice((*byte)(bits), w*h*4)
	return bgraToRGBA(src, w, h), nil
}

func getSystemMetrics(index int) int {
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int(int32(r))
}

// bgraToRGBA 把 GDI 的 BGRA 像素转成 image.RGBA，并把 alpha 补成 255。
// 单独拎出来是为了能脱离真实桌面做单元测试。
func bgraToRGBA(src []byte, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	n := w * h * 4
	if n > len(src) {
		n = len(src) / 4 * 4
	}
	for i, j := 0, 0; i+3 < n && j+3 < len(img.Pix); i, j = i+4, j+4 {
		img.Pix[j] = src[i+2]
		img.Pix[j+1] = src[i+1]
		img.Pix[j+2] = src[i]
		img.Pix[j+3] = 255
	}
	return img
}
