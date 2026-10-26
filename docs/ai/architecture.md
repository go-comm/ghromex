# ghromex 架构地图（AI 上下文）

> 本文件与 `api-reference.md`（自动生成）共同构成 AI 开发上下文。入口见根目录 `AGENTS.md`。

一句话：**ghromex 是一个纯 Go 的「迷你浏览器 + 桌面应用框架」**——用 Go 重写 HTML/CSS 的解析、级联、布局、绘制、事件与文本编辑（一个子集），SDL2 做窗口层，`ui` 包做应用工作台。没有 JS、没有 CGO、没有 WebView，适合做工具类桌面应用。

## 分层与依赖方向

```
                    ui（应用工作台/组件库）
                          │
                       browser（门面 + 主循环 + 持有文档）
                       ╱          ╲
        engine（纯 Go 渲染引擎）   backend/sdl2（平台：窗口/事件/绘制/剪贴板）
                       ╲          ╱
              core（契约层：Graphics / Viewport / 值类型）← 叶子包，不依赖任何包
```

| 包 | 职责 | 关键约束 |
|---|---|---|
| `core` | 引擎与后端之间的契约：`Graphics`、`Viewport`、`Color`/`Size`/`Paint`/`Sprite` | 叶子包，谁都不依赖；可选扩展 `Clipper`、`SpriteDrawer` 靠类型断言启用 |
| `engine` | 解析 HTML/CSS → 级联 → 布局 → 绘制 → 命中测试/事件 | **纯 Go**，不 import 平台包；`Graphics = core.Graphics` 用类型别名保持 API（`engine/graphics.go`） |
| `engine/element_registry.go` | 元素注册表：`CustomElements().Define(tag, proto)`；内置标签走 `SystemElements()`（`html_elements.go`） | 解析时克隆原型（`CloneElement`）生成独立实例 |
| `backend/sdl2` | Windows SDL2 窗口/渲染器/字体纹理/事件解码 | **不认识 engine**；动态加载 DLL，无 CGO；`NewWindow → Run → Close` 同一 OS 线程（`LockOSThread`） |
| `backend/clipboard` `backend/dialog` | 剪贴板复用 SDL2；原生对话框走 Win32 COM + user32 | 仅 Windows，其它平台返回 `ErrUnsupported` |
| `runtime/{clipboard,dialog,screen}` | 上述平台能力的通用调用入口（类型别名转发） | 调用方只 import 这一层，不碰 backend |
| `browser` | 装配门面：持有 `HTMLDocument`，跑主循环（事件 → 引擎动作 → 重排 → 重绘） | 生命周期同一 goroutine；把平台差异挡在应用之外 |
| `ui` | 菜单栏/工具栏/状态栏 + 页面控件（TextInputView/TextAreaView/BarView）+ 浅色/深色主题 + 快捷键 | 装在文档 DOM 里（`position:fixed` + body 占位块），换文档要重装 |

**为什么有 `core`**：Go 接口是结构化的。若后端直接 import 引擎来「实现引擎的接口」，依赖方向就反了。所以把接口和跨层值类型下沉到 `core`，引擎用类型别名（`type Color = core.Color`）保持对外 API 不变。`core/arch_test.go` 是架构护栏：钉死依赖单向、平台层禁止反向 import 上层、`browser` 必须同时依赖 engine 与 backend/sdl2。

## 两个核心抽象

```go
// core/graphics.go —— 绘制后端要实现的全部方法
type Graphics interface {
    DrawText(x, y, w, h int, paint Paint, text string)
    DrawColor(x, y, w, h int, color Color)
    DrawImage(x, y, w, h int, image Image)
    MeasureText(text string, fontSize int, bold bool, family string) (w, h int)
    FontMetrics(text string, fontSize int, bold bool, family string) (ascent, descent int)
}

// core —— 渲染输出目标
type Viewport interface {
    ViewportWidth() int
    ViewportHeight() int
    Graphics() Graphics
}
```

- 引擎只认这两个接口 → SDL2、软件光栅化（`BufferGraphics`）、无头（`FakeGraphics`）跑同一套布局。
- `MeasureText` 与 `DrawText` **必须同一字体解析规则**，否则混排宽度错位。
- 可选扩展（实现了就用，没实现回退到等价但更慢的路径）：`Clipper` 裁剪栈、`SpriteDrawer` 位图 blit。

## 主循环与事件模型

`browser.Window.Run()` 每帧：

1. `PollEvent` 取平台事件（SDL 解码成 `sdl2.Event`）
2. 平台事件 → 引擎动作：`OnDocumentClick` / `OnDocumentKeyDownMods` / `OnDocumentWheel` / `OnDocumentMouseMove`…（命中测试取最深元素 → 沿父链冒泡 → 执行默认动作）
3. `doc.ChangeCount()` 变了才 `LayoutDocument`（变更计数是**唯一**重排触发器，布局期禁止 `markChanged`）
4. `engine.RenderNode(g, doc)` 绘制 + `Present`
5. `engine.TickScroll` / `engine.TickTooltip` 推进滚动动画与提示气泡（决定本帧是否休眠）

三个「状态存在文档上」的伪类：`:hover` / `:focus` / `:active`（`SetHoveredElement` / `SetFocusedElement` / `SetActiveElement`），变化走同一条 `markChanged → 重排重绘` 链路。

## 典型用法（由浅入深）

**最简——一个窗口显示 HTML**（`examples/browser` 同款）：

```go
win, _ := browser.New(900, 620, "记事本")
defer win.Close()
doc, _ := win.OpenDocument(html)
win.Run()
```

**外部样式表（`<link>` / `@import`）**——选项只能在打开时给，`<link>` 与 `@import` 在解析期加载，事后 `SetStylesheetLoader` 补不上已有引用：

```go
doc, _ := win.OpenDocumentWith(src, engine.DocumentOptions{
    BaseURL:        "file:///C:/pages/index.html",
    LoadStylesheet: loadFromDisk, // func(href string) (string, error)，同步加载
})
```

**应用工作台（菜单/工具栏/状态栏/主题）**：

```go
app, _ := ui.New(ui.Spec{
    Title: "记事本", Width: 900, Height: 620,
    Menus: []ui.Menu{{Title: "文件", Items: []ui.Action{
        ui.Item("保存", "Ctrl+S", save).WithIcon("save").When(dirty),
        ui.Sep(), ui.Item("退出", "Alt+Q", ui.Quit),
    }}},
})
defer app.Close()
app.LoadHTML(page)
app.Run()
```

`ui.Install(doc, spec)` 是「只要文档层、不要窗口」的版本（无头也能装、能 DumpSVG；`examples/notepad` 走的就是这条 + `ui.Light()` 主题）。

**原生对话框**：

```go
hwnd, _ := win.NativeHandle()
paths, _ := dialog.OpenFileDialog(dialog.Parent(hwnd), dialog.Options{...})
```

## 关键设计取舍（理解了这些就理解了库）

1. **1 CSS px = 1 物理像素**，布局全程整数像素。简化亚像素问题，代价是与浏览器有差异。
2. **变更计数驱动重排**：`doc.ChangeCount()` 变了才布局；`markChanged` 是唯一入口，布局期禁止调用。
3. **命中测试与绘制同口径**：`ElementAt` 和 `renderNode` 必须一致（裁剪、定位层、select 浮层两侧都算），否则「看得见点不到」。
4. **行盒只有一处口径**：行盒高 = max(ascent) + max(descent)，`Leading` 与 `vertical-align` 由 `line_box.go` 统一算，绘制/命中/SVG 同一份取数。
5. **后端可选扩展**：`Clipper` / `SpriteDrawer` 接口检测，不实现就走等价但更慢的回退。
6. **平台能力下沉**：剪贴板、对话框、抓屏在 `runtime/*` 收口，`engine` 保持纯 Go（读剪贴板靠后端读出后调 `OnDocumentPaste`，写靠引擎给 `SelectedText` 后端去写）。
7. **UI 是文档内自绘**：`position:fixed` 压页面上、body 首尾插占位块；换文档重装。代价是页面 `z-index` 更高的元素会盖住工作台。

## 容易踩的坑 / 限制

- **仅 Windows**：SDL2 后端、原生对话框、剪贴板、抓屏都是 Windows 实现，其它平台返回 `ErrUnsupported`（`browser` 与 `examples/` 不带构建标签，任意平台 `go build ./...` / `go test ./...` 都过，非 Windows 启动窗口时打印提示）。
- **线程**：`NewWindow → OpenDocument → Run → Close` 必须同一 goroutine（SDL `LockOSThread` 要求，见 `backend/sdl2/doc.go`）。
- **无 flex / grid / 过渡动画**，布局只有 block / inline / inline-block + relative/absolute/fixed；UI 主题靠 `inline-block` 和 1px 实体分隔线拼。
- **没有实时 `input` 事件**：实时联动用 `OnKeyDown`（元素级按键处理器，返回 true = 已消费，等价 preventDefault）或在命令执行时读 `Value`。
- **多窗口未实现**：API 塑形成「可多次 New」，底层基础待真实需求再补。
- **行高与 Chrome 可能差 1~4px**；LCD 子像素渲染默认关（灰度 AA），`GHROMEX_LCD=1` 开启；`GHROMEX_DEBUG=1` 打印渲染路径与驱动名。
- **文本度量口径**：`FakeGraphics` 是计数桩，`BufferGraphics` 有软件光栅化度量；像素断言要用与布局同口径的后端。
- **DLL**：`SDL2.dll`、`SDL2_ttf.dll`（及 `zlib1.dll`）放工程根 `libs/`（本仓库已备），或系统可搜索到。

## 扩展点

| 想做的事 | 怎么做 | 相关文件 |
|---|---|---|
| 新增自定义标签 | `engine.CustomElements().Define(tag, proto)`，解析时 `CloneElement` 克隆原型 | `engine/element_registry.go`, `engine/custom_elements.go` |
| 新增 CSS 属性 | 属性解析（css 解析器）→ 样式级联字段 → 布局/绘制消费处，三处都要改 | `engine/`（css_*.go、style/cascade、layout、paint） |
| 新渲染后端 | 实现 `core.Graphics` + `core.Viewport`（可选 `Clipper`/`SpriteDrawer`），不 import engine | 参照 `backend/sdl2`、`engine` 内 `BufferGraphics`/`FakeGraphics` |
| 新平台能力 | `runtime/xxx` 收口 + `backend/xxx` 平台实现，非本平台返回 `ErrUnsupported` | 参照 `runtime/dialog` + `backend/dialog` |
| 页面导航回调 | `SetOnNavigate`（导航在 click 冒泡之后执行） | `engine` 文档 API |
| 应用层变更信号 | `SetOnChanged`（内容**或视口尺寸**变化都通知） | `engine` 文档 API |
| 高频输入按帧合并（拖动窗口等） | `browser.Window.SetFrameHandler`：事件排空后每帧调一次，返回 true 时主循环改用 1ms 轮询（空闲 10ms） | `browser/window.go`、`examples/snip/preview.go` |

## 无头模式与测试

- `-dump-svg` 导出布局结果为 SVG（无头跑通整套管线，见 `examples/ua`）。
- `engine.NewHeadlessViewport(w, h)`：无渲染的视口，配 `engine.LayoutDocument(doc)` + `engine.RenderNode(g, doc)`。
- `engine.NewBufferedViewport(w, h)` 返回 `(vp, buf *BufferGraphics)`：软件光栅化到 RGBA 缓冲，`buf.DumpPNG(...)` 落盘仅缓冲快照（不布局不绘制），CI 里像素级断言（用例见 `ui/theme_test.go`、`examples/kit`）。
- 测试组织：包内 `_test.go` 就近测试；架构护栏 `core/arch_test.go`；主循环端到端在 `browser/run_*_test.go`。
- 验证改动：`go build ./...` + `go test ./...`（任意平台可跑，SDL 测试在无非 Windows 会跳过/提示）。

## 命名与代码风格惯例

- 平台不支持统一返回哨兵错误 `ErrUnsupported`（非 Windows 路径），不要 panic、不要静默。
- 平台文件用构建标签分文件（Windows 实现 / 其它平台 stub），保证跨平台 `go build ./...` 通过。
- 引擎对外 API 通过 `engine` 的类型别名透出 `core` 类型；新增跨层值类型放 `core`，`engine/graphics.go` 加别名。
- 可选能力用「类型断言 + 回退」而非配置开关（`Clipper`、`SpriteDrawer` 模式）。
- 回调注册一律 `SetOnXxx` / `OnXxx`；文档状态读写 `Xxx()` / `SetXxx()`。
- 注释与文档为中文；解释「为什么」多于「是什么」（口径、坑、实测数据）。
- 性能敏感改动要在 README「性能实测」章节留口径一致的前后对比数据。

## 示例索引（examples/，各自可独立运行）

| 目录 | 内容 |
|---|---|
| `examples/browser` | 最小浏览器门面：`browser.New` + `OpenDocument` + `Run` |
| `examples/notepad` | 记事本：`ui.Install` + 主题 + TextAreaView/BarView/TextInputView + Ctrl+S 对话框 |
| `examples/kit` | 组件画廊：表单控件全家桶 + `NewBufferedViewport` 截图 |
| `examples/portal` | 门户页：定位层/层叠/滚动性能基准页 |
| `examples/ua` | UA 样式表对照：`NewHeadlessViewport` + `-dump-svg` |
| `examples/stylebook` | 作者样式表全集：`<link>` 外链样式表（`docOptions` 加载器回调）+ 全标签/CSS 对照页（对照 Chrome） |
| `examples/snip` | 截图小工具：`runtime/screen` 桌面抓屏 |
| `examples/iconsheet` | ui 内置图标表：`ui.Icons()` 全量展示 + 搜索过滤 |
