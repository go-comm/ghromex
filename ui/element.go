// 本文件是组件库内部的 DOM 与按键小工具；包文档见 doc.go。
package ui

import (
	"strings"

	"github.com/go-comm/ghromex/engine"
)

// newEl 创建一个带类名/文本的元素。
//
// 空类名/空文本不落属性：引擎按属性查找类名（GetAttribute("class")），
// class="" 与没有 class 对匹配等价，但会多出一次变更通知。
func newEl(doc engine.HTMLDocument, tag, class, text string) engine.HTMLElement {
	e := doc.CreateElement(tag)
	if class != "" {
		e.SetAttribute("class", class)
	}
	if text != "" {
		e.SetText(text)
	}
	return e
}

// setClass 设置类名，值没变时不碰属性。
//
// 引擎的 SetAttribute 无条件标记文档变更，主循环下一帧就会重排重绘；
// 状态栏/按钮刷新是高频路径，能省一次是一次。
func setClass(e engine.HTMLElement, class string) {
	if e.GetAttribute("class") == class {
		return
	}
	e.SetAttribute("class", class)
}

// firstChild 返回第一个子节点，空容器返回 nil（InsertBefore 的退路是追加）。
func firstChild(e engine.HTMLElement) engine.HTMLElement {
	if cs := e.Children(); len(cs) > 0 {
		return cs[0]
	}
	return nil
}

// enabled 判命令当前是否可用；没有 Enable 钩子表示恒可用。
func enabled(a Action) bool { return a.Enable == nil || a.Enable() }

// keyBind 是一条「快捷键 → 命令」绑定。
type keyBind struct {
	key  int
	mods uint16
	act  Action
}

// normalizeKey 统一字母大小写：SDL 在 Shift 按下时给大写，快捷键表里
// 写的是小写。
func normalizeKey(key int) int {
	if key >= 'A' && key <= 'Z' {
		return key - 'A' + 'a'
	}
	return key
}

// sameMods 按修饰键「家族」比较。
//
// 不能直接 ==：SDL 对左/右 Ctrl 只置各自的位（0x40/0x80），而
// engine.ModCtrl 是两位的掩码；只按了左 Ctrl 时 `mods == ModCtrl`
// 不成立。按家族取与再比布尔值，左右都算数。
func sameMods(have, want uint16) bool {
	for _, family := range []uint16{engine.ModCtrl, engine.ModShift, engine.ModAlt} {
		if (have&family != 0) != (want&family != 0) {
			return false
		}
	}
	return true
}

// parseShortcut 解析 "Ctrl+S"/"Alt+Left"/"Delete" 这类写法。
//
// 只支持引擎有键码的键：字母/数字（单字符）、Enter/Escape/Tab、方向键、
// Home/End/Delete。F 键在引擎里还没有常量，写了会解析失败——调用方
// 忽略这条绑定即可，不会误触。
func parseShortcut(s string) (key int, mods uint16, ok bool) {
	key = -1
	for _, raw := range strings.Split(s, "+") {
		p := strings.ToLower(strings.TrimSpace(raw))
		switch p {
		case "":
			continue
		case "ctrl", "control":
			mods |= engine.ModCtrl
		case "shift":
			mods |= engine.ModShift
		case "alt":
			mods |= engine.ModAlt
		case "enter", "return":
			key = engine.KeyEnter
		case "esc", "escape":
			key = engine.KeyEscape
		case "tab":
			key = engine.KeyTab
		case "left":
			key = engine.KeyLeft
		case "right":
			key = engine.KeyRight
		case "up":
			key = engine.KeyUp
		case "down":
			key = engine.KeyDown
		case "home":
			key = engine.KeyHome
		case "end":
			key = engine.KeyEnd
		case "del", "delete":
			key = engine.KeyDelete
		default:
			rs := []rune(p)
			if len(rs) != 1 {
				return 0, 0, false
			}
			key = normalizeKey(int(rs[0]))
		}
	}
	if key < 0 {
		return 0, 0, false
	}
	return key, mods, true
}
