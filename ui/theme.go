// 本文件是组件库的视觉令牌与样式表生成；包文档见 doc.go。
package ui

import (
	"strconv"
	"strings"

	"github.com/go-comm/ghromex/engine"
)

// Theme 是组件库的视觉令牌：一套配色 + 一组尺寸。
//
// 为什么是「令牌 + 生成样式表」而不是让应用写 CSS：
//   - 引擎的 CSS 是子集（没有 flex/grid），工作台要用的技巧（占位块、
//     1px 分隔线、绝对定位浮层）容易写错；
//   - 主题是一处改、全部一致的：应用只写结构，不碰颜色与间距；
//   - 样式表由 Install 追加在作者样式之后，同特异度时工作台胜出，页面
//     自带的 body/div 规则不会把菜单栏挤歪。
//
// 零值字段由 withDefaults 用 Light() 补齐，应用只需改想改的几项。
type Theme struct {
	Font     string // font-family 原值
	IconFont string // 图标字体（默认 Segoe MDL2 Assets，见 icon.go）

	Bg           string // 画布底色（卡片之外的区域）
	Surface      string // 面板/栏/浮层底色
	SurfaceAlt   string // 工具栏/控件条底色（浮在 Surface 之上的一层）
	Field        string // 输入控件底色（input/select/textarea/中性按钮）：深色主题沉到画布档，一眼看出哪里能打字
	Hover        string // 悬停底色
	Press        string // 按下底色
	Border       string // 分隔线与描边
	BorderStrong string // 需要更清晰边界的地方（浮层、输入框、工具栏分隔线）
	Text         string // 主文字
	Muted        string // 次要文字/禁用态

	// 强调色（Primary）：主按钮、选中态、链接、焦点。字段名沿用 Accent
	// （旧 API），语义上它就是 Primary；Default（默认/中性）不单独存
	// 令牌，复用 Surface/Border/Text/SurfaceAlt/Press——同一组中性色
	// 只留一份，避免两处值漂移。
	Accent       string // 强调色实底（主按钮、选中态）
	AccentHover  string // 强调色悬停/按下
	OnAccent     string // 强调色实底上的文字/图标（对比度由主题负责）
	AccentSoft   string // 强调色淡底（悬停/徽标/提示）
	AccentBorder string // 强调色淡底描边
	AccentText   string // 强调色淡底上的文字

	// 语义色：每组都是「实底 + 悬停 + On（实底文字）+ 淡底 + 描边 +
	// 淡底文字」。按钮用实底/On，徽标与提示用淡底/描边/文字。
	// 浅色主题里 Warning 的 On 用深色（琥珀底压白字不够清楚）；
	// 深色主题的实底更亮，On 统一用同色系深色。
	Success       string
	SuccessHover  string
	OnSuccess     string
	SuccessSoft   string
	SuccessBorder string
	SuccessText   string

	Warning       string
	WarningHover  string
	OnWarning     string
	WarningSoft   string
	WarningBorder string
	WarningText   string

	Danger       string
	DangerHover  string
	OnDanger     string
	DangerSoft   string
	DangerBorder string
	DangerText   string

	Info       string
	InfoHover  string
	OnInfo     string
	InfoSoft   string
	InfoBorder string
	InfoText   string

	Radius           int // 面板/浮层圆角
	MenuHeight       int // 菜单栏高度
	ToolHeight       int // 工具栏高度
	StatusHeight     int // 状态栏高度
	StatusRightWidth int // 状态栏右侧固定区基准预留宽度（右钉段全定宽时按 Σ 外扩）
	ButtonHeight     int // 标准表单档控件高度（按钮/输入框/下拉），默认 32
	// ToolControlHeight 是工具栏内交互控件（图标/文字按钮、槽位里的输入框
	// 与下拉）的统一高度：条内所有控件同高、同用 vertical-align: middle
	// 对齐，才能彼此顶底齐平（默认 28，配 ToolHeight 36）。
	ToolControlHeight int
	// BarHeight 是控件条（NewBar）的高度。控件条是页面内容里的一行工具，
	// 应用要按它给自己的内容让位，所以尺寸得能从主题读到，不能只写在 CSS 里。
	BarHeight int
}

// Light 返回默认浅色主题。
//
// 其它浅色配色可以在这套令牌上改字段；深色见 Dark。两套主题的尺寸、
// 字体完全一致，只换颜色——工作台几何不会因为换皮而分叉。
func Light() Theme {
	return Theme{
		Font:         `"Microsoft YaHei", "Segoe UI", system-ui, sans-serif`,
		IconFont:     iconFontFamily,
		Bg:           "#F1F5F9",
		Surface:      "#FFFFFF",
		SurfaceAlt:   "#F8FAFC",
		Field:        "#FFFFFF",
		Hover:        "#E8EDF3",
		Press:        "#DDE4EC",
		Border:       "#E2E8F0",
		BorderStrong: "#CBD5E1",
		Text:         "#0F172A",
		Muted:        "#64748B",
		Accent:       "#2563EB",
		AccentHover:  "#1D4ED8",
		OnAccent:     "#FFFFFF",
		AccentSoft:   "#EFF6FF",
		AccentBorder: "#BFDBFE",
		AccentText:   "#1D4ED8",

		Success:       "#15803D",
		SuccessHover:  "#166534",
		OnSuccess:     "#FFFFFF",
		SuccessSoft:   "#F0FDF4",
		SuccessBorder: "#BBF7D0",
		SuccessText:   "#15803D",

		Warning:       "#F59E0B",
		WarningHover:  "#D97706",
		OnWarning:     "#451A03",
		WarningSoft:   "#FFFBEB",
		WarningBorder: "#FDE68A",
		WarningText:   "#B45309",

		Danger:       "#DC2626",
		DangerHover:  "#B91C1C",
		OnDanger:     "#FFFFFF",
		DangerSoft:   "#FEF2F2",
		DangerBorder: "#FECACA",
		DangerText:   "#B91C1C",

		Info:       "#0E7490",
		InfoHover:  "#155E75",
		OnInfo:     "#FFFFFF",
		InfoSoft:   "#ECFEFF",
		InfoBorder: "#A5F3FC",
		InfoText:   "#0E7490",

		Radius:           8,
		MenuHeight:       32,
		ToolHeight:       36,
		StatusHeight:     24,
		StatusRightWidth: 280,
		// 标准表单档：按钮/输入框/下拉同高 32（业界紧凑表单口径）；
		// 工具栏档另见 ToolControlHeight 28，两档互不跟随。
		ButtonHeight: 32,
		// 工具栏控件统一 28px：比页面按钮（32）矮一档，配 36 的条高
		// 上下各留 4px，图标/输入框/下拉同高同口径才能彼此齐平。
		ToolControlHeight: 28,
		BarHeight:         44,
	}
}

// Dark 返回默认深色主题。
//
// 从 Light() 拷贝后只覆盖颜色：尺寸/字体不重复写，以后新增令牌也不会
// 出现「浅色补了、深色忘了」。配色取蓝灰系（slate），与浅色主题同一套
// 色相，切换时只有明暗变化，不会有另一种风格。
//
// 强调色比浅色主题更亮（#60A5FA）：深底上 #2563EB 的蓝字会发闷，菜单
// 高亮和图标都看不清。代价是白字压不住它，所以 OnAccent 反过来用深色
// ——主按钮的对比度由主题负责，而不是在 CSS 里写死白色。
//
// 其余语义色沿用同一规律：实底取更亮的一档（400/500），On 用同色系的
// 深色；淡底取深色画布上的低饱和色，淡底文字取 300 档。浅色主题则相反：
// 实底取 600/700，On 用白色（Warning 例外，深色字更清楚）。
func Dark() Theme {
	t := Light()
	t.Bg = "#0F172A"
	t.Surface = "#1E293B"
	// 深色主题的空间层次：画布（含输入区）最深 → 栏面 Surface →
	// 工具栏/控件条 SurfaceAlt 逐级提亮，靠明度拉开而不是描边硬分。
	// Field 沉到画布档：输入框/编辑区在栏底上是一个个「凹下去」的
	// 可打字区，VS Code 同款做法。
	t.SurfaceAlt = "#24334A"
	t.Field = "#0F172A"
	t.Hover = "#293548"
	t.Press = "#334155"
	t.Border = "#2B3A4F"
	// 输入框默认边框取「极暗灰」：#3E5069 在深底上太跳，focus 才交给
	// 亮蓝强调色。
	t.BorderStrong = "#334155"
	t.Text = "#E2E8F0"
	t.Muted = "#94A3B8"
	t.Accent = "#60A5FA"
	t.AccentHover = "#3B82F6"
	t.OnAccent = "#0B1220"
	t.AccentSoft = "#1E3A5F"
	t.AccentBorder = "#2B4C7E"
	t.AccentText = "#93C5FD"

	t.Success = "#4ADE80"
	t.SuccessHover = "#22C55E"
	t.OnSuccess = "#052E16"
	t.SuccessSoft = "#123524"
	t.SuccessBorder = "#1F5A38"
	t.SuccessText = "#86EFAC"

	t.Warning = "#FBBF24"
	t.WarningHover = "#F59E0B"
	t.OnWarning = "#451A03"
	t.WarningSoft = "#3A2A12"
	t.WarningBorder = "#6B4A1B"
	t.WarningText = "#FCD34D"

	t.Danger = "#F87171"
	t.DangerHover = "#EF4444"
	// On 比其它深色档再深一点：EF4444 上的 #450A0A 只有 4.29:1，
	// hover 时不够 AA；#2A0505 能把 hover 抬到 4.97:1。
	t.OnDanger = "#2A0505"
	t.DangerSoft = "#3B1518"
	t.DangerBorder = "#7F1D1D"
	t.DangerText = "#FCA5A5"

	t.Info = "#22D3EE"
	t.InfoHover = "#06B6D4"
	t.OnInfo = "#083344"
	t.InfoSoft = "#123039"
	t.InfoBorder = "#155E75"
	t.InfoText = "#67E8F9"
	return t
}

// withDefaults 用 Light() 补齐零值字段。
//
// 理由：Theme 是值类型，应用常写 ui.Theme{Accent: "#0F766E"} 只改一项；
// 若不补齐，其余颜色是空串，生成出来的 CSS 里就是 `color: ;`——引擎
// 解析失败会静默退回继承值，bug 表现为「有一部分样式没生效」，很难查。
//
// 补的永远是 Light()，不是「就近的那套」：零值 Theme 的默认语义就是
// 浅色。要改深色主题得先取 ui.Dark() 再改字段，不能只写一两个字段
// 指望剩下的自动变深。
func (t Theme) withDefaults() Theme {
	d := Light()
	if t.Font == "" {
		t.Font = d.Font
	}
	if t.IconFont == "" {
		t.IconFont = d.IconFont
	}
	colors := []struct {
		dst *string
		def string
	}{
		{&t.Bg, d.Bg}, {&t.Surface, d.Surface}, {&t.SurfaceAlt, d.SurfaceAlt},
		{&t.Hover, d.Hover}, {&t.Press, d.Press}, {&t.Border, d.Border},
		{&t.BorderStrong, d.BorderStrong},
		{&t.Text, d.Text}, {&t.Muted, d.Muted},
		{&t.Accent, d.Accent}, {&t.AccentHover, d.AccentHover},
		{&t.OnAccent, d.OnAccent}, {&t.AccentSoft, d.AccentSoft},
		{&t.AccentBorder, d.AccentBorder}, {&t.AccentText, d.AccentText},

		{&t.Success, d.Success}, {&t.SuccessHover, d.SuccessHover},
		{&t.OnSuccess, d.OnSuccess}, {&t.SuccessSoft, d.SuccessSoft},
		{&t.SuccessBorder, d.SuccessBorder}, {&t.SuccessText, d.SuccessText},

		{&t.Warning, d.Warning}, {&t.WarningHover, d.WarningHover},
		{&t.OnWarning, d.OnWarning}, {&t.WarningSoft, d.WarningSoft},
		{&t.WarningBorder, d.WarningBorder}, {&t.WarningText, d.WarningText},

		{&t.Danger, d.Danger}, {&t.DangerHover, d.DangerHover},
		{&t.OnDanger, d.OnDanger}, {&t.DangerSoft, d.DangerSoft},
		{&t.DangerBorder, d.DangerBorder}, {&t.DangerText, d.DangerText},

		{&t.Info, d.Info}, {&t.InfoHover, d.InfoHover},
		{&t.OnInfo, d.OnInfo}, {&t.InfoSoft, d.InfoSoft},
		{&t.InfoBorder, d.InfoBorder}, {&t.InfoText, d.InfoText},
	}
	for _, c := range colors {
		if *c.dst == "" {
			*c.dst = c.def
		}
	}
	sizes := []struct {
		dst *int
		def int
	}{
		{&t.Radius, d.Radius}, {&t.MenuHeight, d.MenuHeight},
		{&t.ToolHeight, d.ToolHeight}, {&t.StatusHeight, d.StatusHeight},
		{&t.StatusRightWidth, d.StatusRightWidth}, {&t.ButtonHeight, d.ButtonHeight},
		{&t.ToolControlHeight, d.ToolControlHeight},
		{&t.BarHeight, d.BarHeight},
	}
	for _, s := range sizes {
		if *s.dst <= 0 {
			*s.dst = s.def
		}
	}
	return t
}

// tooltipColors 把主题令牌换算成提示气泡的配色（交给
// engine.SetTooltipColors）。
//
// 气泡是文档之上的绘制层，拿不到级联样式，只能显式下发同一套颜色：
// Surface 本就是「面板/栏/浮层底色」，描边用浮层的 BorderStrong，
// 文字跟主文字色。
func (t Theme) tooltipColors() engine.TooltipColors {
	return engine.TooltipColors{
		Background: engine.ParseColor(t.Surface),
		Border:     engine.ParseColor(t.BorderStrong),
		Text:       engine.ParseColor(t.Text),
	}
}

// css 生成一套组件样式。chromeTop 是页面内容顶部要空出的高度（菜单栏
// 与工具栏之和），status 是底部要空出的高度（没有状态栏时为 0）；
// resetBody 控制是否注入 body 复位块（见 Spec.NoBodyReset），画布底色
// 始终注入。
//
// 只使用引擎支持的 CSS 子集：
//   - 没有 flex/grid：横向排列一律 inline-block，浮层用绝对定位；
//   - 主题保持扁平、不用阴影：分隔线用 1px 的实体 div（.system-ui-hairline）；
//   - 没有 transition：悬停/按下靠 :hover/:active 直接切底色。
func (t Theme) css(menuTop, chromeTop, status int, resetBody bool) string {
	// 控件条的上下内边距由条高减控件高推出来：条内控件靠这条 padding 垂直
	// 居中，写死数字的话换主题（改了 BarHeight）就对不上。
	barDown := (t.BarHeight - t.ButtonHeight) / 2
	if barDown < 0 {
		barDown = 0
	}
	// 工具栏内所有交互控件（按钮/输入框/下拉）统一 ToolControlHeight 高，
	// 上下内边距由条高减控件高推出（默认 36−28=8 → 每侧 4px）。同高 + 同
	// vertical-align: middle，控件盒的中点都落到父级同一参考线上，彼此才
	// 能顶底齐平。
	toolCtrl := t.ToolControlHeight
	if toolCtrl <= 0 {
		toolCtrl = t.ButtonHeight
	}
	// middle 的参考线（父基线 + 半 x 高）天然比几何中线低约 3px：所有
	// 控件同低 3px 彼此是齐平的，但整排内容在条带里看着偏下。padding
	// 上下差 3px 把参考线整体抬回几何中线（像素实测）。
	toolPad := (t.ToolHeight - toolCtrl) / 2
	if toolPad < 3 {
		toolPad = 3
	}
	toolPadUp, toolPadDown := toolPad-3, toolPad+3
	// line-height 恒等于「控件高 − 上下边框 − 1px 光学修正」：按钮是
	// box-sizing: border-box + 1px 边框，内容盒比 height 矮 2px；line-height
	// 若写成 height 同值，行盒比内容盒高 2px，引擎的按钮行盒居中守卫
	// （pctH > lineH）不生效、行盒贴内容盒顶，文字偏低约 2px。再收 1px 是
	// 光学修正：13px 雅黑升降部不对称，墨迹中心比 em 盒中心低 1px，
	// half-leading 居中消不掉，行盒多收 1px 把基线抬回、墨迹正中。
	// 像素实测 kit 主按钮白字：+2 →（−2）+1 →（−3）0 = 盒中线。
	// 公式集中在这里推导，CSS 只引用变量，改 height 或边框时自动跟上。
	btnLH := t.ButtonHeight - 3
	if btnLH < 0 {
		btnLH = 0
	}
	toolLH := toolCtrl - 3
	if toolLH < 0 {
		toolLH = 0
	}
	// 图标行盒取字号（20px，CSS 里与 font-size 就近同写）：middle 的位移
	// 只由字体升降部/x 高推算，与行盒高度无关，line-height 在这里只是
	// 把行盒收到底、避免撑高工具栏行盒。墨迹下沉的补偿见 CSS 的
	// relative top（实测 −4px）。
	r := strings.NewReplacer(
		"$font$", t.Font,
		"$iconFont$", t.IconFont,
		"$bg$", t.Bg,
		"$surface$", t.Surface,
		"$surfaceAlt$", t.SurfaceAlt,
		"$field$", t.Field,
		"$hover$", t.Hover,
		"$press$", t.Press,
		"$border$", t.Border,
		"$borderStrong$", t.BorderStrong,
		"$text$", t.Text,
		"$muted$", t.Muted,
		"$accent$", t.Accent,
		"$accentHover$", t.AccentHover,
		"$onAccent$", t.OnAccent,
		"$accentSoft$", t.AccentSoft,
		"$accentBorder$", t.AccentBorder,
		"$accentText$", t.AccentText,
		"$success$", t.Success,
		"$successHover$", t.SuccessHover,
		"$onSuccess$", t.OnSuccess,
		"$successSoft$", t.SuccessSoft,
		"$successBorder$", t.SuccessBorder,
		"$successText$", t.SuccessText,
		"$warning$", t.Warning,
		"$warningHover$", t.WarningHover,
		"$onWarning$", t.OnWarning,
		"$warningSoft$", t.WarningSoft,
		"$warningBorder$", t.WarningBorder,
		"$warningText$", t.WarningText,
		"$danger$", t.Danger,
		"$dangerHover$", t.DangerHover,
		"$onDanger$", t.OnDanger,
		"$dangerSoft$", t.DangerSoft,
		"$dangerBorder$", t.DangerBorder,
		"$dangerText$", t.DangerText,
		"$info$", t.Info,
		"$infoHover$", t.InfoHover,
		"$onInfo$", t.OnInfo,
		"$infoSoft$", t.InfoSoft,
		"$infoBorder$", t.InfoBorder,
		"$infoText$", t.InfoText,
		"$radius$", strconv.Itoa(t.Radius),
		"$menuH$", strconv.Itoa(t.MenuHeight),
		"$toolH$", strconv.Itoa(t.ToolHeight),
		"$statusH$", strconv.Itoa(t.StatusHeight),
		"$statusRightW$", strconv.Itoa(t.StatusRightWidth),
		"$btnH$", strconv.Itoa(t.ButtonHeight),
		"$btnLH$", strconv.Itoa(btnLH),
		"$toolCtrlH$", strconv.Itoa(toolCtrl),
		"$toolLH$", strconv.Itoa(toolLH),
		"$toolPadUp$", strconv.Itoa(toolPadUp),
		"$toolPadDown$", strconv.Itoa(toolPadDown),
		"$barH$", strconv.Itoa(t.BarHeight),
		"$barPad$", strconv.Itoa(barDown),
		"$toolTop$", strconv.Itoa(menuTop),
		"$chromeH$", strconv.Itoa(chromeTop),
		"$statusSpace$", strconv.Itoa(status),
	)
	src := themeCSS
	if resetBody {
		src = bodyResetCSS + themeCSS
	}
	return scopeCSS(r.Replace(src), uiRootClass)
}

// uiRootClass 是组件样式的作用域根类，Install 会把它加到文档根元素上。
//
// 只改类名前缀还不够：第三方样式里的 `.panel div`（特异度 0,1,1）会压过
// `.system-ui-btn`（0,1,0）。把全部规则挂到 `html.system-ui` 下之后，组件
// 规则至少是 0,2,1；前缀统一后误命中概率降低，隔离主要靠这层根作用域。
const uiRootClass = "system-ui"

// scopeCSS 给样式表里的每条选择器加上根作用域：
//
//	.system-ui-btn -> html.system-ui .system-ui-btn
//	html         -> html.system-ui
//
// 样式表由本包维护，只含顶层规则（没有 @media 等嵌套块），所以按 { }
// 扫描即可；注释原样保留。
func scopeCSS(css, rootClass string) string {
	scope := "." + rootClass
	var b strings.Builder
	b.Grow(len(css) + len(css)/3)
	for len(css) > 0 {
		open := indexOutsideComment(css, '{')
		if open < 0 {
			b.WriteString(css)
			break
		}
		head := css[:open]
		start := selectorStart(head)
		b.WriteString(head[:start])
		b.WriteString(scopeSelectorList(head[start:], scope))
		b.WriteByte('{')

		depth, end := 1, -1
		for i := open + 1; i < len(css); i++ {
			if css[i] == '/' && i+1 < len(css) && css[i+1] == '*' {
				if j := strings.Index(css[i+2:], "*/"); j >= 0 {
					i += 2 + j + 1
					continue
				}
				break
			}
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			b.WriteString(css[open+1:])
			break
		}
		b.WriteString(css[open+1 : end+1])
		css = css[end+1:]
	}
	return b.String()
}

// indexOutsideComment 查找 target，但跳过 /* ... */ 注释里的字符。
func indexOutsideComment(s string, target byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			if j := strings.Index(s[i+2:], "*/"); j >= 0 {
				i += 2 + j + 1
				continue
			}
			return -1
		}
		if s[i] == target {
			return i
		}
	}
	return -1
}

// selectorStart 跳过选择器前面的空白与注释，返回实际选择器的起点。
func selectorStart(s string) int {
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\r', '\n':
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "/*") {
			if j := strings.Index(s[i+2:], "*/"); j >= 0 {
				i += 2 + j + 2
				continue
			}
		}
		break
	}
	return i
}

// scopeSelectorList 给逗号分隔的选择器列表逐项加作用域，保留末尾空白。
func scopeSelectorList(s, scope string) string {
	trimmed := strings.TrimRight(s, " \t\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return s
	}
	parts := strings.Split(trimmed, ",")
	for i, part := range parts {
		sel := strings.TrimSpace(part)
		switch {
		case sel == "":
			parts[i] = ""
		case sel == "html":
			parts[i] = "html" + scope
		case strings.HasPrefix(sel, "html."):
			parts[i] = sel
		default:
			parts[i] = "html" + scope + " " + sel
		}
	}
	return strings.Join(parts, ", ") + s[len(trimmed):]
}

// bodyResetCSS 是 body 复位规则，由 css 按需拼在 themeCSS 之前。
//
// 单独成块是因为它有 opt-out（Spec.NoBodyReset）：演示 UA 样式的页面
// （body 的 margin:8px 本身就是内容）、靠 body padding 给固定栏让位的页面，
// 被复位反而会改坏排版；画布底色（html background-color）没有这个矛盾，
// 留在 themeCSS 里始终注入。
const bodyResetCSS = `
/* 工作台复位：装了 ui 工作台的文档，body 就是工作台画布——UA 的 8px 外边距与
   默认字体字号，不该让每个页面重复复位一遍（颜色字号跟随主题）。
   根作用域后这条特异度是 0,2,1，页面的裸 body 规则盖不过它：自定义排版
   请挂在内容容器（.system-ui-page / 卡片）上，确实要改 body 就用同特异度
   写在本样式表之后，或干脆用 Spec.NoBodyReset 整体跳过复位。 */
body {
	margin: 0; padding: 0;
	color: $text$; font-family: $font$; font-size: 13px;
}
`

// themeCSS 是组件库的全部样式；$xxx$ 是 Theme 的占位符。
//
// 类名统一 system-ui- 前缀，并由 scopeCSS 挂到 html.system-ui 根作用域下；
// 除开头的「工作台复位」（画布底色 + body）有意作用于裸标签外，组件不会去改
// 页面里的裸标签（只有 .system-ui-list li 这种明确在组件内部的才用后代选择器）。
const themeCSS = `
/* 画布底色：页面自己的 body 背景仍然优先（引擎按 body → html 传播）。 */
html { background-color: $bg$; }

/* ===== 工作台：菜单栏 / 工具栏 / 状态栏 ===== */

/* 外壳盒子的几何复位。
   页面若写全局元素规则（div { margin: 8px; padding: 20px; border: 2px }），
   会命中外壳里的 div/span——fixed 的栏被 padding 撑大、按钮被 margin 顶偏、
   分隔线被描边；div 的 width/height 还会改菜单/占位块尺寸（引擎里 width/height
   的 auto 就是「未设」，所以能靠 auto 复位）。这里按类名把盒模型归零，外壳的
   几何只由外壳自己的规则决定。各组件真正需要的边距/内边距/边框/尺寸都写在
   **这条之后**的同名规则里（菜单项 padding、分隔线 margin、状态轨道
   padding-right、输入框宽度等），源码顺序在后，照常生效。
   注意：不要用后代选择器统一复位（特异度会压掉组件自己的 padding）。 */
.system-ui-hairline,
.system-ui-menubar,
.system-ui-menu,
.system-ui-menu-underline,
.system-ui-menu-pop,
.system-ui-menu-item,
.system-ui-menu-sep,
.system-ui-menu-icon,
.system-ui-menu-shortcut,
.system-ui-toolbar,
.system-ui-btn,
.system-ui-tool,
.system-ui-tool-slot,
.system-ui-sep,
.system-ui-icon,
.system-ui-label,
.system-ui-status,
.system-ui-status-track,
.system-ui-status-seg,
.system-ui-status-sep,
.system-ui-status-right,
.system-ui-status-msg,
.system-ui-spacer-top,
.system-ui-spacer-bottom,
.system-ui-bar,
.system-ui-input,
.system-ui-select,
.system-ui-textarea,
.system-ui-check {
	margin: 0; padding: 0; border-width: 0;
	width: auto; height: auto;
}

/* 文字口径复位（与上面的几何复位同因）。
   这些属性组件大多靠**继承**拿值，自己的规则里没写，所以页面 div/span
   规则直接命中外壳子元素时就会生效：body 或 div 写 font-weight:bold 会让
   菜单/按钮文字变粗（引擎的宽度测量忽略字重，几何看不出来，只有像素能抓），
   写 text-align:center 会让菜单项居中，写下划线/white-space:pre 会改观感。
   这里按类名钉回默认值；组件需要别的值（图标按钮 text-align:center、菜单
   标签 13px 等）都由后面的规则覆盖。 */
.system-ui-menu,
.system-ui-menu-pop,
.system-ui-menu-item,
.system-ui-menu-shortcut,
.system-ui-menu-icon,
.system-ui-btn,
.system-ui-tool-slot,
.system-ui-label,
.system-ui-icon {
	font-weight: normal; text-align: left;
	text-decoration: none; white-space: normal;
}

/* 1px 分隔线写在复位块**之后**：复位块把 height 归零，这里再给回 1px
   （写在前面会被 height: auto 覆盖掉，分隔线整条消失）。 */
.system-ui-hairline { height: 1px; background-color: $border$; pointer-events: none; }

/* 外壳三栏（菜单栏/工具栏/状态栏）与控件条都装在**页面文档**里，除了上面的
   盒模型/文字口径复位，容器自己还要挡掉**继承属性**：
   - text-align 被页面 body 写成 center 时（提示页、落地页很常见），整排
     按钮/分段会跟着居中，看上去像工具栏「漂到中间」；
   - font-size/font-weight/color/line-height/text-decoration/white-space/
     pointer-events 同样是继承属性：页面把 body 写大字号、加粗、下划线、
     pre、pointer-events:none，都会原样漏进外壳（最后一条会让整个外壳点不动）。
   子元素（按钮/输入框/分段/菜单标签）都各自声明了字号与颜色，容器的复位
   不会改变正常观感，只把「页面的意外声明」挡在门外。 */
.system-ui-menubar {
	position: fixed; left: 0; top: 0; width: 100%; height: $menuH$px;
	background-color: $surface$; z-index: 1000; font-family: $font$;
	display: block;
	font-size: 13px; font-weight: normal; line-height: $menuH$px; color: $text$;
	text-align: left; text-decoration: none; white-space: normal; pointer-events: auto;
}
.system-ui-menubar .system-ui-hairline { position: absolute; left: 0; bottom: 0; width: 100%; }

.system-ui-menu {
	display: inline-block; position: relative;
	height: $menuH$px; line-height: $menuH$px;
	padding: 0 12px; font-size: 13px; color: $text$;
}
.system-ui-menu:hover { background-color: $accentSoft$; color: $accentText$; }
.system-ui-menu.is-open { background-color: $accentSoft$; color: $accentText$; }
/* 打开态的下划线：与浮层同色的强调块，比整块高亮更克制。
   引擎不支持 right 定位（绝对定位只认 left/top/width），所以用
   width:100% 画满标签宽度，而不是左右各留 10px。 */
.system-ui-menu-underline {
	display: none; position: absolute; left: 0; bottom: 0;
	width: 100%; height: 2px; background-color: $accent$;
}
.system-ui-menu.is-open .system-ui-menu-underline { display: block; }

/* 浮层是菜单标签的子元素：标签 position:relative + 浮层 top:菜单栏高度，
   不需要知道标签的文字宽度，也不需要把浮层挪到 body 下重算坐标。 */
.system-ui-menu-pop {
	display: none; position: absolute; left: 0; top: $menuH$px;
	width: 216px; padding: 6px 0; box-sizing: border-box;
	background-color: $surface$; border: 1px solid $borderStrong$;
	border-radius: $radius$px; z-index: 1100; line-height: 18px;
}
/* 有图标的菜单多留一列；没有图标的菜单不留空列。 */
.system-ui-menu-pop.has-icon { width: 240px; }
.system-ui-menu.is-open .system-ui-menu-pop { display: block; }

.system-ui-menu-item {
	position: relative; height: 30px; line-height: 30px;
	margin: 0 6px; padding: 0 74px 0 10px;
	border-radius: 6px; font-size: 13px; color: $text$;
}
.system-ui-menu-pop.has-icon .system-ui-menu-item { padding-left: 34px; }
.system-ui-menu-item:hover { background-color: $accentSoft$; color: $accentText$; }
.system-ui-menu-item.is-disabled { color: $muted$; }
.system-ui-menu-item.is-disabled:hover { background-color: transparent; color: $muted$; }
.system-ui-menu-shortcut {
	position: absolute; right: 10px; top: 0;
	height: 30px; line-height: 30px; font-size: 12px; color: $muted$;
}
.system-ui-menu-sep { height: 1px; background-color: $border$; margin: 6px 10px; }
.system-ui-menu-icon { position: absolute; left: 9px; top: 7px; color: $muted$; }
.system-ui-menu-item:hover .system-ui-menu-icon { color: $accentText$; }
.system-ui-menu-item.is-disabled .system-ui-menu-icon { color: $muted$; }

.system-ui-toolbar {
	position: fixed; left: 0; top: $toolTop$px; width: 100%; height: $toolH$px;
	padding: $toolPadUp$px 8px $toolPadDown$px; box-sizing: border-box;
	background-color: $surfaceAlt$; z-index: 999;
	font-family: $font$; line-height: $toolCtrlH$px;
	display: block;
	font-size: 13px; font-weight: normal; color: $text$;
	text-align: left; text-decoration: none; white-space: normal; pointer-events: auto;
}
.system-ui-toolbar .system-ui-hairline { position: absolute; left: 0; bottom: 0; width: 100%; }

.system-ui-btn {
	display: inline-block; height: $btnH$px; line-height: $btnLH$px;
	padding: 0 10px; box-sizing: border-box;
	border: 1px solid transparent; border-radius: 6px;
	font-size: 13px; color: $text$; background-color: transparent;
	font-family: $font$; vertical-align: middle;
}
.system-ui-btn:hover { background-color: $hover$; }
.system-ui-btn:active { background-color: $press$; }
.system-ui-btn.is-primary { background-color: $accent$; border-color: $accent$; color: $onAccent$; }
.system-ui-btn.is-primary:hover { background-color: $accentHover$; border-color: $accentHover$; }
.system-ui-btn.is-primary:active { background-color: $accentHover$; border-color: $accentHover$; }

/* 语义变体：实底 + On* 文字。悬停/按下复用更深一档的 *Hover，不再单独
   维护 active 令牌。Default 是中性描边按钮：底色走输入控件同档的 Field、
   描边走 BorderStrong——和输入框同一眼看出「是一个可操作的盒子」；
   Warning 的 On 是深色，琥珀底压白字不够清楚。 */
.system-ui-btn.is-default { background-color: $field$; border-color: $borderStrong$; color: $text$; }
.system-ui-btn.is-default:hover { background-color: $hover$; }
.system-ui-btn.is-default:active { background-color: $press$; }
.system-ui-btn.is-success { background-color: $success$; border-color: $success$; color: $onSuccess$; }
.system-ui-btn.is-success:hover { background-color: $successHover$; border-color: $successHover$; }
.system-ui-btn.is-success:active { background-color: $successHover$; border-color: $successHover$; }
.system-ui-btn.is-warning { background-color: $warning$; border-color: $warning$; color: $onWarning$; }
.system-ui-btn.is-warning:hover { background-color: $warningHover$; border-color: $warningHover$; }
.system-ui-btn.is-warning:active { background-color: $warningHover$; border-color: $warningHover$; }
.system-ui-btn.is-danger { background-color: $danger$; border-color: $danger$; color: $onDanger$; }
.system-ui-btn.is-danger:hover { background-color: $dangerHover$; border-color: $dangerHover$; }
.system-ui-btn.is-danger:active { background-color: $dangerHover$; border-color: $dangerHover$; }
.system-ui-btn.is-info { background-color: $info$; border-color: $info$; color: $onInfo$; }
.system-ui-btn.is-info:hover { background-color: $infoHover$; border-color: $infoHover$; }
.system-ui-btn.is-info:active { background-color: $infoHover$; border-color: $infoHover$; }

.system-ui-btn.is-disabled { color: $muted$; }
/* 实底变体禁用后要退回中性：只改 color 的话绿/红底还在。 */
.system-ui-btn.is-primary.is-disabled,
.system-ui-btn.is-default.is-disabled,
.system-ui-btn.is-success.is-disabled,
.system-ui-btn.is-warning.is-disabled,
.system-ui-btn.is-danger.is-disabled,
.system-ui-btn.is-info.is-disabled {
	background-color: transparent; border-color: transparent; color: $muted$;
}
/* 禁用态要把悬停/按下底色也压住：只写 .is-disabled 的话 :hover 规则
   特异度更低但后写，仍然会亮起来。 */
.system-ui-btn.is-disabled:hover, .system-ui-btn.is-disabled:active {
	background-color: transparent; border-color: transparent; color: $muted$;
}

/* 工具栏内的交互控件统一口径：同高（ToolControlHeight）+ 同
   vertical-align: middle——盒高相同、对位参考线相同，按钮/输入框/下拉
   才能彼此顶底齐平。 */
.system-ui-toolbar .system-ui-btn {
	margin-right: 2px; height: $toolCtrlH$px; line-height: $toolLH$px;
}

/* 图标字形：Segoe MDL2 按 16px 网格设计，颜色跟随 color，所以悬停/禁用
   只需要改按钮的 color，不用为图标单独写状态。 */
.system-ui-icon {
	font-family: $iconFont$; font-size: 16px; line-height: 16px;
	font-weight: normal;
}
/* 工具栏按钮里的图标：放大到 20px（基础 .system-ui-icon 的 16px 是菜单
   图标等共用的，不动），行盒收到字号同高免得撑高工具栏行盒。
   vertical-align: middle 与控件盒同一对位参考线；符号字体没有可信的
   x 高度/升降部（middle 的位移由它们推算），MDL2 20px 实测墨迹比控件
   参考线低 2px，用相对定位一次上移补偿（top 为负值）。补偿量与按钮
   line-height 联动（行盒收矮 1px 基线上移 1px），当前口径实测 −2。
   像素实测：browser/notepad 条带图标/分隔线/下拉/地址栏中心全部落在
   条带中线 ±0.5px。 */
.system-ui-tool .system-ui-icon {
	position: relative; top: -2px;
	font-size: 20px; line-height: 20px; vertical-align: middle;
}

/* 工具栏图标按钮：方形、无边框，悬停只铺一层浅底。没有图标的命令退回
   文字按钮（is-label），不会被压成方块。 */
.system-ui-tool.is-icon { width: $toolCtrlH$px; padding: 0; text-align: center; }
.system-ui-tool.is-label { padding: 0 10px; }
.system-ui-tool:hover { background-color: $hover$; color: $accent$; }
.system-ui-tool:active { background-color: $press$; }
.system-ui-tool.is-disabled { color: $muted$; }
.system-ui-tool.is-disabled:hover, .system-ui-tool.is-disabled:active {
	background-color: transparent; color: $muted$;
}
/* 分隔竖线：与控件同用 vertical-align: middle（同一条参考线，天然齐平）。
   线高 18 比控件盒矮一档，留白显呼吸感。 */
.system-ui-sep {
	display: inline-block; width: 1px; height: 18px; margin: 0 6px;
	background-color: $borderStrong$; vertical-align: middle;
}

/* 工具栏槽位（Slot）：现成控件（输入框/下拉等）嵌进工具栏，与命令按钮
   同排。控件高度统一 ToolControlHeight（与按钮同高同 middle 口径，顶底
   齐平）；输入框沿用主题的表单宽度，下拉是「选择器」不是表单字段，收窄
   一档——都可用内联样式再调（引擎没有 flex，宽度只能定量，不能弹性拉伸）。 */
.system-ui-tool-slot {
	display: inline-block; margin-right: 2px; vertical-align: middle;
}
.system-ui-tool-slot .system-ui-input,
.system-ui-tool-slot .system-ui-select { height: $toolCtrlH$px; }
/* 槽位里的纯文本用 Label/Text 控件包裹（不做伪按钮）：单行标签的
   块级底边距是为表单字段上方设计的，嵌进工具栏要收掉、转 inline。 */
.system-ui-tool-slot .system-ui-label { display: inline; margin: 0; }
.system-ui-tool-slot .system-ui-select { width: 150px; }

.system-ui-status {
	position: fixed; left: 0; bottom: 0; width: 100%; height: $statusH$px;
	line-height: $statusH$px; padding: 0 12px; box-sizing: border-box;
	font-size: 12px; color: $muted$; background-color: $surface$;
	z-index: 1000; font-family: $font$;
	display: block; font-weight: normal;
	text-align: left; text-decoration: none; white-space: normal; pointer-events: auto;
}
.system-ui-status .system-ui-hairline { position: absolute; left: 0; top: 0; width: 100%; }
/* 状态栏内的文字口径写死在**每个盒子**上，而不是只靠 .system-ui-status 继承：
   分段/分隔线是 span，页面写 span { font-size: 20px; color: red } 会直接命中
   它们（元素选择器优先于继承）。显式 12px + muted 让分段口径不受页面影响。 */
.system-ui-status-track,
.system-ui-status-seg,
.system-ui-status-sep,
.system-ui-status-right,
.system-ui-status-msg {
	font-size: 12px; font-weight: normal; color: $muted$;
	text-align: left; text-decoration: none;
}
/* 左侧分段轨道：默认占满整行；有右侧消息时由 .has-right 预留固定宽度。
   white-space:pre 防止分段在空格处折行（引擎没有 nowrap）。 */
.system-ui-status-track {
	display: inline-block; width: 100%; height: $statusH$px;
	box-sizing: border-box; white-space: pre; overflow: hidden;
	vertical-align: top;
}
.system-ui-status.has-right .system-ui-status-track { padding-right: $statusRightW$px; }
.system-ui-status-seg {
	display: inline-block; height: $statusH$px; line-height: $statusH$px;
	padding: 0 10px; box-sizing: border-box; vertical-align: top;
	overflow: hidden;
}
.system-ui-status-seg.is-first { padding-left: 0; }
/* 右区贴边位：最右一段（糖消息缺位时）收掉右内边距，文字直接贴
   right:12px 盒缘——与左区 is-first、糖消息同一边距口径。糖消息
   在场时该类不加，保留的 10px 内边距充当两者间隙。 */
.system-ui-status-seg.is-last { padding-right: 0; }
/* 可点段（VSCode 式状态栏项）：悬停/按下换中性底色，与工具栏按钮
   同款令牌；纯文本段不挂 has-action，hover 不亮、零交互成本。 */
.system-ui-status-seg.has-action:hover { background-color: $hover$; }
.system-ui-status-seg.has-action:active { background-color: $press$; }
.system-ui-status-sep {
	display: inline-block; width: 1px; height: 12px; margin: 6px 0;
	background-color: $border$; vertical-align: top;
}
/* 右侧固定区容器：内装 AnchorRight 段（.system-ui-status-seg，段间带
   分隔线）与糖消息（.system-ui-status-msg，贴右缘、不参与分隔线）。
   text-align:right 让行内子段整体靠右堆叠；white-space:pre 与左轨道
   同口径，防分段折行（引擎没有 nowrap）。 */
.system-ui-status-right {
	position: absolute; right: 12px; top: 0;
	width: $statusRightW$px; height: $statusH$px; line-height: $statusH$px;
	text-align: right; overflow: hidden; color: $muted$;
	white-space: pre;
	display: none;
}
.system-ui-status.has-right .system-ui-status-right { display: block; }
.system-ui-status-msg {
	display: inline-block; height: $statusH$px; line-height: $statusH$px;
	overflow: hidden; vertical-align: top;
}

/* 占位块：fixed 的栏不占文档流，页面内容要靠这两块让出空间。
   放在 body 首尾而不是给 body 加 padding，是为了不覆盖应用自己的
   body 内边距（应用的页面通常自己也排了间距）。
   pointer-events:none：它们是纯排版的盒子，DOM 序又排在页面内容之后，
   留着可命中就会吃掉压在下面的绝对定位内容（浮层、控件条）的点击。 */
.system-ui-spacer-top { height: $chromeH$px; pointer-events: none; }
.system-ui-spacer-bottom { height: $statusSpace$px; pointer-events: none; }

/* ===== 页面组件 ===== */

/* 页面根节点也要给文字色：深色主题下若只改背景，页面里没套
   .system-ui-text/.system-ui-title 的裸文字会保持默认黑色，等于看不见。 */
.system-ui-page {
	padding: 22px 28px 30px 28px;
	background-color: $bg$; color: $text$; font-family: $font$;
}
.system-ui-title {
	margin: 0 0 6px 0; font-size: 22px; line-height: 30px;
	font-weight: bold; color: $text$;
}
.system-ui-subtitle {
	margin: 0 0 18px 0; font-size: 13px; line-height: 20px; color: $muted$;
}
.system-ui-card {
	margin: 0 0 14px 0; padding: 16px 18px;
	background-color: $surface$; border: 1px solid $border$;
	border-radius: 10px;
}
.system-ui-h2 {
	margin: 0 0 8px 0; font-size: 15px; line-height: 22px;
	font-weight: bold; color: $text$;
}
.system-ui-text {
	margin: 0 0 10px 0; font-size: 14px; line-height: 22px; color: $text$;
}
.system-ui-muted { color: $muted$; }
/* 语义文字色：只改颜色，字号/行高仍由 .system-ui-text 等容器决定。 */
.system-ui-text-primary { color: $accentText$; }
.system-ui-text-default { color: $text$; }
.system-ui-text-success { color: $successText$; }
.system-ui-text-warning { color: $warningText$; }
.system-ui-text-danger { color: $dangerText$; }
.system-ui-text-info { color: $infoText$; }

/* 徽标默认是 Primary，也可以用 is-default/is-success… 换语义。 */
.system-ui-badge {
	display: inline-block; padding: 3px 9px; margin: 0 0 10px 0;
	background-color: $accentSoft$; border: 1px solid $accentBorder$;
	border-radius: 999px; font-size: 12px; line-height: 16px; color: $accentText$;
}
.system-ui-badge.is-primary {
	background-color: $accentSoft$; border-color: $accentBorder$; color: $accentText$;
}
.system-ui-badge.is-default {
	background-color: $surfaceAlt$; border-color: $border$; color: $text$;
}
.system-ui-badge.is-success {
	background-color: $successSoft$; border-color: $successBorder$; color: $successText$;
}
.system-ui-badge.is-warning {
	background-color: $warningSoft$; border-color: $warningBorder$; color: $warningText$;
}
.system-ui-badge.is-danger {
	background-color: $dangerSoft$; border-color: $dangerBorder$; color: $dangerText$;
}
.system-ui-badge.is-info {
	background-color: $infoSoft$; border-color: $infoBorder$; color: $infoText$;
}
/* 提示条：淡底 + 同色描边 + 淡底文字。比实底按钮克制，适合正文里的
   成功/警告/错误说明。 */
.system-ui-alert {
	padding: 10px 12px; margin: 0 0 12px 0;
	border: 1px solid $border$; border-radius: 8px;
	background-color: $surfaceAlt$; color: $text$;
	font-size: 13px; line-height: 20px;
}
.system-ui-alert.is-primary {
	background-color: $accentSoft$; border-color: $accentBorder$; color: $accentText$;
}
.system-ui-alert.is-default {
	background-color: $surfaceAlt$; border-color: $border$; color: $text$;
}
.system-ui-alert.is-success {
	background-color: $successSoft$; border-color: $successBorder$; color: $successText$;
}
.system-ui-alert.is-warning {
	background-color: $warningSoft$; border-color: $warningBorder$; color: $warningText$;
}
.system-ui-alert.is-danger {
	background-color: $dangerSoft$; border-color: $dangerBorder$; color: $dangerText$;
}
.system-ui-alert.is-info {
	background-color: $infoSoft$; border-color: $infoBorder$; color: $infoText$;
}
.system-ui-list { margin: 0; padding-left: 18px; }
.system-ui-list li { margin: 4px 0; font-size: 14px; line-height: 22px; color: $text$; }
.system-ui-actions { margin: 4px 0 0 0; }
/* 内容区按钮给描边：卡片上「透明底 + 无边框」看起来像纯文字，
   分不清可点。底色/描边与输入控件同档（Field + BorderStrong），深色下
   靠在比卡片更深的底色上拉开。工具栏命令按钮保持无边框（栏本身提供了
   分组）。 */
.system-ui-actions .system-ui-btn {
	border-color: $borderStrong$; background-color: $field$; margin-right: 8px;
}
.system-ui-actions .system-ui-btn:hover { background-color: $hover$; }
.system-ui-actions .system-ui-btn:active { background-color: $press$; }
.system-ui-actions .system-ui-btn.is-primary {
	border-color: $accent$; background-color: $accent$; color: $onAccent$;
}
.system-ui-actions .system-ui-btn.is-primary:hover {
	border-color: $accentHover$; background-color: $accentHover$;
}
.system-ui-actions .system-ui-btn.is-primary:active {
	border-color: $accentHover$; background-color: $accentHover$;
}
/* 内容区的实底变体也要抬一档特异度，否则后面的 .system-ui-actions .system-ui-btn
   会把底色改回 Surface。Default 与 .system-ui-actions .system-ui-btn 同色，不必重复。 */
.system-ui-actions .system-ui-btn.is-success {
	border-color: $success$; background-color: $success$; color: $onSuccess$;
}
.system-ui-actions .system-ui-btn.is-success:hover {
	border-color: $successHover$; background-color: $successHover$;
}
.system-ui-actions .system-ui-btn.is-success:active {
	border-color: $successHover$; background-color: $successHover$;
}
.system-ui-actions .system-ui-btn.is-warning {
	border-color: $warning$; background-color: $warning$; color: $onWarning$;
}
.system-ui-actions .system-ui-btn.is-warning:hover {
	border-color: $warningHover$; background-color: $warningHover$;
}
.system-ui-actions .system-ui-btn.is-warning:active {
	border-color: $warningHover$; background-color: $warningHover$;
}
.system-ui-actions .system-ui-btn.is-danger {
	border-color: $danger$; background-color: $danger$; color: $onDanger$;
}
.system-ui-actions .system-ui-btn.is-danger:hover {
	border-color: $dangerHover$; background-color: $dangerHover$;
}
.system-ui-actions .system-ui-btn.is-danger:active {
	border-color: $dangerHover$; background-color: $dangerHover$;
}
.system-ui-actions .system-ui-btn.is-info {
	border-color: $info$; background-color: $info$; color: $onInfo$;
}
.system-ui-actions .system-ui-btn.is-info:hover {
	border-color: $infoHover$; background-color: $infoHover$;
}
.system-ui-actions .system-ui-btn.is-info:active {
	border-color: $infoHover$; background-color: $infoHover$;
}
/* 内容区按钮的禁用态要压过上面的 :hover/:active（同特异度时后写胜出，
   这里再抬一档特异度，避免禁用按钮悬停时又亮起来）。 */
.system-ui-actions .system-ui-btn.is-disabled,
.system-ui-actions .system-ui-btn.is-disabled:hover,
.system-ui-actions .system-ui-btn.is-disabled:active {
	border-color: $border$; background-color: $surface$; color: $muted$;
}
.system-ui-actions .system-ui-btn.is-primary.is-disabled,
.system-ui-actions .system-ui-btn.is-primary.is-disabled:hover,
.system-ui-actions .system-ui-btn.is-primary.is-disabled:active {
	border-color: $border$; background-color: $surface$; color: $muted$;
}
.system-ui-label {
	display: block; margin: 0 0 5px 0; font-size: 12px; color: $muted$;
}

/* 控件条：页面内容里的一行工具（查找替换栏、地址栏这类）。
   与工作台的工具栏区别在于它属于页面：定位与可见性由应用决定，所以这里
   不写 position/top，也不 fixed 在视口上；底色、描边、控件间距由主题给。
   横向排布仍是 inline-block（引擎没有 flex），line-height 用控件高度把
   它们顶到同一基线上。 */
.system-ui-bar {
	position: relative; box-sizing: border-box; width: 100%;
	height: $barH$px; padding: $barPad$px 8px;
	background-color: $surfaceAlt$; border-bottom: 1px solid $border$;
	font-family: $font$; font-size: 13px; line-height: $btnH$px;
	white-space: nowrap;
	display: block;
	font-weight: normal; color: $text$;
	text-align: left; text-decoration: none; pointer-events: auto;
}
.system-ui-bar .system-ui-input,
.system-ui-bar .system-ui-select,
.system-ui-bar .system-ui-btn,
.system-ui-bar .system-ui-check { margin-right: 8px; vertical-align: middle; }
/* 条内的输入框默认收窄：280px 是表单页的宽度，一行塞不下几件控件。 */
.system-ui-bar .system-ui-input,
.system-ui-bar .system-ui-select { width: 170px; }
.system-ui-bar .system-ui-check { line-height: $btnH$px; }
/* 条内中性按钮按钮化（与输入框同档 Field 底 + BorderStrong 描边）：
   「替换/关闭」这类动作不能只剩纯文字。is-primary/hover 要在后面显式
   压回：条选择器（0,2,0/0,3,0）后写，会盖过 .is-primary（0,2,0）与
   条内中性 :hover（0,3,0）。禁用态同理，否则 Field 底压掉置灰。 */
.system-ui-bar .system-ui-btn { background-color: $field$; border-color: $borderStrong$; }
.system-ui-bar .system-ui-btn:hover { background-color: $hover$; }
.system-ui-bar .system-ui-btn:active { background-color: $press$; }
.system-ui-bar .system-ui-btn.is-primary { background-color: $accent$; border-color: $accent$; color: $onAccent$; }
.system-ui-bar .system-ui-btn.is-primary:hover,
.system-ui-bar .system-ui-btn.is-primary:active { background-color: $accentHover$; border-color: $accentHover$; }
.system-ui-bar .system-ui-btn.is-disabled,
.system-ui-bar .system-ui-btn.is-disabled:hover,
.system-ui-bar .system-ui-btn.is-disabled:active {
	background-color: transparent; border-color: transparent; color: $muted$;
}

/* 表单控件与按钮同档（ButtonHeight，默认 32）：内容区里三者顶底齐平、
   悬停/焦点反馈节奏一致。工具栏槽位里的会另被 $toolCtrlH$ 覆盖成 28。 */
.system-ui-input {
	width: 280px; height: $btnH$px; padding: 0 10px; box-sizing: border-box;
	border: 1px solid $borderStrong$; border-radius: 7px;
	background-color: $field$; color: $text$; font-size: 13px;
	font-family: $font$;
}
.system-ui-input:focus { border-color: $accent$; }
/* 禁用控件要「沉」不要「浮」：底色退到画布档（浅色在白字段上发灰，
   深色与可用态同深），描边退回弱档 Border，文字 muted。原先用
   SurfaceAlt，深色里它已升格为「悬浮工具栏」的亮档，禁用字段反而
   比可用的更亮，方向反了。 */
.system-ui-input:disabled,
.system-ui-select:disabled,
.system-ui-textarea:disabled {
	background-color: $bg$; border-color: $border$; color: $muted$;
}

/* 表单控件：与页面卡片同一套圆角/描边。textarea 默认给一个可用的高度，
   应用需要更高时直接改内联样式（引擎没有 calc，不能做自适应高度）。 */
.system-ui-select {
	width: 280px; height: $btnH$px; padding: 0 8px; box-sizing: border-box;
	border: 1px solid $borderStrong$; border-radius: 7px;
	background-color: $field$; color: $text$; font-size: 13px;
	font-family: $font$;
}
.system-ui-select:focus { border-color: $accent$; }
.system-ui-textarea {
	width: 100%; height: 120px; padding: 8px 10px; box-sizing: border-box;
	border: 1px solid $borderStrong$; border-radius: 7px;
	background-color: $field$; color: $text$; font-size: 13px;
	line-height: 20px; font-family: $font$; overflow: auto;
}
.system-ui-textarea:focus { border-color: $accent$; }
/* 平铺变体：贴边铺满的编辑区（文档纸面、全屏输入）不要描边和圆角——
   它占的是整块内容区，边框会把纸面切成「一个控件」而不是「一张纸」。
   几何（top/width/height）仍由应用按视口尺寸写内联样式：引擎没有 calc。 */
.system-ui-textarea.is-plain { border: none; border-radius: 0; }
.system-ui-textview {
	height: 140px; padding: 12px; box-sizing: border-box;
	border: 1px solid $border$; border-radius: $radius$px;
	background-color: $surfaceAlt$; color: $text$;
	font-size: 13px; line-height: 20px; white-space: pre; overflow: auto;
}
.system-ui-check {
	display: inline-block; margin: 0 16px 8px 0;
	font-size: 13px; line-height: 20px; color: $text$;
}
.system-ui-check input { margin: 0 6px 0 0; vertical-align: middle; }
.system-ui-check.is-disabled { color: $muted$; }
.system-ui-field { margin: 0 0 14px 0; }
.system-ui-field-hint {
	margin: 5px 0 0 0; font-size: 12px; line-height: 18px; color: $muted$;
}
.system-ui-field-error {
	margin: 5px 0 0 0; font-size: 12px; line-height: 18px; color: $dangerText$;
}
.system-ui-field.is-error .system-ui-input,
.system-ui-field.is-error .system-ui-select,
.system-ui-field.is-error .system-ui-textarea {
	border-color: $danger$;
}
`
