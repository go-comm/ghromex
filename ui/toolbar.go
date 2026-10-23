// 本文件是工具栏；包文档见 doc.go。
package ui

import "github.com/go-comm/ghromex/engine"

// toolView 是一个工具栏按钮的 DOM 句柄。
type toolView struct {
	act Action
	el  engine.HTMLElement
}

// toolClass 返回按钮当前应有的类名。
//
// 有图标时是方形图标按钮（is-icon），没有图标退回文字按钮（is-label）：
// 应用不必为每个命令都配图标，混用时也不会把按钮压扁。
func toolClass(a Action) string {
	class := "system-ui-btn system-ui-tool"
	if iconGlyph(a.Icon) == "" {
		class += " is-label"
	} else {
		class += " is-icon"
	}
	if !enabled(a) {
		class += " is-disabled"
	}
	return class
}

// toolTip 组合按钮的悬停提示文案：标签加快捷键（"保存  Ctrl+S"，浏览器
// 工具栏同款）——图标按钮丢掉的文字在这里还给用户。
func toolTip(a Action) string {
	switch {
	case a.Label == "":
		return a.Shortcut
	case a.Shortcut == "":
		return a.Label
	}
	return a.Label + "  " + a.Shortcut
}

// buildToolbar 生成工具栏。
//
// 图标按钮只画图标，文字进原生 title 提示（悬停气泡由引擎绘制）；没有
// 图标时退回文字按钮。横向排列靠 inline-block（引擎没有 flex）；分组用
// 1px 的竖线。
func (s *Workbench) buildToolbar() {
	s.toolbar = newEl(s.doc, "div", "system-ui-toolbar", "")
	for _, a := range s.spec.Tools {
		if a.Sep {
			s.toolbar.AppendChild(newEl(s.doc, "div", "system-ui-sep", ""))
			continue
		}
		if a.slot != nil {
			// 槽位：现成控件套一层定位 div，控件本体就是应用手里那一个
			// （Element 直接搬进来，不复制、不换 id）。
			if el := a.slot.Element(); el != nil {
				wrap := newEl(s.doc, "div", "system-ui-tool-slot", "")
				wrap.AppendChild(el)
				s.toolbar.AppendChild(wrap)
			}
			continue
		}
		tv := &toolView{act: a, el: newEl(s.doc, "div", toolClass(a), "")}
		if tip := toolTip(a); tip != "" {
			// 悬停提示走原生 title 属性：引擎的提示气泡负责显示
			// （图标按钮的文字只在提示里出现）。
			tv.el.SetAttribute("title", tip)
		}
		if ic := newIcon(s.doc, a.Icon); ic != nil {
			tv.el.AppendChild(ic)
		} else {
			tv.el.SetText(a.Label)
		}
		act := a
		tv.el.OnClick(func(*engine.MouseEvent) {
			// 点击时再判一次：类名可能还没随 Refresh 更新，但命中的
			// 命令已经不可用了。
			if !enabled(act) {
				return
			}
			s.invoke(act)
		})
		s.tools = append(s.tools, tv)
		s.toolbar.AppendChild(tv.el)
	}
	s.toolbar.AppendChild(newEl(s.doc, "div", "system-ui-hairline", ""))
	s.doc.Body().AppendChild(s.toolbar)
}
