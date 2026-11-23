// Package ghromex 是整个模块的概览（本文件不含任何声明，只是一段可被
// go doc 读到的说明）。
//
// 根目录原先只有 go.mod，没有 .go 文件，于是 `go doc
// github.com/go-comm/ghromex` 报「no required module provides package」，
// 接口无处可查。本包刻意**不 import 仓库内任何包**——它保持叶子地位，
// core/arch_test.go 的依赖单向规则因此不变。
//
// # 是什么
//
// Ghromex 是纯 Go 的 HTML/CSS 桌面 GUI 渲染引擎（迷你浏览器 + 应用框架）：
// SDL2 后端、无 CGO、无 JS、无 WebView，适合做工具类桌面应用。用 Go 重写
// HTML/CSS 解析、级联、布局、绘制、事件与文本编辑的一个子集。
//
// # 分层与依赖方向（单向，由 core/arch_test.go 钉死）
//
//	ui         应用工作台：菜单/工具栏/状态栏/主题/快捷键
//	  └ browser    门面：窗口 + 主循环 + 持有文档
//	     ├ engine     纯 Go 引擎：解析 → 级联 → 布局 → 绘制 → 命中/事件
//	     ├ backend/*  平台实现：sdl2 窗口与绘制、dialog、clipboard
//	     └ core       契约：Graphics / Viewport / 跨层值类型（叶子包）
//
//	runtime/{clipboard,dialog,screen} 是平台能力的通用入口：调用方只
//	import 这一层，不直接碰 backend。examples/ 下各目录是可独立运行的示例。
//
// 路径前缀一律 github.com/go-comm/ghromex/，下同。
//
// # 应用层入口
//
//	win, _ := browser.New(900, 620, "记事本")  // New→Run→Close 同一 goroutine
//	defer win.Close()
//	doc, _ := win.OpenDocument(html)
//	win.Run()
//
// 要菜单/工具栏/状态栏/主题用 ui.New(ui.Spec{...}) 或 ui.Install(doc, spec)；
// 原生对话框走 win.NativeHandle() + runtime/dialog，剪贴板走
// runtime/clipboard，抓屏走 runtime/screen。
//
// # 改代码前必读的不变量
//
//  1. 依赖单向：core 是叶子；engine 不 import 平台包；backend 禁止 import
//     engine；browser 才是把 engine 与 backend 缝起来的那一层（违反会挂
//     core/arch_test.go）。
//  2. 1 CSS px = 1 物理像素，布局全程整数像素（与浏览器的亚像素差异是
//     已知取舍）。
//  3. 重排只由 doc.ChangeCount() 变化驱动：markChanged 是唯一入口，**布局
//     期不得调用**；命中测试 ElementAt 与绘制 RenderNode 必须同口径，否则
//     「看得见点不到」。
//  4. 新窗口 → Run → Close 必须同一 goroutine（SDL LockOSThread 要求）；
//     仅 Windows 有平台实现，其它平台返回 ErrUnsupported（别 panic）。
//
// 完整版不变量与坑见 engine/doc.go 与 docs/ai/architecture.md。
//
// # 查接口（AI 自检入口）
//
// `go doc` 不吃 `./...`、也不接受多个包位置参数，按「一个包一条命令」查：
//
//	go doc github.com/go-comm/ghromex          模块总览（就是本文件，也可 go doc .）
//	go doc github.com/go-comm/ghromex/<包>     该包导出 API + 包说明
//	go doc github.com/go-comm/ghromex/<包> <符号>     单个类型/函数（含方法）
//	go list ./...                    包清单（要全量就把清单里每个包喂给上面的 go doc）
//
// 全量落盘版：docs/ai/api-reference.md（由 tools/gen-ai-context.ps1 重新生成）。
//
// 包 → 职责（据此定位该查哪个包）：
//
//	core        契约：Graphics / Viewport / Color / Size / Paint / Sprite
//	engine      引擎：HTML/CSS 解析级联布局绘制 + 事件/编辑 + Fake/Buffer/Headless 视口
//	browser     门面：New / OpenDocument / Run / SetFrameHandler / NativeHandle
//	ui          工作台：菜单 / 工具栏 / 状态栏 / 主题 / TextInputView / BarView
//	runtime/*   平台能力：dialog / clipboard / screen（只 import 这一层，别碰 backend）
//	backend/*   平台实现：sdl2（窗口/绘制）/ dialog / clipboard
//	examples/*  可运行示例：browser / notepad / kit / portal / ua / stylebook / snip /
//	            iconsheet / stressgen
//
// 落盘文档（很长，按需检索，别整读）：
//
//	docs/ai/architecture.md   架构地图（分层、不变量、扩展点、坑）
//	docs/ai/api-reference.md  全量 API（自动生成，勿手改）
//	README.md                 行为口径 + 性能实测
//	AGENTS.md                 AI 开发指引入口
package ghromex
