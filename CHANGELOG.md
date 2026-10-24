# 更新日志

版本号即 git tag（Go module 语义：`v0.2.0` 可从
`github.com/go-comm/ghromex@v0.2.0` 直接取用）。

## 未发布

- **engine**：`BufferGraphics.SavePNG` 更名 `DumpPNG`：它只是「把缓冲当前
  内容编码落盘」的快照动作（不布局、不绘制，没渲染过是空图），与示例
  `-dump-png` 的完整出图流程（装外壳 → Refresh → RenderNode → 落盘）区
  分；词根同时对齐 `doc.DumpSVG()`。升级时替换方法名即可。
- **出图口径**：分析用无头两路——`doc.DumpSVG()`（`-dump-svg`）导出级联+
  布局 SVG（真实 `<text>`，核对几何/层级/配色），`-dump-png` 走完整流程
  产软件光栅像素图（文字为占位方块，CI 逐像素断言），其最后一步
  `BufferGraphics.DumpPNG` 仅存缓冲快照、不是出图入口；截图用
  `Window.Screenshot()` 回读真实窗口像素（应用自行编码 PNG，见
  `examples/snip`）。`browser` 包文档新增「出图口径」小节，示例 flag 帮助
  文案统一为「无头出图 / 真实窗口截图：产出 + 用途」；示例覆盖补全：
  ua / portal / notepad 补 `-dump-png`，browser / notepad 提供本地
  `-shot`（`Window.Screenshot` 回读 + 本地编码 PNG，与 kit 同款，库层
  不提供 PNG 编码；notepad 另有 `-shot-find` 截图前展开查找栏，模拟
  Ctrl+F 交互态）。
- **ui**：新增 `Spec.NoBodyReset`：跳过 `Install` 注入的 body 复位块（margin /
  padding / 颜色 / 字体），画布底色（`html` 的 `background-color`）不受影响、
  始终注入。演示 UA 样式的页面（body 的 `margin:8px` 本身就是演示内容）与
  靠 body padding 给固定栏让位的页面不再被复位顶歪。
- **ui**：工具栏支持嵌入现成控件：`ui.Slot(control)` 可作为 `Spec.Tools` 的
  一项，把输入框、下拉等控件排在命令按钮之间（与 `Sep()` 同属「非命令条目」，
  不参与快捷键注册与可用态刷新）；控件句柄仍归应用，取值设值直接在应用手里
  的对象上完成，不必按 id 回查元素。槽位随工具栏主题样式排版。
- **examples/browser**：外壳从手写工具条（chrome.go，逐个控件算绝对坐标 +
  视口变化重排）改为 `ui.Install` 装配的工作台：后退/前进/刷新/首页是带图标
  的 `ui.Tool`（带 `When` 可用态），示例页面下拉与地址输入框是 `ui.Slot`，
  状态栏走 `StatusSegments`；换页重排地址栏坐标的整条路径删除。
- **ui**：工具栏 `Action` 构造器 `Button` 更名 `Tool`（对应 `Spec.Tools` 与
  `.system-ui-tool`），与内容区控件 `NewButton` 不再一字之差；`Item` / `Sep`
  不变。
- **ui**：`Shell` 更名为 `Workbench`（工作台），`App.Shell()` 相应改为
  `App.Workbench()`；「工作台」指装在文档四周的菜单栏/工具栏/状态栏加
  主题样式，`Install` 的返回值与全部方法不变，升级时替换类型名与方法名
  即可。
- **ui**：`Install` 自动复位 body（去掉 UA 的 8px 外边距，文字颜色、字体、
  字号跟随主题），应用页面不必再自写 `margin:0` / `padding:0` / `color` /
  `font-family` 样板；自定义排版请挂在内容容器上（根作用域下页面的裸
  `body` 规则盖不过工作台的复位规则）。
- **engine**：新增原生 `title` 提示气泡：鼠标停在带非空 `title` 的元素（含
  其子孙）上超过 600ms，在鼠标右下弹出系统气泡（圆角描边、豁免裁剪、画在
  最顶层），离开/按下/点击/实际滚动收起，换目标重新计时。每帧推进挂在
  `TickTooltip`（与 `TickScroll` 并排），弹出与收起只重绘、不重排；
  `TooltipText` / `TooltipPending` 供外部读取状态。气泡是绘制层拿不到级联
  样式，配色经 `SetTooltipColors` 按文档下发（默认深底浅字）；同时导出
  `ParseColor` 供应用把主题里的颜色字符串解析成 `Color`。
- **ui**：工具栏按钮自动带上 `title` 提示（`Label` + `Shortcut` 组合），
  图标按钮悬停即可看到命令文字；气泡配色随主题下发（`Surface` 底 +
  `BorderStrong` 描边 + `Text` 文字），`Install` / `SetTheme` 自动完成。
- **ui**：组件类名由 `ui-*` 统一改为 `system-ui-*`，样式自动挂到
  `html.system-ui` 根作用域下；升级时页面里的旧类名需要同步替换。
- **ui**：新增 `App.SetTheme` / `Workbench.SetTheme`，可在运行时切换主题，
  不用重建菜单、工具栏或页面。
- **ui**：新增 `Workbench.SetStatusVisible`；菜单栏改在工具栏/状态栏之后插入，
  修复菜单浮层与工具栏重叠时点击被工具栏截胡的问题。
- **ui**：状态栏支持左侧分段（`StatusSegment` / `Spec.StatusSegments` /
  `SetStatusSegment`），固定或自适应宽度、段内对齐、隐藏与自动分隔线；
  右侧消息独立更新并按 `Theme.StatusRightWidth` 预留宽度。`Spec.Status` /
  `SetStatus` 保留为单段兼容写法。
- **ui**：新增页面控件 `NewLabel` / `NewTextView` / `NewButton` /
  `NewCheckbox` / `NewSelect` / `NewTextInput` / `NewTextArea` / `NewField`，
  统一处理类名、取值/设值、禁用态与事件适配；`NewButton` 与工具栏的
  `ui.Tool` 区分，后者仍构造 `Action`。
- **ui**：新增控件条 `NewBar`（配 `Append` / `SetInline` / `SetVisible`）与
  主题令牌 `Theme.BarHeight`：条内控件排成一行并按条高垂直居中（内边距由
  条高减控件高推导）；定位声明与显隐分开管，`SetVisible` 只在应用那份声明
  上补 `display:none`，不会把 `top` 冲掉。`NewTextArea` 增加平铺变体
  `SetVariant("plain")`（去描边与圆角，贴边铺满的内容区用）。
- **ui**：占位块（`system-ui-spacer-top/bottom`）与 1px 分隔线
  （`system-ui-hairline`）改为 `pointer-events: none`：它们只是为排版存在，
  DOM 序又排在页面内容之后，留着可命中就会吃掉浮层内容的点击。
- **engine**：新增 `RemoveAttribute`，用于真正移除 `disabled` 等布尔属性
  （只把值改成空串仍然表示属性存在）。
- **engine**：新增 `pointer-events`（`auto` / `none`，沿父链继承）：装饰性
  与纯排版的盒子不再成为命中目标，修复「看得见点不到」。只影响命中测试，
  布局与渲染不变。
- **engine**：`display:none` 的元素矩形彻底空掉——`GetBoundingClientRect`
  返回边框盒，此前留着布局期缓存的 padding/border 边长，隐藏元素仍会报出
  16x15 之类的空壳尺寸。
- **engine**：textarea 在首次布局前（或 `display:none` 时）读 `CaretPosition`
  改为退回按硬换行算逻辑行列：此前折行宽度取未布局的 0，每个字符折成一行，
  状态栏会报出「行 29」这类没意义的数字。
- **examples/kit**：新增「基础控件」卡片，演示 Go 侧构造表单控件。
- **examples/notepad**：状态栏改为文件名/行列/字符数三段独立更新。
- **examples/iconsheet**：工具栏「切换主题」与「视图」菜单支持浅色/深色
  实时切换。
- **examples/notepad**：菜单、工具栏、快捷键和状态栏改用 `ui` 生成，
  删除自绘菜单/按钮/状态栏；编辑、查找替换、撤销和文件逻辑保持不变。
- **examples/notepad**：页面不再依赖 HTML：删除 `index.html` 与 `go:embed`，
  查找栏与编辑区由 `page.go` 用 `ui` 控件拼装（`NewBar` + `NewTextInput` /
  `NewButton` + `NewTextArea` 平铺变体），类名、取值/设值、点击绑定都归控件，
  条内排布与配色归主题；页面 CSS 只留 `body` 归零边距与文字/字体，颜色一律
  取 `ui.Theme` 令牌，不再逐控件写绝对坐标与颜色。
- **examples/notepad**：全部命令配图标——菜单项显示图标列，工具栏退化为
  纯图标按钮。
- **examples/notepad**：查找替换的行列换算按 rune 计列（与
  `engine.CaretPosition` 同口径）：此前字节下标与 rune 列数混加，中文文本里
  状态栏报错列、替换位置也跟着偏。
- **examples**：删除 `examples/form`、`examples/login`、`examples/scroll`；
  对应的滚轮回归测试改用内联长页夹具，不再依赖示例文件。

## v0.2.0 — 2026-10-22

仓库首个正式版本。

**引擎**

- HTML 解析、CSS 级联（特异度 / `!important` / 继承）、盒式布局、文本按字换行
- 行内排版：`line-height`（normal / 倍数 / 百分比 / px）、`vertical-align`
  （含 `text-top`/`text-bottom`/`sub`/`super`）、inline-block
- 定位与层叠：`position: relative / absolute / fixed`、`z-index`、嵌套层叠上下文
- 滚动：`overflow` 滚动容器、文档级滚轮滚动（按时间缓动 + 目标累加）、视口裁剪
- 伪类：`:hover` / `:focus` / `:active` / `:checked` / `:disabled` / `:enabled` /
  `:first-child` / `:last-child` / `:only-child` / `:root` / `:empty`，
  UA 样式表带悬停/按下反馈，状态变化自动重排重绘
- 列表标记（圆点与序号）、最小表格布局、外边距折叠、`box-sizing`
- 无头出图：`NewBufferedViewport` / `OpenBufferedDocument`（软件光栅化，可存 PNG）

**交互与控件**

- `input` / `textarea`：点击聚焦与光标定位、IME 输入、拖选、选区替换、
  Ctrl+A/C/V、Shift+方向键扩展选区、Tab 循环、`password` 掩码口径贯穿光标与命中
- `<select>` 下拉浮层（空间不足时整组上移）、`radio`/`checkbox`（对齐 Chrome，
  同名互斥）、`label` 激活、`<a href>` 导航回调
- 元素级按键处理器（返回 true 等价 `preventDefault`）、伪类状态读写 API、
  `display:none` 元素不参与命中测试

**文本保真**

- 默认灰度 AA，可选 LCD 子像素；hinting 档位 / 对比度 / 全局基线补偿均可用
  环境变量联调
- 真粗体字文件、CJK fallback 分段、行盒高度取字体 hhea/OS2 度量
- 角标精灵 + 覆盖率查表：圆角填充与抗锯齿开销降到可接受范围

**后端与性能**

- 纯动态加载 SDL2，**无 CGO**；GPU 优先，创建失败自动回退软件渲染
- 绘制调用：portal 页一帧 31840 → 2500（整角一次 blit）
- 精灵像素与后端纹理都按「条数 + 像素」双上限 LRU 淘汰（原来无上限）
- 不透明填充跳过逐像素混合（4K 门户页 37ms → 4.07ms）、视口裁剪、纹理只上传一次
- 诊断与基准：`GHROMEX_DEBUG`（渲染路径/驱动名）、`GHROMEX_SOFTWARE`（强制软件）、
  `GHROMEX_BENCH`（布局/清屏/重绘/呈现/空闲分段计时，p50/p95/p99，仅本地诊断）
- 性能口径与实测数据见 README「性能实测」

**平台**

- Windows：原生文件对话框（Shell `IFileDialog`）、消息框（`MessageBoxW`）、
  系统剪贴板；均为纯 Win32/COM 调用
- 非 Windows 平台可正常编译，平台专属能力返回 `ErrUnsupported`

**示例**：`browser`（地址栏 + 前进/后退/刷新/首页）、`portal`、`notepad`
（菜单 + 工具栏 + 撤销重做 + 查找替换）、`form`、`login`、`scroll`、`ua`

## v0.1.0

未单独发版：此前通过伪版本（pseudo-version）引用。v0.2.0 汇总了全部历史变更。
