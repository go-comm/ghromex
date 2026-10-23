// Package ui 是应用工作台与页面组件库。
//
// 它提供三样东西：
//
//   - 工作台：菜单栏（含下拉浮层、快捷键、禁用态）、工具栏、状态栏（左侧分段 + 右侧消息）；
//   - 页面组件：卡片、标题、副标题、徽标、提示条、列表、操作行、输入框、下拉、勾选、只读文本视图、按钮；
//   - 主题：浅色/深色两套视觉令牌 + 生成的样式表，应用一行 CSS 都不用写。
//
// 包内分两层，各管各的生命周期：
//
//   - Workbench（工作台，入口 Install）：装在一份文档上的周边构件——内容四周的
//     菜单栏/工具栏/状态栏、主题样式与快捷键表。只依赖 engine，无头也能装；
//     换文档要重装（工作台活在文档 DOM 里）。
//   - App（应用，入口 New）：窗口的生命周期封装——建 browser.Window、换文档时
//     重装工作台、按键路由到当前工作台、Run/Close。App 只是 Install 的一个
//     固定用法，需要自管窗口（或不要窗口）时直接用 Install。
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
//			ui.Tool("保存", save).WithIcon("save").When(editor.Dirty),
//			ui.Sep(),
//			ui.Tool("复制", copy).WithIcon("copy"),
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
// 页面内容只写类名（<div class="system-ui-page">、<div class="system-ui-card">…），
// 样式全部来自 Install 注入的主题样式表。组件类统一带 system-ui- 前缀，并
// 只在 Install 加到文档根上的 system-ui 作用域下生效；根作用域把组件规则
// 的特异度抬高一档，第三方 `.panel div` 之类的后代选择器压不过。
//
// body 的复位同样归工作台：装了工作台的文档，body 无外边距、颜色字体跟随
// 主题，页面不必自写 margin:0 / padding:0 / color / font-family 这些样板；
// 自定义排版请挂在内容容器上（根作用域下 body 规则特异度更高，页面的
// 裸 body 规则盖不过它）。确实要保留 body 原样的页面（演示 UA 样式、靠
// body padding 给固定栏让位）用 Spec.NoBodyReset 跳过复位。
//
// # 状态栏
//
// 左侧可以分段，段有 Key、固定/自适应宽度、段内对齐和隐藏态；右侧消息独立
// 更新，非空时按 Theme.StatusRightWidth 预留宽度。高频更新只改文字：
//
//	spec.StatusSegments = []ui.StatusSegment{
//		{Key: "file", Text: "未命名", Width: 220},
//		{Key: "pos", Text: "行 1，列 1", Width: 140},
//		{Key: "chars", Text: "0 个字符"},
//	}
//	sh.SetStatusSegment("pos", "行 2，列 3")
//	sh.SetStatusRight("已保存")
//
// Spec.Status / SetStatus 是单段兼容写法。
//
// # 页面控件
//
// 页面可以只写类名，也可以用 Go 侧包装构造原生控件；NewXxx 返回的 View 都
// 实现 Element()，可直接 AppendChild。注意 NewButton 是内容区按钮，工具栏
// 的 ui.Tool 仍构造 Action：
//
//	name := ui.NewTextInput(doc, "未命名.txt")
//	field := ui.NewField(doc, "文件名称", name)
//	field.SetHint("最多 255 个字符")
//	page.AppendChild(field.Element())
//
//	auto := ui.NewCheckbox(doc, "自动保存", true)
//	auto.OnChange(func(on bool) { /* ... */ })
//	page.AppendChild(auto.Element())
//
// 横着一排控件（查找替换栏、地址栏这类）用 NewBar 拼：条内控件排成一行、
// 按 Theme.BarHeight 垂直居中，条自己不管摆哪儿——定位声明交给 SetInline，
// 收起用 SetVisible（它只在应用那份声明上补 display:none，不会冲掉 top）：
//
//	bar := ui.NewBar(doc, keyword, find).SetInline("position:absolute;left:0;top:72px")
//	bar.SetVisible(false)
//	page.AppendChild(bar.Element())
//
// 工具栏里也要放输入框/下拉时，用 Slot 把现成控件嵌进去（槽位不是命令，
// 没有快捷键与可用态；控件句柄仍归应用）：
//
//	addr := ui.NewTextInput(doc, "")
//	...
//	Tools: []ui.Action{ui.Tool("刷新", refresh), ui.Slot(addr)},
//
// 输入框目前没有实时 input/change 事件，实时联动用 OnKeyDown 或在命令执行
// 时读取 Value；只读多行文本用 TextView，可编辑多行用 TextAreaView——要它
// 贴着铺满整块内容区（纸面而不是表单字段）就切平铺变体：
//
//	editor := ui.NewTextArea(doc, "")
//	editor.SetVariant("plain") // 去掉描边与圆角
//
// # 悬停提示
//
// 引擎内置系统气泡式提示（原生 title 属性）：在带非空 title 的元素（含其
// 子孙）上停留片刻，鼠标右下弹出气泡；离开、按下、点击或滚动收起，换目标
// 重新计时。工具栏按钮的提示已按 Label + Shortcut 自动生成，图标按钮悬停
// 就能看到文字；页面元素要提示就直接写 title 属性：
//
//	btn.Element().SetAttribute("title", "保存到当前文件（Ctrl+S）")
//
// 气泡配色跟随主题（Install / SetTheme 自动下发：浮层底色 + 描边 + 主文字）；
// 不用 ui 的裸引擎应用可用 engine.SetTooltipColors 自行指定，默认深底浅字。
//
// # 图标
//
// 命令可以用 WithIcon 挂图标：
//
//	ui.Tool("保存", save).WithIcon("save")
//	ui.Item("新建", "Ctrl+N", newFile).WithIcon("new")
//
// 内置图标名见 iconGlyphs，按用途分组：
//
//	文件/编辑  new open save save-as copy cut paste undo redo search replace
//	          print delete refresh folder page document code edit rename select-all
//	方向/导航  up down left right chevron-up chevron-down chevron-left chevron-right
//	          page-left page-right back forward home go-to-start menu history
//	对齐/排版  align-left align-center align-right bulleted-list list check-list
//	          bold italic underline font-size font-color highlight
//	播放/媒体  play pause stop record previous next rewind fast-forward shuffle
//	          repeat repeat-one volume mute fullscreen back-to-window
//	视图/窗口  zoom-in zoom-out fit-page view view-all preview dock-left dock-right
//	          dock-bottom close-pane open-pane minimize maximize restore
//	通用       add remove check close more filter settings info help warning error
//	          favorite share link attach send folder calendar
//
// 也可以直接给一个字符。字形来自 Windows 10/11 自带的 Segoe MDL2 Assets
// （segmdl2.ttf），按 16px 网格设计、颜色跟随文字，悬停/禁用/换主题都不需要
// 额外资源。工具栏按钮有图标时画成方形图标按钮，没有图标时退回文字按钮；
// 菜单里只要有一项带图标，整张菜单就会预留图标列。
//
// 需要枚举全部图标（做选择器、文档或图标表）时调用 Icons()：返回按名称
// 排序的 Name/Glyph 副本，别名也会作为独立条目出现。examples/iconsheet
// 是一个可直接运行的图标表。
//
// 图标字体是系统字体：跨平台部署时要么在 Theme.IconFont 里给一个同编码
// 的图标字体，要么只写文字（不调 WithIcon）。
//
// # 主题
//
// 内置浅色 Light() 与深色 Dark()；Spec.Theme 留空时用浅色：
//
//	dark := ui.Dark()
//	spec.Theme = &dark
//
// 两套主题的尺寸与字体相同，只换颜色。想微调时先取完整的一套再改字段：
//
//	th := ui.Dark()
//	th.Accent = "#22C55E"
//	spec.Theme = &th
//
// 不能只写 ui.Theme{Bg: "#0F172A"} 就指望得到深色：零值字段统一按浅色
// 补齐（零值 Theme 的默认语义是浅色），其余颜色会留在浅色上。OnAccent
// 是强调色上的文字/图标颜色，深色主题用它把亮蓝底上的字压成深色。
//
// 运行时切换用 App.SetTheme（或 Workbench.SetTheme）：新主题样式追加在旧样式
// 之后，页面内容不重建，适合浅色/深色切换。
//
// # 语义色
//
// 主题提供六组语义色，页面只写类名，不用记十六进制：
//
//	Primary/Accent  强调：主按钮、选中、链接、焦点
//	Default         默认/中性：次按钮、取消
//	Success         成功；Warning 警告；Danger 危险；Info 信息
//
// 每组令牌分实色与淡底两套：Accent/Success/Warning/Danger/Info 是实底，
// On* 是实底上的文字，*Soft/*Border/*Text 是淡底、描边与淡底文字。
// Default 不单独存令牌，复用 Surface/Border/Text/SurfaceAlt/Press 这组
// 中性色。按钮用 is-primary/is-default/is-success/is-warning/is-danger/
// is-info；徽标、提示条与文字色类同样接受这些后缀：
//
//	<div class="system-ui-btn is-success">通过</div>
//	<span class="system-ui-badge is-warning">待处理</span>
//	<div class="system-ui-alert is-danger">删除后无法恢复。</div>
//	<span class="system-ui-text-info">共 12 项</span>
//
// 对比度由主题负责：Warning 的实底用深色 On，其它实底在浅色主题用白字、
// 深色主题用同色系深字。
//
// # 为什么是「文档内自绘」而不是原生控件
//
// 引擎只渲染文档，没有独立的窗口 chrome 层。组件库的做法是把工作台作为
// 文档的一部分：position:fixed 压在页面上、body 首尾插占位块让出空间、
// 换文档时重装。好处是纯 Go、跨平台、可测试（无头就能导出 PNG/SVG 看
// 效果）；代价是页面自己 z-index 更高的元素会盖住工作台，且工作台不参与
// 原生窗口菜单的键盘语义。
//
// # 三个必须知道的约定
//
//   - 工作台是文档的一部分：换文档后要重新 Install（App 已通过
//     browser.Window.SetOnDocument 自动完成）。
//   - 应用快捷键先于页面：App 用 browser.Window.SetKeyHandler 注册，
//     输入框聚焦时 Ctrl+S 也先到应用。
//   - 主题只使用引擎支持的 CSS 子集：没有 flex/grid/过渡，主题保持扁平
//     （不用阴影），横向排列靠 inline-block，分隔线是 1px 的实体元素。
package ui
