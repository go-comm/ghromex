package engine_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// DecodeImage 是宿主把「手里的图片字节」换成位图的入口（窗口图标、预解码缓存
// 等用它），与 <img> 共用 decodeImageBytes：格式支持面与像素口径必须一致。
func TestDecodeImageFormats(t *testing.T) {
	const w, h = 16, 16
	// 品红 (255,0,255)：PNG/JPEG/GIF 三种编码器都能精确/近精确表示
	// （GIF 调色板色可精确命中），逐像素断言才有意义。
	const r, g, b = 255, 0, 255
	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}

	encode := map[string]func() []byte{
		"PNG": func() []byte {
			var b bytes.Buffer
			if err := png.Encode(&b, src); err != nil {
				t.Fatalf("png.Encode: %v", err)
			}
			return b.Bytes()
		},
		"JPEG": func() []byte {
			var b bytes.Buffer
			if err := jpeg.Encode(&b, src, nil); err != nil {
				t.Fatalf("jpeg.Encode: %v", err)
			}
			return b.Bytes()
		},
		"GIF": func() []byte {
			var b bytes.Buffer
			if err := gif.Encode(&b, src, nil); err != nil {
				t.Fatalf("gif.Encode: %v", err)
			}
			return b.Bytes()
		},
	}
	// PNG/GIF 是无损格式，逐像素断言；JPEG 只给容差（有损，8bit 单位四舍五入
	// 与色度抽样都会带来几个单位的偏移）。
	tol := map[string]int{"PNG": 0, "JPEG": 4, "GIF": 0}
	for name, enc := range encode {
		t.Run(name, func(t *testing.T) {
			bmp, err := engine.DecodeImage(enc())
			if err != nil {
				t.Fatalf("DecodeImage: %v", err)
			}
			if bmp.W != w || bmp.H != h {
				t.Fatalf("尺寸 = %dx%d，期望 %dx%d", bmp.W, bmp.H, w, h)
			}
			need := bmp.W * bmp.H * 4
			if len(bmp.Pix) != need {
				t.Fatalf("像素长度 = %d，期望 %d（非预乘 RGBA 行主序）", len(bmp.Pix), need)
			}
			lim := tol[name]
			for i, want := range []int{r, g, b} {
				got := int(bmp.Pix[i])
				// 用 int 比：want-lim 在 want=0 时会向下溢出（uint32 下
				// 0-4 变成 4 亿多，任何像素都判不过）。
				if got < want-lim || got > want+lim {
					t.Fatalf("首像素分量[%d] = %d，期望 %d±%d（非预乘口径被改了？）", i, got, want, lim)
				}
			}
		})
	}
}

// 非图片字节必须报错：调用方拿它做「图标资源是否在」的判断，静默返回空位图
// 会把错误藏到运行期（图标黑一块、窗口却一切正常）。
func TestDecodeImageRejectsGarbage(t *testing.T) {
	if _, err := engine.DecodeImage([]byte("not an image")); err == nil {
		t.Fatal("DecodeImage(垃圾字节) 应报错")
	}
	if _, err := engine.DecodeImage(nil); err == nil {
		t.Fatal("DecodeImage(nil) 应报错")
	}
}
