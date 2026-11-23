//go:build windows

package sdl2

import (
	"os"
	"testing"

	"github.com/go-comm/ghromex/core"
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
	g.DrawSprite(10, 10, &core.Sprite{W: 4, H: 4, Pix: pix})

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

// 同一个精灵画多次，纹理只应上传一次。
//
// 精灵像素创建后不变（引擎按「半径+颜色」缓存复用同一份），所以纹理可以
// 只上传一次：每次绘制都 SDL_UpdateTexture 的话，一帧几百次纹理上传会吃掉
// 大半帧时间（实测门户页 300 次/帧 ≈ 2ms）——240Hz 屏幕的帧预算只有
// 4.2ms，超支就掉帧、滚动发顿。
func TestSpriteTextureUploadedOnce(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(64, 64, "sprite-cache")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g
	g.Clear(255, 255, 255, 255)

	pix := make([]byte, 4*4*4)
	for i := 0; i < 16; i++ {
		pix[i*4+0] = 0xFF
		pix[i*4+1] = 0x4D
		pix[i*4+2] = 0x00
		pix[i*4+3] = 0x80
	}
	sprite := &core.Sprite{W: 4, H: 4, Pix: pix}
	// 先画一次：像素应是半透明橙（与白底混合）。
	g.DrawSprite(10, 10, sprite)
	raw, _, _, _, err := win.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	i := (12*64 + 12) * 4
	got := [3]uint8{raw[i], raw[i+1], raw[i+2]}
	if got[0] < 250 || got[1] < 150 || got[1] > 180 || got[2] < 110 || got[2] > 145 {
		t.Fatalf("精灵像素 = %v，期望约 (255,166,127)", got)
	}
	// 再画 49 次：纹理不该重传（叠加会让颜色变实，这是预期的）。
	for i := 0; i < 49; i++ {
		g.DrawSprite(10, 10, sprite)
	}
	if g.spriteUploads != 1 {
		t.Errorf("纹理上传次数 = %d, want 1（精灵像素不变，不该每次绘制都重传）", g.spriteUploads)
	}
	if len(g.sprites) != 1 {
		t.Errorf("精灵纹理缓存条目 = %d, want 1", len(g.sprites))
	}
}

// 精灵纹理缓存必须有上限。
//
// 精灵的键里含颜色：页面若逐帧改圆角盒的颜色（动画），会不断产生新精灵，
// 纹理缓存不淘汰就是显存泄漏。
func TestSpriteTextureCacheIsBounded(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(64, 64, "sprite-cache-limit")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g
	g.Clear(255, 255, 255, 255)

	pix := make([]byte, 4*4*4)
	for i := range pix {
		pix[i] = uint8(i)
	}
	// 每次都是新精灵（模拟逐帧换色）：缓存必须自己淘汰。
	for i := 0; i < maxCachedSprites+100; i++ {
		g.DrawSprite(0, 0, &core.Sprite{W: 4, H: 4, Pix: pix})
	}
	if len(g.sprites) > maxCachedSprites {
		t.Errorf("精灵纹理缓存条目 = %d, want <= %d", len(g.sprites), maxCachedSprites)
	}
	if g.spritePixels > maxCachedSpritePixels {
		t.Errorf("精灵纹理像素 = %d, want <= %d", g.spritePixels, maxCachedSpritePixels)
	}
}

// shotRGB 读窗口渲染目标某点的 RGB（不需 Present）。
func shotRGB(t *testing.T, win *Window, x, y int) [3]uint8 {
	t.Helper()
	raw, _, w, _, err := win.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	i := (y*w + x) * 4
	return [3]uint8{raw[i], raw[i+1], raw[i+2]}
}

// fillRGBA 把整块像素填成同一 RGBA。
func fillRGBA(p []byte, r, g, b, a uint8) {
	for i := 0; i+3 < len(p); i += 4 {
		p[i], p[i+1], p[i+2], p[i+3] = r, g, b, a
	}
}

// Rev 递增时，同指针精灵只重传像素（复用纹理对象），缓存条目恒定 1 条。
func TestSpriteRevReuploadsPixels(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(64, 64, "sprite-rev")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g

	pix := make([]byte, 4*4*4)
	sprite := &core.Sprite{W: 4, H: 4, Pix: pix}

	fillRGBA(pix, 255, 0, 0, 255) // 红
	g.Clear(255, 255, 255, 255)
	g.DrawSprite(10, 10, sprite)
	if got := shotRGB(t, win, 12, 12); got != [3]uint8{255, 0, 0} {
		t.Fatalf("第一次绘制像素 = %v，期望红色", got)
	}
	if g.spriteUploads != 1 {
		t.Fatalf("上传次数 = %d，期望 1", g.spriteUploads)
	}

	// 就地改写像素 + Rev++：应重传（复用同一纹理对象），画面变绿。
	fillRGBA(pix, 0, 255, 0, 255)
	sprite.Rev++
	g.Clear(255, 255, 255, 255)
	g.DrawSprite(10, 10, sprite)
	if got := shotRGB(t, win, 12, 12); got != [3]uint8{0, 255, 0} {
		t.Fatalf("Rev++ 后像素 = %v，期望绿色（增量重传没生效）", got)
	}
	if g.spriteUploads != 2 {
		t.Errorf("上传次数 = %d，期望 2（Rev 变化应重传一次）", g.spriteUploads)
	}
	if len(g.sprites) != 1 {
		t.Errorf("缓存条目 = %d，期望 1（纹理对象应复用，不新建）", len(g.sprites))
	}
}

// Rev 不变的语义是「像素不可变」：改写 Pix 却不 Rev++ 时，纹理不重传、
// 画面冻结在旧内容上——这条用测试锁住契约，防止有人误以为改 Pix 就够。
func TestSpritePixelsFrozenWithoutRevBump(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(64, 64, "sprite-rev-stale")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g

	pix := make([]byte, 4*4*4)
	fillRGBA(pix, 0, 255, 0, 255) // 绿
	sprite := &core.Sprite{W: 4, H: 4, Pix: pix, Rev: 7}
	g.Clear(255, 255, 255, 255)
	g.DrawSprite(10, 10, sprite)

	// 改写为蓝但不改 Rev：命中缓存，不应重传。
	fillRGBA(pix, 0, 0, 255, 255)
	g.Clear(255, 255, 255, 255)
	g.DrawSprite(10, 10, sprite)
	if got := shotRGB(t, win, 12, 12); got != [3]uint8{0, 255, 0} {
		t.Fatalf("像素 = %v，期望仍是绿色（Rev 未变应冻结旧纹理）", got)
	}
	if g.spriteUploads != 1 {
		t.Errorf("上传次数 = %d，期望 1（Rev 未变不该重传）", g.spriteUploads)
	}
}

// RenderCopy 失败时应触发自愈：把失效纹理移出缓存、记账归零，下一帧
// 缓存 miss 重建后恢复画面——从「永久黑屏」降为「闪烁一帧」。
func TestRenderCopySelfHealsInvalidTexture(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(64, 64, "sprite-heal")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g

	pix := make([]byte, 4*4*4)
	fillRGBA(pix, 255, 0, 0, 255) // 红
	sprite := &core.Sprite{W: 4, H: 4, Pix: pix}
	g.Clear(255, 255, 255, 255)
	g.DrawSprite(10, 10, sprite)
	if len(g.sprites) != 1 || g.spriteUploads != 1 {
		t.Fatalf("初始状态异常：条目=%d 上传=%d", len(g.sprites), g.spriteUploads)
	}

	// 注入 RenderCopy 失败（纹理句柄本身合法，故 invalidate 时 destroy 安全）。
	func() {
		prev := renderCopyHook
		renderCopyHook = func(_, _, _, _ uintptr) uintptr { return 1 }
		defer func() { renderCopyHook = prev }()
		g.Clear(255, 255, 255, 255)
		g.DrawSprite(10, 10, sprite) // 命中缓存 → RenderCopy 失败 → invalidate
	}()

	if len(g.sprites) != 0 {
		t.Fatalf("失效纹理仍在缓存：条目=%d，期望 0", len(g.sprites))
	}
	if g.spritePixels != 0 {
		t.Fatalf("失效纹理记账未归零：spritePixels=%d", g.spritePixels)
	}
	if g.texInvalidations != 1 {
		t.Fatalf("自愈计数 = %d，期望 1", g.texInvalidations)
	}

	// 下一帧：缓存 miss 重建纹理并成功绘制。
	g.Clear(255, 255, 255, 255)
	g.DrawSprite(10, 10, sprite)
	if len(g.sprites) != 1 {
		t.Fatalf("重建后条目 = %d，期望 1", len(g.sprites))
	}
	if g.spriteUploads != 2 {
		t.Errorf("重建后上传次数 = %d，期望 2", g.spriteUploads)
	}
	if got := shotRGB(t, win, 12, 12); got != [3]uint8{255, 0, 0} {
		t.Fatalf("重建后像素 = %v，期望红色（自愈没恢复画面）", got)
	}
}

// 单帧自愈次数必须有上限：RenderCopy 持续失败时，超过上限只记日志、
// 不再反复 invalidate（否则每帧都变成「全量重建 → 又失败」的病理循环）。
func TestRenderCopyInvalidationCap(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(64, 64, "sprite-cap")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g

	pix := make([]byte, 4*4*4)
	fillRGBA(pix, 255, 0, 0, 255)
	sprite := &core.Sprite{W: 4, H: 4, Pix: pix}

	prev := renderCopyHook
	renderCopyHook = func(_, _, _, _ uintptr) uintptr { return 1 }
	defer func() { renderCopyHook = prev }()

	g.Clear(255, 255, 255, 255) // 清零自愈计数
	for i := 0; i < maxTextureInvalidationsPerFrame+10; i++ {
		g.DrawSprite(0, 0, sprite)
	}
	if g.texInvalidations != maxTextureInvalidationsPerFrame {
		t.Fatalf("自愈计数 = %d，期望封顶 %d", g.texInvalidations, maxTextureInvalidationsPerFrame)
	}
	// 达到上限后不再 invalidate，最后一次重建的条目留在缓存里。
	if len(g.sprites) != 1 {
		t.Fatalf("达到上限后条目 = %d，期望 1（不再驱逐）", len(g.sprites))
	}
}
