// 示例：组件库 ui。
//
// 菜单栏、工具栏、状态栏、下拉菜单与页面组件全部由 ui 包生成；本文件
// 只写命令表与接线——应用侧「少量代码」的样子就是这里。
//
//	go run ./examples/kit
//	go run ./examples/kit -dark                # 深色主题
//	go run ./examples/kit -dump-png kit.png    # 无头出图：软件光栅，文字是占位方块
//	go run ./examples/kit -dump-buffer-png kit.png # 无头出图：裸页面快照（不装工作台）
//	go run ./examples/kit -dump-svg kit.svg    # 无头出图：SVG，核对布局/层级
//	go run ./examples/kit -shot kit.png        # 真实窗口截图：真实字体观感
//	go run ./examples/kit -dark -shot kit.png  # 深色主题 + 真实窗口截图
//	go run ./examples/kit -shot kit.png -shot-menu 编辑
//	go run ./examples/kit -exit-after 2s       # 调试/CI
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/ui"
)

//go:embed content.html
var page string

const (
	winW = 900
	winH = 760
)

func main() {
	dumpPNG := flag.String("dump-png", "", "无头出图：软件光栅 PNG，文字是占位方块，断言布局/配色用")
	dumpBufferPNG := flag.String("dump-buffer-png", "", "无头出图：裸页面 buffer 快照 PNG（不装工作台/接线），对照 -dump-png")
	dumpSVG := flag.String("dump-svg", "", "无头出图：级联+布局导出 SVG（真实 <text>），核对几何/层级/配色")
	shot := flag.String("shot", "", "真实窗口截图：真实字体渲染一帧存 PNG（Window.Screenshot 回读）")
	shotMenu := flag.String("shot-menu", "", "与 -shot 配合：先展开指定标题的菜单（如 文件）")
	dark := flag.Bool("dark", false, "使用深色主题")
	exitAfter := flag.Duration("exit-after", 0, "运行指定时长后自动退出（调试/CI）")
	flag.Parse()

	// 命令闭包要在 App 建好之前定义（buildSpec 只接收一个写状态栏的
	// 函数），所以这里用变量延后取值，命令执行时 app 一定已经就绪。
	var app *ui.App
	status := func(text string) {
		if app != nil {
			app.SetStatus(text)
		}
	}
	spec := buildSpec(status)
	if *dark {
		theme := ui.Dark()
		spec.Theme = &theme
	}

	if *dumpBufferPNG != "" {
		if err := dumpBuffer(*dumpBufferPNG); err != nil {
			fmt.Fprintln(os.Stderr, "无头导出失败:", err)
			os.Exit(1)
		}
		return
	}
	if *dumpPNG != "" || *dumpSVG != "" {
		if err := dump(spec, *dumpPNG, *dumpSVG); err != nil {
			fmt.Fprintln(os.Stderr, "无头导出失败:", err)
			os.Exit(1)
		}
		return
	}

	var err error
	app, err = ui.New(spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "建窗失败:", err)
		os.Exit(1)
	}
	defer app.Close()

	doc, err := app.LoadHTML(page)
	if err != nil {
		fmt.Fprintln(os.Stderr, "装载文档失败:", err)
		os.Exit(1)
	}
	addControls(doc, status)

	if *shot != "" {
		if *shotMenu != "" {
			for i, m := range spec.Menus {
				if m.Title == *shotMenu {
					app.Workbench().OpenMenu(i)
					break
				}
			}
		}
		if err := saveShot(app, *shot); err != nil {
			fmt.Fprintln(os.Stderr, "截图失败:", err)
			os.Exit(1)
		}
		return
	}

	// 页面里的按钮：组件是普通元素，事件还是引擎那一套 OnClick。
	bind := func(sel, msg string) {
		if el := doc.QuerySelector(sel); el != nil {
			el.OnClick(func(*engine.MouseEvent) { status(msg) })
		}
	}
	bind("#demo-save", "已保存（来自页面按钮）")
	bind("#demo-preview", "预览：示例没有实现")
	bind("#demo-rename", "重命名：示例没有实现")

	if *exitAfter > 0 {
		app.Window().SetAutoExit(*exitAfter)
	}
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}

// buildSpec 是示例的命令表。
//
// 抽成函数是为了让无头冒烟测试用同一份配置出图——测试与真实窗口看到
// 的是同一套菜单/工具栏，改坏了会直接测出来。
func buildSpec(status func(string)) ui.Spec {
	// cmd 把「写状态栏」变成一条命令，示例里大部分命令只需要这一句。
	cmd := func(text string) func() error {
		return func() error { status(text); return nil }
	}
	// 命令只定义一次，菜单项与工具栏按钮共用：图标、快捷键不会出现
	// 「菜单里改了、工具栏忘了改」。
	newFile := ui.Item("新建", "Ctrl+N", cmd("已新建空白文件")).WithIcon("new")
	openFile := ui.Item("打开…", "Ctrl+O", cmd("打开：示例不读取真实文件")).WithIcon("open")
	saveFile := ui.Item("保存", "Ctrl+S", cmd("已保存")).WithIcon("save")
	saveAs := ui.Item("另存为…", "Ctrl+Shift+S", cmd("另存为：示例不写盘"))
	printDoc := ui.Item("打印…", "Ctrl+P", cmd("打印：示例不打印")).WithIcon("print")
	quit := ui.Item("退出", "Alt+Q", ui.Quit)

	undo := ui.Item("撤销", "Ctrl+Z", cmd("已撤销")).WithIcon("undo")
	redo := ui.Item("重做", "Ctrl+Y", cmd("已重做")).WithIcon("redo")
	cut := ui.Item("剪切", "Ctrl+X", cmd("已剪切")).WithIcon("cut")
	copyCmd := ui.Item("复制", "Ctrl+C", cmd("已复制")).WithIcon("copy")
	paste := ui.Item("粘贴", "Ctrl+V", cmd("已粘贴")).WithIcon("paste")
	del := ui.Item("删除", "Delete", cmd("已删除")).WithIcon("delete")
	find := ui.Item("查找…", "Ctrl+F", cmd("查找：示例没有实现")).WithIcon("search")
	replace := ui.Item("替换…", "Ctrl+H", cmd("替换：示例没有实现")).WithIcon("replace")

	refresh := ui.Item("刷新", "Ctrl+R", cmd("已刷新")).WithIcon("refresh")
	settings := ui.Item("设置…", "", cmd("设置：示例没有实现")).WithIcon("settings")
	about := ui.Item("关于", "", cmd("Ghromex 组件库 · 纯 Go 渲染")).WithIcon("info")

	return ui.Spec{
		Title:       "Ghromex 组件库",
		Width:       winW,
		Height:      winH,
		Status:      "就绪",
		StatusRight: "Ctrl+S 保存 · Ctrl+F 查找 · Ctrl+H 替换",
		Menus: []ui.Menu{
			{Title: "文件", Items: []ui.Action{
				newFile, openFile, ui.Sep(),
				saveFile, saveAs, ui.Sep(),
				printDoc, ui.Sep(),
				quit,
			}},
			{Title: "编辑", Items: []ui.Action{
				undo, redo, ui.Sep(),
				cut, copyCmd, paste, ui.Sep(),
				del, ui.Sep(),
				find, replace,
			}},
			{Title: "工具", Items: []ui.Action{refresh, settings}},
			{Title: "帮助", Items: []ui.Action{about}},
		},
		Tools: []ui.Action{
			newFile, openFile, saveFile,
			ui.Sep(),
			cut, copyCmd, paste,
			ui.Sep(),
			undo, redo,
			ui.Sep(),
			find, replace,
			ui.Sep(),
			printDoc, refresh,
			ui.Sep(),
			settings, about,
		},
	}
}

// addControls 用 Go 侧控件填充页面里的挂载点。
//
// 页面只写一个 #controls-host；控件的类名、取值和事件适配由 ui 包负责。
// dump 路径传 nil 状态回调，保证无头出图与真实窗口是同一套 DOM。
func addControls(doc engine.HTMLDocument, status func(string)) {
	host := doc.QuerySelector("#controls-host")
	if host == nil {
		return
	}
	say := func(text string) {
		if status != nil {
			status(text)
		}
	}

	name := ui.NewTextInput(doc, "未命名.txt")
	nameField := ui.NewField(doc, "文件名称", name)
	nameField.SetHint("最多 255 个字符")
	host.AppendChild(nameField.Element())

	auto := ui.NewCheckbox(doc, "保存时自动备份", true)
	auto.OnChange(func(on bool) {
		if on {
			say("已开启自动备份")
			return
		}
		say("已关闭自动备份")
	})
	host.AppendChild(auto.Element())

	encoding := ui.NewSelect(doc, []ui.SelectOption{
		{Value: "utf-8", Label: "UTF-8"},
		{Value: "gbk", Label: "GBK"},
	})
	encoding.SetValue("utf-8")
	encoding.OnChange(func(value string) { say("编码：" + value) })
	host.AppendChild(ui.NewField(doc, "编码", encoding).Element())

	preview := ui.NewTextView(doc, "这是只读多行文本视图。\n换行会保留，内容超出高度时可滚动。")
	preview.Element().SetAttribute("style", "height:80px") // 主题默认 140，示例压矮一点
	host.AppendChild(ui.NewField(doc, "预览", preview).Element())

	apply := ui.NewButton(doc, "应用设置", func() { say("已应用设置") }).SetVariant("primary")
	host.AppendChild(apply.Element())
}

// dump 在没有窗口的情况下装好工作台并出一张图。
//
// dumpBuffer 出「裸页面」的 buffer 快照：只开缓冲视口 + 打开页面就光栅化
// 落盘——不装工作台、不接线，与 -dump-png（装外壳的完整流程）对照。
// OpenDocument 已完成首次级联+布局，直接 RenderNode 即可。
func dumpBuffer(path string) error {
	vp, buf := engine.NewBufferedViewport(winW, winH)
	doc, err := engine.OpenDocument(vp, page)
	if err != nil {
		return err
	}
	engine.RenderNode(buf, doc)
	return buf.DumpPNG(path)
}

// 这条路径和真实窗口共用 install + 布局 + 渲染，所以 PNG 看到的就是
// 窗口里的形状与配色（文字是软件光栅后端的占位方块）。
func dump(spec ui.Spec, pngPath, svgPath string) error {
	vp, buf := engine.NewBufferedViewport(winW, winH)
	doc, err := engine.OpenDocument(vp, page)
	if err != nil {
		return err
	}
	ui.Install(doc, spec)
	addControls(doc, nil)
	doc.Refresh()
	engine.RenderNode(buf, doc)
	// 两个出口可以同时要：先出 PNG，再出 SVG（同一份布局）。
	if pngPath != "" {
		if err := buf.DumpPNG(pngPath); err != nil {
			return err
		}
	}
	if svgPath != "" {
		return os.WriteFile(svgPath, []byte(doc.DumpSVG()), 0o644)
	}
	return nil
}

// saveShot 用真实字体渲染一帧并截图：Refresh 重排、RenderFrame 画一帧、
// Window.Screenshot 回读窗口像素。
//
// 与 -dump-png 的区别：那条路用软件光栅后端，文字是占位方块（它的用途
// 是逐像素断言）；这条走真实字体与 SDL 渲染器，用来检查字号、行高、
// 垂直居中的观感。
func saveShot(app *ui.App, path string) error {
	// Install 刚改过 DOM 与样式表，必须先重排再画：RenderFrame 只负责
	// 「清屏 + 画当前布局」，不会自己重排（主循环里由 ChangeCount 驱动）。
	if doc := app.Workbench().Doc(); doc != nil {
		doc.Refresh()
	}
	app.Window().RenderFrame()
	raw, _, w, h, err := app.Window().Screenshot()
	if err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// 后端回读格式为 XBGR8888（小端内存序 R,G,B,X）：前三个字节就是
	// 颜色，第 4 字节不是 alpha，统一补 255。
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			o := y*img.Stride + x*4
			img.Pix[o] = raw[i]
			img.Pix[o+1] = raw[i+1]
			img.Pix[o+2] = raw[i+2]
			img.Pix[o+3] = 255
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
