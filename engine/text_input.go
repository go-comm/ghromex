package engine

import "strings"

// 输入编辑用到的控制键码。ASCII 区取值对齐 SDL2 SDL_keysym.sym
// （SDL_keyboard.h 的 SDLK_* 定义），SDL 后端可直接透传；
// 方向/Home/End 属扫描码区：SDLK = 0x40000000 | SDL_SCANCODE_*。
const (
	KeyBackspace = 0x08
	KeyTab       = 0x09
	KeyEnter     = 0x0D
	KeyEscape    = 0x1B
	KeyHome      = 0x4000004A // SDL_SCANCODE_HOME  (74)
	KeyEnd       = 0x4000004D // SDL_SCANCODE_END   (77)
	KeyRight     = 0x4000004F // SDL_SCANCODE_RIGHT (79)
	KeyLeft      = 0x40000050 // SDL_SCANCODE_LEFT  (80)
	KeyDown      = 0x40000051 // SDL_SCANCODE_DOWN  (81)
	KeyUp        = 0x40000052 // SDL_SCANCODE_UP    (82)
)

// editableInput 判断元素是否为可编辑输入：<input> 的 text 类型
// （type 未写按 HTML 规范即 text）与 <textarea>。
func editableInput(e HTMLElement) bool {
	base := inner(e)
	if base == nil {
		return false
	}
	switch strings.ToLower(base.tagName) {
	case "textarea":
		return true
	case "input":
		t := strings.ToLower(base.GetAttribute("type"))
		return t == "" || t == "text"
	}
	return false
}

// FocusedElement 返回文档当前聚焦元素，无则 nil。
func FocusedElement(doc HTMLDocument) HTMLElement {
	if d, ok := doc.(*htmlDocument); ok {
		return d.focus
	}
	return nil
}

// SetFocusedElement 设置聚焦元素；非可编辑输入等效清除（nil）。
// 聚焦时光标移到值末尾（与浏览器点选落位一致；精确落点由点击坐标修正）。
// 焦点变化触发文档变更通知——聚焦框/光标重绘与文本变化走同一条
// changed→重排重绘链路。
func SetFocusedElement(doc HTMLDocument, el HTMLElement) {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return
	}
	if el != nil && !editableInput(el) {
		el = nil
	}
	if d.focus != el {
		d.focus = el
		if el != nil {
			if b := inner(el); b != nil {
				b.caret = len([]rune(b.GetAttribute("value")))
				// Tab/程序化聚焦时光标可能落在可视区外（textarea 多行），
				// 立刻滚动到光标行，否则聚焦后首帧看不到光标。
				ensureCaretVisible(doc, b)
			}
		}
		d.markChanged()
	}
}

// focusTargets 按文档顺序收集全部可编辑输入（Tab 循环用）。
func focusTargets(doc HTMLDocument) []HTMLElement {
	var out []HTMLElement
	var walk func(HTMLElement)
	walk = func(e HTMLElement) {
		for _, c := range e.Children() {
			if editableInput(c) {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(doc)
	return out
}

// OnDocumentTextInput 向聚焦输入插入一段可打印文本（1 个或多个字符；
// 中文等 IME 组合完成的字符串走 SDL_TEXTINPUT 到这里）。插入点为当前
// 光标位置（默认值末尾），随后光标后移。返回是否被消费。
func OnDocumentTextInput(doc HTMLDocument, s string) bool {
	f := FocusedElement(doc)
	if f == nil || s == "" {
		return false
	}
	b := inner(f)
	if b == nil {
		return false
	}
	r := []rune(b.GetAttribute("value"))
	c := b.caret
	if c < 0 {
		c = 0
	}
	if c > len(r) {
		c = len(r)
	}
	b.SetAttribute("value", string(r[:c])+s+string(r[c:]))
	setCaret(doc, b, c+len([]rune(s)))
	return true
}

// OnDocumentKeyDown 向聚焦输入派发一个控制键（key 为 SDL keysym 值）。
// 消费：Backspace、Enter（仅 textarea 换行）、方向键、Home/End、Tab。
func OnDocumentKeyDown(doc HTMLDocument, key int) bool {
	f := FocusedElement(doc)
	if f == nil {
		return false
	}
	b := inner(f)
	if b == nil {
		return false
	}
	r := []rune(b.GetAttribute("value"))
	caret := b.caret
	if caret < 0 {
		caret = 0
	}
	if caret > len(r) {
		caret = len(r)
	}
	multiline := strings.EqualFold(b.tagName, "textarea")

	switch key {
	case KeyBackspace:
		if caret == 0 {
			return false
		}
		b.SetAttribute("value", string(r[:caret-1])+string(r[caret:]))
		setCaret(doc, b, caret-1)
		return true
	case KeyEnter:
		if !multiline {
			return false
		}
		b.SetAttribute("value", string(r[:caret])+"\n"+string(r[caret:]))
		setCaret(doc, b, caret+1)
		return true
	case KeyLeft:
		if caret == 0 {
			return true
		}
		setCaret(doc, b, caret-1)
		return true
	case KeyRight:
		if caret >= len(r) {
			return true
		}
		setCaret(doc, b, caret+1)
		return true
	case KeyHome:
		return moveCaretToLineEdge(doc, b, true)
	case KeyEnd:
		return moveCaretToLineEdge(doc, b, false)
	case KeyUp:
		return moveCaretVertical(doc, b, -1)
	case KeyDown:
		return moveCaretVertical(doc, b, 1)
	case KeyTab:
		list := focusTargets(doc)
		if len(list) == 0 {
			return false
		}
		next := list[0]
		for i, el := range list {
			if el == f && i+1 < len(list) {
				next = list[i+1]
				break
			}
		}
		SetFocusedElement(doc, next)
		return true
	}
	return false
}

// controlLayout 返回当前控件的行布局：textarea 为折行结果，
// 单行 input 视为「不折行的一行」（行高取字体行高）。
func controlLayout(doc HTMLDocument, b *htmlElement) textareaLayout {
	g := docGraphics(doc)
	if strings.EqualFold(b.tagName, "textarea") {
		return layoutTextarea(g, b)
	}
	px, bold, family := fontOf(b.computed)
	lines := wrapTextarea(g, b.GetAttribute("value"), maxInt(b.width<<10, 1), px, bold, family)
	_, lh := g.MeasureText("Mg", px, bold, family)
	if lh <= 0 {
		lh = b.height
	}
	return textareaLayout{x: b.x, y: b.y, w: b.width, h: b.height, lh: lh, lines: lines}
}

// moveCaretToLineEdge 把光标移到本行行首/行尾。start=true 为行首。
func moveCaretToLineEdge(doc HTMLDocument, b *htmlElement, start bool) bool {
	l := controlLayout(doc, b)
	if len(l.lines) == 0 {
		return true
	}
	row := caretRow(l.lines, b.caret)
	c := l.lines[row].start
	if !start {
		c = l.lines[row].end
	}
	setCaret(doc, b, c)
	return true
}

// moveCaretVertical 上/下移动一行：保持列位置，越界时贴边。
// 单行输入没有可移动的行，直接消费不改光标。
func moveCaretVertical(doc HTMLDocument, b *htmlElement, dy int) bool {
	g := docGraphics(doc)
	l := controlLayout(doc, b)
	if len(l.lines) == 0 {
		return true
	}
	px, bold, family := fontOf(b.computed)
	row := caretRow(l.lines, b.caret)
	target := row + dy
	if target < 0 {
		target = 0
	}
	if target >= len(l.lines) {
		target = len(l.lines) - 1
	}
	if target == row {
		return true
	}
	// 以当前光标 x 为目标行反推列位
	cx, _ := l.caretXY(g, b.caret, px, bold, family)
	c := l.caretFromPoint(g, cx, l.y+target*l.lh, px, bold, family)
	setCaret(doc, b, c)
	return true
}

// setControlCaretFromPoint 按点击坐标定位光标（textarea 按行/列，
// 单行 input 按前缀宽度就近吸附）。
func setControlCaretFromPoint(doc HTMLDocument, el HTMLElement, x, y int) {
	b := inner(el)
	if b == nil {
		return
	}
	g := docGraphics(doc)
	px, bold, family := fontOf(b.computed)
	var c int
	if strings.EqualFold(b.tagName, "textarea") {
		c = layoutTextarea(g, b).caretFromPoint(g, x, y, px, bold, family)
	} else {
		c = controlLayout(doc, b).caretFromPoint(g, x, y, px, bold, family)
	}
	setCaret(doc, b, c)
}

// updateClickFocus 在点击派发后更新焦点：命中可编辑输入（或其子盒）
// 则聚焦并按点击坐标落光标，点击别处清除焦点——与浏览器
// 「点空白失焦」一致。
func updateClickFocus(doc HTMLDocument, target HTMLElement, x, y int) {
	var next HTMLElement
	for e := target; e != nil; e = e.ParentElement() {
		if editableInput(e) {
			next = e
			break
		}
	}
	SetFocusedElement(doc, next)
	if next != nil {
		setControlCaretFromPoint(doc, next, x, y)
	}
}
