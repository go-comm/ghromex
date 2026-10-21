// 示例：一个最小的本地浏览器。
//
// 起始页用 go:embed 打进二进制；地址栏、后退/前进/刷新/首页按钮与底部
// 状态栏由 installChrome 注入到每一份加载的文档里。引擎没有独立的窗口
// chrome 层，所以外壳只能作为文档的一部分：position:fixed 让工具条固定
// 在顶部，换页时重新注入，因此打开任何页面（包括错误页）都保留同一套
// 导航控件与状态栏。
//
// 点击链接或地址栏「打开」都由本程序读取目标文件并替换窗口文档；相对路径
// 按当前页面所在目录解析，file:// 也可用；http(s):// 不联网，改为显示
// 提示页；文件不存在则显示错误页。
//
//	go run ./examples/browser
//	go run ./examples/browser -page examples/ua/index.html
//	go run ./examples/browser -exit-after 3s
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

//go:embed index.html
var homeHTML []byte

const (
	// homeURL 是虚拟地址：起始页来自内嵌内容，磁盘上不一定有对应文件。
	homeURL = "about:home"
	// chromeHeight 是顶部工具条高度，statusHeight 是底部状态栏高度。
	chromeHeight = 44
	statusHeight = 26
	// 工具条控件的绝对坐标与尺寸（见 installChrome）。
	chromeTop         = 6
	chromeLeft        = 10
	chromeButtonWidth = 40
	chromeButtonGap   = 6
	// chromeGoWidth 是地址栏右侧「打开」按钮的宽度。
	chromeGoWidth = 64
)

func main() {
	start := flag.String("page", "", "启动时打开的本地 HTML 文件（默认为内嵌起始页）")
	exitAfter := flag.Duration("exit-after", 0, "指定时长后自动退出，如 3s（调试/CI 用）")
	dumpSVG := flag.String("dump-svg", "", "无头模式：完成级联与布局后把结果导出为 SVG 并退出")
	flag.Parse()

	if *dumpSVG != "" {
		// 无头导出也走完整的浏览器外壳，便于检查工具条与页面内容是否重叠。
		hw := &headlessWindow{vp: engine.NewHeadlessViewport(1024, 720)}
		app := &browserApp{win: hw, homeDir: findHomeDir(), index: -1}
		if *start != "" {
			abs, err := filepath.Abs(*start)
			if err != nil {
				fmt.Fprintln(os.Stderr, "打开文档失败:", err)
				os.Exit(1)
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				fmt.Fprintln(os.Stderr, "打开文档失败:", err)
				os.Exit(1)
			}
			app.show(abs, filepath.Dir(abs), data, true)
		} else {
			app.showHome()
		}
		if hw.doc == nil {
			fmt.Fprintln(os.Stderr, "没有可导出的文档")
			os.Exit(1)
		}
		// 注入工具条后布局还是旧的，先重排再导出（真实窗口由后端循环代劳）。
		hw.doc.Refresh()
		if err := os.WriteFile(*dumpSVG, []byte(hw.doc.DumpSVG()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("SVG 已写入:", *dumpSVG)
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

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}

// findHomeDir 返回起始页相对链接的基准目录。
//
// 起始页里的链接写成 ../login/index.html，是按 examples/browser/index.html
// 的磁盘位置写的；但页面本身是内嵌的，运行目录可能任意。这里按常见启动
// 方式依次探测：仓库根（go run ./examples/browser）→ examples 目录内
// （cd examples; go run ./browser）→ 示例目录内（go run .）。都找不到时
// 返回空串，相对链接会走错误页而不是悄悄跳错文件。
func findHomeDir() string {
	for _, dir := range []string{filepath.Join("examples", "browser"), "browser", "."} {
		if st, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !st.IsDir() {
			return dir
		}
	}
	return ""
}

type browserApp struct {
	win     opener
	homeDir string   // 起始页相对链接的基准目录
	dir     string   // 当前页面所在目录，相对链接按它解析
	current string   // 当前地址：homeURL 或文件路径
	history []string // 浏览历史，元素含义同 current
	index   int      // 当前页在 history 中的下标；初始为 -1（空历史）

	// 工具条里的地址栏与「打开」按钮：宽度/位置都跟着视口宽度走，
	// 窗口最大化或还原后要重排（见 syncChrome）。
	addrIn        engine.HTMLElement
	goBtn         engine.HTMLElement
	lastAddrWidth int
}

// opener 是浏览器用到的窗口能力；*browser.Window 满足它，测试可以用无头
// 视口替代，从而不需要真实窗口。
type opener interface {
	OpenDocument(src string) (engine.HTMLDocument, error)
	ViewportWidth() int
}

// headlessWindow 让 -dump-svg 无需真实窗口也能走完整的浏览器加载流程
// （包括注入工具条），导出的 SVG 与窗口里看到的一致。
type headlessWindow struct {
	vp  engine.Viewport
	doc engine.HTMLDocument
}

func (h *headlessWindow) OpenDocument(src string) (engine.HTMLDocument, error) {
	doc, err := engine.OpenDocument(h.vp, src)
	if err == nil {
		h.doc = doc
	}
	return doc, err
}

func (h *headlessWindow) ViewportWidth() int { return h.vp.ViewportWidth() }

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

// show 用 data 替换窗口文档，并注入浏览器工具条。commit 为 true 时把
// 地址写入历史（并丢弃当前页之后的前进分支）。
func (b *browserApp) show(addr, dir string, data []byte, commit bool) bool {
	b.current, b.dir = addr, dir
	if commit {
		b.commit(addr)
	}
	doc, err := b.win.OpenDocument(string(data))
	if err != nil {
		b.showMessage("HTML 解析失败", addr, err.Error())
		return false
	}
	doc.SetOnNavigate(b.gotoAddress)
	doc.SetOnChanged(b.onChanged)
	b.installChrome(doc)
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

// showMessage 显示一页提示/错误信息；工具条照常注入，导航按钮仍可用。
// current/history 由调用方先更新（失败地址也算历史的一页，刷新会重试）。
func (b *browserApp) showMessage(title, subject, detail string) {
	page := messagePage(title, subject, detail, b.index > 0)
	doc, err := b.win.OpenDocument(page)
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法显示提示页:", err)
		return
	}
	doc.SetOnNavigate(b.gotoAddress)
	doc.SetOnChanged(b.onChanged)
	b.installChrome(doc)
}

// installChrome 在文档里注入浏览器外壳：顶部工具条 + 底部状态栏。
//
// 外壳是文档的一部分（引擎没有窗口 chrome 层）：工具条 position:fixed，
// 滚动时留在顶部，z-index 拉高保证盖住页面内容。每次换文档都重新注入，
// 所以地址栏和按钮在任意页面都在。
//
// 给内容让位用的是插在页面内容之前的一个占位块，而不是改 body 的
// margin/padding：
//   - 改 padding-top 会覆盖页面自己的内边距（很多示例页都有）；
//   - 改 margin-top 更糟：body 的盒子会被推到工具条下方，命中测试认为
//     固定工具条不在 body 里，地址栏就点不到了。
//
// 引擎没有 insertBefore，占位块要排到最前面只能把原有子节点摘下来再
// 按原序接回去。
func (b *browserApp) installChrome(doc engine.HTMLDocument) {
	body := doc.Body()
	doc.AddStylesheetText(chromeCSS)

	bar := doc.CreateElement("div")
	bar.SetAttribute("id", "ghromex-chrome")
	bar.SetAttribute("class", "gh-bar")

	// 控件绝对定位：引擎的 inline-block 按基线对齐，按钮和输入框混排时
	// 会互相顶偏；固定坐标更可控，也便于地址栏占满剩余宽度。
	// 没有可后退/前进的页面时按钮置灰（样式类不同，但仍然可点，点了无事发生）。
	back := b.chromeButton(doc, "ghromex-back", "←", chromeLeft, b.index <= 0, b.goBack)
	forward := b.chromeButton(doc, "ghromex-forward", "→", chromeLeft+chromeButtonWidth+chromeButtonGap,
		b.index+1 >= len(b.history), b.goForward)
	refresh := b.chromeButton(doc, "ghromex-refresh", "↻", chromeLeft+2*(chromeButtonWidth+chromeButtonGap),
		false, b.refresh)
	home := b.chromeButton(doc, "ghromex-home", "⌂", chromeLeft+3*(chromeButtonWidth+chromeButtonGap),
		false, b.showHome)

	inputLeft := chromeLeft + 4*(chromeButtonWidth+chromeButtonGap)
	width := b.win.ViewportWidth() - inputLeft - (chromeGoWidth + chromeButtonGap + chromeLeft)
	if width < 240 {
		width = 240
	}
	addr := doc.CreateElement("input")
	addr.SetAttribute("id", "ghromex-addr")
	addr.SetAttribute("class", "gh-addr")
	addr.SetAttribute("value", b.current)
	addr.SetAttribute("style", fmt.Sprintf("left:%dpx;width:%dpx", inputLeft, width))

	open := doc.CreateElement("button")
	open.SetAttribute("id", "ghromex-go")
	open.SetAttribute("class", "gh-go")
	open.SetAttribute("style", fmt.Sprintf("left:%dpx;width:%dpx", inputLeft+width+chromeButtonGap, chromeGoWidth))
	open.SetText("打开")
	b.addrIn, b.goBtn = addr, open
	b.lastAddrWidth = width
	open.OnClick(func(*engine.MouseEvent) {
		b.gotoAddress(addr.GetAttribute("value"))
	})

	bar.AppendChild(back, forward, refresh, home, addr, open)
	body.AppendChild(bar)

	// 底部状态栏：左侧是页面标题，右侧是说明。fixed 定位，不占文档流。
	status := doc.CreateElement("div")
	status.SetAttribute("id", "ghromex-status")
	status.SetAttribute("class", "gh-status")
	title := doc.CreateElement("span")
	title.SetAttribute("class", "gh-status-title")
	title.SetText(pageTitle(doc))
	hint := doc.CreateElement("span")
	hint.SetAttribute("class", "gh-status-hint")
	hint.SetText("只浏览本地文件 · 不联网")
	status.AppendChild(title, hint)
	body.AppendChild(status)

	// 状态栏是 fixed，会盖住页面底部：末尾补一个占位块把内容托起来。
	pad := doc.CreateElement("div")
	pad.SetAttribute("id", "ghromex-spacer-bottom")
	pad.SetAttribute("class", "gh-spacer-bottom")
	body.AppendChild(pad)

	// 顶部占位块要排在页面内容之前：把原有子节点摘下来，插入占位块后
	// 再按原序接回去（Children 返回的是底层数组，RemoveChild 会就地
	// 改动它，所以先复制一份）。
	children := append([]engine.HTMLElement(nil), body.Children()...)
	top := doc.CreateElement("div")
	top.SetAttribute("id", "ghromex-spacer-top")
	top.SetAttribute("class", "gh-spacer-top")
	for _, c := range children {
		body.RemoveChild(c)
	}
	body.AppendChild(top)
	if len(children) > 0 {
		body.AppendChild(children[0], children[1:]...)
	}
}

// onChanged 是文档变更回调：视口尺寸变化时重排工具条。
//
// 引擎在「视口尺寸变化」时也会补一次变更通知（窗口缩放不改文档内容，
// 只靠内容变更收不到信号），因此这里统一处理两种来源。
func (b *browserApp) onChanged() {
	b.syncChrome()
}

// syncChrome 按当前视口宽度重排工具条里的地址栏与「打开」按钮。
//
// 两者都贴着窗口右边缘，宽度是算出来的：窗口最大化/还原后视口宽度变了，
// 不重排的话「打开」按钮就停在旧位置（不再右对齐）。
func (b *browserApp) syncChrome() {
	if b.addrIn == nil || b.goBtn == nil {
		return
	}
	inputLeft := chromeLeft + 4*(chromeButtonWidth+chromeButtonGap)
	width := b.win.ViewportWidth() - inputLeft - (chromeGoWidth + chromeButtonGap + chromeLeft)
	if width < 240 {
		width = 240
	}
	if width == b.lastAddrWidth {
		return
	}
	b.lastAddrWidth = width
	b.addrIn.SetAttribute("style", fmt.Sprintf("left:%dpx;width:%dpx", inputLeft, width))
	b.goBtn.SetAttribute("style", fmt.Sprintf("left:%dpx;width:%dpx", inputLeft+width+chromeButtonGap, chromeGoWidth))
}

// pageTitle 取页面标题（head > title），没有标题时给个占位文案。
func pageTitle(doc engine.HTMLDocument) string {
	if t := strings.TrimSpace(doc.Title()); t != "" {
		return t
	}
	return "无标题"
}

// chromeButton 创建一个工具条按钮并放到工具条的绝对坐标 left。工具条在
// 每份新文档上重建，因此事件处理也在这里按当前状态重新绑定。off 为真时
// 按钮置灰（没有可后退/前进的页面）。
func (b *browserApp) chromeButton(doc engine.HTMLDocument, id, label string, left int, off bool, onClick func()) engine.HTMLElement {
	btn := doc.CreateElement("button")
	btn.SetAttribute("id", id)
	btn.SetAttribute("class", "gh-btn")
	if off {
		btn.SetAttribute("class", "gh-btn gh-btn-off")
	}
	btn.SetAttribute("style", fmt.Sprintf("left:%dpx", left))
	btn.SetText(label)
	btn.OnClick(func(*engine.MouseEvent) { onClick() })
	return btn
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
