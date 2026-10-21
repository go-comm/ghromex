# Ghromex

用 HTML/CSS 写原生桌面 GUI 的迷你 WebView 框架。

用 Go 实现一个小型渲染引擎：解析 HTML/CSS → 级联计算样式 → 盒式布局 → 绘制指令交给渲染后端（当前为 SDL2，纯动态加载，**无需 CGO**）。渲染 1 CSS px = 1 物理像素，窗口尺寸即视口尺寸，天然适合桌面小部件与内部工具界面。

## 特性

- 纯 Go 渲染引擎：HTML 解析、CSS 级联、选择器（标签 / `.class` / `#id` / 简单组合，含特异度）、块级与行内（inline / inline-block）布局、文本按词/按字换行
- 定位与层叠：`position: relative / absolute / fixed`、`top/right/bottom/left`（含负值）、`z-index`；脱流盒第二遍布局，包含块 = 最近非 static 祖先 padding box / 视口；绘制分常规层 + 定位层（z 升序，同 z 按文档序），`z-index` 非 0 的定位元素与 `fixed` 元素建立层叠上下文，内部定位子树先局部排序再整体落层
- 可编辑输入：点击聚焦（蓝框 + 按点击位置落光标）、键入文字（含中文输入法）、Backspace / Delete 删除、鼠标拖选 / Ctrl+A 全选、Ctrl+C 复制选区（后端写系统剪贴板）、Ctrl+V 粘贴（后端读系统剪贴板；单行 input 粘贴多行文本折成单行，textarea 保留换行）、方向键 / Home / End 移动光标、Shift+方向键 / Shift+Home/End 扩展选区（锚点不动、活动端随按键移动，越过锚点后选区反向）、输入与退格替换选区、Tab 循环切换（text / password / textarea；password 可聚焦可输入，值全程按 ● 掩码，光标与点击定位同掩码口径，明文不落画不进 SVG，但 Ctrl+C 复制真实值）；`<textarea>` 多行编辑（折行显示、Enter 换行、跨行拖选、超出盒高的行不绘制、纵向滚动跟随光标并支持滚轮）；其余 input type 按 UA 外观区分渲染：radio/checkbox 13x13 控件、submit/reset/button 按钮外观（value 居中）
- 表单与链接控件：`<select>`/`<option>` 下拉（点击展开选项浮层，点选项选中并派发 `change`，下方放不下且上方放得下时整组上移，浮层压过流内容与定位层、命中测试同层序）；`<button>` 内容按 HTML 规范垂直居中（Chrome/Edge 同款，定高按钮尤其明显）、`<a href>` 链接（UA 蓝色 + 下划线，`SetOnNavigate` 注册导航回调，导航在 click 冒泡之后执行）
- 勾选控件：`<input type=radio|checkbox>` 点击切换选中（`checked` 布尔属性按存在性判定，API `IsChecked` / `SetChecked`）；选中态绘制强调色方块 + 白色对勾、强调色圆环 + 实心圆（Chrome 口径），radio 盒恒为正圆；radio 仅 name 非空且相同的项互斥（实测无 name / `name=""` 各自独立）；点 label（包裹其控件或 `for` 指向）等效点该控件
- DOM 式 API：`QuerySelector` / `GetBoundingClientRect` / `SetText` / `OnClick` / `IsChecked` / `SetChecked` 等；内容变化自动触发重排重绘
- 应用层底座：`SetOnChanged` 变更回调（内容或**视口尺寸**变化都会通知——窗口缩放不改文档内容，只靠内容变更收不到信号）；失焦只停画、**不丢编辑状态**（光标与选区保留，页内菜单动作可作用于失焦的编辑区）；`CaretPosition` 查光标行列；`browser.ClipboardText` / `browser.SetClipboardText` 读写系统剪贴板；`browser.RequestClose` 关窗
- 事件冒泡：命中测试找到最深元素，沿父链依次派发 `click` / `change` / `wheel`，随后执行引擎默认动作（select 展开/选中、勾选切换、链接导航）
- 滚动与裁剪：`overflow(visible/hidden/clip/scroll/auto，单值两轴、两值 x y、overflow-x/overflow-y 分轴)` 滚动容器（滚轮驱动、子树平移 + padding 盒裁剪，被裁内容不可命中）、**圆角裁剪按 border-radius 的形状**（`overflow:hidden` 的圆角盒，溢出到角外的子元素按圆角裁掉，角上透出盒后底色）、文档级页面滚动（根元素以视口为滚动盒，长页面/宽页面均可滚）、`wheel` 事件（`OnWheel` 注册、`MouseEvent.DeltaX/DeltaY` 像素位移、冒泡派发）、Shift+滚轮 转横向滚动、textarea 滚轮滚动（与光标跟随同夹紧口径，全量吸收纵向滚轮、撞边界不带动页面）；裁剪经 `Graphics` 可选 `Clipper`（`PushClip`/`PopClip`，逐层求交）落地：`BufferGraphics` 像素断言、SDL2 `SDL_RenderSetClipRect`，SVG 导出同步生成 `clipPath`
- 可插拔图形后端：`engine.Graphics` 接口（DrawText / DrawColor / DrawImage / MeasureText），内置 `FakeGraphics`（计数）、`BufferGraphics`（软件光栅化到 RGBA 缓冲，可 `SavePNG`）供无显示器测试；圆角抗锯齿走**角标精灵**（可选扩展 `SpriteDrawer`：整块圆角一次 blit，替掉逐像素的半透明小方块）——绘制调用数是 GPU 路径上的主要开销，portal 页一帧从 31840 次降到 2500 次、单帧绘制 149ms → 8.7ms（软件渲染器实测），滚动动画才不会被切成几帧
- 元素注册表：`CustomElements().Define(tag, proto)` + `CloneElement` 注册自定义标签，解析时克隆原型生成独立实例
- 无头运行：`-dump-svg` 导出布局结果；`HeadlessViewport` + `BufferGraphics` 可在 CI 中像素级断言渲染结果

## 环境要求

- Go 1.20+（go.mod 基线，作库供其他项目引用，1.20 以上工具链均可构建；无需 CGO）
- 渲染后端目前提供 Windows 实现（`backend/sdl2`，加载 `SDL2.dll` 2.28 与 `SDL2_ttf.dll`）
- 把 `SDL2.dll`、`SDL2_ttf.dll`（及 `zlib1.dll`）放到工程根目录 `libs/`，或保证系统可搜索到它们
- 平台边界：`engine` 包是纯 Go、可移植的；`backend/sdl2` 是平台实现层——Windows 提供 SDL2 实现，其它平台 `NewWindow` 返回 `ErrUnsupported`；`browser` 与 `examples/` 都不带构建标签，任意平台 `go build ./...` / `go test ./...` 都能通过，非 Windows 启动窗口时会打印明确提示

## 快速开始

示例按目录存放在 `examples/` 下。每个目录都是一个**可独立运行**的小程序
（`main.go` + `index.html`，页面用 `go:embed` 打进二进制，从任意工作目录
运行都不依赖当前路径）：

| 目录 | 运行 | 内容 |
| --- | --- | --- |
| `examples/login` | `go run ./examples/login` | 登录页：登录/重置按钮交互、状态文本更新 |
| `examples/form` | `go run ./examples/form` | 常规表单（员工登记表）：定位/层叠/盒模型/可输入控件 |
| `examples/ua` | `go run ./examples/ua` | 原生 UA 表单 + 常用标签：零作者 CSS，可与 Chrome/Edge 直接对照 |
| `examples/scroll` | `go run ./examples/scroll` | 滚动/裁剪：文档级滚动、overflow 容器、textarea 滚轮、`OnWheel` 回显 |
| `examples/browser` | `go run ./examples/browser` | 迷你本地浏览器：顶部常驻地址栏与后退/前进/刷新/首页按钮（换页后仍在），地址栏可打开本地路径（相对路径/`file://`），支持 `about:home`/`about:back`/`about:forward`，外部链接与缺失文件显示提示页；需在仓库内运行 |
| `examples/portal` | `go run ./examples/portal` | 购物门户：fixed 工具条/品牌头/导航、层叠上下文、分类栏、秒杀与商品网格、搜索提示和购物车计数交互 |
| `examples/notepad` | `go run ./examples/notepad` | 仿 Windows 记事本：菜单栏（文件/编辑/视图/帮助）、路径栏、铺满窗口的多行编辑区、状态栏（文件名/行列/字符数）；`-file` 启动时打开文件 |

每个示例都支持同一组调试参数：

```powershell
go run ./examples/ua -exit-after 3s     # 指定时长后自动退出（调试/CI）
go run ./examples/ua -dump-svg out.svg  # 无头导出 SVG（不需要显示器）
```

窗口交互示例：`examples/login` 中的登录/重置按钮通过 `OnClick` 更新状态文本，引擎检测到文档变化后自动重排重绘；`examples/form` 覆盖 11 项布局特性（负 inset 角标、fixed 底条、水印、包含块链锚定、z-index 层序、box-sizing 跨内核一致等），可点击输入框直接键入文字；`examples/scroll` 演示滚轮滚动（文档级长页、overflow 容器横/纵滚动、overflow:hidden 裁剪、textarea 滚轮、`OnWheel` 事件位移回显）；`examples/browser` 用 `SetOnNavigate` 接管链接点击：读取目标 HTML 并在同一窗口重新 `OpenDocument`，每次换页后把 `position:fixed` 的地址栏与导航按钮重新注入文档（引擎没有独立的窗口 chrome 层），浏览历史支持后退/前进/刷新/首页，未知协议与读取失败会生成带返回链接的提示页；`examples/portal` 用固定顶栏 + 绝对定位侧栏组成购物门户，搜索框聚焦后显示热门提示，所有「加入购物车」按钮共享一个计数徽标。

`examples/notepad` 是最"应用"的一个：它把引擎当桌面 UI 框架用——下拉菜单借 `<select>` 的选项浮层实现（引擎没有通用弹层），编辑区几何由程序按窗口尺寸算出（CSS 没有 `calc`），撤销栈由应用层在变更回调里维护（引擎不内置撤销），文件读写与剪贴板走平台能力（`browser.ClipboardText` / `browser.SetClipboardText`）。

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
  style_cascade.go   选择器匹配 + 特异度级联 + 继承
  style_ua.go        UA 样式表（默认值对齐 Chrome，含表单控件 border-box）
  measure.go         盒模型布局、行盒、换行、定位布局（relative/absolute/fixed）
  scroll.go          overflow 滚动容器 + 文档级滚动（偏移夹紧/子树平移/wheel 分发）
  render.go          绘制指令下发（背景/边框/文本片段/文本装饰/定位层/层叠上下文/裁剪栈）
  text_input.go      可编辑输入焦点/光标/编辑/Tab 循环
  select.go          select 展开态、选项浮层布局、取值与点击默认动作
  textarea.go        textarea 折行、光标↔坐标映射、纵向滚动
  anchor.go          `<a href>` 导航默认动作
  form.go            表单控件取值（GetValue/SetValue）与字体度量辅助
  form_render.go     input/textarea/select 专用绘制（含选项浮层）
  event_dispatch.go  命中测试（含裁剪感知）+ click/change/wheel 冒泡派发
  graphics.go        Graphics 接口 / FakeGraphics / BufferGraphics
  svg_dump.go        布局结果导出 SVG
  element_registry.go 标签原型注册表（自定义元素）
backend/sdl2/      SDL2 后端：窗口、事件循环、SDL_ttf 文本纹理缓存（LRU + 像素双上限，
                   页面文本段数超限时不再逐帧重渲染）、剪贴板读写（Ctrl+C 写 /
                   Ctrl+V 读，引擎只管文本）
examples/          可独立运行的示例（每个目录 main.go + index.html）：
  login/             登录页（按钮交互、状态文本）
  form/              常规表单（员工登记表：布局特性 + 可输入控件）
  ua/                原生 UA 表单 + 常用标签（零作者 CSS，与 Chrome 对照）
  scroll/            滚动/裁剪（文档级滚动、overflow、textarea、wheel 事件）
  browser/           迷你本地浏览器（常驻工具条、历史/刷新、链接导航、错误提示页）
  portal/            购物门户（fixed 顶栏、层叠上下文、商品网格、秒杀、购物车交互）
  notepad/           仿 Windows 记事本（菜单栏/路径栏/编辑区/状态栏、文件读写、
                     撤销栈、剪贴板）
libs/              SDL2 运行库（SDL2.dll / SDL2_ttf.dll / zlib1.dll，见 libs/README.md）
```

**包内文档**：每个包的入口说明写在各自的 `doc.go`（`go doc ./engine` 可读，
也进 pkg.go.dev）：engine 讲分层地图与「改代码前必读的不变量」、backend/sdl2
讲平台边界与线程约束、browser 讲三方分工与用法骨架；`libs/` 里第三方 DLL 的
版本、出处、许可与查找顺序见 `libs/README.md`。

## 已支持的 CSS 子集

`display(none/inline/inline-block/block)`、`width/height`、
`box-sizing(content-box/border-box，表单控件 UA 默认 border-box 对齐 Chrome)`、
`margin*`（相邻块级兄弟折叠：取较大者；父-子穿越/空块折叠未实现）、`padding*`、
`border` 简写与 `border-width/color/style`、`border-radius`(四角统一，仅水平半径)、
`border-collapse(collapse/separate，仅表格布局读取，见「已知限制」的表格模型)`、
`background-color`/`background`、`color`、`font-size`、`font-weight(bold)`、`font-family`、
`text-align(left/center/right)`、`text-decoration(underline/line-through/none，随父链继承)`、
`white-space(pre)`（保留空白与制表符、按换行强制断行、不软换行；UA 的 `pre`/`code` 用它，
`nowrap`/`pre-wrap` 未实现）、
`position(relative/absolute/fixed)`、`top/right/bottom/left`(px 与 %，支持负值；
absolute/fixed 相对包含块锚定，right/bottom 回推)、`z-index`（非 0 的定位元素与 fixed 元素建立层叠上下文；z:auto 与 z:0 暂不区分）、
`overflow(visible/hidden/clip/scroll/auto：单值作用两轴、两值为 overflow-x overflow-y、
overflow-x/overflow-y 分轴；hidden 只裁不滚，scroll 恒可滚，auto 溢出时可滚，
文档级滚动不受 html overflow 关键字约束)；
inline-block 的百分比宽度按包含块内容宽解析（而非行内剩余宽）；
UA 默认样式表覆盖浏览器 Windows 侧的常用规则：body 外边距 8px、
h1-h3 字号/外边距、p 段距、a 蓝色下划线、strong/b 加粗、
em/i/code/small/sub/sup 等行内语义标签为 inline（sub/sup 另按 Chrome 实测做
基线位移：上标 `父字号/3+1`、下标 `父字号/5+1`，字号取 smaller 的 13px）、
dt **不加粗**（Chrome UA 实测计算权重 400；HTML4 的 UA 才加粗 dt）、
dd 缩进 40px、ul/ol 缩进 40px、blockquote/pre/figure 外边距、
hr 边框、form/input/select/textarea/button 的尺寸与外观（多行控件的基线按
CSS 2.1 §10.8.1 取底外边距边——Chrome 的 textarea 计算 overflow 非 visible）；
表格/列表标记等仍是近似模型，见「已知限制」；
字体默认语义对齐 Chrome（Windows）并做 UI 取向调整：初始字号 16px；
standard（未指定 family）默认 sans-serif → Arial（区别于 Chrome 的 Times，更贴桌面 UI）；
serif → Times New Roman、sans-serif → Arial、system-ui → Segoe UI、fantasy → Impact、
monospace/cursive → 新宋体（Chrome 在中文 Windows 上的默认等宽字体就是新宋体，
实测 16px 每字符 8px、行高 18，而不是 Courier New / Comic Sans MS；引擎用 simsun.ttc）；
表单控件 UA 默认字体 `Arial, system-ui, sans-serif`（首项 Arial = 引擎默认字体、
桌面 UI 取向；追加 system-ui 只影响 CJK 回退档位——Chrome 控件的汉字落雅黑、
13px 行高 17，若按 Arial 落 Noto Sans 会把按钮撑高 2px）；
`font-family` 首项为 monospace/ui-monospace 且作者从未显式声明过 font-size 时，
计算字号取通用族默认等宽字号 13px（Chrome 实测：body 不写字号 +
`font-family: monospace` → 13px；祖先显式给过 16px 则保持 16px）；
汉字/假名/谚文按字符级脚本分段 fallback，且按 generic family 分流到 Chrome
在中文 Windows 上实际使用的系统字体：sans-serif/Arial/Segoe UI → Noto Sans SC、
serif/Times → Noto Serif SC、monospace/cursive → 新宋体、system-ui → 微软雅黑；
显式指定中文字体族（雅黑/黑体/宋体/等线…）时该字体自带 CJK 字形、不触发
逐字回退；
行盒（line-height: normal）直接用 SDL_ttf 的 `TTF_FontAscent`/
`TTF_FontDescent`：行盒高 = 各内联盒「基线以上高度」的最大值 +
「基线以下高度」的最大值，无内容时不加 strut。该口径是 FreeType 的像素
取整，不再逐像素对齐 Chrome/DirectWrite（同一字体行高可能差 1~4px），
换取不解析字体表的实现简化；
粗体按字体族选**真粗体字文件**（雅黑 → msyhbd.ttc、Arial → arialbd.ttf、
Segoe UI → segoeuib.ttf），不依赖 FreeType 对常规字形的合成加粗——合成加粗
明显更重，24px 中文标题上还会出现笔画粘连/重影（浏览器按字体族选粗体字重，
同页对照下 h2 墨量比由 1.26 收敛到 1.01）；无真粗体文件的字体（黑体等）
仍走合成加粗，与浏览器一致；
每个 (字体文件, hinting 档位) 只开一个 FT_Face，字号/粗体动态切换
（档位进 key 是为支持运行中切 `GHROMEX_HINT`；切换会新开一个 face）；
文本渲染默认走灰度 AA（无彩边、对落点背景色不敏感），hinting 档位由
`GHROMEX_HINT` 控制（`normal|light|mono|none`，默认 normal）：与 Chrome 灰度
基线的同页对比矩阵（fontcmp 口径指标，13px 中文）显示 normal 最同构——
竖画硬跳变 7（Chrome 0）、横画 237（Chrome 227）、墨核 412（Chrome 347），
而 light 档横画偏软（75，即"糊"感来源）；要更柔可 `GHROMEX_HINT=light`。
`GHROMEX_CONTRAST=k`（>1 生效，如 1.3/1.5，上限 3，默认关）在 blended
覆盖通道施加 Skia 同类的对比度 tone 曲线（以 0.5 为枢轴推两端：边缘更实、
毛边更净），代价是墨色变重、竖边台阶感随 k 增大，常与 light/none 档联调
提利。`GHROMEX_LCD=1` 切换到
LCD 子像素（TTF_RenderUTF8_LCD + LIGHT_SUBPIXEL hinting，竖笔画边缘横向
锐度与浏览器 ClearType 同级；合成按文字落点背景色进行——渲染管线沿父链
携带最近实底，深色背景上白字不会带白底块。代价是彩底上竖笔边缘有轻微
彩边（与浏览器子像素渲染同性质）；LCD 打开时 hinting 强制 subpixel，
`GHROMEX_HINT` 仅影响灰度路径）。灰度 AA 的取舍已定量确认为固有性质、非管线失真：
探针 `backend/sdl2/gray_text_test.go` 以同 face 同 hinting 直出 blended
surface 合成白底，与 `DrawText→RenderCopy→Screenshot` 落屏结果逐像素
比对，双渲染器偏差均值 <1.3、最大 3（超差 >4 为 0%），即绘制链路无
失真；观感软硬由档位选择决定（锯齿与模糊是同一参数的两端），
另受灰度 AA 无横向子像素分解的物理极限约束——用 `GHROMEX_HINT` /
`GHROMEX_CONTRAST` 在锐度谱上取点，需要横向锐度请开 `GHROMEX_LCD=1`
走 LCD 子像素路径；
选择器支持标签、`.class`、`#id`、类型+class/id 组合链（如 `div.wide`）与特异度排序。

## 已知限制

- 布局为块级 + 行内/inline-block 流式布局 + 定位子集；无 flex/grid；滚动仅滚轮驱动（无滚动条 UI、无拖拽滚动、无 PageUp/PageDown 翻页键、无触控板惯性），wheel 步长 100px/档（与 Chrome/Windows 一致）；文档级滚动为**动画式**：按真实时间 120ms ease-out 推进（不按固定帧数，避免排版慢时把一档滚轮拖成几百毫秒），滚动只平移子树——**动画帧不重排**（`engine.TickScroll` 按增量平移已布局的盒子，级联与布局只在内容变化时跑）；字体路径解析与文本度量已缓存（文本度量按「文本+字号+粗细+family」），examples/ua 单次重排约 1~2ms。但滚动偏移无对外 DOM API（`scrollTop`/`scrollLeft` 未导出，仅由 wheel 与 textarea 光标跟随内部驱动），亦无 `scroll`/`wheel` 之外的滚动事件
- position v1 边界：absolute 双锚（left+right 同给）时 auto 宽按内容收缩而非拉伸；未实现嵌套层叠上下文（opacity/transform 成组、负 z-index 压至祖先背景之下）；命中测试未按 z-index 取最上层元素（select 选项浮层例外，按浮层层序优先）
- `<input>` 可编辑仅支持 text/password 类型；radio/checkbox 无键盘 Space 切换、无 disabled / indeterminate / 表单重置；无剪切（Ctrl+X）、无按词扩展选区（Ctrl+Shift+方向键）
- `<select>` 仅鼠标交互（展开/收起/选中），无键盘上下键选择、无 multiple、无 optgroup；选项浮层不随视口裁剪（超出视口的选项画到窗口外）；收起态按内容收缩（文字 + 箭头位 16px，与 Chrome 同口径）
- `<textarea>` 无剪切（Ctrl+X）、无按词扩展选区（Ctrl+Shift+方向键）；滚轮可滚动（超盒高的行随滚动偏移平移、半截行按裁剪截断；后端未实现 `Clipper` 时退化为整行丢弃）
- 行高（`line-height: normal`）由 SDL_ttf 的 `TTF_FontAscent`/`TTF_FontDescent`
  计算（两者都是 **int32 有符号**返回值，`TTF_FontDescent` 为负——不按 int32 重解释
  会被当成巨大正数再钳成 0，所有字体的行盒就会短 2~3px）
  计算，与 Chrome/DirectWrite 可能有 1~4px 差异，不再是逐像素同口径；且布局
  全程为整数像素，浏览器的小数边距/行盒（如 h3 的 `margin: 1em` = 18.72px、
  h2 的 0.83em = 19.92px）只能取整，逐段累积后整页可漂移数像素；
  空块自折叠未实现（父-子穿越 margin 折叠已实现）
- `DrawImage` 尚未实现（接口已预留）
- 圆角裁剪（`overflow:hidden` + `border-radius`）的角区用「盒后底色」补盖实现：背后不是纯色（渐变/图片）时是近似；后端不支持 `engine.SpriteDrawer` 时不补盖（退化为矩形裁剪）
- 裁剪仅由实现 `engine.Clipper`（`PushClip`/`PopClip`）的后端生效（当前 `BufferGraphics`、SDL2 均已实现；自定义后端未实现时静默跳过裁剪、仅不裁剪不报错）
- 边框样式只按**单色实心**绘制：`inset/outset/groove/ridge/dashed/dotted/double`
  都退化为 solid，且不支持四边不同颜色——Chrome 的 `input`（2px inset）、
  `button`（2px outset #000）、`hr`（1px inset，上暗 #808080 下浅）因此只做到
  外盒尺寸一致，明暗双色边框与线型差异保留（对照页 examples/ua 的 hr 引擎画成
  两行同色）
- 字体：generic/具名映射基于 Windows 自带字体文件，缺失时逐级回退 standard；
  无 webfont 加载、无斜体渲染（font-style 仅解析不渲染）；
  `monospace`/`cursive` 映射到 simsun.ttc 的 face 0（SimSun），Chrome 用的是
  同一文件里的新宋体（NSimSun）——行高与整串宽度一致，但单字 ink 高度与
  短串宽度有微差（对照页 `<code>元素</code>` 等宽串 61 vs Chrome 63、ink 高 15 vs 13）
- 表格是最小模型：按行堆叠、同列左缘对齐、列宽取各行最大值、caption 居中、
  th 居中加粗；`border-collapse: collapse` 已支持（单元格只画上/左、外缘右/下
  由表格补画，等价于浏览器的「半边框」：实测紧贴无间隙、每行只摊 1px 线，
  对照页表格 181×112 vs Chrome 181.75×112，单元格外盒 90×30 vs 90.38×30）；
  仍不支持 colspan/rowspan、不做「宽者胜」的边框冲突仲裁（相邻边框取后者），
  列宽按**内容最大值**（不做浏览器的自动表格布局分配），separate 下用 2px
  间隙近似 border-spacing
- 列表：缩进/外边距/标记已实现并与 Chrome 对齐（ul 的 •、ol 的序号；标记位置按内容左缘 −0.75em 近似）；仍无 `display:list-item` 与 `list-style-type`（作者自定义标记类型不生效）
- 无 `vertical-align`（作者声明的垂直对齐不生效；`sub`/`sup` 的基线位移是 layout
  内置行为）；`white-space` 仅 `pre` 一个关键字；无 `line-height` 属性
- 级联为每次重排全量重算：逐元素重跑全部规则匹配并排序（元素数 × 规则数 × 祖先深度）——大样式表 + 大文档时每次输入都会触发一次全量级联，尚未做规则索引/失效缓存（字体路径与文本度量已有缓存，见「滚动」条；类名匹配已改为零分配扫描）
- 灰度 AA（默认）无横向子像素分解，竖笔边缘锐度低于 LCD/ClearType（物理极限）；
  软硬由 hinting/tone 曲线档位决定（`GHROMEX_HINT` 默认 normal、`GHROMEX_CONTRAST`
  默认关，可联调取点，见渲染段），保真度探针证实管线忠实（见上文渲染段；
  需要横向锐度可 `GHROMEX_LCD=1` 开 LCD 子像素）

## 调试环境变量

| 变量 | 作用 |
|---|---|
| `GHROMEX_DEBUG=1` | 回显绘制失败原因与每次点击的命中/派发结果 |
| `GHROMEX_LCD=1` | 开启 LCD 子像素文本渲染（默认关闭：文本默认灰度 AA；开启后竖笔更锐，彩底边缘有轻微彩边，见「已知限制」） |
| `GHROMEX_HINT=档位` | 灰度文本 hinting：`normal`（默认，与 Chrome 灰度轮廓最同构）/`light`（更柔）/`mono`/`none`；LCD 打开时强制 LIGHT_SUBPIXEL，本变量仅影响灰度路径；运行中切换亦生效（face 与纹理按档位重建）；需 SDL2_ttf 导出 `TTF_SetFontHinting`，缺失时固定为 FreeType 默认 hinting |
| `GHROMEX_CONTRAST=k` | 灰度字形 tone 曲线强度（>1 生效、上限 3，默认关）：以 0.5 为枢轴把覆盖度推两端，边缘更实、毛边更净；k 越大墨色越重、竖边台阶感越明显，可与 `GHROMEX_HINT` 联调（如 `light+1.3`、`none+1.5`），仅影响 blended（灰度）路径 |
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
- 示例退出约定：`go run ./examples/login -exit-after 3s` 可用于冒烟验证
- 灰度文字保真度探针：`go test ./backend/sdl2 -run TestGrayscalePipelineFidelity`（默认 dummy 驱动软件渲染器，
  `GHROMEX_REAL=1` 走真机 GPU）——同源直出面与落屏截图逐像素比对，供回归「灰度管线忠实」结论
- 文字锐度/字体对比条：`GHROMEX_CMP=1 go test ./backend/sdl2 -run 'TestFontCompareStrip|TestHintingValueScan'`
  输出 `.temp/fontcmp.png`（NORMAL/LIGHT/NONE/SS2x/LCD 五行并排，肉眼与量化均可比对）
