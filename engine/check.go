package engine

import (
	"math"
	"strings"
)

// radio/checkbox：checked 状态、radio 分组互斥、点击默认动作与选中标记绘制。
//
// 口径（Chrome/Windows 实测，13x13 控件）：
//   - checkbox 选中：强调色铺满圆角方块 + 白色对勾（折线 (3,7)→(5,9)→(9,3)）；
//   - radio 选中：强调色外圆环 + 约 1px 留白 + 强调色实心内圆；
//   - 未选中沿用 UA 外观（白底 + #767676 边框），点击切换；
//   - radio 仅在 name 非空且相同的项之间互斥——实测无 name / name="" 的
//     radio 各自独立、互不影响；
//   - 点 label（包裹其控件，或 for 指向）等效点该控件；
//   - 状态在 click 冒泡之前更新（处理器可读到新值），change 在冒泡之后补发。

// checkKind 返回 input 的勾选种类："radio" / "checkbox"；非勾选元素返回 ""。
// type 缺省即 text（HTML 规范），不属于勾选控件。
func checkKind(b *htmlElement) string {
	if b == nil || !strings.EqualFold(b.tagName, "input") {
		return ""
	}
	switch strings.ToLower(b.GetAttribute("type")) {
	case "radio":
		return "radio"
	case "checkbox":
		return "checkbox"
	}
	return ""
}

// hasChecked 判定 checked 布尔属性是否存在（属性存在即为选中，值不参与判定）。
func hasChecked(b *htmlElement) bool {
	if b == nil {
		return false
	}
	_, ok := b.attrs["checked"]
	return ok
}

// IsChecked 返回 radio/checkbox 的当前选中状态；非勾选控件恒为 false。
func IsChecked(el HTMLElement) bool {
	b := inner(el)
	if checkKind(b) == "" {
		return false
	}
	return hasChecked(b)
}

// markChecked 写 checked 状态并触发重排重绘；返回状态是否真的变化。
func markChecked(b *htmlElement, on bool) bool {
	if b == nil {
		return false
	}
	_, have := b.attrs["checked"]
	if have == on {
		return false
	}
	if b.attrs == nil {
		b.attrs = make(map[string]string)
	}
	if on {
		b.attrs["checked"] = ""
	} else {
		delete(b.attrs, "checked")
	}
	b.onChanged()
	return true
}

// radioGroup 返回与 b 同组的全部 radio（含 b 自身）。仅 name 非空且相同才成组；
// 无 name / name="" 的 radio 各自独立（Chrome 实测口径）。
func radioGroup(b *htmlElement) []*htmlElement {
	name := b.GetAttribute("name")
	if name == "" {
		return []*htmlElement{b}
	}
	var out []*htmlElement
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		if cur := inner(e); cur != nil && checkKind(cur) == "radio" &&
			cur.GetAttribute("name") == name {
			out = append(out, cur)
		}
		for _, c := range e.Children() {
			walk(c)
		}
	}
	walk(rootElementOf(b.self))
	return out
}

// rootElementOf 沿父链上溯到根元素。HTMLDocument 包装器不在父链上
// （根元素 parentElement 为 nil，见 NewDocument 的 self 指针）。
func rootElementOf(e HTMLElement) HTMLElement {
	cur := e
	for cur != nil {
		p := cur.ParentElement()
		if p == nil {
			return cur
		}
		cur = p
	}
	return nil
}

// SetChecked 设置勾选状态并触发重排重绘：radio 置位时同组其余项自动取消，
// 关闭 radio 只改自身。程序化赋值不派发 change（与浏览器一致）。
func SetChecked(el HTMLElement, on bool) {
	b := inner(el)
	if checkKind(b) == "" {
		return
	}
	if on && checkKind(b) == "radio" {
		for _, r := range radioGroup(b) {
			markChecked(r, false)
		}
	}
	markChecked(b, on)
}

// checkTargetOf 返回点击默认动作应作用的勾选控件：
//  1. 命中自身或祖先中的 radio/checkbox（点控件本身）；
//  2. 否则取最近的 label：for 指向的 id 匹配则用它，包裹型取子树内第一个勾选控件。
//
// 直接命中控件时在第 1 步返回，不会因外层 label 重复触发。
func checkTargetOf(target HTMLElement) HTMLElement {
	for cur := target; cur != nil; cur = cur.ParentElement() {
		if checkKind(inner(cur)) != "" {
			return cur
		}
	}
	for cur := target; cur != nil; cur = cur.ParentElement() {
		if !tagIs(cur, "label") {
			continue
		}
		if id := cur.GetAttribute("for"); id != "" {
			if el := elementByID(rootElementOf(cur), id); checkKind(inner(el)) != "" {
				return el
			}
			continue // for 指向不存在/非勾选控件：继续向外层 label 找
		}
		if el := firstCheckable(cur); el != nil {
			return el
		}
	}
	return nil
}

// elementByID 在以 root 为根的子树中按 id 属性查找元素，未找到返回 nil。
func elementByID(root HTMLElement, id string) HTMLElement {
	var found HTMLElement
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		if found != nil || e == nil {
			return
		}
		if b := inner(e); b != nil && b.GetAttribute("id") == id {
			found = e
			return
		}
		for _, c := range e.Children() {
			walk(c)
		}
	}
	walk(root)
	return found
}

// firstCheckable 返回子树中第一个勾选控件（label 包裹情形），无则 nil。
func firstCheckable(root HTMLElement) HTMLElement {
	var found HTMLElement
	var walk func(e HTMLElement)
	walk = func(e HTMLElement) {
		if found != nil || e == nil {
			return
		}
		if checkKind(inner(e)) != "" {
			found = e
			return
		}
		for _, c := range e.Children() {
			walk(c)
		}
	}
	walk(root)
	return found
}

// applyCheckClick 执行 radio/checkbox 的点击默认动作，返回控件与状态是否变化
// （调用方据此补发 change）。radio 已选中时点击不改变状态（浏览器同口径）。
func applyCheckClick(target HTMLElement) (*htmlElement, bool) {
	b := inner(checkTargetOf(target))
	if b == nil {
		return nil, false
	}
	if checkKind(b) == "radio" {
		if hasChecked(b) {
			return nil, false
		}
		for _, r := range radioGroup(b) {
			markChecked(r, false)
		}
		markChecked(b, true)
		return b, true
	}
	return b, markChecked(b, !hasChecked(b))
}

// checkAccent 是选中标记的强调色（与聚焦框同色，保持主题一致；
// Chrome 用系统强调色 #0075FF，会随用户系统配色变化，不作硬编码基准）。
var checkAccent = focusBlue

// renderCheckedMark 绘制选中标记：checkbox 铺强调色圆角方块 + 白色对勾，
// radio 画外圆环 + 留白 + 实心内圆。未选中不调用（UA 外观已由通用路径画完）。
func renderCheckedMark(g Graphics, bx, by, bw, bh, r int, kind string) {
	if bw <= 0 || bh <= 0 {
		return
	}
	if kind == "checkbox" {
		fillRoundedRect(g, bx, by, bw, bh, r, checkAccent)
		drawCheckMark(g, bx, by, bw, bh, NewColor(0xFF, 0xFF, 0xFF, 0xFF))
		return
	}
	fillRadioMark(g, bx, by, bw, bh, checkAccent)
}

// checkPoints 是 13x13 控件上的对勾三点折线（Chrome 实测像素坐标：
// 短臂 (3,7)→(5,9)，长臂 (5,9)→(9,3)），按控件实际尺寸等比缩放。
var checkPoints = [3][2]float64{{3, 7}, {5, 9}, {9, 3}}

// drawCheckMark 画 1px 线宽的白色对勾（Bresenham 整数线段，无抗锯齿——
// 1px 实线在 13px 控件上比半像素 AA 更清晰）。
func drawCheckMark(g Graphics, bx, by, bw, bh int, c Color) {
	sx := float64(bw) / 13
	sy := float64(bh) / 13
	pt := func(p [2]float64) (int, int) {
		return bx + int(p[0]*sx+0.5), by + int(p[1]*sy+0.5)
	}
	x0, y0 := pt(checkPoints[0])
	x1, y1 := pt(checkPoints[1])
	x2, y2 := pt(checkPoints[2])
	drawLinePixels(g, x0, y0, x1, y1, c)
	drawLinePixels(g, x1, y1, x2, y2, c)
}

// drawLinePixels 用 Bresenham 逐像素画线段（含两端点）。
func drawLinePixels(g Graphics, x0, y0, x1, y1 int, c Color) {
	dx, dy := absInt(x1-x0), -absInt(y1-y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		g.DrawColor(x0, y0, 1, 1, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// fillRadioMark 画选中 radio：外圆环 + 留白 + 内实心圆。半径口径按 Chrome
// 实测并在 13px 网格上取整到「像素中心落在 ±0.5 AA 带中点」——
// 内圆 0.54r-0.02（13px → 3.49，轴向实心到 3px）、留白内缘 0.69r+0.02
// （13px → 4.505，轴向 4px 处为纯白）、外缘 = 控件半径 r（轴向 5-6px 实色环），
// 即「点 7px + 白缝 1px + 环 2px」。覆盖率 AA 复用 paintCoverage。
func fillRadioMark(g Graphics, bx, by, bw, bh int, c Color) {
	cx := float64(bx) + float64(bw)/2
	cy := float64(by) + float64(bh)/2
	rOut := math.Min(float64(bw), float64(bh)) / 2
	rDot := rOut*0.54 - 0.02
	rGap := rOut*0.69 + 0.02
	y0 := int(math.Floor(cy-rOut)) - 1
	y1 := int(math.Ceil(cy+rOut)) + 1
	x0 := int(math.Floor(cx-rOut)) - 1
	x1 := int(math.Ceil(cx+rOut)) + 1
	for y := y0; y <= y1; y++ {
		paintCoverage(g, y, x0, x1, c, func(x int) float64 {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			return math.Max(diskCoverage(d, rDot), annulusCoverage(d, rGap, rOut))
		})
	}
}

// diskCoverage 返回距圆心 d 的点对半径 rad 圆盘的覆盖率：d<=rad-0.5 全覆、
// d>=rad+0.5 全空，中间 1px 线性 AA 带（与 coverageAt 的 SDF 口径一致）。
func diskCoverage(d, rad float64) float64 {
	sd := rad - d
	switch {
	case sd <= -0.5:
		return 0
	case sd >= 0.5:
		return 1
	}
	return 0.5 + sd
}

// annulusCoverage 返回点对 [rIn, rOut] 圆环的覆盖率（外盘覆盖减内盘覆盖）。
func annulusCoverage(d, rIn, rOut float64) float64 {
	v := diskCoverage(d, rOut) - diskCoverage(d, rIn)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
