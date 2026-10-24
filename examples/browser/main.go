// 示例：一个最小的本地浏览器。
//
// 起始页用 go:embed 打进二进制；外壳用 ui 工作台装配（见 installChrome）：
// 顶部工具栏是后退/前进/刷新/首页四个命令加两个控件槽位（示例页面下拉、
// 地址输入框），底部状态栏左侧是页面标题。引擎没有独立的窗口 chrome 层，
// 所以外壳仍作为文档的一部分：每换一份文档重新 Install 一套，打开任何
// 页面（包括错误页）都保留同一套导航控件与状态栏。
//
// 点击链接或地址栏「打开」都由本程序读取目标文件并替换窗口文档；相对路径
// 按当前页面所在目录解析，file:// 也可用；http(s):// 不联网，改为显示
// 提示页；文件不存在则显示错误页。
//
//	go run ./examples/browser
//	go run ./examples/browser -page examples/ua/index.html
//	go run ./examples/browser -exit-after 3s
//	go run ./examples/browser -dump-png out.png   # 无头出图：软件光栅，核对布局/配色
//	go run ./examples/browser -dump-svg out.svg   # 无头出图：SVG，核对布局/层级
//	go run ./examples/browser -shot out.png       # 真实窗口截图：真实字体渲染一帧存 PNG
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/ui"
)

//go:embed index.html
var homeHTML []byte

// homeURL 是虚拟地址：起始页来自内嵌内容，磁盘上不一定有对应文件。
const homeURL = "about:home"

func main() {
	start := flag.String("page", "", "启动时打开的本地 HTML 文件（默认为内嵌起始页）")
	exitAfter := flag.Duration("exit-after", 0, "指定时长后自动退出，如 3s（调试/CI 用）")
	dumpPNG := flag.String("dump-png", "", "无头出图：软件光栅 PNG，文字是占位方块，断言布局/配色用")
	dumpSVG := flag.String("dump-svg", "", "无头出图：级联+布局导出 SVG（真实 <text>），核对几何/层级/配色")
	shot := flag.String("shot", "", "真实窗口截图：真实字体渲染一帧存 PNG（Window.Screenshot 回读）")
	flag.Parse()

	if *dumpPNG != "" || *dumpSVG != "" {
		if err := dumpHeadless(*start, *dumpPNG, *dumpSVG); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	win, err := browser.New(1024, 720, "Ghromex 浏览器")
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	defer win.Close()

	app := &browserApp{win: win, homeDir: findHomeDir(), index: -1}
	if *start != "" {
		// -page 的相对路径按调用者的工作目录解析，符合命令行工具直觉。
		if abs, err := filepath.Abs(*start); err == nil {
			*start = abs
		}
		data, err := os.ReadFile(*start)
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开文档失败:", err)
			os.Exit(1)
		}
		app.show(*start, filepath.Dir(*start), data, true)
	} else {
		app.showHome()
	}

	if *shot != "" {
		if err := saveShot(win, app.doc, *shot); err != nil {
			fmt.Fprintln(os.Stderr, "截图失败:", err)
			os.Exit(1)
		}
		fmt.Println("PNG 已写入:", *shot)
		return
	}

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}

// saveShot 用真实字体渲染一帧并截图：Refresh 重排、RenderFrame 画一帧、
// Window.Screenshot 回读窗口像素后本地编码 PNG。
//
// 与 -dump-png 的区别：那条路用软件光栅后端，文字是占位方块（用途是逐
// 像素断言）；这条走真实字体与 SDL 渲染器，用来看最终观感。
func saveShot(win *browser.Window, doc engine.HTMLDocument, path string) error {
	// installChrome 刚改过 DOM，必须先重排再画：RenderFrame 只负责
	// 「清屏 + 画当前布局」，不会自己重排（主循环里由 ChangeCount 驱动）。
	if doc != nil {
		doc.Refresh()
	}
	win.RenderFrame()
	raw, _, w, h, err := win.Screenshot()
	if err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// 后端回读格式为 XBGR8888（小端内存序 R,G,B,X）：前三个字节就是
	// 颜色，第 4 字节不是 alpha，统一补 255。
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

// dumpHeadless 处理 -dump-png / -dump-svg：不建真实窗口，也走完整的浏览器
// 外壳（装工作台），导出的结果与窗口里看到的同源。
func dumpHeadless(start, pngPath, svgPath string) error {
	vp, buf := engine.NewBufferedViewport(1024, 720)
	hw := &headlessWindow{vp: vp, buf: buf}
	app := &browserApp{win: hw, homeDir: findHomeDir(), index: -1}
	if start != "" {
		abs, err := filepath.Abs(start)
		if err != nil {
			return fmt.Errorf("打开文档失败: %w", err)
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("打开文档失败: %w", err)
		}
		app.show(abs, filepath.Dir(abs), data, true)
	} else {
		app.showHome()
	}
	if hw.doc == nil {
		return fmt.Errorf("没有可导出的文档")
	}
	// 装配工具栏后布局还是旧的，先重排再导出（真实窗口由主循环代劳）。
	hw.doc.Refresh()
	if pngPath != "" {
		engine.RenderNode(hw.buf, hw.doc)
		if err := hw.buf.DumpPNG(pngPath); err != nil {
			return err
		}
		fmt.Println("PNG 已写入:", pngPath)
	}
	if svgPath != "" {
		if err := os.WriteFile(svgPath, []byte(hw.doc.DumpSVG()), 0o644); err != nil {
			return err
		}
		fmt.Println("SVG 已写入:", svgPath)
	}
	return nil
}

// findHomeDir 返回起始页相对链接的基准目录（绝对路径）。
//
// 起始页里的链接写成 ../ua/index.html，是按 examples/browser/index.html
// 的磁盘位置写的；但页面本身是内嵌的，运行目录可能任意。这里按常见启动
// 方式依次探测：仓库根（go run ./examples/browser）→ examples 目录内
// （cd examples; go run ./browser）→ 示例目录内（go run .）。都找不到时
// 返回空串，相对链接会走错误页而不是悄悄跳错文件。
//
// 返回值转成绝对路径：工具栏的示例页面下拉按它解析选项，当前页在别的
// 目录时相对路径才不会跑偏。
func findHomeDir() string {
	for _, dir := range []string{filepath.Join("examples", "browser"), "browser", "."} {
		if st, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !st.IsDir() {
			if abs, err := filepath.Abs(dir); err == nil {
				return abs
			}
			return dir
		}
	}
	return ""
}

type browserApp struct {
	win     opener
	doc     engine.HTMLDocument // 当前文档（截图前 Refresh 用），show 打开成功时更新
	homeDir string              // 起始页相对链接的基准目录
	dir     string              // 当前页面所在目录，相对链接按它解析
	current string              // 当前地址：homeURL 或文件路径
	history []string            // 浏览历史，元素含义同 current
	index   int                 // 当前页在 history 中的下标；初始为 -1（空历史）
}

// opener 是浏览器用到的窗口能力；*browser.Window 满足它，测试可以用无头
// 视口替代，从而不需要真实窗口。
type opener interface {
	OpenDocument(src string) (engine.HTMLDocument, error)
	// OpenDocumentWith 允许传文档选项：<link rel="stylesheet"> 与 @import
	// 在解析阶段就要加载器，打开后再补是补不上的。
	OpenDocumentWith(src string, opts engine.DocumentOptions) (engine.HTMLDocument, error)
}

// headlessWindow 让 -dump-png / -dump-svg 无需真实窗口也能走完整的浏览器
// 加载流程（包括装工作台），导出的结果与窗口里看到的同源。buf 是软件光栅
// 缓冲，只在 -dump-png 时消费。
type headlessWindow struct {
	vp  engine.Viewport
	buf *engine.BufferGraphics
	doc engine.HTMLDocument
}

func (h *headlessWindow) OpenDocument(src string) (engine.HTMLDocument, error) {
	return h.OpenDocumentWith(src, engine.DocumentOptions{})
}

func (h *headlessWindow) OpenDocumentWith(src string, opts engine.DocumentOptions) (engine.HTMLDocument, error) {
	doc, err := engine.OpenDocumentWith(h.vp, src, opts)
	if err == nil {
		h.doc = doc
	}
	return doc, err
}

// gotoAddress 是文档导航钩子，也是地址栏「打开」的入口。
func (b *browserApp) gotoAddress(href string) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return // 页内锚点：引擎暂未提供滚动到锚点的 API，先忽略
	}
	switch href {
	case homeURL:
		b.showHome()
		return
	case "about:back":
		b.goBack()
		return
	case "about:forward":
		b.goForward()
		return
	}
	if isExternal(href) {
		// 外部地址也记入历史：地址栏保持用户输入的内容，后退能回到来路。
		b.current = href
		b.commit(href)
		b.showMessage("外部链接", href, "本示例只浏览本地文件、不联网。请把该地址复制到系统浏览器打开。")
		return
	}

	path := href
	if strings.HasPrefix(path, "file://") {
		path = strings.TrimPrefix(path, "file://")
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:] // file:///C:/x → /C:/x → C:/x
		}
		path = filepath.FromSlash(path)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(b.dir, filepath.FromSlash(path))
	}
	path = filepath.Clean(path)

	data, err := os.ReadFile(path)
	if err != nil {
		b.current, b.dir = path, filepath.Dir(path)
		b.commit(path)
		b.showMessage("无法打开页面", path, err.Error())
		return
	}
	b.show(path, filepath.Dir(path), data, true)
}

// isExternal 判断是否是本示例不会联网打开的 http(s) 地址。
func isExternal(addr string) bool {
	return strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://")
}

// showHome 载入内嵌起始页。
func (b *browserApp) showHome() {
	b.show(homeURL, b.homeDir, homeHTML, true)
}

// goBack 回到历史中的上一页；已在最旧一页时不做任何事。
func (b *browserApp) goBack() {
	if b.index <= 0 {
		return
	}
	b.index--
	b.loadHistory(b.history[b.index])
}

// goForward 前进到历史中的下一页；已在最新一页时不做任何事。
func (b *browserApp) goForward() {
	if b.index+1 >= len(b.history) {
		return
	}
	b.index++
	b.loadHistory(b.history[b.index])
}

// refresh 重新加载当前页（文件会重新读盘，起始页重新用内嵌内容）。
func (b *browserApp) refresh() {
	b.loadHistory(b.current)
}

// loadHistory 按地址加载历史中的一页，不改变 history/index。
func (b *browserApp) loadHistory(addr string) {
	if addr == homeURL {
		b.show(homeURL, b.homeDir, homeHTML, false)
		return
	}
	if isExternal(addr) {
		b.current = addr
		b.showMessage("外部链接", addr, "本示例只浏览本地文件、不联网。请把该地址复制到系统浏览器打开。")
		return
	}
	data, err := os.ReadFile(addr)
	if err != nil {
		b.current, b.dir = addr, filepath.Dir(addr)
		b.showMessage("无法打开页面", addr, err.Error())
		return
	}
	b.show(addr, filepath.Dir(addr), data, false)
}

// show 用 data 替换窗口文档，并装上浏览器工作台。commit 为 true 时把
// 地址写入历史（并丢弃当前页之后的前进分支）。
func (b *browserApp) show(addr, dir string, data []byte, commit bool) bool {
	b.current, b.dir = addr, dir
	if commit {
		b.commit(addr)
	}
	doc, err := b.win.OpenDocumentWith(string(data), engine.DocumentOptions{
		BaseURL:        pageBaseURL(addr, dir),
		LoadStylesheet: loadLocalStylesheet,
	})
	if err != nil {
		b.showMessage("HTML 解析失败", addr, err.Error())
		return false
	}
	doc.SetOnNavigate(b.gotoAddress)
	b.installChrome(doc)
	b.doc = doc
	return true
}

// commit 把 addr 作为新的一页写入历史。与当前页相同时不重复入栈
// （例如已经在起始页又点「首页」），否则截断前进分支。
func (b *browserApp) commit(addr string) {
	if b.index >= 0 && b.history[b.index] == addr {
		return
	}
	b.history = append(b.history[:b.index+1], addr)
	b.index = len(b.history) - 1
}

// showMessage 显示一页提示/错误信息；工作台照常装配，导航按钮仍可用。
// current/history 由调用方先更新（失败地址也算历史的一页，刷新会重试）。
func (b *browserApp) showMessage(title, subject, detail string) {
	page := messagePage(title, subject, detail, b.index > 0)
	doc, err := b.win.OpenDocument(page)
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法显示提示页:", err)
		return
	}
	doc.SetOnNavigate(b.gotoAddress)
	b.installChrome(doc)
}

// installChrome在给文档装上浏览器外壳：ui 工作台（工具栏 + 状态栏）。
//
// 工作台是文档的一部分（引擎没有窗口 chrome 层）：工具栏与状态栏
// position:fixed 压在页面上，页面内容前后的占位块由 ui.Install 负责插入，
// 每换一份文档重装一套。
//
// NoBodyReset 必须打开：示例页各有自己的 body 排版——起始页与门户用
// body padding 给固定栏让位，UA 对照页更是把 body 的 UA 外边距当演示内容，
// 工作台默认的 body 复位（根作用域下特异度更高）会把它们顶歪。
func (b *browserApp) installChrome(doc engine.HTMLDocument) {
	ui.Install(doc, ui.Spec{
		NoBodyReset: true,
		Tools:       b.navTools(doc),
		StatusSegments: []ui.StatusSegment{
			{Key: "page", Text: pageTitle(doc)},
		},
		StatusRight: "只浏览本地文件 · 不联网",
	})
}

// navTools 拼工具栏这一排：后退/前进/刷新/首页四个命令，加两个控件槽位
// （示例页面下拉、地址输入框）和「打开」按钮。
//
// 每换一份文档都重装一次工作台，所以按钮可用态（When）与地址栏初值都在
// 这里现取；换页后下拉也自动回到占位项，不必手动复位。
func (b *browserApp) navTools(doc engine.HTMLDocument) []ui.Action {
	// 地址栏：回车与「打开」按钮走同一条路径。
	addr := ui.NewTextInput(doc, b.current)
	addr.OnKeyDown(func(ev *engine.KeyEvent) bool {
		if ev.Key != engine.KeyEnter || ev.Mods&engine.ModCtrl != 0 {
			return false
		}
		b.gotoAddress(addr.Value())
		return true
	})
	open := func() { b.gotoAddress(addr.Value()) }

	pages := ui.NewSelect(doc, append([]ui.SelectOption{{Value: pagesPlaceholder, Label: "打开示例页面…"}}, examplePages...))
	pages.OnChange(b.openExample)

	return []ui.Action{
		ui.Tool("后退", asCmd(b.goBack)).WithIcon("back").When(func() bool { return b.index > 0 }),
		ui.Tool("前进", asCmd(b.goForward)).WithIcon("forward").When(func() bool { return b.index+1 < len(b.history) }),
		ui.Tool("刷新", asCmd(b.refresh)).WithIcon("refresh"),
		ui.Tool("首页", asCmd(b.showHome)).WithIcon("home"),
		ui.Sep(),
		ui.Slot(pages),
		ui.Slot(addr),
		ui.Tool("打开", asCmd(open)),
	}
}

// examplePages 是示例页面下拉里的内置入口。路径按起始页所在目录（homeDir）
// 解析——当前页在别的目录时也不会跟着跑偏。
var examplePages = []ui.SelectOption{
	{Value: "../ua/index.html", Label: "UA 与表单控件"},
	{Value: "../portal/index.html", Label: "购物门户"},
	{Value: "external.html", Label: "外部样式表"},
}

// pagesPlaceholder 是下拉第一项的取值：option 没写 value 时按 HTML 语义
// 回退为可见文字，占位项要给一个明确不会命中示例页面的值。
const pagesPlaceholder = "placeholder"

// openExample 打开下拉里选中的示例页面；占位项不动作。
func (b *browserApp) openExample(rel string) {
	if rel == "" || rel == pagesPlaceholder {
		return
	}
	b.gotoAddress(filepath.Join(b.homeDir, filepath.FromSlash(rel)))
}

// asCmd 把无返回值的窗口动作适配成 ui 命令：工具栏按钮要的是 func() error。
func asCmd(f func()) func() error {
	return func() error {
		f()
		return nil
	}
}

// pageTitle 取页面标题（head > title），没有标题时给个占位文案。
func pageTitle(doc engine.HTMLDocument) string {
	if t := strings.TrimSpace(doc.Title()); t != "" {
		return t
	}
	return "无标题"
}

// messagePage 生成提示页（外部链接 / 打不开的文件）。subject/detail 来自
// 地址与错误信息，必须转义，否则文件名里的 <、& 会被当成标签解析。
func messagePage(title, subject, detail string, canBack bool) string {
	back := ""
	if canBack {
		back = ` · <a href="about:back">返回上一页</a>`
	}
	return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>Ghromex 浏览器 · ` + html.EscapeString(title) + `</title>
<style>
	body {
		margin: 0;
		padding: 56px 32px;
		background-color: #F1F5F9;
		color: #0F172A;
		font-family: system-ui;
		font-size: 14px;
		text-align: center;
	}
	.card {
		display: inline-block;
		vertical-align: top;
		width: 520px;
		padding: 32px 32px 28px 32px;
		background-color: #FFFFFF;
		border: 1px solid #E2E8F0;
		border-radius: 14px;
		box-sizing: border-box;
		text-align: left;
	}
	.badge {
		display: inline-block;
		width: 36px;
		height: 36px;
		border-radius: 10px;
		background-color: #FEF3C7;
		border: 1px solid #FDE68A;
		color: #B45309;
		font-size: 18px;
		font-weight: bold;
		text-align: center;
	}
	h1 { margin: 16px 0 12px 0; font-size: 22px; }
	.subject {
		margin: 0 0 10px 0;
		padding: 8px 10px;
		background-color: #F8FAFC;
		border: 1px solid #E2E8F0;
		border-radius: 8px;
		color: #1D4ED8;
		font-family: monospace;
		font-size: 13px;
	}
	.detail { margin: 0 0 22px 0; color: #475569; }
	.nav { margin: 0; }
	.nav a { color: #1D4ED8; }
</style>
</head>
<body>
	<div class="card">
		<span class="badge">!</span>
		<h1>` + html.EscapeString(title) + `</h1>
		<p class="subject">` + html.EscapeString(subject) + `</p>
		<p class="detail">` + html.EscapeString(detail) + `</p>
		<p class="nav"><a href="` + homeURL + `">回到起始页</a>` + back + `</p>
	</div>
</body>
</html>
`
}
