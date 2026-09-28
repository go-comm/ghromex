# Ghromex

用 HTML/CSS 写原生桌面 GUI 的迷你 WebView 框架。

用 Go 实现一个小型渲染引擎：解析 HTML/CSS → 级联计算样式 → 盒式布局 → 绘制指令交给渲染后端（当前为 SDL2，纯动态加载，**无需 CGO**）。渲染 1 CSS px = 1 物理像素，窗口尺寸即视口尺寸，天然适合桌面小部件与内部工具界面。

## 特性

- 纯 Go 渲染引擎：HTML 解析、CSS 级联、选择器（标签 / `.class` / `#id` / 简单组合，含特异度）、块级与行内（inline / inline-block）布局、文本按词/按字换行
- DOM 式 API：`QuerySelector` / `GetBoundingClientRect` / `SetText` / `OnClick` 等；内容变化自动触发重排重绘
- 事件冒泡：命中测试找到最深元素，沿父链依次派发 `click`
- 可插拔图形后端：`engine.Graphics` 接口（DrawText / DrawColor / DrawImage / MeasureText），内置 `FakeGraphics`（计数）、`BufferGraphics`（软件光栅化到 RGBA 缓冲，可 `SavePNG`）供无显示器测试
- 元素注册表：`CustomElements().Define(tag, proto)` + `CloneElement` 注册自定义标签，解析时克隆原型生成独立实例
- 无头运行：`--dump-svg` 导出布局结果；`HeadlessViewport` + `BufferGraphics` 可在 CI 中像素级断言渲染结果

## 环境要求

- Go 1.25+（无需 CGO）
- 渲染后端目前提供 Windows 实现（`backend/sdl2`，加载 `SDL2.dll` 2.28 与 `SDL2_ttf.dll`）
- 把 `SDL2.dll`、`SDL2_ttf.dll`（及 `zlib1.dll`）放到工程根目录 `libs/`，或保证系统可搜索到它们

## 快速开始

```powershell
# 运行内置登录页 demo
go run ./demo/

# 渲染自己的页面
go run ./demo/ --file page.html

# 无头导出 SVG（不需要显示器）
go run ./demo/ --dump-svg out.svg
```

窗口交互示例：`demo/index.html` 中的登录/重置按钮通过 `OnClick` 更新状态文本，引擎检测到文档变化后自动重排重绘。

## 代码示例

```go
package main

import (
	"fmt"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/engine"
)

func main() {
	win, err := sdl2.NewWindow(800, 600, "Hello Ghromex")
	if err != nil {
		panic(err)
	}
	defer win.Close()

	doc, err := win.OpenDocument(`<body style="padding:16px">
		<button id="ok" style="background-color:#2563EB;color:#FFF">点我</button>
		<span id="n">0</span>
	</body>`)
	if err != nil {
		panic(err)
	}

	n, clicks := doc.QuerySelector("#n"), 0
	doc.QuerySelector("#ok").OnClick(func(ev *engine.MouseEvent) {
		clicks++
		n.SetText(fmt.Sprintf("已点击 %d 次", clicks)) // 修改文档即触发重排重绘
	})

	// 注意：SDL 要求窗口生命周期在同一 OS 线程，NewWindow/Run/Close 必须在同一 goroutine
	if err := win.Run(); err != nil {
		panic(err)
	}
}
```

无头测试（不依赖任何 DLL）：

```go
vp := engine.NewHeadlessViewport(400, 300)
doc, _ := engine.OpenDocument(vp, `<html><body><div class="box">hi</div></body></html>`)
buf := engine.NewBufferGraphics(400, 300)
engine.RenderNode(buf, doc)
_ = buf.SavePNG("out.png") // 或直接 buf.ColorAt(x, y) 断言像素
```

## 项目结构

```
engine/            渲染引擎（纯 Go，不依赖任何图形后端）
  html_parser.go     HTML 解析 → DOM 树
  css_parser.go      CSS / 内联样式解析
  style_cascade.go   UA 默认样式 + 选择器匹配 + 特异度级联
  measure.go         盒模型布局、行盒、换行
  render.go          绘制指令下发（背景/边框/文本片段）
  event_dispatch.go  命中测试 + click 冒泡派发
  graphics.go        Graphics 接口 / FakeGraphics / BufferGraphics
  svg_dump.go        布局结果导出 SVG
  element_registry.go 标签原型注册表（自定义元素）
backend/sdl2/      SDL2 后端：窗口、事件循环、SDL_ttf 文本纹理缓存
components/        组件工厂（button / input / class 工具）
demo/              演示应用（含内置登录页）
libs/              SDL2 运行库（SDL2.dll / SDL2_ttf.dll / zlib1.dll）
```

## 已支持的 CSS 子集

`display(none/inline/inline-block/block)`、`width/height`、`margin*`、`padding*`、
`border` 简写与 `border-width/color/style`、`border-radius`(四角统一，仅水平半径)、
`background-color`/`background`、`color`、`font-size`、`font-weight(bold)`、`font-family`、
`text-align(left/center/right)`；
字体默认语义对齐 Chrome（Windows）并做 UI 取向调整：初始字号 16px；
standard（未指定 family）默认 sans-serif → Arial（区别于 Chrome 的 Times，更贴桌面 UI）；
serif → Times New Roman、monospace → Courier New、system-ui → Segoe UI；
汉字/假名/谚文按字符级脚本分段 fallback 到微软雅黑（中英混排与 Chrome 一致）；
每个字体文件只开一个 FT_Face，字号/粗体动态切换；
文字渲染用 TTF LIGHT hinting（较默认 NORMAL 的网格吸附，小字号纵向硬跳变降约 60%，锯齿感明显减弱）；
选择器支持标签、`.class`、`#id`、类型+class/id 组合链（如 `div.wide`）与特异度排序。

## 已知限制

- 布局仅覆盖块级 + 行内/inline-block 流式布局；无 flex/grid、无滚动、无 position 偏移
- `DrawImage` 尚未实现（接口已预留）
- `input` 元素仅占位显示，键盘输入/焦点/IME 未接入
- 边框样式目前按单色实心绘制（dashed/dotted 等不做虚线区分）
- 字体：generic/具名映射基于 Windows 自带字体文件，缺失时逐级回退 standard；无 webfont 加载、无斜体渲染

## 调试环境变量

| 变量 | 作用 |
|---|---|
| `GHROMEX_DEBUG=1` | 回显绘制失败原因与每次点击的命中/派发结果 |
| `GHROMEX_SOFTWARE=1` | 强制软件渲染器（默认 GPU 加速，创建失败自动回退软件；用于 GPU 可创建但不出像素的虚拟显示环境） |
| `GHROMEX_FONT=路径` | 指定 standard 字体（默认 Arial，即 sans-serif） |
| `GHROMEX_CJK_FONT=路径` | 指定 CJK fallback 字体（默认微软雅黑） |
| `SDL_VIDEO_DRIVER=dummy` | 无显示器运行（配合 `Window.Screenshot()` 做回读断言） |

## 开发与测试

```powershell
go vet ./...
go test ./...
```

- `engine` 包测试全为纯 Go（FakeGraphics / BufferGraphics / SVG 断言），无显示器即可跑
- `backend/sdl2` 的事件解码回归测试不加载 DLL；事件字段偏移以真实 SDL 产生的事件实测为准（见 `decodeEvent` 注释）
- demo 退出约定：`go run ./demo/ --exit-after 3s` 可用于冒烟验证
