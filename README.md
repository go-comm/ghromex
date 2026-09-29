# Ghromex

用 HTML/CSS 写原生桌面 GUI 的迷你 WebView 框架。

用 Go 实现一个小型渲染引擎：解析 HTML/CSS → 级联计算样式 → 盒式布局 → 绘制指令交给渲染后端（当前为 SDL2，纯动态加载，**无需 CGO**）。渲染 1 CSS px = 1 物理像素，窗口尺寸即视口尺寸，天然适合桌面小部件与内部工具界面。

## 特性

- 纯 Go 渲染引擎：HTML 解析、CSS 级联、选择器（标签 / `.class` / `#id` / 简单组合，含特异度）、块级与行内（inline / inline-block）布局、文本按词/按字换行
- 定位与层叠：`position: relative / absolute / fixed`、`top/right/bottom/left`（含负值）、`z-index`；脱流盒第二遍布局，包含块 = 最近非 static 祖先 padding box / 视口；绘制分常规层 + 定位层（z 升序，同 z 按文档序）
- 可编辑输入：点击聚焦（蓝框 + 按点击位置落光标）、键入文字（含中文输入法）、Backspace 删除、方向键 / Home / End 移动光标、Tab 循环切换（text / password / textarea；password 可聚焦可输入，值全程按 ● 掩码，光标与点击定位同掩码口径，明文不落画不进 SVG）；`<textarea>` 多行编辑（折行显示、Enter 换行、超出盒高的行不绘制、纵向滚动跟随光标）；其余 input type 按 UA 外观区分渲染：radio/checkbox 13x13 控件、submit/reset/button 按钮外观（value 居中）
- 表单与链接控件：`<select>`/`<option>` 下拉（点击展开选项浮层，点选项选中并派发 `change`，下方放不下且上方放得下时整组上移，浮层压过流内容与定位层、命中测试同层序）、`<a href>` 链接（UA 蓝色 + 下划线，`SetOnNavigate` 注册导航回调，导航在 click 冒泡之后执行）
- 勾选控件：`<input type=radio|checkbox>` 点击切换选中（`checked` 布尔属性按存在性判定，API `IsChecked` / `SetChecked`）；选中态绘制强调色方块 + 白色对勾、强调色圆环 + 实心圆（Chrome 口径），radio 盒恒为正圆；radio 仅 name 非空且相同的项互斥（实测无 name / `name=""` 各自独立）；点 label（包裹其控件或 `for` 指向）等效点该控件
- DOM 式 API：`QuerySelector` / `GetBoundingClientRect` / `SetText` / `OnClick` / `IsChecked` / `SetChecked` 等；内容变化自动触发重排重绘
- 事件冒泡：命中测试找到最深元素，沿父链依次派发 `click` / `change`，随后执行引擎默认动作（select 展开/选中、勾选切换、链接导航）
- 可插拔图形后端：`engine.Graphics` 接口（DrawText / DrawColor / DrawImage / MeasureText），内置 `FakeGraphics`（计数）、`BufferGraphics`（软件光栅化到 RGBA 缓冲，可 `SavePNG`）供无显示器测试
- 元素注册表：`CustomElements().Define(tag, proto)` + `CloneElement` 注册自定义标签，解析时克隆原型生成独立实例
- 无头运行：`-dump-svg` 导出布局结果；`HeadlessViewport` + `BufferGraphics` 可在 CI 中像素级断言渲染结果

## 环境要求

- Go 1.20+（go.mod 基线，作库供其他项目引用，1.20 以上工具链均可构建；无需 CGO）
- 渲染后端目前提供 Windows 实现（`backend/sdl2`，加载 `SDL2.dll` 2.28 与 `SDL2_ttf.dll`）
- 把 `SDL2.dll`、`SDL2_ttf.dll`（及 `zlib1.dll`）放到工程根目录 `libs/`，或保证系统可搜索到它们

## 快速开始

```powershell
# 运行内置登录页 demo
go run ./demo

# 布局能力示例（员工登记表：定位/层叠/盒模型/可输入表单）
go run ./demo -file demo/form.html

# UA 原生对照页（零作者 CSS，可与浏览器直接打开同一文件比对）
go run ./demo -file demo/form-ua.html

# 渲染自己的页面
go run ./demo -file page.html

# 无头导出 SVG（不需要显示器）
go run ./demo -dump-svg out.svg
```

窗口交互示例：`demo/index.html` 中的登录/重置按钮通过 `OnClick` 更新状态文本，引擎检测到文档变化后自动重排重绘；`demo/form.html` 覆盖 11 项布局特性（负 inset 角标、fixed 底条、水印、包含块链锚定、z-index 层序、box-sizing 跨内核一致等），可点击输入框直接键入文字。

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
  render.go          绘制指令下发（背景/边框/文本片段/文本装饰/定位层）
  text_input.go      可编辑输入焦点/光标/编辑/Tab 循环
  select.go          select 展开态、选项浮层布局、取值与点击默认动作
  textarea.go        textarea 折行、光标↔坐标映射、纵向滚动
  anchor.go          `<a href>` 导航默认动作
  form.go            表单控件取值（GetValue/SetValue）与字体度量辅助
  form_render.go     input/textarea/select 专用绘制（含选项浮层）
  event_dispatch.go  命中测试 + click/change 冒泡派发
  graphics.go        Graphics 接口 / FakeGraphics / BufferGraphics
  svg_dump.go        布局结果导出 SVG
  element_registry.go 标签原型注册表（自定义元素）
backend/sdl2/      SDL2 后端：窗口、事件循环、SDL_ttf 文本纹理缓存
components/        组件工厂（button / input / class 工具）
demo/              演示应用（登录页 index.html + 布局示例 form.html）
libs/              SDL2 运行库（SDL2.dll / SDL2_ttf.dll / zlib1.dll）
```

## 已支持的 CSS 子集

`display(none/inline/inline-block/block)`、`width/height`、
`box-sizing(content-box/border-box，表单控件 UA 默认 border-box 对齐 Chrome)`、
`margin*`（相邻块级兄弟折叠：取较大者；父-子穿越/空块折叠未实现）、`padding*`、
`border` 简写与 `border-width/color/style`、`border-radius`(四角统一，仅水平半径)、
`background-color`/`background`、`color`、`font-size`、`font-weight(bold)`、`font-family`、
`text-align(left/center/right)`、`text-decoration(underline/line-through/none，随父链继承)`、
`position(relative/absolute/fixed)`、`top/right/bottom/left`(px 与 %，支持负值；
absolute/fixed 相对包含块锚定，right/bottom 回推)、`z-index`；
inline-block 的百分比宽度按包含块内容宽解析（而非行内剩余宽）；
字体默认语义对齐 Chrome（Windows）并做 UI 取向调整：初始字号 16px；
standard（未指定 family）默认 sans-serif → Arial（区别于 Chrome 的 Times，更贴桌面 UI）；
serif → Times New Roman、monospace → Courier New、system-ui → Segoe UI；
汉字/假名/谚文按字符级脚本分段 fallback 到微软雅黑（中英混排与 Chrome 一致）；
每个字体文件只开一个 FT_Face，字号/粗体动态切换；
文字渲染默认走 LCD 子像素（TTF_RenderUTF8_LCD + LIGHT_SUBPIXEL hinting，
竖笔画边缘横向锐度与浏览器 ClearType 同级；合成按文字落点背景色进行——
渲染管线沿父链携带最近实底，深色背景上白字不会带白底块。代价是彩底上
竖笔边缘有轻微彩边（与浏览器子像素渲染同性质）；`GHROMEX_LCD=0` 回退
TTF LIGHT hinting 的灰度 AA，后者较默认 NORMAL 网格吸附的小字号纵向
硬跳变降约 60%）。灰度 AA 的取舍已定量确认为固有性质、非管线失真：
探针 `backend/sdl2/gray_text_test.go` 以同 face 同 hinting 直出 blended
surface 合成白底，与 `DrawText→RenderCopy→Screenshot` 落屏结果逐像素
比对，双渲染器偏差均值 <1.3、最大 3（超差 >4 为 0%），即绘制链路无
失真；观感偏软仅源于 ① LIGHT hinting 用消锯齿换取的纵向柔和
（13px 中文纵向硬跳变 161→53——锯齿与模糊是同一参数的两端）与
② 灰度 AA 没有横向子像素分解的物理极限。故灰度路径维持现状不改，
需要横向锐度请走默认 LCD 路径；
选择器支持标签、`.class`、`#id`、类型+class/id 组合链（如 `div.wide`）与特异度排序。

## 已知限制

- 布局为块级 + 行内/inline-block 流式布局 + 定位子集；无 flex/grid、无滚动
- position v1 边界：absolute 双锚（left+right 同给）时 auto 宽按内容收缩而非拉伸；未实现嵌套层叠上下文（opacity/transform 成组、负 z-index 压至祖先背景之下）；命中测试未按 z-index 取最上层元素（select 选项浮层例外，按浮层层序优先）
- `<input>` 可编辑仅支持 text 类型；radio/checkbox 无键盘 Space 切换、无 disabled / indeterminate / 表单重置；无选区/复制粘贴
- `<select>` 仅鼠标交互（展开/收起/选中），无键盘上下键选择、无 multiple、无 optgroup；选项浮层不随视口裁剪（超出视口的选项画到窗口外）
- `<textarea>` 无选区/复制粘贴、无按住 Shift 扩展选区；盒外整行不绘制（引擎无裁剪原语），需自备足够 height
- 行高按字形高度量（无 line-height 属性），与浏览器 normal（随字体 ≈1.15-1.5 倍）
  行盒高度存在小差；父-子穿越 margin 折叠与空块自折叠未实现
- `DrawImage` 尚未实现（接口已预留）
- 边框样式目前按单色实心绘制（dashed/dotted 等不做虚线区分）
- 字体：generic/具名映射基于 Windows 自带字体文件，缺失时逐级回退 standard；无 webfont 加载、无斜体渲染
- `GHROMEX_LCD=0` 灰度 AA 的字缘较 LCD 偏软：LIGHT hinting 消锯齿与灰度无横向子像素分解所致，
  属取舍而非缺陷（保真度探针 `backend/sdl2/gray_text_test.go` 证实管线忠实，见上文渲染段）

## 调试环境变量

| 变量 | 作用 |
|---|---|
| `GHROMEX_DEBUG=1` | 回显绘制失败原因与每次点击的命中/派发结果 |
| `GHROMEX_LCD=0` | 关闭 LCD 子像素文本渲染（默认开启；回退灰度 AA，字缘较软属固有取舍，见「已知限制」） |
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
- demo 退出约定：`go run ./demo -exit-after 3s` 可用于冒烟验证
- 灰度文字保真度探针：`go test ./backend/sdl2 -run TestGrayscalePipelineFidelity`（默认 dummy 驱动软件渲染器，
  `GHROMEX_REAL=1` 走真机 GPU）——同源直出面与落屏截图逐像素比对，供回归「灰度管线忠实」结论
- 文字锐度/字体对比条：`GHROMEX_CMP=1 go test ./backend/sdl2 -run 'TestFontCompareStrip|TestHintingValueScan'`
  输出 `.temp/fontcmp.png`（NORMAL/LIGHT/NONE/SS2x/LCD 五行并排，肉眼与量化均可比对）
