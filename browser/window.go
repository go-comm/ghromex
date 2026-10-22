// 本文件是 browser 门面的窗口封装与主循环；包文档见 doc.go。

package browser

import (
	"fmt"
	"time"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/core"
	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/runtime/clipboard"
)

// wheelStepPx 是一格滚轮滚动的像素数（应用层策略：一格滚 100px）。
const wheelStepPx = 100

// Window 是一个可承载 HTML 文档的窗口，实现 engine.Viewport。
//
// 生命周期约定：New → OpenDocument → Run → Close，且须在同一 goroutine
// （SDL 要求窗口/渲染器/事件循环同 OS 线程）。
//
// 主循环在这一层：后端（backend/sdl2）只提供窗口、绘制与解码后的平台
// 事件，「事件 → 引擎动作 → 重排 → 重绘」由这里串起来。分层理由见
// core 包的说明。
type Window struct {
	sdl *sdl2.Window
	doc engine.HTMLDocument

	// lastChange 是上次布局时文档的变更计数：变了就重排。
	lastChange int64
	// dirty 为 true 才重绘：内容变化/窗口尺寸变化/暴露(EXPOSED)/首帧。
	// 空闲时零绘制，避免每帧产生的绘制指令垃圾把稳态内存抬高数 MB；
	// 但 EXPOSED 必须重绘——否则无 DWM 环境下窗口被别窗覆盖后永不可恢复。
	dirty bool
	// autoExit/hasAutoExit：到点自动退出事件循环（调试/CI 用）。
	autoExit    time.Duration
	hasAutoExit bool

	// 鼠标拖选状态：按下点只记录锚点，位移超过阈值才真正扩选。
	// 这样单击仍走原 click 链路（按钮/链接/select/radio 的行为不变），
	// 拖动结束也不派发 click——浏览器里拖选不应触发链接导航。
	mouseDown    bool
	selecting    bool
	dragSelect   bool
	downX, downY int
}

// New 创建一个窗口。坐标体系 1:1（1 CSS px = 1 物理像素）。
func New(w, h int, title string) (*Window, error) {
	sdl, err := sdl2.NewWindow(w, h, title)
	if err != nil {
		return nil, err
	}
	return &Window{sdl: sdl}, nil
}

// ViewportWidth 实现 engine.Viewport。
func (win *Window) ViewportWidth() int { return win.sdl.ViewportWidth() }

// ViewportHeight 实现 engine.Viewport。
func (win *Window) ViewportHeight() int { return win.sdl.ViewportHeight() }

// Graphics 实现 engine.Viewport。
func (win *Window) Graphics() engine.Graphics { return win.sdl.Graphics() }

// OpenDocument 解析并布局 HTML 源码，绑定到本窗口。
func (win *Window) OpenDocument(src string) (engine.HTMLDocument, error) {
	doc, err := engine.OpenDocument(win, src)
	if err != nil {
		return nil, err
	}
	win.doc = doc
	win.lastChange = doc.ChangeCount()
	win.dirty = true // 首次打开强制出一帧
	if t := doc.Title(); t != "" {
		win.SetTitle(t)
	}
	return doc, nil
}

// SetAutoExit 让 Run 在窗口打开指定时长后自动返回（调试/CI 用）。
func (win *Window) SetAutoExit(d time.Duration) {
	win.autoExit = d
	win.hasAutoExit = d > 0
}

// RenderFrame 手动绘制一帧（清屏+渲染+提交），供无头渲染测试/截图使用；
// 常规用法交给 Run 的帧循环。
func (win *Window) RenderFrame() {
	if win.doc == nil {
		return
	}
	win.clearCanvas()
	engine.RenderNode(win.sdl.Graphics(), win.doc)
	win.sdl.Present()
}

// clearCanvas 清屏：铺满整个视口——没有画布背景时用白底，有的话用画布
// 背景色（CSS canvas propagation，html/body 背景）。否则内容高度以下的
// 区域会露出白底，窗口看起来"铺不满"。
func (win *Window) clearCanvas() {
	g := win.sdl.Graphics()
	w, h := win.sdl.ViewportWidth(), win.sdl.ViewportHeight()
	for _, c := range clearLayers(win.doc) {
		g.DrawColor(0, 0, w, h, c)
	}
}

// clearLayers 返回清屏要铺的颜色层（自底向上）。
//
// 画布背景**不透明**时，白底那层是完全被盖住的重复填充——大窗体下就是
// 一整屏的像素（软件光栅器下 2560×1440 要 0.3ms），没必要画。
//
// **前提**：画布背景是「铺满整个视口的不透明矩形」。
// 若将来画布背景支持圆角、边距或半透明，这条优化必须回退——否则四角/
// 边缘会露出上一帧的残留（这种 bug 像素测试能抓到，但没人会想到去测
// 四角）。改动前先确认这一点。
func clearLayers(doc engine.HTMLDocument) []core.Color {
	white := engine.NewColor(255, 255, 255, 255)
	bg := engine.CanvasBackground(doc)
	if bg == nil {
		return []core.Color{white}
	}
	if _, _, _, a := bg.RGBA(); a != 255 {
		return []core.Color{white, bg}
	}
	return []core.Color{bg}
}

// Screenshot 回读渲染目标当前内容（原生字节+格式+尺寸）。
func (win *Window) Screenshot() ([]byte, uint32, int, int, error) {
	return win.sdl.Screenshot()
}

// Run 进入事件循环：处理关闭/尺寸变化/暴露/点击，并按内容变化重排、逐帧重绘。
func (win *Window) Run() error {
	if win.sdl.Closed() {
		return fmt.Errorf("window already closed")
	}
	if win.doc == nil {
		return fmt.Errorf("window has no document, call OpenDocument first")
	}
	start := uint32(0)
	var exitAt uint32
	if win.hasAutoExit {
		start = win.sdl.Ticks()
		exitAt = start + uint32(win.autoExit/time.Millisecond)
	}

	running := true
	// GHROMEX_BENCH：分段计时（布局/清屏/重绘/呈现/空闲）。只做本地诊断，
	// 不进 CI——计时不可复现，断言它只会得到 flaky 测试。口径说明见 bench.go。
	var bench *bench
	var counter *countingGraphics
	if benchEnabled() {
		bench = newBench(win)
		counter = &countingGraphics{inner: win.sdl.Graphics()}
	}
	for running {
		frameStart := time.Now()
		for {
			ev, ok := win.sdl.PollEvent()
			if !ok {
				break
			}
			switch ev.Kind {
			case sdl2.EventQuit:
				running = false
			case sdl2.EventWindowClose:
				running = false
			case sdl2.EventUnknown:
				// 不关心的事件（窗口移动、鼠标进入等）：忽略。
			case sdl2.EventMouseLeave:
				// 鼠标离开窗口：清掉悬停态（否则最后悬停的按钮一直高亮）。
				if win.doc != nil {
					engine.SetHoveredElement(win.doc, nil)
				}
			case sdl2.EventWindowResized:
				if ev.Width > 0 && ev.Height > 0 {
					// 后端在解码尺寸变化时已更新自己的视口尺寸，这里只需
					// 强制重排（文档内容没变，变更计数不会动）。
					win.lastChange = -1 // 强制重排
					win.dirty = true
				}
			case sdl2.EventWindowExposed:
				// 被覆盖后重新暴露：内容已丢（Win7 无 DWM + GPU 后缓冲），
				// 布局未变无需重排，强制重绘即可。
				win.dirty = true
			case sdl2.EventKeyDown:
				win.onKeyDown(ev)
			case sdl2.EventTextInput:
				win.onTextInput(ev)
			case sdl2.EventMouseButtonDown:
				win.onMouseDown(ev)
			case sdl2.EventMouseMotion:
				win.onMouseMove(ev)
			case sdl2.EventMouseButtonUp:
				win.onMouseUp(ev)
			case sdl2.EventMouseWheel:
				win.onWheel(ev)
			}
			if !running {
				// 关闭事件已到达：不再消费队列里的剩余事件——否则窗口已关
				// 仍会有点击/按键改写文档并触发重排（对已销毁的渲染器绘制）。
				break
			}
		}

		pending := false
		var dLayout, dClear, dRender, dPresent, dIdle time.Duration
		if doc := win.doc; doc != nil {
			if c := doc.ChangeCount(); c != win.lastChange {
				t0 := time.Now()
				engine.LayoutDocument(doc)
				dLayout = time.Since(t0)
				win.lastChange = c
				win.dirty = true
			}
			// 滚动动画：只平移已布局的盒子，不重排（滚动偏移与盒模型无关，
			// 每帧一次全量级联+布局足以让大页面滚动发顿）。
			if engine.TickScroll(doc) {
				win.dirty = true
			}
			if win.dirty || bench != nil {
				// 基准模式强制每帧重绘：静态页面平时不重绘（省 CPU），
				// 那样量到的只是空闲休眠，渲染管线的成本根本测不到。
				t0 := time.Now()
				win.clearCanvas()
				dClear = time.Since(t0)
				g := win.sdl.Graphics()
				if counter != nil {
					counter.reset()
					g = counter
				}
				t0 = time.Now()
				engine.RenderNode(g, doc)
				dRender = time.Since(t0)
				t0 = time.Now()
				win.sdl.Present()
				dPresent = time.Since(t0)
				win.dirty = false
			}
			// 布局期可能又标记了变更（滚动动画的下一帧）：此时不能睡 10ms，
			// 否则每帧白等 10ms，120ms 的动画会被拖到 200ms 以上；空闲时仍
			// 保留 10ms 休眠省 CPU（present 有 vsync 时本就会自然限帧）。
			pending = doc.ChangeCount() != win.lastChange || engine.ScrollAnimating(doc)
		}

		if !pending {
			t0 := time.Now()
			win.sdl.Delay(10)
			dIdle = time.Since(t0)
		}

		if bench != nil {
			calls := 0
			if counter != nil {
				calls = counter.calls
			}
			bench.add(time.Since(frameStart), dLayout, dClear, dRender, dPresent, dIdle, calls)
			if len(bench.period) >= benchFrames() {
				// 首次出报告前先做一次 vsync 校准：把「呈现」里的等待与
				// 拷贝拆开（加速路径的 16.6ms 里有多少是真实工作，全靠它）。
				if !bench.hasCalib {
					bench.calibVsync()
				}
				bench.report()
			}
		}

		if win.hasAutoExit && int32(win.sdl.Ticks()-exitAt) >= 0 {
			running = false
		}
	}
	if bench != nil {
		bench.report() // 退出前把不足一批的样本也报出来
	}
	return nil
}

// onKeyDown 处理按键：Ctrl+C 复制选区、Ctrl+V 粘贴，其余交给引擎。
func (win *Window) onKeyDown(ev sdl2.Event) {
	if win.doc == nil {
		return
	}
	// Ctrl+C：引擎只给选区文本，写系统剪贴板由平台能力完成。
	// 无选区时 SelectedText 返回 false，保留用户原有剪贴板
	//（浏览器 Ctrl+C 无选区也不会清空剪贴板）。
	if ev.Mod&sdl2.KmodCtrl != 0 && (ev.Key == 'c' || ev.Key == 'C') {
		if text, ok := engine.SelectedText(win.doc); ok {
			clipboard.SetText(text)
		}
	}
	// Ctrl+V：读系统剪贴板，交给引擎粘贴（引擎不碰平台剪贴板，
	// 只负责在光标处落文本）。
	if ev.Mod&sdl2.KmodCtrl != 0 && (ev.Key == 'v' || ev.Key == 'V') {
		if text := clipboard.Text(); text != "" {
			engine.OnDocumentPaste(win.doc, text)
		}
	}
	// 控制键（Backspace/Tab/Enter/方向键/Home/End）、Ctrl+A 全选、
	// Shift+方向键扩选；value/焦点变化经 markChanged 触发 changed →
	// 下方重排+重绘链路，无需手动置 dirty。
	engine.OnDocumentKeyDownMods(win.doc, ev.Key, ev.Mod)
}

// onTextInput 处理文本输入。
func (win *Window) onTextInput(ev sdl2.Event) {
	if win.doc == nil || ev.Text == "" {
		return
	}
	// 带 Ctrl 的"文本"不是打字：Windows 把 Ctrl+V 之类的组合键以控制码
	//（0x16 等）送进文本通道，照单插入会在输入框里留下不可见控制字符。
	// 带 Ctrl 且不带 Alt 时丢弃（AltGr 组合仍需放行——部分布局用 Ctrl+Alt 打字）。
	mods := win.sdl.ModState()
	if mods&sdl2.KmodCtrl != 0 && mods&sdl2.KmodAlt == 0 {
		return
	}
	engine.OnDocumentTextInput(win.doc, ev.Text)
}

// onMouseDown 记录按下点，准备拖选。
func (win *Window) onMouseDown(ev sdl2.Event) {
	if ev.Button != sdl2.MouseButtonLeft {
		return
	}
	win.mouseDown = true
	win.dragSelect = false
	win.downX, win.downY = ev.X, ev.Y
	if win.doc != nil {
		win.selecting = engine.OnDocumentMouseDown(win.doc, ev.X, ev.Y)
	}
}

// onMouseMove 更新悬停态；拖动超过阈值才当作拖选（否则是普通移动）。
func (win *Window) onMouseMove(ev sdl2.Event) {
	if win.doc == nil {
		return
	}
	// 悬停态：无论是否在拖选都更新（:hover 样式要跟手）。悬停目标没变
	// 时引擎不触发重排，移动事件因此不带来额外开销。
	engine.OnDocumentMouseMove(win.doc, ev.X, ev.Y)
	if !win.mouseDown || !win.selecting {
		return
	}
	dx, dy := ev.X-win.downX, ev.Y-win.downY
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if !win.dragSelect && (dx > 3 || dy > 3) {
		win.dragSelect = true
	}
	if win.dragSelect {
		engine.OnDocumentMouseDrag(win.doc, ev.X, ev.Y)
	}
}

// onMouseUp 抬起：拖选过就不派发 click（拖过链接会误导航）。
func (win *Window) onMouseUp(ev sdl2.Event) {
	if ev.Button != sdl2.MouseButtonLeft {
		return
	}
	wasDrag := win.dragSelect
	win.mouseDown = false
	win.selecting = false
	win.dragSelect = false
	if win.doc != nil {
		engine.OnDocumentMouseUp(win.doc)
	}
	if wasDrag {
		return
	}
	if win.doc != nil {
		engine.OnDocumentClick(win.doc, ev.X, ev.Y)
	}
}

// onWheel 处理滚轮：Shift+竖轮转横向。
func (win *Window) onWheel(ev sdl2.Event) {
	if win.doc == nil {
		return
	}
	// 坐标符号：OnDocumentWheel 要求 dy>0 = 向下滚（scrollTop 增）、
	// dx>0 = 向右滚；SDL y 正=滚轮向上 → dy 取反。
	dx := ev.WheelX * wheelStepPx
	dy := -ev.WheelY * wheelStepPx
	if dx == 0 && dy != 0 && win.sdl.ModState()&sdl2.KmodShift != 0 {
		// Shift+滚轮：竖轮转横滚（浏览器同语义）。
		dx, dy = dy, 0
	}
	engine.OnDocumentWheel(win.doc, ev.X, ev.Y, dx, dy)
}

// InjectEvent 往事件队列注入一个平台事件（测试用）。
//
// 真实事件由 SDL 产生；测试要用「按下鼠标」「敲键盘」这类事件驱动窗口，
// 就靠它——比在测试里手搓平台事件字节布局稳得多。
func (win *Window) InjectEvent(ev sdl2.Event) { win.sdl.InjectEvent(ev) }

// RequestClose 请求关闭窗口（应用层「退出」菜单项用）：向事件队列压入
// 一个退出事件，事件循环收到后正常返回，资源仍由 Close 释放。
func (win *Window) RequestClose() { win.sdl.RequestClose() }

// SetTitle 更新窗口标题（如编辑器显示当前文件名）。
func (win *Window) SetTitle(title string) { win.sdl.SetTitle(title) }

// Close 释放窗口资源。
func (win *Window) Close() { win.sdl.Close() }

// NativeHandle 返回窗口的原生句柄（Windows 上是 HWND），供 runtime/dialog
// 的原生对话框当父窗口。
//
// 取不到时返回 ErrUnsupported（例如无头模式或非 Windows 平台）。
func (win *Window) NativeHandle() (uintptr, error) {
	if win == nil || win.sdl == nil {
		return 0, ErrUnsupported
	}
	return win.sdl.NativeHandle()
}

var _ engine.Viewport = (*Window)(nil)
