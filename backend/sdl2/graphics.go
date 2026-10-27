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

	"github.com/go-comm/ghromex/core"
)

// drawDebug 为 true 时（环境变量 GHROMEX_DEBUG 非空），绘制调用失败会打印 SDL 错误。
var drawDebug = envBool("GHROMEX_DEBUG")

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
	} else if b := bundledFontsDir(); b != "" {
		// 系统字体没有的（如 tools/gen-cjk-instances.ps1 生成的静态字重面）
		// 再查仓库内补充字体目录 libs/fonts（.gitignore 排除、按需生成）。
		if cand := filepath.Join(b, name); fileExists(cand) {
			p = cand
		}
	}
	fontPathCache.Store(name, p)
	return p
}

var bundledFontsOnce sync.Once
var bundledFontsVal string

// bundledFontsDir 返回仓库内补充字体目录 libs/fonts（tools/gen-cjk-instances.ps1
// 生成的静态字重面等）。查找口径同 findLibsDir：先 cwd 的 .、..、../..
// 三级（测试的 cwd 是包目录），再试可执行文件旁（打包运行时随包分发）。
// 进程内只找一次；不存在返回 ""，fontPathOf 直接按未命中处理。
func bundledFontsDir() string {
	bundledFontsOnce.Do(func() {
		try := func(p string) bool {
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				bundledFontsVal = p
				return true
			}
			return false
		}
		for _, up := range []string{".", "..", filepath.Join("..", "..")} {
			if try(filepath.Join(up, "libs", "fonts")) {
				return
			}
		}
		if exe, err := os.Executable(); err == nil && exe != "" {
			try(filepath.Join(filepath.Dir(exe), "libs", "fonts"))
		}
	})
	return bundledFontsVal
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

// Graphics 基于 SDL2 Renderer + SDL_ttf 实现 core.Graphics。
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
	// texClock 是纹理使用的单调时钟（LRU 淘汰用），texPixels 是当前缓存的
	// 纹理像素总数（显存占用估算）。缓存无上限会让动态文本页面
	// （计数/时间戳）的显存单调增长，长跑必然耗尽，故按"条数 + 像素"双上限
	// 淘汰最近最少使用的纹理。
	texClock  uint64
	texPixels int
	// sprites 是位图精灵的纹理缓存（键是引擎侧缓存的精灵指针）。精灵
	// 像素只随「半径+颜色」变，引擎侧已缓存，这里按指针复用纹理即可。
	//
	// 精灵纹理同样要有上限：键里含颜色，页面逐帧改色（动画）会不断产生
	// 新精灵，不淘汰就是显存泄漏。按「条数 + 像素」双上限淘汰最近最少使用
	// 的纹理（与文本纹理缓存同策略）。
	sprites      map[*core.Sprite]*spriteTex
	spritePixels int // 当前精灵纹理占用的像素总数
	// spriteUploads 统计纹理上传次数（测试用）：精灵像素不变，纹理只该
	// 上传一次——每次绘制都上传会把帧预算吃光（实测 300 次/帧 ≈ 2ms）。
	spriteUploads int
	// cursors 缓存已创建的系统光标（SDL_SystemCursor 枚举值 → SDL_Cursor*）。
	// 按实例缓存：SDL_Quit 后句柄失效，跨实例复用会拿到野指针。
	cursors map[uintptr]uintptr

	// env* 是文本渲染相关的环境变量取值，每帧刷新一次（见 refreshEnv）。
	envLCD      bool
	envHint     uintptr
	envContrast float64

	// reuseFaces 为 false 时（旧版 SDL2_ttf 无 TTF_SetFontSize），
	// faceKey 带上 size/bold，行为退化为每组合一个 face（与历史版本一致）。
	reuseFaces bool

	// clips 是裁剪矩形栈（与 core.Clipper 的 Push/Pop 严格配对），栈顶
	// 即当前生效裁剪区；空栈 = 无裁剪。与 SDL viewport 同坐标系。
	clips []sdlRect
}

// 编译期断言：SDL 后端必须实现 core.Clipper，否则 overflow 裁剪会静默失效。
var _ core.Clipper = (*Graphics)(nil)
var _ core.WeightedMeasurer = (*Graphics)(nil)

type faceKey struct {
	path string
	size int
	bold bool
	// italic 只在 !reuseFaces（旧版 SDL_ttf 无 TTF_SetFontSize）时进 key：
	// 复用路径下一个 face 用 TTF_SetFontStyle 动态切粗体/斜体，样式不进 key。
	italic bool
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
	// italic 记 handle 当前的合成斜体位（TTF_STYLE_ITALIC），切换时才调
	// TTF_SetFontStyle（与 bold 同一套「记实际样式位」的逻辑）。
	italic bool
	// realBold 表示 handle 打开的是真粗体字文件：此时不应再叠加
	// TTF_STYLE_BOLD（会糊成双倍粗），但斜体仍要能合成。
	realBold bool
}

// lcdText 报告 LCD 子像素渲染是否启用（TTF_RenderUTF8_LCD + LIGHT_SUBPIXEL
// hinting）。默认关闭：文本默认灰度 AA（hinting 档位见 fontHinting，默认
// NORMAL；GHROMEX_HINT 可切 light|mono|none），无彩边、对落点背景色不敏感。
// GHROMEX_LCD=1 开启：竖笔画边缘横向锐度与浏览器 ClearType
// 同级，代价是彩底上竖笔边缘轻微彩边（与浏览器子像素渲染同性质），且需按
// 落点背景色合成（见 DrawText）。
// 惰性函数而非包级变量：hasTTFLCD 在 Load()（DLL 符号解析）后才有值，
// 包初始化时求值恒为 false。
// LCDTextEnabled 报告当前是否走 LCD（子像素）文本路径：既要环境变量
// GHROMEX_LCD=1 打开，也要 DLL 真的导出 TTF_RenderUTF8_LCD。
//
// 供上层（browser 层）判断「这条渲染路径在本机可用吗」——LCD 相关的
// 断言要用真实字形贴图，路径不可用时只能跳过。
func LCDTextEnabled() bool { return lcdText() }

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
	// tier = 字面档位（见 fontTierOf）。不能用 bold 代替：fontPath 是变体
	// 解析**前**的段路径（如 segoeui.ttf），600（→seguisb）与 700
	// （→segoeuib）若同 key 会串用彼此的纹理。
	tier   uint8
	italic bool
	lcd    bool
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
	// lastUsed 是最近一次命中时的时钟值（LRU 淘汰用）。FIFO 淘汰会把
	// "被复用最多"的纹理（页面上反复出现的分隔符、价签等）先淘汰掉，
	// 工作集略大于缓存上限时每帧全部重渲染（缓存颠簸）。
	lastUsed uint64
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
		sprites:      make(map[*core.Sprite]*spriteTex),
		measures:     make(map[measureKey]measureVal),
		familyPaths:  make(map[string]string),
		cursors:      make(map[uintptr]uintptr),
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
	// Noto Sans/Serif SC 的 VF 版：fvar 默认面是 Thin(100)（Google 出厂如此），
	// SDL2_ttf 读不了 VF 实例，Chrome 靠 DirectWrite 命名实例拿 400/700 真字重。
	// 粗体档指向 tools/gen-cjk-instances.ps1 生成的静态 Bold 面；未生成时
	// fontPathOf 落空，退回合成加粗（旧行为）。正体档见 cjkFallbackPath。
	"notosanssc-vf.ttf":       "NotoSansSC-Bold.ttf",
	"notosanssc-regular.ttf":  "NotoSansSC-Bold.ttf",
	"notoserifsc-vf.ttf":      "NotoSerifSC-Bold.ttf",
	"notoserifsc-regular.ttf": "NotoSerifSC-Bold.ttf",
}

// semiboldVariants 给出各字体文件对应的**真半粗体**（Semibold，600 档）
// 文件。Chrome 就近选面实测（Segoe UI 族含 300/350/400/600/700/900 六面）：
// font-weight 550~649 落 600 面（"2026-10-23 AI" 32px 宽 202.7，而
// segoeuib Bold 为 214.6——两档归档的 600 会重 6% 且行宽偏大）。
// 没有半粗面的族（Arial/雅黑/宋体）不登记：未命中落 boldVariants 或合成
// 粗，与 Chrome 的就近选面（600→700 面）一致。
var semiboldVariants = map[string]string{
	"segoeui.ttf": "seguisb.ttf",
}

// 字面档位：由数值 font-weight 与两档布尔归档得出，face/度量/纹理三处
// 共用同一口径。
const (
	tierRegular  = iota // 0：正体
	tierSemibold        // 1：半粗（550~649，优先真 Semibold 面）
	tierBold            // 2：粗体（650+ 或 legacy bold）
)

// fontTierOf 把（数值字重, 两档粗体）归档到字面档位。weightNum<=0 表示
// 调用方未带数字（UA 直接 SetFontWeight 的 legacy 路径），按两档布尔；
// 带数字时以数字为准（与解析层 ≥550→Bold 的归档保证一致）。
func fontTierOf(bold bool, weightNum int) int {
	if weightNum > 0 {
		switch {
		case weightNum >= 650:
			return tierBold
		case weightNum >= 550:
			return tierSemibold
		default:
			return tierRegular
		}
	}
	if bold {
		return tierBold
	}
	return tierRegular
}

// facePath 返回该 (路径, 档位) 实际会打开的字体文件路径（半粗/粗体可能
// 命中真字重变体）。仅用于诊断输出。
func (g *Graphics) facePath(path string, size, tier int) string {
	base := strings.ToLower(filepath.Base(path))
	if tier >= tierSemibold {
		if tier == tierSemibold {
			if name, ok := semiboldVariants[base]; ok {
				if p := fontPathOf(name); p != "" {
					return p
				}
			}
		}
		// 半粗未命中（族无 Semibold 面）落粗体面——与 face 的变体解析
		// 同构（Chrome 对 {400,700} 族 600→700 就近选面）。
		if name, ok := boldVariants[base]; ok {
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
func (g *Graphics) face(path string, size, tier int, italic bool) *fontFace {
	// 粗字重优先用真字面文件（700 档见 boldVariants、600 档见
	// semiboldVariants）：合成加粗比真粗体重得多，中文尤甚。命中真字面时
	// 该 face 自身已带字重，不再叠加 TTF_STYLE_BOLD。半粗档未命中（族无
	// Semibold 面）落粗体面——与 Chrome 对 {400,700} 族 600→700 的就近
	// 选面同口径；两者都未命中才合成加粗（如 Noto VF，SDL2_ttf 无 VF API）。
	base := strings.ToLower(filepath.Base(path))
	realBold := false
	if tier >= tierSemibold {
		if tier == tierSemibold {
			if name, ok := semiboldVariants[base]; ok {
				if p := fontPathOf(name); p != "" {
					path = p
					realBold = true
				}
			}
		}
		if !realBold {
			if name, ok := boldVariants[base]; ok {
				if p := fontPathOf(name); p != "" {
					path = p
					realBold = true
				}
			}
		}
	}
	bold := tier >= tierSemibold
	key := faceKey{path: path, hint: uint8(g.envHint)}
	if !g.reuseFaces {
		key.size, key.bold, key.italic = size, bold, italic
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
		f = &fontFace{handle: h, path: path, size: size, bold: realBold, realBold: realBold}
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
	if f.bold != bold || f.italic != italic {
		style := uintptr(0)
		if bold && !f.realBold {
			style |= ttfStyleBold
		}
		if italic {
			style |= ttfStyleItalic
		}
		ttfSetFontStyle(f.handle, style)
		f.bold = bold
		f.italic = italic
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
		cjkFallbackPaths.serif = cjkNotoInstance("NotoSerifSC-VF.ttf", "NotoSerifSC-Regular.otf")
		cjkFallbackPaths.sans = cjkNotoInstance("NotoSansSC-VF.ttf", "NotoSansSC-Regular.otf")
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

// cjkNotoInstance 解析 Noto CJK 回退的**正体**字面。门控与选面顺序：
//
//  1. 系统里没有该族任何字面（VF 或静态 otf）→ 返回空：这台机器的 Chrome
//     会把 CJK 交给雅黑，引擎必须同步落到 g.cjkFont，否则两边字形错配。
//     libs/fonts 的静态面只是 VF 命名实例的替身，不参与门控判定。
//  2. 系统装了 VF（本机如此，Chrome 即用 Noto 的命名实例渲染 400/700）→
//     优先取同族静态 Regular 面（tools/gen-cjk-instances.ps1 生成到
//     libs/fonts）：该 VF 的 fvar 默认字重是 Thin(100)（Google 出厂如此），
//     SDL2_ttf 读不了 VF 实例，直接打开渲染出的正文会比 Chrome 细三档。
//  3. 静态面未生成 → 退回 VF 本身（旧行为，Thin 正文），粗体档仍由
//     boldVariants 接住（见同名 -Bold.ttf，未生成则落回合成加粗）。
//
// 粗体/半粗档的选面不在此处：face() 拿到本函数的返回值后按 tier 走
// boldVariants 就近选面（Chrome 对无 600 面的 VF 同样 600→700）。
func cjkNotoInstance(vfName, otfFallback string) string {
	if !systemFontExists(vfName) && !systemFontExists(otfFallback) {
		return ""
	}
	static := strings.Replace(vfName, "-VF.ttf", "-Regular.ttf", 1)
	for _, n := range []string{static, otfFallback, vfName} {
		if p := fontPathOf(n); p != "" {
			return p
		}
	}
	return ""
}

// systemFontExists 只看系统字体目录（不含 libs/fonts 捆绑面）：用于判定
// 本机 Chrome 会不会选中该族——捆绑面是 VF 实例的替身，不该反过来改变
// 族选择。
func systemFontExists(name string) bool {
	return fileExists(filepath.Join(fontsDir, name))
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
	text string
	size int
	// tier 语义同 texKey：600/700 度量命中不同字面，不能共键。
	tier   uint8
	family string
	hint   uint8
	ttf    bool
}
type measureVal struct{ w, h int }

// maxCachedMeasures 是文本度量缓存上限。动态文本页面（计数/时间戳）的
// 唯一字符串无上限，超出后整体清空——度量重算比无限增长安全。
const maxCachedMeasures = 8192

// MeasureText 实现 core.Graphics：两档口径（bold 决定正/粗体面），
// 未携带数值字重的调用方走这里。
func (g *Graphics) MeasureText(text string, fontSize int, bold bool, family string) (int, int) {
	return g.measureText(text, fontSize, fontTierOf(bold, 0), family)
}

// MeasureTextWeight 实现 core.WeightedMeasurer：按数值 font-weight 归档
// 字面档位度量（550~649 → Semibold 面，如 segoeui→seguisb），与 DrawText
// 从 paint 读到的档位同口径，否则混排宽度会错位。
func (g *Graphics) MeasureTextWeight(text string, fontSize, weightNum int, family string) (int, int) {
	return g.measureText(text, fontSize, fontTierOf(weightNum >= 550, weightNum), family)
}

// measureText 是两种度量的共同实现：与 DrawText 同一套脚本分段与字体
// 解析，宽度为各段之和，行高取各段最大值（近似 Chrome 的行盒语义）。
func (g *Graphics) measureText(text string, fontSize, tier int, family string) (int, int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	key := measureKey{text: text, size: fontSize, tier: uint8(tier), family: family,
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
			f := g.face(path, fontSize, tier, false)
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

// FontMetrics 实现 core.Graphics：两档口径（bold 决定正/粗体面），与
// MeasureText 同义。
func (g *Graphics) FontMetrics(text string, fontSize int, bold bool, family string) (int, int) {
	return g.fontMetrics(text, fontSize, fontTierOf(bold, 0), family)
}

// FontMetricsWeight 实现 core.WeightedMeasurer：数值字重档位下的行度量
//（半粗面与粗体面的 ascent/descent 可能有细微差异，需同档位取用）。
func (g *Graphics) FontMetricsWeight(text string, fontSize, weightNum int, family string) (int, int) {
	return g.fontMetrics(text, fontSize, fontTierOf(weightNum >= 550, weightNum), family)
}

// fontMetrics 是两种行度量的共同实现：返回行度量（基线以上 ascent、基线
// 以下 descent）。各脚本段取最大值：行盒高度 = 各内联盒 ascent 的最大者 +
// descent 的最大者（同基线对齐）。度量来自 SDL_ttf 的 TTF_FontAscent/
// TTF_FontDescent，行高与 Chrome/DirectWrite 可能有 1~4px 差异。
// text 为空时只取主字体（不触发 CJK 逐字回退），对应行盒的 strut。
func (g *Graphics) fontMetrics(text string, fontSize, tier int, family string) (int, int) {
	if fontSize <= 0 {
		fontSize = 16
	}
	if !hasTTF() {
		h := fontSize * 13 / 10
		return h / 2, h - h/2
	}
	if text == "" {
		asc, desc, ok := g.lineMetricsOf(g.resolveFontPath(family), fontSize, tier)
		if !ok {
			h := fontSize * 13 / 10
			return h / 2, h - h/2
		}
		return asc, desc
	}
	maxAsc, maxDesc := 0, 0
	for _, seg := range splitScripts(text) {
		asc, desc, ok := g.lineMetricsOf(g.segFontPath(seg, family), fontSize, tier)
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

// lineMetricsOf 返回（字体文件, 字号, 字重档位）下的行度量：ascent 取
// TTF_FontAscent，descent 取 -TTF_FontDescent（SDL_ttf 返回负值）。
//
// 这里直接用 SDL_ttf 的 FreeType 像素取整，不再解析字体的 head/hhea/OS2。
// 代价是行高不再逐像素对齐 Chrome/DirectWrite（同一字体可能差 1~4px），
// 换来的是实现简单、无字体表解析代码；布局与绘制仍使用同一个 face，
// ascent/descent 与真实字形基线至少自洽。
func (g *Graphics) lineMetricsOf(path string, size, tier int) (int, int, bool) {
	if !hasTTFFontAscent || !hasTTFFontDescent {
		return 0, 0, false
	}
	f := g.face(path, size, tier, false)
	if f == nil {
		return 0, 0, false
	}
	// nativeCall 统一返回 uintptr，而 TTF_FontAscent/Descent 返回的是**有符号
	// int32**（Descent 为负）。不按 int32 重解释，负数会变成巨大的正数
	// （-3 → 4294967293），取负后仍是负 → 被下面的钳制吞成 0：所有字体的
	// descent 恒为 0，行盒比 Chrome 短 2~3px（文字行距偏紧、按钮内文字居中
	// 也偏下）。
	asc := int(int32(ttfFontAscent(f.handle)))
	desc := -int(int32(ttfFontDescent(f.handle)))
	if asc < 0 {
		asc = 0
	}
	if desc < 0 {
		desc = 0
	}
	return asc, desc, true
}

// DrawText 实现 core.Graphics：按脚本段绘制纹理，x 依次推进各段测量宽度，
// 各段以基线（TTF_FontAscent）对齐。
func (g *Graphics) DrawText(x, y, w, h int, paint core.Paint, text string) {
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
	italic := paintItalic(paint)
	// 数值字重：paint 携带时按档位选字面（600 → Semibold 面），与度量侧
	// MeasureTextWeight 同口径；不携带则退回两档 bold（legacy 调用）。
	wn := 0
	if wp, ok := paint.(core.WeightedPaint); ok {
		wn = wp.FontWeightNum()
	}
	tier := fontTierOf(bold, wn)

	segs := splitScripts(text)
	// 第一段（standard 段）的基线作为对齐参考；取各段中最大的 ascent
	refAscent := int32(-1)
	for _, seg := range segs {
		path := g.segFontPath(seg, family)
		if a := g.ascentOf(path, size, tier); a > refAscent {
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
		key := texKey{fontPath: path, text: seg.text, size: size, tier: uint8(tier), italic: italic, lcd: lcd,
			hint:     hint,
			contrast: uint16(kc * 100),
			r:        cr, g: cg, b: cb, a: ca}
		if lcd {
			// 落点背景色只有 LCD 路径用得到（灰度纹理与底色无关）：不写进
			// key，否则同一段文字落在 N 种底色上会建 N 张完全相同的纹理。
			key.br, key.bg, key.bb = br, bg, bb
		}
		info, ok := g.textures[key]
		if ok {
			// 命中：刷新 LRU 时钟（写回 map 才能更新 lastUsed）。
			info.lastUsed = g.nextTexClock()
			g.textures[key] = info
		} else {
			f := g.face(path, size, tier, italic)
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
			info = texInfo{tex: tex, w: int(tw), h: int(th), ascent: asc,
				lastUsed: g.nextTexClock()}
			g.textures[key] = info
			g.texPixels += int(tw) * int(th)
			g.evictTexturesIfNeeded()
		}
		if drawDebug {
			// 记录每段的解析路径与"实际渲染纹理尺寸"：若 SetFontSize 参数传递失效，
			// actual 会与 req 不符（字号被 XMM 残值之类的外部因素带偏），此处一眼可见。
			// 路径取 face 实际打开的文件（粗体可能命中真粗体变体 msyhbd.ttc 等）。
			fmt.Fprintf(os.Stderr, "[font] %q req=%dpx tier=%d -> %s actual=%dx%d ascent=%d\n",
				seg.text, size, tier, filepath.Base(g.facePath(path, size, tier)), info.w, info.h, info.ascent)
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
// paintItalic 读取 Paint 的斜体标记；自定义 Paint 不实现 core.StyledPaint
// 时按正体处理（只是少了斜体观感）。
func paintItalic(p core.Paint) bool {
	if sp, ok := p.(core.StyledPaint); ok {
		return sp.Italic()
	}
	return false
}

func (g *Graphics) ascentOf(path string, size, tier int) int32 {
	asc, _, ok := g.lineMetricsOf(path, size, tier)
	if !ok {
		return 0
	}
	return int32(asc)
}

// sdlRendererInfo 对应 SDL2 的 SDL_RendererInfo（64 位下 **88 字节**）：
//
//	const char *name;            // 8
//	Uint32 flags;                // 4
//	Uint32 num_texture_formats;  // 4
//	Uint32 texture_formats[16];  // 64 —— **内联数组**，不是指针
//	int max_texture_width;       // 4
//	int max_texture_height;      // 4
//
// 注意 texture_formats 是**内联数组**：早先按「指针 + 32 字节」估大小，
// 申请 40 字节缓冲 → SDL 按真实的 88 字节写，**越界写坏调用者的栈**
// （现场症状：调用之后渲染器句柄变成 0、随后空指针崩溃）。
// 结构体大小必须与运行库一致，不是「够读 name 就行」。
//
// 大小用 unsafe.Sizeof 断言（见 sdl_struct_test.go）。
type sdlRendererInfo struct {
	name              uintptr
	flags             uint32
	numTextureFormats uint32
	textureFormats    [16]uint32
	maxTextureWidth   int32
	maxTextureHeight  int32
}

// RendererDriver 返回渲染后端的驱动名（"direct3d11" / "opengl" /
// "software" / "llvmpipe" …），取自 SDL_GetRendererInfo。
//
// 用于诊断：软件路径的瓶颈是**填充率**（每像素纳秒数），GPU 路径的瓶颈
// 通常在别处（纹理上传次数、draw call 数、swap 间隔）——两者的排查方向
// 完全不同，先确认走的是哪条路再量。
func RendererDriver(renderer uintptr) string {
	if renderer == 0 || sdlGetRendererInfo == nil {
		return ""
	}
	var info sdlRendererInfo
	if sdlGetRendererInfo(renderer, uintptr(unsafe.Pointer(&info))) != 0 {
		return ""
	}
	if info.name == 0 {
		return ""
	}
	// 读 C 侧字符串：用 goCString（指针值的传递方式见那里的说明）。驱动名
	// 很短，64 字节足够。
	return goCString(info.name, 64)
}

// DrawColor 实现 core.Graphics。
func (g *Graphics) DrawColor(x, y, w, h int, color core.Color) {
	if color == nil || w <= 0 || h <= 0 {
		return
	}
	r, gr, b, a := color.RGBA()
	// 混合模式按**本次填充的 alpha** 分流：不透明 → NONE（直接写像素），
	// 半透明 → BLEND（逐像素混合）。
	//
	// 之前渲染器全局设成 BLEND（见 NewWindow），于是**每次填充都是逐像素
	// 混合**：软件光栅器下约 2.1ns/像素（直接写像素只要 0.2ns）。门户页一帧
	// 要填 1700 万像素（清屏两块 + 页面背景 + 卡片），2560×1440 窗体下单帧
	// 37ms —— 最大化后滚动直接掉到 27fps。小窗体看不出来（像素少），最大化
	// 后才发现是填充率的问题。
	//
	// 判定必须严格按 **A == 255**：SDL_BLENDMODE_NONE 下源 alpha 是**直接
	// 拷贝**进目标（不是忽略）。把 A=250 当成不透明走 NONE，目标里就会留下
	// alpha=250 的像素——主窗口最终输出忽略 alpha 看不出来，但离屏目标、
	// 后续读回（截图）会带上它。
	blend := uintptr(blendModeBlend)
	if a == 255 {
		blend = blendModeNone
	}
	sdlSetRenderDrawBlendMode(g.renderer, blend)
	// 成对复原：混合模式是渲染器的**全局状态**，漏切会让后面的贴图拷贝
	// （精灵、文字的抗锯齿边缘）用错模式。用 defer 保证任何退出路径
	// （提前返回、错误、panic）都复原——不靠调用方「记得切回来」。
	defer sdlSetRenderDrawBlendMode(g.renderer, blendModeBlend)
	sdlSetRenderDrawColor(g.renderer, uintptr(r), uintptr(gr), uintptr(b), uintptr(a))
	rect := sdlRect{int32(x), int32(y), int32(w), int32(h)}
	checkDraw("SDL_RenderFillRect", sdlRenderFillRect(g.renderer, uintptr(unsafe.Pointer(&rect))))
}

// DrawImage 暂不支持（预留接口）。
func (g *Graphics) DrawImage(x, y, w, h int, image core.Image) {
}

// pixelFormatRGBA32 是 SDL_PIXELFORMAT_RGBA32（0x16762004）：小端下内存
// 字节序恰为 R,G,B,A —— 引擎的精灵像素可直接灌入，无需逐像素换序。
//
// 常量值是实测出来的（不能凭 SDL 头文件的枚举顺序推断：相邻的几个
// 8888 格式只差一个字节序位，用错就是"alpha 读成 R"或"整块不透明"）。
const pixelFormatRGBA32 = 0x16762004

// textureAccessStreaming 是 SDL_TEXTUREACCESS_STREAMING：纹理由
// SDL_UpdateTexture 灌像素（STATIC 只允许从 surface 创建）。
const textureAccessStreaming = 0

// DrawSprite 实现 core.SpriteDrawer：把位图精灵上传成纹理并 blit 一次。
//
// 精灵按指针缓存（引擎侧的角标精灵按「半径+颜色」缓存，同一份像素只上传
// 一次）。整块圆角一次 blit，替掉逐像素的半透明小方块——后者是绘制调用
// 的主要来源（实测 portal 页一帧 1.8 万次 1×1 绘制），GPU 路径上每次都
// 是驱动调用，帧时间被拖到几十毫秒，滚动动画被切成两三帧。
func (g *Graphics) DrawSprite(x, y int, s *core.Sprite) {
	if s == nil || s.W <= 0 || s.H <= 0 || g.renderer == 0 || len(s.Pix) < s.W*s.H*4 {
		return
	}
	entry, ok := g.sprites[s]
	if !ok {
		tex := sdlCreateTexture(g.renderer, uintptr(pixelFormatRGBA32),
			uintptr(textureAccessStreaming), uintptr(s.W), uintptr(s.H))
		if tex == 0 {
			checkDraw("SDL_CreateTexture", 0)
			return
		}
		sdlSetTextureBlendMode(tex, uintptr(blendModeBlend))
		// 精灵的像素创建后不变（引擎按「半径+颜色」缓存复用同一份，见
		// engine 的 spriteCache），所以纹理**只上传一次**：每次绘制都
		// UpdateTexture 的话，一帧几百次纹理上传会吃掉大半帧时间——实测
		// 门户页 300 次/帧 ≈ 2ms，240Hz 屏幕（帧预算 4.2ms）直接超支，
		// 滚动就掉帧。
		rect := sdlRect{0, 0, int32(s.W), int32(s.H)}
		if sdlUpdateTexture(tex, uintptr(unsafe.Pointer(&rect)),
			uintptr(unsafe.Pointer(&s.Pix[0])), uintptr(s.W*4)) != 0 {
			checkDraw("SDL_UpdateTexture", 0)
			return
		}
		entry = &spriteTex{tex: tex, w: s.W, h: s.H, lastUsed: g.nextTexClock()}
		g.sprites[s] = entry
		g.spritePixels += s.W * s.H
		g.spriteUploads++
		g.evictSpritesIfNeeded()
	} else {
		entry.lastUsed = g.nextTexClock()
	}
	dst := sdlRect{int32(x), int32(y), int32(s.W), int32(s.H)}
	checkDraw("SDL_RenderCopy", sdlRenderCopy(g.renderer, entry.tex, 0, uintptr(unsafe.Pointer(&dst))))
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

// PushClip 实现 core.Clipper：与当前裁剪区求交后下推并应用。
// 裁剪区由引擎在 padding 盒/滚动容器上给出，坐标即渲染坐标。
func (g *Graphics) PushClip(x, y, w, h int) {
	r := sdlRect{int32(x), int32(y), int32(w), int32(h)}
	if n := len(g.clips); n > 0 {
		r = intersectSdlRect(g.clips[n-1], r)
	}
	g.clips = append(g.clips, r)
	g.applyClip()
}

// PopClip 实现 core.Clipper：弹出栈顶并应用剩余栈顶（空栈=关闭裁剪）。
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
	g.texPixels = 0
	for _, t := range g.sprites {
		if t.tex != 0 {
			sdlDestroyTexture(t.tex)
		}
	}
	g.sprites = make(map[*core.Sprite]*spriteTex)
	g.spritePixels = 0
}

// maxCachedTextures 是文本纹理缓存的条数上限，maxCachedTexPixels 是像素
// 上限（显存占用上限，纹理按 4 字节/像素估）。超出后按**最近最少使用**
// 淘汰。
//
// 淘汰策略必须是 LRU 而非 FIFO：FIFO 按插入顺序淘汰，会把页面上复用最多的
// 纹理（分隔符、价签、图标文字等）先淘汰掉——工作集略大于上限时每帧都要
// 重新渲染全部文本（缓存颠簸），实测 portal 页 774 段文本 / 上限 512 时
// 单帧绘制从 52ms 涨到 149ms，滚动肉眼可见地"一卡一卡"。
const (
	maxCachedTextures  = 4096
	maxCachedTexPixels = 8 << 20 // 8M 像素 ≈ 32MB
)

// spriteTex 是精灵纹理的缓存项：纹理 + 尺寸 + 最近使用时刻。
type spriteTex struct {
	tex      uintptr
	w, h     int
	lastUsed uint64
}

// 精灵纹理缓存的上限：条数与像素数（像素数按 4 字节/像素估显存）。
//
// 精灵纹理比文本纹理小得多（一个圆角 r×r），但键里含颜色——页面逐帧改色
// 会不断产生新精灵，不设上限就是显存泄漏。
const (
	maxCachedSprites      = 2048    // 条目上限
	maxCachedSpritePixels = 2 << 20 // 像素上限（2M 像素 ≈ 8MB）
)

// evictSpritesIfNeeded 精灵纹理超出上限时按最近最少使用淘汰。
//
// 一次淘汰一批（降到 90%），摊平扫描成本：满页逐帧换色时每次新建纹理都
// 扫全表，帧时间会被吃掉。
func (g *Graphics) evictSpritesIfNeeded() {
	if len(g.sprites) <= maxCachedSprites && g.spritePixels <= maxCachedSpritePixels {
		return
	}
	targetItems := maxCachedSprites * 9 / 10
	targetPixels := maxCachedSpritePixels * 9 / 10
	for len(g.sprites) > targetItems || g.spritePixels > targetPixels {
		oldKey, old, ok := g.lruSprite()
		if !ok {
			return
		}
		if old.tex != 0 {
			sdlDestroyTexture(old.tex)
		}
		g.spritePixels -= old.w * old.h
		delete(g.sprites, oldKey)
	}
}

// lruSprite 找出最近最少使用的精灵纹理。
func (g *Graphics) lruSprite() (*core.Sprite, *spriteTex, bool) {
	var oldKey *core.Sprite
	var old *spriteTex
	found := false
	for k, e := range g.sprites {
		if !found || e.lastUsed < old.lastUsed {
			oldKey, old, found = k, e, true
		}
	}
	return oldKey, old, found
}

// evictTexturesIfNeeded 超出缓存上限（条数或像素）时淘汰最近最少使用的纹理。
func (g *Graphics) evictTexturesIfNeeded() {
	for len(g.textures) > maxCachedTextures || g.texPixels > maxCachedTexPixels {
		old, ok := g.lruTexture()
		if !ok {
			return
		}
		if info, ok := g.textures[old]; ok {
			if info.tex != 0 {
				sdlDestroyTexture(info.tex)
			}
			g.texPixels -= info.w * info.h
			delete(g.textures, old)
		}
	}
}

// lruTexture 找出最近最少使用的纹理键。
func (g *Graphics) lruTexture() (texKey, bool) {
	var old texKey
	found := false
	var oldest uint64
	for k, info := range g.textures {
		if !found || info.lastUsed < oldest {
			old, oldest, found = k, info.lastUsed, true
		}
	}
	return old, found
}

// nextTexClock 返回单调递增的纹理使用时钟（LRU 序）。
func (g *Graphics) nextTexClock() uint64 {
	g.texClock++
	return g.texClock
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

var _ core.Graphics = (*Graphics)(nil)
