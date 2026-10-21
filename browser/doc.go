// Package browser 是面向应用层的装配门面：把渲染后端（当前为
// backend/sdl2）的选择与直接依赖挡在应用之外，应用只需 import browser
// 与 engine，不必接触底层类型。
//
// # 分工
//
//   - engine：渲染引擎，纯 Go，不认识窗口与剪贴板；
//   - backend/sdl2：平台实现层（Windows），提供窗口、事件循环、剪贴板；
//   - browser：门面，装配 New → OpenDocument → Run → Close 并转发调用。
//
// 平台差异收在 backend/sdl2：非 Windows 下 NewWindow 返回 ErrUnsupported，
// 本层原样透传，应用可据此提示「当前平台不支持」。
//
// API 按「可多次 New」塑形（每次返回独立窗口）；多窗口共存的底层基础
// （SDL 子系统引用计数、windowID 事件路由、face 缓存归属）待真实需求出现
// 时在 backend 内解决，届时本层对外签名不变。
//
// # 线程约定
//
// Window 的生命周期（New → OpenDocument → Run → Close）必须在同一个
// goroutine：SDL 要求窗口的创建、事件循环与销毁在同一 OS 线程。
//
// # 常用片段
//
//	win, err := browser.New(900, 620, "记事本")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer win.Close()
//	doc, err := win.OpenDocument(string(page))
//	if err != nil {
//		log.Fatal(err)
//	}
//	// ...绑定事件、注册 doc.SetOnChanged...
//	err = win.Run()
//
// 窗口标题、关窗、剪贴板等平台能力也由本层转发（SetTitle / RequestClose
// / ClipboardText / SetClipboardText）——引擎只管文本，剪贴板读写留在
// 平台侧（engine 保持纯 Go、可在无显示环境测试）。
//
// # 原生对话框
//
// Window.OpenFileDialog / OpenMultipleFilesDialog / OpenDirectoryDialog /
// SaveFileDialog 弹 Windows 原生文件对话框（Shell 的 IFileDialog，
// Vista 起的现代外观，支持文件类型过滤与多选）：
//
//	paths, err := win.OpenFileDialog(browser.DialogOptions{
//		Title:      "打开",
//		DefaultDir: "C:\\Users\\me\\Documents",
//		Filters: []browser.FileFilter{
//			{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
//			{Name: "所有文件 (*.*)", Pattern: "*.*"},
//		},
//	})
//	if err == nil && len(paths) > 0 {
//		// 用户选中了文件（取消时 paths 为空、err 为 nil）
//	}
//
// Window.MessageDialog 弹原生消息框（信息/警告/错误/询问，OK、是-否等
// 按钮组合），返回用户点击的按钮：
//
//	res, err := win.MessageDialog(browser.MessageOptions{
//		Title:   "确认",
//		Message: "文件已存在，要覆盖吗？",
//		Icon:    browser.IconQuestion,
//		Buttons: browser.ButtonsYesNo,
//	})
//
// 对话框是模态的：调用期间阻塞当前线程（父窗口的 SDL 事件循环随之
// 暂停），因此在按钮/菜单的回调里直接调用即可。非 Windows 平台返回
// ErrUnsupported。
//
// 无头调试用 engine.NewHeadlessViewport + engine.RenderNode +
// doc.DumpSVG：不需要显示器即可断言布局几何。
package browser
