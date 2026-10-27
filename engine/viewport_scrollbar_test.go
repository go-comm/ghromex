package engine

import (
	"strings"
	"testing"
)

// openLongDoc 开一个 contentH px 高的长页（视口 400×300），文档级滚动条
// 的几何/拖拽测试都以它为现场。
func openLongDoc(t *testing.T, contentH int) HTMLDocument {
	t.Helper()
	vp := NewHeadlessViewport(400, 300)
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html><body style="margin:0">`)
	for i := 0; i < contentH/100; i++ {
		sb.WriteString(`<div style="height:100px">block</div>`)
	}
	sb.WriteString(`</body></html>`)
	doc, err := OpenDocument(vp, sb.String())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// 溢出出条、滑块按比例、滚到底贴轨道末端；不溢出不出现。
func TestViewportScrollbarGeometry(t *testing.T) {
	doc := openLongDoc(t, 2000)
	d := doc.(*htmlDocument)
	root := inner(doc)
	vw, vh := d.viewportSize()
	v, h := viewportBars(root, vw, vh)
	if !v.visible {
		t.Fatal("2000px 内容在 300px 视口应出纵条")
	}
	if h.visible {
		t.Error("内容没有横向溢出，不该出横条")
	}
	if want := vw - viewportScrollbarWidth; v.x != want || v.w != viewportScrollbarWidth || v.h != vh {
		t.Errorf("纵条轨道 = (%d,%d,%d), want (%d,%d,%d)", v.x, v.w, v.h, want, viewportScrollbarWidth, vh)
	}
	if v.tSize >= v.h {
		t.Errorf("滑块长 %d 应短于轨道 %d（内容 6.7 屏）", v.tSize, v.h)
	}
	if v.tPos != 0 {
		t.Errorf("未滚动时滑块顶 = %d, want 0", v.tPos)
	}
	// 滚到底：滑块贴轨道末端
	_, maxY := scrollMax(root, vw, vh, true)
	root.scrollTop = maxY
	v2, _ := viewportBars(root, vw, vh)
	if v2.tPos+v2.tSize != v2.h {
		t.Errorf("滚到底滑块底 = %d, want %d（轨道末端）", v2.tPos+v2.tSize, v2.h)
	}
	// 短内容：没有滚动条
	short := openLongDoc(t, 200)
	v3, h3 := viewportBars(inner(short), 400, 300)
	if v3.visible || h3.visible {
		t.Error("200px 内容在 300px 视口不该出滚动条")
	}
}

// 轨道按下即跳转、拖动跟随、抬起结束；全程不改焦点。
func TestViewportScrollbarDrag(t *testing.T) {
	doc := openLongDoc(t, 2000)
	d := doc.(*htmlDocument)
	root := inner(doc)
	vw, vh := d.viewportSize()
	v, _ := viewportBars(root, vw, vh)
	px, py := v.x+viewportScrollbarWidth/2, v.h-10 // 轨道末端附近
	if !OnDocumentMouseDown(doc, px, py) {
		t.Fatal("滚动条按下应进入拖动通道（返回 true 抑制 click）")
	}
	if root.scrollTop == 0 {
		t.Fatal("按下即跳到点击位置，scrollTop 不该还是 0")
	}
	// 拖回轨道顶部：滚动贴 0
	OnDocumentMouseDrag(doc, px, 0)
	if root.scrollTop != 0 {
		t.Errorf("拖到轨道顶端 scrollTop = %d, want 0", root.scrollTop)
	}
	// 拖到轨道中部：滚动约在半程（±1 像素容差）
	_, maxY := scrollMax(root, vw, vh, true)
	OnDocumentMouseDrag(doc, px, vh/2)
	mid := root.scrollTop
	if want := (vh/2 - v.tSize/2) * maxY / (vh - v.tSize); abs(mid-want) > 1 {
		t.Errorf("拖到中部 scrollTop = %d, want %d±1", mid, want)
	}
	// 拖动期间该轴动画贴到目标（跟手，不补间）
	if root.scrollAnimTop != root.scrollTop {
		t.Errorf("拖拽后视觉位置 %d ≠ 目标 %d（动画没吸附）", root.scrollAnimTop, root.scrollTop)
	}
	OnDocumentMouseUp(doc)
	if d.dragViewAxis != 0 {
		t.Error("抬起应清空拖条状态")
	}
	// 抬起后再拖不再滚动页面
	before := root.scrollTop
	OnDocumentMouseDrag(doc, px, 10)
	if root.scrollTop != before {
		t.Errorf("抬起后的拖动滚动了页面：%d → %d", before, root.scrollTop)
	}
}

// 滚动条上的按下不改焦点（按下-抬起未移动的 click 路径同样被挡）。
func TestViewportScrollbarKeepsFocusPath(t *testing.T) {
	doc := openLongDoc(t, 2000)
	root := inner(doc)
	vw, vh := doc.(*htmlDocument).viewportSize()
	v, _ := viewportBars(root, vw, vh)
	px, py := v.x+viewportScrollbarWidth/2, v.h/2
	if FocusedElement(doc) != nil {
		t.Fatal("前置：不该已有焦点")
	}
	OnDocumentMouseDown(doc, px, py)
	OnDocumentMouseUp(doc)
	if FocusedElement(doc) != nil {
		t.Error("滚动条按下-抬起改动了焦点")
	}
	// 未移动过阈值时后端照常派发 click：引擎侧必须挡住
	if OnDocumentClick(doc, px, py) {
		t.Error("滚动条上的 click 被派发给了页面内容")
	}
}

// 横轴：内容横向溢出时出横条，拖横条改 scrollLeft、不动 scrollTop。
func TestViewportScrollbarHorizontal(t *testing.T) {
	vp := NewHeadlessViewport(400, 300)
	doc, err := OpenDocument(vp, `<!doctype html><html><body style="margin:0">
		<div style="width:2000px;height:50px">wide</div>
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	root := inner(doc)
	_, h := viewportBars(root, 400, 300)
	if !h.visible {
		t.Fatal("2000px 宽内容在 400px 视口应出横条")
	}
	if h.y != 300-viewportScrollbarWidth {
		t.Errorf("横条贴底：y = %d, want %d", h.y, 300-viewportScrollbarWidth)
	}
	py := h.y + viewportScrollbarWidth/2
	OnDocumentMouseDown(doc, h.x+h.w-10, py)
	if root.scrollLeft == 0 {
		t.Fatal("横条按下应跳转 scrollLeft")
	}
	if root.scrollTop != 0 {
		t.Errorf("横条不该带动纵向：scrollTop = %d", root.scrollTop)
	}
	OnDocumentMouseDrag(doc, 0, py)
	if root.scrollLeft != 0 {
		t.Errorf("拖到轨道左端 scrollLeft = %d, want 0", root.scrollLeft)
	}
	OnDocumentMouseUp(doc)
}

// 滑块位置跟随**视觉**偏移：缓动动画途中（目标已改、视觉未到位）滑块
// 画在当前画面位置，而不是跳到目标处。
//
// advanceScrollAnim 按墙钟推进（单帧 ≤20ms），测试里两次调用间隔是微秒
// 级、推不动动画——这里直接把动画时间拨到半程再走一帧。
func TestViewportScrollbarThumbFollowsVisual(t *testing.T) {
	doc := openLongDoc(t, 2000)
	root := inner(doc)
	startScrollAnim(root, true) // 开动画：视觉停在 0
	root.scrollTop = 500        // 目标前进
	root.scrollAnimElapsedTopNs = int64(scrollAnimDuration) / 2
	advanceScrollAnim(root)
	v2, _ := viewportBars(root, 400, 300)
	if v2.tPos <= 0 {
		t.Errorf("动画半程滑块应已离开轨道顶（easeOutQuad 半程 ≈ 3/4 位移），tPos = %d", v2.tPos)
	}
	// 收尾：动画时间走满 → 滑块精确贴目标
	root.scrollAnimElapsedTopNs = int64(scrollAnimDuration)
	advanceScrollAnim(root)
	v3, _ := viewportBars(root, 400, 300)
	_, maxY := scrollMax(root, 400, 300, true)
	if want := 500 * (v3.h - v3.tSize) / maxY; v3.tPos != want {
		t.Errorf("动画收尾滑块 = %d, want %d（目标 500 对应位置）", v3.tPos, want)
	}
	if v3.tPos <= v2.tPos {
		t.Errorf("收尾滑块应 ≥ 半程位置：%d ≤ %d", v3.tPos, v2.tPos)
	}
}

// 固定栏插入带：纵条只在 [top, 高−bottom] 里绘制/命中/映射（ui 工作台：
// 滚动条不压工具栏与状态栏），横条可关（滚轮横滚不受影响）。
func TestViewportScrollbarInsets(t *testing.T) {
	doc := openLongDoc(t, 2000)
	SetViewportScrollbarInsets(doc, 40, 24)
	root := inner(doc)
	vw, vh := doc.(*htmlDocument).viewportSize()
	v, _ := viewportBars(root, vw, vh)
	if !v.visible || v.y != 40 || v.h != vh-64 {
		t.Fatalf("纵条轨道 = (y %d, h %d), want (y 40, h %d)", v.y, v.h, vh-64)
	}
	if v.tPos != 40 {
		t.Errorf("未滚动滑块顶 = %d, want 40（轨道起点，不是视口 0）", v.tPos)
	}
	// 插入带区（工具栏位置）按下不算滚动条；带内照常命中。
	if _, ok := viewportBarHit(root, vw, vh, vw-2, 10); ok {
		t.Error("顶部插入带区不应命中纵条")
	}
	if _, ok := viewportBarHit(root, vw, vh, vw-2, 41); !ok {
		t.Error("带内按下应命中纵条")
	}
	// 拖拽映射按轨道起点：拖到带顶 → 0，拖到带底 → 滚到底。
	_, maxY := scrollMax(root, vw, vh, true)
	if !OnDocumentMouseDown(doc, vw-4, v.y+v.h-10) {
		t.Fatal("带底按下应进入拖动通道")
	}
	OnDocumentMouseDrag(doc, vw-4, v.y)
	if root.scrollTop != 0 {
		t.Errorf("拖到带顶 scrollTop = %d, want 0", root.scrollTop)
	}
	OnDocumentMouseDrag(doc, vw-4, v.y+v.h-1)
	if root.scrollTop != maxY {
		t.Errorf("拖到带底 scrollTop = %d, want %d（最大滚动）", root.scrollTop, maxY)
	}
	OnDocumentMouseUp(doc)
	// 滚到底：滑块底贴带底（视口高 − 下插入），不伸进状态栏。
	root.scrollTop = maxY
	v2, _ := viewportBars(root, vw, vh)
	if v2.tPos+v2.tSize != v2.y+v2.h {
		t.Errorf("滚到底滑块底 = %d, want %d", v2.tPos+v2.tSize, v2.y+v2.h)
	}
}

// 横向覆盖滚动条关掉后：横向溢出不出条、不命中（纵条不受扰）；负插入值按 0 钳。
func TestViewportHorizontalScrollbarHidden(t *testing.T) {
	vp := NewHeadlessViewport(400, 300)
	doc, err := OpenDocument(vp, `<!doctype html><html><body style="margin:0">
		<div style="width:2000px;height:1000px">wide</div>
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	root := inner(doc)
	if _, h := viewportBars(root, 400, 300); !h.visible {
		t.Fatal("前置：默认应出横条")
	}
	SetViewportHorizontalScrollbar(doc, false)
	SetViewportScrollbarInsets(doc, -8, -8) // 非法负值：按 0 处理，不改变几何
	_, h := viewportBars(root, 400, 300)
	if h.visible {
		t.Error("关闭后不应出横条")
	}
	if _, ok := viewportBarHit(root, 400, 300, 200, 300-4); ok {
		t.Error("关闭后横条命中带应失效")
	}
	v, _ := viewportBars(root, 400, 300)
	if !v.visible || v.y != 0 || v.h != 300 {
		t.Errorf("负插入应钳 0 且纵条不受横条开关影响：(y %d, h %d, visible %v)", v.y, v.h, v.visible)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
