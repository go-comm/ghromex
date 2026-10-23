// 本文件是菜单栏与下拉浮层；包文档见 doc.go。
package ui

import "github.com/go-comm/ghromex/engine"

// menuView 是一个顶级菜单的 DOM 句柄。
type menuView struct {
	menu  Menu
	label engine.HTMLElement // 菜单栏上的标签（position:relative）
	pop   engine.HTMLElement // 浮层（标签的子元素，top = 菜单栏高度）
}

// buildMenubar 生成菜单栏。
func (s *Shell) buildMenubar() {
	s.menubar = newEl(s.doc, "div", "system-ui-menubar", "")
	s.menubar.AppendChild(newEl(s.doc, "div", "system-ui-hairline", ""))
	for i := range s.spec.Menus {
		s.addMenu(i)
	}
	s.doc.Body().AppendChild(s.menubar)
}

// addMenu 追加一个菜单标签及其浮层。
//
// 浮层放进标签内部（而不是 body 下）是关键：标签 position:relative +
// 浮层 top:菜单栏高度，浮层天然贴在标签左缘，既不用测量文字宽度，也
// 不用在滚动/缩放后重算坐标。
func (s *Shell) addMenu(i int) {
	mv := &menuView{menu: s.spec.Menus[i]}
	mv.label = newEl(s.doc, "div", "system-ui-menu", mv.menu.Title)
	mv.pop = newEl(s.doc, "div", "system-ui-menu-pop", "")
	mv.label.AppendChild(mv.pop)
	// 打开态的下划线：2px 强调色，横跨整个标签。用实体元素而不是
	// ::after——引擎没实现伪元素。
	mv.label.AppendChild(newEl(s.doc, "div", "system-ui-menu-underline", ""))

	label, idx := mv.label, i
	label.OnClick(func(ev *engine.MouseEvent) {
		// 不能靠 ev.Element 区分「点标签」和「点浮层里的项」：dispatch
		// 每冒泡到一层都会把 ev.Element 改写成当前元素，点到浮层项时
		// 传到标签这里 Element 又变回标签。只能看坐标——浮层在标签盒
		// 之外（top = 菜单栏高度），落在标签盒内才是点标签。
		if !hitRect(label, ev.X, ev.Y) {
			return
		}
		s.toggle(idx)
	})

	s.menus = append(s.menus, mv)
	s.menubar.AppendChild(label)
}

// OpenMenu 打开第 i 个菜单（越界忽略）。
//
// 每次打开都按当前状态重建浮层：菜单项数量少，重建比「先建好再逐个
// 更新禁用态」简单，也保证可用性是打开那一刻的最新值。
func (s *Shell) OpenMenu(i int) {
	if s == nil || i < 0 || i >= len(s.menus) {
		return
	}
	s.CloseMenus()
	mv := s.menus[i]
	if cs := mv.pop.Children(); len(cs) > 0 {
		mv.pop.RemoveChild(cs[0], cs[1:]...)
	}
	// 只要有任意一项带图标，整张菜单就预留图标列：否则各项文字会在
	// 有无图标之间左右跳，看起来像没对齐。
	withIcon := false
	for _, a := range mv.menu.Items {
		if !a.Sep && iconGlyph(a.Icon) != "" {
			withIcon = true
			break
		}
	}
	if withIcon {
		setClass(mv.pop, "system-ui-menu-pop has-icon")
	} else {
		setClass(mv.pop, "system-ui-menu-pop")
	}
	for _, a := range mv.menu.Items {
		mv.pop.AppendChild(s.menuItemEl(a, withIcon))
	}
	setClass(mv.label, "system-ui-menu is-open")
	s.open = mv
}

// CloseMenus 收起所有菜单。
func (s *Shell) CloseMenus() {
	if s == nil {
		return
	}
	for _, mv := range s.menus {
		setClass(mv.label, "system-ui-menu")
	}
	s.open = nil
}

// toggle 在「已开 → 关」「未开 → 开」之间切换。
func (s *Shell) toggle(i int) {
	if s.open != nil && s.open == s.menus[i] {
		s.CloseMenus()
		return
	}
	s.OpenMenu(i)
}

// menuItemEl 生成一条菜单项（或分隔线）；withIcon 决定是否预留图标列。
func (s *Shell) menuItemEl(a Action, withIcon bool) engine.HTMLElement {
	if a.Sep {
		return newEl(s.doc, "div", "system-ui-menu-sep", "")
	}
	el := newEl(s.doc, "div", "system-ui-menu-item", "")
	if withIcon {
		if ic := newIcon(s.doc, a.Icon); ic != nil {
			setClass(ic, "system-ui-icon system-ui-menu-icon")
			el.AppendChild(ic)
		}
	}
	el.AppendChild(newEl(s.doc, "span", "", a.Label))
	if a.Shortcut != "" {
		el.AppendChild(newEl(s.doc, "span", "system-ui-menu-shortcut", a.Shortcut))
	}
	// 可用性在生成时求值（即打开菜单那一刻），点的时候再判一次，
	// 防止浮层还没重建、状态已经变了。
	ok := enabled(a)
	if !ok {
		setClass(el, "system-ui-menu-item is-disabled")
	}
	act := a
	el.OnClick(func(*engine.MouseEvent) {
		if !enabled(act) {
			return
		}
		s.invoke(act)
	})
	return el
}

// insideMenu 判断一个点是否落在菜单栏或当前展开的浮层里。
//
// 用坐标而不是 ev.Element：click 冒泡时引擎会把 ev.Element 逐层改写成
// 当前元素，冒到 body 时它已经是 body 本身，「点到了谁」的信息丢了。
func (s *Shell) insideMenu(x, y int) bool {
	if hitRect(s.menubar, x, y) {
		return true
	}
	if s.open != nil && hitRect(s.open.pop, x, y) {
		return true
	}
	return false
}

// hitRect 判断点是否在元素的边框盒内（零尺寸盒恒假）。
func hitRect(e engine.HTMLElement, x, y int) bool {
	if e == nil {
		return false
	}
	r := e.GetBoundingClientRect()
	return r.Right().Pixel() > r.Left().Pixel() &&
		r.Bottom().Pixel() > r.Top().Pixel() &&
		x >= r.Left().Pixel() && x < r.Right().Pixel() &&
		y >= r.Top().Pixel() && y < r.Bottom().Pixel()
}
