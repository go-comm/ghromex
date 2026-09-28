package sdl2

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/go-comm/ghromex/engine"
)

// drawDebug 为 true 时（环境变量 GHROMEX_DEBUG 非空），绘制调用失败会打印 SDL 错误。
var drawDebug = os.Getenv("GHROMEX_DEBUG") != ""

func checkDraw(what string, ret uintptr) {
	if drawDebug && ret != 0 {
		fmt.Fprintf(os.Stderr, "[ghromex] %s 失败: %s\n", what, lastError())
	}
}

// Graphics 基于 SDL2 Renderer + SDL_ttf 实现 engine.Graphics。
type Graphics struct {
	renderer    uintptr
	defaultFont string

	fonts    map[fontKey]*fontEntry
	textures map[texKey]texInfo
}

type fontKey struct {
	path string
	size int
	bold bool
}

type fontEntry struct {
	handle uintptr
}

type texKey struct {
	fontPath   string
	text       string
	size       int
	bold       bool
	r, g, b, a uint8
}

type texInfo struct {
	tex uintptr
	w   int
	h   int
}

var fontCandidates = []string{
	`C:\Windows\Fonts\msyh.ttc`,
	`C:\Windows\Fonts\msyh.ttf`,
	`C:\Windows\Fonts\simhei.ttf`,
	`C:\Windows\Fonts\simsun.ttc`,
	`C:\Windows\Fonts\arial.ttf`,
}

// DefaultFontPath 返回系统上第一个可用的字体文件。
func DefaultFontPath() string {
	if p := os.Getenv("GHROMEX_FONT"); p != "" {
		return p
	}
	for _, p := range fontCandidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return fontCandidates[0]
}

// NewGraphics 创建渲染上下文。renderer 为 SDL_Renderer 句柄。
func NewGraphics(renderer uintptr, fontPath string) *Graphics {
	if fontPath == "" {
		fontPath = DefaultFontPath()
	}
	return &Graphics{
		renderer:    renderer,
		defaultFont: fontPath,
		fonts:       make(map[fontKey]*fontEntry),
		textures:    make(map[texKey]texInfo),
	}
}

func (g *Graphics) resolveFontPath(family string) string {
	if family != "" && family != "default" {
		if _, err := os.Stat(family); err == nil {
			return family
		}
		for _, ext := range []string{".ttf", ".ttc", ".otf"} {
			p := `C:\Windows\Fonts\` + family + ext
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return g.defaultFont
}

func (g *Graphics) font(path string, size int, bold bool) uintptr {
	if size <= 0 {
		size = 14
	}
	key := fontKey{path: path, size: size, bold: bold}
	if f, ok := g.fonts[key]; ok {
		return f.handle
	}
	b, p := cBytes(path)
	h := ttfOpenFont(p, uintptr(size))
	runtime.KeepAlive(b)
	if h == 0 {
		return 0
	}
	if bold {
		ttfSetFontStyle(h, ttfStyleBold)
	}
	g.fonts[key] = &fontEntry{handle: h}
	return h
}

// MeasureText 实现 engine.Graphics。
func (g *Graphics) MeasureText(text string, fontSize int, bold bool) (int, int) {
	if text == "" {
		return 0, 0
	}
	f := g.font(g.defaultFont, fontSize, bold)
	if f == 0 {
		// 字体缺失时按引擎默认估算，保证布局可以继续
		w := 0
		for _, r := range text {
			if r > 0x2E80 {
				w += fontSize
			} else {
				w += fontSize * 3 / 5
			}
		}
		return w, fontSize * 13 / 10
	}
	var w, h int32
	b, p := cBytes(text)
	ttfSizeUTF8(f, p, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)))
	runtime.KeepAlive(b)
	return int(w), int(h)
}

// DrawText 实现 engine.Graphics：文本按（字体+字号+粗体+颜色+内容）缓存纹理。
func (g *Graphics) DrawText(x, y, w, h int, paint engine.Paint, text string) {
	if text == "" {
		return
	}
	size := 14
	if s := paint.Size(); s != nil {
		size = s.Pixel()
	}
	var cr, cg, cb, ca uint8 = 0, 0, 0, 255
	if c := paint.Color(); c != nil {
		cr, cg, cb, ca = c.RGBA()
	}
	fontPath := g.resolveFontPath(paint.FontFamily())
	bold := paint.Bold()

	key := texKey{fontPath: fontPath, text: text, size: size, bold: bold, r: cr, g: cg, b: cb, a: ca}
	info, ok := g.textures[key]
	if !ok {
		f := g.font(fontPath, size, bold)
		if f == 0 {
			return
		}
		// SDL_Color 是 4 字节结构体，Windows x64 ABI 下按值传入寄存器（R8 低 32 位），
		// 必须按 RGBA 小端打包，不能传指针。
		color := uintptr(cr) | uintptr(cg)<<8 | uintptr(cb)<<16 | uintptr(ca)<<24
		b, p := cBytes(text)
		surf := ttfRenderUTF8Blended(f, p, color)
		runtime.KeepAlive(b)
		if surf == 0 {
			return
		}
		// SDL_Surface: flags(4)+pad(4)+format ptr(8)+w(4)+h(4)
		tex := sdlCreateTextureFromSurface(g.renderer, surf)
		sdlFreeSurface(surf)
		if tex == 0 {
			return
		}
		var tw, th int32
		sdlQueryTexture(tex, 0, 0,
			uintptr(unsafe.Pointer(&tw)), uintptr(unsafe.Pointer(&th)))
		sdlSetTextureBlendMode(tex, blendModeBlend)
		info = texInfo{tex: tex, w: int(tw), h: int(th)}
		g.textures[key] = info
	}

	dst := sdlRect{int32(x), int32(y), int32(info.w), int32(info.h)}
	checkDraw("SDL_RenderCopy", sdlRenderCopy(g.renderer, info.tex, 0, uintptr(unsafe.Pointer(&dst))))
}

// DrawColor 实现 engine.Graphics。
func (g *Graphics) DrawColor(x, y, w, h int, color engine.Color) {
	if color == nil || w <= 0 || h <= 0 {
		return
	}
	r, gr, b, a := color.RGBA()
	sdlSetRenderDrawColor(g.renderer, uintptr(r), uintptr(gr), uintptr(b), uintptr(a))
	rect := sdlRect{int32(x), int32(y), int32(w), int32(h)}
	checkDraw("SDL_RenderFillRect", sdlRenderFillRect(g.renderer, uintptr(unsafe.Pointer(&rect))))
}

// DrawImage 暂不支持（预留接口）。
func (g *Graphics) DrawImage(x, y, w, h int, image engine.Image) {
}

// Clear 清屏。
func (g *Graphics) Clear(r, gr, b, a uint8) {
	checkDraw("SDL_SetRenderDrawColor", sdlSetRenderDrawColor(g.renderer, uintptr(r), uintptr(gr), uintptr(b), uintptr(a)))
	checkDraw("SDL_RenderClear", sdlRenderClear(g.renderer))
}

// Present 提交帧。
func (g *Graphics) Present() {
	sdlRenderPresent(g.renderer)
}

var _ engine.Graphics = (*Graphics)(nil)
