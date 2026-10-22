package main

import (
	"image"
	"image/draw"

	"github.com/go-comm/ghromex/core"
)

// normalizeRect 把任意方向的拖拽终点变成左上/右下为正的矩形。
//
// 截图时经常从右下往左上拖；fp 坐标不能假设先后顺序，否则会得到负宽高，
// 裁剪与遮罩尺寸全错。
func normalizeRect(x0, y0, x1, y1 int) image.Rectangle {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	return image.Rect(x0, y0, x1, y1)
}

// cropRGBA 复制 src 的 r 区域。
//
// 不用 SubImage：子图的 Pix 仍按原图 stride 排列，而 Sprite 要求连续
// 的 w*h*4 字节；直接画进一块新图最省心，也避免之后整屏图被意外回收。
func cropRGBA(src *image.RGBA, r image.Rectangle) *image.RGBA {
	r = r.Intersect(src.Bounds())
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}

// spriteOf 把不透明 RGBA 包成窗口背景精灵。
//
// 调用方必须保证像素 alpha=255：DrawSprite 走 alpha 混合，alpha=0 的
// 像素不会覆盖底色（整张图会“消失”）。
func spriteOf(img *image.RGBA) *core.Sprite {
	b := img.Bounds()
	return &core.Sprite{W: b.Dx(), H: b.Dy(), Pix: img.Pix}
}

// opaque 把 alpha 统一补成 255。桌面抓屏由 runtime/screen 保证；从图片
// 文件加载的“假桌面”可能带透明通道，作为背景同样必须不透明。
func opaque(img *image.RGBA) {
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
}

// clampWindowPos 把预览窗夹进屏幕：窗口不能整块跑到屏幕外。
// w/h 放不下时贴左上角（宁可超出右边/底部，也不让标题区不可见）。
func clampWindowPos(x, y, w, h, sw, sh int) (int, int) {
	if w < sw {
		if x+w > sw {
			x = sw - w
		}
	} else {
		x = 0
	}
	if x < 0 {
		x = 0
	}
	if h < sh {
		if y+h > sh {
			y = sh - h
		}
	} else {
		y = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

// previewLayout 计算预览窗的图片区与工具条位置。
//
// 默认工具条在图片下方（图完整可见）；选区几乎占满屏幕时工具条会顶出
// 屏幕，改为叠在图片底部——窗口仍放得下。
func previewLayout(imgH, screenH int) (barTop, winH int) {
	barTop, winH = imgH, imgH+barH
	if winH > screenH {
		barTop, winH = imgH-barH, imgH
	}
	if barTop < 0 {
		barTop = 0
	}
	return barTop, winH
}
