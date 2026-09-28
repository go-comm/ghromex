//go:build windows

// Package sdl2 是 ghromex 的 SDL2 渲染后端。
//
// 通过 golang.org/x/sys/windows 动态加载 SDL2.dll / SDL2_ttf.dll（Windows），
// 无需 CGO 与 C 工具链。运行前需将 SDL2.dll、SDL2_ttf.dll（及其依赖 zlib1.dll）
// 放入工程根目录 libs/，或保证系统可搜索到它们。
package sdl2

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// nativeCall 是 C 导出函数的统一调用签名（x64 Windows，参数均为指针宽度值）。
type nativeCall = func(args ...uintptr) uintptr

var (
	loadOnce sync.Once
	loadErr  error
	bindErr  error

	dllSDL *windows.LazyDLL
	dllTTF *windows.LazyDLL

	sdlInit                     nativeCall
	sdlQuit                     nativeCall
	sdlGetError                 nativeCall
	sdlCreateWindow             nativeCall
	sdlSetWindowTitle           nativeCall
	sdlDestroyWindow            nativeCall
	sdlCreateRenderer           nativeCall
	sdlDestroyRenderer          nativeCall
	sdlSetRenderDrawBlendMode   nativeCall
	sdlSetRenderDrawColor       nativeCall
	sdlRenderClear              nativeCall
	sdlRenderPresent            nativeCall
	sdlRenderFillRect           nativeCall
	sdlRenderCopy               nativeCall
	sdlCreateTextureFromSurface nativeCall
	sdlDestroyTexture           nativeCall
	sdlSetTextureBlendMode      nativeCall
	sdlFreeSurface              nativeCall
	sdlPollEvent                nativeCall
	sdlDelay                    nativeCall
	sdlGetTicks                 nativeCall
	sdlQueryTexture             nativeCall
	sdlRenderReadPixels         nativeCall
	sdlGetRenderOutputFormat    nativeCall
	sdlClearError               nativeCall

	ttfInit              nativeCall
	ttfQuit              nativeCall
	ttfOpenFont          nativeCall
	ttfCloseFont         nativeCall
	ttfSizeUTF8          nativeCall
	ttfRenderUTF8Blended nativeCall
	ttfSetFontStyle      nativeCall
	ttfSetFontSize       nativeCall
	ttfFontAscent        nativeCall
	ttfSetFontHinting    nativeCall
	ttfRenderUTF8LCD     nativeCall
	hasTTFSetFontSize    bool
	hasTTFHinting        bool
	hasTTFLCD            bool
)

// SDL 常量
const (
	initVideo          = 0x00000020
	windowShown        = 0x00000004
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
	eventMouseButtonDown = 0x401
	eventMouseButtonUp   = 0x402

	// SDL_WindowEvent.event 真实取值：CLOSE=0x0E RESIZED=0x05 SIZE_CHANGED=0x06
	windowEventClose       = 0x0E
	windowEventResized     = 0x05
	windowEventSizeChanged = 0x06

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
	sdlSetWindowTitle = bindSdl("SDL_SetWindowTitle")
	sdlDestroyWindow = bindSdl("SDL_DestroyWindow")
	sdlCreateRenderer = bindSdl("SDL_CreateRenderer")
	sdlDestroyRenderer = bindSdl("SDL_DestroyRenderer")
	sdlSetRenderDrawBlendMode = bindSdl("SDL_SetRenderDrawBlendMode")
	sdlSetRenderDrawColor = bindSdl("SDL_SetRenderDrawColor")
	sdlRenderClear = bindSdl("SDL_RenderClear")
	sdlRenderPresent = bindSdl("SDL_RenderPresent")
	sdlRenderFillRect = bindSdl("SDL_RenderFillRect")
	sdlRenderCopy = bindSdl("SDL_RenderCopy")
	sdlCreateTextureFromSurface = bindSdl("SDL_CreateTextureFromSurface")
	sdlDestroyTexture = bindSdl("SDL_DestroyTexture")
	sdlSetTextureBlendMode = bindSdl("SDL_SetTextureBlendMode")
	sdlFreeSurface = bindSdl("SDL_FreeSurface")
	sdlPollEvent = bindSdl("SDL_PollEvent")
	sdlDelay = bindSdl("SDL_Delay")
	sdlGetTicks = bindSdl("SDL_GetTicks")
	sdlQueryTexture = bindSdl("SDL_QueryTexture")
	sdlRenderReadPixels = bindProcOpt(dllSDL, "SDL_RenderReadPixels")
	sdlGetRenderOutputFormat = bindProcOpt(dllSDL, "SDL_GetRenderOutputFormat")
	sdlClearError = bindProcOpt(dllSDL, "SDL_ClearError")
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
	ttfFontAscent, _ = bindProbe(dllTTF, "TTF_FontAscent")
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
// 生命周期由 SDL 保证，并非 Go 指针的 uintptr 中转。直接写
// unsafe.Pointer(ptr) 会被 go vet 的 unsafeptr 检查误报，这里采用
// 内存重解释（按位读取局部变量的值）绕过误报，语义完全等价。
func goString(uptr uintptr) string {
	if uptr == 0 {
		return ""
	}
	base := *(*unsafe.Pointer)(unsafe.Pointer(&uptr))
	for n := 0; n < 4096; n++ {
		if *(*byte)(unsafe.Add(base, n)) == 0 {
			buf := make([]byte, n)
			for i := 0; i < n; i++ {
				buf[i] = *(*byte)(unsafe.Add(base, uintptr(i)))
			}
			return string(buf)
		}
	}
	return ""
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

// event 是 SDL_Event 常用字段的解析结果。
type event struct {
	typ         uint32
	windowEvent uint8
	data1       int32
	data2       int32
	button      uint8
	state       uint8
	x, y        int32
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
//     （SDL2 从未把 timestamp 移到偏移 8，那是 SDL3 的布局，勿混淆）
func decodeEvent(buf *[eventBufferSize]byte) event {
	ev := event{typ: le32(buf[0:4])}
	switch ev.typ {
	case eventWindow:
		ev.windowEvent = buf[12]
		ev.data1 = int32(le32(buf[16:20]))
		ev.data2 = int32(le32(buf[20:24]))
	case eventMouseButtonDown, eventMouseButtonUp:
		ev.button = buf[16]
		ev.state = buf[17]
		ev.x = int32(le32(buf[20:24]))
		ev.y = int32(le32(buf[24:28]))
	}
	return ev
}
