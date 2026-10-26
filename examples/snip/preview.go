package main

import (
	"fmt"
	"image"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

// previewState 是“悬浮预览”这一页。
//
// 窗口里图片区 1:1 铺在背景层，工具条是页面元素；拖动图片区时直接
// 改窗口位置，工具条区域放行给引擎处理点击。
type previewState struct {
	img *image.RGBA

	w, h   int // 图片尺寸（也是选区尺寸）
	barTop int // 工具条在窗口内的 y
	winW   int
	winH   int

	x, y         int // 当前窗口位置
	dragging     bool
	grabX, grabY int // 按下时窗口左上角相对光标的偏移（窗口内坐标）
	// targetX/targetY 是本帧要落成的窗口位置，movePending 表示有待应用的
	// 位移。拖动时不在鼠标事件里直接 MoveTo：一个事件批次常有多条 motion，
	// 逐条重定位窗口会让它在一毫秒内跳好几次；帧末合并成一次（见 onFrame）。
	targetX, targetY int
	movePending      bool
}

// startPreview 把当前窗口从“整屏选区”原地缩成悬浮预览。
//
// 复用同一个窗口而不是关掉重开：sdl2.Window.Close 会调 SDL_Quit，
// 顺序开第二个窗会有明显桌面闪烁，还会丢焦点。原地改尺寸/位置/文档
// 则在一帧之内完成变化。
func (a *app) startPreview(r image.Rectangle) error {
	r = r.Intersect(image.Rect(0, 0, a.w, a.h))
	if r.Dx() < minSel || r.Dy() < minSel {
		return nil
	}

	img := cropRGBA(a.desktop, r)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	barTop, winH := previewLayout(h, a.h)
	x, y := clampWindowPos(r.Min.X, r.Min.Y, w, winH, a.w, a.h)

	a.phase = phasePreview
	a.prev = &previewState{
		img: img, w: w, h: h, barTop: barTop, winW: w, winH: winH, x: x, y: y,
	}

	a.win.SetBackdrop(spriteOf(img))
	doc, err := a.win.OpenDocument(previewHTML(w, h, barTop))
	if err != nil {
		return err
	}
	a.doc = doc

	if el := doc.GetElementById("save"); el != nil {
		el.OnClick(func(*engine.MouseEvent) { a.save() })
	}
	if el := doc.GetElementById("close"); el != nil {
		el.OnClick(func(*engine.MouseEvent) { a.win.RequestClose() })
	}

	// 先移动再改尺寸：两步在同一个事件批次里完成，主循环只 Present
	// 最终状态。Resize 会强制下一次布局，新文档按新视口排版。
	a.win.MoveTo(x, y)
	a.win.Resize(w, winH)
	return nil
}

// previewHTML 生成预览页。工具条默认在图片下方；选区几乎占满屏幕时
// barTop 会落在图片底部（见 previewLayout）。
func previewHTML(w, h, barTop int) string {
	// 工具条按钮：右下角两个 40px 小按钮。窗口太窄时至少保留“关闭”，
	// 保存仍可用 Ctrl+S，避免两个按钮叠在一起。
	closeX := w - 46
	saveX := w - 90
	saveStyle := ""
	if saveX < 2 {
		saveX = 2
		saveStyle = "display:none;"
	}
	if closeX < 2 {
		closeX = 2
	}
	info := "display:block;left:8px;top:7px"
	if w < 190 {
		info = "display:none"
	}
	return fmt.Sprintf(`<!doctype html><html><head><style>
html,body{margin:0;padding:0;}
#frame{position:absolute;left:0;top:0;width:100%%;height:%dpx;border:1px solid rgba(255,255,255,0.45);box-sizing:border-box;}
#bar{position:absolute;left:0;top:%dpx;width:100%%;height:%dpx;background:rgba(15,23,42,0.9);}
#info{position:absolute;color:#CBD5E1;font-size:11px;line-height:14px;}
.btn{position:absolute;top:4px;width:40px;height:20px;background:#334155;color:#FFFFFF;font-size:11px;line-height:20px;text-align:center;border-radius:4px;}
#save{left:%dpx;%s}
#close{left:%dpx;}
</style></head><body>
<div id="frame"></div>
<div id="bar">
  <span id="info" style="%s">%d × %d</span>
  <div class="btn" id="save">保存</div>
  <div class="btn" id="close">关闭</div>
</div>
</body></html>`, h, barTop, barH, saveX, saveStyle, closeX, info, w, h)
}

// onMouse 处理预览页的鼠标事件：图片区拖动窗口，工具条放行给 HTML。
func (p *previewState) onMouse(a *app, ev browser.MouseEvent) bool {
	switch ev.Kind {
	case browser.MouseDown:
		if ev.Button == sdl2.MouseButtonRight {
			a.win.RequestClose()
			return true
		}
		if ev.Button != sdl2.MouseButtonLeft {
			return true
		}
		if ev.Y >= p.barTop {
			// 工具条：交给引擎做命中测试，按钮才能收到 click。
			return false
		}
		p.dragging = true
		// 抓取偏移 = 按下时光标相对窗口左上角的偏移。之后窗口位置取
		// 「当前窗口位置 + 光标位移」：窗口跟着光标走，事件坐标是相对
		// 当前窗口的，所以只要 p.x 与真实窗口位置同步（帧末应用过），
		// 这个式子就不会累积误差，也不会因丢事件而漂移。
		p.grabX, p.grabY = ev.X, ev.Y
		p.movePending = false
		return true

	case browser.MouseMove:
		if !p.dragging {
			return false
		}
		// 只记目标位置，不在事件里 MoveTo：一个批次常有多条 motion，
		// 逐条 SetWindowPosition 会让窗口在 1ms 内跳好几次，配合主循环
		// 10ms 的空闲休眠就是「一顿一顿」。帧末由 onFrame 合并成一次。
		p.targetX, p.targetY = p.x+ev.X-p.grabX, p.y+ev.Y-p.grabY
		p.movePending = true
		return true

	case browser.MouseUp:
		if p.dragging {
			p.dragging = false
			return true
		}
		return false
	}
	return false
}

// onFrame 是主循环的每帧回调：把拖动期间记下的目标位置落成一次窗口移动。
//
// 返回 true 表示还在拖动——主循环据此把空闲休眠从 10ms 降到 1ms。鼠标
// 125Hz 时事件间隔 8ms，10ms 休眠会把窗口位置量化成 10~20ms 一跳，
// 看起来就是不跟手。
func (p *previewState) onFrame(win *browser.Window) bool {
	if p.movePending {
		p.x, p.y = p.targetX, p.targetY
		win.MoveTo(p.x, p.y)
		p.movePending = false
	}
	return p.dragging
}
