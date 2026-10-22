// 示例：截图工具。
//
// 启动后冻结主屏，拖选一块区域；松开鼠标后同一个窗口原地缩成置顶、
// 无边框的悬浮预览，拖动图片可移动，Ctrl+S 保存 PNG，Esc/关闭退出。
//
//	go run ./examples/snip
//	go run ./examples/snip -delay 1s          # 留时间切换窗口再抓屏
//	go run ./examples/snip -source a.png -shot-overlay o.png -shot-preview p.png
//
// 只支持主屏：多显示器虚拟桌面（负坐标、混合 DPI）留给后续能力。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/runtime/dialog"
	"github.com/go-comm/ghromex/runtime/screen"
)

const (
	// minSel 是有效选区的最小边长：低于它的拖拽按误触处理，留在选区页。
	minSel = 8
	// barH 是预览窗底部工具条高度。
	barH = 28
)

type phase uint8

const (
	phaseSelect phase = iota
	phasePreview
)

type app struct {
	win     *browser.Window
	desktop *image.RGBA
	w, h    int // 桌面（也是选区窗口）尺寸

	phase phase
	doc   engine.HTMLDocument
	ov    *overlay
	prev  *previewState
}

func main() {
	delay := flag.Duration("delay", 0, "抓屏前等待，留时间切换/整理窗口")
	source := flag.String("source", "", "用图片代替桌面抓屏（调试/无头出图）")
	shotOverlay := flag.String("shot-overlay", "", "把选区页渲染成 PNG 后退出")
	shotPreview := flag.String("shot-preview", "", "把预览页渲染成 PNG 后退出")
	exitAfter := flag.Duration("exit-after", 0, "打开后指定时长自动退出（调试/CI）")
	flag.Parse()

	if err := run(*delay, *source, *shotOverlay, *shotPreview, *exitAfter); err != nil {
		fmt.Fprintln(os.Stderr, "snip:", err)
		os.Exit(1)
	}
}

func run(delay time.Duration, source, overlayPath, previewPath string, exitAfter time.Duration) error {
	var (
		desktop *image.RGBA
		err     error
	)
	if source != "" {
		desktop, err = loadImage(source)
	} else {
		if delay > 0 {
			time.Sleep(delay)
		}
		desktop, err = screen.Capture()
	}
	if err != nil {
		return err
	}

	b := desktop.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("桌面图像尺寸非法: %v", b)
	}

	win, err := browser.NewWithOptions(browser.Options{
		Width: w, Height: h, Title: "截图",
		X: 0, Y: 0,
		Borderless:  true,
		AlwaysOnTop: true,
		SkipTaskbar: true,
	})
	if err != nil {
		return err
	}
	defer win.Close()

	a := newApp(win, desktop)

	if overlayPath != "" || previewPath != "" {
		return a.renderShots(overlayPath, previewPath)
	}

	win.SetKeyHandler(a.onKey)
	win.SetMouseHandler(a.onMouse)
	if exitAfter > 0 {
		win.SetAutoExit(exitAfter)
	}
	if err := a.startOverlay(); err != nil {
		return err
	}
	return win.Run()
}

func newApp(win *browser.Window, desktop *image.RGBA) *app {
	b := desktop.Bounds()
	return &app{win: win, desktop: desktop, w: b.Dx(), h: b.Dy()}
}

// onMouse 按阶段分发：选区页接管所有事件，预览页只接管图片区拖动。
func (a *app) onMouse(ev browser.MouseEvent) bool {
	if a.phase == phasePreview && a.prev != nil {
		return a.prev.onMouse(a, ev)
	}
	return a.onOverlayMouse(ev)
}

func (a *app) onKey(key int, mods uint16) bool {
	if key == engine.KeyEscape {
		a.win.RequestClose()
		return true
	}
	if a.phase == phasePreview && mods&engine.ModCtrl != 0 && (key == 's' || key == 'S') {
		a.save()
		return true
	}
	return false
}

// save 弹系统另存为对话框并写 PNG。对话框是模态的：期间主循环暂停，
// 与原生应用一致。
func (a *app) save() {
	if a.prev == nil {
		return
	}
	hwnd, _ := a.win.NativeHandle()
	path, err := dialog.SaveFileDialog(dialog.Parent(hwnd), dialog.Options{
		Title:       "保存截图",
		DefaultName: time.Now().Format("screenshot-20060102-150405.png"),
		Filters: []dialog.FileFilter{
			{Name: "PNG 图片 (*.png)", Pattern: "*.png"},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "保存对话框失败:", err)
		return
	}
	if path == "" {
		return // 用户取消：不是错误
	}
	if !strings.EqualFold(filepath.Ext(path), ".png") {
		path += ".png"
	}
	if err := writePNG(path, a.prev.img); err != nil {
		fmt.Fprintln(os.Stderr, "写 PNG 失败:", err)
		return
	}
	fmt.Println("已保存:", path)
}

// renderShots 不走真实事件循环：摆一个固定选区、渲染选区页，再变形到
// 预览页各出一张 PNG。用 dummy SDL 驱动即可在无显示器环境检查观感。
func (a *app) renderShots(overlayPath, previewPath string) error {
	if err := a.startOverlay(); err != nil {
		return err
	}
	r := demoRect(a.w, a.h)
	a.ov.selectRect(r)
	if overlayPath != "" {
		if err := a.saveWindowPNG(overlayPath); err != nil {
			return err
		}
	}
	if err := a.startPreview(r); err != nil {
		return err
	}
	if previewPath != "" {
		if err := a.saveWindowPNG(previewPath); err != nil {
			return err
		}
	}
	return nil
}

// saveWindowPNG 重排当前文档、渲染一帧并回读窗口像素。
//
// RenderFrame 只负责“清屏 + 画当前布局”，不自己重排——改了文档/窗口
// 尺寸之后必须先 Refresh，否则截到的是旧布局。
func (a *app) saveWindowPNG(path string) error {
	if a.doc != nil {
		a.doc.Refresh()
	}
	a.win.RenderFrame()
	raw, _, w, h, err := a.win.Screenshot()
	if err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// 后端回读格式为 XBGR8888（小端内存序 R,G,B,X）：前三个字节就是
	// 颜色，第 4 字节不是 alpha，统一补 255。
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			o := y*img.Stride + x*4
			img.Pix[o] = raw[i]
			img.Pix[o+1] = raw[i+1]
			img.Pix[o+2] = raw[i+2]
			img.Pix[o+3] = 255
		}
	}
	return writePNG(path, img)
}

// demoRect 给无头演示挑一个不贴边的选区（留出尺寸标签的位置）。
func demoRect(w, h int) image.Rectangle {
	r := image.Rect(w/8, h/6, w*7/8, h*3/4)
	if r.Dx() < 40 {
		r.Max.X = r.Min.X + 40
	}
	if r.Dy() < 30 {
		r.Max.Y = r.Min.Y + 30
	}
	return r.Intersect(image.Rect(0, 0, w, h))
}

func loadImage(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	img := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(img, img.Bounds(), src, b.Min, draw.Src)
	// 假桌面可能带透明通道；作为窗口背景必须不透明（Sprite 走 alpha 混合）。
	opaque(img)
	return img, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
