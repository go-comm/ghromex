// 本文件是记事本的动作实现（撤销/重做、查找替换、字体与状态栏开关）；
// 包文档见 main.go。

package main

import (
	"fmt"
	"strings"

	"github.com/go-comm/ghromex/engine"
)

// redoEdit 重做上一步被撤销的操作。
//
// 引擎没有撤销栈：示例程序在文档变更回调里比对文本，变了就把旧值压栈
// （一次改动一条快照），撤销即弹栈并写回。重做对称——把当前值压回撤销
// 栈、弹出重做栈的值写回。撤销/重做本身不再压栈（applying 标记）。
func (n *notepad) redoEdit() {
	if len(n.redo) == 0 {
		n.message("没有可重做的操作")
		return
	}
	next := n.redo[len(n.redo)-1]
	n.redo = n.redo[:len(n.redo)-1]
	n.undo = append(n.undo, n.cur)
	if len(n.undo) > maxUndo {
		n.undo = n.undo[1:]
	}
	n.applying = true
	n.editor.SetValue(next)
	n.applying = false
	if n.shell != nil {
		n.shell.Refresh()
	}
	n.message("已重做")
}

// deleteSelection 删除选区或光标处的字符（与 Chrome 的 Delete 一致）。
func (n *notepad) deleteSelection() {
	n.focusEditor()
	if !engine.OnDocumentKeyDown(n.doc, engine.KeyDelete) {
		n.message("没有可删除的内容")
	}
}

// showFindBar 展开查找替换栏（编辑区相应下移一行）。
func (n *notepad) showFindBar() {
	if n.findBar == nil {
		return
	}
	n.findOpen = true
	n.lastW = 0 // 强制重排：查找栏显示、编辑区位置变了
	n.syncLayout()
	if n.findIn != nil {
		engine.SetFocusedElement(n.doc, n.findIn.Element())
	}
}

// hideFindBar 收起查找替换栏。
func (n *notepad) hideFindBar() {
	if n.findBar == nil {
		return
	}
	n.findOpen = false
	n.lastW = 0
	n.syncLayout()
	n.focusEditor()
}

// findNext 查找下一处匹配。
//
// 引擎没有「设置选区」的接口，应用层无法把匹配处选中（浏览器里的高亮
// 做不到），只能报出它的位置——状态栏给出行列，够用了。
func (n *notepad) findNext() {
	needle := strings.TrimSpace(n.findIn.Value())
	if needle == "" {
		n.message("请输入查找内容")
		return
	}
	text := n.cur
	from := n.caretOffset()
	idx := strings.Index(text[from:], needle)
	if idx < 0 {
		// 从头再找一遍（循环查找）。
		idx = strings.Index(text, needle)
		if idx < 0 {
			n.matchAt = -1
			n.message("未找到「" + needle + "」")
			return
		}
	} else {
		idx += from
	}
	n.matchAt = idx
	line, col := offsetToLineCol(text, idx)
	n.message(fmt.Sprintf("找到「%s」：第 %d 处，行 %d，列 %d", needle, n.matchCount(text, needle, idx), line, col))
}

// matchCount 数出匹配在文本里是第几处（从开头数到 pos 为止）。
func (n *notepad) matchCount(text, needle string, pos int) int {
	return strings.Count(text[:pos], needle) + 1
}

// replaceCurrent 把当前找到的一处替换掉（先「查找下一个」再「替换」）。
func (n *notepad) replaceCurrent() {
	needle := strings.TrimSpace(n.findIn.Value())
	if needle == "" {
		n.message("请输入查找内容")
		return
	}
	if n.matchAt < 0 || !strings.HasPrefix(n.cur[n.matchAt:], needle) {
		n.findNext()
		if n.matchAt < 0 {
			return
		}
	}
	repl := n.replaceIn.Value()
	text := n.cur
	updated := text[:n.matchAt] + repl + text[n.matchAt+len(needle):]
	n.applyText(updated)
	n.matchAt = -1
	n.message("已替换 1 处")
}

// replaceAll 替换全部匹配。
func (n *notepad) replaceAll() {
	needle := strings.TrimSpace(n.findIn.Value())
	if needle == "" {
		n.message("请输入查找内容")
		return
	}
	count := strings.Count(n.cur, needle)
	if count == 0 {
		n.message("未找到「" + needle + "」")
		return
	}
	repl := n.replaceIn.Value()
	n.applyText(strings.ReplaceAll(n.cur, needle, repl))
	n.message(fmt.Sprintf("已替换 %d 处", count))
}

// applyText 用新文本替换编辑区内容（进撤销栈，与普通编辑同一条路径）。
func (n *notepad) applyText(text string) {
	n.applying = true
	n.editor.SetValue(text)
	n.applying = false
	n.cur = text
	n.dirty = true
}

// caretOffset 返回光标处的字符下标（按 rune 计）。
//
// 引擎只给行列号（1 起），应用层自己换算成下标——查找从这里往后开始。
func (n *notepad) caretOffset() int {
	if n.editor == nil {
		return 0
	}
	line, col, ok := engine.CaretPosition(n.editorEl())
	if !ok {
		return 0
	}
	return offsetOfLineCol(n.cur, line, col)
}

// offsetOfLineCol 把（行，列）换算成文本里的 rune 下标。
func offsetOfLineCol(text string, line, col int) int {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	offset := 0
	cur := 1
	for cur < line {
		i := strings.IndexByte(text[offset:], '\n')
		if i < 0 {
			return len([]rune(text))
		}
		offset += i + 1
		cur++
	}
	rest := []rune(text[offset:])
	if col-1 > len(rest) {
		col = len(rest) + 1
	}
	return offset + len(rest[:col-1])
}

// offsetToLineCol 把 rune 下标换算成（行，列），供状态栏显示。
func offsetToLineCol(text string, offset int) (int, int) {
	runes := []rune(text)
	if offset > len(runes) {
		offset = len(runes)
	}
	line, col := 1, 1
	for i := 0; i < offset; i++ {
		if runes[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
}

// setMono 切换编辑区字体：等宽（代码）/ 无衬线（正文）。
func (n *notepad) setMono(mono bool) {
	n.mono = mono
	n.lastW = 0 // 强制重排：字体变了要重写内联样式
	n.syncLayout()
	if mono {
		n.message("已切换到等宽字体")
		return
	}
	n.message("已切换到无衬线字体")
}

// toggleStatus 显示/隐藏状态栏。
func (n *notepad) toggleStatus() {
	n.showStatus = !n.showStatus
	if n.shell != nil {
		n.shell.SetStatusVisible(n.showStatus)
	}
	n.lastW = 0
	n.syncLayout()
	if n.showStatus {
		n.message("已显示状态栏")
		return
	}
	n.message("已隐藏状态栏")
}
