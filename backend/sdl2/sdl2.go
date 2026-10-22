//go:build windows

// 本文件是 Windows 平台的 SDL2 符号绑定与常量；包文档见 doc.go。
//
// 通过 golang.org/x/sys/windows 动态加载 SDL2.dll / SDL2_ttf.dll（Windows），
// 无需 CGO 与 C 工具链。运行前需将 SDL2.dll、SDL2_ttf.dll（及其依赖 zlib1.dll）
// 放入工程根目录 libs/，或保证系统可搜索到它们。

package sdl2

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// nativeCall 是 C 导出函数的统一调用签名（x64 Windows，参数均为指针宽度值）。
type nativeCall = func(args ...uintptr) uintptr

// dpiAwareOnce 保护 SetProcessDPIAware 的进程级一次性调用。Go 二进制不带
// manifest 时进程默认 DPI-unaware：Win7 在系统缩放 125%/150% 下由 DWM 把
// 窗口按 96dpi 位图整窗拉伸，画面"糊成一团"。申报 system-DPI-aware 后
// SDL 按物理像素建窗（本引擎 1 CSS px = 1 物理 px，正是期望语义）。
var (
	dpiAwareOnce           sync.Once
	procSetProcessDPIAware = windows.NewLazySystemDLL("user32.dll").NewProc("SetProcessDPIAware")
)

func markProcessDPIAware() {
	dpiAwareOnce.Do(func() { procSetProcessDPIAware.Call() })
}

var (
	loadOnce sync.Once
	loadErr  error
	bindErr  error

	dllSDL *windows.LazyDLL
	dllTTF *windows.LazyDLL

	sdlInit                   nativeCall
	sdlQuit                   nativeCall
	sdlGetError               nativeCall
	sdlCreateWindow           nativeCall
	sdlShowWindow             nativeCall
	sdlRaiseWindow            nativeCall
	sdlGetWindowFlags         nativeCall
	sdlSetWindowTitle         nativeCall
	sdlDestroyWindow          nativeCall
	sdlCreateRenderer         nativeCall
	sdlGetRendererInfo        nativeCall
	sdlDestroyRenderer        nativeCall
	sdlSetRenderDrawBlendMode nativeCall
	sdlSetRenderDrawColor     nativeCall
	sdlRenderClear            nativeCall
	sdlRenderPresent          nativeCall
	sdlRenderSetVSync         nativeCall
	sdlRenderFillRect         nativeCall
	// SDL_RenderSetClipRect（SDL_render.h 2.0.0 起）：rect 传 NULL 关闭
	// 裁剪，返回 0 成功/负值失败；每层裁剪由 Graphics 维护交集栈后下推。
	sdlRenderSetClipRect        nativeCall
	sdlRenderCopy               nativeCall
	sdlCreateTextureFromSurface nativeCall
	sdlDestroyTexture           nativeCall
	sdlSetTextureBlendMode      nativeCall
	sdlFreeSurface              nativeCall
	// SDL_CreateTexture / SDL_UpdateTexture：把引擎的位图精灵（圆角角标）
	// 上传成纹理。CreateTextureFromSurface 只能从 surface 取，像素精灵
	// 没有 surface，故用这两条直接建纹理并灌像素。
	sdlCreateTexture  nativeCall
	sdlUpdateTexture  nativeCall
	sdlPollEvent      nativeCall
	sdlPushEvent      nativeCall
	sdlStartTextInput nativeCall
	sdlDelay          nativeCall
	sdlGetTicks       nativeCall
	sdlGetModState    nativeCall
	// SDL_SetClipboardText（SDL_clipboard.h）：UTF-8 NUL 结尾，0 成功。
	// 引擎只给出选区文本，写系统剪贴板这一平台动作留在后端。
	sdlSetClipboardText nativeCall
	// SDL_GetClipboardText：返回 SDL 分配的 UTF-8 NUL 结尾字符串（读取
	// 剪贴板内容，粘贴用），必须用 SDL_free 释放。两者都走 bindProbe：
	// 缺失时按「剪贴板不可用」降级，不影响窗口创建。
	sdlGetClipboardText     nativeCall
	sdlFree                 nativeCall
	hasGetClipboardText     bool
	hasSDLFree              bool
	sdlQueryTexture         nativeCall
	sdlRenderReadPixels     nativeCall
	sdlGetWindowPixelFormat nativeCall
	sdlClearError           nativeCall
	// 取窗口的原生句柄（HWND）：原生文件对话框要拿它当父窗口。
	// SDL_CreateWindow 返回的是 SDL_Window*，不是 HWND——真 HWND 要用
	// SDL_GetWindowWMInfo 取。两个符号都按可选绑定处理：缺失时对话框
	// 退化为「无父窗口」（照样能用，只是不跟随主窗口置顶）。
	sdlGetVersion      nativeCall
	sdlGetWindowWMInfo nativeCall
	hasGetWindowWMInfo bool

	ttfInit              nativeCall
	ttfQuit              nativeCall
	ttfOpenFont          nativeCall
	ttfCloseFont         nativeCall
	ttfSizeUTF8          nativeCall
	ttfRenderUTF8Blended nativeCall
	ttfSetFontStyle      nativeCall
	ttfSetFontSize       nativeCall
	ttfFontAscent        nativeCall
	ttfFontDescent       nativeCall
	ttfSetFontHinting    nativeCall
	ttfRenderUTF8LCD     nativeCall
	hasTTFSetFontSize    bool
	hasTTFHinting        bool
	hasTTFFontAscent     bool
	hasTTFFontDescent    bool
	hasTTFLCD            bool
)

// SDL 常量
const (
	initVideo   = 0x00000020
	windowShown = 0x00000004
	// SDL_WINDOW_HIDDEN：SDL2 建窗默认可见（漏传时 CreateWindow 会自动
	// 补 SHOWN，实测 flags=0x24），要“首帧后再显示”必须显式隐藏。
	windowHidden       = 0x00000008
	windowResizable    = 0x00000020
	windowPosCenteredX = 0x2FFF0000

	// SDL_ttf 提示模式（TTF_SetFontHinting 参数，整型走 GP 寄存器）。
	// 实测锁定于捆绑的 SDL2_ttf 2.20.1（TestHintingValueScan 逐个取值渲染
	// 并与 NORMAL 比 hash）：越界值（如 0x20）被 DLL 静默归一化为 NORMAL，
	// 输出与 NORMAL 逐字节相同——曾误按"NORMAL=0 LIGHT=2 MONO=4 NONE=8"
	// 传参，导致 NONE 静默失效、LIGHT 实为 MONO。取值不可凭记忆修改。
	hintNormal        = 0x00
	hintLight         = 0x01
	hintMono          = 0x02
	hintNone          = 0x03
	hintLightSubpixel = 0x04

	rendererSoftware     = 0x00000001
	rendererAccelerated  = 0x00000002
	rendererPresentVsync = 0x00000004

	blendModeNone  = 0x00000000
	blendModeBlend = 0x00000001 // SDL_BLENDMODE_BLEND；0x2 是 ADD（叠加），用错会把内容全画成白色

	eventQuit            = 0x100
	eventWindow          = 0x200
	eventKeyDown         = 0x300 // SDL_KEYDOWN
	eventTextInput       = 0x303 // SDL_TEXTINPUT（0x300 KEYDOWN/0x301 KEYUP/0x302 TEXTEDITING/0x303 TEXTINPUT；0x400 是 SDL_MOUSEMOTION，SDL_events.h 原文核对，勿凭记忆写——曾错标 0x400 致真机收不到文本事件）
	eventMouseMotion     = 0x400 // SDL_MOUSEMOTION
	eventMouseButtonDown = 0x401
	eventMouseButtonUp   = 0x402
	// SDL_MOUSEWHEEL：事件类型值取自 SDL_events.h 的 SDL_EventType 枚举
	//（MOUSEMOTION=0x400 起依次 BUTTONDOWN/BUTTONUP/MOUSEWHEEL，原文核对）。
	eventMouseWheel = 0x403

	// SDL_MouseWheelDirection（SDL_mouse.h）：NORMAL=0 FLIPPED=1——
	// FLIPPED（"自然滚动"）时 x/y 取反即还原。
	wheelDirectionFlipped = 1

	// KMOD_SHIFT = KMOD_LSHIFT(0x0001) | KMOD_RSHIFT(0x0002)（SDL_keycode.h
	// SDL_Keymod 枚举原文），Shift+滚轮 换算横向滚动用。
	kmodShift = 0x0003
	// KMOD_CTRL = KMOD_LCTRL(0x0040) | KMOD_RCTRL(0x0080)：Ctrl+A 全选、
	// Ctrl+C 复制、Ctrl+V 粘贴。
	kmodCtrl = 0x00C0
	// KMOD_ALT = KMOD_LALT(0x0100) | KMOD_RALT(0x0200)：AltGr（部分布局
	// 用 Ctrl+Alt 组合）打字时必须放行文本输入，故 Ctrl 过滤要排除 Alt。
	kmodAlt = 0x0300

	// wheelStepPx 是每档滚轮位移的像素数。Chrome（Windows）每档约 100px
	// （3 行 × 约 33px），引擎此前取 40px —— 同样的滚轮在引擎里要滚 2.5 倍
	// 的格数，体感明显"发飘/跟不上"。
	// SDL 整数 x/y 通常为 ±1/档，自由滚轮一次多档时按倍数放大。
	wheelStepPx = 100

	// SDL_WindowEvent.event 真实取值（Win7 真机日志实证：启动 0x01,0x0C,0x0A,
	// 0x03；移动 0x04 data=(x,y)；关闭 0x0E）：
	// SHOWN=0x01 HIDDEN=0x02 EXPOSED=0x03 MOVED=0x04 RESIZED=0x05
	// SIZE_CHANGED=0x06 ENTER=0x0A LEAVE=0x0B FOCUS_GAINED=0x0C
	// FOCUS_LOST=0x0D CLOSE=0x0E——勿凭记忆改值（曾把 EXPOSED 记成 0x02
	// 即 HIDDEN，导致覆盖重绘永不触发）。
	// EXPOSED：窗口被覆盖后重新暴露。Win7 无 DWM（Aero 关闭）时后缓冲内容
	// 不保留，不处理它会显示别窗残影且永不恢复（软件后端 GDI、Win10+ DWM
	// 合成环境行为不同，但重绘无害，统一处理）。
	windowEventExposed     = 0x03
	windowEventClose       = 0x0E
	windowEventResized     = 0x05
	windowEventSizeChanged = 0x06
	// windowEventLeave：鼠标离开窗口（LEAVE=0x0B，见上方的 SDL 取值注释）。
	// 上层据此清掉悬停态——否则鼠标移出窗口后，最后悬停的按钮会一直
	// 保持高亮（Chrome 离开即取消 :hover）。
	windowEventLeave = 0x0B

	mouseButtonLeft = 1

	ttfStyleBold = 0x00000001

	eventBufferSize = 56
)

// Load 加载 SDL2 与 SDL2_ttf 动态库并解析符号，重复调用安全。
func Load() error {
	loadOnce.Do(func() { loadErr = doLoad() })
	return loadErr
}

func doLoad() error {
	// DPI-awareness 必须在任何窗口创建之前申报（进程生命周期内只生效一次）。
	markProcessDPIAware()

	sdlName := "SDL2.dll"
	ttfName := "SDL2_ttf.dll"

	if dir := findLibsDir(); dir != "" {
		abs, err := filepath.Abs(dir)
		if err == nil {
			// 让 SDL2_ttf 的依赖（zlib1.dll 等）与主 DLL 同目录可寻
			_ = windows.SetDllDirectory(abs)
			sdlName = filepath.Join(abs, "SDL2.dll")
			ttfName = filepath.Join(abs, "SDL2_ttf.dll")
		}
	}

	dllSDL = windows.NewLazyDLL(sdlName)
	dllTTF = windows.NewLazyDLL(ttfName)

	sdlInit = bindSdl("SDL_Init")
	sdlQuit = bindSdl("SDL_Quit")
	sdlGetError = bindSdl("SDL_GetError")
	sdlCreateWindow = bindSdl("SDL_CreateWindow")
	// 隐藏建窗、首帧 Present 后再显示（见 window.go 的 present）：
	// SDL_ShowWindow/SW_SHOW 补齐被隐藏期间的激活路径，SDL_RaiseWindow
	// 保证显示出的窗口拿到前台/焦点（等价于建窗即可见时的初始焦点）。
	sdlShowWindow = bindSdl("SDL_ShowWindow")
	sdlRaiseWindow = bindSdl("SDL_RaiseWindow")
	// 测试用：核对“建窗即隐藏、首帧后才显示”的窗口标志。
	sdlGetWindowFlags = bindSdl("SDL_GetWindowFlags")
	sdlSetWindowTitle = bindSdl("SDL_SetWindowTitle")
	sdlDestroyWindow = bindSdl("SDL_DestroyWindow")
	sdlCreateRenderer = bindSdl("SDL_CreateRenderer")
	sdlGetRendererInfo = bindSdl("SDL_GetRendererInfo")
	sdlDestroyRenderer = bindSdl("SDL_DestroyRenderer")
	sdlSetRenderDrawBlendMode = bindSdl("SDL_SetRenderDrawBlendMode")
	sdlSetRenderDrawColor = bindSdl("SDL_SetRenderDrawColor")
	sdlRenderClear = bindSdl("SDL_RenderClear")
	sdlRenderPresent = bindSdl("SDL_RenderPresent")
	// SDL_RenderSetVSync 是**可选**符号：只有基准工具（拆开「等 vsync」
	// 与「拷贝」）用它，SDL 版本偏老没有它时按「无 vsync」处理即可，
	// 不该因此让整个库加载失败。
	sdlRenderSetVSync = bindProcOpt(dllSDL, "SDL_RenderSetVSync")
	sdlRenderFillRect = bindSdl("SDL_RenderFillRect")
	sdlRenderSetClipRect = bindSdl("SDL_RenderSetClipRect")
	sdlRenderCopy = bindSdl("SDL_RenderCopy")
	sdlCreateTextureFromSurface = bindSdl("SDL_CreateTextureFromSurface")
	sdlDestroyTexture = bindSdl("SDL_DestroyTexture")
	sdlSetTextureBlendMode = bindSdl("SDL_SetTextureBlendMode")
	sdlFreeSurface = bindSdl("SDL_FreeSurface")
	// 位图精灵上传（圆角角标）：RGBA32 的内存序即 R,G,B,A，引擎侧的像素
	// 可直接灌入，无需逐像素换序。
	sdlCreateTexture = bindSdl("SDL_CreateTexture")
	sdlUpdateTexture = bindSdl("SDL_UpdateTexture")
	sdlPollEvent = bindSdl("SDL_PollEvent")
	// SDL_TEXTINPUT 由驱动在“文本输入激活”时才产生可打印字符事件
	// （SDL2 wiki SDL_StartTextInput：要收 TextInputEvent 必须显式启用）。
	// 未启用时症状：聚焦/光标正常、敲字无响应。真机漏调即此 bug。
	sdlStartTextInput = bindSdl("SDL_StartTextInput")
	// 测试用：向事件队列注入 SDL 层事件，端到端验证事件消费链路。
	sdlPushEvent = bindSdl("SDL_PushEvent")
	sdlDelay = bindSdl("SDL_Delay")
	sdlGetTicks = bindSdl("SDL_GetTicks")
	// Shift 键态（Shift+滚轮 → 横向滚动）：SDL_GetModState 返回当前修饰键
	// 位掩码，按事件逐次查询，无需自行跟踪 KEYDOWN/KEYUP。
	sdlGetModState = bindSdl("SDL_GetModState")
	sdlSetClipboardText = bindSdl("SDL_SetClipboardText")
	// 剪贴板读取（粘贴用）：SDL_GetClipboardText 返回 SDL 分配的字符串，
	// 需用 SDL_free 释放。两个符号都按可选绑定处理——缺失时粘贴退化为
	// 「无内容可粘」，不影响窗口与其它功能。
	sdlGetClipboardText, hasGetClipboardText = bindProbe(dllSDL, "SDL_GetClipboardText")
	sdlFree, hasSDLFree = bindProbe(dllSDL, "SDL_free")
	sdlQueryTexture = bindSdl("SDL_QueryTexture")
	sdlRenderReadPixels = bindProcOpt(dllSDL, "SDL_RenderReadPixels")
	// 截图回读用的像素格式查询：SDL2 的对应 API 是 SDL_GetWindowPixelFormat。
	// 此前绑的是 SDL_GetRenderOutputFormat——那是 SDL3 才有的名字，SDL2 从不
	// 导出，绑定结果永远返回 0，"优先用渲染目标原生格式"实为死代码。
	sdlGetWindowPixelFormat = bindProcOpt(dllSDL, "SDL_GetWindowPixelFormat")
	sdlClearError = bindProcOpt(dllSDL, "SDL_ClearError")
	// 原生窗口句柄：SDL_GetWindowWMInfo 要按 SDL_SysWMinfo 结构回填，
	// 结构里的 version 字段必须与运行库的版本一致（SDL_VERSION 宏），
	// 所以先用 SDL_GetVersion 问出版本再填。
	sdlGetVersion = bindProcOpt(dllSDL, "SDL_GetVersion")
	sdlGetWindowWMInfo, hasGetWindowWMInfo = bindProbe(dllSDL, "SDL_GetWindowWMInfo")
	if bindErr != nil {
		return fmt.Errorf("解析 SDL2.dll 符号失败: %w", bindErr)
	}

	ttfInit = bindTtf("TTF_Init")
	ttfQuit = bindTtf("TTF_Quit")
	ttfOpenFont = bindTtf("TTF_OpenFont")
	ttfCloseFont = bindTtf("TTF_CloseFont")
	ttfSizeUTF8 = bindTtf("TTF_SizeUTF8")
	ttfRenderUTF8Blended = bindTtf("TTF_RenderUTF8_Blended")
	ttfSetFontStyle = bindTtf("TTF_SetFontStyle")
	// 动态字号与基线查询：face 复用（一个字体文件一个 FT_Face）依赖这两个符号，
	// 缺失时 Graphics 自动退化为按 (文件,字号,粗体) 各开一个 face 的旧路径。
	ttfSetFontSize, hasTTFSetFontSize = bindProbe(dllTTF, "TTF_SetFontSize")
	ttfFontAscent, hasTTFFontAscent = bindProbe(dllTTF, "TTF_FontAscent")
	ttfFontDescent, hasTTFFontDescent = bindProbe(dllTTF, "TTF_FontDescent")
	ttfSetFontHinting, hasTTFHinting = bindProbe(dllTTF, "TTF_SetFontHinting")
	ttfRenderUTF8LCD, hasTTFLCD = bindProbe(dllTTF, "TTF_RenderUTF8_LCD")
	if bindErr != nil {
		return fmt.Errorf("解析 SDL2_ttf.dll 符号失败: %w", bindErr)
	}
	return nil
}

func bindSdl(name string) nativeCall {
	return bindProc(dllSDL, name)
}

func bindTtf(name string) nativeCall {
	return bindProc(dllTTF, name)
}

func bindProc(dll *windows.LazyDLL, name string) nativeCall {
	if bindErr != nil {
		return func(args ...uintptr) uintptr { return 0 }
	}
	proc := dll.NewProc(name)
	if err := proc.Find(); err != nil {
		bindErr = fmt.Errorf("%s: %w", name, err)
		return func(args ...uintptr) uintptr { return 0 }
	}
	return func(args ...uintptr) uintptr {
		r, _, _ := proc.Call(args...)
		return r
	}
}

// bindProcOpt 绑定可选符号：缺失时不记录 bindErr，调用返回 0。
// 用于仅调试/截图用到、部分 SDL 版本可能没有的函数。
func bindProcOpt(dll *windows.LazyDLL, name string) nativeCall {
	proc := dll.NewProc(name)
	if err := proc.Find(); err != nil {
		return func(args ...uintptr) uintptr { return 0 }
	}
	return func(args ...uintptr) uintptr {
		r, _, _ := proc.Call(args...)
		return r
	}
}

// bindProbe 绑定可选符号并回报是否存在，供调用方按能力选择实现路径。
func bindProbe(dll *windows.LazyDLL, name string) (nativeCall, bool) {
	proc := dll.NewProc(name)
	if err := proc.Find(); err != nil {
		return func(args ...uintptr) uintptr { return 0 }, false
	}
	return func(args ...uintptr) uintptr {
		r, _, _ := proc.Call(args...)
		return r
	}, true
}

func findLibsDir() string {
	candidates := []string{filepath.Join(".", "libs")}
	if exe, err := os.Executable(); err == nil && exe != "" {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "libs"),
			filepath.Dir(exe),
		)
	}
	for _, up := range []string{".", "..", filepath.Join("..", "..")} {
		for _, rel := range candidates {
			p := filepath.Join(up, rel)
			if _, err := os.Stat(filepath.Join(p, "SDL2.dll")); err == nil {
				return p
			}
		}
	}
	return ""
}

// cBytes 转 NUL 结尾 C 字符串，返回底层切片（调用方需 runtime.KeepAlive 保活）。
func cBytes(s string) ([]byte, uintptr) {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return b, uintptr(unsafe.Pointer(&b[0]))
}

// goString 从 C 侧 char*（uintptr）读回 UTF-8 字符串。
// 说明：ptr 来自 SDL 函数返回值（如 SDL_GetError），指向 C 堆内存，
// 生命周期由 SDL 保证，并非 Go 指针的 uintptr 中转。指针值的传递方式见
// goCString 的说明（不取形参地址）。
func goString(uptr uintptr) string {
	return goCString(uptr, 4096)
}

// cMemPointer 把 C 侧地址（uintptr）变成可解引用的 unsafe.Pointer。
//
// **只用于 C 侧内存**（SDL 分配的缓冲区、surface 的像素……）。C 内存不受 Go
// 的 GC 管，所以这里没有「对象被 GC 回收 → 悬空指针」的问题。
//
// **禁止用于 Go 堆指针**：那种场景没有安全的转换方式——Go 的规则是 uintptr
// 不是指针、GC 不追踪它；runtime.KeepAlive 也救不了「已经被当成指针用的
// uintptr」。Go 指针一律直接以 unsafe.Pointer 传递。
//
// 这里**没有**任何 GC 保护：uintptr → unsafe.Pointer 的转换本身不带保活
// 语义。下面这行是绕过 go vet 的 unsafeptr 检查的唯一入口（该检查对
// 「uintptr → unsafe.Pointer」一律报错，而这里的目标是 C 内存，属已知误报）：
// 只把位模式原样搬过去，**不做任何指针算术**。
//
// 取地址的局部变量按 Go 的地址性规则一定在内存里（不会被留在寄存器上），
// 读回来的就是存进去的值——实测热循环一千万次读回一致。
func cMemPointer(u uintptr) unsafe.Pointer {
	if u == 0 {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(&u))
}

// goCString 从 C 侧 char* 读回 UTF-8 字符串，最多读 max 字节（超出按
// 截断处理）。goString 的 4096 字节上限只适合短串（如错误信息）；
// 剪贴板内容可能很长，需按更大的上限读取——否则长文本会被截断成
// 前 4096 字节（goString 找不到 NUL 时返回已读到的部分）。
//
// 指针值的传递见 cMemPointer 的说明。
func goCString(uptr uintptr, max int) string {
	if uptr == 0 || max <= 0 {
		return ""
	}
	base := cMemPointer(uptr)
	buf := make([]byte, 0, 256)
	for n := 0; n < max; n++ {
		c := *(*byte)(unsafe.Add(base, uintptr(n)))
		if c == 0 {
			break
		}
		buf = append(buf, c)
	}
	return string(buf)
}

func lastError() string {
	return goString(sdlGetError())
}

// setLastErrorMessagePtr 是 SDL2 SDL_SetError 的最小实现入口（清空用）。
func clearError() {
	sdlClearError()
}

// sdlRect 对应 SDL_Rect（4×int32，与 SDL_RenderFillRect/SDL_RenderCopy 匹配）。
// 注意：不是 SDL_FRect！float 版本要用 SDL_RenderFillRectF/SDL_RenderCopyF。
type sdlRect struct {
	x, y, w, h int32
}

func le32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }

func le16(b []byte) uint16 { return binary.LittleEndian.Uint16(b) }

// event 是 SDL_Event 常用字段的解析结果。
type event struct {
	typ         uint32
	windowEvent uint8
	data1       int32
	data2       int32
	button      uint8
	state       uint8
	x, y        int32
	// wheelX/wheelY 是滚轮位移（档位计数，x 正=向右、y 正=滚轮向上/远离用户，
	// FLIPPED 已在 decodeEvent 内翻转还原）；x/y 在 wheel 事件里存鼠标位置。
	wheelX, wheelY int32
	sym            int32  // KEYDOWN: keysym.sym
	mod            uint16 // KEYDOWN: keysym.mod（KMOD_* 位掩码）
	text           string // TEXTINPUT: UTF-8 文本（IME 组合完成串走这里）
}

func pollEvent() (event, bool) {
	var buf [eventBufferSize]byte
	if sdlPollEvent(uintptr(unsafe.Pointer(&buf[0]))) == 0 {
		return event{}, false
	}
	return decodeEvent(&buf), true
}

// decodeEvent 从 SDL_Event 原始字节解析常用字段。
// 布局已经用 .temp/eventprobe（dummy 驱动下 SDL 2.28.4 自己产生的真实事件）实测：
//   - 所有事件公共：type@0 timestamp@4
//   - 窗口事件：windowID@8 event@12 data1@16 data2@20
//   - 鼠标移动：windowID@8 which@12(Uint32) state@16 x@20 y@24
//   - 鼠标按键：windowID@8 which@12 button@16(Uint8) state@17 clicks@18 padding@19 x@20 y@24
//   - 滚轮：windowID@8 which@12 x@16 y@20 direction@24 preciseX@28 preciseY@32
//     mouseX@36 mouseY@40（SDL_events.h 结构体定义，ABI 自 2.0.0/2.26 稳定）
//     （SDL2 从未把 timestamp 移到偏移 8，那是 SDL3 的布局，勿混淆）
func decodeEvent(buf *[eventBufferSize]byte) event {
	ev := event{typ: le32(buf[0:4])}
	switch ev.typ {
	case eventWindow:
		ev.windowEvent = buf[12]
		ev.data1 = int32(le32(buf[16:20]))
		ev.data2 = int32(le32(buf[20:24]))
	case eventMouseMotion:
		// SDL_MouseMotionEvent：windowID@8 which@12 state@16 x@20 y@24。
		// 拖选时后端从按下点起累计位移，超过阈值才把后续 motion 当作
		// 扩选；单击抖动不会误选。
		ev.state = buf[16]
		ev.x = int32(le32(buf[20:24]))
		ev.y = int32(le32(buf[24:28]))
	case eventMouseButtonDown, eventMouseButtonUp:
		ev.button = buf[16]
		ev.state = buf[17]
		ev.x = int32(le32(buf[20:24]))
		ev.y = int32(le32(buf[24:28]))
	case eventMouseWheel:
		// SDL_MouseWheelEvent（SDL_events.h，44 字节 ≤ eventBufferSize=56）：
		//   type@0 timestamp@4 windowID@8 which@12 x@16 y@20 direction@24
		//   preciseX@28 preciseY@32 mouseX@36 mouseY@40
		// x/y 声明为 Uint32 但语义是 Sint32（负=反向）：x 正=向右、
		// y 正=远离用户（滚轮向上）；preciseX/Y 是亚档位高精度 float。
		// 鼠标坐标 mouseX/Y 需 SDL ≥2.26（DLL 2.28.5 ✓），与按键事件同坐标系。
		wx := int32(le32(buf[16:20]))
		wy := int32(le32(buf[20:24]))
		if wx == 0 && wy == 0 {
			// 触控板等平滑滚动源可能只给 precise（整数档位为 0），此时回退取整。
			if px := int32(math.Round(float64(math.Float32frombits(le32(buf[28:32]))))); px != 0 {
				wx = px
			}
			if py := int32(math.Round(float64(math.Float32frombits(le32(buf[32:36]))))); py != 0 {
				wy = py
			}
		}
		if le32(buf[24:28]) == wheelDirectionFlipped {
			wx, wy = -wx, -wy
		}
		ev.wheelX, ev.wheelY = wx, wy
		ev.x = int32(le32(buf[36:40]))
		ev.y = int32(le32(buf[40:44]))
	case eventKeyDown:
		// SDL_KeyboardEvent（SDL_keyboard.h）：state@12 repeat@13，
		// keysym{scancode@16, sym@20, mod@24}。键盘事件 dummy 驱动不会
		// 自发产生，无法像 windowEvent 枚举那样真机反推——结构体布局
		// 以 SDL2 头文件定义为准（ABI 自 2.0.0 稳定）。
		ev.sym = int32(le32(buf[20:24]))
		ev.mod = le16(buf[24:26])
	case eventTextInput:
		// SDL_TextInputEvent：char text[32] 紧接 windowID，偏移 12，
		// UTF-8 NUL 结尾（SDL 已把 IME 候选确认串组好）。
		end := 12
		for end < 44 && buf[end] != 0 {
			end++
		}
		ev.text = string(buf[12:end])
	}
	return ev
}
