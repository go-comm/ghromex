# ghromex 架构地图（AI 上下文）

> 本文件与根包的 `go doc github.com/go-comm/ghromex`（模块总览与接口导航）共同构成 AI 开发上下文。入口见根目录 `AGENTS.md`。

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
| `core` | 引擎与后端之间的契约：`Graphics`、`Viewport`、`Color`/`Size`/`Paint`/`Sprite`/`Bitmap` | 叶子包，谁都不依赖；可选扩展 `Clipper`、`SpriteDrawer`、`ImageBlitter`、`IconSetter` 靠类型断言启用；`Image` 是空接口，`Bitmap` 是 `<img>`、背景图与窗口图标的载荷 |
| `engine` | 解析 HTML/CSS → 级联 → 布局 → 绘制 → 命中测试/事件 | **纯 Go**，不 import 平台包；`Graphics = core.Graphics` 用类型别名保持 API（`engine/graphics.go`） |
| `engine/element_registry.go` | 元素注册表：`CustomElements().Define(tag, proto)`；内置标签走 `SystemElements()`（`html_elements.go`） | 解析时克隆原型（`CloneElement`）生成独立实例 |
| `backend/sdl2` | Windows SDL2 窗口/渲染器/字体纹理/事件解码 | **不认识 engine**；动态加载 DLL，无 CGO；`NewWindow → Run → Close` 同一 OS 线程（`LockOSThread`） |
| `backend/clipboard` `backend/dialog` | 剪贴板复用 SDL2；原生对话框走 Win32 COM + user32 | 仅 Windows，其它平台返回 `ErrUnsupported` |
| `runtime/{clipboard,dialog,screen}` | 上述平台能力的通用调用入口（类型别名转发） | 调用方只 import 这一层，不碰 backend |
| `browser` | 装配门面：持有 `HTMLDocument`，跑主循环（事件 → 引擎动作 → 重排 → 重绘） | 生命周期同一 goroutine；把平台差异挡在应用之外 |
| `ui` | 菜单栏/工具栏/状态栏 + 页面控件（TextInputView/TextAreaView/BarView）+ 浅色/深色主题 + 快捷键 | 装在文档 DOM 里（`position:fixed` + body 占位块），换文档要重装 |
| `debug` | 本地调试服务：HTTP/JSON 注入输入 + 观测状态（`Serve`/`ServeFromEnv`） | **可选开发期外挂**，只被示例/宿主显式引用；`engine`/`browser`/`ui` 都不许 import 它（`TestDebugIsNotADependency`） |

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

// IconSetter —— Viewport 的**可选扩展**：把一张位图设为窗口图标（标题栏 + 任务栏 + Alt+Tab）。
// 不实现的后端（无头、缓冲渲染）静默忽略。返回 error 表示当前平台或 SDL 运行库
// 没有这条能力（非 Windows 返回 `sdl2.ErrUnsupported`）——图标纯属外观，调用方一般
// 只需忽略错误，不该因此让应用起不来。
// 位图口径与 Bitmap 相同（RGBA8888、非预乘 alpha）。建议给 32×32 或 64×64：
// 太小会被系统放得模糊，太大平台层要缩放。
type IconSetter interface {
    SetIcon(icon *Bitmap) error
}
```

- 引擎只认这两个接口 → SDL2、软件光栅化（`BufferGraphics`）、无头（`FakeGraphics`）跑同一套布局。
- `MeasureText` 与 `DrawText` **必须同一字体解析规则**，否则混排宽度错位。
- 可选扩展（实现了就用，没实现回退到等价但更慢的路径）：`Clipper` 裁剪栈、`SpriteDrawer` 位图 blit、`ImageBlitter` **按源矩形** blit（`BlitImage(dst, src)`，背景图平铺时只画可见的那一块）、`IconSetter` 窗口图标（这个是「纯装饰」，不实现是**静默忽略**而非回退）。
- `DrawImage` 的载荷是 `core.Image`（空接口）：引擎传 `*core.Bitmap`（RGBA8888、非预乘 alpha、`Pix` 解码后不再改），后端用类型断言识别；不认识的值（含 `nil`）按占位/跳过处理——新增图片类型不必改接口。

## 主循环与事件模型

`browser.Window.Run()` 每帧：

1. `PollEvent` 取平台事件（SDL 解码成 `sdl2.Event`；`PollEvent` 自身先排空外部拖入的文件批），本帧把队列取空
2. `drainTasks` 跑主线程任务队列（`Window.Post` / `Window.Do`）——**调试服务的读状态、截图、改 DOM、注入输入的「落地」都在这里**，排空点刻意在事件之后：那时本帧收到的事件还没派发，队列里排着的正是「上一动作注入的事件」，跑完即等价于「上一动作已生效」
3. 平台事件 → 引擎动作：`OnDocumentClick` / `OnDocumentKeyDownMods` / `OnDocumentWheel` / `OnDocumentMouseMove`…（命中测试取最深元素 → 沿父链冒泡 → 执行默认动作）
4. `doc.ChangeCount()` 变了才 `LayoutDocument`（变更计数是**唯一**重排触发器，布局期禁止 `markChanged`）
5. `engine.RenderNode(g, doc)` 绘制 + `Present`
6. `engine.TickScroll` / `engine.TickTooltip` 推进滚动动画与提示气泡（决定本帧是否休眠）；队列非空时本帧不休眠，让一次请求往返落在下几帧内

## 主线程任务队列（`browser/tasks.go`）

SDL 的窗口/渲染器绑在 `Run` 所在 OS 线程，引擎文档也不接受跨线程访问。别的
goroutine（`debug` 的 HTTP handler、宿主的后台 worker）要对窗口做事，只能：

- `win.Post(func())` —— 把任务排进队列，**不等待**（任务里 panic 只打一行诊断）；
- `win.Do(func() (any, error), timeout)` —— 排队 + 阻塞等结果（任务 panic 被收敛成 error）；**只能在非主循环 goroutine 上调**，同 goroutine 调等于让主循环等自己；
- `win.Running()` —— 主循环是否在跑（竞态口径：读到 true 之后可能已退出，只适合观测）。

`Run` 未跑/已退出时 `Post`/`Do` 立刻返回 `ErrLoopNotRunning`（检查与 `stopLoop` 在同一把锁里，不存在「刚要执行、主循环刚好退出」的夹缝）。超时返回 `ErrTaskTimeout`（任务仍在队列里，之后可能照跑——所以闭包要么幂等要么只读）；主循环在任务执行前退出则等待方拿到 `ErrLoopStopped`，不是干等。`Close` 先 `stopLoop` 再结清队列。

`win.Document()` 直接返回当前文档（不排队）、`win.ScreenshotPNG()` 内部走 `Do`——两者都**必须**在主循环 goroutine 上用。


三个「状态存在文档上」的伪类：`:hover` / `:focus` / `:active`（`SetHoveredElement` / `SetFocusedElement` / `SetActiveElement`），变化走同一条 `markChanged → 重排重绘` 链路。

**文件拖放（拖入窗口）不走 SDL 事件队列**：`WM_DROPFILES` 由后端窗口过程截获（`backend/sdl2/filedrop.go`，子类化 + `DragAcceptFiles`），`PollEvent` 先把待处理的整批路径交成 `sdl2.EventFilesDropped`；browser 在事件阶段调 `SetFileDropHandler`（一次拖放一批路径，多选不拆分）。方向只有「拖入」：拖出是 OLE 源端 `DoDragDrop`，不属于这里。

**两套选区模型**：可编辑控件（input/textarea）的选区在元素自身（`caret` / `selAnchor`，value 的 rune 下标，见 `selection.go` / `text_input.go`）；普通页面文本用**文档级选区**（`flow_selection.go`），位置 = `(文本节点, 该节点「流文本」的 rune 偏移)`，流文本 = 节点 `runs` 文本按序拼接（按 run 下标存会在重排后错位）。更新选区时把覆盖区间预算到文本节点上（`flowSelLo/flowSelHi`），渲染与 SVG 只读，不做全文档遍历；拖选只 `markPaintChanged`。`SelectedText` / `SelectionLength` 优先返回文档级选区，`browser` 的 Ctrl+C 与状态栏因此对页面文字同样有效。两套互斥：点页面文本让输入框失焦并清页面选区；输入框拖选/键入/粘贴/Ctrl+A 时清页面选区。

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

**图片（`<img>` 与 `background-image`）**——同一个选项结构，但加载是异步的；`src` 到加载器时已是绝对地址，`data:` URL 由引擎自解：

```go
doc, _ := win.OpenDocumentWith(src, engine.DocumentOptions{
    BaseURL:   "file:///C:/pages/index.html",
    LoadImage: loadBytes, // func(src string) ([]byte, error)，在 worker goroutine 里调用
})
```

链路：布局前 `resolveImages` 给每个 `<img>` 绑定位图（未到货为 `nil`，盒 0×0）→
worker 调加载器 + `image.Decode`（PNG/JPEG/GIF 首帧），在 `imgMu` 内把位图写回
`images[src]` 并给 `imgReady` 原子计数 +1 → 主 goroutine 的
`PendingChange`/`LayoutDocument` 用 `drainImages` 把计数换算成一次 `changed` 增长 →
重排按固有尺寸定盒、`renderNode` 调 `DrawImage`。**worker 绝不碰文档/Graphics**
（SDL 要求渲染状态单线程），也不向 channel 投递（见取舍 6）。

**背景图**（`engine/background.go`）复用同一条加载通路，但记账不同：层里存的是
**未解析的原始引用**，绘制期才按文档 BaseURL 解析并绑定位图（级联期拿不到
BaseURL）。背景图不影响盒模型，因此到货只涨**重绘**计数（`drainBackgroundImages`
只加 `paintChanged`），不触发重排；`imageEntry` 用 `usedByLayout` / `usedByBackground`
两个标记把 `<img>` 与背景图的到货分别记账。绘制顺序是「背景色 → 背景图层 → 边框
→ 内容」，多层按 CSS 顺序叠放（首层在最上面）。

**窗口/任务栏图标**（`core.IconSetter`）——只吃**已解码的位图**，解码入口与 `<img>`
同一条路径（`engine.DecodeImage`），像素口径也一致（RGBA8888、非预乘 alpha）：

```go
// 手里有 PNG/JPEG/GIF 字节 → 位图 → 建窗即设（首帧前生效，避免闪默认图标）。
pngBytes, _ := os.ReadFile("icon.png")
icon, _ := engine.DecodeImage(pngBytes)
win, _ := browser.NewWithOptions(browser.Options{
    Width: 900, Height: 620, Title: "记事本",
    Centered: true, Resizable: true, Icon: icon, // nil 用系统默认
})
// 运行期换：
win.SetIcon(other) // 非 Windows 返回 ErrUnsupported；失败不阻断应用
```

`ui.Spec.Icon` 是同一份位图，给 `ui.New` 用（`Install` 不建窗、忽略它）。图标纯
装饰：老 SDL 缺符号、位图不合法都不让建窗失败，建窗路径上的失败只在 `GHROMEX_DEBUG=1`
打一行。走 `SDL_SetWindowIcon`（SDL 内部按 `SM_CXICON`/`SM_CXSMICON` 两档缩放，一次设
好标题栏/任务栏两档）——细节与坑见「关键设计取舍 9」与「容易踩的坑」。

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
5. **后端可选扩展**：`Clipper` / `SpriteDrawer` / `ImageBlitter` 接口检测，不实现就走等价但更慢的回退。
6. **异步图片只写缓存 + 原子计数回主线程**：`<img>` 的解码在 worker goroutine，结果在 `imgMu` 内写回 `images[src]` 并给 `imgReady` +1，由主循环的 `drainImages` 把计数换算成一次 `ChangeCount` 增长——文档状态与渲染状态仍然只有一个线程在碰（`PendingChange`/`LayoutDocument` 两个入口都会 drain）。刻意不用 channel 投递结果：宿主换文档又漏 `Close` 时，投递式 worker 会永久阻塞在发送上，把整份文档钉在内存里。
7. **平台能力下沉**：剪贴板、对话框、抓屏在 `runtime/*` 收口，`engine` 保持纯 Go（读剪贴板靠后端读出后调 `OnDocumentPaste`，写靠引擎给 `SelectedText` 后端去写）。图片同理：引擎只解码，字节由宿主注入的 `LoadImage` 提供，不读文件、不联网。
8. **UI 是文档内自绘**：`position:fixed` 压页面上、body 首尾插占位块；换文档重装。代价是页面 `z-index` 更高的元素会盖住工作台。
9. **窗口图标走 `SDL_SetWindowIcon`，不自己调 `WM_SETICON`**：SDL 的 Windows 实现内部按系统 `SM_CXICON` / `SM_CXSMICON` 两档缩放后 `CreateIconIndirect`，一次把 `ICON_SMALL` 和 `ICON_BIG` 都设好——自己只设一个档位会出现「标题栏换了、任务栏还是默认图标」的半截效果。
10. **文档级选区是绘制状态，不是布局状态**：拖选只记重绘账（`markPaintChanged`），命中与复制都在布局结果（`runs`）上做，大页面拖选不重排。代价是内容变更后旧偏移要按新流文本长度夹紧（见 `flowSelectionBounds` 的 `clampInt`），极端情况下贴到最近边界而不是原字符。

## 容易踩的坑 / 限制

- **仅 Windows**：SDL2 后端、原生对话框、剪贴板、抓屏都是 Windows 实现，其它平台返回 `ErrUnsupported`（`browser` 与 `examples/` 不带构建标签，任意平台 `go build ./...` / `go test ./...` 都过，非 Windows 启动窗口时打印提示）。
- **线程**：`NewWindow → OpenDocument → Run → Close` 必须同一 goroutine（SDL `LockOSThread` 要求，见 `backend/sdl2/doc.go`）。唯一的例外是 `<img>` 的加载 worker：它只调 `LoadImage`、解码、把位图写回缓存并给原子计数 +1——`LoadImage` 因此必须线程安全。
- **`<img>` 的两个「到货」时点**：`data:` URL 同步解码（首帧可见），其余异步（首帧 0×0，解码完成后自动重排一次）；没有加载器时非 `data:` 图片按空图处理，不会报错也不会重试（改了 `src` 才会重新登记）。无动画 GIF、无 `alt` 回退。
- **背景图只做 `url()`/`none`**：渐变等 `<image>`、`background-attachment: fixed`、`background-repeat` 的 `space`/`round` 铺法未实现（按 CSS 语义整条声明忽略）；圆角元素上的背景图仍是「角标补盖」近似，不是真正的圆角遮罩。
- **无 flex / grid / 过渡动画**，布局只有 block / inline / inline-block + relative/absolute/fixed；UI 主题靠 `inline-block` 和 1px 实体分隔线拼。
- **没有实时 `input` 事件**：实时联动用 `OnKeyDown`（元素级按键处理器，返回 true = 已消费，等价 preventDefault）或在命令执行时读 `Value`。
- **页面文本选区**是「最近块级祖先变化即换行」的复制口径：跨 `<p>`/`<li>`/单元格复制插 `\n`，同一块内直接拼；`<br>` 不产生文本节点，跨 `<br>` 复制不插换行。未实现双击选词/三击选行/拖选越出视口自动滚动；它只 `markPaintChanged`，不会让 `ChangeCount` 变化。
- **多窗口未实现**：API 塑形成「可多次 New」，底层基础待真实需求再补。
- **行高与 Chrome 可能差 1~4px**；LCD 子像素渲染默认关（灰度 AA），`GHROMEX_LCD=1` 开启；`GHROMEX_DEBUG=1` 打印渲染路径与驱动名。
- **文本度量口径**：`FakeGraphics` 是计数桩，`BufferGraphics` 有软件光栅化度量；像素断言要用与布局同口径的后端。
- **DLL**：`SDL2.dll`、`SDL2_ttf.dll`（及 `zlib1.dll`）放工程根 `libs/`（本仓库已备），或系统可搜索到。
- **SDL 鼠标按钮事件坐标缓存**：SDL2 的 `WM_LBUTTONDOWN/UP` 不读 `lParam`，直接取内部 `mouse->x/y`（仅 `WM_MOUSEMOVE` 刷新）。窗口移动/缩放但光标静止时，下一击携带的仍是**旧窗口位置相对**的坐标——snip「先点一下才拖得动」即此根因（见 `backend/sdl2/window.go:fixStale` 与 `.temp/mousecache` 实证）。
- **`SDL_CreateRGBSurface*From` 只借用像素、不拷贝**：窗口图标把 `*core.Bitmap` 的 `Pix` 包成 surface 时，surface 活多久压着的 Go 堆数组就得活多久——`runtime.KeepAlive(icon.Pix)` 必须落在 `SDL_FreeSurface` **之后**（放在之前等于没保活）。`SetWindowIcon` 会把像素拷进 Win32 `HICON`，所以 surface 用后即弃。三个图标符号（`SetWindowIcon`、`CreateRGBSurfaceWithFormatFrom`、`CreateRGBSurfaceFrom`）一律 `bindProbe` 可选绑定：缺了只是图标不可用，不该让整个后端加载失败。

## 扩展点

| 想做的事 | 怎么做 | 相关文件 |
|---|---|---|
| 新增自定义标签 | `engine.CustomElements().Define(tag, proto)`，解析时 `CloneElement` 克隆原型 | `engine/element_registry.go`, `engine/custom_elements.go` |
| 新增 CSS 属性 | 属性解析（css 解析器）→ 样式级联字段 → 布局/绘制消费处，三处都要改 | `engine/`（css_*.go、style/cascade、layout、paint） |
| 新渲染后端 | 实现 `core.Graphics` + `core.Viewport`（可选 `Clipper`/`SpriteDrawer`/`ImageBlitter`），不 import engine | 参照 `backend/sdl2`、`engine` 内 `BufferGraphics`/`FakeGraphics` |
| 新平台能力 | `runtime/xxx` 收口 + `backend/xxx` 平台实现，非本平台返回 `ErrUnsupported` | 参照 `runtime/dialog` + `backend/dialog` |
| 新图片字节来源（网络/资源包） | 打开文档时传 `DocumentOptions.LoadImage`（`func(src string) ([]byte, error)`，`src` 已按 `BaseURL` 解析；worker 并发调用，需线程安全） | `engine/image.go`, `engine/document.go` |
| 新图片格式 | 在 `engine/image.go` 里给 `image.Decode` 注册新解码器（`image.RegisterFormat` / 空白 import） | `engine/image.go` |
| 新的背景图层能力 | 在 `engine/background.go` 加属性解析（`applyBackground*Decl`）+ 几何（`imageSize`/`backgroundAxisOffset`/`backgroundBoxRect`）+ 绘制（`paintBackgroundImages`）；口径字节打进 `backgroundKey` 时记得换字典指针 | `engine/background.go` |
| 页面导航回调 | `SetOnNavigate`（导航在 click 冒泡之后执行） | `engine` 文档 API |
| 应用层变更信号 | `SetOnChanged`（内容**或视口尺寸**变化都通知） | `engine` 文档 API |
| 高频输入按帧合并（拖动窗口等） | `browser.Window.SetFrameHandler`：事件排空后每帧调一次，返回 true 时主循环改用 1ms 轮询（空闲 10ms） | `browser/window.go`、`examples/snip/preview.go` |
| 接收拖入窗口的文件 | `browser.Window.SetFileDropHandler`：一次拖放调一次，给整批绝对路径（多选不拆分）；后端 `WM_DROPFILES` → `sdl2.EventFilesDropped`。只做接收方向 | `backend/sdl2/filedrop.go`、`browser/window.go` |
| 自定义窗口图标 | 造一个 `*core.Bitmap`（`engine.DecodeImage` 解 PNG/JPEG/GIF 字节），传给 `browser.Options.Icon`（建窗即设）或 `browser.Window.SetIcon`（运行期换）；`ui.Spec.Icon` 同理。**只吃位图**，`.ico` 多帧容器要宿主先取出其中一档 | `core/graphics.go`（`IconSetter`）、`backend/sdl2/window.go`、`browser/window.go` |
| 从外部脚本驱动/观测窗口 | `debug.Serve(win, debug.Options{Addr:...})`（或 `ServeFromEnv` + `GHROMEX_DEBUG_PORT`）；输入走 `Window.InjectEvent`，读状态/截图走 `Window.Do`。别在生产代码里 import `debug` | `debug/server.go`、`debug/input.go`、`debug/state.go`、`examples/browser/main.go` |
| 别的 goroutine 想碰窗口/文档 | `browser.Window.Post`（不等）/ `browser.Window.Do`（等结果）交回主循环；`Run` 没跑时返回 `ErrLoopNotRunning`。`Run` 内每帧事件排空后执行 | `browser/tasks.go`、`browser/window.go`（`Run`） |

## 无头模式与测试

- `-dump-svg` 导出布局结果为 SVG（无头跑通整套管线，见 `examples/ua`）。
- `engine.NewHeadlessViewport(w, h)`：无渲染的视口，配 `engine.LayoutDocument(doc)` + `engine.RenderNode(g, doc)`。
- `engine.NewBufferedViewport(w, h)` 返回 `(vp, buf *BufferGraphics)`：软件光栅化到 RGBA 缓冲，`buf.DumpPNG(...)` 落盘仅缓冲快照（不布局不绘制），CI 里像素级断言（用例见 `ui/theme_test.go`、`examples/kit`）。
- 测试组织：包内 `_test.go` 就近测试；架构护栏 `core/arch_test.go`；主循环端到端在 `browser/run_*_test.go`。
- 验证改动：`go build ./...` + `go test ./...`（任意平台可跑，SDL 测试在无非 Windows 会跳过/提示）。

## 命名与代码风格惯例

- 平台不支持统一返回哨兵错误 `ErrUnsupported`（非 Windows 路径），不要 panic、不要静默。
- **跨线程碰窗口只走 `Post`/`Do`**：别的 goroutine 上「顺手调用一下窗口方法」是 bug——SDL 与文档树都要求单线程。读文档直接 `Do(func() (any, error) { return win.Document(), nil }, t)`，别自己 Export 状态。
- 平台文件用构建标签分文件（Windows 实现 / 其它平台 stub），保证跨平台 `go build ./...` 通过。
- 引擎对外 API 通过 `engine` 的类型别名透出 `core` 类型；新增跨层值类型放 `core`，`engine/graphics.go` 加别名。
- 可选能力用「类型断言 + 回退」而非配置开关（`Clipper`、`SpriteDrawer` 模式）。
- 回调注册一律 `SetOnXxx` / `OnXxx`；文档状态读写 `Xxx()` / `SetXxx()`。
- 注释与文档为中文；解释「为什么」多于「是什么」（口径、坑、实测数据）。
- 性能敏感改动要在 [docs/performance.md](../performance.md) 留口径一致的前后对比数据。

## 示例索引（examples/，各自可独立运行）

| 目录 | 内容 |
|---|---|
| `examples/browser` | 最小浏览器门面：`browser.New` + `OpenDocument` + `Run`；`-debug-port` / `GHROMEX_DEBUG_PORT` 挂调试服务 |
| `examples/notepad` | 记事本：`ui.Install` + 主题 + TextAreaView/BarView/TextInputView + Ctrl+S 对话框 |
| `examples/kit` | 组件画廊：表单控件全家桶 + `NewBufferedViewport` 截图 + 程序化窗口图标（`ui.Spec.Icon` / `-dump-icon` / `-no-icon`） |
| `examples/portal` | 门户页：定位层/层叠/滚动性能基准页 |
| `examples/ua` | UA 样式表对照：`NewHeadlessViewport` + `-dump-svg` |
| `examples/stylebook` | 作者样式表全集：`<link>` 外链样式表（`docOptions` 加载器回调）+ 全标签/CSS 对照页（对照 Chrome） |
| `examples/snip` | 截图小工具：`runtime/screen` 桌面抓屏 |
| `examples/iconsheet` | ui 内置图标表：`ui.Icons()` 全量展示 + 搜索过滤 |
