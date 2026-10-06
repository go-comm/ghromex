//go:build windows

package sdl2

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// ---------------------------------------------------------------------------
// select 展开浮层 LCD 可读性回归
//
// 用户反馈：改选中行为 #1967D2 蓝底白字后，"文字区域都变成了白的，看不到
// 是什么字"。根因在 LCD 落屏合成（graphics.go DrawText）：TTF_RenderUTF8_LCD
// 需要 paint.Background() 作为落点底色，Background=nil 时按白底合成——浮层
// option 自身无背景色、种子 Paint 不带底，白字配白底合成出纯白不透明纹理直贴
// 蓝底上，字形整个变实心白块。修复：renderSelectPopup 给选中项种子 Paint 携带
// selectHighlight 蓝底（引擎层 TestSelectPopupTextPaintCarriesBlue 锁 Paint
// 契约，本测试锁真实 LCD 落屏结果）。
//
// 校准口径：截图 4 字节序未知 → 黑底画纯 R/G/B 色块识别通道位；option 盒内
// 统计。修复后选中行：蓝底主导、纯白像素≈0（字形带亚像素毛边、不落纯白）；
// 白块回归时字形纹理区（约 26×16≈400+ 像素）整片纯白。
// ---------------------------------------------------------------------------

func TestSelectPopupLCDTextLegible(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")
	t.Setenv("GHROMEX_LCD", "1") // 文本默认灰度 AA，本测试专锁 LCD 路径，显式开启

	html, err := os.ReadFile(filepath.Join("..", "..", "demo", "form-ua.html"))
	if err != nil {
		t.Fatalf("read form-ua: %v", err)
	}

	win, err := NewWindow(400, 700, "select-lcd")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	if !lcdText() {
		t.Skip("DLL 无 TTF_RenderUTF8_LCD，LCD 路径不可用")
	}

	// 通道序校准：黑底画纯 R/G/B 色块，识别各通道在 4 字节中的位置。
	win.g.Clear(0, 0, 0, 255)
	win.g.DrawColor(300, 0, 10, 10, engine.NewColor(255, 0, 0, 255))
	win.g.DrawColor(320, 0, 10, 10, engine.NewColor(0, 255, 0, 255))
	win.g.DrawColor(340, 0, 10, 10, engine.NewColor(0, 0, 255, 255))
	win.g.Present()
	cbuf, _, cw, _, err := win.Screenshot()
	if err != nil {
		t.Fatalf("calib Screenshot: %v", err)
	}
	px := func(b []byte, w, x, y int) [4]byte {
		o := (y*w + x) * 4
		return [4]byte{b[o], b[o+1], b[o+2], b[o+3]}
	}
	r3 := px(cbuf, cw, 305, 5)
	g3 := px(cbuf, cw, 325, 5)
	b3 := px(cbuf, cw, 345, 5)
	idx := func(r, g, b [4]byte) int {
		for i := 0; i < 4; i++ {
			if r[i] == 255 && g[i] == 0 && b[i] == 0 {
				return i
			}
		}
		return -1
	}
	idxR, idxG, idxB := idx(r3, g3, b3), idx(g3, b3, r3), idx(b3, r3, g3)
	if idxR < 0 || idxG < 0 || idxB < 0 {
		t.Fatalf("通道校准失败: r=%v g=%v b=%v", r3, g3, b3)
	}

	// 加载页面 → 点 select 展开 → 渲染 → 截图。
	doc, err := win.OpenDocument(string(html))
	if err != nil {
		t.Fatal(err)
	}
	sel := doc.QuerySelector("select")
	if sel == nil {
		t.Fatal("select not found")
	}
	rc := sel.GetBoundingClientRect()
	engine.OnDocumentClick(doc,
		(rc.Left().Pixel()+rc.Right().Pixel())/2,
		(rc.Top().Pixel()+rc.Bottom().Pixel())/2)
	engine.LayoutDocument(doc)
	win.clearCanvas()
	engine.RenderNode(win.g, doc)
	win.present()
	buf, _, vw, vh, err := win.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, vw, vh))
	for y := 0; y < vh; y++ {
		for x := 0; x < vw; x++ {
			i, o := (y*vw+x)*4, (y*vw+x)*4
			img.Pix[o+0] = buf[i+idxR]
			img.Pix[o+1] = buf[i+idxG]
			img.Pix[o+2] = buf[i+idxB]
			img.Pix[o+3] = 255
		}
	}
	cmpSavePNG(t, filepath.Join(os.TempDir(), "opencode", "select_lcd_popup.png"), img)

	count := func(x0, y0, x1, y1 int, match func(r, g, b uint8) bool) int {
		if x0 < 0 {
			x0 = 0
		}
		if y0 < 0 {
			y0 = 0
		}
		if x1 > vw {
			x1 = vw
		}
		if y1 > vh {
			y1 = vh
		}
		n := 0
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				o := (y*vw + x) * 4
				if match(img.Pix[o], img.Pix[o+1], img.Pix[o+2]) {
					n++
				}
			}
		}
		return n
	}
	isWhite := func(r, g, b uint8) bool { return r == 255 && g == 255 && b == 255 }
	isBlue := func(r, g, b uint8) bool { return r == 0x19 && g == 0x67 && b == 0xD2 }

	// form-ua：option 序 bj/sh/gz，sh 选中 → option[1] 为蓝底白字行。
	opts := doc.QuerySelectorAll("option")
	if len(opts) != 3 {
		t.Fatalf("option 数=%d, want 3", len(opts))
	}
	boxes := make([][4]int, 3)
	for i, o := range opts {
		or := o.GetBoundingClientRect()
		boxes[i] = [4]int{
			int(or.Left().Pixel()) + 2, int(or.Top().Pixel()) + 2,
			int(or.Right().Pixel()) - 2, int(or.Bottom().Pixel()) - 2,
		}
	}
	selBox := boxes[1]
	area := (selBox[2] - selBox[0]) * (selBox[3] - selBox[1])
	white := count(selBox[0], selBox[1], selBox[2], selBox[3], isWhite)
	blue := count(selBox[0], selBox[1], selBox[2], selBox[3], isBlue)
	t.Logf("selected box=%v area=%d white=%d blue=%d", selBox, area, white, blue)

	// 白块回归（修复前）：LCD 白字×白底合成的字形纹理整片纯白 ≈ 400+ 像素。
	if white > 40 {
		t.Errorf("选中行纯白像素 %d 个（>40）：字形被渲成实心白块（LCD 白字按白底合成？）", white)
	}
	// 选中行必须是 #1967D2 高亮主导，否则整体配色回退。
	if blue*100 < area*60 {
		t.Errorf("选中行 #1967D2 占 %d/%d（<60%%）：高亮底没铺上", blue, area)
	}
	// 未选中行必须仍有字（非纯白）：防止顺手把文字颜色统一成白后两边都白。
	for _, i := range []int{0, 2} {
		b := boxes[i]
		w := count(b[0], b[1], b[2], b[3], isWhite)
		a := (b[2] - b[0]) * (b[3] - b[1])
		if w*100 > a*95 {
			t.Errorf("未选中行 option[%d] 纯白 %d/%d（>95%%）：文字没画出来", i, w, a)
		}
	}
}
