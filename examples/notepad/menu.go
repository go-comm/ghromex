// 本文件是记事本的菜单：菜单表与菜单栏/浮层的实现；包文档见 main.go。
//
// 引擎没有通用弹层，菜单用「菜单栏标签 + 浮层面板」实现：浮层绝对定位、
// 盖在内容之上（与 <select> 的选项浮层同理）——点标签打开、点菜单项执行
// 动作、点别处关闭。

package main

import (
	"fmt"

	"github.com/go-comm/ghromex/engine"
)

// menuSlot 是菜单栏里每个菜单标签占的宽度。
//
// 固定槽位而不是按文字排布：引擎不提供文本测量接口，应用层拿不到标签的
// 实际宽度，菜单浮层的左缘只能按槽位算。
const menuSlot = 62

// menuItem 是一个菜单项；sep 为真时是分隔线（没有动作）。
type menuItem struct {
	id    string // 动作 id（分发给 action）
	label string // 显示文字
	sep   bool   // 是否是分隔线
}

// menu 是一个下拉菜单。
type menu struct {
	title string
	items []menuItem
}

// menus 是记事本的菜单表。新增功能在这里加一项即可——菜单栏标签与浮层
// 面板都由它生成。
var menus = []menu{
	{title: "文件", items: []menuItem{
		{id: "new", label: "新建"},
		{id: "open", label: "打开…"},
		{id: "opendir", label: "打开文件夹…"},
		{sep: true},
		{id: "save", label: "保存"},
		{id: "saveas", label: "另存为…"},
		{sep: true},
		{id: "exit", label: "退出"},
	}},
	{title: "编辑", items: []menuItem{
		{id: "undo", label: "撤销"},
		{id: "redo", label: "重做"},
		{sep: true},
		{id: "cut", label: "剪切"},
		{id: "copy", label: "复制"},
		{id: "paste", label: "粘贴"},
		{id: "delete", label: "删除"},
		{sep: true},
		{id: "selectall", label: "全选"},
		{id: "find", label: "查找/替换…"},
		{sep: true},
		{id: "time", label: "时间/日期"},
	}},
	{title: "格式", items: []menuItem{
		{id: "zoomin", label: "放大字号"},
		{id: "zoomout", label: "缩小字号"},
		{id: "zoomreset", label: "默认字号"},
		{sep: true},
		{id: "fontmono", label: "等宽字体"},
		{id: "fontsans", label: "无衬线字体"},
	}},
	{title: "视图", items: []menuItem{
		{id: "statusbar", label: "显示状态栏"},
	}},
	{title: "帮助", items: []menuItem{
		{id: "about", label: "关于记事本"},
	}},
}

// buildMenuBar 在菜单栏里生成菜单标签。
func (n *notepad) buildMenuBar() {
	if n.menuBar == nil {
		return
	}
	for i, m := range menus {
		label := n.doc.CreateElement("div")
		label.SetAttribute("class", "menu")
		label.SetAttribute("style", fmt.Sprintf("left:%dpx", i*menuSlot))
		label.SetText(m.title)
		idx := i
		label.OnClick(func(*engine.MouseEvent) { n.openMenu(idx) })
		n.menuLabels = append(n.menuLabels, label)
		n.menuBar.AppendChild(label)
	}
}

// openMenu 打开第 i 个菜单：在菜单栏下方弹出浮层面板。
func (n *notepad) openMenu(i int) {
	if n.menuPop == nil || i < 0 || i >= len(menus) {
		return
	}
	m := menus[i]

	// 菜单项每次重建：菜单表变了不用改这里，重开一次就是新的。
	old := append([]engine.HTMLElement(nil), n.menuPop.Children()...)
	for _, c := range old {
		n.menuPop.RemoveChild(c)
	}
	for _, it := range m.items {
		if it.sep {
			sep := n.doc.CreateElement("div")
			sep.SetAttribute("class", "menu-sep")
			n.menuPop.AppendChild(sep)
			continue
		}
		item := n.doc.CreateElement("div")
		item.SetAttribute("class", "menu-item")
		item.SetText(it.label)
		act := it.id
		item.OnClick(func(*engine.MouseEvent) {
			n.closeMenu()
			n.action(act)
		})
		n.menuPop.AppendChild(item)
	}
	n.menuPop.SetAttribute("style", fmt.Sprintf("display:block;left:%dpx;top:%dpx", i*menuSlot, menuH))
	n.menuOpen = true
	n.highlightMenu(i)
}

// closeMenu 关闭菜单浮层。
func (n *notepad) closeMenu() {
	if n.menuPop == nil || !n.menuOpen {
		return
	}
	n.menuPop.SetAttribute("style", "display:none")
	n.menuOpen = false
	n.highlightMenu(-1)
}

// highlightMenu 高亮当前打开的菜单标签（-1 表示不高亮）。
func (n *notepad) highlightMenu(i int) {
	for idx, label := range n.menuLabels {
		if idx == i {
			label.SetAttribute("class", "menu menu-open")
			continue
		}
		label.SetAttribute("class", "menu")
	}
}
