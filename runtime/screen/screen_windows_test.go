//go:build windows

package screen

import "testing"

// BGRA → RGBA：通道顺序要换，alpha 必须补 255（否则精灵全透明）。
func TestBGRAtoRGBA(t *testing.T) {
	src := []byte{
		10, 20, 30, 0, // B,G,R,A → R=30
		40, 50, 60, 7,
	}
	img := bgraToRGBA(src, 2, 1)
	if got := img.Rect.Dx(); got != 2 {
		t.Fatalf("宽 = %d，期望 2", got)
	}
	if got := img.Pix[0]; got != 30 {
		t.Fatalf("R = %d，期望 30", got)
	}
	if got := img.Pix[1]; got != 20 {
		t.Fatalf("G = %d，期望 20", got)
	}
	if got := img.Pix[2]; got != 10 {
		t.Fatalf("B = %d，期望 10", got)
	}
	if got := img.Pix[3]; got != 255 {
		t.Fatalf("A = %d，期望 255", got)
	}
	if got := img.Pix[4]; got != 60 {
		t.Fatalf("第二个像素 R = %d，期望 60", got)
	}
}

// 真实抓屏：无桌面会话（服务、锁屏）时跳过，不把环境问题当代码失败。
func TestCapturePrimary(t *testing.T) {
	img, err := Capture()
	if err != nil {
		t.Skipf("当前会话没有可抓取的桌面: %v", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("抓取尺寸 = %v", img.Bounds())
	}
	if len(img.Pix) < 4 {
		t.Fatal("抓取结果没有像素")
	}
	if a := img.Pix[3]; a != 255 {
		t.Fatalf("首像素 alpha = %d，期望 255（BitBlt 不写 alpha，必须补）", a)
	}
}
