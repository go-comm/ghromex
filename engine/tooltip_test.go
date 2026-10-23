package engine

import (
	"testing"
	"time"
)

// title 提示气泡测试（白盒：拨动计时字段驱动 TickTooltip，真等 600ms 没必要）。

// tooltipPage 是一张「工具栏」：#btn 带提示且内嵌 #glyph（验证沿父链继承），
// #plain 无提示，#blank 的 title 只有空白（等于没有）。#filler 撑高页面，
// 让滚轮真的能滚（滚动收起气泡的测试要用）。
const tooltipPage = `<!doctype html><html><head><style>
body{margin:0}
#bar{width:240px;height:40px}
.tool{display:inline-block;width:40px;height:40px}
#glyph{width:16px;height:16px}
#filler{height:400px}
</style></head><body><div id="bar">
<div id="btn" class="tool" title="保存  Ctrl+S"><div id="glyph"></div></div><div
id="plain" class="tool"></div><div id="blank" class="tool" title="   "></div>
</div><div id="filler"></div></body></html>`

func openTooltipDoc(t *testing.T, src string) (*BufferGraphics, HTMLDocument, *htmlDocument) {
	t.Helper()
	buf, doc, err := OpenBufferedDocument(240, 120, src)
	if err != nil {
		t.Fatalf("OpenBufferedDocument: %v", err)
	}
	d, ok := doc.(*htmlDocument)
	if !ok {
		t.Fatal("not an htmlDocument")
	}
	return buf, doc, d
}

// armTooltip 把计时拨回「已经超过延时」——测试不睡真时间。
func armTooltip(d *htmlDocument) {
	d.tooltipSinceNs = time.Now().UnixNano() - int64(tooltipDelay) - 1
}

// hoverShow 悬停到 (x,y) 并等气泡弹出：第一次 tick 登记新目标（刚进入不该
// 立刻弹出），拨过延时后第二次 tick 应弹出。返回气泡文案。
func hoverShow(t *testing.T, doc HTMLDocument, d *htmlDocument, x, y int) string {
	t.Helper()
	OnDocumentMouseMove(doc, x, y)
	if TickTooltip(doc) {
		t.Fatalf("(%d,%d) 刚进入目标就弹出了（延时被跳过）", x, y)
	}
	armTooltip(d)
	if !TickTooltip(doc) {
		t.Fatalf("(%d,%d) 计时到点应请求重绘", x, y)
	}
	return TooltipText(doc)
}

func bufHasText(buf *BufferGraphics, s string) bool {
	for _, t := range buf.Texts {
		if t == s {
			return true
		}
	}
	return false
}

// 悬停延时弹出：到点前不弹、到点后弹，弹出时文字与底色真的画出来。
func TestTooltipAppearsAfterDelay(t *testing.T) {
	buf, doc, d := openTooltipDoc(t, tooltipPage)

	if TickTooltip(doc) {
		t.Fatal("鼠标还没进文档，不该请求重绘")
	}
	if TooltipPending(doc) || TooltipText(doc) != "" {
		t.Fatal("无悬停时不该有计时项与气泡")
	}

	OnDocumentMouseMove(doc, 10, 10) // 悬在 #glyph 上
	if !TooltipPending(doc) {
		t.Fatal("进入带提示的元素就该开始计时")
	}
	if TooltipText(doc) != "" {
		t.Fatal("延时未到不该弹出")
	}
	if TickTooltip(doc) {
		t.Fatal("计时未到不该请求重绘（这一拍登记悬停目标）")
	}
	armTooltip(d)
	if !TickTooltip(doc) {
		t.Fatal("计时到点应请求重绘")
	}
	if got := TooltipText(doc); got != "保存  Ctrl+S" {
		t.Fatalf("气泡文案 = %q，期望沿父链取到 #btn 的 title", got)
	}
	if TooltipPending(doc) {
		t.Fatal("已弹出就不该再有在途计时")
	}

	// 绘制核对：气泡锚在鼠标右下 (10+tooltipGapX, 10+tooltipGapY)。
	RenderNode(buf, doc)
	if !bufHasText(buf, "保存  Ctrl+S") {
		t.Fatal("气泡文字没被画出来")
	}
	_, th := buf.MeasureText("保存  Ctrl+S", tooltipFontSize, false, "")
	bx, by := 10+tooltipGapX, 10+tooltipGapY
	bh := th + 2*tooltipPadY
	// 取左 padding 中段：文字区在 x+padX 之后，边框只占最外 1px，
	// 这个点必为纯底色。
	if r, g, b, a, ok := buf.ColorAt(bx+4, by+bh/2); !ok || r != 0x0F || g != 0x17 || b != 0x2A || a != 255 {
		t.Fatalf("气泡底色 = %02X%02X%02X%02X (ok=%v)，期望 #0F172A 不透明", r, g, b, a, ok)
	}
}

// 目标切换收起并重新计时；同目标内移动不重置计时、气泡不动。
func TestTooltipTargetSwitchRetimes(t *testing.T) {
	_, doc, d := openTooltipDoc(t, tooltipPage)
	if got := hoverShow(t, doc, d, 10, 10); got != "保存  Ctrl+S" {
		t.Fatalf("前置：气泡没弹出（%q）", got)
	}

	// 移到同一目标的另一处（#btn 直接命中）：计时不重置、气泡保持。
	OnDocumentMouseMove(doc, 35, 10)
	if TickTooltip(doc) {
		t.Fatal("同目标内移动不该重绘")
	}
	if got := TooltipText(doc); got != "保存  Ctrl+S" {
		t.Fatalf("同目标内移动气泡不该消失（%q）", got)
	}

	// 换到无提示元素：收起并请求重绘。
	OnDocumentMouseMove(doc, 60, 10) // #plain
	if !TickTooltip(doc) {
		t.Fatal("目标切换，旧气泡收起要重绘")
	}
	if TooltipText(doc) != "" || TooltipPending(doc) {
		t.Fatal("切到无提示目标：既无气泡也无在途计时")
	}

	// 换回来重新计时：先不弹，过点才弹。
	if got := hoverShow(t, doc, d, 10, 10); got != "保存  Ctrl+S" {
		t.Fatalf("重新悬停后气泡 = %q", got)
	}

	// 鼠标离开文档（后端走 SetHoveredElement(nil)，不经移动入口）。
	SetHoveredElement(doc, nil)
	if !TickTooltip(doc) {
		t.Fatal("悬停清除后气泡收起要重绘")
	}
	if TooltipText(doc) != "" {
		t.Fatal("离开文档后不该有气泡")
	}
}

// 空/纯空白 title 等于没有提示。
func TestTooltipBlankTitleIgnored(t *testing.T) {
	_, doc, d := openTooltipDoc(t, tooltipPage)
	OnDocumentMouseMove(doc, 100, 10) // #blank: title 全是空格
	for i := 0; i < 2; i++ {
		if TickTooltip(doc) {
			t.Fatal("空白 title 不该产生气泡")
		}
		armTooltip(d)
	}
	if TooltipPending(doc) || TooltipText(doc) != "" {
		t.Fatal("空白 title 不该有在途计时")
	}
}

// 按下与点击收起气泡；收起后不换目标不追回，换目标才重新计时。
func TestTooltipDismissedByPressAndClick(t *testing.T) {
	_, doc, d := openTooltipDoc(t, tooltipPage)
	if got := hoverShow(t, doc, d, 10, 10); got == "" {
		t.Fatal("前置：气泡没弹出")
	}
	OnDocumentMouseDown(doc, 10, 10)
	if TooltipText(doc) != "" {
		t.Fatal("按下应收起气泡")
	}
	if TickTooltip(doc) {
		t.Fatal("收起后不该再请求弹出")
	}
	// 同一目标内移动：不追回（浏览器行为——要换目标才重新计时）。
	OnDocumentMouseMove(doc, 35, 10)
	armTooltip(d)
	if TickTooltip(doc) || TooltipText(doc) != "" {
		t.Fatal("按下收起后，同目标内移动不该重新弹出")
	}
	// 换目标再换回：重新武装（hoverShow 内含「先不弹、过点才弹」的断言）。
	OnDocumentMouseMove(doc, 60, 10)
	TickTooltip(doc)
	if got := hoverShow(t, doc, d, 10, 10); got != "保存  Ctrl+S" {
		t.Fatalf("换目标后气泡 = %q", got)
	}

	// 点击同样收起（程序派发可以不经按下直达 click）。
	OnDocumentClick(doc, 10, 10)
	if TooltipText(doc) != "" {
		t.Fatal("点击应收起气泡")
	}
}

// 实际滚动收起气泡（锚点留在屏幕坐标，内容动了就对不上位置）。
func TestTooltipDismissedByWheel(t *testing.T) {
	_, doc, d := openTooltipDoc(t, tooltipPage)
	if got := hoverShow(t, doc, d, 10, 10); got == "" {
		t.Fatal("前置：气泡没弹出")
	}
	if !OnDocumentWheel(doc, 10, 10, 0, 20) {
		t.Fatal("前置：页面应能滚动（#filler 撑高）")
	}
	if TooltipText(doc) != "" {
		t.Fatal("滚动后气泡应收起")
	}
}

// 贴窗口边缘的气泡夹回视口内、下方放不下翻到鼠标上方——不能被截掉。
func TestTooltipClampedIntoViewport(t *testing.T) {
	// 整页悬在 body 的提示上，任意坐标都有气泡，方便贴边取样。
	const src = `<!doctype html><html><head><style>
body{margin:0;height:400px}
</style></head><body title="页边提示"></body></html>`
	buf, doc, d := openTooltipDoc(t, src)
	if got := hoverShow(t, doc, d, 235, 115); got != "页边提示" {
		t.Fatalf("贴边悬停气泡 = %q", got)
	}
	RenderNode(buf, doc)
	if !bufHasText(buf, "页边提示") {
		t.Fatal("气泡文字没被画出来")
	}
	tw, th := buf.MeasureText("页边提示", tooltipFontSize, false, "")
	bw, bh := tw+2*tooltipPadX, th+2*tooltipPadY
	// 期望位置：x 夹到右缘（240 视口），y 翻到鼠标上方。
	wantX, wantY := buf.W-bw, 115-tooltipGapY-bh
	// 底色取样点：盒左内缩 4px、垂直中线（圆角带之外）。
	if r, g, b, a, ok := buf.ColorAt(wantX+4, wantY+bh/2); !ok || r != 0x0F || g != 0x17 || b != 0x2A || a != 255 {
		t.Fatalf("夹取后气泡底色 (%d,%d) = %02X%02X%02X%02X (ok=%v)，期望 #0F172A", wantX+4, wantY+bh/2, r, g, b, a, ok)
	}
}
