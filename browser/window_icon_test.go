//go:build windows

// 窗口图标（标题栏 + 任务栏）的入口在 browser：Options.Icon 建窗即设、
// SetIcon 运行期换。这里锁三条口径：合法位图要成功、空位图要报错、
// 建窗传参不阻断建窗。
//
// dummy 驱动下 SDL 的 SetWindowIcon 仍走 Windows 后端的实现（它只依赖
// HWND，不依赖真实显示），所以断言在 CI 上也有意义；图标像素画在标题栏，
// 不进 Screenshot，故不比对像素。
package browser

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/go-comm/ghromex/core"
	"github.com/go-comm/ghromex/engine"
)

// newTestWindowWithOptions 同 newTestWindow，但要自定义建窗参数（图标用例）。
func newTestWindowWithOptions(t *testing.T, o Options) *Window {
	t.Helper()
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	t.Cleanup(func() { os.Unsetenv("SDL_VIDEO_DRIVER") })
	win, err := NewWithOptions(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(win.Close)
	return win
}

// iconPNG 生成一张 w×h 的品红 PNG 字节（engine.DecodeImage 的输入）。
func iconPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 30, B: 140, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestSetIconAcceptsDecodedBitmap(t *testing.T) {
	win := newTestWindow(t, 64, 48, "icon-test")

	bmp, err := engine.DecodeImage(iconPNG(t, 4, 4))
	if err != nil {
		t.Fatalf("DecodeImage: %v", err)
	}
	if err := win.SetIcon(bmp); err != nil {
		t.Fatalf("SetIcon: %v", err)
	}
}

// 空位图与「尺寸对不上像素长度」都要报错而不是打到 SDL：后者会让 surface
// 建出来却越界读 Go 堆（SDL 只借用像素、不拷贝）。
func TestSetIconRejectsBadBitmap(t *testing.T) {
	win := newTestWindow(t, 64, 48, "icon-bad-test")

	if err := win.SetIcon(nil); err == nil {
		t.Error("SetIcon(nil) 应报错")
	}
	if err := win.SetIcon(&core.Bitmap{W: 4, H: 4, Pix: make([]byte, 4)}); err == nil {
		t.Error("像素长度不足应报错")
	}
	if err := win.SetIcon(&core.Bitmap{W: 0, H: 8, Pix: make([]byte, 4)}); err == nil {
		t.Error("尺寸非法应报错")
	}
}

// Options.Icon 在建窗路径里就设上（首帧显示前，避免闪一下默认图标）；
// 图标失败不该让建窗失败。
func TestNewWithOptionsSetsIcon(t *testing.T) {
	bmp, err := engine.DecodeImage(iconPNG(t, 8, 8))
	if err != nil {
		t.Fatalf("DecodeImage: %v", err)
	}
	win := newTestWindowWithOptions(t, Options{
		Width: 80, Height: 60, Title: "icon-opt-test",
		Centered: true, Icon: bmp,
	})
	if win == nil {
		t.Fatal("带 Icon 建窗失败")
	}
}
