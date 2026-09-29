package engine

import "strings"

// 输入编辑用到的控制键码。取值对齐 SDL2 SDL_keysym.sym 的 ASCII 键区
// （SDL_keyboard.h 的 SDLK_* 定义），SDL 后端可直接透传。
const (
	KeyBackspace = 0x08
	KeyTab       = 0x09
)

// editableInput 判断元素是否为可编辑输入：v1 支持 <input> 的 text 类型
// （type 未写按 HTML 规范即 text）。
func editableInput(e HTMLElement) bool {
	base := inner(e)
	if base == nil || !strings.EqualFold(base.tagName, "input") {
		return false
	}
	t := strings.ToLower(base.GetAttribute("type"))
	return t == "" || t == "text"
}

// FocusedElement 返回文档当前聚焦元素，无则 nil。
func FocusedElement(doc HTMLDocument) HTMLElement {
	if d, ok := doc.(*htmlDocument); ok {
		return d.focus
	}
	return nil
}

// SetFocusedElement 设置聚焦元素；非可编辑输入等效清除（nil）。
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

// OnDocumentTextInput 向聚焦输入派发一段可打印文本（1 个或多个字符；
// 中文等 IME 组合完成的字符串走 SDL_TEXTINPUT 到这里）。文本追加到
// 当前值末尾（v1 光标恒在行尾）。返回是否被消费。
func OnDocumentTextInput(doc HTMLDocument, s string) bool {
	f := FocusedElement(doc)
	if f == nil || s == "" {
		return false
	}
	f.SetAttribute("value", f.GetAttribute("value")+s)
	return true
}

// OnDocumentKeyDown 向聚焦输入派发一个控制键（key 为 SDL keysym 值）。
// 当前消费：Backspace（删最后一个字符，按 rune 安全）、Tab（循环焦点）。
func OnDocumentKeyDown(doc HTMLDocument, key int) bool {
	f := FocusedElement(doc)
	if f == nil {
		return false
	}
	switch key {
	case KeyBackspace:
		v := f.GetAttribute("value")
		if v == "" {
			return false
		}
		r := []rune(v)
		f.SetAttribute("value", string(r[:len(r)-1]))
		return true
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

// updateClickFocus 在点击派发后更新焦点：命中可编辑 input（或其子盒）
// 则聚焦它，点击别处清除焦点——与浏览器"点空白失焦"一致。
func updateClickFocus(doc HTMLDocument, target HTMLElement) {
	var next HTMLElement
	for e := target; e != nil; e = e.ParentElement() {
		if editableInput(e) {
			next = e
			break
		}
	}
	SetFocusedElement(doc, next)
}
