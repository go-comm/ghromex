// 本文件是状态栏；包文档见 doc.go。
package ui

// buildStatus 生成状态栏：左侧文本 + 右侧提示，顶部一条分隔线。
//
// 状态栏始终生成（哪怕初始为空）：它是框架感的一部分，应用随时可以
// 用 SetStatus 写进去，不必为了「先有状态栏」再改 Spec 重装。
func (s *Shell) buildStatus() {
	s.status = newEl(s.doc, "div", "ui-status", "")
	s.status.AppendChild(newEl(s.doc, "div", "ui-hairline", ""))
	s.statusLeft = newEl(s.doc, "span", "", s.spec.Status)
	s.statusRight = newEl(s.doc, "span", "ui-status-right", s.spec.StatusRight)
	s.status.AppendChild(s.statusLeft, s.statusRight)
	s.doc.Body().AppendChild(s.status)
}
