// 本文件是组件库的视觉令牌与样式表生成；包文档见 doc.go。
package ui

import (
	"strconv"
	"strings"
)

// Theme 是组件库的视觉令牌：一套配色 + 一组尺寸。
//
// 为什么是「令牌 + 生成样式表」而不是让应用写 CSS：
//   - 引擎的 CSS 是子集（没有 flex/grid），外壳要用的技巧（占位块、
//     1px 分隔线、绝对定位浮层）容易写错；
//   - 主题是一处改、全部一致的：应用只写结构，不碰颜色与间距；
//   - 样式表由 Install 追加在作者样式之后，同特异度时外壳胜出，页面
//     自带的 body/div 规则不会把菜单栏挤歪。
//
// 零值字段由 withDefaults 用 Light() 补齐，应用只需改想改的几项。
type Theme struct {
	Font     string // font-family 原值
	IconFont string // 图标字体（默认 Segoe MDL2 Assets，见 icon.go）

	Bg           string // 画布底色（卡片之外的区域）
	Surface      string // 面板/栏/浮层底色
	SurfaceAlt   string // 工具栏底色
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

	Radius       int // 面板/浮层圆角
	MenuHeight   int // 菜单栏高度
	ToolHeight   int // 工具栏高度
	StatusHeight int // 状态栏高度
	ButtonHeight int // 按钮高度
}

// Light 返回默认浅色主题。
//
// 其它浅色配色可以在这套令牌上改字段；深色见 Dark。两套主题的尺寸、
// 字体完全一致，只换颜色——外壳几何不会因为换皮而分叉。
func Light() Theme {
	return Theme{
		Font:         `"Microsoft YaHei", "Segoe UI", system-ui, sans-serif`,
		IconFont:     iconFontFamily,
		Bg:           "#F1F5F9",
		Surface:      "#FFFFFF",
		SurfaceAlt:   "#F8FAFC",
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

		Radius:       8,
		MenuHeight:   32,
		ToolHeight:   40,
		StatusHeight: 24,
		ButtonHeight: 30,
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
	t.SurfaceAlt = "#172033"
	t.Hover = "#293548"
	t.Press = "#334155"
	t.Border = "#2B3A4F"
	t.BorderStrong = "#3E5069"
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
		{&t.ButtonHeight, d.ButtonHeight},
	}
	for _, s := range sizes {
		if *s.dst <= 0 {
			*s.dst = s.def
		}
	}
	return t
}

// css 生成一套组件样式。chromeTop 是页面内容顶部要空出的高度（菜单栏
// 与工具栏之和），status 是底部要空出的高度（没有状态栏时为 0）。
//
// 只使用引擎支持的 CSS 子集：
//   - 没有 flex/grid：横向排列一律 inline-block，浮层用绝对定位；
//   - 主题保持扁平、不用阴影：分隔线用 1px 的实体 div（.ui-hairline）；
//   - 没有 transition：悬停/按下靠 :hover/:active 直接切底色。
func (t Theme) css(menuTop, chromeTop, status int) string {
	r := strings.NewReplacer(
		"$font$", t.Font,
		"$iconFont$", t.IconFont,
		"$bg$", t.Bg,
		"$surface$", t.Surface,
		"$surfaceAlt$", t.SurfaceAlt,
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
		"$btnH$", strconv.Itoa(t.ButtonHeight),
		"$toolTop$", strconv.Itoa(menuTop),
		"$chromeH$", strconv.Itoa(chromeTop),
		"$statusSpace$", strconv.Itoa(status),
	)
	return r.Replace(themeCSS)
}

// themeCSS 是组件库的全部样式；$xxx$ 是 Theme 的占位符。
//
// 类名统一 ui- 前缀：应用自己的样式不会误命中，组件也不会去改页面里
// 的裸标签（只有 .ui-list li 这种明确在组件内部的才用后代选择器）。
const themeCSS = `
/* 画布底色：页面自己的 body 背景仍然优先（引擎按 body → html 传播）。 */
html { background-color: $bg$; }

/* ===== 外壳：菜单栏 / 工具栏 / 状态栏 ===== */

.ui-hairline { height: 1px; background-color: $border$; }

.ui-menubar {
	position: fixed; left: 0; top: 0; width: 100%; height: $menuH$px;
	background-color: $surface$; z-index: 1000; font-family: $font$;
}
.ui-menubar .ui-hairline { position: absolute; left: 0; bottom: 0; width: 100%; }

.ui-menu {
	display: inline-block; position: relative;
	height: $menuH$px; line-height: $menuH$px;
	padding: 0 12px; font-size: 13px; color: $text$;
}
.ui-menu:hover { background-color: $accentSoft$; color: $accentText$; }
.ui-menu.is-open { background-color: $accentSoft$; color: $accentText$; }
/* 打开态的下划线：与浮层同色的强调块，比整块高亮更克制。
   引擎不支持 right 定位（绝对定位只认 left/top/width），所以用
   width:100% 画满标签宽度，而不是左右各留 10px。 */
.ui-menu-underline {
	display: none; position: absolute; left: 0; bottom: 0;
	width: 100%; height: 2px; background-color: $accent$;
}
.ui-menu.is-open .ui-menu-underline { display: block; }

/* 浮层是菜单标签的子元素：标签 position:relative + 浮层 top:菜单栏高度，
   不需要知道标签的文字宽度，也不需要把浮层挪到 body 下重算坐标。 */
.ui-menu-pop {
	display: none; position: absolute; left: 0; top: $menuH$px;
	width: 216px; padding: 6px 0; box-sizing: border-box;
	background-color: $surface$; border: 1px solid $borderStrong$;
	border-radius: $radius$px; z-index: 1100; line-height: 18px;
}
/* 有图标的菜单多留一列；没有图标的菜单不留空列。 */
.ui-menu-pop.has-icon { width: 240px; }
.ui-menu.is-open .ui-menu-pop { display: block; }

.ui-menu-item {
	position: relative; height: 30px; line-height: 30px;
	margin: 0 6px; padding: 0 74px 0 10px;
	border-radius: 6px; font-size: 13px; color: $text$;
}
.ui-menu-pop.has-icon .ui-menu-item { padding-left: 34px; }
.ui-menu-item:hover { background-color: $accentSoft$; color: $accentText$; }
.ui-menu-item.is-disabled { color: $muted$; }
.ui-menu-item.is-disabled:hover { background-color: transparent; color: $muted$; }
.ui-menu-shortcut {
	position: absolute; right: 10px; top: 0;
	height: 30px; line-height: 30px; font-size: 12px; color: $muted$;
}
.ui-menu-sep { height: 1px; background-color: $border$; margin: 6px 10px; }
.ui-menu-icon { position: absolute; left: 9px; top: 7px; color: $muted$; }
.ui-menu-item:hover .ui-menu-icon { color: $accentText$; }
.ui-menu-item.is-disabled .ui-menu-icon { color: $muted$; }

.ui-toolbar {
	position: fixed; left: 0; top: $toolTop$px; width: 100%; height: $toolH$px;
	padding: 5px 8px; box-sizing: border-box;
	background-color: $surfaceAlt$; z-index: 999;
	font-family: $font$; line-height: $btnH$px;
}
.ui-toolbar .ui-hairline { position: absolute; left: 0; bottom: 0; width: 100%; }

.ui-btn {
	display: inline-block; height: $btnH$px; line-height: $btnH$px;
	padding: 0 10px; box-sizing: border-box;
	border: 1px solid transparent; border-radius: 6px;
	font-size: 13px; color: $text$; background-color: transparent;
	vertical-align: middle;
}
.ui-btn:hover { background-color: $hover$; }
.ui-btn:active { background-color: $press$; }
.ui-btn.is-primary { background-color: $accent$; border-color: $accent$; color: $onAccent$; }
.ui-btn.is-primary:hover { background-color: $accentHover$; border-color: $accentHover$; }
.ui-btn.is-primary:active { background-color: $accentHover$; border-color: $accentHover$; }

/* 语义变体：实底 + On* 文字。悬停/按下复用更深一档的 *Hover，不再单独
   维护 active 令牌。Default 是中性描边按钮，复用 Surface/Border/Text
   这组中性色；Warning 的 On 是深色，琥珀底压白字不够清楚。 */
.ui-btn.is-default { background-color: $surface$; border-color: $border$; color: $text$; }
.ui-btn.is-default:hover { background-color: $surfaceAlt$; }
.ui-btn.is-default:active { background-color: $press$; }
.ui-btn.is-success { background-color: $success$; border-color: $success$; color: $onSuccess$; }
.ui-btn.is-success:hover { background-color: $successHover$; border-color: $successHover$; }
.ui-btn.is-success:active { background-color: $successHover$; border-color: $successHover$; }
.ui-btn.is-warning { background-color: $warning$; border-color: $warning$; color: $onWarning$; }
.ui-btn.is-warning:hover { background-color: $warningHover$; border-color: $warningHover$; }
.ui-btn.is-warning:active { background-color: $warningHover$; border-color: $warningHover$; }
.ui-btn.is-danger { background-color: $danger$; border-color: $danger$; color: $onDanger$; }
.ui-btn.is-danger:hover { background-color: $dangerHover$; border-color: $dangerHover$; }
.ui-btn.is-danger:active { background-color: $dangerHover$; border-color: $dangerHover$; }
.ui-btn.is-info { background-color: $info$; border-color: $info$; color: $onInfo$; }
.ui-btn.is-info:hover { background-color: $infoHover$; border-color: $infoHover$; }
.ui-btn.is-info:active { background-color: $infoHover$; border-color: $infoHover$; }

.ui-btn.is-disabled { color: $muted$; }
/* 实底变体禁用后要退回中性：只改 color 的话绿/红底还在。 */
.ui-btn.is-primary.is-disabled,
.ui-btn.is-default.is-disabled,
.ui-btn.is-success.is-disabled,
.ui-btn.is-warning.is-disabled,
.ui-btn.is-danger.is-disabled,
.ui-btn.is-info.is-disabled {
	background-color: transparent; border-color: transparent; color: $muted$;
}
/* 禁用态要把悬停/按下底色也压住：只写 .is-disabled 的话 :hover 规则
   特异度更低但后写，仍然会亮起来。 */
.ui-btn.is-disabled:hover, .ui-btn.is-disabled:active {
	background-color: transparent; border-color: transparent; color: $muted$;
}

.ui-toolbar .ui-btn { margin-right: 2px; }

/* 图标字形：Segoe MDL2 按 16px 网格设计，颜色跟随 color，所以悬停/禁用
   只需要改按钮的 color，不用为图标单独写状态。 */
.ui-icon {
	font-family: $iconFont$; font-size: 16px; line-height: 16px;
	font-weight: normal;
}
.ui-tool .ui-icon { line-height: $btnH$px; vertical-align: top; }

/* 工具栏图标按钮：方形、无边框，悬停只铺一层浅底。没有图标的命令退回
   文字按钮（is-label），不会被压成方块。 */
.ui-tool.is-icon { width: $btnH$px; padding: 0; text-align: center; }
.ui-tool.is-label { padding: 0 10px; }
.ui-tool:hover { background-color: $hover$; color: $accent$; }
.ui-tool:active { background-color: $press$; }
.ui-tool.is-disabled { color: $muted$; }
.ui-tool.is-disabled:hover, .ui-tool.is-disabled:active {
	background-color: transparent; color: $muted$;
}
.ui-sep {
	display: inline-block; width: 1px; height: 18px; margin: 0 6px;
	background-color: $borderStrong$; vertical-align: middle;
}

.ui-status {
	position: fixed; left: 0; bottom: 0; width: 100%; height: $statusH$px;
	line-height: $statusH$px; padding: 0 12px; box-sizing: border-box;
	font-size: 12px; color: $muted$; background-color: $surface$;
	z-index: 1000; font-family: $font$;
}
.ui-status .ui-hairline { position: absolute; left: 0; top: 0; width: 100%; }
.ui-status-right {
	position: absolute; right: 12px; top: 0;
	height: $statusH$px; line-height: $statusH$px; color: $muted$;
}

/* 占位块：fixed 的栏不占文档流，页面内容要靠这两块让出空间。
   放在 body 首尾而不是给 body 加 padding，是为了不覆盖应用自己的
   body 内边距（应用的页面通常自己也排了间距）。 */
.ui-spacer-top { height: $chromeH$px; }
.ui-spacer-bottom { height: $statusSpace$px; }

/* ===== 页面组件 ===== */

/* 页面根节点也要给文字色：深色主题下若只改背景，页面里没套
   .ui-text/.ui-title 的裸文字会保持默认黑色，等于看不见。 */
.ui-page {
	padding: 22px 28px 30px 28px;
	background-color: $bg$; color: $text$; font-family: $font$;
}
.ui-title {
	margin: 0 0 6px 0; font-size: 22px; line-height: 30px;
	font-weight: bold; color: $text$;
}
.ui-subtitle {
	margin: 0 0 18px 0; font-size: 13px; line-height: 20px; color: $muted$;
}
.ui-card {
	margin: 0 0 14px 0; padding: 16px 18px;
	background-color: $surface$; border: 1px solid $border$;
	border-radius: 10px;
}
.ui-h2 {
	margin: 0 0 8px 0; font-size: 15px; line-height: 22px;
	font-weight: bold; color: $text$;
}
.ui-text {
	margin: 0 0 10px 0; font-size: 14px; line-height: 22px; color: $text$;
}
.ui-muted { color: $muted$; }
/* 语义文字色：只改颜色，字号/行高仍由 .ui-text 等容器决定。 */
.ui-text-primary { color: $accentText$; }
.ui-text-default { color: $text$; }
.ui-text-success { color: $successText$; }
.ui-text-warning { color: $warningText$; }
.ui-text-danger { color: $dangerText$; }
.ui-text-info { color: $infoText$; }

/* 徽标默认是 Primary，也可以用 is-default/is-success… 换语义。 */
.ui-badge {
	display: inline-block; padding: 3px 9px; margin: 0 0 10px 0;
	background-color: $accentSoft$; border: 1px solid $accentBorder$;
	border-radius: 999px; font-size: 12px; line-height: 16px; color: $accentText$;
}
.ui-badge.is-primary {
	background-color: $accentSoft$; border-color: $accentBorder$; color: $accentText$;
}
.ui-badge.is-default {
	background-color: $surfaceAlt$; border-color: $border$; color: $text$;
}
.ui-badge.is-success {
	background-color: $successSoft$; border-color: $successBorder$; color: $successText$;
}
.ui-badge.is-warning {
	background-color: $warningSoft$; border-color: $warningBorder$; color: $warningText$;
}
.ui-badge.is-danger {
	background-color: $dangerSoft$; border-color: $dangerBorder$; color: $dangerText$;
}
.ui-badge.is-info {
	background-color: $infoSoft$; border-color: $infoBorder$; color: $infoText$;
}
/* 提示条：淡底 + 同色描边 + 淡底文字。比实底按钮克制，适合正文里的
   成功/警告/错误说明。 */
.ui-alert {
	padding: 10px 12px; margin: 0 0 12px 0;
	border: 1px solid $border$; border-radius: 8px;
	background-color: $surfaceAlt$; color: $text$;
	font-size: 13px; line-height: 20px;
}
.ui-alert.is-primary {
	background-color: $accentSoft$; border-color: $accentBorder$; color: $accentText$;
}
.ui-alert.is-default {
	background-color: $surfaceAlt$; border-color: $border$; color: $text$;
}
.ui-alert.is-success {
	background-color: $successSoft$; border-color: $successBorder$; color: $successText$;
}
.ui-alert.is-warning {
	background-color: $warningSoft$; border-color: $warningBorder$; color: $warningText$;
}
.ui-alert.is-danger {
	background-color: $dangerSoft$; border-color: $dangerBorder$; color: $dangerText$;
}
.ui-alert.is-info {
	background-color: $infoSoft$; border-color: $infoBorder$; color: $infoText$;
}
.ui-list { margin: 0; padding-left: 18px; }
.ui-list li { margin: 4px 0; font-size: 14px; line-height: 22px; color: $text$; }
.ui-actions { margin: 4px 0 0 0; }
/* 内容区按钮给描边：白卡片上「透明底 + 无边框」看起来像纯文字，
   分不清可点。工具栏按钮保持无边框（栏本身提供了分组）。 */
.ui-actions .ui-btn {
	border-color: $border$; background-color: $surface$; margin-right: 8px;
}
.ui-actions .ui-btn:hover { background-color: $surfaceAlt$; }
.ui-actions .ui-btn:active { background-color: $press$; }
.ui-actions .ui-btn.is-primary {
	border-color: $accent$; background-color: $accent$; color: $onAccent$;
}
.ui-actions .ui-btn.is-primary:hover {
	border-color: $accentHover$; background-color: $accentHover$;
}
.ui-actions .ui-btn.is-primary:active {
	border-color: $accentHover$; background-color: $accentHover$;
}
/* 内容区的实底变体也要抬一档特异度，否则后面的 .ui-actions .ui-btn
   会把底色改回 Surface。Default 与 .ui-actions .ui-btn 同色，不必重复。 */
.ui-actions .ui-btn.is-success {
	border-color: $success$; background-color: $success$; color: $onSuccess$;
}
.ui-actions .ui-btn.is-success:hover {
	border-color: $successHover$; background-color: $successHover$;
}
.ui-actions .ui-btn.is-success:active {
	border-color: $successHover$; background-color: $successHover$;
}
.ui-actions .ui-btn.is-warning {
	border-color: $warning$; background-color: $warning$; color: $onWarning$;
}
.ui-actions .ui-btn.is-warning:hover {
	border-color: $warningHover$; background-color: $warningHover$;
}
.ui-actions .ui-btn.is-warning:active {
	border-color: $warningHover$; background-color: $warningHover$;
}
.ui-actions .ui-btn.is-danger {
	border-color: $danger$; background-color: $danger$; color: $onDanger$;
}
.ui-actions .ui-btn.is-danger:hover {
	border-color: $dangerHover$; background-color: $dangerHover$;
}
.ui-actions .ui-btn.is-danger:active {
	border-color: $dangerHover$; background-color: $dangerHover$;
}
.ui-actions .ui-btn.is-info {
	border-color: $info$; background-color: $info$; color: $onInfo$;
}
.ui-actions .ui-btn.is-info:hover {
	border-color: $infoHover$; background-color: $infoHover$;
}
.ui-actions .ui-btn.is-info:active {
	border-color: $infoHover$; background-color: $infoHover$;
}
/* 内容区按钮的禁用态要压过上面的 :hover/:active（同特异度时后写胜出，
   这里再抬一档特异度，避免禁用按钮悬停时又亮起来）。 */
.ui-actions .ui-btn.is-disabled,
.ui-actions .ui-btn.is-disabled:hover,
.ui-actions .ui-btn.is-disabled:active {
	border-color: $border$; background-color: $surface$; color: $muted$;
}
.ui-actions .ui-btn.is-primary.is-disabled,
.ui-actions .ui-btn.is-primary.is-disabled:hover,
.ui-actions .ui-btn.is-primary.is-disabled:active {
	border-color: $border$; background-color: $surface$; color: $muted$;
}
.ui-label {
	display: block; margin: 0 0 5px 0; font-size: 12px; color: $muted$;
}
.ui-input {
	width: 280px; height: 30px; padding: 0 10px; box-sizing: border-box;
	border: 1px solid $borderStrong$; border-radius: 7px;
	background-color: $surface$; color: $text$; font-size: 13px;
	font-family: $font$;
}
.ui-input:focus { border-color: $accent$; }
`
