# Ghromex

用 HTML/CSS 写原生桌面 GUI 的迷你 WebView 框架。

用 Go 实现一个小型渲染引擎：解析 HTML/CSS → 级联计算样式 → 盒式布局 → 绘制指令交给渲染后端（当前为 SDL2，纯动态加载，**无需 CGO**）。渲染 1 CSS px = 1 物理像素，窗口尺寸即视口尺寸，天然适合桌面小部件与内部工具界面。

开发从 `go doc github.com/go-comm/ghromex`（模块总览与接口导航）和 [AGENTS.md](AGENTS.md) 入手。
本文只留门面（定位 / 特性 / 环境 / 上手 / 最小示例），细节按文末[文档地图](#文档地图)进分册，不要整读。

## 特性

- 纯 Go 渲染引擎：HTML 解析、CSS 级联、选择器（标签 / `.class` / `#id` / 简单组合，含特异度）、块级与行内（inline / inline-block）布局、文本按词/按字换行
- 行内排版：`line-height`（normal / 倍数 / 百分比 / px；行距一半在文字之上、一半在之下，行盒高度严格等于声明值，倍数与百分比相对元素自身字号解析）、`vertical-align`（baseline / top / middle / bottom / text-top / text-bottom / sub / super / 长度 / 百分比；行内盒相对基线摆位，`<sup>`/`<sub>` 走同一条 UA 规则）
- 伪类：`:hover` / `:focus` / `:active` / `:checked` / `:disabled` / `:enabled` / `:first-child` / `:last-child` / `:only-child` / `:root` / `:empty` / `:not()` / `:nth-child()`；动态伪类的状态存在文档上（悬停/按下/焦点），状态变化触发重排重绘 → 级联重算，样式随之更新；`:hover`/`:active` 沿父链继承（鼠标停在子元素上时祖先同样匹配）
- 定位与层叠：`position: relative / absolute / fixed`、`top/right/bottom/left`（含负值）、`z-index`；脱流盒第二遍布局，包含块 = 最近非 static 祖先 padding box / 视口；绘制分常规层 + 定位层（z 升序，同 z 按文档序），`z-index` 非 0 的定位元素与 `fixed` 元素建立层叠上下文，内部定位子树先局部排序再整体落层
- 原生文件对话框：Windows Shell 的 `IFileDialog`（Vista 起的现代对话框），支持打开文件、打开多个文件、选择文件夹、另存为，带文件类型过滤与初始目录；纯 Win32 COM 调用，无 CGO、非 Windows 返回 `ErrUnsupported`
- 原生消息框：user32 的 `MessageBoxW`（信息/警告/错误/询问四种图标，OK、确定-取消、是-否、是-否-取消四种按钮组合），不走 COM、不需要 STA
- 可编辑输入：点击聚焦（蓝框 + 按点击位置落光标）、键入文字（含中文输入法）、Backspace / Delete 删除、鼠标拖选 / Ctrl+A 全选、Ctrl+C 复制选区（后端写系统剪贴板）、Ctrl+V 粘贴（后端读系统剪贴板；单行 input 粘贴多行文本折成单行，textarea 保留换行）、方向键 / Home / End 移动光标、Shift+方向键 / Shift+Home/End 扩展选区（锚点不动、活动端随按键移动，越过锚点后选区反向）、输入与退格替换选区、Tab 循环切换（text / password / textarea；password 可聚焦可输入，值全程按 ● 掩码，光标与点击定位同掩码口径，明文不落画不进 SVG，但 Ctrl+C 复制真实值）；`<textarea>` 多行编辑（折行显示、Enter 换行、跨行拖选、超出盒高的行不绘制、纵向滚动跟随光标并支持滚轮）；其余 input type 按 UA 外观区分渲染：radio/checkbox 13x13 控件、submit/reset/button 按钮外观（value 居中）
- 页面文本可选中：普通 HTML 文本（`<p>` / `<div>` / 标题 / 列表 / 表格单元格等，不含 input/textarea）用鼠标拖选，选区以半透明蓝高亮、`Ctrl+C` 复制（跨段落/列表项/单元格自动补换行）。拖选是**文档级选区**（位置 = 文本节点 + 流文本 rune 偏移），只记重绘账、不触发重排；与输入框选区互斥——点页面文本让输入框失焦并清页面选区，点/拖输入框或在其内键入、粘贴、Ctrl+A 时清页面选区。高亮区间在更新选区时预算到文本节点上，渲染与 SVG 导出直接读，不做全文档遍历
- 表单与链接控件：`<select>`/`<option>` 下拉（点击展开选项浮层，点选项选中并派发 `change`，下方放不下且上方放得下时整组上移，浮层压过流内容与定位层、命中测试同层序）；`<button>` 内容按 HTML 规范垂直居中（Chrome/Edge 同款，定高按钮尤其明显）、`<a href>` 链接（UA 蓝色 + 下划线，`SetOnNavigate` 注册导航回调，导航在 click 冒泡之后执行）
- 勾选控件：`<input type=radio|checkbox>` 点击切换选中（`checked` 布尔属性按存在性判定，API `IsChecked` / `SetChecked`）；选中态绘制强调色方块 + 白色对勾、强调色圆环 + 实心圆（Chrome 口径），radio 盒恒为正圆；radio 仅 name 非空且相同的项互斥（实测无 name / `name=""` 各自独立）；点 label（包裹其控件或 `for` 指向）等效点该控件
- DOM 式 API：`QuerySelector` / `GetBoundingClientRect` / `SetText` / `OnClick` / `OnKeyDown` / `IsChecked` / `SetChecked` 等；内容变化自动触发重排重绘。`OnKeyDown` 注册**元素级按键处理器**（派发给聚焦元素，处理器返回 true 表示已消费、引擎的默认动作不再执行，等价网页的 preventDefault）——浏览器地址栏「回车打开」、记事本查找框「回车查找」都靠它接，不必改引擎的默认动作；伪类状态也可直接读写：`SetHoveredElement` / `HoveredElement` / `SetActiveElement` / `ActiveElement` / `OnDocumentMouseMove`（自建主循环的应用层用它驱动 `:hover`/`:active`，`browser` 门面已自动接好）
- 应用层底座：`SetOnChanged` 变更回调（内容或**视口尺寸**变化都会通知——窗口缩放不改文档内容，只靠内容变更收不到信号）；失焦只停画、**不丢编辑状态**（光标与选区保留，页内菜单动作可作用于失焦的编辑区）；`CaretPosition` 查光标行列；`clipboard.Text` / `clipboard.SetText`（`runtime/clipboard`）读写系统剪贴板；`browser.RequestClose` 关窗
- 事件冒泡：命中测试找到最深元素，沿父链依次派发 `click` / `change` / `wheel`，随后执行引擎默认动作（select 展开/选中、勾选切换、链接导航）
- 滚动与裁剪：`overflow(visible/hidden/clip/scroll/auto，单值两轴、两值 x y、overflow-x/overflow-y 分轴)` 滚动容器（滚轮驱动、子树平移 + padding 盒裁剪，被裁内容不可命中）、**圆角裁剪按 border-radius 的形状**（`overflow:hidden` 的圆角盒，溢出到角外的子元素按圆角裁掉，角上透出盒后底色）、文档级页面滚动（根元素以视口为滚动盒，长页面/宽页面均可滚）、`wheel` 事件（`OnWheel` 注册、`MouseEvent.DeltaX/DeltaY` 像素位移、冒泡派发）、Shift+滚轮 转横向滚动、textarea 滚轮滚动（与光标跟随同夹紧口径，全量吸收纵向滚轮、撞边界不带动页面）；裁剪经 `Graphics` 可选 `Clipper`（`PushClip`/`PopClip`，逐层求交）落地：`BufferGraphics` 像素断言、SDL2 `SDL_RenderSetClipRect`，SVG 导出同步生成 `clipPath`
- 图片 `<img>`：替换元素布局（尺寸按 CSS width/height → HTML width/height 属性 → 固有纵横比 → 固有像素推导，行内基线取盒底）、解码 PNG / JPEG / GIF（GIF 取第一帧）、绘制走 `Graphics.DrawImage`（后端缩放、alpha 混合，`opacity` 与父级 `overflow` 裁剪同时生效）、SVG 导出为内嵌 `data:image/png` 的 `<image>`。字节来源由宿主注入 `DocumentOptions.LoadImage`（`src` 已按 `BaseURL` 解析成绝对地址，worker goroutine 里异步加载 + 解码，到货后自动重排一次），引擎内建 `data:` URL（base64 / 百分号转义）同步解码以保证首帧可见；同一 `src` 只加载一次，纹理按位图指针上传一次并受 LRU 约束
- 窗口/任务栏图标：`browser.Options.Icon` 建窗即设、`Window.SetIcon` 运行期换，标题栏 + 任务栏 + Alt+Tab 一起变（走 `SDL_SetWindowIcon`，SDL 内部按 `SM_CXICON`/`SM_CXSMICON` 两档缩放后一次设好大小两档，不会出现「标题栏换了任务栏没换」）；位图输入为 `*core.Bitmap`（RGBA8888 非预乘，与页面图片同一口径），PNG/JPEG/GIF 字节用 `engine.DecodeImage` 解码（与 `<img>` 同一路径）；`ui.Spec.Icon` 让组件库外壳也带图标。纯装饰：非 Windows 返回 `ErrUnsupported`、老 SDL 缺符号也只是图标不生效，都不阻断建窗
- 可插拔图形后端：`engine.Graphics` 接口（DrawText / DrawColor / DrawImage / MeasureText），内置 `FakeGraphics`（计数）、`BufferGraphics`（软件光栅化到 RGBA 缓冲，可 `DumpPNG`）供无显示器测试；圆角抗锯齿走**角标精灵**（可选扩展 `SpriteDrawer`：整块圆角一次 blit，替掉逐像素的半透明小方块）——绘制调用数是 GPU 路径上的主要开销，portal 页一帧从 31840 次降到 2500 次、单帧绘制 149ms → 8.7ms（软件渲染器实测），滚动动画才不会被切成几帧；精灵的像素（引擎侧）与纹理（后端侧）都按「条数 + 像素」双上限、LRU 淘汰——键里含颜色，页面逐帧改色（动画）会不断产生新精灵，不设上限就是内存/显存泄漏；**填充率**同样要命：SDL 的绘制混合模式一旦开着，每块不透明填充都走逐像素混合（软件光栅器 2.1ns/像素，直接写像素只要 0.2ns），门户页一帧要填 1700 万像素——2560×1440 的窗体下单帧 37ms，**最大化后滚动只剩 27fps**（小窗体看不出来）。按填充的 alpha 临时切换混合模式（画完复原，贴图拷贝还要混合）后，门户页整帧（软件光栅器，含呈现）：800×600 1.35ms、1280×820 1.95ms、1920×1080 2.84ms、2560×1440 **4.07ms**（原 37ms，9 倍）、3840×2160 7.11ms；画面逐像素不变。清屏也不再重复铺白底（画布背景不透明时白底那层完全被盖住）。**诊断**：`GHROMEX_DEBUG=1` 会打印渲染路径与驱动名（默认 `renderer=software (driver=software; 默认内存渲染)`；`GHROMEX_GPU=1` 时 `renderer=accelerated(GPU, driver=direct3d; GHROMEX_GPU=1)`）——软件路径的瓶颈是填充率，GPU 路径通常在纹理上传/draw call/swap 间隔（GPU 路径实测：1280×820 到 4K 的整帧都是 16.6ms = vsync 周期，稳在 60fps；其中 GPU 真实工作只有 1.3~3ms，其余 13.5~14.5ms 是等 vsync）。**这些数字的口径与分段拆解见[docs/performance.md](docs/performance.md)**——不同口径的数不能并排比
- 元素注册表：`CustomElements().Define(tag, proto)` + `CloneElement` 注册自定义标签，解析时克隆原型生成独立实例
- 无头运行：`-dump-svg` 导出布局结果；`NewBufferedViewport` / `OpenBufferedDocument`（视口 + 软件光栅化缓冲）可在 CI 中像素级断言渲染结果；真实窗口的最终观感用 `Window.Screenshot()` 回读窗口像素（应用自行编码 PNG，见 `examples/snip`）

## 环境要求

- Go 1.20+（go.mod 基线，作库供其他项目引用，1.20 以上工具链均可构建；无需 CGO）
- 渲染后端目前提供 Windows 实现（`backend/sdl2`，加载 `SDL2.dll` 2.28 与 `SDL2_ttf.dll`）
- 把 `SDL2.dll`、`SDL2_ttf.dll`（及 `zlib1.dll`）放到工程根目录 `libs/`，或保证系统可搜索到它们
- 平台边界：`engine` 包是纯 Go、可移植的；`backend/sdl2` 是渲染后端——Windows 提供 SDL2 实现，其它平台 `NewWindow` 返回 `ErrUnsupported`；`runtime/` 是平台能力的通用调用入口（`runtime/dialog` 原生对话框、`runtime/clipboard` 系统剪贴板、`runtime/screen` 桌面抓屏），平台实现在 `backend/dialog`、`backend/clipboard`、`runtime/screen` 内部；Windows 用纯 Win32 / SDL2 调用实现，其它平台返回 `ErrUnsupported` 或空操作；`browser` 与 `examples/` 都不带构建标签，任意平台 `go build ./...` / `go test ./...` 都能通过，非 Windows 启动窗口时会打印明确提示
- 依赖方向：`core`（契约：Graphics/Viewport/Paint/Color…，叶子包）← `engine` 与 `backend/*` 各自向上依赖它，再由 `browser`（应用层：窗口 + 文档 + 主循环）把两者缝起来。平台层不 import 引擎，`core/arch_test.go` 把这条规则钉死

## 快速开始

示例按目录存放在 `examples/` 下。每个目录都是一个**可独立运行**的小程序
（页面可能是内嵌 HTML、HTML 资产或由 Go 构造，从任意工作目录运行都不
依赖当前路径）：

| 目录 | 运行 | 内容 |
| --- | --- | --- |
| `examples/ua` | `go run ./examples/ua` | 原生 UA 表单 + 常用标签：零作者 CSS，可与 Chrome/Edge 直接对照 |
| `examples/stylebook` | `go run ./examples/stylebook` | 样式全集：作者样式表（`<link>` 外链 + `go:embed`）逐节摆出引擎支持的全部标签与 CSS，每节带「支持/部分/不支持」徽标与边界注记；同一 `index.html` 可直接丢进 Chrome 对照 |
| `examples/browser` | `go run ./examples/browser` | 迷你本地浏览器：顶部常驻地址栏与后退/前进/刷新/首页按钮、底部常驻状态栏（页标题 + 说明，换页后仍在），地址栏可打开本地路径（相对路径/`file://`），支持 `about:home`/`about:back`/`about:forward`，外部链接与缺失文件显示提示页；需在仓库内运行 |
| `examples/portal` | `go run ./examples/portal` | 购物门户：fixed 工具条/品牌头/导航、层叠上下文、分类栏、秒杀与商品网格、搜索提示和购物车计数交互 |
| `examples/notepad` | `go run ./examples/notepad` | 仿 Windows 记事本：页面由 Go 构造 DOM，无 `index.html`；`ui` 菜单（菜单项带图标）/纯图标工具栏/状态栏（文件/编辑/格式/视图/帮助，新建/打开/保存/另存为/撤销/重做）、原生「打开/另存为/选择文件夹」对话框、查找替换栏、铺满窗口的多行编辑区、状态栏（文件名/行列/字符数）；`-file` 启动时打开文件 |
| `examples/snip` | `go run ./examples/snip` | 截图工具：冻结主屏拖选，松开后同一窗口原地缩成置顶无边框的悬浮预览（图片区即选区大小），拖动图片移动窗口，Ctrl+S 保存 PNG，Esc/关闭退出；`-delay` 留时间整理窗口 |

每个示例都支持 `-exit-after`（指定时长后自动退出，调试/CI）；出图按用途分两类，别混用：

```powershell
# 分析（无头，不需要显示器）
go run ./examples/ua -exit-after 3s     # 指定时长后自动退出（调试/CI）
go run ./examples/ua -dump-svg out.svg  # 级联+布局导出 SVG（真实 <text>），核对几何/层级/配色
go run ./examples/kit -dump-png out.png # 软件光栅 PNG（文字是占位方块），断言布局/配色
# 截图（真实窗口 + 真实字体；dummy 驱动可无显示器跑）
go run ./examples/kit -shot out.png
```

`browser/doc.go` 的「出图口径」小节有完整分工说明（分析用 dump-svg / dump-png，快照存档用 `BufferGraphics.DumpPNG`，截图用 `Window.Screenshot`）。

`examples/snip` 另有截图专用参数：`-source` 用一张图片代替真实桌面，`-shot-overlay`/`-shot-preview` 把选区页、预览页各存成 PNG 后退出（回读走 `Window.Screenshot`）；配合 `SDL_VIDEO_DRIVER=dummy` 即可在无显示器环境检查观感。

```powershell
go run ./examples/snip -source examples/kit/screenshot.png -shot-overlay overlay.png -shot-preview preview.png
```

预览窗口的拖动走 `browser.Window.SetFrameHandler`：鼠标事件只记目标位置，帧末合并成一次 `MoveTo`，拖动期间主循环以 1ms 轮询（空闲时 10ms）——逐事件移动窗口 + 10ms 休眠会把窗口位置量化成 10~20ms 一跳，看起来一顿一顿。选区遮罩只压暗 30%，底层内容仍然看得清。

交互示例：`examples/browser` 用 `SetOnNavigate` 接管链接点击：读取目标 HTML 并在同一窗口重新 `OpenDocument`，外壳用 `ui.Install` 装配——后退/前进/刷新/首页是带图标与可用态的 `ui.Tool`，示例页面下拉和地址栏是嵌在工具栏里的 `ui.Slot` 控件，状态栏显示页面标题（引擎没有独立的窗口 chrome 层，换页时重装一套工作台；页面靠 `Spec.NoBodyReset` 保住自己的 body 排版），浏览历史支持后退/前进/刷新/首页（没有可后退的页面时按钮置灰），地址栏里按回车直接打开当前输入的地址，未知协议与读取失败会生成带返回链接的提示页；`examples/portal` 用固定顶栏 + 绝对定位侧栏组成购物门户，搜索框聚焦后显示热门提示，所有「加入购物车」按钮共享一个计数徽标。

`examples/notepad` 是最"应用"的一个：菜单、工具栏、快捷键和状态栏交给 `ui`（`ui.Install` 生成工作台，`ui.Spec` 声明命令），示例只保留记事本自己的部分——编辑区几何由程序按窗口尺寸算出（CSS 没有 `calc`，固定栏高度取 `ui.Theme`），查找替换栏与撤销/重做栈由应用层维护（引擎不内置撤销），文件读写与剪贴板走平台能力（`runtime/dialog` 的原生文件对话框 / `runtime/clipboard` 的系统剪贴板）。`ui.Workbench.SetStatusVisible` 支撑「视图 → 显示状态栏」，快捷键由 `Workbench.HandleKey` 接到窗口。

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
// 只做布局（不需要像素）：FakeGraphics 后端，绘制指令只计数
vp := engine.NewHeadlessViewport(400, 300)
doc, _ := engine.OpenDocument(vp, `<html><body><div class="box">hi</div></body></html>`)
```

要出图 / 逐像素断言像素时，用带缓冲的视口——视口与缓冲同尺寸，一次建好：

```go
// 只要视口：vp, buf := engine.NewBufferedViewport(400, 300)
buf, doc, err := engine.OpenBufferedDocument(400, 300,
	`<html><body><div class="box">hi</div></body></html>`)
if err != nil {
	panic(err)
}
_ = buf.DumpPNG("out.png") // OpenBufferedDocument 已画过一帧；或直接 buf.ColorAt(x, y) 断言像素
```

### 原生对话框

Windows 上可弹系统原生的文件对话框（打开文件 / 打开多个文件 / 选择文件夹 /
另存为），外观与资源管理器一致，支持文件类型过滤与多选：

```go
win, _ := browser.New(900, 620, "记事本")
paths, err := win.OpenFileDialog(browser.DialogOptions{
	Title:      "打开",
	DefaultDir: `C:\Users\me\Documents`,
	Filters: []browser.FileFilter{
		{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
		{Name: "所有文件 (*.*)", Pattern: "*.*"},
	},
})
// 用户取消时 paths 为空、err 为 nil；非 Windows 平台 err 为 browser.ErrUnsupported。

// 一次选多个文件用 OpenMultipleFilesDialog（返回多个路径）。
many, err := win.OpenMultipleFilesDialog(browser.DialogOptions{Title: "批量打开"})
```

还有原生消息框（提示 / 警告 / 询问），走 user32 的 `MessageBoxW`：

```go
res, err := win.MessageDialog(browser.MessageOptions{
	Title:   "确认",
	Message: "文件已存在，要覆盖吗？",
	Icon:    browser.IconQuestion,
	Buttons: browser.ButtonsYesNo,
})
if err == nil && res == browser.ResultYes {
	// 用户点了「是」
}
```

对话框是模态的：调用期间阻塞当前线程（父窗口的 SDL 事件循环随之暂停），
在按钮/菜单的回调里直接调用即可。文件对话框走 Shell 的 `IFileDialog`（纯
Win32 COM 调用，无 CGO），消息框走 user32 的 `MessageBoxW`（不需要 COM、
不需要 STA），细节见 `runtime/dialog/dialog.go`。

## 文档地图

| 想知道 | 去哪看 |
| --- | --- |
| 某个包 / 类型 / 函数怎么用 | `go doc github.com/go-comm/ghromex/<包>[ <符号>]`；网页版 [pkg.go.dev/github.com/go-comm/ghromex](https://pkg.go.dev/github.com/go-comm/ghromex) |
| 模块定位、分层、必守不变量 | `go doc github.com/go-comm/ghromex`（根包 `ghromex.go`） |
| 架构与主循环、扩展点、坑与惯例 | [docs/ai/architecture.md](docs/ai/architecture.md) |
| AI 开发指引入口 | [AGENTS.md](AGENTS.md) |
| 目录树与各包职责 | [docs/project-structure.md](docs/project-structure.md) |
| 组件库 ui（菜单 / 工具栏 / 状态栏 / 主题 / 图标） | [docs/ui.md](docs/ui.md) |
| 支持哪些 CSS（属性、值、UA 默认样式、字体口径） | [docs/css-support.md](docs/css-support.md) |
| 哪里和浏览器不一样 | [docs/limitations.md](docs/limitations.md) |
| 性能数字怎么量、怎么读 | [docs/performance.md](docs/performance.md) |
| 调试环境变量、跑测试 | [docs/development.md](docs/development.md) |
| 平台能力怎么分层 | [runtime/README.md](runtime/README.md) |
| 依赖的第三方 DLL 版本与许可 | [libs/README.md](libs/README.md) |
| 设计决策的来龙去脉 | [docs/rfc/](docs/rfc/) |

## 更新日志

当前版本 **v0.6.0**，逐版本变更见 [CHANGELOG.md](CHANGELOG.md)。
