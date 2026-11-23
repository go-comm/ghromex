# ghromex — AI 开发指引

纯 Go 的 HTML/CSS 桌面 GUI 渲染引擎（迷你浏览器 + 应用框架）：SDL2 后端、无 CGO、无 JS。模块 `github.com/go-comm/ghromex`，Go 1.20+。

## 开始开发前必读

1. **`go doc github.com/go-comm/ghromex`** — 模块总览与**接口导航**：定位、分层图、核心入口、必守不变量，以及「该查哪个包、用哪条命令查」。跑完这条就知道接口在哪，不必猜包名。
2. **`docs/ai/architecture.md`** — 架构地图：分层与依赖规则、核心接口、主循环、典型用法代码、扩展点、坑与惯例。**改动前先读它。**
3. **接口就地查，不落盘**：`go doc github.com/go-comm/ghromex/<包>[ <符号>]`（离线可用）或 https://pkg.go.dev/github.com/go-comm/ghromex 。仓库里不再维护自动生成的 API 清单——它与 `go doc` 同源、只会过期和制造 diff 噪音。
4. 行为口径与专项数据按 `README.md` 的「文档地图」进分册：CSS 支持 `docs/css-support.md`、与浏览器的差异 `docs/limitations.md`、性能实测 `docs/performance.md`、组件库 `docs/ui.md`、调试与测试 `docs/development.md`。

## 30 秒速览

- 依赖单向：`core`（契约，叶子包）← `engine`（纯 Go 引擎）← `browser`（门面+主循环）← `ui`（工作台）；`backend/*` 只依赖 `core`，**禁止 import engine**（`core/arch_test.go` 会挂）。
- 应用层入口：`browser.New → OpenDocument/OpenDocumentWith → Run`；要菜单/工具栏用 `ui.New`/`ui.Install`。平台能力走 `runtime/{dialog,clipboard,screen}`，不直接碰 backend。
- 重排只由 `doc.ChangeCount()` 变化驱动；命中测试 `ElementAt` 与绘制 `RenderNode` 必须同口径。
- 仅 Windows 有平台实现，其它平台返回 `ErrUnsupported`（保持这一约定，别 panic）。

## 改动约定

- 构建/测试任意平台都要过：`go build ./...`、`go test ./...`（平台差异用构建标签 + stub 文件）。
- `NewWindow→Run→Close` 同一 goroutine（SDL `LockOSThread`）。
- 新增 API 写中文 doc comment（`go doc` / pkg.go.dev 即时可见）；跨层值类型放 `core`，`engine/graphics.go` 加类型别名。
- 提交前更新 `CHANGELOG.md`；性能敏感改动在 `docs/performance.md` 留同口径对比数据。

## 无头验证（无需显示器/DLL）

```
go run ./examples/ua -dump-svg out.svg        # 布局导出 SVG
go run ./examples/kit -shot kit.png           # 软件光栅化截图
```

测试里用 `engine.NewHeadlessViewport` / `engine.NewBufferedViewport` 做像素级断言。
