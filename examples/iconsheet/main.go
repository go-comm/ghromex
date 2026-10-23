// 示例：内置图标表。
//
// 把 ui 包里的全部图标名排成一张可滚动的表：字形、名称、码位一起展示；
// 顶部可以按名称实时过滤，单击卡片会把图标名复制到剪贴板。工具栏的
// 「切换主题」和「视图」菜单可以在浅色/深色之间实时切换。
//
//	go run ./examples/iconsheet
//	go run ./examples/iconsheet -dark                 # 以深色主题启动
//	go run ./examples/iconsheet -search align         # 启动时预填搜索词
//	go run ./examples/iconsheet -shot iconsheet.png   # 真实字体截图（需要 SDL）
//	go run ./examples/iconsheet -dump-png sheet.png   # 无头出图（文字是占位方块）
//	go run ./examples/iconsheet -dump-svg sheet.svg
package main

import (
	"flag"
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/runtime/clipboard"
	"github.com/go-comm/ghromex/ui"
)

const (
	winW = 1180
	winH = 860
)

// iconGroups 是图标表的展示顺序。
//
// 这里只负责「怎么分组、组内怎么排」；真正的图标集合来自 ui.Icons()，
// 没列到的名字会落到最后的「其他」，以后 ui 新增图标也不会漏。
var iconGroups = []struct {
	Title string
	Names []string
}{
	{"方向 / 导航", []string{
		"up", "down", "left", "right",
		"arrow-up", "arrow-down", "arrow-left", "arrow-right",
		"back", "forward", "home",
		"chevron-up", "chevron-down", "chevron-left", "chevron-right",
		"page-left", "page-right", "go-to-start",
		"menu", "history", "recent", "location", "map-pin", "map-layers",
	}},
	{"对齐 / 排版", []string{
		"align-left", "align-center", "align-right",
		"bulleted-list", "list", "check-list",
		"bold", "italic", "underline",
		"font-increase", "font-decrease", "font-size", "font-color",
		"highlight",
	}},
	{"播放 / 媒体", []string{
		"play", "pause", "stop", "record",
		"previous", "next", "rewind", "fast-forward",
		"shuffle", "repeat", "repeat-one",
		"volume", "volume-0", "volume-1", "volume-2", "volume-3",
		"mute", "speakers", "headphone",
		"fullscreen", "back-to-window",
		"movies", "video", "streaming", "slideshow", "projector",
	}},
	{"文件 / 编辑", []string{
		"new", "open", "open-file", "save", "save-as",
		"copy", "cut", "paste", "undo", "redo",
		"delete", "trash", "search", "replace",
		"print", "printer", "refresh", "reload",
		"document", "page", "folder", "folder-open", "new-folder",
		"code", "edit", "rename", "select-all",
	}},
	{"视图 / 窗口", []string{
		"zoom", "zoom-in", "zoom-out", "fit-page",
		"view", "view-all", "preview",
		"dock-left", "dock-right", "dock-bottom",
		"close-pane", "open-pane",
		"new-window", "open-in-new-window",
		"minimize", "maximize", "restore", "window-close",
	}},
	{"通用 / 状态", []string{
		"add", "plus", "remove", "minus",
		"check", "close", "more",
		"settings", "info", "help", "warning", "error", "completed",
		"filter", "pin", "unpin",
		"favorite", "favorite-fill", "unfavorite",
		"share", "link", "attach", "send", "mail",
		"comment", "chat", "people", "contact",
		"tag", "flag", "bookmark", "library",
		"music", "music-note", "picture", "photo", "camera", "microphone",
		"network", "wifi", "bluetooth", "usb", "cloud", "sync",
		"calendar", "stopwatch", "calculator", "game",
		"sort", "download", "upload",
	}},
}

func main() {
	dark := flag.Bool("dark", false, "使用深色主题")
	search := flag.String("search", "", "启动时预填搜索词（便于演示/截图）")
	dumpPNG := flag.String("dump-png", "", "无头渲染成 PNG 后退出（文字是占位方块）")
	dumpSVG := flag.String("dump-svg", "", "无头导出布局 SVG 后退出")
	shot := flag.String("shot", "", "用真实字体渲染一帧并存成 PNG 后退出（需要 SDL）")
	exitAfter := flag.Duration("exit-after", 0, "运行指定时长后自动退出（调试/CI）")
	flag.Parse()

	entries := ui.Icons()
	page := buildPage(entries)

	var app *ui.App
	status := func(text string) {
		if app != nil {
			app.SetStatus(text)
		}
	}
	darkMode := *dark
	themeLabel := func() string {
		if darkMode {
			return "深色"
		}
		return "浅色"
	}
	themeToggle := ui.Item("切换主题", "", func() error {
		darkMode = !darkMode
		theme := ui.Light()
		if darkMode {
			theme = ui.Dark()
		}
		if app != nil {
			app.SetTheme(theme)
			if sh := app.Workbench(); sh != nil {
				sh.SetStatusRight(fmt.Sprintf("Segoe MDL2 Assets · %d 个图标名 · %s主题", len(entries), themeLabel()))
			}
		}
		status(fmt.Sprintf("已切换到%s主题", themeLabel()))
		return nil
	})
	spec := buildSpec(entries, status, themeToggle)
	spec.StatusRight = fmt.Sprintf("Segoe MDL2 Assets · %d 个图标名 · %s主题", len(entries), themeLabel())
	if darkMode {
		theme := ui.Dark()
		spec.Theme = &theme
	}

	if *dumpPNG != "" || *dumpSVG != "" {
		if err := dump(spec, page, *search, *dumpPNG, *dumpSVG); err != nil {
			fmt.Fprintln(os.Stderr, "无头导出失败:", err)
			os.Exit(1)
		}
		return
	}

	var err error
	app, err = ui.New(spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "建窗失败:", err)
		os.Exit(1)
	}
	defer app.Close()

	doc, err := app.LoadHTML(page)
	if err != nil {
		fmt.Fprintln(os.Stderr, "装载文档失败:", err)
		os.Exit(1)
	}

	bindCards(doc, app)
	wireSearch(doc, app.SetStatus)
	if *search != "" {
		if el := doc.QuerySelector("#icon-search"); el != nil {
			el.SetAttribute("value", *search)
			engine.SetFocusedElement(doc, el)
		}
	}
	if *shot != "" {
		if err := saveShot(app, *shot); err != nil {
			fmt.Fprintln(os.Stderr, "截图失败:", err)
			os.Exit(1)
		}
		return
	}
	if *exitAfter > 0 {
		app.Window().SetAutoExit(*exitAfter)
	}
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}

func buildSpec(entries []ui.IconEntry, status func(string), themeToggle ui.Action) ui.Spec {
	about := ui.Item("关于图标表", "", func() error {
		status(fmt.Sprintf("内置图标 %d 个 · Segoe MDL2 Assets", len(entries)))
		return nil
	}).WithIcon("info")
	quit := ui.Item("退出", "Alt+Q", ui.Quit).WithIcon("close")

	return ui.Spec{
		Title:       "图标表",
		Width:       winW,
		Height:      winH,
		Status:      "就绪 · 单击图标卡片复制名称",
		StatusRight: fmt.Sprintf("Segoe MDL2 Assets · %d 个图标名", len(entries)),
		Menus: []ui.Menu{
			{Title: "文件", Items: []ui.Action{quit}},
			{Title: "视图", Items: []ui.Action{themeToggle}},
			{Title: "帮助", Items: []ui.Action{about}},
		},
		Tools: []ui.Action{about, themeToggle},
	}
}

// buildPage 按 iconGroups 的顺序生成页面；未分组的图标落到「其他」。
func buildPage(entries []ui.IconEntry) string {
	byName := make(map[string]ui.IconEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	seen := make(map[string]bool, len(entries))
	type group struct {
		Title   string
		Entries []ui.IconEntry
	}
	groups := make([]group, 0, len(iconGroups)+1)
	for _, g := range iconGroups {
		var list []ui.IconEntry
		for _, name := range g.Names {
			e, ok := byName[name]
			if !ok || seen[name] {
				continue
			}
			seen[name] = true
			list = append(list, e)
		}
		if len(list) > 0 {
			groups = append(groups, group{Title: g.Title, Entries: list})
		}
	}

	var rest []ui.IconEntry
	for _, e := range entries {
		if !seen[e.Name] {
			rest = append(rest, e)
		}
	}
	if len(rest) > 0 {
		groups = append(groups, group{Title: "其他", Entries: rest})
	}

	var b strings.Builder
	b.WriteString(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<style>
/* body 的复位（外边距/颜色字体）由 ui 工作台负责，页面只写自己的排版。 */
.icons-page .icon-search { margin: 0 0 14px 0; }
.icons-page .icon-search-row .system-ui-input {
	width: 360px; margin-right: 8px; vertical-align: middle;
}
.icons-page .icon-search-row .system-ui-btn { vertical-align: middle; }
.icons-page .icon-search-result {
	display: inline-block; line-height: 30px; margin-left: 8px;
	vertical-align: middle;
}
.icons-page .icon-search-hint { margin: 4px 0 0 0; font-size: 12px; line-height: 18px; }
.icons-page .icon-empty { margin: 0 0 12px 0; }
.icons-page .icon-empty.is-hidden { display: none; }
.icons-page .icon-group { margin: 0 0 10px 0; }
.icons-page .icon-group.is-hidden { display: none; }
.icons-page .icon-grid { margin: 4px 0 0 0; }
/* 卡片和图标类同时带 system-ui-btn/system-ui-icon，主题规则挂在
   html.system-ui 下（0,2,1）。这里用三层类名（0,3,0）才压得住
   主题的尺寸/行高，否则卡片会退回 30px 高的工具栏按钮。 */
.icons-page .icon-grid .icon-card {
	display: inline-block; width: 176px; height: 136px;
	margin: 0 8px 8px 0; padding: 12px 8px; box-sizing: border-box;
	text-align: center; vertical-align: top; line-height: 16px;
}
.icons-page .icon-grid .icon-card.is-hidden { display: none; }
.icons-page .icon-grid .icon-glyph {
	display: block; font-size: 42px; line-height: 48px; margin: 0 0 6px 0;
}
.icons-page .icon-grid .icon-name { display: block; font-size: 13px; line-height: 18px; }
.icons-page .icon-grid .icon-code { display: block; font-size: 11px; line-height: 15px; }
</style>
</head>
<body>
<div class="system-ui-page icons-page">
`)
	fmt.Fprintf(&b, `<span class="system-ui-badge">ui · %d 个图标名</span>`, len(entries))
	b.WriteString(`<div class="system-ui-title">图标表</div>`)
	b.WriteString(`<div class="system-ui-subtitle">Segoe MDL2 Assets · 按名称搜索；单击任意卡片复制图标名。</div>`)
	b.WriteString(`<div class="icon-search">
<div class="system-ui-label">搜索图标名</div>
<div class="icon-search-row">
<input class="system-ui-input" id="icon-search" type="text">
<div class="system-ui-btn is-default" id="icon-search-clear">清空</div>
`)
	fmt.Fprintf(&b, `<span class="system-ui-muted icon-search-result" id="icon-search-result">共 %d 个名字</span>`, len(entries))
	b.WriteString(`
</div>
<div class="system-ui-muted icon-search-hint">输入名称片段，例如 align、volume；多个关键词用空格分开。</div>
</div>
<div class="system-ui-alert is-default icon-empty is-hidden" id="icon-empty">没有匹配的图标名。换个关键词，或点「清空」。</div>
`)

	for _, g := range groups {
		fmt.Fprintf(&b, `<div class="icon-group"><div class="system-ui-h2">%s <span class="system-ui-muted icon-count">%d</span></div><div class="icon-grid">`,
			html.EscapeString(g.Title), len(g.Entries))
		for _, e := range g.Entries {
			writeCard(&b, e)
		}
		b.WriteString(`</div></div>`)
	}

	b.WriteString(`</div></body></html>`)
	return b.String()
}

func writeCard(b *strings.Builder, e ui.IconEntry) {
	if e.Glyph == "" {
		return
	}
	r, _ := utf8.DecodeRuneInString(e.Glyph)
	code := fmt.Sprintf("U+%04X", r)
	fmt.Fprintf(b, `<div class="system-ui-btn is-default icon-card" data-icon="%s" data-code="%s">`,
		html.EscapeString(e.Name), code)
	fmt.Fprintf(b, `<span class="system-ui-icon icon-glyph">&#x%X;</span>`, r)
	fmt.Fprintf(b, `<span class="icon-name">%s</span>`, html.EscapeString(e.Name))
	fmt.Fprintf(b, `<span class="icon-code system-ui-muted">%s</span>`, code)
	b.WriteString(`</div>`)
}

func bindCards(doc engine.HTMLDocument, app *ui.App) {
	for _, el := range doc.GetElementsByClassName("icon-card") {
		bindCard(el, app)
	}
}

// bindCard 单独成函数，避免 Go 1.20 的 range 变量复用让闭包拿到最后一项。
func bindCard(el engine.HTMLElement, app *ui.App) {
	name := el.GetAttribute("data-icon")
	code := el.GetAttribute("data-code")
	el.OnClick(func(*engine.MouseEvent) {
		clipboard.SetText(name)
		app.SetStatus(fmt.Sprintf("已复制 %s · %s", name, code))
	})
}

const (
	iconCardClass  = "system-ui-btn is-default icon-card"
	iconGroupClass = "icon-group"
	iconEmptyClass = "system-ui-alert is-default icon-empty"
)

type iconCardView struct {
	name string
	el   engine.HTMLElement
}

type iconGroupView struct {
	el    engine.HTMLElement
	count engine.HTMLElement
	cards []iconCardView
	total int
}

// wireSearch 给顶部输入框接上实时过滤。
//
// 引擎没有 input 事件；输入框的 value 变化会走文档变更回调，这里借它
// 驱动过滤。过滤自身也会改 DOM（隐藏卡片、更新计数），用 filtering 挡住
// 递归。只改 class/计数文本，不重建节点，页面滚动位置不会跳。
func wireSearch(doc engine.HTMLDocument, status func(string)) {
	input := doc.QuerySelector("#icon-search")
	if input == nil {
		return
	}
	result := doc.QuerySelector("#icon-search-result")
	empty := doc.QuerySelector("#icon-empty")
	clear := doc.QuerySelector("#icon-search-clear")

	groups := make([]iconGroupView, 0, len(iconGroups)+1)
	for _, groupEl := range doc.GetElementsByClassName(iconGroupClass) {
		cards := make([]iconCardView, 0)
		for _, cardEl := range groupEl.GetElementsByClassName("icon-card") {
			cards = append(cards, iconCardView{
				name: cardEl.GetAttribute("data-icon"),
				el:   cardEl,
			})
		}
		if len(cards) == 0 {
			continue
		}
		groups = append(groups, iconGroupView{
			el:    groupEl,
			count: groupEl.QuerySelector(".icon-count"),
			cards: cards,
			total: len(cards),
		})
	}

	total := 0
	for _, g := range groups {
		total += g.total
	}

	lastQuery := "\x00"
	filtering := false
	var filter func()
	filter = func() {
		if filtering {
			return
		}
		q := strings.TrimSpace(input.GetAttribute("value"))
		if q == lastQuery {
			return
		}
		lastQuery = q
		filtering = true
		defer func() { filtering = false }()

		terms := strings.Fields(strings.ToLower(q))
		matches := 0
		for i := range groups {
			g := &groups[i]
			n := 0
			for _, c := range g.cards {
				ok := matchIconName(c.name, terms)
				if ok {
					n++
				}
				setHidden(c.el, iconCardClass, !ok)
			}
			matches += n
			setHidden(g.el, iconGroupClass, q != "" && n == 0)
			if g.count != nil {
				text := fmt.Sprintf("%d", g.total)
				if q != "" {
					text = fmt.Sprintf("%d/%d", n, g.total)
				}
				if g.count.Text() != text {
					g.count.SetText(text)
				}
			}
		}

		if result != nil {
			text := fmt.Sprintf("共 %d 个名字", total)
			if q != "" {
				text = fmt.Sprintf("找到 %d 个名字", matches)
			}
			if result.Text() != text {
				result.SetText(text)
			}
		}
		if empty != nil {
			setHidden(empty, iconEmptyClass, !(q != "" && matches == 0))
		}

		if status != nil {
			switch {
			case q == "":
				status("就绪 · 单击图标卡片复制名称")
			case matches == 0:
				status(fmt.Sprintf("没有匹配 %q 的图标名", q))
			default:
				status(fmt.Sprintf("搜索 %q · %d 个图标名", q, matches))
			}
		}
	}

	// 输入框的 value 变化（打字/退格/粘贴/清空）都会触发文档变更回调；
	// 回调里只在查询串真的变了时才重排可见性。
	doc.SetOnChanged(func() {
		if filtering {
			return
		}
		filter()
	})

	if clear != nil {
		clear.OnClick(func(*engine.MouseEvent) {
			input.SetAttribute("value", "")
			engine.SetFocusedElement(doc, input)
		})
	}
	input.OnKeyDown(func(ev *engine.KeyEvent) bool {
		if ev.Key == engine.KeyEscape {
			input.SetAttribute("value", "")
			return true
		}
		return false
	})

	filter()
}

func matchIconName(name string, terms []string) bool {
	name = strings.ToLower(name)
	for _, term := range terms {
		if !strings.Contains(name, term) {
			return false
		}
	}
	return true
}

func setHidden(el engine.HTMLElement, base string, hidden bool) {
	class := base
	if hidden {
		class += " is-hidden"
	}
	if el.GetAttribute("class") != class {
		el.SetAttribute("class", class)
	}
}

func dump(spec ui.Spec, page, search, pngPath, svgPath string) error {
	vp, buf := engine.NewBufferedViewport(winW, winH)
	doc, err := engine.OpenDocument(vp, page)
	if err != nil {
		return err
	}
	ui.Install(doc, spec)
	wireSearch(doc, nil)
	if search != "" {
		if el := doc.QuerySelector("#icon-search"); el != nil {
			el.SetAttribute("value", search)
		}
	}
	doc.Refresh()
	engine.RenderNode(buf, doc)
	if pngPath != "" {
		if err := buf.SavePNG(pngPath); err != nil {
			return err
		}
	}
	if svgPath != "" {
		return os.WriteFile(svgPath, []byte(doc.DumpSVG()), 0o644)
	}
	return nil
}

func saveShot(app *ui.App, path string) error {
	if doc := app.Workbench().Doc(); doc != nil {
		doc.Refresh()
	}
	app.Window().RenderFrame()
	raw, _, w, h, err := app.Window().Screenshot()
	if err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			o := y*img.Stride + x*4
			img.Pix[o] = raw[i]
			img.Pix[o+1] = raw[i+1]
			img.Pix[o+2] = raw[i+2]
			img.Pix[o+3] = 255
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
