package main

import (
	"image"
	"image/color"
	"testing"
)

func TestNormalizeRect(t *testing.T) {
	// 从右下往左上拖：必须归一化成左上/右下。
	if r := normalizeRect(10, 20, 2, 5); r != image.Rect(2, 5, 10, 20) {
		t.Fatalf("归一化 = %v，期望 (2,5)-(10,20)", r)
	}
	if r := normalizeRect(3, 4, 8, 9); r != image.Rect(3, 4, 8, 9) {
		t.Fatalf("正向拖拽被改了: %v", r)
	}
}

func TestCropRGBA(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	src.SetRGBA(2, 3, color.RGBA{11, 22, 33, 255})
	src.SetRGBA(9, 9, color.RGBA{44, 55, 66, 255})

	dst := cropRGBA(src, image.Rect(2, 3, 5, 6))
	if got := dst.Bounds().Dx(); got != 3 {
		t.Fatalf("裁剪宽 = %d，期望 3", got)
	}
	if got := dst.Bounds().Dy(); got != 3 {
		t.Fatalf("裁剪高 = %d，期望 3", got)
	}
	if got := dst.RGBAAt(0, 0); got != (color.RGBA{11, 22, 33, 255}) {
		t.Fatalf("裁剪原点 = %v，期望 (11,22,33,255)", got)
	}
	// 越界部分被交集裁掉，不会 panic。
	out := cropRGBA(src, image.Rect(8, 8, 20, 20))
	if out.Bounds().Dx() != 2 || out.Bounds().Dy() != 2 {
		t.Fatalf("越界裁剪尺寸 = %v", out.Bounds())
	}
}

func TestClampWindowPos(t *testing.T) {
	cases := []struct {
		name               string
		x, y, w, h, sw, sh int
		wantX, wantY       int
	}{
		{"窗口内不动", 10, 10, 50, 30, 200, 150, 10, 10},
		{"右下越界贴边", 180, 140, 50, 30, 200, 150, 150, 120},
		{"负坐标贴左上", -20, -10, 50, 30, 200, 150, 0, 0},
		{"窗口比屏幕大", 30, 30, 300, 200, 200, 150, 0, 0},
	}
	for _, c := range cases {
		x, y := clampWindowPos(c.x, c.y, c.w, c.h, c.sw, c.sh)
		if x != c.wantX || y != c.wantY {
			t.Errorf("%s: 位置 = (%d,%d)，期望 (%d,%d)", c.name, x, y, c.wantX, c.wantY)
		}
	}
}

func TestPreviewLayout(t *testing.T) {
	if barTop, winH := previewLayout(117, 200); barTop != 117 || winH != 145 {
		t.Fatalf("普通选区: barTop=%d winH=%d，期望 117/145", barTop, winH)
	}
	// 图片 + 工具条超过屏幕：工具条改叠在图片底部，窗口高度不超屏。
	if barTop, winH := previewLayout(190, 200); barTop != 162 || winH != 190 {
		t.Fatalf("满屏选区: barTop=%d winH=%d，期望 162/190", barTop, winH)
	}
}

func TestOpaqueAndSprite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{1, 2, 3, 0})
	opaque(img)
	if a := img.Pix[3]; a != 255 {
		t.Fatalf("alpha = %d，期望 255", a)
	}
	sp := spriteOf(img)
	if sp.W != 2 || sp.H != 2 || len(sp.Pix) != 16 {
		t.Fatalf("精灵 = %dx%d len=%d", sp.W, sp.H, len(sp.Pix))
	}
}
