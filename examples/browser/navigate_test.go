package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

type headlessOpener struct {
	vp    engine.Viewport
	width int
	doc   engine.HTMLDocument
}

func (h *headlessOpener) OpenDocument(src string) (engine.HTMLDocument, error) {
	doc, err := engine.OpenDocument(h.vp, src)
	if err == nil {
		h.doc = doc
	}
	return doc, err
}

func (h *headlessOpener) ViewportWidth() int { return h.width }

func TestLinkNavigation(t *testing.T) {
	h := &headlessOpener{vp: engine.NewHeadlessViewport(1024, 720), width: 1024}
	app := &browserApp{win: h, homeDir: ".", index: -1}

	// 无头视口不会自动触发布局，点击前先 Refresh，模拟后端的换页重排。
	clickID := func(id string) {
		t.Helper()
		h.doc.Refresh()
		el := h.doc.QuerySelector("#" + id)
		if el == nil {
			t.Fatalf("找不到 #%s", id)
		}
		r := el.GetBoundingClientRect()
		engine.OnDocumentClick(h.doc, (r.Left().Pixel()+r.Right().Pixel())/2, (r.Top().Pixel()+r.Bottom().Pixel())/2)
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
	assertChrome := func() {
		t.Helper()
		for _, id := range []string{"ghromex-chrome", "ghromex-back", "ghromex-forward", "ghromex-refresh", "ghromex-home", "ghromex-addr", "ghromex-go"} {
			if h.doc.QuerySelector("#"+id) == nil {
				t.Fatalf("当前页面缺少工具条元素 #%s", id)
			}
		}
	}

	app.showHome()
	assertTitle("Ghromex 浏览器 · 起始页")
	assertChrome()
	if got := h.doc.QuerySelector("#ghromex-addr").GetAttribute("value"); got != homeURL {
		t.Fatalf("起始页地址栏 = %q, want %q", got, homeURL)
	}

	// 地址栏是固定工具条里的 input：鼠标拖选必须命中并产生选区，
	// 再点右侧可折叠选区继续输入（用户报告过 input 无法选中文字）。
	h.doc.Refresh()
	addrBox := h.doc.QuerySelector("#ghromex-addr")
	r := addrBox.GetBoundingClientRect()
	midY := (r.Top().Pixel() + r.Bottom().Pixel()) / 2
	if !engine.OnDocumentMouseDown(h.doc, r.Left().Pixel()+2, midY) ||
		!engine.OnDocumentMouseDrag(h.doc, r.Right().Pixel()-2, midY) {
		t.Fatal("地址栏未进入拖选状态（固定工具条的 input 命中失败）")
	}
	engine.OnDocumentMouseUp(h.doc)
	if got, ok := engine.SelectedText(h.doc); !ok || got != homeURL {
		t.Fatalf("地址栏拖选 = %q, ok=%v, want %q", got, ok, homeURL)
	}

	// 地址栏是可编辑 input：点右侧后键入应追加到末尾（真实窗口可直接输入）。
	r = addrBox.GetBoundingClientRect()
	engine.OnDocumentClick(h.doc, r.Right().Pixel()-4, (r.Top().Pixel()+r.Bottom().Pixel())/2)
	if !engine.OnDocumentTextInput(h.doc, "x") {
		t.Fatal("地址栏没有消费键盘输入")
	}
	if got := addrBox.GetAttribute("value"); got != homeURL+"x" {
		t.Fatalf("地址栏输入后 = %q, want %q", got, homeURL+"x")
	}

	// 地址栏「打开」
	h.doc.QuerySelector("#ghromex-addr").SetAttribute("value", "../form/index.html")
	clickID("ghromex-go")
	assertTitle("员工登记表 · 布局示例")
	assertChrome()
	wantForm := filepath.Join("..", "form", "index.html")
	if app.current != wantForm {
		t.Fatalf("地址栏导航后 current = %q, want %q", app.current, wantForm)
	}
	if got := h.doc.QuerySelector("#ghromex-addr").GetAttribute("value"); got != wantForm {
		t.Fatalf("新页面地址栏 = %q, want %q", got, wantForm)
	}

	// 后退 / 前进
	clickID("ghromex-back")
	assertTitle("Ghromex 浏览器 · 起始页")
	assertChrome()
	clickID("ghromex-forward")
	assertTitle("员工登记表 · 布局示例")

	// 首页 / 刷新
	clickID("ghromex-home")
	assertTitle("Ghromex 浏览器 · 起始页")
	clickID("ghromex-refresh")
	assertTitle("Ghromex 浏览器 · 起始页")

	// 滚动后工具条仍固定在视口顶部：滚轮把内容上移，点顶部「后退」仍命中。
	clickLink("scroll")
	if !strings.HasSuffix(filepath.ToSlash(app.current), "scroll/index.html") {
		t.Fatalf("滚动页 current = %q", app.current)
	}
	if !engine.OnDocumentWheel(h.doc, 512, 400, 0, 200) {
		t.Fatal("滚动页没有发生滚动")
	}
	clickID("ghromex-back")
	assertTitle("Ghromex 浏览器 · 起始页")

	// 链接导航后工具条仍在
	clickLink("login")
	assertTitle("Ghromex · 登录")
	assertChrome()

	// 外部链接：提示页 + 工具条；about:home 回起始页
	clickID("ghromex-back")
	clickLink("https://")
	assertTitle("Ghromex 浏览器 · 外部链接")
	assertChrome()
	clickLink("about:home")
	assertTitle("Ghromex 浏览器 · 起始页")

	// 缺失文件：错误页 + 工具条；后退返回
	clickLink("missing.html")
	assertTitle("Ghromex 浏览器 · 无法打开页面")
	assertChrome()
	clickID("ghromex-back")
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
	clickID("ghromex-refresh")
	assertTitle("第二版")
	assertChrome()
}

// 视口变宽后工具条要重排：地址栏变宽、「打开」按钮贴到窗口右边缘。
//
// 工具条是换页时注入的，不跟着视口宽度重排的话，窗口最大化后「打开」
// 按钮会停在旧位置（不再右对齐）。
func TestChromeFollowsViewportResize(t *testing.T) {
	vp := engine.NewHeadlessViewport(1024, 720)
	h := &headlessOpener{vp: vp, width: 1024}
	app := &browserApp{win: h, homeDir: ".", index: -1}
	app.showHome()
	h.doc.Refresh()

	goBtn := h.doc.QuerySelector("#ghromex-go")
	addr := h.doc.QuerySelector("#ghromex-addr")
	if goBtn == nil || addr == nil {
		t.Fatal("工具条元素缺失")
	}
	before := goBtn.GetBoundingClientRect()
	addrBefore := addr.GetBoundingClientRect()
	if got := before.Right().Pixel(); got != 1024-chromeLeft {
		t.Fatalf("「打开」按钮右缘 = %d, want %d", got, 1024-chromeLeft)
	}

	// 模拟窗口最大化：视口变宽，引擎补一次变更通知。
	h.width = 1400
	vp.W = 1400
	h.doc.Refresh()
	h.doc.Refresh()

	after := goBtn.GetBoundingClientRect()
	if got := after.Right().Pixel(); got != 1400-chromeLeft {
		t.Fatalf("放大后「打开」按钮右缘 = %d, want %d", got, 1400-chromeLeft)
	}
	if got := after.Right().Pixel() - after.Left().Pixel(); got != before.Right().Pixel()-before.Left().Pixel() {
		t.Fatalf("放大后按钮宽度 = %d, want %d", got, before.Right().Pixel()-before.Left().Pixel())
	}
	addrAfter := addr.GetBoundingClientRect()
	if addrAfter.Right().Pixel()-addrAfter.Left().Pixel() <= addrBefore.Right().Pixel()-addrBefore.Left().Pixel() {
		t.Fatalf("放大后地址栏宽度 = %d, want > %d",
			addrAfter.Right().Pixel()-addrAfter.Left().Pixel(),
			addrBefore.Right().Pixel()-addrBefore.Left().Pixel())
	}
}
