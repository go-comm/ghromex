// 本文件是记事本的命令表：菜单、工具栏与快捷键都由 ui.Spec 生成。
//
// 命令只定义一次，菜单项和工具栏按钮共用同一份 Action；快捷键也由
// ui.Shell 统一绑定，不再由页面自己处理。
//
// 每条命令都配了图标：菜单项在浮层里显示「图标 + 文字」，工具栏则退化成
// 纯图标按钮（ui 的约定：Action 有图标时工具栏只画图标，Label 留作兜底）。
// 图标名见 ui/icon.go 的 iconGlyphs。
package main

import (
	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/ui"
)

// buildSpec 生成记事本的外壳配置。
func (n *notepad) buildSpec() ui.Spec {
	cmd := func(label, shortcut string, f func()) ui.Action {
		return ui.Item(label, shortcut, func() error {
			f()
			return nil
		})
	}

	newAct := cmd("新建", "Ctrl+N", n.newFile).WithIcon("new")
	openAct := cmd("打开…", "Ctrl+O", n.openFile).WithIcon("open")
	openDirAct := cmd("打开文件夹…", "", n.openDir).WithIcon("folder-open")
	saveAct := cmd("保存", "Ctrl+S", n.saveFile).WithIcon("save")
	saveAsAct := cmd("另存为…", "Ctrl+Shift+S", n.saveAs).WithIcon("save-as")
	exitAct := ui.Item("退出", "Alt+Q", func() error {
		if n.win == nil {
			n.message("无头模式没有窗口，无法退出")
			return nil
		}
		n.win.RequestClose()
		return nil
	}).WithIcon("close")

	undoAct := cmd("撤销", "Ctrl+Z", n.undoEdit).
		WithIcon("undo").
		When(func() bool { return len(n.undo) > 0 })
	redoAct := cmd("重做", "Ctrl+Y", n.redoEdit).
		WithIcon("redo").
		When(func() bool { return len(n.redo) > 0 })
	cutAct := cmd("剪切", "Ctrl+X", n.cut).WithIcon("cut")
	copyAct := cmd("复制", "Ctrl+C", n.copySelection).WithIcon("copy")
	pasteAct := cmd("粘贴", "Ctrl+V", n.paste).WithIcon("paste")
	deleteAct := cmd("删除", "", n.deleteSelection).WithIcon("delete")
	selectAllAct := cmd("全选", "Ctrl+A", n.selectAll).WithIcon("select-all")
	findAct := cmd("查找/替换…", "Ctrl+F", n.showFindBar).WithIcon("search")
	timeAct := cmd("时间/日期", "", n.insertTime).WithIcon("history")

	zoomInAct := cmd("放大字号", "", func() { n.zoom(1) }).WithIcon("font-increase")
	zoomOutAct := cmd("缩小字号", "", func() { n.zoom(-1) }).WithIcon("font-decrease")
	zoomResetAct := cmd("默认字号", "", func() { n.zoom(0) }).WithIcon("font-size")
	monoAct := cmd("等宽字体", "", func() { n.setMono(true) }).WithIcon("code")
	sansAct := cmd("无衬线字体", "", func() { n.setMono(false) }).WithIcon("align-left")

	statusAct := cmd("显示状态栏", "", n.toggleStatus).WithIcon("dock-bottom")
	aboutAct := cmd("关于记事本", "", func() {
		n.message(appName + " — 基于 Ghromex 引擎的示例（纯 Go，无 CGO）")
	}).WithIcon("info")

	return ui.Spec{
		Theme: &n.theme,
		StatusSegments: []ui.StatusSegment{
			{Key: "file", Text: untitled, Width: 220},
			{Key: "pos", Text: "行 1，列 1", Width: 140},
			{Key: "chars", Text: "0 个字符", Width: 110},
		},
		Menus: []ui.Menu{
			{Title: "文件", Items: []ui.Action{
				newAct, openAct, openDirAct, ui.Sep(), saveAct, saveAsAct, ui.Sep(), exitAct,
			}},
			{Title: "编辑", Items: []ui.Action{
				undoAct, redoAct, ui.Sep(), cutAct, copyAct, pasteAct, deleteAct,
				ui.Sep(), selectAllAct, findAct, ui.Sep(), timeAct,
			}},
			{Title: "格式", Items: []ui.Action{
				zoomInAct, zoomOutAct, zoomResetAct, ui.Sep(), monoAct, sansAct,
			}},
			{Title: "视图", Items: []ui.Action{statusAct}},
			{Title: "帮助", Items: []ui.Action{aboutAct}},
		},
		Tools: []ui.Action{
			newAct, openAct, saveAct, saveAsAct, ui.Sep(), undoAct, redoAct,
		},
		Focus: func() engine.HTMLElement { return n.editorEl() },
	}
}
