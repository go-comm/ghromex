// 本文件是状态栏；包文档见 doc.go。
package ui

import (
	"fmt"

	"github.com/go-comm/ghromex/engine"
)

// StatusSegment 是状态栏左侧的一段文字。
//
// 段按切片顺序从左到右排列，段与段之间自动加 1px 分隔线。固定 Width 的
// 段适合「文件名 / 行列 / 字符数」这类需要稳定列宽的内容；Width 为 0 的
// 段按内容自适应，文本变化会推动后面的段。
type StatusSegment struct {
	Key    string           // 更新用；空时按位置生成，调用方应保证唯一
	Text   string           // 初始文字
	Width  int              // 0 = 自适应；>0 = 固定宽度（含 padding，单位 px）
	Align  engine.TextAlign // 段内对齐，零值即左对齐
	Hidden bool             // 为 true 时不生成该段，也不生成它前面的分隔线
}

// statusSegView 是一个已生成的段句柄。
type statusSegView struct {
	key string
	el  engine.HTMLElement
}

// buildStatus 生成状态栏：左侧分段轨道 + 右侧提示，顶部一条分隔线。
//
// 状态栏始终生成（哪怕初始为空）：它是框架感的一部分，应用随时可以
// 用 SetStatus / SetStatusSegment 写进去，不必为了「先有状态栏」再改
// Spec 重装。
func (s *Workbench) buildStatus() {
	s.status = newEl(s.doc, "div", "system-ui-status", "")
	s.status.AppendChild(newEl(s.doc, "div", "system-ui-hairline", ""))

	s.statusTrack = newEl(s.doc, "div", "system-ui-status-track", "")
	s.statusRight = newEl(s.doc, "span", "system-ui-status-right", "")
	s.status.AppendChild(s.statusTrack, s.statusRight)
	s.doc.Body().AppendChild(s.status)

	segs := s.spec.StatusSegments
	if len(segs) == 0 && s.spec.Status != "" {
		segs = []StatusSegment{{Key: "status", Text: s.spec.Status}}
	}
	s.SetStatusSegments(segs)
	s.SetStatusRight(s.spec.StatusRight)
}

// SetStatusSegments 重建左侧全部分段。
//
// 结构变化（增删段、改宽度/对齐/隐藏）走这里；只改文字用
// SetStatusSegment，避免每次光标移动都重建 DOM。
func (s *Workbench) SetStatusSegments(segs []StatusSegment) {
	if s == nil || s.statusTrack == nil {
		return
	}
	// RemoveChild 会改写 children 切片，先拷一份再遍历删除。
	children := append([]engine.HTMLElement(nil), s.statusTrack.Children()...)
	s.statusTrack.RemoveChild(nil, children...)

	s.statusSegs = nil
	s.statusLeft = nil
	visible := 0
	for i, seg := range segs {
		if seg.Hidden {
			continue
		}
		if visible > 0 {
			s.statusTrack.AppendChild(newEl(s.doc, "span", "system-ui-status-sep", ""))
		}

		key := seg.Key
		if key == "" {
			key = fmt.Sprintf("seg-%d", i)
		}
		el := newEl(s.doc, "span", "system-ui-status-seg", seg.Text)
		if visible == 0 {
			addClass(el, "is-first")
		}
		style := ""
		if seg.Width > 0 {
			style += fmt.Sprintf("width:%dpx;", seg.Width)
		}
		switch seg.Align {
		case engine.TextAlignCenter:
			style += "text-align:center;"
		case engine.TextAlignRight:
			style += "text-align:right;"
		}
		if style != "" {
			el.SetAttribute("style", style)
		}
		s.statusTrack.AppendChild(el)
		s.statusSegs = append(s.statusSegs, statusSegView{key: key, el: el})
		if s.statusLeft == nil {
			s.statusLeft = el
		}
		visible++
	}
}

// SetStatusSegment 按 Key 更新一段文字；找不到 Key 时返回 false。
//
// 只改文本，不重建段结构：适合光标位置、字符数这类高频更新。
func (s *Workbench) SetStatusSegment(key, text string) bool {
	if s == nil {
		return false
	}
	for i := range s.statusSegs {
		if s.statusSegs[i].key != key {
			continue
		}
		if s.statusSegs[i].el.Text() != text {
			s.statusSegs[i].el.SetText(text)
		}
		return true
	}
	return false
}

// StatusSegment 返回一段当前文字；没有该 Key 时返回 "", false。
func (s *Workbench) StatusSegment(key string) (string, bool) {
	if s == nil {
		return "", false
	}
	for i := range s.statusSegs {
		if s.statusSegs[i].key == key {
			return s.statusSegs[i].el.Text(), true
		}
	}
	return "", false
}

// SetStatus 更新左侧文本。
//
// 兼容旧 API：有 Key 为 "status" 的段时更新它；没有分段时创建一个；
// 已有自定义分段但没有 "status" 时更新最左一段。新代码建议直接用
// SetStatusSegment。
func (s *Workbench) SetStatus(text string) {
	if s == nil || s.statusTrack == nil {
		return
	}
	if s.SetStatusSegment("status", text) {
		return
	}
	if len(s.statusSegs) == 0 {
		s.SetStatusSegments([]StatusSegment{{Key: "status", Text: text}})
		return
	}
	if s.statusSegs[0].el.Text() != text {
		s.statusSegs[0].el.SetText(text)
	}
}

// SetStatusRight 更新状态栏右侧文本（提示/计数/快捷键）。
//
// 右侧消息固定宽度、右对齐；有消息时左侧轨道自动预留
// Theme.StatusRightWidth，没有消息时收起预留。
func (s *Workbench) SetStatusRight(text string) {
	if s == nil || s.statusRight == nil {
		return
	}
	if s.statusRight.Text() != text {
		s.statusRight.SetText(text)
	}
	if text == "" {
		removeClass(s.status, "has-right")
		return
	}
	addClass(s.status, "has-right")
}
