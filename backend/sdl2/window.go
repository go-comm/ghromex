//go:build windows

package sdl2

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/go-comm/ghromex/core"
)

// Window 是 SDL2 窗口：平台层，只负责窗口/渲染器/字体纹理/事件解码/绘制。
//
// 这里**不认识引擎**：文档与主循环（事件 → 引擎输入处理 → 重排 → 重绘）
// 在上一层（browser）做。后端只提供三样东西：
//
//   - 视口契约（core.Viewport / core.Graphics）：引擎靠它量尺寸、落笔；
//   - 事件（Event）：平台事件由上层解释成引擎动作；
//   - 呈现（Present）：把这一帧提交到窗口。
//
// 这样依赖方向是单向的：core ← engine，core ← backend/sdl2。
type Window struct {
	w, h     int
	title    string
	win      uintptr
	renderer uintptr
	g        *Graphics

	// shown 记录窗口是否已显示。首帧 Present 之前保持隐藏（建窗带
	// windowHidden），首帧提交后才 ShowWindow + RaiseWindow——见 Present。
	shown bool

	// closed 记录窗口是否已关闭：Close 幂等（重复 SDL_Destroy* 是
	// use-after-free），关闭后绘制一律拒绝。
	closed bool

	// rendererDesc 记录渲染路径的描述（供诊断/基准工具归因）：
	// "software (driver=software; 默认内存渲染)"（默认）/
	// "accelerated(GPU, driver=direct3d11; GHROMEX_GPU=1)" /
	// "software (driver=software; 强制软件：GHROMEX_SOFTWARE/dummy)" 等。
	// 建窗时算一次存下来，别处要显示时直接取（driver 名要现查，开销不小）。
	rendererDesc string

	// posX/posY 是窗口左上角的当前位置（SDL 逻辑坐标，虚拟桌面系）。
	// posKnown=false 表示还没拿到可信位置：Centered 建窗时真实坐标由 SDL
	// 内部算出，要等第一条 MOVED 事件才知道。
	posX, posY int
	posKnown   bool

	// originX/originY 是「SDL 内部鼠标缓存坐标」所相对的窗口位置。SDL 的
	// 按钮事件不读消息的 lParam，直接用缓存值（mouse->x/y），而缓存只在
	// motion 里刷新——于是「窗口移动后、下一条 motion 之前」的按钮/滚轮
	// 事件，坐标仍是相对旧窗口位置的。originKnown=false 表示本窗口还没
	// 观测到任何 motion（缓存未被刷新过），此时不做平移（见 staleOffset）。
	originX, originY int
	originKnown      bool
}

// NewWindow 创建带渲染器的窗口。坐标体系 1:1（1 CSS px = 1 物理像素）。
//
// 注意：SDL（Direct3D 后端）要求窗口与渲染器的创建、事件循环、销毁
// 发生在同一个 OS 线程上，否则退出释放资源时会死锁。这里在创建时锁定
// 当前 goroutine 到 OS 线程，Close 时解锁；调用方应在同一个 goroutine
// 中依次使用 NewWindow → Run → Close（典型即 main goroutine）。
func NewWindow(w, h int, title string) (*Window, error) {
	return NewWindowWithOptions(WindowOptions{
		Width: w, Height: h, Title: title,
		Centered: true, Resizable: true,
	})
}

// NewWindowWithOptions 同 NewWindow，可指定位置与窗口类型。
//
// 无论哪种窗口都先隐藏，等首帧 Present 后再显示（见 Present 的说明）：
// 截图遮罩如果建窗即显示，会先闪一帧黑底再出现桌面冻结图。
func NewWindowWithOptions(o WindowOptions) (*Window, error) {
	w, h, title := o.Width, o.Height, o.Title
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("窗口尺寸非法: %dx%d", w, h)
	}
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
	flags := uintptr(windowHidden)
	if o.Resizable {
		flags |= windowResizable
	}
	if o.Borderless {
		flags |= windowBorderless
	}
	if o.AlwaysOnTop {
		flags |= windowAlwaysOnTop
	}
	if o.SkipTaskbar {
		flags |= windowSkipTaskbar
	}
	x, y := uintptr(windowPosCenteredX), uintptr(windowPosCenteredX)
	if !o.Centered {
		x, y = uintptr(o.X), uintptr(o.Y)
	}
	b, p := cBytes(title)
	hwnd := sdlCreateWindow(p, x, y, uintptr(w), uintptr(h), flags)
	runtime.KeepAlive(b)
	if hwnd == 0 {
		// 与上方各失败分支保持一致：解锁 OS 线程 + 退出 SDL 子系统。
		// 漏掉会让该 goroutine 永久钉在 OS 线程上，且 video 子系统残留。
		runtime.UnlockOSThread()
		sdlQuit()
		return nil, fmt.Errorf("SDL_CreateWindow: %s", lastError())
	}

	// 渲染器路径的选择与描述组包都收在 env.go（planRenderer/rendererDescFor），
	// 这里只执行：默认内存渲染，GHROMEX_GPU=1 才尝试加速，强制口与回退见
	// planRenderer 的注释。
	wantGPU, forced := planRenderer(
		os.Getenv("GHROMEX_GPU"),
		os.Getenv("GHROMEX_SOFTWARE"),
		os.Getenv("SDL_VIDEO_DRIVER"))
	gpu := false
	var gpuErr string
	var renderer uintptr
	if wantGPU {
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
	// 渲染路径的描述**总是**算出来存下（基准工具/诊断要拿它归因，不该
	// 依赖 GHROMEX_DEBUG）；打印才受 drawDebug 控制。
	driver := RendererDriver(renderer)
	rendererDesc := rendererDescFor(gpu, forced, gpuErr != "", driver)
	if drawDebug {
		// 驱动名要能查：软件路径的瓶颈是填充率，GPU 路径的瓶颈通常在
		// 纹理上传/draw call/swap 间隔——两者的排查方向完全不同，先确认
		// 走的是哪条路再量（GHROMEX_DEBUG 非空时打印）。
		fmt.Fprintf(os.Stderr, "ghromex: renderer=%s\n", rendererDesc)
	}
	// 渲染器的绘制混合模式的**全局不变量是 BLEND**：贴图拷贝（精灵、文字的
	// 抗锯齿边缘）要靠混合，且 SDL 各后端对「拷贝用纹理的还是渲染器的混合
	// 模式」处理不一致，保持渲染器状态是 BLEND 最稳。
	//
	// 不透明填充会临时切到 NONE（直接写像素，软件光栅器快 10 倍），但由
	// Graphics.DrawColor 成对复原（defer），不会漏到这里。
	sdlSetRenderDrawBlendMode(renderer, blendModeBlend)
	// 激活 SDL 文本输入：不启用则驱动不会为可打印字符发 SDL_TEXTINPUT
	//（中文 IME 组合串同样依赖此开关），真机表现为“有光标无法输入”。
	sdlStartTextInput()

	win := &Window{
		w: w, h: h, title: title,
		win: hwnd, renderer: renderer,
		g:            NewGraphics(renderer, ""),
		rendererDesc: rendererDesc,
	}
	// 显式给位的窗口位置已知；Centered 的真实位置等 MOVED 事件（在此之前
	// 不做鼠标坐标平移，宁可保持原样也不能猜）。
	if !o.Centered {
		win.posX, win.posY = o.X, o.Y
		win.posKnown = true
	}
	return win, nil
}

// ViewportWidth 实现 core.Viewport。
func (win *Window) ViewportWidth() int { return win.w }

// ViewportHeight 实现 core.Viewport。
func (win *Window) ViewportHeight() int { return win.h }

// Graphics 实现 core.Viewport。
func (win *Window) Graphics() core.Graphics { return win.g }

// Present 提交当前帧；首帧提交后才真正显示窗口。
//
// 时序原因见 NewWindow 中 windowHidden 的注释：黑底出现在“建窗可见”与
// “首帧就绪”之间的空窗期，因此建窗隐藏，等这一帧内容进了交换链
// （Present 已返回）再 ShowWindow——窗口一出现就是完整页面，没有黑闪。
// RaiseWindow 复原建窗即可见时的初始前台/焦点（否则可能落在别的窗口后面）。
func (win *Window) Present() {
	win.g.Present()
	if win.shown {
		return
	}
	win.shown = true
	sdlShowWindow(win.win)
	sdlRaiseWindow(win.win)
}

// Delay 等待若干毫秒（主循环空闲时用来省 CPU，不空转）。
func (win *Window) Delay(ms int) { sdlDelay(uintptr(ms)) }

// RendererInfo 返回渲染路径的描述（"accelerated(GPU, driver=direct3d11)" /
// "software (driver=software; …)"）。诊断与基准工具据此归因——数字必须
// 能说明「是哪条路径、哪个驱动量出来的」，否则无法解读。
func (win *Window) RendererInfo() string {
	if win == nil {
		return ""
	}
	return win.rendererDesc
}

// SetVSync 开/关垂直同步（诊断用）。
//
// 加速路径的 Present **含等 vsync 的阻塞**（不阻塞就抢不到交换链），
// 所以「Present 的耗时」既含拷贝也含等待，两个数混在一起没法归因。
// 关掉 vsync 后再量一次 Present，得到的就是纯拷贝耗时，二者相减即
// vsync 等待。软件路径没有 vsync，此调用无副作用。
func (win *Window) SetVSync(on bool) {
	if win == nil || win.renderer == 0 {
		return
	}
	n := 0
	if on {
		n = 1
	}
	sdlRenderSetVSync(win.renderer, uintptr(n))
}

// PollEvent 取下一个平台事件；队列为空时返回 false。
//
// 事件怎么解释成引擎动作（点击、按键、滚轮）是上一层的事：后端只解码
// SDL 事件，不认识文档与引擎。
func (win *Window) PollEvent() (Event, bool) {
	ev, ok := pollEvent()
	if !ok {
		return Event{}, false
	}
	return win.toPublicEvent(ev), true
}

// toPublicEvent 把内部解码结果转成公开的 Event。
//
// 窗口尺寸/位置变化顺带更新后端自己的尺寸与位置：视口尺寸与「鼠标缓存
// 坐标的平移基准」都是平台事实，上层只管读。
//
// 认不出的事件（鼠标进入/离开等）返回 EventUnknown——上层按未知事件忽略
// 即可（不能落到 EventQuit 上，否则窗口会莫名其妙地退出）。
func (win *Window) toPublicEvent(ev event) Event {
	switch ev.typ {
	case eventQuit:
		return Event{Kind: EventQuit}
	case eventWindow:
		switch ev.windowEvent {
		case windowEventClose:
			return Event{Kind: EventWindowClose}
		case windowEventResized, windowEventSizeChanged:
			if ev.data1 > 0 && ev.data2 > 0 {
				win.w, win.h = int(ev.data1), int(ev.data2)
			}
			return Event{Kind: EventWindowResized, Width: win.w, Height: win.h}
		case windowEventMoved:
			// 位置变化：系统可能夹取请求值（如贴边吸附），所以必须以事件
			// 里的权威值为准更新平移基准。
			win.applyMoved(int(ev.data1), int(ev.data2))
			return Event{}
		case windowEventExposed:
			return Event{Kind: EventWindowExposed}
		case windowEventLeave:
			// 鼠标离开窗口：上层清掉悬停态（:hover 随之失效）。
			return Event{Kind: EventMouseLeave}
		}
	case eventMouseMotion:
		// motion 刷新 SDL 内部缓存：从这条起，缓存坐标相对的就是当前窗口
		// 位置（SDL 用 lParam 现算后写缓存，坐标系随窗口走）。
		win.noteMotion()
		return Event{Kind: EventMouseMotion, X: int(ev.x), Y: int(ev.y)}
	case eventMouseButtonDown:
		x, y := win.fixStale(int(ev.x), int(ev.y))
		return Event{Kind: EventMouseButtonDown, Button: ev.button, X: x, Y: y}
	case eventMouseButtonUp:
		x, y := win.fixStale(int(ev.x), int(ev.y))
		return Event{Kind: EventMouseButtonUp, Button: ev.button, X: x, Y: y}
	case eventMouseWheel:
		x, y := win.fixStale(int(ev.x), int(ev.y))
		return Event{Kind: EventMouseWheel, WheelX: int(ev.wheelX), WheelY: int(ev.wheelY), X: x, Y: y}
	case eventKeyDown:
		return Event{Kind: EventKeyDown, Key: int(ev.sym), Mod: ev.mod}
	case eventTextInput:
		return Event{Kind: EventTextInput, Text: ev.text}
	}
	return Event{}
}

// noteMotion 记录「鼠标缓存刚被这条 motion 刷新」：缓存坐标从此相对
// 当前窗口位置。SDL 只在 motion 里更新 mouse->x/y（WM_LBUTTONDOWN 用的是
// 缓存值），按钮/滚轮事件的坐标平移基准全靠这条约定跟踪。
func (win *Window) noteMotion() {
	win.originX, win.originY = win.posX, win.posY
	win.originKnown = win.posKnown
}

// fixStale 把按钮/滚轮事件里可能过期的缓存坐标校正回当前窗口坐标系。
//
// 背景：SDL2 的 button/wheel 事件不读消息的 lParam，直接取内部缓存
// mouse->x/y（实测：给 WM_LBUTTONDOWN 塞新 lParam，解出的仍是上一条
// motion 的值），而该缓存只在 motion 时刷新。于是「窗口移动之后、下一条
// motion 之前」的按下事件，坐标仍是相对旧窗口位置的——snip 截图后原地
// 缩窗，第一次点击带的是旧全屏窗口的坐标，被工具条判定放行给引擎，
// 表现为「必须先点一下才拖得动」。
//
// 只要观测到过 motion（originKnown）且位置已知（posKnown），就能按
// 「缓存原点相对当前位置」的位移平移回来；二者任一未知时无法推断，
// 保持原样（宁可不动，也不能猜错方向）。
func (win *Window) fixStale(x, y int) (int, int) {
	if win.originKnown && win.posKnown {
		return x + (win.originX - win.posX), y + (win.originY - win.posY)
	}
	return x, y
}

// Ticks 返回自 SDL 初始化以来的毫秒数（主循环用来算自动退出时刻）。
func (win *Window) Ticks() uint32 { return uint32(sdlGetTicks()) }

// ModState 返回当前修饰键位掩码（KmodShift/KmodCtrl/KmodAlt）。
//
// 文本输入通道会混进 Ctrl+V 之类的控制码，要靠它把「组合键」和「打字」
// 分开——所以掩码的含义属于平台层，由后端定义。
func (win *Window) ModState() uint16 { return uint16(sdlGetModState()) }

// InjectEvent 往事件队列注入一个平台事件（测试用）。
//
// 真实事件由 SDL 产生；测试要用「按下鼠标」「敲键盘」这类事件驱动窗口，
// 就靠它——比在测试里手搓 SDL_Event 字节布局稳得多。
//
// 字节布局按 SDL2 的事件结构填（字段偏移见 sdl2.go 的 decodeEvent 注释）：
// 类型码用 SDL 的值，事件体内各字段的位置与真实事件一致，decodeEvent
// 才能照常解出。
func (win *Window) InjectEvent(ev Event) {
	var buf [eventBufferSize]byte
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(buf[off:off+4], v) }
	switch ev.Kind {
	case EventQuit:
		put32(0, eventQuit)
	case EventWindowClose:
		put32(0, eventWindow)
		put32(12, uint32(windowEventClose))
	case EventWindowResized:
		put32(0, eventWindow)
		put32(12, uint32(windowEventResized))
		put32(16, uint32(int32(ev.Width)))
		put32(20, uint32(int32(ev.Height)))
	case EventWindowExposed:
		put32(0, eventWindow)
		put32(12, uint32(windowEventExposed))
	case EventMouseLeave:
		put32(0, eventWindow)
		put32(12, uint32(windowEventLeave))
	case EventMouseMotion:
		put32(0, eventMouseMotion)
		put32(20, uint32(int32(ev.X)))
		put32(24, uint32(int32(ev.Y)))
	case EventMouseButtonDown, EventMouseButtonUp:
		if ev.Kind == EventMouseButtonDown {
			put32(0, eventMouseButtonDown)
		} else {
			put32(0, eventMouseButtonUp)
		}
		buf[16] = ev.Button
		buf[17] = 1 // 按下态
		put32(20, uint32(int32(ev.X)))
		put32(24, uint32(int32(ev.Y)))
	case EventMouseWheel:
		put32(0, eventMouseWheel)
		put32(16, uint32(int32(ev.WheelX)))
		put32(20, uint32(int32(ev.WheelY)))
		put32(36, uint32(int32(ev.X)))
		put32(40, uint32(int32(ev.Y)))
	case EventKeyDown:
		put32(0, eventKeyDown)
		put32(20, uint32(int32(ev.Key)))
		binary.LittleEndian.PutUint16(buf[24:26], ev.Mod)
	case EventTextInput:
		put32(0, eventTextInput)
		copy(buf[12:44], ev.Text)
	}
	sdlPushEvent(uintptr(unsafe.Pointer(&buf[0])))
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

// SetClipboardText 把 UTF-8 文本写入系统剪贴板。
//
// 引擎只负责选区文本，剪贴板读写属平台能力：应用经 runtime/clipboard
// （通用入口）→ backend/clipboard（平台实现）落盘，engine 保持纯 Go、
// 无平台依赖。
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

// NativeHandle 返回窗口的原生 HWND（Windows），供原生对话框（runtime/dialog
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

// SetSize 运行期修改窗口尺寸（截图预览从整屏缩成选区大小）。
//
// 立即更新视口尺寸：SDL_SetWindowSize 的 WINDOWEVENT 要到下一轮事件
// 才到，而调用方通常在同一轮里还要打开新文档/重排——等事件会让第一帧
// 按旧尺寸布局。事件到达后再写一次同样的值，没有副作用。
func (win *Window) SetSize(w, h int) {
	if win == nil || win.closed || win.win == 0 || w <= 0 || h <= 0 {
		return
	}
	win.w, win.h = w, h
	sdlSetWindowSize(win.win, uintptr(w), uintptr(h))
}

// MoveTo 运行期移动窗口（拖动悬浮预览）。坐标是虚拟桌面的全局坐标。
func (win *Window) MoveTo(x, y int) {
	if win == nil || win.closed || win.win == 0 {
		return
	}
	sdlSetWindowPosition(win.win, uintptr(x), uintptr(y))
	win.applyMoved(x, y)
}

// applyMoved 更新「窗口当前位置」——鼠标缓存坐标的平移基准。
//
// 两个来源：MoveTo（请求值，立即可用，不必等事件绕一圈）与 SDL 的
// MOVED 事件（权威值：系统可能夹取请求位置）。语义相同，收敛到一个
// 方法里，免得两处各自维护 posKnown。
func (win *Window) applyMoved(x, y int) {
	win.posX, win.posY = x, y
	win.posKnown = true
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

// Closed 报告窗口是否已关闭（Close 之后为 true）。
func (win *Window) Closed() bool { return win != nil && win.closed }

// Shown 报告窗口是否已经显示（首次 Present 之后为 true）。
//
// 窗口建出来时是隐藏的：交换链在首次 Present 前没有有效画面，可见就会
// 先闪一段黑底。
func (win *Window) Shown() bool { return win != nil && win.shown }

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

var _ core.Viewport = (*Window)(nil)
