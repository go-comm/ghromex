package main

import (
	"fmt"
	"image"
	"os"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

// overlay 是“冻结桌面 + 拖选”这一页。
//
// 背景图由窗口的 backdrop 提供，这一页只画四个暗化块（把选区抠亮）、
// 1px 边框、尺寸标签和十字线。全部用绝对定位 div，坐标在鼠标事件里
// 直接写进 style——引擎支持运行期改 style 并重排。
type overlay struct {
	app *app
	doc engine.HTMLDocument
	w   int
	h   int

	shadeT, shadeB, shadeL, shadeR engine.HTMLElement
	sel, label, hx, hy             engine.HTMLElement

	dragging bool
	ax, ay   int
	cur      image.Rectangle
}

const overlayHTML = `<!doctype html><html><head><style>
html,body{margin:0;padding:0;}
/* 遮罩只做「轻微压暗」：0.30 的深蓝黑既能把桌面推后、又看得清底层内容
   （0.46 时整屏像蒙了层灰，用户反馈"暗得太多"）。 */
#shade-t,#shade-b,#shade-l,#shade-r{position:absolute;background:rgba(10,16,30,0.30);}
#sel{position:absolute;display:none;border:1px solid #3B82F6;}
#size{position:absolute;display:none;background:rgba(15,23,42,0.9);color:#FFFFFF;font-size:12px;line-height:18px;padding:2px 8px;border-radius:4px;}
#hx,#hy{position:absolute;display:none;background:rgba(255,255,255,0.35);}
</style></head><body>
<div id="shade-t"></div><div id="shade-b"></div><div id="shade-l"></div><div id="shade-r"></div>
<div id="sel"></div><div id="size"></div><div id="hx"></div><div id="hy"></div>
</body></html>`

// startOverlay 装载选区页，并把整屏截图铺成窗口背景。
func (a *app) startOverlay() error {
	a.phase = phaseSelect
	a.win.SetBackdrop(spriteOf(a.desktop))
	doc, err := a.win.OpenDocument(overlayHTML)
	if err != nil {
		return err
	}
	a.doc = doc

	o := &overlay{app: a, doc: doc, w: a.w, h: a.h}
	o.shadeT = doc.GetElementById("shade-t")
	o.shadeB = doc.GetElementById("shade-b")
	o.shadeL = doc.GetElementById("shade-l")
	o.shadeR = doc.GetElementById("shade-r")
	o.sel = doc.GetElementById("sel")
	o.label = doc.GetElementById("size")
	o.hx = doc.GetElementById("hx")
	o.hy = doc.GetElementById("hy")
	if o.shadeT == nil || o.shadeB == nil || o.shadeL == nil || o.shadeR == nil ||
		o.sel == nil || o.label == nil || o.hx == nil || o.hy == nil {
		return fmt.Errorf("选区页元素缺失：overlayHTML 与元素 id 不同步")
	}
	o.reset()
	a.ov = o
	return nil
}

// reset 回到“未开始选择”：整屏压暗、隐藏选区/标签/十字线。
func (o *overlay) reset() {
	o.dragging = false
	o.cur = image.Rectangle{}
	o.box(o.shadeT, 0, 0, o.w, o.h)
	o.hide(o.shadeB, o.shadeL, o.shadeR, o.sel, o.label, o.hx, o.hy)
}

// box 写绝对定位几何。静止样式（颜色/边框）在 <style> 里，这里只覆盖
// left/top/width/height，所以每次整串重写也不会丢样式。
func (o *overlay) box(el engine.HTMLElement, x, y, w, h int) {
	if el == nil {
		return
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	el.SetAttribute("style", fmt.Sprintf(
		"display:block;left:%dpx;top:%dpx;width:%dpx;height:%dpx", x, y, w, h))
}

func (o *overlay) hide(els ...engine.HTMLElement) {
	for _, el := range els {
		if el != nil {
			el.SetAttribute("style", "display:none")
		}
	}
}

// cross 更新十字线：竖线贯穿全高，横线贯穿全宽。
func (o *overlay) cross(x, y int) {
	o.box(o.hx, x, 0, 1, o.h)
	o.box(o.hy, 0, y, o.w, 1)
}

// showRect 用四块遮罩把选区“抠”出来，并更新边框与尺寸标签。
func (o *overlay) showRect(r image.Rectangle) {
	r = r.Intersect(image.Rect(0, 0, o.w, o.h))
	o.cur = r

	o.box(o.shadeT, 0, 0, o.w, r.Min.Y)
	o.box(o.shadeB, 0, r.Max.Y, o.w, o.h-r.Max.Y)
	o.box(o.shadeL, 0, r.Min.Y, r.Min.X, r.Dy())
	o.box(o.shadeR, r.Max.X, r.Min.Y, o.w-r.Max.X, r.Dy())

	if r.Dx() <= 0 || r.Dy() <= 0 {
		o.hide(o.sel, o.label)
		return
	}
	o.box(o.sel, r.Min.X, r.Min.Y, r.Dx(), r.Dy())

	text := fmt.Sprintf("%d × %d", r.Dx(), r.Dy())
	o.label.SetText(text)
	lx, ly := r.Min.X, r.Min.Y-24
	if ly < 0 {
		ly = r.Min.Y + 4
	}
	if lw := len(text)*8 + 20; lx+lw > o.w {
		lx = o.w - lw
	}
	if lx < 0 {
		lx = 0
	}
	o.label.SetAttribute("style", fmt.Sprintf("display:block;left:%dpx;top:%dpx", lx, ly))
}

// selectRect 供无头演示使用：不经过鼠标，直接把选区摆好。
func (o *overlay) selectRect(r image.Rectangle) {
	o.dragging = false
	o.showRect(r)
	o.cross(r.Max.X-1, r.Max.Y-1)
}

// onMouse 处理选区页的鼠标事件：按下定锚点、移动实时更新、抬起进入预览。
func (a *app) onOverlayMouse(ev browser.MouseEvent) bool {
	o := a.ov
	switch ev.Kind {
	case browser.MouseDown:
		if ev.Button == sdl2.MouseButtonRight {
			a.win.RequestClose()
			return true
		}
		if ev.Button != sdl2.MouseButtonLeft {
			return true
		}
		x, y := clampPoint(ev.X, ev.Y, o.w, o.h)
		o.dragging = true
		o.ax, o.ay = x, y
		o.cross(x, y)
		o.showRect(image.Rect(x, y, x, y))
		return true

	case browser.MouseMove:
		x, y := clampPoint(ev.X, ev.Y, o.w, o.h)
		o.cross(x, y)
		if o.dragging {
			o.showRect(normalizeRect(o.ax, o.ay, x, y))
		}
		return true

	case browser.MouseUp:
		if ev.Button != sdl2.MouseButtonLeft || !o.dragging {
			return true
		}
		o.dragging = false
		r := o.cur
		if r.Dx() < minSel || r.Dy() < minSel {
			// 误触：回到待选状态，不退出。
			o.reset()
			return true
		}
		if err := a.startPreview(r); err != nil {
			fmt.Fprintln(os.Stderr, "进入预览失败:", err)
			a.win.RequestClose()
		}
		return true
	}
	return true
}

// clampPoint 把鼠标坐标收进窗口：拖动中鼠标可能跑出窗口，选区不能出现
// 负数/越界坐标（裁剪会算到图外）。
func clampPoint(x, y, w, h int) (int, int) {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x > w {
		x = w
	}
	if y > h {
		y = h
	}
	return x, y
}
