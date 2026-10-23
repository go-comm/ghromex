// 本文件是把组件库接到窗口上的便利层（App）；包文档见 doc.go。
package ui

import (
	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

// App 是「窗口 + 工作台」的便利封装：建窗、装菜单/工具栏/状态栏、换页
// 重装、快捷键路由一手包办。
//
// 只想要文档层（无头导出、嵌入已有窗口）时用 Install，不必用 App：
// App 只是 Install 的一个固定用法。
type App struct {
	win       *browser.Window
	spec      Spec
	workbench *Workbench
}

// New 按 Spec 建窗口并注册文档/按键钩子。
//
// 此时还没有文档：菜单栏等要等 LoadHTML 之后、文档存在时装上去
// （SetOnDocument 回调负责每次换文档后重装）。
func New(spec Spec) (*App, error) {
	w, h := spec.Width, spec.Height
	if w <= 0 {
		w = 900
	}
	if h <= 0 {
		h = 620
	}
	win, err := browser.New(w, h, spec.Title)
	if err != nil {
		return nil, err
	}
	a := &App{win: win, spec: spec}
	win.SetOnDocument(func(doc engine.HTMLDocument) {
		a.workbench = Install(doc, a.spec)
		if a.workbench != nil {
			// 命令返回 ErrQuit 时走到这里：投递退出事件，Run 正常
			// 返回，资源仍由应用侧的 Close 释放（不在事件循环里
			// 直接销毁窗口）。
			a.workbench.onQuit = win.RequestClose
		}
	})
	win.SetKeyHandler(func(key int, mods uint16) bool {
		if a.workbench == nil {
			return false
		}
		return a.workbench.HandleKey(key, mods)
	})
	return a, nil
}

// LoadHTML 解析页面源码并装好工作台，返回文档。
func (a *App) LoadHTML(src string) (engine.HTMLDocument, error) {
	return a.win.OpenDocument(src)
}

// LoadHTMLWith 同 LoadHTML，可传文档选项（外部样式表加载器、地址基准）。
func (a *App) LoadHTMLWith(src string, opts engine.DocumentOptions) (engine.HTMLDocument, error) {
	return a.win.OpenDocumentWith(src, opts)
}

// Workbench 是装在当前文档上的工作台；还没加载文档时为 nil。
func (a *App) Workbench() *Workbench { return a.workbench }

// Window 暴露底层窗口（原生对话框父句柄、标题、截图等）。
func (a *App) Window() *browser.Window { return a.win }

// SetStatus 更新状态栏左侧文本（工作台还没建好时忽略，便于在命令闭包
// 里无脑调用）。
func (a *App) SetStatus(text string) {
	if a.workbench != nil {
		a.workbench.SetStatus(text)
	}
}

// SetTheme 切换当前窗口的主题。
//
// 已经装好的工作台会追加一套新主题样式，页面内容与滚动位置保持不变；之后
// 如果换文档，新文档也沿用这套主题。
func (a *App) SetTheme(theme Theme) {
	if a == nil {
		return
	}
	a.spec.Theme = &theme
	if a.workbench != nil {
		a.workbench.SetTheme(theme)
	}
}

// Run 进入事件循环，直到窗口关闭。
func (a *App) Run() error { return a.win.Run() }

// RequestClose 请求退出：投递关闭事件，Run 会正常返回。
func (a *App) RequestClose() { a.win.RequestClose() }

// Close 释放窗口资源。
func (a *App) Close() { a.win.Close() }
