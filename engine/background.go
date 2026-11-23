package engine

import (
	"math"
	"strconv"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// CSS 背景图层（background-image 及配套长写属性）
//
// 引擎此前只认 background-color：作者写 background-image: url(...) 时整条
// 声明被丢弃。这一层补上 CSS 背景的**单层完整语义 + 多层堆叠**——
//
//	background-image、background（简写）、background-repeat、
//	background-position、background-size、background-origin、background-clip
//
// 不做（docs/limitations.md）：渐变 <image>、background-attachment: fixed、
// repeat 的 space/round 两种铺法（退化为普通平铺）；圆角仍用「角标补盖」
// 近似（引擎的裁剪栈只有矩形）。
//
// 图片字节仍由宿主注入的加载器提供（引擎不读文件、不联网），与 <img> 共用
// 同一套文档级缓存与异步收口：见 image.go 的 lookupImage/fetchImage。
//
// 图层表的顺序就是 CSS 声明里的顺序：**首层画在最上面**（CSS 的
// background-image 列表首层是「最靠近观察者的层」），背景色画在所有层之下。
// ---------------------------------------------------------------------------

// backgroundLayer 是一张背景图层：引用、几何口径与已绑定的位图。
//
// 零值即 CSS 各长写属性的**初始值**（repeat、position 0% 0%、size auto、
// origin padding-box、clip border-box）：图层可能由任一长写属性首次创建
// （作者先写 background-repeat 再写 background-image 很常见），创建处一律
// append 零值，不手工填默认值——所以下面几组常量的 0 号位必须正好是各自
// 属性的初始值。
type backgroundLayer struct {
	// key 是关键字型取值的共享字典（repeat/origin/clip 三个口径）：整页
	// 几十万个元素都写同样的背景口径时，指针相等即口径相等，级联里
	// 「图层是否还是初始值」的判定是指针比较而不是逐字段比较。
	key *backgroundKey
	// bitmap 是已解码的位图，绘制期按 src 从文档图片缓存绑定（见
	// bindBackgroundImages）；nil 表示未声明图片、尚未到货或加载失败。
	bitmap *Bitmap
	// src 是 url() 里的原始引用（**未**解析）。空串表示这层没有图片
	// （CSS 的 none）。
	//
	// 为什么不解析成绝对地址再存：级联期拿不到文档 BaseURL——样式表自己的
	// 地址没有随规则保存（simpleDecl 只有 prop/value/important），@import
	// 的基准只在解析阶段用了一次。绘制期能拿到文档，所以这里存原样引用，
	// 绑定时再解析（口径与 <img> 的 src 一致：相对文档地址）。
	src string
	// posX/posY 是 background-position 的两个轴。解析层会把单值简写补齐
	// （另一轴是 center），所以这里不再区分「某一轴没写」。
	posX backgroundLen
	posY backgroundLen
	// sizeW/sizeH 是 background-size 的两个轴；sizeMode 非 auto 时忽略。
	sizeW backgroundLen
	sizeH backgroundLen
}

// backgroundKey 是图层的三个单字节口径（共享字典项）。
type backgroundKey struct {
	repeat   uint8
	sizeMode uint8
	boxPair  uint8 // origin<<2 | clip（两个盒子口径打包进一字节）
}

// backgroundLen 是 background-position / background-size 的一个分量。
//
// 这两处允许的形态不同，用一个 kind 区分：
//   - bgLenAuto：auto 或未声明（position 按 0%、size 按固有尺寸）；
//   - bgLenKey：轴向关键字（left/center/right/top/bottom）；
//   - bgLenVal：长度或百分比（unit 决定，val 存数值）；
//   - bgLenEdge：「贴某条边再偏移」（right 10px bottom 20px），key 是那条边。
type backgroundLen struct {
	kind uint8
	key  uint8
	unit uint8
	val  float32
}

// 每组常量各自一个 const 块：**iota 在块内独立计数**，所以每组的 0 号位
// 正好是「初始值」——backgroundKey 的三个口径就是靠这个前提打包进一字节
// 的（origin<<2 | clip，clip 必须落在低 2 位才能用 & 3 取出来）。
const (
	// backgroundLen.kind
	bgLenAuto = iota
	bgLenKey
	bgLenVal
	bgLenEdge
)

const (
	// backgroundLen.key：轴向关键字，同时也是 bgLenEdge 的参考边
	bgKeyNone = iota
	bgKeyLeft
	bgKeyCenter
	bgKeyRight
	bgKeyTop
	bgKeyBottom
)

const (
	// backgroundLen.unit
	bgUnitPx = iota
	bgUnitPct
)

const (
	// backgroundKey.repeat：0 号位 = CSS 初始值 repeat。
	// space/round 解析成 repeat（不做等距铺开，见文件头）。
	bgRepeatRepeat = iota
	bgRepeatX
	bgRepeatY
	bgRepeatNone
)

const (
	// backgroundKey.sizeMode：0 号位 = auto（按 sizeW/sizeH 或固有尺寸）
	bgSizeAuto = iota
	bgSizeCover
	bgSizeContain
)

const (
	// backgroundKey 里的裁剪盒口径，0 号位 = background-clip 初始值。
	bgClipBorder = iota
	bgClipPadding
	bgClipContent
)

const (
	// backgroundKey 里的定位盒口径，0 号位 = background-origin 初始值。
	bgOriginPadding = iota
	bgOriginBorder
	bgOriginContent
)

// 每轴最多平铺这么多块：1px 的图铺满大盒子时瓦片数等于裁剪区宽度，
// 超出的部分不画（宁可少画，也不让一帧被瓦片循环吃满）。
const maxBackgroundTiles = 4096

// backgroundKeyOf 从图层的共享字典取三项口径（字典为 nil 即全初始值）。
func backgroundKeyOf(l *backgroundLayer) (repeat, sizeMode, clip uint8) {
	if l.key == nil {
		return bgRepeatRepeat, bgSizeAuto, bgClipBorder
	}
	return l.key.repeat, l.key.sizeMode, l.key.boxPair & 3
}

// layerOrigin 取图层的定位盒口径。
func (l *backgroundLayer) originBox() uint8 {
	if l.key == nil {
		return bgOriginPadding
	}
	return l.key.boxPair >> 2
}

// newBackgroundLayer 构造「位图铺满定位盒、不平铺」的一层背景，供公开 API
// SetBackgroundImage 用（等价 background-size: 100% 100%）。
//
// 定位盒与裁剪盒都取**边框盒**：这是对旧行为的兼容口径——旧实现直接
// DrawImage(bx, by, bw, bh) 铺满整个边框盒。CSS 走解析后的图层表，不经过这里。
func newBackgroundLayer(bmp *Bitmap, wPct, hPct int) backgroundLayer {
	l := backgroundLayer{bitmap: bmp, key: &backgroundKey{
		repeat:   bgRepeatNone,
		boxPair:  (bgOriginBorder << 2) | bgClipBorder,
		sizeMode: bgSizeAuto,
	}}
	if wPct > 0 {
		l.sizeW = backgroundLen{kind: bgLenVal, unit: bgUnitPct, val: float32(wPct)}
	}
	if hPct > 0 {
		l.sizeH = backgroundLen{kind: bgLenVal, unit: bgUnitPct, val: float32(hPct)}
	}
	return l
}

// copyBackgroundLayers 拷贝图层表。图层里的位图指针指向解码后不再改动的
// 数据，共享字典按指针复用，所以一次元素级拷贝就是深拷贝。copyStyle 用它：
// 直接赋值会让两个元素共用同一块底层数组，其中一方改写图层会串到另一方。
func copyBackgroundLayers(src []backgroundLayer) []backgroundLayer {
	if len(src) == 0 {
		return nil
	}
	out := make([]backgroundLayer, len(src))
	copy(out, src)
	return out
}

// ---------------------------------------------------------------------------
// 解析：长写属性
//
// CSS 的背景长写属性都是**值表**（逗号分层），与 background-image 的层一一对齐。
// 层数由 background-image 决定；先出现的长写属性就地创建图层（append 零值
// 即各属性初始值），后到的 background-image 只填 src，于是「background-repeat
// 写在 background-image 之前」也能对上（同一轮级联里声明顺序本就不保证）。
//
// 一条声明里任何一层非法就整条忽略：CSS 的语义是「声明要么全部生效要么
// 全部无效」，只落一半比不落更糟。
// ---------------------------------------------------------------------------

// alignBackgroundLayers 把图层表长到至少 n 层（新层是零值 = 各属性初始值）。
func (cs *cssStyleDeclaration) alignBackgroundLayers(n int) {
	for n > 0 && len(cs.background) < n {
		cs.background = append(cs.background, backgroundLayer{})
	}
}

// applyBackgroundImageDecl 处理 background-image。
func applyBackgroundImageDecl(cs *cssStyleDeclaration, value string) {
	parts := splitTopLevel(value, ',')
	refs := make([]string, 0, len(parts))
	for _, part := range parts {
		ref, ok := parseImageRef(part)
		if !ok {
			return
		}
		refs = append(refs, ref)
	}
	cs.alignBackgroundLayers(len(refs))
	if len(cs.background) > len(refs) {
		// 图层数由 background-image 决定：新的列表更短就截断（CSS 里其它长写
		// 的值表按索引对齐到新的层数，多余的层连几何口径一起消失）。
		cs.background = cs.background[:len(refs)]
	}
	for i, ref := range refs {
		cs.background[i].src = ref
		cs.background[i].bitmap = nil // 换图即失效，绘制期重新绑定
	}
}

// parseImageRef 解析一层 <image> 取值：url(...) 返回引用，none 返回空串。
// 渐变与其它 <image> 形态按非法处理（整条声明忽略）。
//
// 一层只能有**一个** <image>：`url(a.png) url(b.png)` 在 CSS 里非法，
// 浏览器整条丢弃；这里同样整条忽略（只认头一个 url 的话，作者写错了也
// 会「看起来生效」，反而更迷惑）。
func parseImageRef(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, "none") {
		return "", true
	}
	if len(v) >= 4 && strings.EqualFold(v[:4], "url(") {
		ref, end, ok := scanURLToken(v)
		if !ok || ref == "" || strings.TrimSpace(v[end:]) != "" {
			return "", false
		}
		return ref, true
	}
	return "", false
}

// parseURLToken 取出 url( ... ) 里的引用（去掉可选引号与首尾空白）。
func parseURLToken(v string) (string, bool) {
	ref, _, ok := scanURLToken(v)
	return ref, ok
}

// scanURLToken 取出 url( ... ) 里的引用，并返回右括号之后的位置（end）。
// end 用于判断 url() 之后是否还有别的内容（background-image 一层只允许
// 一个 <image>）。
func scanURLToken(v string) (ref string, end int, ok bool) {
	i := strings.IndexByte(v, '(')
	if i < 0 {
		return "", 0, false
	}
	rest := v[i+1:]
	depth := 1
	stop := -1
	for j := 0; j < len(rest); j++ {
		switch rest[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				stop = j
			}
		}
		if stop >= 0 {
			break
		}
	}
	if stop < 0 {
		return "", 0, false
	}
	ref = strings.TrimSpace(rest[:stop])
	if len(ref) >= 2 {
		if q := ref[0]; q == '"' || q == '\'' {
			if ref[len(ref)-1] == q {
				ref = ref[1 : len(ref)-1]
			}
		}
	}
	return ref, i + 1 + stop + 1, true
}

// applyBackgroundRepeatDecl 处理 background-repeat。值表**循环**对齐到各层
// （CSS 2.1 §12.4：repeat 列表比图层短时整表循环）。
func applyBackgroundRepeatDecl(cs *cssStyleDeclaration, value string) {
	parts := splitTopLevel(value, ',')
	modes := make([]uint8, 0, len(parts))
	for _, part := range parts {
		m, err := parseBackgroundRepeatValue(part)
		if err != nil {
			return
		}
		modes = append(modes, m)
	}
	if len(cs.background) == 0 {
		cs.alignBackgroundLayers(1)
	}
	for i := range cs.background {
		setBackgroundRepeat(&cs.background[i], modes[i%len(modes)])
	}
}

// parseBackgroundRepeatValue 解析一层的平铺方式：可给 1~2 个关键字
// （第二个是纵向），space/round 归入 repeat。
func parseBackgroundRepeatValue(v string) (uint8, error) {
	toks := splitValueTokens(strings.ToLower(strings.TrimSpace(v)))
	if len(toks) == 0 || len(toks) > 2 {
		return bgRepeatRepeat, errBadBackgroundValue
	}
	x, err := repeatAxis(toks[0])
	if err != nil {
		return bgRepeatRepeat, err
	}
	if len(toks) == 1 {
		// 单值 repeat-x/repeat-y 已直接给出轴向答案，其余形态两轴同值。
		return x, nil
	}
	y, err := repeatAxis(toks[1])
	if err != nil {
		return bgRepeatRepeat, err
	}
	switch {
	case x == bgRepeatNone && y == bgRepeatNone:
		return bgRepeatNone, nil
	case x == bgRepeatNone: // 横向不铺、纵向铺
		return bgRepeatY, nil
	case y == bgRepeatNone: // 横向铺、纵向不铺
		return bgRepeatX, nil
	}
	return bgRepeatRepeat, nil
}

// repeatAxis 解析平铺关键字；no-repeat 之外的都归一为「这个轴要铺」。
func repeatAxis(tok string) (uint8, error) {
	switch tok {
	case "repeat", "space", "round":
		return bgRepeatRepeat, nil
	case "repeat-x":
		return bgRepeatX, nil
	case "repeat-y":
		return bgRepeatY, nil
	case "no-repeat":
		return bgRepeatNone, nil
	}
	return bgRepeatRepeat, errBadBackgroundValue
}

// applyBackgroundPositionDecl 处理 background-position（值表循环对齐到各层）。
func applyBackgroundPositionDecl(cs *cssStyleDeclaration, value string) {
	parts := splitTopLevel(value, ',')
	xs := make([]backgroundLen, 0, len(parts))
	ys := make([]backgroundLen, 0, len(parts))
	for _, part := range parts {
		x, y, err := parseBackgroundPositionValue(part)
		if err != nil {
			return
		}
		xs, ys = append(xs, x), append(ys, y)
	}
	if len(cs.background) == 0 {
		cs.alignBackgroundLayers(1)
	}
	// CSS Backgrounds 3 §3：背景长写的值表是「循环表」——比图层少就整表循环，
	// 多出来的取值忽略（不是只给前几层，也不让后面的层回初始值）。
	for i := range cs.background {
		cs.background[i].posX, cs.background[i].posY = xs[i%len(xs)], ys[i%len(ys)]
	}
}

// bgPosAxis 是 <position> 解析过程中某一个轴的临时状态。
type bgPosAxis struct {
	set      bool          // 这一轴已被占用
	key      uint8         // 边关键字（left/right/top/bottom）
	haveEdge bool          // key 是边关键字（不是 center）
	edgeSet  bool          // 边关键字后面已经吃到偏移量
	edgeLen  backgroundLen // 偏移量
	plain    backgroundLen // 普通长度/百分比
}

// parseBackgroundPositionValue 解析一层的 <position>，返回两个轴。
//
// 支持三种形态（CSS Backgrounds 3 §3.5 的 <position> 文法）：
//   - 单值：另一轴按规范补 center（top/bottom 算纵轴，其余算横轴）；
//   - 双值：一横一纵，顺序可换（"bottom right" 与 "right bottom" 同义）；
//   - 边偏移：left|right <长度> 与 top|bottom <长度>（最长四值）。
func parseBackgroundPositionValue(v string) (x, y backgroundLen, err error) {
	toks := splitValueTokens(strings.ToLower(strings.TrimSpace(v)))
	if len(toks) == 0 || len(toks) > 4 {
		return x, y, errBadBackgroundValue
	}
	var xa, ya bgPosAxis
	axisFor := func(k uint8) *bgPosAxis {
		if k == bgKeyTop || k == bgKeyBottom {
			return &ya
		}
		return &xa
	}
	// pendingEdge 返回「刚写了边关键字、还没吃到偏移量」的那一轴：紧跟的
	// 长度就是那条边的偏移量（right 10px）。
	pendingEdge := func() *bgPosAxis {
		if xa.haveEdge && !xa.edgeSet {
			return &xa
		}
		if ya.haveEdge && !ya.edgeSet {
			return &ya
		}
		return nil
	}
	for _, tok := range toks {
		if k := positionKeyword(tok); k != bgKeyNone {
			if k == bgKeyCenter {
				// center 两轴都能落：先到先得（"center center" 合法）。
				switch {
				case !xa.set:
					xa.set, xa.plain = true, backgroundLen{kind: bgLenKey, key: bgKeyCenter}
				case !ya.set:
					ya.set, ya.plain = true, backgroundLen{kind: bgLenKey, key: bgKeyCenter}
				default:
					return x, y, errBadBackgroundValue
				}
				continue
			}
			a := axisFor(k)
			if a.set {
				return x, y, errBadBackgroundValue
			}
			*a = bgPosAxis{set: true, key: k, haveEdge: true}
			continue
		}
		val, e := parseBackgroundLen(tok)
		if e != nil {
			return x, y, e
		}
		if val.kind != bgLenVal {
			return x, y, errBadBackgroundValue // position 不接受 auto
		}
		if a := pendingEdge(); a != nil {
			a.edgeSet, a.edgeLen = true, val
			continue
		}
		// 普通长度：先横后纵。
		switch {
		case !xa.set:
			xa.set, xa.plain = true, val
		case !ya.set:
			ya.set, ya.plain = true, val
		default:
			return x, y, errBadBackgroundValue
		}
	}
	if !xa.set && !ya.set {
		return x, y, errBadBackgroundValue
	}
	axisLen := func(a *bgPosAxis) backgroundLen {
		switch {
		case !a.set:
			// 单值简写：没给的这一轴按规范是 center。
			return backgroundLen{kind: bgLenKey, key: bgKeyCenter}
		case a.haveEdge && !a.edgeSet:
			return backgroundLen{kind: bgLenKey, key: a.key}
		case a.haveEdge:
			return backgroundLen{kind: bgLenEdge, key: a.key, unit: a.edgeLen.unit, val: a.edgeLen.val}
		}
		return a.plain
	}
	return axisLen(&xa), axisLen(&ya), nil
}

// positionKeyword 识别 <position> 的关键字，非关键字返回 bgKeyNone。
func positionKeyword(tok string) uint8 {
	switch tok {
	case "left":
		return bgKeyLeft
	case "center":
		return bgKeyCenter
	case "right":
		return bgKeyRight
	case "top":
		return bgKeyTop
	case "bottom":
		return bgKeyBottom
	}
	return bgKeyNone
}

// parseBackgroundLen 解析长度或百分比（px/无单位数字/%）。
func parseBackgroundLen(tok string) (backgroundLen, error) {
	tok = strings.TrimSpace(tok)
	if strings.HasSuffix(tok, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(tok, "%"), 32)
		if err != nil {
			return backgroundLen{}, errBadBackgroundValue
		}
		return backgroundLen{kind: bgLenVal, unit: bgUnitPct, val: float32(f)}, nil
	}
	if strings.EqualFold(tok, "auto") {
		return backgroundLen{kind: bgLenAuto}, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(tok), "px"), 32)
	if err != nil {
		return backgroundLen{}, errBadBackgroundValue
	}
	return backgroundLen{kind: bgLenVal, unit: bgUnitPx, val: float32(f)}, nil
}

// applyBackgroundSizeDecl 处理 background-size（值表循环对齐到各层）。
func applyBackgroundSizeDecl(cs *cssStyleDeclaration, value string) {
	parts := splitTopLevel(value, ',')
	modes := make([]uint8, 0, len(parts))
	ws := make([]backgroundLen, 0, len(parts))
	hs := make([]backgroundLen, 0, len(parts))
	for _, part := range parts {
		mode, w, h, err := parseBackgroundSizeValue(part)
		if err != nil {
			return
		}
		modes, ws, hs = append(modes, mode), append(ws, w), append(hs, h)
	}
	if len(cs.background) == 0 {
		cs.alignBackgroundLayers(1)
	}
	for i := range cs.background {
		j := i % len(modes)
		setBackgroundSize(&cs.background[i], modes[j])
		cs.background[i].sizeW, cs.background[i].sizeH = ws[j], hs[j]
	}
}

// parseBackgroundSizeValue 解析一层的 <bg-size>：cover/contain 记成模式，
// auto/长度/百分比记成两个轴（单值时另一轴是 auto）。
func parseBackgroundSizeValue(v string) (uint8, backgroundLen, backgroundLen, error) {
	toks := splitValueTokens(strings.ToLower(strings.TrimSpace(v)))
	if len(toks) == 0 {
		return bgSizeAuto, backgroundLen{}, backgroundLen{}, errBadBackgroundValue
	}
	if len(toks) == 1 {
		switch toks[0] {
		case "cover":
			return bgSizeCover, backgroundLen{}, backgroundLen{}, nil
		case "contain":
			return bgSizeContain, backgroundLen{}, backgroundLen{}, nil
		}
	}
	if len(toks) > 2 {
		return bgSizeAuto, backgroundLen{}, backgroundLen{}, errBadBackgroundValue
	}
	w, err := parseSizeAxis(toks[0])
	if err != nil {
		return bgSizeAuto, backgroundLen{}, backgroundLen{}, err
	}
	h := backgroundLen{kind: bgLenAuto}
	if len(toks) > 1 {
		if h, err = parseSizeAxis(toks[1]); err != nil {
			return bgSizeAuto, backgroundLen{}, backgroundLen{}, err
		}
	}
	// CSS：背景图尺寸为 0 非法（0 长度与 0% 都不接受），整条声明忽略。
	for _, l := range []backgroundLen{w, h} {
		if l.kind == bgLenVal && l.val == 0 {
			return bgSizeAuto, backgroundLen{}, backgroundLen{}, errBadBackgroundValue
		}
	}
	return bgSizeAuto, w, h, nil
}

// parseSizeAxis 解析 size 的一个轴：auto 或非负的长度/百分比。
func parseSizeAxis(tok string) (backgroundLen, error) {
	if positionKeyword(tok) != bgKeyNone {
		return backgroundLen{}, errBadBackgroundValue // size 不接受位置关键字
	}
	l, err := parseBackgroundLen(tok)
	if err != nil {
		return l, err
	}
	if l.kind == bgLenVal && l.val < 0 {
		return l, errBadBackgroundValue
	}
	return l, nil
}

// applyBackgroundBoxDecl 处理 background-origin / background-clip（值表循环
// 对齐到各层）。
func applyBackgroundBoxDecl(cs *cssStyleDeclaration, value string, origin bool) {
	parts := splitTopLevel(value, ',')
	boxes := make([]uint8, 0, len(parts))
	for _, part := range parts {
		b, err := parseBackgroundBoxValue(part, origin)
		if err != nil {
			return
		}
		boxes = append(boxes, b)
	}
	if len(cs.background) == 0 {
		cs.alignBackgroundLayers(1)
	}
	for i := range cs.background {
		b := boxes[i%len(boxes)]
		if origin {
			setBackgroundOrigin(&cs.background[i], b)
		} else {
			setBackgroundClip(&cs.background[i], b)
		}
	}
}

// parseBackgroundBoxValue 解析一个 box 关键字，按 origin/clip 各自的常量组
// 返回（两组的 0 号位不同：origin 初始是 padding-box，clip 初始是 border-box）。
func parseBackgroundBoxValue(v string, origin bool) (uint8, error) {
	toks := splitValueTokens(strings.ToLower(strings.TrimSpace(v)))
	if len(toks) != 1 {
		return 0, errBadBackgroundValue
	}
	switch toks[0] {
	case "border-box":
		if origin {
			return bgOriginBorder, nil
		}
		return bgClipBorder, nil
	case "padding-box":
		if origin {
			return bgOriginPadding, nil
		}
		return bgClipPadding, nil
	case "content-box":
		if origin {
			return bgOriginContent, nil
		}
		return bgClipContent, nil
	}
	return 0, errBadBackgroundValue
}

// ---------------------------------------------------------------------------
// 口径字段的读写（key 是指向「本元素共享的口径字典」的指针，改一项就得
// 换一份新的字典，绝不能就地改：字典可能被多个元素共用，也是 copyStyle
// 的浅拷贝目标）
// ---------------------------------------------------------------------------

// keyOf 取图层当前口径的可写字典（nil 表示全初始值，按需复制一份）。
func layerKey(l *backgroundLayer) backgroundKey {
	if l.key == nil {
		return backgroundKey{repeat: bgRepeatRepeat, sizeMode: bgSizeAuto,
			boxPair: (bgOriginPadding << 2) | bgClipBorder}
	}
	return *l.key
}

func setBackgroundRepeat(l *backgroundLayer, repeat uint8) {
	k := layerKey(l)
	k.repeat = repeat
	l.key = internBackgroundKey(k)
}

func setBackgroundSize(l *backgroundLayer, mode uint8) {
	k := layerKey(l)
	k.sizeMode = mode
	l.key = internBackgroundKey(k)
}

func setBackgroundClip(l *backgroundLayer, clip uint8) {
	k := layerKey(l)
	k.boxPair = (k.boxPair &^ 3) | (clip & 3)
	l.key = internBackgroundKey(k)
}

func setBackgroundOrigin(l *backgroundLayer, origin uint8) {
	k := layerKey(l)
	k.boxPair = (k.boxPair &^ (3 << 2)) | ((origin & 3) << 2)
	l.key = internBackgroundKey(k)
}

// internBackgroundKey 把口径字典收进进程级共享表（组合数有限：4×3×3×3=108，
// 加上简写可能产生的中间态也不会过百）。返回 nil 表示「全初始值」——图层
// 的 key 留空，零值即初始值，不必占一次分配。
func internBackgroundKey(k backgroundKey) *backgroundKey {
	if k.repeat == bgRepeatRepeat && k.sizeMode == bgSizeAuto &&
		k.boxPair == ((bgOriginPadding<<2)|bgClipBorder) {
		return nil
	}
	backgroundKeyMu.Lock()
	defer backgroundKeyMu.Unlock()
	if p := backgroundKeyTable[k]; p != nil {
		return p
	}
	p := &backgroundKey{repeat: k.repeat, sizeMode: k.sizeMode, boxPair: k.boxPair}
	backgroundKeyTable[k] = p
	return p
}

var (
	backgroundKeyMu    = sync.Mutex{}
	backgroundKeyTable = map[backgroundKey]*backgroundKey{}
)

// ---------------------------------------------------------------------------
// 解析：background 简写
//
// 简写**重置**它覆盖的图层的全部长写属性：CSS 的 background 是八个长写的
// 简写，没写出来的那些一律回初始值。背景色只属于最后一层（CSS 规定），
// 这里按「后出现的颜色覆盖前面的」取——多层的 background 简写同时带颜色
// 在真实页面里几乎见不到，见到就按最后一层。
// ---------------------------------------------------------------------------

// errBadBackgroundValue 表示这条背景声明非法（整条忽略）。
var errBadBackgroundValue = &backgroundError{}

type backgroundError struct{}

func (*backgroundError) Error() string { return "engine: 非法的 background 取值" }

// applyBackgroundShorthand 解析 background 简写，返回图层表与背景色。
// ok=false 表示整条声明非法，调用方什么都不改。
//
// 一层的文法是 `<pos> / <size> <repeat> <origin> [<clip>] <attachment> <color>`，
// 除 image 与 color 外全部可省，且顺序不固定（CSS 允许任意交错）。这里按
// 「先摘 url()，再按斜杠切出 size，剩下的 token 逐个归类」两趟走：归到
// position 的 token 攒起来最后交给 parseBackgroundPositionValue，这样
// `right 10px bottom 20px` 这种边偏移写法不必在这里重复一遍轴判定。
func applyBackgroundShorthand(value string) (layers []backgroundLayer, bg Color, ok bool) {
	if v := strings.ToLower(strings.TrimSpace(value)); v == "" ||
		v == "inherit" || v == "initial" || v == "unset" || v == "revert" {
		return nil, nil, false
	}
	for _, part := range splitTopLevel(value, ',') {
		var l backgroundLayer
		ref, bad := parseLayerImage(part)
		if bad {
			return nil, nil, false
		}
		l.src = ref
		sizeToks, restToks, bad := splitBackgroundSize(splitBackgroundTokens(stripURLFunctions(part)))
		if bad {
			return nil, nil, false
		}
		if len(sizeToks) > 0 {
			mode, w, h, err := parseBackgroundSizeValue(strings.Join(sizeToks, " "))
			if err != nil {
				return nil, nil, false
			}
			l.sizeW, l.sizeH = w, h
			if mode != bgSizeAuto {
				setBackgroundSize(&l, mode)
			}
		}
		var (
			repeatToks = make([]string, 0, 2)
			boxToks    = make([]string, 0, 2)
			posToks    = make([]string, 0, 4)
		)
		for _, tok := range restToks {
			// 关键字类属性大小写不敏感；颜色与 url 引用则要保留原样
			// （url 已经在 parseLayerImage 里单独摘走，这里剩的是颜色）。
			ltok := strings.ToLower(tok)
			switch {
			case isRepeatToken(ltok):
				repeatToks = append(repeatToks, ltok)
			case isBoxToken(ltok):
				boxToks = append(boxToks, ltok)
			case ltok == "scroll" || ltok == "fixed" || ltok == "local":
				// background-attachment：只实现 scroll，其余按 scroll 收下
				// （不做 fixed 的「相对视口定位」口径）。
			case ltok == "none":
				// <image> 的 none：没有图片（parseLayerImage 已把 url 取走，
				// none 只是把这一层的图片置空）。
			case positionKeyword(ltok) != bgKeyNone:
				posToks = append(posToks, ltok)
			default:
				if c := parseColor(tok); c != nil {
					// CSS：背景色属于**最后一层**（Backgrounds 3 §3.9）。
					// 多层简写同时带颜色极罕见，这里按「后出现的颜色覆盖前面的」
					// 处理——比「取第一个」更贴近规范。
					if strings.EqualFold(strings.TrimSpace(tok), "transparent") {
						bg = nil // 与 background-color 长写同口径：透明
					} else {
						bg = c
					}
					continue
				}
				if _, err := parseBackgroundLen(ltok); err == nil {
					posToks = append(posToks, ltok)
					continue
				}
				return nil, nil, false // 认不出来的 token：整条声明非法
			}
		}
		if len(repeatToks) > 0 {
			m, err := parseBackgroundRepeatValue(strings.Join(repeatToks, " "))
			if err != nil {
				return nil, nil, false
			}
			setBackgroundRepeat(&l, m)
		}
		// CSS：一个 box 值同时设 origin 与 clip；两个值前者 origin、后者 clip。
		if len(boxToks) > 0 {
			origin, err := parseBackgroundBoxValue(boxToks[0], true)
			if err != nil {
				return nil, nil, false
			}
			setBackgroundOrigin(&l, origin)
			clipSrc := boxToks[0]
			if len(boxToks) > 1 {
				clipSrc = boxToks[1]
			}
			clip, err := parseBackgroundBoxValue(clipSrc, false)
			if err != nil {
				return nil, nil, false
			}
			setBackgroundClip(&l, clip)
		}
		if len(posToks) > 0 {
			x, y, err := parseBackgroundPositionValue(strings.Join(posToks, " "))
			if err != nil {
				return nil, nil, false
			}
			l.posX, l.posY = x, y
		}
		layers = append(layers, l)
	}
	if len(layers) == 0 {
		return nil, nil, false
	}
	return layers, bg, true
}

// splitBackgroundTokens 按空白切分简写一层的 token，并把斜杠单独切出来。
//
// 与 splitValueTokens 的差别只有一处：`<position> / <bg-size>` 的斜杠两侧
// 空白都是可选的（CSS Backgrounds 3 §3.10），`50%/cover` 要切成三段，而
// `url(a/b.png)` 里的斜杠已经在 stripURLFunctions 里随函数一起摘掉了。
// 括号内一律不切（rgba(0, 0, 0, .3) 仍是一个 token）。
func splitBackgroundTokens(s string) []string {
	var out []string
	depth := 0
	start := -1
	flush := func(end int) {
		if start >= 0 {
			if tok := s[start:end]; tok != "" {
				out = append(out, tok)
			}
			start = -1
		}
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case depth == 0 && (c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r'):
			flush(i)
			if c == '/' {
				out = append(out, "/")
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	flush(len(s))
	return out
}

// splitBackgroundSize 按斜杠把一层的 token 切成 size 段与其余段。
//
// 斜杠两侧的空白都是可选的（`50%/cover` 与 `50% / cover` 同义），所以只能
// 按 token 找 "/"，不能按空白切原始字符串。斜杠前是 position 的分量，斜杠后
// 最多两个 token 是 size，再往后的（no-repeat 那类）退回其余段。
func splitBackgroundSize(toks []string) (size, rest []string, bad bool) {
	slash := -1
	for i, tok := range toks {
		if tok == "/" {
			if slash >= 0 {
				return nil, nil, true // 两个斜杠：非法
			}
			slash = i
		}
	}
	if slash < 0 {
		return nil, toks, false
	}
	rest = append(rest, toks[:slash]...)
	tail := toks[slash+1:]
	for len(tail) > 0 && len(size) < 2 {
		tok := tail[0]
		ltok := strings.ToLower(tok)
		if ltok != "cover" && ltok != "contain" {
			if _, err := parseSizeAxis(ltok); err != nil {
				break // size 段到此为止，剩下的按其它类别继续分
			}
		}
		size = append(size, ltok)
		tail = tail[1:]
	}
	if len(size) == 0 {
		return nil, nil, true // 斜杠后面一个合法尺寸都没有
	}
	rest = append(rest, tail...)
	return size, rest, false
}

// parseLayerImage 解析一层里的 <image>：至多一个 url()，none 是空引用。
// bad=true 表示有未实现的 <image> 形态（渐变之类别名）。
//
// 只按**镜像函数名单**判定，不能见括号就拒：`background: rgba(0,0,0,.3)`
// 里的 rgba( 是颜色，一并拒掉会让整条声明失效（snip 的选区暗化层就是这么
// 丢的）。名单外的未知函数按非法处理——CSS 的 <image> 新增类型本来就要求
// 老引擎整条忽略。
func parseLayerImage(part string) (ref string, bad bool) {
	low := strings.ToLower(part)
	for {
		i := strings.Index(low, "url(")
		if i < 0 {
			break
		}
		r, ok := parseURLToken(part[i:])
		if !ok || r == "" || ref != "" {
			return "", true
		}
		ref = r
		part, low = part[i+4:], low[i+4:]
	}
	if hasImageFunction(low) {
		return "", true
	}
	return ref, false
}

// imageFunctions 是引擎**不支持**的 <image> 函数名（url 之外）。
var imageFunctions = []string{
	"linear-gradient", "radial-gradient", "conic-gradient",
	"repeating-linear-gradient", "repeating-radial-gradient", "repeating-conic-gradient",
	"cross-fade", "element(", "image-set", "-webkit-gradient", "-webkit-image-set",
}

// hasImageFunction 判断剩余文本里是否还有未实现的 <image> 函数。
func hasImageFunction(s string) bool {
	for _, name := range imageFunctions {
		if strings.Contains(s, name) {
			return true
		}
	}
	// 其它带括号的函数：认不出来就按非法（颜色函数除外——它们在下面的
	// 名单里，由 classify 阶段交给 parseColor）。
	for i := 0; i < len(s); i++ {
		if s[i] != '(' {
			continue
		}
		j := i - 1
		for j >= 0 && isIdentByte(s[j]) {
			j--
		}
		switch strings.ToLower(s[j+1 : i]) {
		case "rgb", "rgba", "hsl", "hsla", "color", "lab", "lch", "oklab", "oklch", "":
			// 颜色函数或非函数（孤立的 '('）：交给 parseColor 判定。
		default:
			return true
		}
	}
	return false
}

// isIdentByte 判断字节是否属于 CSS 标识符（字母/数字/-/_）。
func isIdentByte(c byte) bool {
	return c == '-' || c == '_' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isRepeatToken(tok string) bool {
	_, err := repeatAxis(strings.ToLower(tok))
	return err == nil
}

func isBoxToken(tok string) bool {
	_, err := parseBackgroundBoxValue(strings.ToLower(tok), false)
	return err == nil
}

// ---------------------------------------------------------------------------
// 绑定与绘制
// ---------------------------------------------------------------------------

// bindBackgroundImages 把图层引用解析成绝对地址，从文档图片缓存取位图。
//
// 只在绘制期做（级联期拿不到文档 BaseURL，见 backgroundLayer.src 的说明）：
// 已绑定的层直接跳过，所以静止页面里每层的缓存查询只发生一次。尚未登记的
// 地址在这里触发异步加载，与 <img> 同一通路；到货后由 drainBackgroundImages
// 记一笔**重绘级**变更（背景不参与布局，不该付整树重排的钱）。
func bindBackgroundImages(base *htmlElement, layers []backgroundLayer) {
	var doc *htmlDocument
	queried := false
	for i := range layers {
		l := &layers[i]
		if l.src == "" {
			l.bitmap = nil
			continue
		}
		if l.bitmap != nil {
			continue
		}
		if !queried {
			doc, _ = docOf(base.self).(*htmlDocument)
			queried = true
		}
		if doc == nil {
			continue
		}
		l.bitmap = doc.imageForBackground(resolveURL(doc.baseURL, l.src))
	}
}

// paintBackgroundImages 绘制元素的背景图层，返回是否真的落了墨（决定要不要
// 补圆角角标）。
//
// 每层依次：定位盒 = background-origin，裁剪盒 = background-clip，尺寸按
// background-size（cover/contain/长度/百分比/auto 与固有尺寸），位置按
// background-position（关键字/长度/百分比/边偏移），平铺按 background-repeat。
// CSS 的图层列表首层在最上面，切片顺序即 CSS 顺序，所以倒序绘制。
func paintBackgroundImages(g Graphics, base *htmlElement, comp CSSStyleDeclaration,
	bx, by, bw, bh, r int, pd, bd edges) bool {
	if comp == nil {
		return false
	}
	cs, ok := comp.(*cssStyleDeclaration)
	if !ok || len(cs.background) == 0 {
		return false
	}
	layers := cs.background
	bindBackgroundImages(base, layers)
	painted := false
	for i := len(layers) - 1; i >= 0; i-- {
		if paintBackgroundLayer(g, &layers[i], bx, by, bw, bh, pd, bd) {
			painted = true
		}
	}
	if painted && r > 0 {
		// 圆角：引擎的裁剪栈只有矩形，图片会露在圆角外。沿用既有近似——
		// 画完后用**元素背后的底色**盖掉四角（见 clipCornerMask）。
		maskBackgroundCorners(g, base, bx, by, bw, bh, r)
	}
	return painted
}

// paintBackgroundLayer 绘制一层背景图，返回是否落了墨。
func paintBackgroundLayer(g Graphics, l *backgroundLayer,
	bx, by, bw, bh int, pd, bd edges) bool {
	if l.bitmap == nil || bw <= 0 || bh <= 0 {
		return false
	}
	bmp := l.bitmap
	repeat, sizeMode, clip := backgroundKeyOf(l)
	ax, ay, aw, ah := backgroundBoxRect(l.originBox(), bx, by, bw, bh, pd, bd)
	if aw <= 0 || ah <= 0 {
		return false
	}
	cx, cy, cw, ch := backgroundBoxRect(clipBoxToOrigin(clip), bx, by, bw, bh, pd, bd)
	if cw <= 0 || ch <= 0 {
		return false
	}
	tw, th := l.imageSize(bmp, aw, ah, sizeMode)
	if tw <= 0 || th <= 0 {
		return false
	}
	// 平铺起点：定位盒 + position。不铺时超出定位盒的部分仍要画（CSS 的定位盒
	// 只是原点的参照系，画面边界由裁剪盒决定）。
	ox, oy := l.imageOffset(aw, ah, tw, th)
	startX, startY := ax+ox, ay+oy
	repeatX := repeat == bgRepeatRepeat || repeat == bgRepeatX
	repeatY := repeat == bgRepeatRepeat || repeat == bgRepeatY

	xList := tileStarts(startX, tw, cx, cw, repeatX)
	yList := tileStarts(startY, th, cy, ch, repeatY)
	if len(xList) == 0 || len(yList) == 0 {
		return false
	}
	clipped := pushClip(g, cx, cy, cw, ch)
	for _, ty := range yList {
		for _, tx := range xList {
			paintBackgroundTile(g, tx, ty, tw, th, bmp, cx, cy, cw, ch, clipped)
		}
	}
	if clipped {
		popClip(g)
	}
	return true
}

// clipBoxToOrigin 把 clip 口径换算成 backgroundBoxRect 认识的常量组。
func clipBoxToOrigin(clip uint8) uint8 {
	switch clip {
	case bgClipPadding:
		return bgOriginPadding
	case bgClipContent:
		return bgOriginContent
	}
	return bgOriginBorder
}

// backgroundBoxRect 解析某个盒子口径对应的矩形。
func backgroundBoxRect(box uint8, bx, by, bw, bh int, pd, bd edges) (x, y, w, h int) {
	switch box {
	case bgOriginBorder:
		return bx, by, bw, bh
	case bgOriginContent:
		return boxInset(bx, by, bw, bh, bd, pd)
	}
	return boxInset(bx, by, bw, bh, bd, edges{})
}

// boxInset 从边框盒往里缩：先缩边框，再缩 padding。
func boxInset(bx, by, bw, bh int, bd, pd edges) (x, y, w, h int) {
	x = bx + bd[eLeft] + pd[eLeft]
	y = by + bd[eTop] + pd[eTop]
	w = bw - bd[eLeft] - bd[eRight] - pd[eLeft] - pd[eRight]
	h = bh - bd[eTop] - bd[eBottom] - pd[eTop] - pd[eBottom]
	return
}

// tileStarts 返回某一轴上要画的瓦片左上角坐标列表。
//
// repeat=false 时只有一块（起点就是 position 算出的原点，即使它落在裁剪区
// 外——越界部分由裁剪负责）。repeat=true 时按平铺网格向两侧扩展，只保留与
// 裁剪区相交的块。
func tileStarts(start, size, clipX, clipW int, repeat bool) []int {
	if size <= 0 || clipW <= 0 {
		return nil
	}
	if !repeat {
		return []int{start}
	}
	lo, hi := clipX, clipX+clipW
	first := floorDiv(lo-start, size)
	last := floorDiv(hi-1-start, size)
	if last < first {
		return nil
	}
	n := last - first + 1
	if n > maxBackgroundTiles {
		n = maxBackgroundTiles
	}
	out := make([]int, n)
	for i := range out {
		out[i] = start + (first+i)*size
	}
	return out
}

// paintBackgroundTile 画一块瓦片：只落出与裁剪矩形相交的像素。
//
// 后端实现 ImageBlitter 时把可见子矩形映射到源图的对应子块（边上一块只画
// 半张图，与浏览器一致，也不依赖裁剪栈）；只有 DrawImage 的后端退化成整图
// 缩放到完整瓦片——裁剪栈生效时结果相同，后端不支持裁剪时这块干脆不画
// （宁可缺一角，也不能让半张图错位铺在盒外）。
func paintBackgroundTile(g Graphics, tx, ty, tw, th int, bmp *Bitmap,
	cx, cy, cw, ch int, clipped bool) {
	dx, dy := maxInt(tx, cx), maxInt(ty, cy)
	dxe, dye := minInt(tx+tw, cx+cw), minInt(ty+th, cy+ch)
	if dxe <= dx || dye <= dy {
		return
	}
	if bl, ok := g.(ImageBlitter); ok {
		sx := (dx - tx) * bmp.W / tw
		sw := maxInt((dxe-dx)*bmp.W/tw, 1)
		if sx+sw > bmp.W {
			sw = bmp.W - sx
		}
		sy := (dy - ty) * bmp.H / th
		sh := maxInt((dye-dy)*bmp.H/th, 1)
		if sy+sh > bmp.H {
			sh = bmp.H - sy
		}
		if sw <= 0 || sh <= 0 {
			return
		}
		bl.BlitImage(dx, dy, dxe-dx, dye-dy, bmp, sx, sy, sw, sh)
		return
	}
	if !clipped && (dx != tx || dy != ty || dxe != tx+tw || dye != ty+th) {
		return
	}
	g.DrawImage(tx, ty, tw, th, bmp)
}

// maskBackgroundCorners 用背后底色盖掉圆角边框盒四角「圆角之外」的部分。
func maskBackgroundCorners(g Graphics, base *htmlElement, bx, by, bw, bh, r int) {
	sd, ok := g.(SpriteDrawer)
	if !ok || r <= 0 || bw <= 0 || bh <= 0 {
		return
	}
	bg := behindBackground(base.self)
	if bg == nil {
		return // 背后底色未知：宁可不动（保持方角）
	}
	if lim := minInt(bw, bh) / 2; r > lim {
		r = lim
	}
	if r <= 0 {
		return
	}
	sd.DrawSprite(bx, by, cornerMaskSprite(r, bg, false, false))
	sd.DrawSprite(bx+bw-r, by, cornerMaskSprite(r, bg, true, false))
	sd.DrawSprite(bx, by+bh-r, cornerMaskSprite(r, bg, false, true))
	sd.DrawSprite(bx+bw-r, by+bh-r, cornerMaskSprite(r, bg, true, true))
}

// imageSize 算出这层图片的绘制尺寸（CSS background-size）。
//
// 口径（CSS Images 3 §7.2 的简化版）：
//   - cover/contain：按固有宽高比缩放，分别取「填满定位盒」「装进定位盒」；
//   - 两轴都给：各自按长度/百分比（百分比相对定位盒）；
//   - 只给一轴：另一轴按固有宽高比推；
//   - 都不给：固有尺寸。
func (l *backgroundLayer) imageSize(bmp *Bitmap, areaW, areaH int, sizeMode uint8) (w, h int) {
	natW, natH := bmp.W, bmp.H
	if natW <= 0 || natH <= 0 {
		return 0, 0
	}
	if sizeMode != bgSizeAuto {
		scaleW, scaleH := float64(areaW)/float64(natW), float64(areaH)/float64(natH)
		scale := scaleW
		if sizeMode == bgSizeCover {
			if scaleH > scaleW {
				scale = scaleH
			}
		} else if scaleH < scaleW {
			scale = scaleH
		}
		return roundLen(scale * float64(natW)), roundLen(scale * float64(natH))
	}
	w = resolveSizeAxis(l.sizeW, areaW)
	h = resolveSizeAxis(l.sizeH, areaH)
	switch {
	case w > 0 && h > 0:
	case w > 0:
		h = roundLen(float64(w) * float64(natH) / float64(natW))
	case h > 0:
		w = roundLen(float64(h) * float64(natW) / float64(natH))
	default:
		w, h = natW, natH
	}
	return w, h
}

// resolveSizeAxis 解析 size 的一个轴为像素；auto/非法返回 0。
func resolveSizeAxis(v backgroundLen, area int) int {
	if v.kind != bgLenVal {
		return 0
	}
	if v.unit == bgUnitPct {
		return roundLen(float64(area) * float64(v.val) / 100)
	}
	return roundLen(float64(v.val))
}

// imageOffset 算出第一块瓦片相对定位盒的偏移（CSS background-position）。
//
// 百分比的基数是「定位盒尺寸 − 图片尺寸」（所以 50% 就是居中）；关键字
// 同理换算；边偏移从指定的那条边往里缩。
func (l *backgroundLayer) imageOffset(areaW, areaH, imgW, imgH int) (x, y int) {
	return backgroundAxisOffset(l.posX, areaW, imgW),
		backgroundAxisOffset(l.posY, areaH, imgH)
}

func backgroundAxisOffset(v backgroundLen, area, img int) int {
	free := area - img
	switch v.kind {
	case bgLenVal:
		if v.unit == bgUnitPct {
			return roundLen(float64(free) * float64(v.val) / 100)
		}
		return roundLen(float64(v.val))
	case bgLenEdge:
		off := roundLen(float64(v.val))
		if v.unit == bgUnitPct {
			off = roundLen(float64(free) * float64(v.val) / 100)
		}
		if v.key == bgKeyRight || v.key == bgKeyBottom {
			return free - off
		}
		return off
	case bgLenKey:
		switch v.key {
		case bgKeyCenter:
			return free / 2
		case bgKeyRight, bgKeyBottom:
			return free
		}
	}
	// auto / 未声明：CSS 初始值 0% 0%。
	return 0
}

// roundLen 把浮点像素落成整数：CSS 口径是「加 0.5 后向下取整」（对负数
// 同样成立，-10.5 → -10），不是 math.Round 的「远离 0」。
func roundLen(v float64) int { return int(math.Floor(v + 0.5)) }
