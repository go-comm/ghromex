//go:build windows

package sdl2

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"

	"github.com/go-comm/ghromex/engine"
)

// Window 是 SDL2 窗口，实现 engine.Viewport，承载单个 HTMLDocument。
type Window struct {
	w, h     int
	title    string
	win      uintptr
	renderer uintptr
	g        *Graphics
	doc      engine.HTMLDocument

	lastChange  int64
	autoExit    time.Duration
	hasAutoExit bool

	// dirty 为 true 才重绘：内容变化/窗口尺寸变化/暴露(EXPOSED)/首帧。
	// 空闲时零绘制，避免每帧产生的绘制指令垃圾把稳态内存抬高数 MB；
	// 但 EXPOSED 必须重绘——否则无 DWM 环境下窗口被别窗覆盖后永不可恢复。
	dirty bool

	// shown 记录窗口是否已显示。首帧 Present 之前保持隐藏（建窗带
	// windowHidden），首帧提交后才 ShowWindow + RaiseWindow——见 present。
	shown bool

	// closed 记录窗口是否已关闭：Close 幂等（重复 SDL_Destroy* 是
	// use-after-free），关闭后 Run/绘制一律拒绝。
	closed bool
}

// NewWindow 创建带渲染器的窗口。坐标体系 1:1（1 CSS px = 1 物理像素）。
//
// 注意：SDL（Direct3D 后端）要求窗口与渲染器的创建、事件循环、销毁
// 发生在同一个 OS 线程上，否则退出释放资源时会死锁。这里在创建时锁定
// 当前 goroutine 到 OS 线程，Close 时解锁；调用方应在同一个 goroutine
// 中依次使用 NewWindow → Run → Close（典型即 main goroutine）。
func NewWindow(w, h int, title string) (*Window, error) {
	runtime.LockOSThread()
	if err := Load(); err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}
	if sdlInit(initVideo) != 0 {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("SDL_Init: %s", lastError())
	}
	if ttfInit() != 0 {
		runtime.UnlockOSThread()
		sdlQuit()
		return nil, fmt.Errorf("TTF_Init: %s", lastError())
	}

	// windowHidden：窗口先隐藏，首帧 Present 之后才显示（见 present）。
	// SDL2 建窗默认可见（漏传 HIDDEN 即带 SHOWN，实测 flags=0x24），而交换链
	// 在首次 Present 前不含任何有效画面——窗口若建窗即可见，从建窗到解析/
	// 级联/布局/首绘（含首次字体加载，可达数百毫秒）会一直显示黑底，表现为
	// “先黑一下再闪出页面”。
	b, p := cBytes(title)
	hwnd := sdlCreateWindow(p, windowPosCenteredX, windowPosCenteredX,
		uintptr(w), uintptr(h), windowHidden|windowResizable)
	runtime.KeepAlive(b)
	if hwnd == 0 {
		// 与上方各失败分支保持一致：解锁 OS 线程 + 退出 SDL 子系统。
		// 漏掉会让该 goroutine 永久钉在 OS 线程上，且 video 子系统残留。
		runtime.UnlockOSThread()
		sdlQuit()
		return nil, fmt.Errorf("SDL_CreateWindow: %s", lastError())
	}

	// GPU（accelerated + vsync）优先：硬件后端缩放/合成表现好、功耗低。
	// 创建失败（无 GPU、驱动不可用）自动回退软件渲染器——"software" 把像素
	// 直接落在窗口表面，跨驱动/远程桌面环境表现一致，且支持
	// SDL_RenderReadPixels 回读（Screenshot 依赖它）。
	// 两类环境直接用软件：GHROMEX_SOFTWARE=1 逃生口（部分虚拟显示驱动能创建
	// accelerated 渲染器却"窗口能开、像素不出"，自动回退不会触发）；
	// dummy 无头自检要保持确定性回读行为，不尝试 GPU。
	softwareOnly := os.Getenv("GHROMEX_SOFTWARE") != "" ||
		os.Getenv("SDL_VIDEO_DRIVER") == "dummy"
	gpu := false
	var gpuErr string
	var renderer uintptr
	if !softwareOnly {
		clearError()
		renderer = sdlCreateRenderer(hwnd, ^uintptr(0), rendererAccelerated|rendererPresentVsync)
		if renderer != 0 {
			gpu = true
		} else {
			gpuErr = lastError()
			fmt.Fprintf(os.Stderr, "ghromex: GPU 渲染器不可用（%s），回退软件渲染\n", gpuErr)
		}
	}
	if renderer == 0 {
		clearError()
		renderer = sdlCreateRenderer(hwnd, ^uintptr(0), rendererSoftware)
	}
	if renderer == 0 {
		sdlDestroyWindow(hwnd)
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("SDL_CreateRenderer: %s", lastError())
	}
	if drawDebug {
		if gpu {
			fmt.Fprintln(os.Stderr, "ghromex: renderer=accelerated(GPU)")
		} else {
			fmt.Fprintln(os.Stderr, "ghromex: renderer=software")
		}
	}
	sdlSetRenderDrawBlendMode(renderer, blendModeBlend)
	// 激活 SDL 文本输入：不启用则驱动不会为可打印字符发 SDL_TEXTINPUT
	//（中文 IME 组合串同样依赖此开关），真机表现为“有光标无法输入”。
	sdlStartTextInput()

	return &Window{
		w: w, h: h, title: title,
		win: hwnd, renderer: renderer,
		g:          NewGraphics(renderer, ""),
		lastChange: -1,
	}, nil
}

// ViewportWidth 实现 engine.Viewport。
func (win *Window) ViewportWidth() int { return win.w }

// ViewportHeight 实现 engine.Viewport。
func (win *Window) ViewportHeight() int { return win.h }

// Graphics 实现 engine.Viewport。
func (win *Window) Graphics() engine.Graphics { return win.g }

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
		b, p := cBytes(t)
		sdlSetWindowTitle(win.win, p)
		runtime.KeepAlive(b)
	}
	return doc, nil
}

// SetAutoExit 让 Run 在窗口打开指定时长后自动返回（调试/CI 用）。
func (win *Window) SetAutoExit(d time.Duration) {
	win.autoExit = d
	win.hasAutoExit = d > 0
}

// clearCanvas 清屏：先白底，再用文档的画布背景色（CSS canvas propagation，
// html/body 背景）铺满整个视口——否则内容高度以下的区域会露出白底，
// 窗口看起来“铺不满”。
func (win *Window) clearCanvas() {
	win.g.Clear(255, 255, 255, 255)
	if bg := engine.CanvasBackground(win.doc); bg != nil {
		win.g.DrawColor(0, 0, win.w, win.h, bg)
	}
}

// present 提交当前帧；首帧提交后才真正显示窗口。
//
// 时序原因见 NewWindow 中 windowHidden 的注释：黑底出现在“建窗可见”与
// “首帧就绪”之间的空窗期，因此建窗隐藏，等这一帧内容进了交换链
// （Present 已返回）再 ShowWindow——窗口一出现就是完整页面，没有黑闪。
// RaiseWindow 复原建窗即可见时的初始前台/焦点（否则可能落在别的窗口后面）。
func (win *Window) present() {
	win.g.Present()
	if win.shown {
		return
	}
	win.shown = true
	sdlShowWindow(win.win)
	sdlRaiseWindow(win.win)
}

// RenderFrame 手动绘制一帧（清屏+渲染+提交），供无头渲染测试/截图使用；
// 常规用法交给 Run 的帧循环。
func (win *Window) RenderFrame() {
	if win.doc == nil {
		return
	}
	win.clearCanvas()
	engine.RenderNode(win.g, win.doc)
	win.present()
}

// Screenshot 回读渲染目标当前内容（32bpp 原生字节，行主序）及其像素格式与尺寸。
// 优先使用窗口的原生像素格式（SDL2 的 SDL_GetWindowPixelFormat）；查不到
// 或非 32bpp 时固定请求 XBGR8888（0x16562004，小端下内存字节序 R,G,B,X），
// 由 SDL 负责转换。注意渲染目标无 alpha 通道时第 4 字节恒为 0（不是 alpha），
// 颜色只能按前三个通道读。
// 配合 SDL_VIDEO_DRIVER=dummy 可在无显示器环境验证渲染结果。
func (win *Window) Screenshot() ([]byte, uint32, int, int, error) {
	const pixelFormatXBGR8888 = 0x16562004
	w, h := win.w, win.h
	format := uint32(sdlGetWindowPixelFormat(win.win))
	if format == 0 {
		format = pixelFormatXBGR8888
	}
	if (format>>8)&0xFF != 32 {
		format = pixelFormatXBGR8888
	}
	buf := make([]byte, w*h*4)
	clearError()
	ret := sdlRenderReadPixels(win.renderer, 0, uintptr(format),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(w*4))
	runtime.KeepAlive(buf)
	if ret != 0 {
		return nil, 0, 0, 0, fmt.Errorf("SDL_RenderReadPixels: %s", lastError())
	}
	if e := lastError(); e != "" {
		return nil, 0, 0, 0, fmt.Errorf("SDL_RenderReadPixels: %s", e)
	}
	return buf, format, w, h, nil
}

// Run 进入事件循环：处理关闭/尺寸变化/暴露/点击，并按内容变化重排、逐帧重绘。
func (win *Window) Run() error {
	if win.closed {
		return fmt.Errorf("window already closed")
	}
	if win.doc == nil {
		return fmt.Errorf("window has no document, call OpenDocument first")
	}
	start := uint32(sdlGetTicks())
	var exitAt uint32
	if win.hasAutoExit {
		exitAt = start + uint32(win.autoExit/time.Millisecond)
	}

	running := true
	// 鼠标拖选状态：按下点只记录锚点，位移超过阈值才真正扩选。
	// 这样单击仍走原 click 链路（按钮/链接/select/radio 的行为不变），
	// 拖动结束也不派发 click——浏览器里拖选不应触发链接导航。
	mouseDown := false
	selecting := false
	dragSelect := false
	downX, downY := 0, 0
	for running {
		for {
			ev, ok := pollEvent()
			if !ok {
				break
			}
			switch ev.typ {
			case eventQuit:
				running = false
			case eventWindow:
				if drawDebug {
					// 诊断行：SDL2 枚举 EXPOSED=0x03 SHOWN=0x01 MOVED=0x04
					// RESIZED=0x05 CLOSE=0x0E（完整表见 sdl2.go 常量注释）。
					// Win7 覆盖异常时据此判断事件是否到达。
					fmt.Fprintf(os.Stderr, "[ghromex] win-event 0x%02X data=(%d,%d)\n", ev.windowEvent, ev.data1, ev.data2)
				}
				switch ev.windowEvent {
				case windowEventClose:
					running = false
				case windowEventResized, windowEventSizeChanged:
					if ev.data1 > 0 && ev.data2 > 0 {
						win.w = int(ev.data1)
						win.h = int(ev.data2)
						win.lastChange = -1 // 强制重排
						win.dirty = true
					}
				case windowEventExposed:
					// 被覆盖后重新暴露：内容已丢（Win7 无 DWM + GPU 后缓冲），
					// 布局未变无需重排，强制重绘即可。
					win.dirty = true
				}
			case eventKeyDown:
				if win.doc != nil {
					// Ctrl+C：引擎只给选区文本，写系统剪贴板由后端完成。
					// 无选区时 SelectedText 返回 false，保留用户原有剪贴板
					//（浏览器 Ctrl+C 无选区也不会清空剪贴板）。
					if ev.mod&kmodCtrl != 0 && (ev.sym == 'c' || ev.sym == 'C') {
						if text, ok := engine.SelectedText(win.doc); ok {
							setClipboardText(text)
						}
					}
					// Ctrl+V：读系统剪贴板，交给引擎粘贴（引擎不碰平台
					// 剪贴板，只负责在光标处落文本）。
					if ev.mod&kmodCtrl != 0 && (ev.sym == 'v' || ev.sym == 'V') {
						if text := readClipboardText(); text != "" {
							engine.OnDocumentPaste(win.doc, text)
						}
					}
					// 控制键（Backspace/Tab/Enter/方向键/Home/End）、Ctrl+A
					// 全选、Shift+方向键扩选；value/焦点变化经 markChanged
					// 触发 changed → 下方重排+重绘链路，无需手动置 dirty。
					engine.OnDocumentKeyDownMods(win.doc, int(ev.sym), ev.mod)
				}
			case eventTextInput:
				if win.doc != nil && ev.text != "" {
					// 带 Ctrl 的"文本"不是打字：Windows 把 Ctrl+V 之类的
					// 组合键以控制码（0x16 等）送进文本通道，照单插入会在
					// 输入框里留下不可见控制字符。带 Ctrl 且不带 Alt 时丢弃
					//（AltGr 组合仍需放行——部分布局用 Ctrl+Alt 打字）。
					mods := sdlGetModState()
					if mods&kmodCtrl != 0 && mods&kmodAlt == 0 {
						break
					}
					engine.OnDocumentTextInput(win.doc, ev.text)
				}
			case eventMouseButtonDown:
				if ev.button == mouseButtonLeft {
					mouseDown = true
					dragSelect = false
					downX, downY = int(ev.x), int(ev.y)
					if win.doc != nil {
						selecting = engine.OnDocumentMouseDown(win.doc, int(ev.x), int(ev.y))
					}
				}
			case eventMouseMotion:
				if mouseDown && selecting && win.doc != nil {
					dx, dy := int(ev.x)-downX, int(ev.y)-downY
					if dx < 0 {
						dx = -dx
					}
					if dy < 0 {
						dy = -dy
					}
					if !dragSelect && (dx > 3 || dy > 3) {
						dragSelect = true
					}
					if dragSelect {
						engine.OnDocumentMouseDrag(win.doc, int(ev.x), int(ev.y))
					}
				}
			case eventMouseButtonUp:
				if ev.button == mouseButtonLeft {
					wasDrag := dragSelect
					mouseDown = false
					selecting = false
					dragSelect = false
					if win.doc != nil {
						engine.OnDocumentMouseUp(win.doc)
					}
					if wasDrag {
						// 拖选结束：不派发 click（否则拖过 <a> 会导航）。
						if drawDebug {
							fmt.Fprintf(os.Stderr, "[ghromex] drag-select up(%d,%d)\n", ev.x, ev.y)
						}
						break
					}
					if win.doc != nil {
						handled := engine.OnDocumentClick(win.doc, int(ev.x), int(ev.y))
						if drawDebug {
							target := engine.ElementAt(win.doc, int(ev.x), int(ev.y))
							tag := "<none>"
							if target != nil {
								tag = target.TagName()
							}
							fmt.Fprintf(os.Stderr, "[ghromex] click(%d,%d) hit=%s handled=%v\n", ev.x, ev.y, tag, handled)
						}
					}
				}
			case eventMouseWheel:
				// 坐标符号：OnDocumentWheel 要求 dy>0 = 向下滚（scrollTop 增）、
				// dx>0 = 向右滚；SDL y 正=滚轮向上、x 正=向右 → dy 取反。
				if win.doc != nil {
					dx := int(ev.wheelX) * wheelStepPx
					dy := -int(ev.wheelY) * wheelStepPx
					if dx == 0 && dy != 0 && sdlGetModState()&kmodShift != 0 {
						// Shift+滚轮：竖轮转横滚（浏览器同语义）。
						dx, dy = dy, 0
					}
					engine.OnDocumentWheel(win.doc, int(ev.x), int(ev.y), dx, dy)
				}
			}
			if !running {
				// 关闭事件已到达：不再消费队列里的剩余事件——否则窗口已关
				// 仍会有点击/按键改写文档并触发重排（对已销毁的渲染器绘制）。
				break
			}
		}

		pending := false
		if doc := win.doc; doc != nil {
			if c := doc.ChangeCount(); c != win.lastChange {
				engine.LayoutDocument(doc)
				win.lastChange = c
				win.dirty = true
			}
			// 滚动动画：只平移已布局的盒子，不重排（滚动偏移与盒模型无关，
			// 每帧一次全量级联+布局足以让大页面滚动发顿）。
			if engine.TickScroll(doc) {
				win.dirty = true
			}
			if win.dirty {
				win.clearCanvas()
				engine.RenderNode(win.g, doc)
				win.present()
				win.dirty = false
			}
			// 布局期可能又标记了变更（滚动动画的下一帧）：此时不能睡 10ms，
			// 否则每帧白等 10ms，120ms 的动画会被拖到 200ms 以上；空闲时仍
			// 保留 10ms 休眠省 CPU（present 有 vsync 时本就会自然限帧）。
			pending = doc.ChangeCount() != win.lastChange || engine.ScrollAnimating(doc)
		}

		if !pending {
			sdlDelay(10)
		}

		if win.hasAutoExit && int32(uint32(sdlGetTicks())-exitAt) >= 0 {
			running = false
		}
	}
	return nil
}

// setClipboardText 把 UTF-8 文本写入系统剪贴板。SDL2 的接口是
// int SDL_SetClipboardText(const char *text)，成功返回 0；失败不影响
// 界面（复制是尽力而为，不打断事件循环）。cBytes 返回的切片必须
// KeepAlive 到调用结束，否则 GC 可能在 SDL 读取前回收。
func setClipboardText(text string) {
	if text == "" {
		return
	}
	buf, ptr := cBytes(text)
	sdlSetClipboardText(ptr)
	runtime.KeepAlive(buf)
}

// SetClipboardText 把 UTF-8 文本写入系统剪贴板（应用层接口）。
//
// 引擎只负责选区文本，剪贴板读写属平台能力，故放在后端：应用经
// browser.SetClipboardText 落盘，engine 保持纯 Go、无平台依赖。
func SetClipboardText(text string) { setClipboardText(text) }

// ClipboardText 读取系统剪贴板文本（UTF-8）。剪贴板为空或 SDL 未导出
// 相关符号时返回空串。
func ClipboardText() string { return readClipboardText() }

// readClipboardText 读取系统剪贴板文本（UTF-8），供 Ctrl+V 粘贴。
//
// SDL_GetClipboardText 返回 **SDL 分配的** C 字符串，必须用 SDL_free
// 释放——否则每次粘贴都漏一块堆内存。两个符号都按可选绑定处理：
// SDL 未导出（或尚未 Load）时返回空串，粘贴退化为「无内容可粘」。
func readClipboardText() string {
	if !hasGetClipboardText {
		return ""
	}
	ptr := sdlGetClipboardText()
	if ptr == 0 {
		return ""
	}
	// 剪贴板内容可能远长于错误信息，上限给到 8MB。
	s := goCString(ptr, 8<<20)
	if hasSDLFree {
		sdlFree(ptr)
	}
	return s
}

// NativeHandle 返回窗口的原生 HWND（Windows），供原生对话框（backend/dialog
// 的 IFileDialog）当父窗口。
//
// SDL_CreateWindow 返回的是 SDL_Window*，不是 HWND——原生对话框要拿真正的
// HWND 当父窗口（否则 Show 会回 E_INVALIDARG），只能问 SDL_GetWindowWMInfo。
// 取不到时返回 0：对话框照样能弹，只是不再挂在主窗口下面。
func (win *Window) NativeHandle() (uintptr, error) {
	if win == nil || win.closed || win.win == 0 {
		return 0, fmt.Errorf("窗口尚未创建，无法取原生句柄")
	}
	return win.hwnd(), nil
}

// sdlSysWMinfo 对应 SDL_syswm.h 的 SDL_SysWMinfo（Windows 分支）：
// version（SDL_version，3 字节 + 1 字节填充）、subsystem，以及 info
// 联合体里的 HWND / HDC / HINSTANCE。
type sdlSysWMinfo struct {
	version   [3]uint8
	_         [1]byte
	subsystem uint32
	winWindow uintptr
	winHDC    uintptr
	winHInst  uintptr
}

// hwnd 取窗口的原生 HWND；取不到返回 0。
func (win *Window) hwnd() uintptr {
	if !hasGetWindowWMInfo || sdlGetWindowWMInfo == nil {
		return 0
	}
	var info sdlSysWMinfo
	// info.version 必须按 SDL_VERSION 宏填成运行库的版本号，否则
	// SDL_GetWindowWMInfo 直接返回 SDL_FALSE。
	var ver [3]uint8
	sdlGetVersion(uintptr(unsafe.Pointer(&ver[0])))
	runtime.KeepAlive(&ver[0])
	info.version = ver
	if sdlGetWindowWMInfo(win.win, uintptr(unsafe.Pointer(&info))) == 0 {
		return 0
	}
	runtime.KeepAlive(&info)
	return info.winWindow
}

// SetTitle 设置窗口标题（应用层可随时更新，如编辑器显示当前文件名）。
func (win *Window) SetTitle(title string) {
	if win == nil || win.closed || win.win == 0 {
		return
	}
	b, p := cBytes(title)
	sdlSetWindowTitle(win.win, p)
	runtime.KeepAlive(b)
}

// RequestClose 请求关闭窗口：向事件队列压入一个 SDL_QUIT 事件，事件
// 循环收到后按「用户点关闭按钮」的同一条路径退出（应用层的「退出」
// 菜单项用）。
func (win *Window) RequestClose() {
	if win == nil || win.closed {
		return
	}
	var buf [eventBufferSize]byte
	binary.LittleEndian.PutUint32(buf[0:4], eventQuit)
	sdlPushEvent(uintptr(unsafe.Pointer(&buf[0])))
}

// Close 释放窗口资源。幂等：重复调用直接返回——重复 SDL_Destroy* 会对已
// 销毁对象二次释放（use-after-free），句柄不置零也会让后续绘制打到野指针。
func (win *Window) Close() {
	if win.closed {
		return
	}
	win.closed = true
	if win.g != nil {
		win.g.CloseFonts()
	}
	if win.renderer != 0 {
		sdlDestroyRenderer(win.renderer)
		win.renderer = 0
	}
	if win.win != 0 {
		sdlDestroyWindow(win.win)
		win.win = 0
	}
	ttfQuit()
	sdlQuit()
	runtime.UnlockOSThread()
}

var _ engine.Viewport = (*Window)(nil)
