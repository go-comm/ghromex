package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

type headlessOpener struct {
	vp  engine.Viewport
	doc engine.HTMLDocument
}

func (h *headlessOpener) OpenDocument(src string) (engine.HTMLDocument, error) {
	return h.OpenDocumentWith(src, engine.DocumentOptions{})
}

func (h *headlessOpener) OpenDocumentWith(src string, opts engine.DocumentOptions) (engine.HTMLDocument, error) {
	doc, err := engine.OpenDocumentWith(h.vp, src, opts)
	if err == nil {
		h.doc = doc
	}
	return doc, err
}

func TestLinkNavigation(t *testing.T) {
	h := &headlessOpener{vp: engine.NewHeadlessViewport(1024, 720)}
	app := &browserApp{win: h, homeDir: ".", index: -1}

	// 无头视口不会自动触发布局，点击前先 Refresh，模拟后端的换页重排。
	clickCenter := func(el engine.HTMLElement) {
		t.Helper()
		h.doc.Refresh()
		r := el.GetBoundingClientRect()
		engine.OnDocumentClick(h.doc, (r.Left().Pixel()+r.Right().Pixel())/2, (r.Top().Pixel()+r.Bottom().Pixel())/2)
	}
	// 工作台控件按 ui 的类名与 title 找：工具栏按钮的悬停提示就是 Label
	// （见 ui.toolTip），不必给控件单独设 id。
	clickTool := func(title string) {
		t.Helper()
		el := h.doc.QuerySelector(`.system-ui-tool[title="` + title + `"]`)
		if el == nil {
			t.Fatalf("找不到工具按钮 %q", title)
		}
		clickCenter(el)
	}
	addrEl := func() engine.HTMLElement {
		t.Helper()
		el := h.doc.QuerySelector(".system-ui-tool-slot .system-ui-input")
		if el == nil {
			t.Fatal("工具栏里没有地址栏")
		}
		return el
	}
	clickLink := func(part string) {
		t.Helper()
		h.doc.Refresh()
		for _, a := range h.doc.GetElementsByTagName("a") {
			if strings.Contains(a.GetAttribute("href"), part) {
				r := a.GetBoundingClientRect()
				engine.OnDocumentClick(h.doc, (r.Left().Pixel()+r.Right().Pixel())/2, (r.Top().Pixel()+r.Bottom().Pixel())/2)
				return
			}
		}
		t.Fatalf("找不到包含 %q 的链接", part)
	}
	assertTitle := func(want string) {
		t.Helper()
		if got := h.doc.Title(); got != want {
			t.Fatalf("标题 = %q, want %q", got, want)
		}
	}
	// 外壳由 ui 工作台装配：工具栏（5 个按钮 + 示例页面下拉 + address input）
	// 与状态栏在每份文档上都必须在场。
	assertChrome := func() {
		t.Helper()
		for _, sel := range []string{
			".system-ui-toolbar", ".system-ui-status",
			`.system-ui-tool[title="后退"]`, `.system-ui-tool[title="前进"]`,
			`.system-ui-tool[title="刷新"]`, `.system-ui-tool[title="首页"]`,
			`.system-ui-tool[title="打开"]`,
			".system-ui-tool-slot .system-ui-input", ".system-ui-tool-slot .system-ui-select",
		} {
			if h.doc.QuerySelector(sel) == nil {
				t.Fatalf("当前页面缺少工作台元素 %s", sel)
			}
		}
	}

	app.showHome()
	assertTitle("Ghromex 浏览器 · 起始页")
	assertChrome()
	if got := engine.GetValue(addrEl()); got != homeURL {
		t.Fatalf("起始页地址栏 = %q, want %q", got, homeURL)
	}
	// 起始页没有来路：后退按钮必须一眼看出禁用（点了也无效）。
	back := h.doc.QuerySelector(`.system-ui-tool[title="后退"]`)
	if !strings.Contains(back.GetAttribute("class"), "is-disabled") {
		t.Fatalf("起始页后退按钮类名 = %q，期望 is-disabled", back.GetAttribute("class"))
	}
	clickCenter(back)
	assertTitle("Ghromex 浏览器 · 起始页")

	// 地址栏是工具栏槽位里的 input：鼠标拖选必须命中并产生选区，
	// 再点右侧可折叠选区继续输入（用户报告过 input 无法选中文字）。
	h.doc.Refresh()
	addr := addrEl()
	r := addr.GetBoundingClientRect()
	midY := (r.Top().Pixel() + r.Bottom().Pixel()) / 2
	if !engine.OnDocumentMouseDown(h.doc, r.Left().Pixel()+2, midY) ||
		!engine.OnDocumentMouseDrag(h.doc, r.Right().Pixel()-2, midY) {
		t.Fatal("地址栏未进入拖选状态（固定工具栏槽位的 input 命中失败）")
	}
	engine.OnDocumentMouseUp(h.doc)
	if got, ok := engine.SelectedText(h.doc); !ok || got != homeURL {
		t.Fatalf("地址栏拖选 = %q, ok=%v, want %q", got, ok, homeURL)
	}

	// 地址栏是可编辑 input：点右侧后键入应追加到末尾（真实窗口可直接输入）。
	r = addr.GetBoundingClientRect()
	engine.OnDocumentClick(h.doc, r.Right().Pixel()-4, (r.Top().Pixel()+r.Bottom().Pixel())/2)
	addr = addrEl()
	if !engine.OnDocumentTextInput(h.doc, "x") {
		t.Fatal("地址栏没有消费键盘输入")
	}
	if got := engine.GetValue(addrEl()); got != homeURL+"x" {
		t.Fatalf("地址栏输入后 = %q, want %q", got, homeURL+"x")
	}

	// 地址栏「打开」
	addrEl().SetAttribute("value", "../ua/index.html")
	clickTool("打开")
	assertTitle("UA 与排版对照 · 浏览器对照")
	assertChrome()
	wantUA := filepath.Join("..", "ua", "index.html")
	if app.current != wantUA {
		t.Fatalf("地址栏导航后 current = %q, want %q", app.current, wantUA)
	}
	if got := engine.GetValue(addrEl()); got != wantUA {
		t.Fatalf("新页面地址栏 = %q, want %q", got, wantUA)
	}

	// 后退 / 前进
	clickTool("后退")
	assertTitle("Ghromex 浏览器 · 起始页")
	assertChrome()
	clickTool("前进")
	assertTitle("UA 与排版对照 · 浏览器对照")

	// 首页 / 刷新
	clickTool("首页")
	assertTitle("Ghromex 浏览器 · 起始页")
	clickTool("刷新")
	assertTitle("Ghromex 浏览器 · 起始页")

	// 滚动后工具栏仍固定在视口顶部：滚轮把内容上移，点顶部后退仍命中。
	clickLink("portal")
	if !strings.HasSuffix(filepath.ToSlash(app.current), "portal/index.html") {
		t.Fatalf("门户页 current = %q", app.current)
	}
	if !engine.OnDocumentWheel(h.doc, 512, 400, 0, 200) {
		t.Fatal("滚动页没有发生滚动")
	}
	clickTool("后退")
	assertChrome()
	assertTitle("Ghromex 浏览器 · 起始页")

	// 链接导航后工具栏仍在
	clickLink("ua")
	assertTitle("UA 与排版对照 · 浏览器对照")
	assertChrome()

	// 外部链接：提示页 + 工具栏；about:home 回起始页
	clickTool("后退")
	clickLink("https://")
	assertTitle("Ghromex 浏览器 · 外部链接")
	assertChrome()
	clickLink("about:home")
	assertTitle("Ghromex 浏览器 · 起始页")

	// 缺失文件：错误页 + 工具栏；后退返回
	clickLink("missing.html")
	assertTitle("Ghromex 浏览器 · 无法打开页面")
	assertChrome()
	clickTool("后退")
	assertTitle("Ghromex 浏览器 · 起始页")

	// file:// 绝对路径同样走本地文件加载
	app.gotoAddress("file:///C:/definitely-missing-ghromex.html")
	assertTitle("Ghromex 浏览器 · 无法打开页面")
	assertChrome()

	// 刷新重新读盘：改写磁盘上的页面后点「刷新」应看到新内容。
	dir := t.TempDir()
	page := filepath.Join(dir, "refresh.html")
	if err := os.WriteFile(page, []byte("<!doctype html><title>第一版</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	app.show(page, dir, data, true)
	assertTitle("第一版")
	if err := os.WriteFile(page, []byte("<!doctype html><title>第二版</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	clickTool("刷新")
	assertTitle("第二版")
	assertChrome()
}

// 示例页面下拉（工具栏槽位里的 select）：选中一项就导航过去；
// 导航换文档后下拉重新生成，自动回到占位项。
func TestExamplePagesSelect(t *testing.T) {
	h := &headlessOpener{vp: engine.NewHeadlessViewport(1024, 720)}
	app := &browserApp{win: h, homeDir: ".", index: -1}
	app.showHome()
	h.doc.Refresh()

	sel := func() engine.HTMLElement {
		t.Helper()
		el := h.doc.QuerySelector(".system-ui-tool-slot .system-ui-select")
		if el == nil {
			t.Fatal("工具栏里没有示例页面下拉")
		}
		return el
	}
	if got := engine.SelectValue(sel()); got != pagesPlaceholder {
		t.Fatalf("初始选中值 = %q, want %q（占位项）", got, pagesPlaceholder)
	}

	// 用户选择触发 change → 导航。值按 homeDir 解析（这里是测试工作目录
	// examples/browser，"../portal/index.html" 指向 examples/portal）。
	if !engine.SetSelectValue(sel(), "../portal/index.html") {
		t.Fatal("SetSelectValue 没找到该选项")
	}
	if got := h.doc.Title(); got != "GHROMEX MARKET · 今日上新" {
		t.Fatalf("下拉导航后标题 = %q", got)
	}
	assertChrome := func() {
		t.Helper()
		for _, s := range []string{".system-ui-toolbar", ".system-ui-status"} {
			if h.doc.QuerySelector(s) == nil {
				t.Fatalf("新页面缺少 %s", s)
			}
		}
	}
	assertChrome()
	if sel() == nil {
		t.Fatal("换页后示例页面下拉丢失")
	}
	if got := engine.SelectValue(sel()); got != pagesPlaceholder {
		t.Fatalf("换页后下拉选中值 = %q, want %q（回到占位项）", got, pagesPlaceholder)
	}
}

// 工具栏（含地址栏槽位）fixed 在视口上：视口变宽后整栏跟着变宽，
// 不需要应用参与重排（引擎没有窗口 chrome 层，宽度全靠 width:100% +
// 重排；旧实现按视口宽度算控件坐标的路径已经删掉了）。
func TestToolbarFollowsViewportResize(t *testing.T) {
	vp := engine.NewHeadlessViewport(1024, 720)
	h := &headlessOpener{vp: vp}
	app := &browserApp{win: h, homeDir: ".", index: -1}
	app.showHome()
	h.doc.Refresh()

	toolbar := h.doc.QuerySelector(".system-ui-toolbar")
	if toolbar == nil {
		t.Fatal("没有生成工具栏")
	}
	if got := toolbar.GetBoundingClientRect().Right().Pixel(); got != 1024 {
		t.Fatalf("工具栏右缘 = %d, want 1024", got)
	}

	// 模拟窗口最大化：视口变宽后重排一次即可。
	vp.W = 1400
	h.doc.Refresh()
	if got := toolbar.GetBoundingClientRect().Right().Pixel(); got != 1400 {
		t.Fatalf("放大后工具栏右缘 = %d, want 1400", got)
	}
}

// 地址栏里按回车直接打开（与点「打开」按钮同一条路径）。
//
// 回车由引擎派发给聚焦元素的 keydown 处理器（元素级 OnKeyDown）：浏览器
// 应用不用改引擎的默认动作，也不用在窗口层单独截按键。
func TestAddressBarEnterNavigates(t *testing.T) {
	h := &headlessOpener{vp: engine.NewHeadlessViewport(1024, 720)}
	app := &browserApp{win: h, homeDir: ".", index: -1}
	app.showHome()
	h.doc.Refresh()

	addr := h.doc.QuerySelector(".system-ui-tool-slot .system-ui-input")
	if addr == nil {
		t.Fatal("缺少地址栏")
	}
	dir := t.TempDir()
	page := filepath.Join(dir, "p.html")
	const want = "回车打开的页"
	if err := os.WriteFile(page, []byte("<!doctype html><html><head><title>"+want+
		"</title></head><body>hi</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	addr.SetAttribute("value", page)
	engine.SetFocusedElement(h.doc, addr)

	if !engine.OnDocumentKeyDown(h.doc, engine.KeyEnter) {
		t.Fatal("回车没有被消费（地址栏的 keydown 处理器没跑）")
	}
	h.doc.Refresh()
	if got := h.doc.Title(); got != want {
		t.Fatalf("回车后标题 = %q, want %q", got, want)
	}
}
