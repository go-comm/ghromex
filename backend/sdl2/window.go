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
	//
	// GHROMEX_SOFTWARE 按布尔解析（1/true/yes/on 才算开）：只判"非空"的
	// 时候，GHROMEX_SOFTWARE=0 也会强制软件渲染——用户明明要求走 GPU，却拿到
	// 软件路径，且输出里看不出是谁强制的。
	softwareOnly := envBool("GHROMEX_SOFTWARE") ||
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
		// 驱动名要能查：软件路径的瓶颈是填充率，GPU 路径的瓶颈通常在
		// 纹理上传/draw call/swap 间隔——两者的排查方向完全不同，先确认
		// 走的是哪条路再量（GHROMEX_DEBUG 非空时打印）。
		driver := RendererDriver(renderer)
		switch {
		case gpu:
			fmt.Fprintf(os.Stderr, "ghromex: renderer=accelerated(GPU, driver=%s)\n", driver)
		case softwareOnly:
			// 强制软件与"GPU 不可用已回退"是两回事，排查方向不同：前者是
			// 配置（把开关去掉即可），后者是环境（换驱动/显卡）。
			fmt.Fprintf(os.Stderr,
				"ghromex: renderer=software (driver=%s; 强制软件：GHROMEX_SOFTWARE/dummy)\n", driver)
		default:
			fmt.Fprintf(os.Stderr,
				"ghromex: renderer=software (driver=%s; GPU 不可用，已回退)\n", driver)
		}
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

	return &Window{
		w: w, h: h, title: title,
		win: hwnd, renderer: renderer,
		g: NewGraphics(renderer, ""),
	}, nil
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
// 窗口尺寸变化顺带更新后端自己的尺寸：视口尺寸是平台事实，上层只管
// 读（ViewportWidth/Height）。
//
// 认不出的事件（窗口移动、鼠标进入/离开等）返回 EventUnknown——上层按
// 未知事件忽略即可（不能落到 EventQuit 上，否则窗口会莫名其妙地退出）。
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
		case windowEventExposed:
			return Event{Kind: EventWindowExposed}
		}
	case eventMouseMotion:
		return Event{Kind: EventMouseMotion, X: int(ev.x), Y: int(ev.y)}
	case eventMouseButtonDown:
		return Event{Kind: EventMouseButtonDown, Button: ev.button, X: int(ev.x), Y: int(ev.y)}
	case eventMouseButtonUp:
		return Event{Kind: EventMouseButtonUp, Button: ev.button, X: int(ev.x), Y: int(ev.y)}
	case eventMouseWheel:
		return Event{Kind: EventMouseWheel, WheelX: int(ev.wheelX), WheelY: int(ev.wheelY), X: int(ev.x), Y: int(ev.y)}
	case eventKeyDown:
		return Event{Kind: EventKeyDown, Key: int(ev.sym), Mod: ev.mod}
	case eventTextInput:
		return Event{Kind: EventTextInput, Text: ev.text}
	}
	return Event{}
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
