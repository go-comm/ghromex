//go:build windows

// 窗口图标的端到端验证：SetIcon 之后，向 HWND 发 WM_GETICON 把系统里存的
// 图标句柄读回来（ICON_SMALL / ICON_BIG 都要有——标题栏用小、任务栏用大，
// 只设一个档位会出现「换了一半」的效果），再用 GetDIBits 把图标的颜色位图
// 回读，逐像素断言颜色没被格式转换弄错。
//
// 只在真机跑（dummy 驱动没有真实 HWND）：GHROMEX_REAL=1 时执行，否则跳过。
package sdl2

import (
	"os"
	"testing"
	"unsafe"

	"github.com/go-comm/ghromex/core"
	"golang.org/x/sys/windows"
)

var (
	procSendMessage  = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageW")
	procGetIconInfo  = windows.NewLazySystemDLL("user32.dll").NewProc("GetIconInfo")
	procGetDC        = windows.NewLazySystemDLL("user32.dll").NewProc("GetDC")
	procReleaseDC    = windows.NewLazySystemDLL("user32.dll").NewProc("ReleaseDC")
	procGetObject    = windows.NewLazySystemDLL("gdi32.dll").NewProc("GetObjectW")
	procGetDIBits    = windows.NewLazySystemDLL("gdi32.dll").NewProc("GetDIBits")
	procDeleteObject = windows.NewLazySystemDLL("gdi32.dll").NewProc("DeleteObject")
	procDestroyIcon  = windows.NewLazySystemDLL("user32.dll").NewProc("DestroyIcon")
)

// WM_GETICON（0x007F）：取窗口当前设置的图标句柄，wParam 是 ICON_SMALL/BIG。
const (
	wmGetIcon  = 0x007F
	iconSmall  = 0
	iconBig    = 1
	ditherType = 0 // BI_RGB
)

// iconInfo 对应 Win32 的 ICONINFO 结构（fIcon + 两个热点 + 两个位图句柄）。
type iconInfo struct {
	fIcon    uint32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

// bitMapInfo 对应 Win32 的 BITMAP 结构（GetObjectW 回读颜色位图句柄的属性）。
// 布局按头文件原样：4 个 LONG + 2 个 WORD + 指针。
type bitMapInfo struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

// testIconBitmap 生成一张 w×h 的纯品红（255,0,255,255）位图：纯色最干净，
// 任何缩放/格式转换的边界像素都还是品红，断言不受插值干扰。
func testIconBitmap(w, h int) *core.Bitmap {
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i] = 255   // R
		pix[i+1] = 0   // G
		pix[i+2] = 255 // B
		pix[i+3] = 255 // A
	}
	return &core.Bitmap{W: w, H: h, Pix: pix}
}

func TestSetIconSetsBothWindowsIcons(t *testing.T) {
	if os.Getenv("GHROMEX_REAL") != "1" {
		t.Skip("非真机（GHROMEX_REAL!=1），无真实 HWND 可回读图标")
	}
	win, err := NewWindowWithOptions(WindowOptions{
		Width: 320, Height: 240, Title: "icon-e2e",
	})
	if err != nil {
		t.Skipf("建窗失败（无显示环境？）: %v", err)
	}
	defer win.Close()

	bmp := testIconBitmap(32, 32)
	if err := win.SetIcon(bmp); err != nil {
		t.Fatalf("SetIcon: %v", err)
	}

	hwnd := win.hwnd()
	if hwnd == 0 {
		t.Fatal("取不到 HWND，无法回读图标")
	}
	for _, c := range []struct {
		name  string
		which uint32
	}{
		{"ICON_SMALL", iconSmall},
		{"ICON_BIG", iconBig},
	} {
		r, _, _ := procSendMessage.Call(hwnd, wmGetIcon, uintptr(c.which), 0)
		if r == 0 {
			t.Fatalf("%s 没设上：WM_GETICON 返回空句柄", c.name)
		}
		assertIconMagenta(t, c.name, r)
		procDestroyIcon.Call(r)
	}
}

// assertIconMagenta 回读 HICON 的颜色位图，断言中心块是品红。
//
// 图标的 HBITMAP 可能是 1bpp 单色或 32bpp 彩色：1bpp 说明系统没存 alpha
// 通道（格式不对），直接判失败。颜色位图按 32bpp BI_RGB 回读（字节序
// 00BBGGRR、自下而上），中心 8×8 区块应全是品红（源 32×32 纯色，SDL 缩
// 放后整块仍是品红；边缘可能被系统加边框，故只断言中心）。
func assertIconMagenta(t *testing.T, name string, hicon uintptr) {
	t.Helper()
	var info iconInfo
	ret, _, _ := procGetIconInfo.Call(hicon, uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		t.Fatalf("%s: GetIconInfo 失败", name)
	}
	defer procDeleteObject.Call(info.hbmMask)
	if info.hbmColor != 0 {
		defer procDeleteObject.Call(info.hbmColor)
	}
	if info.hbmColor == 0 {
		t.Fatalf("%s: HICON 没有颜色位图（可能是 1bpp 单色图标，SDL 没吃到 alpha）", name)
	}
	// 图标尺寸是动态的：ICON_SMALL 按 SM_CXSMICON（通常 16）、ICON_BIG 按
	// SM_CXICON（通常 32）。先问 GetObject 拿到真实宽高再分配回读缓冲。
	var bmpInfo bitMapInfo
	size, _, _ := procGetObject.Call(info.hbmColor, unsafe.Sizeof(bmpInfo),
		uintptr(unsafe.Pointer(&bmpInfo)))
	if size == 0 || bmpInfo.bmWidth <= 0 || bmpInfo.bmHeight <= 0 {
		t.Fatalf("%s: GetObject 读不到颜色位图尺寸（size=%d w=%d h=%d）",
			name, size, bmpInfo.bmWidth, bmpInfo.bmHeight)
	}
	w := int(bmpInfo.bmWidth)
	h := int(bmpInfo.bmHeight)
	if w < 16 || h < 16 {
		t.Fatalf("%s: 图标只有 %dx%d，比系统小档还小（格式可能错了）", name, w, h)
	}
	// DIB：按颜色位图真实位深回读（通常 32bpp BI_RGB，自下而上 biHeight 正数）。
	// BITMAPINFOHEADER 的字段偏移：biSize 0-3、biWidth 4-7、biHeight 8-11、
	// biPlanes 12-13、biBitCount 14-15、biCompression 16-19（勿按结构体字段
	// 顺序猜——DWORD 与 WORD 混排，差 4 字节就全错位，GetDIBits 静默回 0 行）。
	bpp := uint16(bmpInfo.bmBitsPixel)
	if bpp != 32 {
		// 32bpp 之外的位深（24/8/4/1）字节排布不同，本测试只锁 32bpp 品红；
		// 其它深度直接报，而不是按错格式去解出垃圾。
		t.Fatalf("%s: 颜色位图 %d bpp，测试只支持 32bpp", name, bpp)
	}
	var dib [40]byte
	copy(dib[0:4], u32le(40))           // biSize
	copy(dib[4:8], u32le(uint32(w)))    // biWidth
	copy(dib[8:12], u32le(uint32(h)))   // biHeight
	copy(dib[12:14], u16le(1))          // biPlanes
	copy(dib[14:16], u16le(bpp))        // biBitCount
	copy(dib[16:20], u32le(ditherType)) // biCompression = BI_RGB
	buf := make([]byte, w*h*4)
	// GetDIBits 需要有效 DC：取屏幕 DC（hdc 传 NULL 是未定义行为，会回 0 行）。
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		t.Fatalf("%s: GetDC 失败，无法回读图标位图", name)
	}
	n, _, _ := procGetDIBits.Call(hdc, info.hbmColor, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&dib[0])), 0)
	procReleaseDC.Call(0, hdc)
	if int(n) != h {
		t.Fatalf("%s: GetDIBits 回读 %d 行，期望 %d", name, n, h)
	}
	// 中心 8×8 区块：源图纯品红，缩放/原样都应全是品红。DIB 自下而上，但
	// 纯色整块都一样，方向不影响断言。
	ox, oy := (w-8)/2, (h-8)/2
	for y := oy; y < oy+8; y++ {
		for x := ox; x < ox+8; x++ {
			i := (y*w + x) * 4
			b, g, r := buf[i], buf[i+1], buf[i+2]
			if r != 255 || g != 0 || b != 255 {
				t.Fatalf("%s: 中心像素(%d,%d) = (r%d,g%d,b%d)，期望品红 (255,0,255)：格式转换错了？",
					name, x, y, r, g, b)
			}
		}
	}
}

func u32le(v uint32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
}

func u16le(v uint16) []byte {
	return []byte{byte(v), byte(v >> 8)}
}
