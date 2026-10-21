//go:build windows

package sdl2

import (
	"os"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 位图精灵（圆角角标）的上传与混合必须正确：
//
//   - 像素格式常量取错时，alpha 会被读成别的通道——表现是"整块不透明"
//     或"整块透明"，画面上表现为圆角变成方块/缺角，肉眼难察觉；
//   - 混合模式错了则半透明像素变成实心。
//
// 故这里用一张半透明橙色的 4×4 精灵直接断言像素值。
func TestDrawSpriteBlendAndFormat(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(64, 64, "sprite")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g
	g.Clear(255, 255, 255, 255)

	// 4×4 半透明橙：R=255 G=0x4D B=0 A=0x80
	pix := make([]byte, 4*4*4)
	for i := 0; i < 16; i++ {
		pix[i*4+0] = 0xFF
		pix[i*4+1] = 0x4D
		pix[i*4+2] = 0x00
		pix[i*4+3] = 0x80
	}
	g.DrawSprite(10, 10, &engine.Sprite{W: 4, H: 4, Pix: pix})

	raw, _, _, _, err := win.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	px := func(x, y int) [3]uint8 {
		i := (y*64 + x) * 4
		return [3]uint8{raw[i], raw[i+1], raw[i+2]}
	}
	got := px(12, 12)
	// 与白底 50% 混合：R=255、G=0x4D/2+255/2=166、B=0/2+255/2=127
	if got[0] < 250 || got[1] < 150 || got[1] > 180 || got[2] < 110 || got[2] > 145 {
		t.Fatalf("精灵像素 = %v，期望约 (255,166,127)（半透明混合或像素格式取错）", got)
	}
	// 精灵之外仍是白底
	if bg := px(5, 5); bg != [3]uint8{255, 255, 255} {
		t.Fatalf("精灵外像素 = %v，期望白底", bg)
	}
}
