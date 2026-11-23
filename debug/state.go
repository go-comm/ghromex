// 本文件把引擎的 DOM 读接口翻译成 JSON：元素摘要、命中链、DOM 树快照。
//
// 这里所有函数都**必须在主循环 goroutine 上执行**（由 handler 包进 Do 的
// 任务里）：它们直接遍历文档树，而文档树按单线程访问设计。

package debug

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-comm/ghromex/engine"
)

// summarize 造一个元素摘要；el 为 nil 时返回 nil（JSON 里字段整个省略）。
func summarize(el engine.HTMLElement) *elemInfo {
	if el == nil {
		return nil
	}
	info := &elemInfo{
		Tag:   el.TagName(),
		ID:    el.ID(),
		Class: el.GetAttribute("class"),
		Box:   box{X: el.X(), Y: el.Y(), W: el.Width(), H: el.Height()},
	}
	// 滚动偏移只在非零时给：绝大多数元素没有滚动容器，恒为 0 的字段
	// 只会让 JSON 变胖、让脚本读着费劲。
	if sx, sy := engine.ScrollOffset(el); sx != 0 || sy != 0 {
		info.Scroll = &scrollBox{X: sx, Y: sy}
	}
	if info.Tag != "#text" {
		info.Selector = guessSelector(info)
		// GetValue 对非表单元素就是读 value 属性（没有则空串），
		// 所以这里对谁都能调，不必先判标签。
		if v := engine.GetValue(el); v != "" {
			info.Value = clipRunes(v, textClip)
		}
	}
	info.Text = clipRunes(el.Text(), textClip)
	return info
}

// guessSelector 造一个「大概能贴回去用」的选择器：#id 优先，其次 tag.class。
// 不保证唯一（页面上同类的按钮多得是），作用是让人一眼认出命中链上是谁，
// 并能在 /debug/text 里改改就查到值。
func guessSelector(info *elemInfo) string {
	if info.ID != "" {
		return "#" + info.ID
	}
	tag := info.Tag
	if tag == "" || tag == "#text" {
		return ""
	}
	for _, c := range strings.Fields(info.Class) {
		if c != "" {
			return tag + "." + c
		}
	}
	return tag
}

// same 判断两个元素是否同一个。
//
// 不直接写 a == b：接口相等要求动态类型可比较，引擎的元素目前都是指针
// （*htmlElement 或包装器指针）所以没问题，但万一将来出现按值传递的包装器，
// `==` 会当场 panic（"comparing uncomparable type"）——调试服务里为观测
// 而崩掉宿主不值当。走 reflect 拿指针，非指针类型一律判不同。
func same(a, b engine.HTMLElement) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Type() != vb.Type() {
		return false
	}
	if va.Kind() != reflect.Ptr {
		return false
	}
	return va.Pointer() == vb.Pointer()
}

// buildDOM 递归生成 DOM 树快照，depth/limit 双上限。
//
// 属性走**白名单**：引擎的元素接口只有 GetAttribute(name)，没有「列出全部
// 属性」的入口（几十万节点的页面里为每个节点建一张属性表也不是好主意）。
// 白名单之外的属性用 /debug/attr?selector=&name= 单点取。
func buildDOM(el engine.HTMLElement, depth, limit int, count *int, truncated *bool) domNode {
	*count++
	node := domNode{
		Tag:   el.TagName(),
		ID:    el.ID(),
		Class: el.GetAttribute("class"),
		Box:   box{X: el.X(), Y: el.Y(), W: el.Width(), H: el.Height()},
		Text:  clipRunes(el.Text(), textClip),
	}
	if node.Tag != "#text" {
		if v := engine.GetValue(el); v != "" {
			node.Value = clipRunes(v, textClip)
		}
		// 同 summarize：滚动偏移只在非零时给。
		if sx, sy := engine.ScrollOffset(el); sx != 0 || sy != 0 {
			node.Scroll = &scrollBox{X: sx, Y: sy}
		}
		attrs := make(map[string]string, 4)
		for _, name := range domAttrNames {
			if v := el.GetAttribute(name); v != "" {
				attrs[name] = clipRunes(v, textClip)
			}
		}
		if len(attrs) > 0 {
			node.Attrs = attrs
		}
	}
	if depth <= 0 {
		return node
	}
	for _, child := range el.Children() {
		if child == nil {
			continue
		}
		if *count >= limit {
			*truncated = true
			node.Truncated = true
			break
		}
		node.Children = append(node.Children, buildDOM(child, depth-1, limit, count, truncated))
	}
	return node
}

// domAttrNames 是 DOM 快照里会读的属性白名单。
var domAttrNames = []string{
	"id", "class", "style", "href", "src", "alt", "title", "type", "name",
	"value", "placeholder", "disabled", "readonly", "checked", "selected",
	"multiple", "rows", "cols", "target", "rel", "width", "height",
	"tabindex", "role", "aria-label", "lang", "dir",
}

// querySelector 在**当前文档**里找一个元素（必须主线程调用）。
func (s *Server) querySelector(sel string) (engine.HTMLElement, error) {
	doc := s.win.Document()
	if doc == nil {
		return nil, errNoDocument
	}
	el := doc.QuerySelector(sel)
	if el == nil {
		return nil, fmt.Errorf("debug: 选择器无匹配元素: %q", sel)
	}
	return el, nil
}

// clipRunes 按 rune 截断（中文一个字节都别切半），超长时补 …。
func clipRunes(s string, n int) string {
	if n <= 0 || len(s) <= n { // 字节数已达标时不必再数 rune（常见情形）
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// cursorName 把指针形状翻成 CSS 里那个词（观测输出用，不参与逻辑）。
func cursorName(c engine.Cursor) string {
	switch c {
	case engine.CursorDefault:
		return "default"
	case engine.CursorText:
		return "text"
	case engine.CursorPointer:
		return "pointer"
	case engine.CursorMove:
		return "move"
	case engine.CursorNotAllowed:
		return "not-allowed"
	case engine.CursorWait:
		return "wait"
	case engine.CursorHelp:
		return "help"
	case engine.CursorCrosshair:
		return "crosshair"
	case engine.CursorGrab:
		return "grab"
	case engine.CursorGrabbing:
		return "grabbing"
	case engine.CursorEWResize:
		return "ew-resize"
	case engine.CursorNSResize:
		return "ns-resize"
	case engine.CursorNESWResize:
		return "nesw-resize"
	case engine.CursorNWSEResize:
		return "nwse-resize"
	}
	return "shape" + strconv.Itoa(int(c))
}

// parseInt 读一个**必填**的查询参数。
func parseInt(v, name string) (int, error) {
	if v == "" {
		return 0, fmt.Errorf("debug: 缺少参数 %s", name)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("debug: 参数 %s 不是整数: %q", name, v)
	}
	return n, nil
}

// parseIntDefault 读一个可省略的整数参数（-1 表示「不限」，由调用方解释）。
func parseIntDefault(v, name string, def int) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("debug: 参数 %s 不是整数: %q", name, v)
	}
	return n, nil
}
