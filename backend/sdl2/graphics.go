//go:build windows

package sdl2

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
//   serif → Times New Roman；sans-serif → Arial；monospace/cursive → 宋体
//   fantasy → Impact；system-ui → Segoe UI
//   ！通用族的实际取值以 Chrome 实测为准：Chrome 在中文 Windows 上
//   monospace/cursive 用的是新宋体（默认等宽字体），不是 Courier New /
//   Comic Sans MS；拉丁字符在新宋体里是半角定宽，实测宽度与行高都与
//   Chrome 逐像素一致。
//   汉字/假名/谚文等 CJK 字符按 per-script fallback：sans-serif → Noto Sans SC、
//   serif → Noto Serif SC、monospace/cursive → 宋体、system-ui → 微软雅黑。
// ---------------------------------------------------------------------------

const fontsDir = `C:\Windows\Fonts`

// genericFamilyFiles CSS 通用族 → 字体文件名。
// 取值来自 Chrome（中文 Windows）实测的通用族回退：monospace/cursive 都落到
// 新宋体（16px 每字符 8px、行高 18；Chrome 的默认等宽字体就是新宋体，
// 而非 Courier New），其余见下方注释。
var genericFamilyFiles = map[string]string{
	"serif":         "times.ttf",
	"sans-serif":    "arial.ttf",
	"monospace":     "simsun.ttc",
	"cursive":       "simsun.ttc",
	"fantasy":       "impact.ttf",
	"system-ui":     "segoeui.ttf",
	"ui-serif":      "times.ttf",
	"ui-sans-serif": "segoeui.ttf",
	"ui-monospace":  "simsun.ttc",
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

// fontPathCache 缓存 fontPathOf 的结果（"" = 不存在）。系统字体在进程生命周期
// 内不会变化，而字体解析在**每次排版与绘制**都会被调用（每段文本、每帧）：
// 不缓存时一次重排要做上万次 os.Stat（Windows 上是 GetFileAttributesEx 系统
// 调用），实测 examples/ua/index.html 单次重排 75ms、其中约一半耗在这里；滚动动画
// 每帧都要重排，于是滚轮手感变成"慢吞吞"。缓存后同一查询是纯内存 map 命中。
var fontPathCache sync.Map // map[string]string

// fontPathOf 把文件名或绝对路径规范为存在的完整路径；不存在返回 ""。
func fontPathOf(name string) string {
	if name == "" {
		return ""
	}
	if v, ok := fontPathCache.Load(name); ok {
		return v.(string)
	}
	p := ""
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			p = name
		}
	} else if cand := filepath.Join(fontsDir, name); fileExists(cand) {
		p = cand
	}
	fontPathCache.Store(name, p)
	return p
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
	measures map[measureKey]measureVal
	// familyPaths 缓存 font-family 列表 → 字体文件的解析结果。解析本身要拆分
	// 列表、查表、stat 多个候选，而同一份 CSS 的 family 字符串在每帧重排/重绘
	// 中反复出现；结果只依赖本 Graphics 的 standardFont，故按实例缓存。
	familyPaths map[string]string
	// texOrder 记录纹理插入顺序（FIFO 淘汰用）；缓存无上限会让动态文本
	// 页面（计数/时间戳）的显存单调增长，长跑必然耗尽。
	texOrder []texKey

	// env* 是文本渲染相关的环境变量取值，每帧刷新一次（见 refreshEnv）。
	envLCD      bool
	envHint     uintptr
	envContrast float64

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
	// hint = hinting 档位（fontHinting()）：hinting 是 face 级状态，档位进了
	// key 才能在运行中切 GHROMEX_HINT 时重建 face（否则改了环境变量无效果，
	// 与 GHROMEX_CONTRAST 可运行中切换的语义不一致）。
	hint uint8
}

type fontFace struct {
	handle uintptr
	path   string // 实际打开的字体文件（粗体可能是真粗体变体）
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
// 默认 normal 的依据（README「渲染段」的矩阵实测 vs Chrome 灰度基线）：
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
	// hint = hinting 档位（fontHinting()）：同 faceKey 的理由——运行中切
	// GHROMEX_HINT 后必须重建纹理，否则仍命中旧 hinting 栅格化的缓存。
	hint uint8
	// contrast = textContrast()×100（灰度路径的 tone 曲线档位）：随环境变量
	// 变化重建纹理，避免切换后仍命中旧缓存。LCD 路径不施加曲线（恒记 0）。
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
	g := &Graphics{
		renderer:     renderer,
		standardFont: std,
		cjkFont:      CJKFontPath(),
		faces:        make(map[faceKey]*fontFace),
		textures:     make(map[texKey]texInfo),
		measures:     make(map[measureKey]measureVal),
		familyPaths:  make(map[string]string),
		reuseFaces:   hasTTFSetFontSize,
	}
	g.refreshEnv()
	return g
}

// refreshEnv 刷新文本渲染相关的环境变量取值。
// DrawText 每段文本都要读这些值，若每次都 os.Getenv 会按帧产生一批字符串
// 分配（Windows 上每次 Getenv 都新分配）；Clear 是帧起点，在那里刷新一次
// 即可——运行中切换环境变量仍然下一帧生效。
func (g *Graphics) refreshEnv() {
	g.envLCD = lcdText()
	g.envHint = fontHinting()
	g.envContrast = textContrast()
}

// resolveFontPath 把 CSS font-family（可含逗号分隔列表）解析为字体文件路径。
// 逐个候选尝试：通用族 → 具名别名 → 直接文件名/绝对路径 → 家族名+常见扩展。
// 结果按 family 缓存（见 familyPaths）：字体不能在运行中新增，重复解析没有意义。
func (g *Graphics) resolveFontPath(family string) string {
	if p, ok := g.familyPaths[family]; ok {
		return p
	}
	p := g.resolveFontPathUncached(family)
	g.familyPaths[family] = p
	return p
}

func (g *Graphics) resolveFontPathUncached(family string) string {
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

// boldVariants 给出各字体文件对应的"真粗体"文件。浏览器按字体族挑选粗体
// 字重（微软雅黑 → msyhbd.ttc、Arial → arialbd.ttf、Segoe UI →
// segoeuib.ttf），而不是对常规字形做合成加粗。FreeType 的合成加粗
// （FT_Outline_Embolden）明显比真粗体更重，24px 中文标题上还会出现笔画
// 粘连/重影，与浏览器观感差别很大（实测 h2"用户注册"两字）。
// 没有真粗体文件的字体（如 simhei.ttf）仍走合成加粗——与浏览器一致。
var boldVariants = map[string]string{
	"arial.ttf":   "arialbd.ttf",
	"times.ttf":   "timesbd.ttf",
	"cour.ttf":    "courbd.ttf",
	"segoeui.ttf": "segoeuib.ttf",
	"msyh.ttc":    "msyhbd.ttc",
	"simsun.ttc":  "simsunb.ttf",
}

// facePath 返回该 (路径, 粗细) 实际会打开的字体文件路径（粗体可能命中真粗体
// 变体）。仅用于诊断输出。
func (g *Graphics) facePath(path string, size int, bold bool) string {
	if bold {
		if name, ok := boldVariants[strings.ToLower(filepath.Base(path))]; ok {
			if p := fontPathOf(name); p != "" {
				return p
			}
		}
	}
	return path
}

// face 取得（并按需创建/切换）字体句柄。reuseFaces 时一个 (文件, hinting
// 档位) 一个 face，用 TTF_SetFontSize / TTF_SetFontStyle 动态切换，避免同一
// TTC 按字号重复加载；hinting 档位进 key 是为支持运行中切 GHROMEX_HINT。
func (g *Graphics) face(path string, size int, bold bool) *fontFace {
	// 粗体优先用真粗体字文件（见 boldVariants）：合成加粗比真粗体重得多，
	// 中文尤甚。命中时该 face 本身就是粗体，不再叠加 TTF_STYLE_BOLD。
	realBold := false
	if bold {
		if name, ok := boldVariants[strings.ToLower(filepath.Base(path))]; ok {
			if p := fontPathOf(name); p != "" {
				path = p
				realBold = true
			}
		}
	}
	key := faceKey{path: path, hint: uint8(g.envHint)}
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
		// 常见，如 ua 示例的 h2）handle 一直按 regular 度量与绘制，直到某次
		// 非 bold 请求翻转标记、再来一次 bold 请求才纠正；那次翻转通常发生在
		// 首次点击触发的重排，表现为"点一下标题位移/变粗"的首帧不一致。
		// 真粗体字文件本身就是粗体：直接记 true，后续不再切样式。
		f = &fontFace{handle: h, path: path, size: size, bold: realBold}
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
		return g.cjkFontPathFor(family)
	}
	return g.resolveFontPath(family)
}

// cjkCapableFamilies 是自带 CJK 字形的字体族（族名关键字，小写比较）。
// 浏览器在字体含目标字形时直接用该字体、不触发逐字回退；引擎同理：显式
// 指定 SimHei/SimSun/雅黑等中文字体时，CJK 字符应按该字体度量与绘制，
// 否则同一行的行高会按引擎的回退字体（雅黑）算，与浏览器差 3~4px。
var cjkCapableFamilies = map[string]bool{
	"microsoft yahei": true, "微软雅黑": true,
	"simhei": true, "黑体": true,
	"simsun": true, "宋体": true, "nsimsun": true, "新宋体": true,
	"fangsong": true, "仿宋": true, "kaiti": true, "楷体": true,
	"msjh": true, "微軟正黑體": true, "mingliu": true, "細明體": true,
	"dengxian": true, "等线": true,
	"noto sans cjk": true, "noto serif cjk": true,
	"source han sans": true, "source han serif": true,
}

// cjkFontPathFor 返回该 font-family 列表中自带 CJK 字形的字体文件；
// 列表中没有中文字体时返回引擎的 CJK 回退字体（雅黑）。
func (g *Graphics) cjkFontPathFor(family string) string {
	for _, raw := range strings.Split(family, ",") {
		name := strings.ToLower(strings.Trim(strings.TrimSpace(raw), `"'`))
		if name == "" {
			continue
		}
		if !cjkCapableFamilies[name] {
			continue
		}
		if p := g.resolveFontPath(name); p != "" {
			return p
		}
	}
	// 列表里没有中文字体：按 generic family 选 Chrome 对应的系统回退字体
	// （Chrome 实测：sans-serif→Noto Sans SC、serif→Noto Serif SC、
	// monospace/cursive→宋体、system-ui→雅黑——同一个汉字在不同 generic
	// 下行高不同，正是回退字体不同的结果）。
	return g.cjkFallbackPath(cjkFallbackClass(family))
}

// cjkFallbackClass 把 font-family 列表归类到 Chrome 的 CJK 回退档位。
// 注意 "sans-serif" 里含 "serif"：必须先判 sans-serif。
func cjkFallbackClass(family string) string {
	lower := strings.ToLower(family)
	has := func(k string) bool { return strings.Contains(lower, k) }
	switch {
	case has("monospace"), has("courier new"), has("cursive"), has("comic sans"):
		return "mono"
	case has("system-ui"):
		return "ui"
	case has("sans-serif"), has("arial"), has("segoe"), has("verdana"), has("tahoma"):
		return "sans"
	case has("serif"), has("times"):
		return "serif"
	default:
		return "sans"
	}
}

var cjkFallbackPaths struct {
	once              sync.Once
	mono, serif, sans string
}

// cjkFallbackPath 返回该档位的回退字体文件；未安装时退回 g.cjkFont。
// 结果缓存：segFontPath 在每次度量/绘制时都会调用，不能反复 os.Stat。
func (g *Graphics) cjkFallbackPath(class string) string {
	cjkFallbackPaths.once.Do(func() {
		first := func(names ...string) string {
			for _, n := range names {
				if p := fontPathOf(n); p != "" {
					return p
				}
			}
			return ""
		}
		cjkFallbackPaths.mono = first("simsun.ttc")
		cjkFallbackPaths.serif = first("NotoSerifSC-VF.ttf", "NotoSerifSC-Regular.otf")
		cjkFallbackPaths.sans = first("NotoSansSC-VF.ttf", "NotoSansSC-Regular.otf")
	})
	p := ""
	switch class {
	case "mono":
		p = cjkFallbackPaths.mono
	case "serif":
		p = cjkFallbackPaths.serif
	case "sans":
		p = cjkFallbackPaths.sans
	}
	if p == "" {
		return g.cjkFont
	}
	return p
}

// fontPath 兼容旧调用：不带 family 信息时的 standard 解析。
func (g *Graphics) fontPath(family string) string { return g.resolveFontPath(family) }

// measureKey/measureVal 是 MeasureText 结果的缓存键值。文本度量在每帧重排中
// 被反复请求（同一个词、同一段文本、同一份 CSS），而结果只取决于
// （文本, 字号, 粗细, family, hinting 档位, 是否有 TTF）——字体文件在进程内
// 不变。不缓存时每次都要走 SDL_ttf 的 TTF_SizeUTF8（CGO 调用），实测占
// examples/ua/index.html 单次重排 80% 以上，是滚动掉帧的最后一处大头。
// ttf 进 key：DLL 是否可用会改变度量口径（估算 vs 真实），不能串用。
type measureKey struct {
	text   string
	size   int
	bold   bool
	family string
	hint   uint8
	ttf    bool
}
type measureVal struct{ w, h int }

// maxCachedMeasures 是文本度量缓存上限。动态文本页面（计数/时间戳）的
// 唯一字符串无上限，超出后整体清空——度量重算比无限增长安全。
const maxCachedMeasures = 8192

// MeasureText 实现 engine.Graphics。与 DrawText 同一套脚本分段与字体解析，
// 宽度为各段之和，行高取各段最大值（近似 Chrome 的行盒语义）。
func (g *Graphics) MeasureText(text string, fontSize int, bold bool, family string) (int, int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	key := measureKey{text: text, size: fontSize, bold: bold, family: family,
		hint: uint8(g.envHint), ttf: hasTTF()}
	if v, ok := g.measures[key]; ok {
		return v.w, v.h
	}
	totalW, maxH := 0, 0
	cacheable := true
	if !key.ttf {
		totalW, maxH = estimateText(text, fontSize)
	} else {
		for _, seg := range splitScripts(text) {
			path := g.segFontPath(seg, family)
			f := g.face(path, fontSize, bold)
			if f == nil {
				// 字体句柄打开失败是暂时性的（文件被占用等）：本次按估算
				// 返回但不入缓存，后续帧还有机会拿到真实度量。
				cacheable = false
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
	}
	if len(g.measures) >= maxCachedMeasures {
		// 清空而非淘汰最旧：度量重算代价远低于维护 LRU 的复杂度，
		// 且正常页面（静态文本）不会触顶。
		g.measures = make(map[measureKey]measureVal)
	}
	if cacheable {
		g.measures[key] = measureVal{w: totalW, h: maxH}
	}
	return totalW, maxH
}

// FontMetrics 实现 engine.Graphics：返回行度量（基线以上 ascent、基线以下
// descent）。各脚本段取最大值：行盒高度 = 各内联盒 ascent 的最大者 +
// descent 的最大者（同基线对齐）。度量来自 SDL_ttf 的 TTF_FontAscent/
// TTF_FontDescent，行高与 Chrome/DirectWrite 可能有 1~4px 差异。
// text 为空时只取主字体（不触发 CJK 逐字回退），对应行盒的 strut。
func (g *Graphics) FontMetrics(text string, fontSize int, bold bool, family string) (int, int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	if !hasTTF() {
		h := fontSize * 13 / 10
		return h / 2, h - h/2
	}
	if text == "" {
		asc, desc, ok := g.lineMetricsOf(g.resolveFontPath(family), fontSize, bold)
		if !ok {
			h := fontSize * 13 / 10
			return h / 2, h - h/2
		}
		return asc, desc
	}
	maxAsc, maxDesc := 0, 0
	for _, seg := range splitScripts(text) {
		asc, desc, ok := g.lineMetricsOf(g.segFontPath(seg, family), fontSize, bold)
		if !ok {
			continue
		}
		if asc > maxAsc {
			maxAsc = asc
		}
		if desc > maxDesc {
			maxDesc = desc
		}
	}
	if maxAsc == 0 && maxDesc == 0 {
		h := fontSize * 13 / 10
		return h / 2, h - h/2
	}
	return maxAsc, maxDesc
}

// lineMetricsOf 返回（字体文件, 字号, 粗细）下的行度量：ascent 取
// TTF_FontAscent，descent 取 -TTF_FontDescent（SDL_ttf 返回负值）。
//
// 这里直接用 SDL_ttf 的 FreeType 像素取整，不再解析字体的 head/hhea/OS2。
// 代价是行高不再逐像素对齐 Chrome/DirectWrite（同一字体可能差 1~4px），
// 换来的是实现简单、无字体表解析代码；布局与绘制仍使用同一个 face，
// ascent/descent 与真实字形基线至少自洽。
func (g *Graphics) lineMetricsOf(path string, size int, bold bool) (int, int, bool) {
	if !hasTTFFontAscent || !hasTTFFontDescent {
		return 0, 0, false
	}
	f := g.face(path, size, bold)
	if f == nil {
		return 0, 0, false
	}
	asc := int(ttfFontAscent(f.handle))
	desc := -int(ttfFontDescent(f.handle))
	if asc < 0 {
		asc = 0
	}
	if desc < 0 {
		desc = 0
	}
	return asc, desc, true
}

// DrawText 实现 engine.Graphics：按脚本段绘制纹理，x 依次推进各段测量宽度，
// 各段以基线（TTF_FontAscent）对齐。
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
	lcd := g.envLCD
	kc := g.envContrast
	if lcd {
		kc = 0 // LCD 纹理在 DLL 内已合成，tone 曲线不适用（也不该因它重建缓存）
	}
	hint := uint8(g.envHint)
	for _, seg := range segs {
		path := g.segFontPath(seg, family)
		key := texKey{fontPath: path, text: seg.text, size: size, bold: bold, lcd: lcd,
			hint:     hint,
			contrast: uint16(kc * 100),
			r:        cr, g: cg, b: cb, a: ca}
		if lcd {
			// 落点背景色只有 LCD 路径用得到（灰度纹理与底色无关）：不写进
			// key，否则同一段文字落在 N 种底色上会建 N 张完全相同的纹理。
			key.br, key.bg, key.bb = br, bg, bb
		}
		info, ok := g.textures[key]
		if !ok {
			f := g.face(path, size, bold)
			if f == nil {
				continue
			}
			asc := int32(0)
			if hasTTFFontAscent {
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
			g.texOrder = append(g.texOrder, key)
			g.evictTexturesIfNeeded()
		}
		if drawDebug {
			// 记录每段的解析路径与"实际渲染纹理尺寸"：若 SetFontSize 参数传递失效，
			// actual 会与 req 不符（字号被 XMM 残值之类的外部因素带偏），此处一眼可见。
			// 路径取 face 实际打开的文件（粗体可能命中真粗体变体 msyhbd.ttc 等）。
			fmt.Fprintf(os.Stderr, "[font] %q req=%dpx bold=%v -> %s actual=%dx%d ascent=%d\n",
				seg.text, size, bold, filepath.Base(g.facePath(path, size, bold)), info.w, info.h, info.ascent)
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

// ascentOf 读取指定字体当前样式的基线（TTF_FontAscent）。
// 与 lineMetricsOf 同源，保证绘制基线与布局行盒使用同一个 face 的度量。
func (g *Graphics) ascentOf(path string, size int, bold bool) int32 {
	asc, _, ok := g.lineMetricsOf(path, size, bold)
	if !ok {
		return 0
	}
	return int32(asc)
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
// 也是每帧的起点：在这里刷新文本渲染的环境变量取值（见 refreshEnv）。
func (g *Graphics) Clear(r, gr, b, a uint8) {
	g.refreshEnv()
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
	g.texOrder = nil
}

// maxCachedTextures 是文本纹理缓存上限。缓存此前只增不减：每个新的
// (文本,字号,粗体,颜色) 组合都新建一张 SDL_Texture，计数/时间戳这类
// 动态文本的页面显存单调增长，长跑必然耗尽。超出后按插入顺序淘汰最旧的。
const maxCachedTextures = 512

// evictTexturesIfNeeded 超出缓存上限时按插入顺序（FIFO）淘汰最旧的纹理。
func (g *Graphics) evictTexturesIfNeeded() {
	for len(g.texOrder) > maxCachedTextures {
		old := g.texOrder[0]
		// 头移出后拷贝剩余部分（回收被移出的数组槽位，避免底层数组无限增长）
		g.texOrder = append(g.texOrder[:0], g.texOrder[1:]...)
		if info, ok := g.textures[old]; ok {
			if info.tex != 0 {
				sdlDestroyTexture(info.tex)
			}
			delete(g.textures, old)
		}
	}
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
