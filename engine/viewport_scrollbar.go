package engine

// ---------------------------------------------------------------------------
// 文档级覆盖式滚动条（viewport scrollbar）
//
// 页面滚动此前只有滚轮驱动、没有任何视觉指示（examples/browser 长页面
// 「不知道能不能滚、滚到哪了」）。本文件给文档级滚动补上覆盖式滚动条：
// 叠在视口右缘/下缘、不占布局宽度（折行口径不变），绘制与命中共用
// viewportBars 同一几何源（「看到的就是能点的」，同 textarea 滚动条口径）。
//
// 与 textarea 覆盖滚动条（textarea.go）的差别：
//   - 只画滑块不画轨道底：视口边缘常驻一条灰带比常驻滚动条更打扰；
//   - 拖拽**立即到位**不缓动：滚轮的弹簧动画是「滚轮只改目标」的补间，
//     而拖滑块要的是跟手——viewportBarScrollTo 把该轴动画直接贴到目标
//     （Chrome 同款：拖滚动条无缓动，滚轮有）。
//
// 宿主有固定栏时（ui 工作台的工具栏/状态栏是 fixed 压在页面上的），滚动条
// 不该画到栏上：SetViewportScrollbarInsets 把纵条限制在两栏之间的带里，
// SetViewportHorizontalScrollbar 关掉横条。几何仍以 viewportBars 为唯一
// 口径（绘制/命中/拖拽映射同步生效）。
//
// 只处理根元素（文档级滚动）；通用 overflow 容器暂未覆盖（scroll.go 头注
// 的「容器无滚动条」现状对容器仍成立）。
// ---------------------------------------------------------------------------

// viewportScrollbarWidth 是文档级覆盖滚动条的命中带宽度（px），与 textarea
// 的 8px 同口径。
const viewportScrollbarWidth = 8

// sbBar 是单轴覆盖滚动条的视口坐标几何：轨道 (x,y,w,h) 为命中带，
// 滑块沿轨道方向的起止为 tPos/tSize（绝对视口坐标）。
type sbBar struct {
	x, y, w, h int
	tPos       int
	tSize      int
	visible    bool
}

// viewportBars 返回文档级滚动的纵/横滚动条几何。该轴内容溢出视口才出现；
// 滑块位置取**视觉**滚动偏移（scrollVisualOffset），滚轮缓动动画途中滑块
// 与画面同步移动。固定栏插入带（sbInset*）把纵条限制在 [top, 高−bottom]、
// 横条贴到下插入带之上；sbNoHBar 时横条不出现。
func viewportBars(root *htmlElement, vw, vh int) (v, h sbBar) {
	if root == nil || vw <= 0 || vh <= 0 {
		return
	}
	top, bottom := root.sbInsetTop, root.sbInsetBottom
	if top < 0 {
		top = 0
	}
	if bottom < 0 {
		bottom = 0
	}
	maxX, maxY := scrollMax(root, vw, vh, true)
	ox, oy := scrollVisualOffset(root)
	if maxY > 0 {
		if trackH := vh - top - bottom; trackH > 0 {
			oy = clampInt(oy, 0, maxY)
			v = sbBar{x: vw - viewportScrollbarWidth, y: top, w: viewportScrollbarWidth, h: trackH, visible: true}
			v.tSize = sbThumbSize(v.h, maxY)
			if v.h > v.tSize {
				v.tPos = top + oy*(v.h-v.tSize)/maxY
			}
		}
	}
	if maxX > 0 && !root.sbNoHBar {
		ox = clampInt(ox, 0, maxX)
		tw := vw
		if v.visible {
			tw = vw - viewportScrollbarWidth // 右下角让给纵条（浏览器同款）
		}
		y := vh - bottom - viewportScrollbarWidth
		if y < 0 {
			y = 0
		}
		h = sbBar{x: 0, y: y, w: tw, h: viewportScrollbarWidth, visible: true}
		h.tSize = sbThumbSize(h.w, maxX)
		if h.w > h.tSize {
			h.tPos = ox * (h.w - h.tSize) / maxX
		}
	}
	return
}

// sbThumbSize 滑块长：按视口占内容比例折算（track × track/(track+max)），
// 最小 24px 可抓握、不超过轨道（textarea 同口径）。
func sbThumbSize(track, maxScroll int) int {
	size := track * track / (track + maxScroll)
	if size < 24 {
		size = 24
	}
	if size > track {
		size = track
	}
	return size
}

// viewportBarHit 判断 (x,y) 落在哪条文档级滚动条上：axisX=false 纵条、
// true 横条，ok=false 不在任何条上。纵条优先（横条轨道已避开右下角）。
func viewportBarHit(root *htmlElement, vw, vh, x, y int) (axisX, ok bool) {
	v, h := viewportBars(root, vw, vh)
	if v.visible && x >= v.x && x < v.x+v.w && y >= v.y && y < v.y+v.h {
		return false, true
	}
	if h.visible && x >= h.x && x < h.x+h.w && y >= h.y && y < h.y+h.h {
		return true, true
	}
	return false, false
}

// viewportBarScrollTo 把指针在滚动条轨道内的位置映射为文档级滚动目标
// （滑块中心对准指针），并立即生效：该轴缓动动画贴到目标（拖拽跟手，
// 不补间），变化/吸附都发滚动通知让宿主重绘。
func viewportBarScrollTo(d *htmlDocument, axisX bool, x, y int) {
	root := inner(d)
	if root == nil {
		return
	}
	vw, vh := d.viewportSize()
	v, h := viewportBars(root, vw, vh)
	maxX, maxY := scrollMax(root, vw, vh, true)
	var track, tSize, maxScroll, pos, origin int
	if axisX {
		track, tSize, maxScroll, pos, origin = h.w, h.tSize, maxX, x, h.x
	} else {
		track, tSize, maxScroll, pos, origin = v.h, v.tSize, maxY, y, v.y
	}
	if maxScroll <= 0 || track <= tSize {
		return
	}
	// 指针坐标减去轨道起点即轨道内距（纵条起点随固定栏插入带下移；
	// 横条起点在 x=0）。
	p := clampInt(pos-origin-tSize/2, 0, track-tSize)
	next := p * maxScroll / (track - tSize)
	if axisX {
		root.scrollLeft = next
	} else {
		root.scrollTop = next
	}
	snapScrollAnim(root, axisX)
	root.onScrollChanged()
}

// snapScrollAnim 把文档级缓动动画的**指定轴**贴到目标：滚动条拖拽要立即
// 到位（Chrome 拖滚动条无缓动）。另一轴不受扰（scrollAnimOn 按两轴进度
// 重算，未走满的一轴继续自己的动画）。
func snapScrollAnim(el *htmlElement, axisX bool) {
	if axisX {
		el.scrollAnimElapsedLeftNs = int64(scrollAnimDuration)
		el.scrollAnimBaseLeft = float64(el.scrollLeft)
		el.scrollAnimPosLeft = float64(el.scrollLeft)
		el.scrollAnimLeft = el.scrollLeft
	} else {
		el.scrollAnimElapsedTopNs = int64(scrollAnimDuration)
		el.scrollAnimBaseTop = float64(el.scrollTop)
		el.scrollAnimPosTop = float64(el.scrollTop)
		el.scrollAnimTop = el.scrollTop
	}
	el.scrollAnimOn = scrollAnimProgress(el.scrollAnimElapsedTopNs) < 1 ||
		scrollAnimProgress(el.scrollAnimElapsedLeftNs) < 1
}

// renderViewportScrollbars 绘制文档级覆盖滚动条（只画滑块）。层序由
// RenderNode 保证：所有页面内容（含定位层）之上、select 浮层与提示气泡
// 之下。
func renderViewportScrollbars(g Graphics, root *htmlElement, vw, vh int) {
	v, h := viewportBars(root, vw, vh)
	if v.visible {
		g.DrawColor(v.x, v.tPos, v.w, v.tSize, scrollbarThumb)
	}
	if h.visible {
		g.DrawColor(h.tPos, h.y, h.tSize, h.h, scrollbarThumb)
	}
}

// SetViewportScrollbarInsets 约束文档级纵向覆盖滚动条的带：只在
// [top, 视口高−bottom] 之间绘制与命中（横条贴下插入带之上）。宿主的
// 固定栏（ui 工作台的工具栏/状态栏）占满视口右缘时，滚动条要让出栏带、
// 只服务页面内容区——「滚动的是网页，不是整个浏览器」。
//
// 负值按 0 处理；带宽 ≤ 0 时不画纵条（不影响滚动本身）。纯几何状态、
// 不触发重绘——调用方自己保证有一次重绘（工作台在装文档时调用，随后
// 必然重排重绘）。
func SetViewportScrollbarInsets(doc HTMLDocument, top, bottom int) {
	if d, ok := doc.(*htmlDocument); ok {
		if root := inner(d); root != nil {
			if top < 0 {
				top = 0
			}
			if bottom < 0 {
				bottom = 0
			}
			root.sbInsetTop, root.sbInsetBottom = top, bottom
		}
	}
}

// SetViewportHorizontalScrollbar 开关文档级横向覆盖滚动条（默认显示）。
// 关掉后横向溢出不再出横条，滚轮的横向滚动不受影响；工作台系应用
// （浏览器/记事本等）关掉它——横条会压在固定状态栏上。
func SetViewportHorizontalScrollbar(doc HTMLDocument, show bool) {
	if d, ok := doc.(*htmlDocument); ok {
		if root := inner(d); root != nil {
			root.sbNoHBar = !show
		}
	}
}

// viewportSize 返回当前视口的 CSS 像素尺寸（未设视口时为 0×0）。
func (document *htmlDocument) viewportSize() (int, int) {
	if v := document.Viewport(); v != nil {
		return v.ViewportWidth(), v.ViewportHeight()
	}
	return 0, 0
}
