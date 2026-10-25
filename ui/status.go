// 本文件是状态栏；包文档见 doc.go。
package ui

import (
	"fmt"

	"github.com/go-comm/ghromex/engine"
)

// statusRightKey 是右侧糖消息的保留段键：SetStatusSegment/StatusSegment
// 用它走统一段路径读写右侧消息，应用自己的段不应占用该键。
const statusRightKey = "status-right"

// StatusSegment 是状态栏的一段文字。
//
// 整条状态栏只有一种结构——段。默认段进左侧轨道，按切片顺序从左到右
// 排列，段与段之间自动加 1px 分隔线；AnchorRight 为 true 的段进右侧
// 固定区（钉在状态栏右缘，区域宽度由 Theme.StatusRightWidth 统一预留），
// 多个右钉段在区内依次排布、同样带分隔线。固定 Width 的段适合
// 「文件名 / 行列 / 字符数」这类需要稳定列宽的内容；Width 为 0 的段
// 按内容自适应，文本变化会推动后面的段。
type StatusSegment struct {
	Key    string           // 更新用；空时按位置生成，调用方应保证唯一
	Text   string           // 初始文字
	Width  int              // 0 = 自适应；>0 = 固定宽度（含 padding，单位 px）
	Align  engine.TextAlign // 段内对齐；零值左侧段左对齐、右钉段右对齐（贴列右缘）
	Hidden bool             // 为 true 时不生成该段，也不生成它前面的分隔线

	// AnchorRight 把该段钉在状态栏右缘的固定区，不进左侧轨道。
	// 空文本的右钉段视为「无消息」：不生成、不留占位、不参与预留
	// （左段空文本仍占位）；右钉段跨重建保留的能力不受影响——
	// SetStatusSegment 按 Key 照常只改文字。
	//
	// 多个右钉段共享右区：全部给定 Width 时右区按「Σ段宽+分隔线」
	// 自动外扩（含任意自适应段则回退 Theme.StatusRightWidth 预留，
	// 超出被裁）。要多项稳定共存，逐段定宽即可。零值对齐按右区口径
	// 视为右对齐；糖消息缺位时最右一段收掉右内边距贴 right:12px 盒缘。
	AnchorRight bool
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
	s.statusRightBox = newEl(s.doc, "div", "system-ui-status-right", "")
	s.status.AppendChild(s.statusTrack, s.statusRightBox)
	s.doc.Body().AppendChild(s.status)

	segs := s.spec.StatusSegments
	if len(segs) == 0 && s.spec.Status != "" {
		segs = []StatusSegment{{Key: "status", Text: s.spec.Status}}
	}
	s.SetStatusSegments(segs)
	s.SetStatusRight(s.spec.StatusRight)
}

// SetStatusSegments 重建状态栏全部分段（左侧轨道 + 右侧固定区）。
//
// 结构变化（增删段、改宽度/对齐/隐藏、改钉边侧）走这里；只改文字用
// SetStatusSegment，避免每次光标移动都重建 DOM。AnchorRight 段进右区，
// 空文本视为无消息不生成；糖消息（SetStatusRight）跨重建保留，固定排在
// 右区最右。
func (s *Workbench) SetStatusSegments(segs []StatusSegment) {
	if s == nil || s.statusTrack == nil {
		return
	}
	// RemoveChild 会改写 children 切片，先拷一份再遍历删除。
	children := append([]engine.HTMLElement(nil), s.statusTrack.Children()...)
	s.statusTrack.RemoveChild(nil, children...)
	rightChildren := append([]engine.HTMLElement(nil), s.statusRightBox.Children()...)
	s.statusRightBox.RemoveChild(nil, rightChildren...)

	s.statusSegs = nil
	s.statusLeft = nil
	s.statusRight = nil
	s.rightVisible = 0
	s.lastRightSeg = nil
	visible := 0       // 左侧已生成段数
	rightSum := 0      // 可见右钉段显式宽度累计
	rightAuto := false // 有无自适应右钉段（无法预量）
	for i, seg := range segs {
		if seg.Hidden || (seg.AnchorRight && seg.Text == "") {
			continue
		}
		owner := s.statusTrack
		if seg.AnchorRight {
			owner = s.statusRightBox
		}
		// 分隔线只在同侧相邻两段之间生成；is-first 只给左侧首段。
		if (seg.AnchorRight && s.rightVisible > 0) ||
			(!seg.AnchorRight && visible > 0) {
			owner.AppendChild(newEl(s.doc, "span", "system-ui-status-sep", ""))
		}

		key := seg.Key
		if key == "" {
			key = fmt.Sprintf("seg-%d", i)
		}
		el := newEl(s.doc, "span", "system-ui-status-seg", seg.Text)
		if !seg.AnchorRight && visible == 0 {
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
		owner.AppendChild(el)
		s.statusSegs = append(s.statusSegs, statusSegView{key: key, el: el})
		if seg.AnchorRight {
			s.rightVisible++
			s.lastRightSeg = el
			if seg.Width > 0 {
				rightSum += seg.Width
			} else {
				rightAuto = true
			}
			continue
		}
		if s.statusLeft == nil {
			s.statusLeft = el
		}
		visible++
	}
	// 糖消息跨重建保留，重挂到右区最右（不参与段分隔线）。
	if s.statusRightText != "" {
		s.statusRight = newEl(s.doc, "span", "system-ui-status-msg", s.statusRightText)
		s.statusRightBox.AppendChild(s.statusRight)
		s.statusSegs = append(s.statusSegs,
			statusSegView{key: statusRightKey, el: s.statusRight})
	} else if s.lastRightSeg != nil {
		// 糖消息缺位：最右一段收掉右内边距，文字贴 right:12px 盒缘，
		// 与左区 is-first / 糖消息同一边距口径。
		addClass(s.lastRightSeg, "is-last")
	}
	s.layoutRightZone(rightSum, rightAuto)
	s.syncHasRight()
}

// layoutRightZone 回写右侧固定区宽度：多右项场景下主题预留可能装不下。
// 可见右钉段全部显式定宽时，按「Σ段宽 + 段间分隔线」外扩右区宽度与左
// 轨道垫宽（仅超出主题 StatusRightWidth 时写内联样式，且段宽含 padding、
// 稳定不随文字抖动）；只要有自适应段就无法预量，清掉内联样式回退主题
// 常量——超出部分由 overflow:hidden 裁掉，与旧口径一致。
func (s *Workbench) layoutRightZone(sum int, auto bool) {
	zone := 0
	if s.rightVisible > 0 && !auto {
		zone = sum + (s.rightVisible - 1)
	}
	if zone <= s.theme.StatusRightWidth {
		s.statusRightBox.RemoveAttribute("style")
		s.statusTrack.RemoveAttribute("style")
		return
	}
	s.statusRightBox.SetAttribute("style", fmt.Sprintf("width:%dpx;", zone))
	s.statusTrack.SetAttribute("style", fmt.Sprintf("padding-right:%dpx;", zone))
}

// SetStatusSegment 按 Key 更新一段文字；找不到 Key 时返回 false。
//
// 只改文本，不重建段结构：适合光标位置、字符数这类高频更新。保留键
// "status-right" 转交 SetStatusRight（右区显隐要同步 has-right 类，
// 通用段路径不碰类名）。
func (s *Workbench) SetStatusSegment(key, text string) bool {
	if s == nil {
		return false
	}
	if key == statusRightKey {
		if s.statusRightBox == nil {
			return false
		}
		s.SetStatusRight(text)
		return true
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
	// 兜底更新最左一段：rightVisible 段不在轨道里，statusLeft 才是。
	if s.statusLeft == nil {
		return
	}
	if s.statusLeft.Text() != text {
		s.statusLeft.SetText(text)
	}
}

// SetStatusRight 更新状态栏右侧消息（提示/计数/快捷键）。
//
// 这是右钉段的兼容糖：模型上等价于 Key 为保留键 "status-right" 的
// AnchorRight 段，固定排在右区最右、不参与段分隔线。有消息时左侧轨道
// 按 Theme.StatusRightWidth 预留宽度，置空收起预留。文字没变时不写
// DOM，高频调用代价低；新代码若要右侧多段，直接在 StatusSegments 里
// 放 AnchorRight 段。
func (s *Workbench) SetStatusRight(text string) {
	if s == nil || s.statusRightBox == nil {
		return
	}
	if s.statusRightText == text && (text == "") == (s.statusRight == nil) {
		return // 状态没变（含「无消息保持无消息」），不碰 DOM
	}
	s.statusRightText = text
	if text == "" {
		if s.statusRight != nil {
			s.statusRightBox.RemoveChild(nil, s.statusRight)
			s.statusRight = nil
		}
		s.dropStatusSeg(statusRightKey)
		// 糖让位：贴边位还给最右右钉段。
		if s.lastRightSeg != nil {
			addClass(s.lastRightSeg, "is-last")
		}
		s.syncHasRight()
		return
	}
	if s.statusRight == nil {
		s.statusRight = newEl(s.doc, "span", "system-ui-status-msg", text)
		s.statusRightBox.AppendChild(s.statusRight)
		s.statusSegs = append(s.statusSegs,
			statusSegView{key: statusRightKey, el: s.statusRight})
		// 糖占贴边位：最右右钉段让出，保留右内边距做两者间隙。
		if s.lastRightSeg != nil {
			removeClass(s.lastRightSeg, "is-last")
		}
	} else {
		s.statusRight.SetText(text)
	}
	s.syncHasRight()
}

// dropStatusSeg 从已注册段句柄里摘掉指定 Key（糖消息收起时用）。
func (s *Workbench) dropStatusSeg(key string) {
	for i := range s.statusSegs {
		if s.statusSegs[i].key == key {
			s.statusSegs = append(s.statusSegs[:i], s.statusSegs[i+1:]...)
			return
		}
	}
}

// syncHasRight 依据「糖消息非空 或 已生成右钉段数 > 0」刷新 has-right 类：
// 它同时控制右区容器显示和左轨道的预留垫宽。
func (s *Workbench) syncHasRight() {
	if s.statusRightText != "" || s.rightVisible > 0 {
		addClass(s.status, "has-right")
		return
	}
	removeClass(s.status, "has-right")
}
