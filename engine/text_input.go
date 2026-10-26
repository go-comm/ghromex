package engine

import (
	"os"
	"strconv"
	"strings"
)

// 输入编辑用到的控制键码。ASCII 区取值对齐 SDL2 SDL_keysym.sym
// （SDL_keyboard.h 的 SDLK_* 定义），SDL 后端可直接透传；
// 方向/Home/End 属扫描码区：SDLK = 0x40000000 | SDL_SCANCODE_*；
// Delete 是例外：SDL_keycode.h 直接把它定义为 ASCII DEL(0x7F)。
const (
	KeyBackspace = 0x08
	KeyTab       = 0x09
	KeyEnter     = 0x0D
	KeyEscape    = 0x1B
	KeyHome      = 0x4000004A // SDL_SCANCODE_HOME  (74)
	KeyDelete    = 0x7F       // SDLK_DELETE（SDL_keycode.h 直接定义为 ASCII DEL，不是扫描码区！）
	KeyEnd       = 0x4000004D // SDL_SCANCODE_END   (77)
	KeyRight     = 0x4000004F // SDL_SCANCODE_RIGHT (79)
	KeyLeft      = 0x40000050 // SDL_SCANCODE_LEFT  (80)
	KeyDown      = 0x40000051 // SDL_SCANCODE_DOWN  (81)
	KeyUp        = 0x40000052 // SDL_SCANCODE_UP    (82)
)

// 修饰键位掩码，取值对齐 SDL2 SDL_Keymod（SDL_keycode.h）：
// KMOD_LSHIFT/RESHIFT = 0x1/0x2、LCTRL/RCTRL = 0x40/0x80、
// LALT/RALT = 0x100/0x200、LGUIM/RGUIM = 0x400/0x800。
// 后端把 KeyDown 的 keysym.mod 原样传入，引擎据此识别 Ctrl+A / Ctrl+C。
const (
	ModShift = 0x0003
	ModCtrl  = 0x00C0
	ModAlt   = 0x0300
	ModGUI   = 0x0C00
)

// editableInput 判断元素是否为可编辑输入：<input> 的 text/password 类型
// （type 未写按 HTML 规范即 text）与 <textarea>。password 与 text 同为
// 可聚焦输入框（编辑语义一致，仅绘制时掩码，见 displayValue）。
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
		return t == "" || t == "text" || t == "password"
	}
	return false
}

// displayValue 返回输入的显示文本：password 逐位掩码为 ●（每位一个实心
// 圆点，与浏览器一致），其余原样。掩码 rune 数与原值一致，光标下标、
// 行区间在原值与显示文本间可直接互换；但宽度不同（● ≠ 明文字符），
// 所以一切按显示文本做的测量——controlLayout（点击落光标/Home/End 行
// 边界）、渲染截断与光标前缀宽、SVG 导出——必须统一走这里，否则光标
// 会按明文宽度画到错位。
func displayValue(b *htmlElement) string {
	v := b.GetAttribute("value")
	if v == "" || !strings.EqualFold(b.tagName, "input") ||
		strings.ToLower(b.GetAttribute("type")) != "password" {
		return v
	}
	return strings.Repeat("●", len([]rune(v)))
}

// focusableElement 判断元素是否可聚焦：可编辑输入（input/textarea）、
// button、select、带 href 的链接，以及带 tabindex 的元素。
//
// 聚焦与「可编辑」是两件事：按钮/链接有焦点（:focus 样式要生效），
// 但没有光标与选区——光标那套状态仍只挂在可编辑输入上。
func focusableElement(e HTMLElement) bool {
	if e == nil {
		return false
	}
	if editableInput(e) {
		return true
	}
	switch strings.ToLower(e.TagName()) {
	case "button", "select":
		return true
	case "a":
		return e.GetAttribute("href") != ""
	}
	return e.GetAttribute("tabindex") != ""
}

// FocusedElement 返回文档当前聚焦元素，无则 nil。
func FocusedElement(doc HTMLDocument) HTMLElement {
	if d, ok := doc.(*htmlDocument); ok {
		return d.focus
	}
	return nil
}

// SetFocusedElement 设置聚焦元素；不可聚焦的元素等效清除（nil）。
// 首次聚焦某控件时光标移到值末尾（与浏览器点选落位一致；精确落点由
// 点击坐标修正）；再次聚焦保留该控件原有的光标与选区。
// 焦点变化触发文档变更通知——聚焦框/光标重绘与文本变化走同一条
// changed→重排重绘链路；:focus 样式也因此更新。
//
// 失焦不丢弃编辑状态：选区与光标原样留在控件上，只是停止绘制（渲染
// 路径按 focused 决定是否画选区）。页内菜单（记事本的「复制/剪切/
// 粘贴」）点击时编辑区会先失焦，若失焦即清空选区，菜单动作就无选中
// 可用；保留状态后，应用层把焦点交还给编辑区即可继续操作。
func SetFocusedElement(doc HTMLDocument, el HTMLElement) {
	d, ok := doc.(*htmlDocument)
	if !ok {
		return
	}
	if el != nil && !focusableElement(el) {
		el = nil
	}
	if !sameElement(d.focus, el) {
		d.focus = el
		// 光标与选区只挂在可编辑输入上（按钮/链接只有「有焦点」这一态）。
		if el != nil && editableInput(el) {
			if b := inner(el); b != nil {
				if !b.caretInit {
					// 首次聚焦：光标落到值末尾（精确落点由点击坐标修正）。
					b.caret = len([]rune(b.GetAttribute("value")))
					b.selAnchor = b.caret
					b.caretInit = true
				} else {
					// 保留原光标与选区，但失焦期间值可能被改过（如程序化
					// 设值），下标要按新长度夹紧，否则光标会落到值外。
					n := len([]rune(b.GetAttribute("value")))
					b.caret = clampCaret(b.caret, n)
					b.selAnchor = clampCaret(b.selAnchor, n)
				}
				// Tab/程序化聚焦时光标可能落在可视区外（textarea 多行），
				// 立刻滚动到光标行，否则聚焦后首帧看不到光标。
				ensureCaretVisible(doc, b)
			}
		}
		d.markChanged()
	}
}

// CaretPosition 返回可编辑控件的光标位置（行、列，均从 1 起）。
// textarea 按折行结果分行（软折行也算新行），单行 input 恒为第 1 行。
// 返回 false 表示该元素不是可编辑控件（或没有光标状态）。
//
// 失焦后仍返回最后的光标位置（编辑状态不随失焦丢弃，见
// SetFocusedElement），因此状态栏之类的 UI 可持续显示光标位置。
func CaretPosition(el HTMLElement) (line, col int, ok bool) {
	b := inner(el)
	if b == nil || !editableInput(el) {
		return 0, 0, false
	}
	caret := clampCaret(b.caret, len([]rune(b.GetAttribute("value"))))
	if strings.EqualFold(b.tagName, "textarea") {
		// 折行宽度来自布局结果。还没布局（首次重排之前，应用设完值就急着
		// 刷状态栏）或已经 display:none 时它是 0，按 0 宽折行会把每个字符
		// 折成一行，行列号就成了「行 29」这种没意义的数字。这种时候退回
		// 按硬换行分行：逻辑行列至少和文本本身对得上。
		if b.width <= 0 {
			line, col = logicalLineCol(b.GetAttribute("value"), caret)
			return line, col, true
		}
		l := layoutTextarea(docGraphics(docOf(el)), b)
		if len(l.lines) == 0 {
			return 1, 1, true
		}
		row := caretRow(l.lines, caret)
		return row + 1, caret - l.lines[row].start + 1, true
	}
	return 1, caret + 1, true
}

// logicalLineCol 按硬换行（\n）算光标所在的逻辑行列（均从 1 起）。
func logicalLineCol(value string, caret int) (int, int) {
	runes := []rune(value)
	if caret > len(runes) {
		caret = len(runes)
	}
	line, col := 1, 1
	for i := 0; i < caret; i++ {
		if runes[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
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
// 光标位置（默认值末尾），随后光标后移；若有选区则先删除选区再插入
// （浏览器同语义）。返回是否被消费。
//
// 控制字符（<0x20 与 0x7F）被丢弃：Windows 会把 Ctrl+C / Ctrl+V 这类
// 组合键以控制码（0x03 / 0x16）的形式送进文本输入通道，不过滤的话
// 剪贴板操作会在输入框里留下一个看不见的控制字符。
func OnDocumentTextInput(doc HTMLDocument, s string) bool {
	f := FocusedElement(doc)
	if f == nil || s == "" {
		return false
	}
	b := inner(f)
	if b == nil || !editableInput(f) {
		// 焦点在按钮/链接等非编辑元素上：没有光标可插，文本不消费
		//（浏览器同语义：按钮聚焦时打字不会改按钮的 value）。
		return false
	}
	s = stripControlChars(s)
	if s == "" {
		return false
	}
	insertTextAtCaret(doc, b, s)
	return true
}

// OnDocumentPaste 向聚焦输入粘贴文本（Ctrl+V 的引擎侧入口）。
//
// 剪贴板读取属平台能力，由后端（SDL2）读出后调用本函数——引擎不碰
// 系统剪贴板，保持纯 Go、无显示环境可测（与 Ctrl+C 的分工一致：
// 引擎给文本，后端写/读剪贴板）。
//
// 语义对齐浏览器：
//   - 在光标处插入；若有选区则先删除选区（粘贴替换选区）；
//   - 单行 input 粘贴多行文本时折成单行（换行折成空格，连续换行只留
//     一个空格）；
//   - textarea 保留换行（多行粘贴即多行文本）。
//
// 返回是否被消费（无焦点 / 无可粘贴内容时不消费）。
func OnDocumentPaste(doc HTMLDocument, text string) bool {
	f := FocusedElement(doc)
	if f == nil || text == "" {
		return false
	}
	b := inner(f)
	if b == nil || !editableInput(f) {
		// 焦点不在可编辑元素上：无处可粘（按钮的 value 不该被粘贴改写）。
		return false
	}
	if !strings.EqualFold(b.tagName, "textarea") {
		text = stripLineBreaks(text)
	} else {
		// textarea 保留换行，但换行符本身要归一：Windows 剪贴板/文件里
		// 是 CRLF，'\r' 留在 value 里会被当字形画出来（行尾方块）。
		text = normalizeNewlines(text)
	}
	if text == "" {
		return false
	}
	insertTextAtCaret(doc, b, text)
	return true
}

// insertTextAtCaret 在光标处插入文本：有选区则先删除选区内容（浏览器
// 「输入/粘贴替换选区」语义），插入后光标落在插入文本末尾、选区折叠。
func insertTextAtCaret(doc HTMLDocument, b *htmlElement, s string) {
	r := []rune(b.GetAttribute("value"))
	n := len(r)
	lo, hi, hasSel := selectionRange(b)
	c := clampCaret(b.caret, n)
	if hasSel {
		c = lo
	} else {
		hi = c
	}
	b.SetAttribute("value", string(r[:c])+s+string(r[hi:]))
	setCaret(doc, b, c+len([]rune(s)))
}

// stripControlChars 丢弃控制字符（<0x20 与 0x7F）。文本输入通道理论上
// 只送可打印字符，但 Windows 的 Ctrl+C/Ctrl+V 会以控制码混入。
func stripControlChars(s string) string {
	clean := true
	for _, r := range s {
		if r < 0x20 || r == 0x7F {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7F {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// normalizeNewlines 把 Windows/旧 Mac 换行统一成 '\n'：CRLF 与孤立 CR
// 都折成一个 LF。HTML 规范在解析前就做这一步；textarea 的 value 是纯
// 文本，混进 '\r' 会被当成字形画出来（行尾多一个方块），并且让光标/选区
// 的 rune 下标与折行区间错位。
func normalizeNewlines(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// stripLineBreaks 把多行文本折成单行：每个换行符折成一个空格，连续
// 换行只留一个空格，行首换行不留空格（对齐 Chrome 往单行 input 粘贴
// 多行文本的口径）。
func stripLineBreaks(s string) string {
	runes := []rune(s)
	hasBreak := false
	for _, r := range runes {
		if r == '\n' || r == '\r' {
			hasBreak = true
			break
		}
	}
	if !hasBreak {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	last := rune(0) // 已输出的最后一个字符（0 = 还没有输出）
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != '\n' && r != '\r' {
			b.WriteRune(r)
			last = r
			continue
		}
		if r == '\r' && i+1 < len(runes) && runes[i+1] == '\n' {
			i++ // \r\n 视作一个换行
		}
		if last == 0 || last == ' ' {
			continue // 行首或已是空格：不重复补
		}
		b.WriteRune(' ')
		last = ' '
	}
	return b.String()
}

// clampCaret 把光标/锚点下标夹到 [0, n]（n 为 value 的 rune 长度）。
func clampCaret(v, n int) int {
	if v < 0 {
		return 0
	}
	if v > n {
		return n
	}
	return v
}

// baselineAdjust 是全局文本基线补偿（像素，正数=下移）。
//
// 用途：与 Chrome/DirectWrite 对照时的微调。引擎的行盒高度取自字体的
// hhea/OS2 度量（与 Chrome 同源 ✓），但两边的**取整**不同（引擎按整数像素
// 取整 ascent/descent，Chrome 走 DirectWrite 的 26.6 定点），同一字号下
// 基线可差 0.5~1px。这个补偿只挪**基线**、不排行盒高度，故不影响行距
// 与任何盒的尺寸。
//
// 默认 0（不动）。可用环境变量 GHROMEX_BASELINE_ADJ 设定，或调用
// SetBaselineAdjust 运行时调整。
var baselineAdjust = envBaselineAdjust()

// envBaselineAdjust 读环境变量里的基线补偿值（非法值按 0 处理）。
func envBaselineAdjust() int {
	v := strings.TrimSpace(os.Getenv("GHROMEX_BASELINE_ADJ"))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// SetBaselineAdjust 设置全局文本基线补偿（像素，正数=下移），返回设置前的
// 值。用于与 Chrome 对照时的微调；0 = 不补偿（默认）。
func SetBaselineAdjust(px int) int {
	old := baselineAdjust
	baselineAdjust = px
	return old
}

// BaselineAdjust 返回当前全局文本基线补偿（像素）。
func BaselineAdjust() int { return baselineAdjust }

// OnDocumentKeyDown 向聚焦输入派发一个控制键（key 为 SDL keysym 值）。
// 消费：Backspace、Delete、Enter（仅 textarea 换行）、方向键、Home/End、Tab。
// 这是无修饰键入口（保持既有调用方兼容）；带 Shift/Ctrl 的按键走
// OnDocumentKeyDownMods。
func OnDocumentKeyDown(doc HTMLDocument, key int) bool {
	return OnDocumentKeyDownMods(doc, key, 0)
}

// OnDocumentKeyDownMods 处理带修饰键的按键：Ctrl+A 全选、Ctrl+C 表示
// 「有选区可复制」（真正的剪贴板写入由后端完成——引擎不依赖 SDL）、
// Shift+方向键/Home/End 扩展选区。其余按键行为与 OnDocumentKeyDown 相同。
func OnDocumentKeyDownMods(doc HTMLDocument, key int, mods uint16) bool {
	f := FocusedElement(doc)
	if f == nil {
		return false
	}
	b := inner(f)
	if b == nil {
		return false
	}
	// 元素级 keydown 先派发：应用层要能看到所有按键（含修饰键），
	// 返回 true 表示已消费，引擎的默认动作不再执行。
	if b.keyHandler != nil {
		if b.keyHandler(&KeyEvent{Type: EventKeyDown, Key: key, Mods: mods}) {
			return true
		}
	}
	if mods&ModCtrl != 0 {
		switch key {
		case 'a', 'A':
			if !editableInput(f) {
				return false
			}
			n := len([]rune(b.GetAttribute("value")))
			setSelection(doc, b, 0, n)
			return true
		case 'c', 'C':
			if !editableInput(f) {
				return false
			}
			_, _, ok := selectionRange(b)
			return ok
		}
	}
	// Shift+方向键：扩展选区（锚点不动，活动端随按键移动）。
	// 带 Ctrl 时不走本分支——Ctrl+Shift+方向键是「按词扩展」，引擎未
	// 实现，按词会被降级成按字符；此处让 Ctrl+组合沿用无修饰键的折叠
	// 行为，至少不产生「按词」的错觉。
	if mods&ModShift != 0 && mods&ModCtrl == 0 {
		if !editableInput(f) {
			// 焦点不在可编辑元素上：没有选区可扩（按钮聚焦时按 Shift+方向键
			// 不该改出选区，更不该触发变更通知）。
			return false
		}
		if onShiftSelectKey(doc, b, key) {
			return true
		}
	}
	return onDocumentKeyDown(doc, key)
}

// onShiftSelectKey 处理 Shift+方向键/Home/End：把选区活动端（caret）
// 按按键方向移动，锚点（selAnchor）保持不动。
//
// 语义对齐浏览器（Blink 的 base/extent 模型）：
//   - 无选区：锚点即光标，按键从光标向该方向扩选一格；
//   - 已有选区：活动端沿按键方向移动一格（选区随之伸缩）；越过锚点
//     后选区反向（selectionRange 按 min/max 归一化，方向不丢）。
//
// 单行 input 没有上下行，Shift+Up/Down 不改变选区（按键仍被消费）。
func onShiftSelectKey(doc HTMLDocument, b *htmlElement, key int) bool {
	n := len([]rune(b.GetAttribute("value")))
	caret := clampCaret(b.caret, n)
	anchor := clampCaret(b.selAnchor, n)
	var target int
	switch key {
	case KeyLeft:
		target = caret - 1
		if target < 0 {
			target = 0
		}
	case KeyRight:
		target = caret + 1
		if target > n {
			target = n
		}
	case KeyUp:
		target = caretVerticalTarget(doc, b, -1, caret)
	case KeyDown:
		target = caretVerticalTarget(doc, b, 1, caret)
	case KeyHome:
		target = caretLineEdgeTarget(doc, b, true, caret)
	case KeyEnd:
		target = caretLineEdgeTarget(doc, b, false, caret)
	default:
		return false
	}
	setSelection(doc, b, anchor, target)
	return true
}

func onDocumentKeyDown(doc HTMLDocument, key int) bool {
	// Tab 循环与「焦点是否可编辑」无关：按钮/链接持有焦点时按 Tab，
	// 同样要在可编辑输入之间移动。
	if key == KeyTab {
		f := FocusedElement(doc)
		list := focusTargets(doc)
		if len(list) == 0 {
			return false
		}
		next := list[0]
		for i, el := range list {
			if sameElement(el, f) && i+1 < len(list) {
				next = list[i+1]
				break
			}
		}
		SetFocusedElement(doc, next)
		return true
	}
	f := FocusedElement(doc)
	if f == nil {
		return false
	}
	b := inner(f)
	if b == nil || !editableInput(f) {
		// 焦点在按钮/链接等非编辑元素上：没有光标与选区，编辑类按键
		// 不消费（浏览器同语义——按钮聚焦时按方向键不会改按钮的 value）。
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
	lo, hi, hasSel := selectionRange(b)
	multiline := strings.EqualFold(b.tagName, "textarea")

	switch key {
	case KeyBackspace:
		if hasSel {
			b.SetAttribute("value", string(r[:lo])+string(r[hi:]))
			setCaret(doc, b, lo)
			return true
		}
		if caret == 0 {
			return false
		}
		b.SetAttribute("value", string(r[:caret-1])+string(r[caret:]))
		setCaret(doc, b, caret-1)
		return true
	case KeyDelete:
		if hasSel {
			b.SetAttribute("value", string(r[:lo])+string(r[hi:]))
			setCaret(doc, b, lo)
			return true
		}
		if caret >= len(r) {
			return false
		}
		b.SetAttribute("value", string(r[:caret])+string(r[caret+1:]))
		setCaret(doc, b, caret)
		return true
	case KeyEnter:
		if !multiline {
			return false
		}
		if hasSel {
			b.SetAttribute("value", string(r[:lo])+"\n"+string(r[hi:]))
			setCaret(doc, b, lo+1)
			return true
		}
		b.SetAttribute("value", string(r[:caret])+"\n"+string(r[caret:]))
		setCaret(doc, b, caret+1)
		return true
	case KeyLeft:
		if hasSel {
			setCaret(doc, b, lo)
			return true
		}
		if caret == 0 {
			return true
		}
		setCaret(doc, b, caret-1)
		return true
	case KeyRight:
		if hasSel {
			setCaret(doc, b, hi)
			return true
		}
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
		if hasSel {
			setCaret(doc, b, caret) // 先折叠，再按折叠后的位置上下移
		}
		return moveCaretVertical(doc, b, -1)
	case KeyDown:
		if hasSel {
			setCaret(doc, b, caret)
		}
		return moveCaretVertical(doc, b, 1)
	}
	return false
}

// inputVisibleWindow 计算单行 input 的可见窗口：窗口起点 = textScrollX
// （像素，rune 边界向下取整），由 ensureCaretVisible 跟随光标驱动（Chrome
// scroll-follows-caret 同口径：Home/左移回首字时窗口滚回开头）。未被光标
// 定位过（textScrollSet=false）则保持"舍头保尾"——静态展示的长值与地址栏
// 历史显示的旧口径。渲染与光标映射（点击落光标/Home/End 行边界）必须同
// 口径——此前渲染按舍头保尾裁掉前缀，而光标映射仍从字符串头部量前缀宽，
// 点击可见区会把光标定位到不可见的前缀里，随后插入的字符落到错误位置。
func inputVisibleWindow(g Graphics, b *htmlElement, px, weightNum int, bold bool, family string) (text string, cut, tw, th int) {
	full := displayValue(b)
	runes := []rune(full)
	tw, th = measureTextW(g, full, px, weightNum, bold, family)
	// 按钮型不截断：auto 宽按 value 文本测量（Chrome 同语义）。
	typ := strings.ToLower(b.GetAttribute("type"))
	buttonLike := typ == "submit" || typ == "reset" || typ == "button"
	if !buttonLike {
		avail := b.width - 4
		maxScroll := tw - avail
		if maxScroll < 0 {
			maxScroll = 0
		}
		// 目标偏移：已定位用光标跟随值；未定位保尾（旧口径）。
		target := maxScroll
		if b.textScrollSet {
			target = b.textScrollX
			if target > maxScroll {
				target = maxScroll
			}
			if target < 0 {
				target = 0
			}
		}
		// 舍头到 rune 边界：舍掉累计宽不超过 target 的最长前缀。取整误差
		// 让可见宽略超 avail 时由绘制层按控件内盒裁剪（Chrome 同口径），
		// 不再强制舍满——保头（target=0）必须能看到首字。
		// 例外：保尾（target == maxScroll，含未定位默认）向上取整，保证末字完整可见
		// —— Chrome 的 max scroll 将窗口钉在文本末尾，旧口径同样“末尾完整可见”。
		dropped := 0
		for len(runes) > 1 {
			cw, _ := measureTextW(g, string(runes[0]), px, weightNum, bold, family)
			if cw <= 0 || dropped+cw > target {
				break
			}
			runes = runes[1:]
			dropped += cw
			tw, th = measureTextW(g, string(runes), px, weightNum, bold, family)
		}
		if target == maxScroll {
			for len(runes) > 1 && tw > avail {
				runes = runes[1:]
				tw, th = measureTextW(g, string(runes), px, weightNum, bold, family)
			}
		}
	}
	// 声明的 line-height 决定控件内**一行**的高度：文本的垂直居中位置、
	// 选区高亮与光标高度都按它算（Chrome 同口径）。
	if decl := resolvedLineHeight(b.computed); decl >= 0 {
		th = decl
	}
	return string(runes), len([]rune(full)) - len(runes), tw, th
}

// controlLayout 返回当前控件的行布局：textarea 为折行结果，
// 单行 input 视为「不折行的一行」（行高取字体行高）。
func controlLayout(doc HTMLDocument, b *htmlElement) textareaLayout {
	g := docGraphics(doc)
	if strings.EqualFold(b.tagName, "textarea") {
		return layoutTextarea(g, b)
	}
	px, weightNum, bold, family := fontOf(b.computed)
	// 按显示文本测量：password 走 ● 掩码（宽度与明文不同），点击落光标、
	// Home/End 行边界的前缀宽必须与绘制的掩码文本同口径。
	// 单行 input 的「行」= 可见窗口（舍头保尾），行区间按完整显示值
	// 编号，光标下标因此仍是完整值里的位置。
	visible, cut, tw, _ := inputVisibleWindow(g, b, px, weightNum, bold, family)
	full := len([]rune(displayValue(b)))
	lines := []textareaLine{{text: visible, start: cut, end: full, w: tw}}
	_, lh := measureTextW(g, "Mg", px, weightNum, bold, family)
	if lh <= 0 {
		lh = b.height
	}
	return textareaLayout{x: b.x, y: b.y, w: b.width, h: b.height, lh: lh, lines: lines}
}

// caretLineEdgeTarget 返回光标所在行的行首/行尾下标（start=true 为行首）。
//
// 单行 input：整条 value 就是一行 —— 行首/行尾即值首/值尾。不能取
// controlLayout 给的行首：那是被可见窗口裁掉的前缀起点，超宽输入按它
// 算会把光标送到看不见的区间（Home 键尤其明显）。
func caretLineEdgeTarget(doc HTMLDocument, b *htmlElement, start bool, caret int) int {
	if !strings.EqualFold(b.tagName, "textarea") {
		n := len([]rune(b.GetAttribute("value")))
		if start {
			return 0
		}
		return n
	}
	l := controlLayout(doc, b)
	if len(l.lines) == 0 {
		return caret
	}
	row := caretRow(l.lines, caret)
	if start {
		return l.lines[row].start
	}
	return l.lines[row].end
}

// moveCaretToLineEdge 把光标移到本行行首/行尾。start=true 为行首。
func moveCaretToLineEdge(doc HTMLDocument, b *htmlElement, start bool) bool {
	n := len([]rune(b.GetAttribute("value")))
	setCaret(doc, b, caretLineEdgeTarget(doc, b, start, clampCaret(b.caret, n)))
	return true
}

// caretVerticalTarget 计算上/下移动一行后的光标下标（保持列位置，越界
// 时贴边）。单行输入没有可移动的行，返回原光标。
func caretVerticalTarget(doc HTMLDocument, b *htmlElement, dy, caret int) int {
	g := docGraphics(doc)
	l := controlLayout(doc, b)
	if len(l.lines) == 0 {
		return caret
	}
	px, weightNum, bold, family := fontOf(b.computed)
	row := caretRow(l.lines, caret)
	target := row + dy
	if target < 0 {
		target = 0
	}
	if target >= len(l.lines) {
		target = len(l.lines) - 1
	}
	if target == row {
		return caret
	}
	// 以当前光标 x 为目标行反推列位
	cx, _ := l.caretXY(g, caret, px, weightNum, bold, family)
	return l.caretFromPoint(g, cx, l.y+target*l.lh, px, weightNum, bold, family)
}

// moveCaretVertical 上/下移动一行：保持列位置，越界时贴边。
// 单行输入没有可移动的行，直接消费不改光标。
func moveCaretVertical(doc HTMLDocument, b *htmlElement, dy int) bool {
	n := len([]rune(b.GetAttribute("value")))
	setCaret(doc, b, caretVerticalTarget(doc, b, dy, clampCaret(b.caret, n)))
	return true
}

// setControlCaretFromPoint 按点击坐标定位光标（textarea 按行/列，
// 单行 input 按前缀宽度就近吸附），并折叠选区。
func setControlCaretFromPoint(doc HTMLDocument, el HTMLElement, x, y int) {
	b := inner(el)
	if b == nil {
		return
	}
	setCaret(doc, b, controlCaretFromPoint(doc, b, x, y))
}

// updateClickFocus 在点击派发后更新焦点：命中可聚焦元素（或其子盒）
// 则聚焦，点击别处清除焦点——与浏览器「点空白失焦」一致。
//
// 命中的是可编辑输入时，按点击坐标落光标；按钮/链接只取焦点，没有光标。
func updateClickFocus(doc HTMLDocument, target HTMLElement, x, y int) {
	var next HTMLElement
	for e := target; e != nil; e = e.ParentElement() {
		if focusableElement(e) {
			next = e
			break
		}
	}
	SetFocusedElement(doc, next)
	if next != nil && editableInput(next) && !scrollbarClick(doc, next, x, y) {
		setControlCaretFromPoint(doc, next, x, y)
	}
}
