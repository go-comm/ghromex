// Package browser 是面向应用层的装配门面：把渲染后端（当前为
// backend/sdl2）的选择与直接依赖挡在应用之外，应用只需 import browser
// 与 engine，不必接触底层类型。
//
// # 分工
//
//   - core：引擎与后端之间的契约（Graphics / Viewport / Paint / Color…），
//     叶子包，engine 与 backend 都只依赖它；
//   - engine：渲染引擎，纯 Go，不认识窗口与剪贴板；
//   - backend/sdl2：平台实现层（Windows），提供窗口、事件解码、绘制、
//     剪贴板——**不认识引擎**，只按 core 的契约干活；
//   - browser：应用层，持有文档并跑主循环（平台事件 → 引擎动作 →
//     重排 → 重绘），把引擎与后端缝在一起。
//
// 主循环放在这一层而不是后端：后端只该做平台的事。事件怎么解释成引擎
// 动作（点击、选区、滚轮）、什么时候重排重绘，都是应用层的事；后端若
// 直接依赖引擎，平台层就反过来依赖了它上面的层（core/arch_test.go 把
// 这条规则钉死）。
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
// 需要外部样式表（页面里的 <link rel="stylesheet"> 与 CSS 的 @import）时
// 改用 OpenDocumentWith 并传入加载器——引用于解析期就加载，事后再装加载器
// 补不上：
//
//	doc, err := win.OpenDocumentWith(string(page), engine.DocumentOptions{
//		BaseURL:        "file:///C:/pages/index.html",
//		LoadStylesheet: loadFromDisk, // func(href string) (string, error)
//	})
//
// 窗口标题、关窗等平台能力也由本层转发（SetTitle / RequestClose）——
// 引擎只管文本，剪贴板读写留在平台侧（runtime/clipboard，engine 保持
// 纯 Go、可在无显示环境测试）。
//
// # 原生对话框
//
// 原生对话框不在本包：调用入口是 runtime/dialog（通用 API），平台实现
// 在 backend/dialog。本包只提供 NativeHandle——原生对话框要挂在应用
// 窗口下面，需要平台的原生窗口句柄：
//
//	hwnd, err := win.NativeHandle()
//	paths, err := dialog.OpenFileDialog(dialog.Parent(hwnd), dialog.Options{
//		Title: "打开",
//		Filters: []dialog.FileFilter{
//			{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
//		},
//	})
//
// 对话框是模态的：调用期间阻塞当前线程（父窗口的 SDL 事件循环随之
// 暂停），因此在按钮/菜单的回调里直接调用即可。非 Windows 平台返回
// dialog.ErrUnsupported。
//
// # 出图三件套（别混）
//
// 看渲染结果有三条路，产物不同、用途不同，别当同一件事：
//
//   - doc.DumpSVG()（-dump-svg）：走引擎级联+布局，把结果序列化成 SVG。
//     文字是真实 <text>、可在浏览器里看真字体；但它不经过光栅绘制器，
//     反映的是「布局与样式」，不是最终像素（圆角精灵、抗锯齿这类绘制细节
//     看不到）。适合核对几何、层级、配色。
//   - engine.NewBufferedViewport + BufferGraphics.SavePNG（-dump-png）：
//     无头软件光栅化到 RGBA 缓冲。文字是占位方块（不依赖字体后端），
//     可 ColorAt 逐像素断言——CI/headless 的首选，也是「只看布局与配色」
//     的快图。
//   - Window.SavePNG(path)：开真实窗口（SDL 渲染器 + 真实字体）重排、
//     画一帧、回读像素存 PNG。字体的字号/行高/字距、抗锯齿、圆角精灵
//     都是最终观感——核对「用户实际看到的样子」走这条；配合
//     SDL_VIDEO_DRIVER=dummy 无显示器也能跑。
//
// 一句话：SVG 看布局，BufferedPNG 看像素断言，Window.SavePNG 看真实观感。
package browser
