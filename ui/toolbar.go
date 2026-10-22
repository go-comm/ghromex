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
	class := "ui-btn ui-tool"
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

// buildToolbar 生成工具栏。
//
// 图标按钮只画图标（文字留在 Action.Label 里，给无图标兜底与读屏/文档用）。
// 横向排列靠 inline-block（引擎没有 flex）；分组用 1px 的竖线。
func (s *Shell) buildToolbar() {
	s.toolbar = newEl(s.doc, "div", "ui-toolbar", "")
	for _, a := range s.spec.Tools {
		if a.Sep {
			s.toolbar.AppendChild(newEl(s.doc, "div", "ui-sep", ""))
			continue
		}
		tv := &toolView{act: a, el: newEl(s.doc, "div", toolClass(a), "")}
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
	s.toolbar.AppendChild(newEl(s.doc, "div", "ui-hairline", ""))
	s.doc.Body().AppendChild(s.toolbar)
}
