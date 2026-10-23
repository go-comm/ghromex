# ghromex — AI 开发指引

纯 Go 的 HTML/CSS 桌面 GUI 渲染引擎（迷你浏览器 + 应用框架）：SDL2 后端、无 CGO、无 JS。模块 `github.com/go-comm/ghromex`，Go 1.20+。

## 开始开发前必读

1. **`docs/ai/architecture.md`** — 架构地图：分层与依赖规则、核心接口、主循环、典型用法代码、扩展点、坑与惯例。**改动前先读它。**
2. **`docs/ai/api-reference.md`** — 全部公开 API（自动生成，勿手改）。接口变了运行 `powershell -File tools\gen-ai-context.ps1` 重新生成。
3. 详细行为口径、性能实测数据见 `README.md`（很长，按需检索章节，不要整读）。

## 30 秒速览

- 依赖单向：`core`（契约，叶子包）← `engine`（纯 Go 引擎）← `browser`（门面+主循环）← `ui`（工作台）；`backend/*` 只依赖 `core`，**禁止 import engine**（`core/arch_test.go` 会挂）。
- 应用层入口：`browser.New → OpenDocument/OpenDocumentWith → Run`；要菜单/工具栏用 `ui.New`/`ui.Install`。平台能力走 `runtime/{dialog,clipboard,screen}`，不直接碰 backend。
- 重排只由 `doc.ChangeCount()` 变化驱动；命中测试 `ElementAt` 与绘制 `RenderNode` 必须同口径。
- 仅 Windows 有平台实现，其它平台返回 `ErrUnsupported`（保持这一约定，别 panic）。

## 改动约定

- 构建/测试任意平台都要过：`go build ./...`、`go test ./...`（平台差异用构建标签 + stub 文件）。
- `NewWindow→Run→Close` 同一 goroutine（SDL `LockOSThread`）。
- 新增 API 写中文 doc comment（会进入 api-reference）；跨层值类型放 `core`，`engine/graphics.go` 加类型别名。
- 提交前更新 `CHANGELOG.md`；性能敏感改动在 README 性能章节留同口径对比数据。

## 无头验证（无需显示器/DLL）

```
go run ./examples/ua -dump-svg out.svg        # 布局导出 SVG
go run ./examples/kit -shot kit.png           # 软件光栅化截图
```

测试里用 `engine.NewHeadlessViewport` / `engine.NewBufferedViewport` 做像素级断言。
