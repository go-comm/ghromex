package engine

// <a> 链接的点击默认动作：定位带 href 的链接祖先并回调文档导航钩子。
// 引擎是单文档视图，具体跳转（换一份 HTML、打开系统浏览器）由应用决定。

// anchorOf 返回元素自身或祖先中第一个带非空 href 的 <a>，无则 nil。
func anchorOf(e HTMLElement) HTMLElement {
	for cur := e; cur != nil; cur = cur.ParentElement() {
		if tagIs(cur, "a") && cur.GetAttribute("href") != "" {
			return cur
		}
	}
	return nil
}

// navigate 执行链接的点击默认动作（在 click 冒泡之后调用，
// 保证处理器仍能读到跳转前的文档状态）。
func navigate(doc HTMLDocument, target HTMLElement) {
	a := anchorOf(target)
	if a == nil || doc == nil {
		return
	}
	d, ok := doc.(*htmlDocument)
	if !ok || d.onNavigate == nil {
		return
	}
	d.onNavigate(a.GetAttribute("href"))
}
