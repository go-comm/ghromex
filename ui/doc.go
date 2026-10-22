// Package ui 是应用外壳与页面组件库。
//
// 它提供三样东西：
//
//   - 外壳：菜单栏（含下拉浮层、快捷键、禁用态）、工具栏、状态栏；
//   - 页面组件：卡片、标题、副标题、徽标、列表、操作行、输入框、按钮；
//   - 主题：一套视觉令牌 + 生成的样式表，应用一行 CSS 都不用写。
//
// # 最小用法
//
//	spec := ui.Spec{
//		Title: "记事本", Width: 900, Height: 620, Status: "就绪",
//		Menus: []ui.Menu{{Title: "文件", Items: []ui.Action{
//			ui.Item("保存", "Ctrl+S", save).WithIcon("save").When(editor.Dirty),
//			ui.Sep(),
//			ui.Item("退出", "Alt+Q", ui.Quit),
//		}}},
//		Tools: []ui.Action{
//			ui.Button("保存", save).WithIcon("save").When(editor.Dirty),
//			ui.Sep(),
//			ui.Button("复制", copy).WithIcon("copy"),
//		},
//	}
//	app, err := ui.New(spec)
//	if err != nil {
//		return err
//	}
//	defer app.Close()
//	if _, err := app.LoadHTML(page); err != nil {
//		return err
//	}
//	return app.Run()
//
// 页面内容只写类名（<div class="ui-page">、<div class="ui-card">…），
// 样式全部来自 Install 注入的主题样式表。
//
// # 图标
//
// 命令可以用 WithIcon 挂图标：
//
//	ui.Button("保存", save).WithIcon("save")
//	ui.Item("新建", "Ctrl+N", newFile).WithIcon("new")
//
// 内置图标名见 iconGlyphs（new/open/save/copy/cut/paste/undo/redo/search/
// replace/settings/info/delete/refresh/print/sort/download/upload/home/back/
// forward/help/play/pause/document），也可以直接给一个字符。字形来自
// Windows 10/11 自带的 Segoe MDL2 Assets（segmdl2.ttf），按 16px 网格设计、颜色跟随文字，
// 悬停/禁用/换主题都不需要额外资源。工具栏按钮有图标时画成方形图标按钮，
// 没有图标时退回文字按钮；菜单里只要有一项带图标，整张菜单就会预留图标列。
//
// 图标字体是系统字体：跨平台部署时要么在 Theme.IconFont 里给一个同编码
// 的图标字体，要么只写文字（不调 WithIcon）。
//
// # 为什么是「文档内自绘」而不是原生控件
//
// 引擎只渲染文档，没有独立的窗口 chrome 层。组件库的做法是把外壳作为
// 文档的一部分：position:fixed 压在页面上、body 首尾插占位块让出空间、
// 换文档时重装。好处是纯 Go、跨平台、可测试（无头就能导出 PNG/SVG 看
// 效果）；代价是页面自己 z-index 更高的元素会盖住外壳，且外壳不参与
// 原生窗口菜单的键盘语义。
//
// # 三个必须知道的约定
//
//   - 外壳是文档的一部分：换文档后要重新 Install（App 已通过
//     browser.Window.SetOnDocument 自动完成）。
//   - 应用快捷键先于页面：App 用 browser.Window.SetKeyHandler 注册，
//     输入框聚焦时 Ctrl+S 也先到应用。
//   - 主题只使用引擎支持的 CSS 子集：没有 flex/grid/阴影/过渡，横向排列
//     靠 inline-block，分隔线是 1px 的实体元素。
package ui
