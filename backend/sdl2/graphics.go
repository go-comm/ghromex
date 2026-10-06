package sdl2

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

// ---------------------------------------------------------------------------
// 字体解析：对齐 Chrome 在 Windows 上的默认字体映射，唯一例外：
// standard（未指定 family）默认取 sans-serif（Arial）而非 Chrome 的 Times New Roman，
// 更贴合桌面应用 UI 习惯；serif 族仍映射 Times New Roman，不损失语义。
//   serif → Times New Roman；sans-serif → Arial；monospace → Courier New
//   cursive → Comic Sans MS；fantasy → Impact；system-ui → Segoe UI
//   汉字/假名/谚文等 CJK 字符按 per-script fallback → 微软雅黑
// ---------------------------------------------------------------------------

const fontsDir = `C:\Windows\Fonts`

// genericFamilyFiles CSS 通用族 → 字体文件名。
var genericFamilyFiles = map[string]string{
	"serif":         "times.ttf",
	"sans-serif":    "arial.ttf",
	"monospace":     "cour.ttf",
	"cursive":       "comic.ttf",
	"fantasy":       "impact.ttf",
	"system-ui":     "segoeui.ttf",
	"ui-serif":      "times.ttf",
	"ui-sans-serif": "segoeui.ttf",
	"ui-monospace":  "cour.ttf",
	"ui-rounded":    "segoeui.ttf",
}

// namedFontFiles 常见 family 名 → 文件名（多词名的文件名与家族名不同）。
var namedFontFiles = map[string]string{
	"times new roman": "times.ttf",
	"arial":           "arial.ttf",
	"courier new":     "cour.ttf",
	"segoe ui":        "segoeui.ttf",
	"microsoft yahei": "msyh.ttc",
	"微软雅黑":            "msyh.ttc",
	"simhei":          "simhei.ttf",
	"黑体":              "simhei.ttf",
	"simsun":          "simsun.ttc",
	"宋体":              "simsun.ttc",
	"comic sans ms":   "comic.ttf",
	"impact":          "impact.ttf",
}

var cjkFontCandidates = []string{"msyh.ttc", "simhei.ttf", "simsun.ttc"}

// fontFileOr 返回第一个存在的候选文件（相对 C:\Windows\Fonts），都不存在返回 def。
func fontFileOr(def, first string, rest ...string) string {
	check := append([]string{first}, rest...)
	for _, f := range check {
		if p := fontPathOf(f); p != "" {
			return p
		}
	}
	return def
}

// fontPathOf 把文件名或绝对路径规范为存在的完整路径；不存在返回 ""。
func fontPathOf(name string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return ""
	}
	if p := filepath.Join(fontsDir, name); fileExists(p) {
		return p
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// DefaultFontPath 返回 standard 字体（未指定 family 时使用）。
// 默认 sans-serif（Arial）：桌面 UI 习惯，有意区别于 Chrome 的 standard=Times。
// 可用 GHROMEX_FONT 覆盖。
func DefaultFontPath() string {
	if p := fontPathOf(os.Getenv("GHROMEX_FONT")); p != "" {
		return p
	}
	return fontFileOr(filepath.Join(fontsDir, "arial.ttf"),
		"arial.ttf", "segoeui.ttf", "msyh.ttc", "simsun.ttc", "arial.ttf")
}

// CJKFontPath 返回 CJK per-script fallback 字体，对齐 Chrome（zh Windows）：微软雅黑。
// 可用 GHROMEX_CJK_FONT 覆盖。
func CJKFontPath() string {
	if p := fontPathOf(os.Getenv("GHROMEX_CJK_FONT")); p != "" {
		return p
	}
	return fontFileOr(filepath.Join(fontsDir, "msyh.ttc"), cjkFontCandidates[0], cjkFontCandidates[1:]...)
}

// isScriptCJK 判断字符是否属于需要走 CJK fallback 的脚本
// （汉字/假名/谚文及 CJK 标点、全角符号，覆盖 Chrome per-script map 的中文域）。
func isScriptCJK(r rune) bool {
	switch {
	case r >= 0x2E80 && r <= 0x303E,
		r >= 0x3041 && r <= 0x33FF,
		r >= 0x31C0 && r <= 0x31EF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x4E00 && r <= 0x9FFF,
		r >= 0xA960 && r <= 0xA97F,
		r >= 0xAC00 && r <= 0xD7AF,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFFEF,
		r >= 0x20000 && r <= 0x2FA1F:
		return true
	}
	return false
}

// scriptSeg 是按脚本切出的一个连续子段。
type scriptSeg struct {
	text string
	cjk  bool
}

// splitScripts 按字符级脚本把文本切成连续段；单脚本时原样返回一段。
// 度量与绘制共用，保证布局宽度与落笔位置一致。
func splitScripts(text string) []scriptSeg {
	segs := make([]scriptSeg, 0, 4)
	cur := -1
	var b strings.Builder
	flush := func() {
		if cur >= 0 && b.Len() > 0 {
			segs = append(segs, scriptSeg{text: b.String(), cjk: cur == 1})
			b.Reset()
		}
	}
	for _, r := range text {
		k := 0
		if isScriptCJK(r) {
			k = 1
		}
		if cur < 0 {
			cur = k
		} else if k != cur {
			flush()
			cur = k
		}
		b.WriteRune(r)
	}
	flush()
	if len(segs) == 0 {
		return []scriptSeg{{text: text}}
	}
	return segs
}

// ---------------------------------------------------------------------------
// Graphics
// ---------------------------------------------------------------------------

// Graphics 基于 SDL2 Renderer + SDL_ttf 实现 engine.Graphics。
// 每个字体文件只打开一个 FT_Face（动态切字号/粗体），文本纹理按
// （解析后的文件+子段+字号+粗体+颜色）缓存；含 CJK 字符的段用 fallback 字体。
type Graphics struct {
	renderer     uintptr
	standardFont string // 未指定 family 时的 standard 字体
	cjkFont      string // CJK per-script fallback 字体

	faces    map[faceKey]*fontFace
	textures map[texKey]texInfo

	// reuseFaces 为 false 时（旧版 SDL2_ttf 无 TTF_SetFontSize），
	// faceKey 带上 size/bold，行为退化为每组合一个 face（与历史版本一致）。
	reuseFaces bool

	// clips 是裁剪矩形栈（与 engine.Clipper 的 Push/Pop 严格配对），栈顶
	// 即当前生效裁剪区；空栈 = 无裁剪。与 SDL viewport 同坐标系。
	clips []sdlRect
}

// 编译期断言：SDL 后端必须实现 engine.Clipper，否则 overflow 裁剪会静默失效。
var _ engine.Clipper = (*Graphics)(nil)

type faceKey struct {
	path string
	size int
	bold bool
}

type fontFace struct {
	handle uintptr
	size   int
	bold   bool
}

// lcdText 报告 LCD 子像素渲染是否启用（TTF_RenderUTF8_LCD + LIGHT_SUBPIXEL
// hinting）。默认关闭：文本默认灰度 AA（hinting 档位见 fontHinting，默认
// NORMAL；GHROMEX_HINT 可切 light|mono|none），无彩边、对落点背景色不敏感。
// GHROMEX_LCD=1 开启：竖笔画边缘横向锐度与浏览器 ClearType
// 同级，代价是彩底上竖笔边缘轻微彩边（与浏览器子像素渲染同性质），且需按
// 落点背景色合成（见 DrawText）。
// 惰性函数而非包级变量：hasTTFLCD 在 Load()（DLL 符号解析）后才有值，
// 包初始化时求值恒为 false。
func lcdText() bool {
	return os.Getenv("GHROMEX_LCD") == "1" && hasTTFLCD
}

// fontHinting 返回建 face 时设置的 hinting 档位（hinting 是 face 级状态，
// 进程启动前设定的环境变量才对全部 face 生效）。LCD 强制 LIGHT_SUBPIXEL
// （子像素分解依赖该档位）；灰度路径按 GHROMEX_HINT 选档：
// normal|light|mono|none，默认/非法值 = normal。
// 默认 normal 的依据（.temp/gray-vs-chrome 矩阵实测 vs Chrome 灰度基线）：
// CJK13 hardH 7≈0（Chrome 0，竖画无锯齿）、hardV 237≈227（横画利）、ink 412
// 接近 Chrome 347——是全部候选里与 Chrome 轮廓最同构的一档；light 纵向更软
// （hardV 75，即"糊"的来源）。回退/扫描用 GHROMEX_HINT=light|none|mono。
// 该变量只影响灰度路径——LCD 打开时被 subpixel 覆盖。
func fontHinting() uintptr {
	if lcdText() {
		return hintLightSubpixel
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GHROMEX_HINT"))) {
	case "light":
		return hintLight
	case "mono":
		return hintMono
	case "none":
		return hintNone
	default: // ""/"normal"/未知值 → normal
		return hintNormal
	}
}

// textContrast 返回灰度文本对比度系数 k（GHROMEX_CONTRAST），驱动 blended
// 字形 surface 覆盖通道的 tone 曲线（见 text_contrast.go）。空/off/非法/≤1
// = 关闭（k=0）；上限 3。LCD 路径不适用（其纹理已在 DLL 内按前景×背景合成）。
func textContrast() float64 {
	v := strings.TrimSpace(os.Getenv("GHROMEX_CONTRAST"))
	if v == "" || strings.EqualFold(v, "off") {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 1 {
		return 0
	}
	if f > 3 {
		return 3
	}
	return f
}

type texKey struct {
	fontPath string
	text     string
	size     int
	bold     bool
	lcd      bool
	// contrast = textContrast()×100（灰度路径的 tone 曲线档位）：随环境变量
	// 变化重建纹理，避免切换后仍命中旧缓存。LCD 路径不施加曲线（值恒同）。
	contrast   uint16
	r, g, b, a uint8
	// 落点背景色（LCD 合成用；背景不同必须分别缓存）
	br, bg, bb uint8
}
type texInfo struct {
	tex    uintptr
	w      int
	h      int
	ascent int32 // 该 face 当前样式的基线（TTF_FontAscent），0 表示不可用
}

// NewGraphics 创建渲染上下文。renderer 为 SDL_Renderer 句柄；
// fontPath 非空时作为 standard 字体（测试/定制用）。
func NewGraphics(renderer uintptr, fontPath string) *Graphics {
	std := DefaultFontPath()
	if fontPath != "" {
		std = fontPath
	}
	return &Graphics{
		renderer:     renderer,
		standardFont: std,
		cjkFont:      CJKFontPath(),
		faces:        make(map[faceKey]*fontFace),
		textures:     make(map[texKey]texInfo),
		reuseFaces:   hasTTFSetFontSize,
	}
}

// resolveFontPath 把 CSS font-family（可含逗号分隔列表）解析为字体文件路径。
// 逐个候选尝试：通用族 → 具名别名 → 直接文件名/绝对路径 → 家族名+常见扩展。
func (g *Graphics) resolveFontPath(family string) string {
	for _, raw := range strings.Split(family, ",") {
		name := strings.ToLower(strings.Trim(strings.TrimSpace(raw), `"'`))
		if name == "" || name == "standard" {
			return g.standardFont
		}
		if file, ok := genericFamilyFiles[name]; ok {
			if p := fontPathOf(file); p != "" {
				return p
			}
			continue
		}
		if file, ok := namedFontFiles[name]; ok {
			if p := fontPathOf(file); p != "" {
				return p
			}
			continue
		}
		// 直接给出文件名（含或不含目录）或绝对路径
		if p := fontPathOf(name); p != "" {
			return p
		}
		for _, ext := range []string{".ttf", ".ttc", ".otf"} {
			if p := fontPathOf(name + ext); p != "" {
				return p
			}
		}
	}
	return g.standardFont
}

// face 取得（并按需创建/切换）字体句柄。reuseFaces 时一个文件一个 face，
// 用 TTF_SetFontSize / TTF_SetFontStyle 动态切换，避免同一 TTC 按字号重复加载。
func (g *Graphics) face(path string, size int, bold bool) *fontFace {
	key := faceKey{path: path}
	if !g.reuseFaces {
		key.size, key.bold = size, bold
	}
	f, ok := g.faces[key]
	if !ok {
		b, p := cBytes(path)
		h := ttfOpenFont(p, uintptr(size))
		runtime.KeepAlive(b)
		if h == 0 {
			return nil
		}
		// f.bold 记 handle 的**实际**样式而非请求值：TTF_OpenFont 初始恒为
		// regular，先记 false，下方统一样式切换才会把首个 bold 请求真正落到
		// handle 上。若记请求值 bold=true，会因 "f.bold == bold" 跳过
		// TTF_SetFontStyle——首个请求即 bold 时（页面首段文字是粗体标题很
		// 常见，如 form-ua 的 h2）handle 一直按 regular 度量与绘制，直到某次
		// 非 bold 请求翻转标记、再来一次 bold 请求才纠正；那次翻转通常发生在
		// 首次点击触发的重排，表现为"点一下标题位移/变粗"的首帧不一致。
		f = &fontFace{handle: h, size: size, bold: false}
		if hasTTFHinting {
			// 档位见 fontHinting：默认 NORMAL（对齐 Chrome 灰度轮廓，矩阵实测
			// 见 .temp/gray-vs-chrome），GHROMEX_HINT 可切 light/mono/none 扫描
			// 对比。hinting 是 face 级状态，建 face 时设一次，SetFontSize/
			// SetFontStyle 后持续生效（TestSetFontSizePreservesHinting 回归锁定）。
			// 注意 LIGHT_SUBPIXEL 仅影响后续 LCD 渲染的子像素分解路径，非 LCD
			// 的 blended 调用在同一 face 上输出仍是灰度语义。
			ttfSetFontHinting(h, fontHinting())
		}
		g.faces[key] = f
	}
	if f.size != size {
		ttfSetFontSize(f.handle, uintptr(size))
		f.size = size
	}
	if f.bold != bold {
		style := uintptr(0)
		if bold {
			style = ttfStyleBold
		}
		ttfSetFontStyle(f.handle, style)
		f.bold = bold
	}
	return f
}

// segFontPath 返回一个脚本段应使用的字体文件。
func (g *Graphics) segFontPath(seg scriptSeg, family string) string {
	if seg.cjk {
		return g.cjkFont
	}
	return g.resolveFontPath(family)
}

// fontPath 兼容旧调用：不带 family 信息时的 standard 解析。
func (g *Graphics) fontPath(family string) string { return g.resolveFontPath(family) }

// MeasureText 实现 engine.Graphics。与 DrawText 同一套脚本分段与字体解析，
// 宽度为各段之和，行高取各段最大值（近似 Chrome 的行盒语义）。
func (g *Graphics) MeasureText(text string, fontSize int, bold bool, family string) (int, int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	if !hasTTF() {
		return estimateText(text, fontSize)
	}
	totalW, maxH := 0, 0
	for _, seg := range splitScripts(text) {
		path := g.segFontPath(seg, family)
		f := g.face(path, fontSize, bold)
		if f == nil {
			w, h := estimateText(seg.text, fontSize)
			totalW += w
			if h > maxH {
				maxH = h
			}
			continue
		}
		var w, h int32
		b, p := cBytes(seg.text)
		ttfSizeUTF8(f.handle, p, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)))
		runtime.KeepAlive(b)
		totalW += int(w)
		if int(h) > maxH {
			maxH = int(h)
		}
	}
	return totalW, maxH
}

// DrawText 实现 engine.Graphics：按脚本段绘制纹理，x 依次推进各段测量宽度，
// 各段以基线（TTF_FontAscent）对齐，与 Chrome 混排效果一致。
func (g *Graphics) DrawText(x, y, w, h int, paint engine.Paint, text string) {
	if text == "" || !hasTTF() {
		return
	}
	size := 16
	if s := paint.Size(); s != nil && s.Pixel() > 0 {
		size = s.Pixel()
	}
	var cr, cg, cb, ca uint8 = 0, 0, 0, 255
	if c := paint.Color(); c != nil {
		cr, cg, cb, ca = c.RGBA()
	}
	family := paint.FontFamily()
	bold := paint.Bold()

	segs := splitScripts(text)
	// 第一段（standard 段）的基线作为对齐参考；取各段中最大的 ascent
	refAscent := int32(-1)
	for _, seg := range segs {
		path := g.segFontPath(seg, family)
		if a := g.ascentOf(path, size, bold); a > refAscent {
			refAscent = a
		}
	}
	if refAscent < 0 {
		return
	}

	cx := x
	// LCD 子像素渲染需要落点背景色合成：paint.Background() 为沿父链最近
	// 实底（renderNode 组装）；nil 时（理论上仅测试路径）按白底处理。
	var br, bg, bb uint8 = 255, 255, 255
	if bc := paint.Background(); bc != nil {
		br, bg, bb, _ = bc.RGBA()
	}
	lcd := lcdText()
	kc := textContrast()
	for _, seg := range segs {
		path := g.segFontPath(seg, family)
		key := texKey{fontPath: path, text: seg.text, size: size, bold: bold, lcd: lcd,
			contrast: uint16(kc * 100),
			r:        cr, g: cg, b: cb, a: ca, br: br, bg: bg, bb: bb}
		info, ok := g.textures[key]
		if !ok {
			f := g.face(path, size, bold)
			if f == nil {
				continue
			}
			asc := int32(0)
			if ttfFontAscent != nil {
				asc = int32(ttfFontAscent(f.handle))
			}
			b, p := cBytes(seg.text)
			var tex uintptr
			opaque := false
			if lcd {
				// LCD 子像素：前景色与落点背景色合成出实色 surface
				//（TTF_RenderUTF8_LCD 的三通道即子像素分解），纹理按不透明
				// 直贴（BLENDMODE_NONE）。彩底上竖笔边缘会有轻微彩边
				//（浏览器子像素渲染同性质，README 已注明）。
				fgc := uintptr(cr) | uintptr(cg)<<8 | uintptr(cb)<<16 | uintptr(ca)<<24
				bgc := uintptr(br) | uintptr(bg)<<8 | uintptr(bb)<<16 | uintptr(0xFF)<<24
				surf := ttfRenderUTF8LCD(f.handle, p, fgc, bgc)
				runtime.KeepAlive(b)
				if surf != 0 {
					tex = sdlCreateTextureFromSurface(g.renderer, surf)
					sdlFreeSurface(surf)
					opaque = true
				}
			}
			if tex == 0 {
				// 默认路径：灰度 AA blended（BLENDMODE_BLEND）
				//（GHROMEX_LCD 未开、DLL 无 LCD 符号或 LCD 渲染失败时）
				color := uintptr(cr) | uintptr(cg)<<8 | uintptr(cb)<<16 | uintptr(ca)<<24
				surf := ttfRenderUTF8Blended(f.handle, p, color)
				runtime.KeepAlive(b)
				if surf == 0 {
					continue
				}
				// GHROMEX_CONTRAST>1 时对覆盖通道施加 tone 曲线（直通 alpha
				// 只需改覆盖字节），贴图后即为对比度调整过的字形。
				applyTextContrast(surf, kc)
				tex = sdlCreateTextureFromSurface(g.renderer, surf)
				sdlFreeSurface(surf)
			}
			if tex == 0 {
				continue
			}
			var tw, th int32
			sdlQueryTexture(tex, 0, 0,
				uintptr(unsafe.Pointer(&tw)), uintptr(unsafe.Pointer(&th)))
			mode := blendModeBlend
			if opaque {
				mode = blendModeNone
			}
			sdlSetTextureBlendMode(tex, uintptr(mode))
			info = texInfo{tex: tex, w: int(tw), h: int(th), ascent: asc}
			g.textures[key] = info
		}
		if drawDebug {
			// 记录每段的解析路径与"实际渲染纹理尺寸"：若 SetFontSize 参数传递失效，
			// actual 会与 req 不符（字号被 XMM 残值之类的外部因素带偏），此处一眼可见。
			fmt.Fprintf(os.Stderr, "[font] %q req=%dpx bold=%v -> %s actual=%dx%d ascent=%d\n",
				seg.text, size, bold, filepath.Base(path), info.w, info.h, info.ascent)
		}
		top := y
		if info.ascent > 0 && refAscent > 0 {
			top = y + int(refAscent-info.ascent)
		}
		dst := sdlRect{int32(cx), int32(top), int32(info.w), int32(info.h)}
		checkDraw("SDL_RenderCopy", sdlRenderCopy(g.renderer, info.tex, 0, uintptr(unsafe.Pointer(&dst))))
		cx += info.w
	}
}

// ascentOf 读取指定字体当前样式的基线（调用后 face 的 size/bold 已就位）。
func (g *Graphics) ascentOf(path string, size int, bold bool) int32 {
	f := g.face(path, size, bold)
	if f == nil || ttfFontAscent == nil {
		return 0
	}
	return int32(ttfFontAscent(f.handle))
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

// Clear 清屏。同时把裁剪栈清空并关闭 SDL 裁剪（SDL_RenderClear 本身无视
// 裁剪区，这里仅为让 Go 侧栈与 SDL 实际状态保持一致，防上一帧配对失衡）。
func (g *Graphics) Clear(r, gr, b, a uint8) {
	g.clips = g.clips[:0]
	checkDraw("SDL_RenderSetClipRect", sdlRenderSetClipRect(g.renderer, 0))
	checkDraw("SDL_SetRenderDrawColor", sdlSetRenderDrawColor(g.renderer, uintptr(r), uintptr(gr), uintptr(b), uintptr(a)))
	checkDraw("SDL_RenderClear", sdlRenderClear(g.renderer))
}

// PushClip 实现 engine.Clipper：与当前裁剪区求交后下推并应用。
// 裁剪区由引擎在 padding 盒/滚动容器上给出，坐标即渲染坐标。
func (g *Graphics) PushClip(x, y, w, h int) {
	r := sdlRect{int32(x), int32(y), int32(w), int32(h)}
	if n := len(g.clips); n > 0 {
		r = intersectSdlRect(g.clips[n-1], r)
	}
	g.clips = append(g.clips, r)
	g.applyClip()
}

// PopClip 实现 engine.Clipper：弹出栈顶并应用剩余栈顶（空栈=关闭裁剪）。
func (g *Graphics) PopClip() {
	if n := len(g.clips); n > 0 {
		g.clips = g.clips[:n-1]
	}
	g.applyClip()
}

// applyClip 把栈顶同步给 SDL；栈空传 NULL 关闭裁剪。
func (g *Graphics) applyClip() {
	if n := len(g.clips); n > 0 {
		r := g.clips[n-1]
		checkDraw("SDL_RenderSetClipRect", sdlRenderSetClipRect(g.renderer, uintptr(unsafe.Pointer(&r))))
		return
	}
	checkDraw("SDL_RenderSetClipRect", sdlRenderSetClipRect(g.renderer, 0))
}

// intersectSdlRect 求两矩形交集；不相交返回位于左上、宽高为 0 的空矩形
// （SDL 按空裁剪区处理 = 什么都不画）。
func intersectSdlRect(a, b sdlRect) sdlRect {
	x0, y0 := a.x, a.y
	if b.x > x0 {
		x0 = b.x
	}
	if b.y > y0 {
		y0 = b.y
	}
	x1, y1 := a.x+a.w, a.y+a.h
	if b.x+b.w < x1 {
		x1 = b.x + b.w
	}
	if b.y+b.h < y1 {
		y1 = b.y + b.h
	}
	if x1-x0 <= 0 || y1-y0 <= 0 {
		return sdlRect{x0, y0, 0, 0}
	}
	return sdlRect{x0, y0, x1 - x0, y1 - y0}
}

// Present 提交帧。
func (g *Graphics) Present() {
	sdlRenderPresent(g.renderer)
}

// CloseFonts 释放全部 face（由 Window.Close 调用）。
func (g *Graphics) CloseFonts() {
	for _, f := range g.faces {
		if f.handle != 0 {
			ttfCloseFont(f.handle)
			f.handle = 0
		}
	}
	g.faces = make(map[faceKey]*fontFace)
	for _, t := range g.textures {
		if t.tex != 0 {
			sdlDestroyTexture(t.tex)
		}
	}
	g.textures = make(map[texKey]texInfo)
}

// hasTTF 判断 TTF 符号是否已绑定（Load 之后为 true）。
func hasTTF() bool { return ttfOpenFont != nil }

// estimateText 是字体缺失/未加载时的粗略估算：CJK 按全宽、拉丁按 0.6 宽。
func estimateText(text string, fontSize int) (int, int) {
	w := 0
	for _, r := range text {
		if isScriptCJK(r) {
			w += fontSize
		} else if r == ' ' {
			w += fontSize / 2
		} else {
			w += fontSize * 3 / 5
		}
	}
	return w, fontSize * 13 / 10
}

var _ engine.Graphics = (*Graphics)(nil)
