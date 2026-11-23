// 本文件集中放调试服务的 JSON 外壳与机器可读的帮助表。
//
// 字段命名按「外部脚本想读什么」定，不是引擎内部字段名的直译：
// changeCount/paintCount 是引擎口径（重排档/重绘档计数），box/text/value
// 是给脚本断言用的几何与文本。加字段可以，改名要走 CHANGELOG。

package debug

// box 是一个矩形（x/y 是左上角，w/h 是宽高），或一组位置数值（滚动偏移
// 用它的 x/y）。
//
// 字段**不带 omitempty**：几何里的 0 是真值（元素贴左边、页面没滚动就是
// x=0/scroll=0），省掉会让脚本读到 undefined 而不是 0。整个对象想省略时
// 用指针（见 elemInfo.Scroll / docInfo.Scroll）。
type box struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// scrollBox 是滚动偏移：只有 x/y 两个量，别混进矩形的 w/h（它们对偏移没有
// 意义，读的人还要猜「w=0 是什么意思」）。
type scrollBox struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// elemInfo 是一个元素的摘要：够定位「这是谁」，不含整棵子树。
type elemInfo struct {
	Tag   string `json:"tag"`
	ID    string `json:"id,omitempty"`
	Class string `json:"class,omitempty"`
	// Selector 是一个可直接喂给 /debug/dom?selector= 的猜测选择器
	// （#id 优先，其次 tag.class）。不保证唯一。
	Selector string `json:"selector,omitempty"`
	Box      box    `json:"box"`
	// Scroll 是元素自身的滚动偏移（只非零时给）：回答「滚轮到底滚了谁、
	// 滚了多少」。文档级滚动看 /debug/state 的 document.scroll，
	// 容器滚动看这里（engine.ScrollOffset）。
	Scroll *scrollBox `json:"scroll,omitempty"`
	// Text 是元素**自身**的文本（子树的文本走 /debug/text）。
	Text string `json:"text,omitempty"`
	// Value 是表单控件的当前值（engine.GetValue），非控件为空。
	Value string `json:"value,omitempty"`

	// 命中链上附带的伪类状态（只有 /debug/hit 会填）。
	Hovered bool `json:"hovered,omitempty"`
	Focused bool `json:"focused,omitempty"`
	Active  bool `json:"active,omitempty"`
}

// docInfo 是文档级的观测项。
type docInfo struct {
	Title       string `json:"title"`
	ChangeCount int64  `json:"changeCount"`
	PaintCount  int64  `json:"paintCount"`
	// ScrollAnimating 为 true 表示还有滚动动画在推进（主循环不会空闲休眠）。
	ScrollAnimating bool `json:"scrollAnimating"`
	// TooltipPending 为 true 表示有提示气泡在计时。
	TooltipPending bool `json:"tooltipPending"`
	// Scroll 是文档根（页面级）的滚动偏移。
	Scroll *scrollBox `json:"scroll,omitempty"`
}

// selectionInfo 是当前选区（页面文本或控件内的文本选中）。
type selectionInfo struct {
	Text   string `json:"text"`
	Length int    `json:"length"`
}

// stateResp 是 GET /debug/state 的响应。
type stateResp struct {
	OK      bool `json:"ok"`
	Running bool `json:"running"`
	// Viewport 是视口尺寸（w/h，像素）。
	Viewport  box            `json:"viewport"`
	Document  *docInfo       `json:"document,omitempty"`
	Hover     *elemInfo      `json:"hover,omitempty"`
	Focus     *elemInfo      `json:"focus,omitempty"`
	Active    *elemInfo      `json:"active,omitempty"`
	Selection *selectionInfo `json:"selection,omitempty"`
}

// hitResp 是 GET /debug/hit 的响应。
type hitResp struct {
	OK bool `json:"ok"`
	X  int  `json:"x"`
	Y  int  `json:"y"`
	// Chain 是自下而上的命中链：Chain[0] 是最深的那个元素（真正收到点击的），
	// 末尾接近根。没有命中时为空——「点了没反应」的第一条线索。
	Chain     []elemInfo `json:"chain"`
	Cursor    string     `json:"cursor,omitempty"`
	Truncated bool       `json:"truncated,omitempty"`
}

// textResp 是 GET /debug/text 的响应。
type textResp struct {
	OK       bool   `json:"ok"`
	Selector string `json:"selector"`
	Tag      string `json:"tag"`
	// Text 是子树的可见文本（engine.HTMLElement.InnerText）。
	Text string `json:"text"`
	// Value 是表单控件的当前值。
	Value string `json:"value,omitempty"`
}

// attrResp 是 GET /debug/attr 的响应。
type attrResp struct {
	OK       bool   `json:"ok"`
	Selector string `json:"selector"`
	Name     string `json:"name"`
	Value    string `json:"value"`
}

// domNode 是 DOM 树快照里的一个节点。
type domNode struct {
	Tag   string `json:"tag"`
	ID    string `json:"id,omitempty"`
	Class string `json:"class,omitempty"`
	Box   box    `json:"box"`
	// Scroll 是元素自身的滚动偏移（只非零时给）。
	Scroll *scrollBox `json:"scroll,omitempty"`
	// Attrs 只含白名单属性（引擎没有属性枚举接口，见 buildDOM 说明）。
	Attrs map[string]string `json:"attrs,omitempty"`
	Text  string            `json:"text,omitempty"`
	Value string            `json:"value,omitempty"`
	// Truncated 挂在这个节点上表示它的子树被深度或条数上限切掉了。
	Truncated bool      `json:"truncated,omitempty"`
	Children  []domNode `json:"children,omitempty"`
}

// domResp 是 GET /debug/dom 的响应。
type domResp struct {
	OK        bool    `json:"ok"`
	Selector  string  `json:"selector,omitempty"`
	Count     int     `json:"count"`
	Truncated bool    `json:"truncated,omitempty"`
	Tree      domNode `json:"tree"`
}

// docReq 是 POST /debug/doc 的请求体。
type docReq struct {
	HTML string `json:"html"`
	Path string `json:"path"`
}

// inputReq 是 POST /debug/input 的请求体（也接受裸数组形式，见 parseActions）。
type inputReq struct {
	Actions []action `json:"actions"`
	// ContinueOnError 为 true 时不中断，把每个失败的动作都报出来。
	// 默认 false：脚本跑到第一步就错，后面的动作只会制造无关噪音。
	ContinueOnError bool `json:"continue_on_error"`
	// Sync 为 true（默认）时每个动作后插一道主循环栅栏，保证下一个动作
	// 看到的是上一个动作落地后的状态。设 false 换吞吐（一次性灌几千个
	// 滚轮事件），此后自己用 wait 对齐。
	Sync *bool `json:"sync"`
}

// action 是一个输入动作。省略的数值字段按「保持上一个值」解释（见 input.go）。
type action struct {
	Op string `json:"op"`

	X  *int `json:"x,omitempty"`
	Y  *int `json:"y,omitempty"`
	X2 *int `json:"x2,omitempty"`
	Y2 *int `json:"y2,omitempty"`

	Button *string `json:"button,omitempty"` // left / middle / right
	Steps  *int    `json:"steps,omitempty"`  // drag 的中间移动帧数

	DX *int `json:"dx,omitempty"` // 滚轮档位（SDL 口径：正 = 向上/向右）
	DY *int `json:"dy,omitempty"`

	Key  string `json:"key,omitempty"`  // "ctrl+shift+s" / "enter" / "F5"
	Code *int   `json:"code,omitempty"` // 直接给 SDL 键码
	Mods *int   `json:"mods,omitempty"` // 直接给修饰键位掩码

	Text string `json:"text,omitempty"` // text 动作的字符串（s 是同义别名）
	S    string `json:"s,omitempty"`

	MS *int `json:"ms,omitempty"` // wait 的毫秒

	Files []string `json:"files,omitempty"` // drop 的绝对路径
	HTML  string   `json:"html,omitempty"`  // doc
	Path  string   `json:"path,omitempty"`  // doc / shot 的输出文件
	Sel   string   `json:"selector,omitempty"`
	W     *int     `json:"w,omitempty"` // resize
	H     *int     `json:"h,omitempty"`

	// 观测类动作（hit/state/text）的返回值会进响应的 results。
}

// actionResult 是单个动作的执行记录（失败时才带 error）。
type actionResult struct {
	Index int    `json:"index"`
	Op    string `json:"op"`
	Err   string `json:"error,omitempty"`
	// Value 是观测类动作（state/hit/text/dom）的返回值。
	Value any `json:"value,omitempty"`
}

// inputResp 是 POST /debug/input 的响应。
type inputResp struct {
	OK       bool           `json:"ok"`
	Executed int            `json:"executed"`
	Total    int            `json:"total"`
	Results  []actionResult `json:"results,omitempty"`
}

// ===== 帮助表（/debug/help 与首页共用一份，改端点不必改两处）=====

type helpEndpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Desc   string `json:"desc"`
}

type helpAction struct {
	Op   string `json:"op"`
	Args string `json:"args"`
	Desc string `json:"desc"`
}

// helpResp 是 GET /debug/help 的响应。
type helpResp struct {
	OK        bool           `json:"ok"`
	Endpoints []helpEndpoint `json:"endpoints"`
	Actions   []helpAction   `json:"actions"`
	Notes     []string       `json:"notes"`
}

var helpEndpoints = []helpEndpoint{
	{"GET", "/debug/state", "窗口与文档状态：视口、标题、变更计数、悬停/焦点/按下元素、选区"},
	{"GET", "/debug/hit?x=&y=", "命中测试：点上的元素链（自下而上）+ 伪类状态 + 光标形状"},
	{"GET", "/debug/dom?selector=&depth=&limit=", "DOM 树快照（属性白名单 + 盒 + 文本）"},
	{"GET", "/debug/text?selector=", "元素的可见文本（InnerText）与表单取值"},
	{"GET", "/debug/attr?selector=&name=", "元素的单个属性值（任意属性名）"},
	{"GET", "/debug/shot.png", "当前画面 PNG"},
	{"GET", "/debug/layout.svg", "当前布局 SVG（与 -dump-svg 同源）"},
	{"POST", "/debug/doc", "{\"html\":\"...\"} 或 {\"path\":\"...\"} 换文档"},
	{"POST", "/debug/input", "执行一串输入动作；body 是动作数组或 {\"actions\":[...]}"},
	{"GET", "/debug/help", "本清单"},
}

var helpActions = []helpAction{
	{"move", "x, y", "鼠标移动（更新 :hover 与光标）"},
	{"down / up", "x?, y?, button?", "按下 / 抬起；button = left|middle|right"},
	{"click", "x?, y?, button?", "move + down + up（一次点击）"},
	{"drag", "x, y, x2, y2, steps?", "按下后分 steps 帧移动到 (x2,y2) 再抬起，走真实拖选阈值"},
	{"wheel", "x?, y?, dx?, dy?", "滚轮档位；SDL 口径：dy 正 = 滚轮向上（页面往回滚，露出更靠上的内容）"},
	{"key", "key | code+mods", "按键：\"ctrl+shift+s\"、\"enter\"、\"F5\"；也可直接给 SDL 键码"},
	{"text", "text（或 s）", "文本输入（与 IME 上屏同一路径，中文可用）"},
	{"drop", "files[]", "拖入窗口的文件（触发 SetFileDropHandler）"},
	{"wait", "ms", "真实睡眠：让滚动动画/气泡计时推进（不是帧数）"},
	{"sync", "—", "只做一次主循环栅栏（通常不必自己写，sync=true 已自动插）"},
	{"state / hit / text / dom", "x?, y?, selector?", "在脚本里读观测值，结果进响应的 results"},
	{"doc", "html | path", "换文档"},
	{"resize", "w, h", "改窗口尺寸（强制重排）"},
	{"shot", "path?", "截图；给了 path 落盘，否则只回字节数"},
	{"close", "—", "请求关闭窗口（Run 正常返回）"},
}

// 观测上限：42 万元素页面一次全量序列化能把连接撑爆，
// 所以深度和条数都有默认上限，读的人自己用 selector 收窄。
const (
	defaultDepth     = 6
	defaultNodeLimit = 2000
	maxChain         = 64
)

// textClip 是摘要里文本的截断长度（rune）。
const textClip = 80
