package sdl2

import (
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

	b, p := cBytes(title)
	hwnd := sdlCreateWindow(p, windowPosCenteredX, windowPosCenteredX,
		uintptr(w), uintptr(h), windowShown|windowResizable)
	runtime.KeepAlive(b)
	if hwnd == 0 {
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

// RenderFrame 手动绘制一帧（清屏+渲染+提交），供无头渲染测试/截图使用；
// 常规用法交给 Run 的帧循环。
func (win *Window) RenderFrame() {
	if win.doc == nil {
		return
	}
	win.clearCanvas()
	engine.RenderNode(win.g, win.doc)
	win.g.Present()
}

// Screenshot 回读渲染目标当前内容（32bpp 原生字节，行主序）及其像素格式与尺寸。
// 优先使用渲染目标原生格式；不支持查询时（旧版 DLL 无 SDL_GetRenderOutputFormat）
// 固定请求 SDL_PIXELFORMAT_ARGB8888，由 SDL 负责转换。
// 配合 SDL_VIDEO_DRIVER=dummy 可在无显示器环境验证渲染结果。
func (win *Window) Screenshot() ([]byte, uint32, int, int, error) {
	const pixelFormatARGB8888 = 0x16562004
	w, h := win.w, win.h
	format := uint32(sdlGetRenderOutputFormat(win.renderer))
	if format == 0 {
		format = pixelFormatARGB8888
	}
	if (format>>8)&0xFF != 32 {
		format = pixelFormatARGB8888
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
	if win.doc == nil {
		return fmt.Errorf("window has no document, call OpenDocument first")
	}
	start := uint32(sdlGetTicks())
	var exitAt uint32
	if win.hasAutoExit {
		exitAt = start + uint32(win.autoExit/time.Millisecond)
	}

	running := true
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
			case eventMouseButtonUp:
				if ev.button == mouseButtonLeft && win.doc != nil {
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
		}

		if doc := win.doc; doc != nil {
			if c := doc.ChangeCount(); c != win.lastChange {
				engine.LayoutDocument(doc)
				win.lastChange = c
				win.dirty = true
			}
			if win.dirty {
				win.clearCanvas()
				engine.RenderNode(win.g, doc)
				win.g.Present()
				win.dirty = false
			}
		}

		sdlDelay(10)

		if win.hasAutoExit && int32(uint32(sdlGetTicks())-exitAt) >= 0 {
			running = false
		}
	}
	return nil
}

// Close 释放窗口资源。
func (win *Window) Close() {
	if win.g != nil {
		win.g.CloseFonts()
	}
	if win.renderer != 0 {
		sdlDestroyRenderer(win.renderer)
	}
	if win.win != 0 {
		sdlDestroyWindow(win.win)
	}
	ttfQuit()
	sdlQuit()
	runtime.UnlockOSThread()
}

var _ engine.Viewport = (*Window)(nil)
