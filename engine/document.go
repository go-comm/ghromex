package engine

import "strings"

// HTMLDocument 是文档根节点，持有样式表与所属视口。
type HTMLDocument interface {
	HTMLElement

	CreateElement(tag string) HTMLElement
	GetElementById(id string) HTMLElement
	GetElementsByName(name string) []HTMLElement

	Body() HTMLBodyElement
	Head() HTMLHeadElement

	// Stylesheet 返回文档 <style> 编译出的样式表。
	Stylesheet() *Stylesheet
	// AddStylesheetText 解析并合并一段 CSS（其中的 @import 会被加载）。
	AddStylesheetText(css string)
	// AddStylesheetLink 解析并合并 <link rel="stylesheet" href> 引用的样式表。
	// 解析器遇到样式表链接时调用；运行期想动态挂样式表也可直接调它。
	AddStylesheetLink(href string)
	SetStylesheet(sheet *Stylesheet)
	// SetStylesheetLoader 安装外部样式表加载器；nil 关闭外部样式表
	// （<link rel="stylesheet"> 与 @import 都被忽略，其余照常）。
	SetStylesheetLoader(f StylesheetLoader)
	// SetBaseURL 设置文档地址：<link> 与第一层 @import 的相对引用按它解析。
	// 嵌套 @import 的基准是上一层样式表的地址，由引擎自动接续。
	SetBaseURL(base string)

	// Title 返回 head > title 的文本。
	Title() string

	Viewport() Viewport
	// Refresh 基于当前视口重新执行级联与布局。
	Refresh()
	// DumpSVG 将当前布局导出为 SVG 文本（调试/无头验证用）。
	DumpSVG() string

	// SetOnChanged 注册文档内容变化回调（布局失效通知，供后端重排）。
	SetOnChanged(f func())
	// SetOnNavigate 注册链接导航回调：点击带 href 的 <a> 时以 href 调用。
	// 引擎不内置多文档/外跳，跳转策略由应用决定；f 为 nil 时移除。
	SetOnNavigate(f func(href string))
	// ChangeCount 返回内容变化计数，后端据此判断是否需要重排。
	ChangeCount() int64

	Close()
}

// StylesheetLoader 加载一份外部样式表，返回 CSS 文本；失败返回 error。
//
// href 是**已解析成绝对地址**的引用（见 resolveURL）：<link> 按文档的
// BaseURL 解析，@import 按上一层样式表的地址解析，所以加载器拿到的永远
// 是可以直接用的地址，不用自己再拼相对路径。
//
// 加载是**同步**的：在解析 <link>/@import 时当场调用，跑在打开文档的那个
// goroutine（浏览器的 UI 线程）。要联网的宿主应在加载器里做缓存或预取，
// 别在这里等一个慢请求——引擎没有「样式到货后再重排」的链路。
//
// 返回 error 时该样式表被忽略（页面继续渲染，其余样式照常生效），错误
// 由宿主自己上报——引擎不打印、不弹窗：缺一份 CSS 不该让页面打不开。
type StylesheetLoader func(href string) (string, error)

// DocumentOptions 是 OpenDocumentWith 的可选能力。零值等同于 OpenDocument：
// 不加载外部样式表。
type DocumentOptions struct {
	// BaseURL 是文档地址：<link rel="stylesheet" href> 的相对引用按它解析，
	// 也是第一层 @import 的引用方。按 URL 口径用 `/` 分隔；本地文件先转成
	// file:// 地址（见 examples/browser 的 fileURL），空串表示没有基准
	// （此时相对引用原样交给加载器）。
	BaseURL string
	// LoadStylesheet 非 nil 时启用外部样式表：<link rel="stylesheet" href>
	// 与 CSS 的 @import 都会经它加载。
	LoadStylesheet StylesheetLoader
}

func NewDocument() HTMLDocument {
	document := &htmlDocument{}
	document.HTMLElement = newElement("html")
	document.setPaintBase(NewPaint())
	document.head = newHeadElement()
	document.body = newBodyElement()

	// 根节点 self 指向文档包装器：子节点父链上溯到根后可命中 HTMLDocument，
	// 变更通知（markChanged）与事件冒泡依赖该链路。
	if root, ok := document.HTMLElement.(*htmlElement); ok {
		root.self = document
	}
	document.AppendChild(document.head, document.body)

	return document
}

type htmlDocument struct {
	HTMLElement

	viewport Viewport
	sheet    *Stylesheet
	// baseURL / loadSheet 是外部样式表的接线：baseURL 是文档地址（相对
	// href 的基准），loadSheet 由宿主提供（引擎不碰 I/O）。两者都由
	// OpenDocumentWith 在解析前装好——<link>/@import 是解析阶段就加载的。
	baseURL   string
	loadSheet StylesheetLoader
	head      HTMLHeadElement
	body      HTMLBodyElement
	focus     HTMLElement // 当前聚焦的元素（可编辑输入、按钮、链接等），nil 无焦点
	// hover 是鼠标当前悬停的元素（:hover 规则的依据），active 是鼠标
	// 按下中的元素（:active 规则）。两者变化都会改样式，故与 focus 一样
	// 走 markChanged → 重排重绘 → 级联重算。
	hover  HTMLElement
	active HTMLElement
	// tooltip 提示气泡状态：target/text 是上一帧解析出的提示元素与文案
	// （从 hover 沿父链取最近非空 title），sinceNs 是当前悬停的起算时间
	// （目标或文案变化就重新计时），mouseX/Y 是鼠标最后位置（气泡锚点，
	// 弹出后不再跟手），shown 表示气泡是否弹出中，dismissed 表示「被
	// 按下/点击/滚动收掉、鼠标还没换目标」（不追回，浏览器同行为）。
	// 推进与绘制见 tooltip.go。
	tooltipTarget    HTMLElement
	tooltipText      string
	tooltipSinceNs   int64
	tooltipMouseX    int
	tooltipMouseY    int
	tooltipShown     bool
	tooltipDismissed bool
	// tooltipColors 是应用下发的气泡配色（空字段用引擎默认）：气泡是绘制
	// 层、拿不到级联样式，跟随主题只能靠显式下发（见 SetTooltipColors）。
	tooltipColors TooltipColors
	// dragInput 是鼠标按下时命中的可编辑控件。拖选期间活动端可移出控件
	// 甚至窗口，仍应把坐标反推为同一控件的光标位置（浏览器同语义）；
	// 因此不能在每次 MouseMotion 时重新命中测试。鼠标抬起即清空。
	dragInput *htmlElement
	// dragScroll 是按在 textarea 覆盖滚动条上时的控件（与 dragInput 互斥）：
	// 拖动期间指针 y 映射为滚动偏移（见 textareaScrollTo），抬起即清空。
	dragScroll *htmlElement
	// dragViewAxis 是按在文档级覆盖滚动条上时的拖拽轴（0=未在拖、1=纵、
	// -1=横）：拖动期间指针映射为文档级滚动（见 viewportBarScrollTo），
	// 抬起即清零。与 dragInput/dragScroll 互斥，按下时先于命中测试判掉。
	dragViewAxis int
	changed      int64
	// scrollMark 是最近一次**滚动**变更的计数（scrollBy/wheel/光标跟随等
	// 只动 scrollTop/scrollLeft 的路径），用于布局快速路径：滚动不改变盒
	// 模型与级联，只需重平移，不必每帧全量重排。lastLayoutChange 是上次
	// 全量布局时的 changed，lastAppliedScrollMark 是上次已平移到的滚动
	// 标记——相等即无新滚动可平。
	scrollMark          int64
	lastLayoutChange    int64
	lastAppliedScrollMark int64
	onChanged  func()
	onNavigate func(href string)
	// vpW/vpH 是上次布局时的视口尺寸。窗口缩放后后端只强制重排、并不
	// 改文档内容（onChanged 不触发），应用层无从得知窗口变了——而
	// 「编辑区铺满窗口剩余区域」这类 chrome 必须跟着窗口尺寸走。布局
	// 时比对尺寸，变化则补一次变更通知（见 LayoutDocument）。
	vpW, vpH int
}

func (document *htmlDocument) setPaintBase(p Paint) {
	if base := inner(document); base != nil {
		base.setPaint(p)
	}
}

func (document *htmlDocument) CreateElement(tag string) HTMLElement {
	e := SystemElements().Get(tag)
	if e == nil {
		e = CustomElements().Get(tag)
	}
	if e != nil {
		// CloneElement 对 typed nil 原型返回 nil（调 Clone 会空指针崩溃），
		// 此时退化为未知元素，不让创建流程崩掉。
		if c := CloneElement(e); c != nil {
			return c
		}
	}
	return newHTMLUnknownElement(tag)
}

func (document *htmlDocument) GetElementById(id string) HTMLElement {
	return findElementBy(document, func(e HTMLElement) bool {
		return e.GetAttribute("id") == id
	})
}

func (document *htmlDocument) GetElementsByName(name string) []HTMLElement {
	var out []HTMLElement
	for _, e := range findElementsBy(document, func(el HTMLElement) bool {
		return el.GetAttribute("name") == name
	}) {
		out = append(out, e)
	}
	return out
}

func findElementBy(root HTMLElement, pred func(HTMLElement) bool) HTMLElement {
	for _, child := range root.Children() {
		if pred(child) {
			return child
		}
		if found := findElementBy(child, pred); found != nil {
			return found
		}
	}
	return nil
}

func findElementsBy(root HTMLElement, pred func(HTMLElement) bool) []HTMLElement {
	var out []HTMLElement
	for _, child := range root.Children() {
		if pred(child) {
			out = append(out, child)
		}
		out = append(out, findElementsBy(child, pred)...)
	}
	return out
}

// docOf 沿父链找元素所属的文档：根元素的 self 指向文档包装器，普通
// 元素的 self 指向自身。找不到时返回 nil（接口 nil，调用方需判空）。
func docOf(e HTMLElement) HTMLDocument {
	for cur := e; cur != nil; cur = cur.ParentElement() {
		b := inner(cur)
		if b == nil {
			continue
		}
		if d, ok := b.self.(*htmlDocument); ok {
			return d
		}
	}
	return nil
}

func (document *htmlDocument) Head() HTMLHeadElement {
	return document.head
}

func (document *htmlDocument) Body() HTMLBodyElement {
	return document.body
}

func (document *htmlDocument) Stylesheet() *Stylesheet {
	return document.sheet
}

// maxStylesheetDepth 限制 @import 的嵌套层数。正常主题不会这么深，超过就
// 按循环/异常处理——防止 A→B→A 这种互相引用无限递归到栈溢出。
const maxStylesheetDepth = 32

func (document *htmlDocument) AddStylesheetText(css string) {
	document.addStylesheetText(css, document.baseURL, nil)
	document.markChanged()
}

// AddStylesheetLink 按文档 BaseURL 解析 href，交给加载器读回 CSS。
//
// 加载失败（文件不存在、不联网、解析错误）时**静默忽略**：一份样式表缺失
// 不该让页面打不开，其余样式照常生效。错误由加载器自己上报（它就住在宿主
// 代码里，比引擎更清楚该怎么提示）。
func (document *htmlDocument) AddStylesheetLink(href string) {
	href = strings.TrimSpace(href)
	if href == "" || document.loadSheet == nil {
		return
	}
	abs := resolveURL(document.baseURL, href)
	if abs == "" {
		return
	}
	css, err := document.loadSheet(abs)
	if err != nil {
		return
	}
	document.addStylesheetText(css, abs, []string{abs})
	document.markChanged()
}

func (document *htmlDocument) SetStylesheetLoader(f StylesheetLoader) {
	document.loadSheet = f
}

func (document *htmlDocument) SetBaseURL(base string) {
	document.baseURL = base
}

// addStylesheetText 解析 CSS 并把规则并入文档样式表。
//
// @import 按 CSS 语义**先**加载、先合并：被导入的规则落在导入语句的位置
// （即本文件规则之前），同特异度时本文件的规则胜出。嵌套 @import 的基准
// 是上一层样式表的地址（base 形参），不是页面地址——主题文件放在子目录里
// 时这一点决定了它引用的相对路径对不对。
func (document *htmlDocument) addStylesheetText(css, base string, chain []string) {
	rules, imports := parseCSSRules(css)
	for _, ref := range imports {
		if document.loadSheet == nil || len(chain) >= maxStylesheetDepth {
			continue
		}
		abs := resolveURL(base, ref)
		if abs == "" || inChain(chain, abs) {
			continue
		}
		text, err := document.loadSheet(abs)
		if err != nil {
			continue
		}
		document.addStylesheetText(text, abs, append(chain, abs))
	}
	if document.sheet == nil {
		document.sheet = &Stylesheet{}
	}
	document.sheet.Merge(&Stylesheet{rules: rules})
}

// inChain 判断 href 是否已在 @import 链上（直接或间接自引用）。
func inChain(chain []string, href string) bool {
	for _, s := range chain {
		if s == href {
			return true
		}
	}
	return false
}

func (document *htmlDocument) SetStylesheet(sheet *Stylesheet) {
	document.sheet = sheet
	document.markChanged()
}

func (document *htmlDocument) Title() string {
	if document.head == nil {
		return ""
	}
	t := document.head.Title()
	if t == nil {
		return ""
	}
	return t.Text()
}

func (document *htmlDocument) Viewport() Viewport {
	return document.viewport
}

func (document *htmlDocument) Refresh() {
	LayoutDocument(document)
}

func (document *htmlDocument) SetOnChanged(f func()) {
	document.onChanged = f
}

func (document *htmlDocument) SetOnNavigate(f func(href string)) {
	document.onNavigate = f
}

func (document *htmlDocument) ChangeCount() int64 {
	return document.changed
}

func (document *htmlDocument) markChanged() {
	document.changed++
	if document.onChanged != nil {
		document.onChanged()
	}
}

// markScrollChanged 标记一次「滚动类」变更：changed 照常 +1（宿主据此
// 重绘并调用 LayoutDocument），同时 scrollMark +1 供布局快速路径识别——
// 两次计数增量相等说明自上次全量布局以来**只有滚动**发生，级联与盒模型
// 都没变，LayoutDocument 只平移已布局的盒子，不重排（大页面滚动卡顿的
// 根因就是每档滚轮触发一次全量级联+布局）。
func (document *htmlDocument) markScrollChanged() {
	document.changed++
	document.scrollMark++
	if document.onChanged != nil {
		document.onChanged()
	}
}

func (document *htmlDocument) Close() {
	document.viewport = nil
}
