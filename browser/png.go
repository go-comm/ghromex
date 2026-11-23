// 本文件是截图字节 → PNG 的落盘编码；公开入口是 Window.ScreenshotPNG。

package browser

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// ScreenshotPNG 回读渲染目标当前内容并编码成 PNG 字节。
//
// 与 Screenshot 的分工：那个给原始 32bpp 内存（要自己拼 image.RGBA、或只做
// 像素断言时更灵活），这个直接出可写盘的 PNG——省掉每个调用方一遍通道转换
// （仓库里曾散落着七八份几乎相同的 raw→RGBA 循环）。
//
// 通道口径：SDL 回读的是 32bpp、小端内存序 R,G,B,X。渲染目标通常没有 alpha
// 通道，第 4 字节不是透明度（实测恒为 0），因此统一补 255——PNG 里画面全不透明。
//
// 线程口径同 Screenshot：必须在主循环的 goroutine 上调（跨线程用 Do 包一层）。
// PNG 编码本身在调用方 goroutine 完成，1024×720 约 3ms，主循环不该等它，
// 所以先取原始字节、再编码这一步留给调用方安排。
func (win *Window) ScreenshotPNG() ([]byte, error) {
	raw, format, w, h, err := win.sdl.Screenshot()
	if err != nil {
		return nil, err
	}
	return encodePNG(raw, format, w, h)
}

// encodePNG 把回读的 32bpp 内存编码成 PNG。format 是 SDL 的像素格式枚举，
// 只用来挡住「非 32bpp」这种读法根本不适用的情形（16bpp 背板极少见，
// 真遇到了应该改回读格式，而不是按 4 字节步长硬解）。
func encodePNG(raw []byte, format uint32, w, h int) ([]byte, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("browser: 截图尺寸非法 %dx%d", w, h)
	}
	if (format>>8)&0xFF != 32 {
		return nil, fmt.Errorf("browser: 渲染目标不是 32bpp（format=0x%08x）", format)
	}
	if len(raw) < w*h*4 {
		return nil, fmt.Errorf("browser: 截图字节数不足：%d < %d", len(raw), w*h*4)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		in := raw[y*w*4:]
		out := img.Pix[y*img.Stride:]
		for x := 0; x < w*4; x += 4 {
			o := x
			out[o], out[o+1], out[o+2], out[o+3] = in[x], in[x+1], in[x+2], 255
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
