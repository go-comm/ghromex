# 更新日志

版本号即 git tag（Go module 语义：`v0.2.0` 可从
`github.com/go-comm/ghromex@v0.2.0` 直接取用）。

## 未发布

- **ui**：状态栏统一到段模型——`StatusSegment` 新增 `AnchorRight`，右钉段
  与左段走同一套构建/更新路径（进右侧固定区、参与 `has-right` 预留、
  `SetStatusSegment`/`StatusSegment` 按 Key 读写），空文本右钉段视为无
  消息不占位（左段空文本仍占位）。`Spec.StatusRight` / `SetStatusRight`
  降为兼容糖：等价保留键 `"status-right"` 的右钉段，跨重建保留、固定排
  在右区最右；左轨道与右区的分隔线、`is-first`、幂等更新口径不变。
  新增 `TestStatusRightSegment` 守卫：右区生成与统一更新路径、糖消息与
  自建右钉段共存互不拆台、空文本收起、糖跨 `SetStatusSegments` 重建保留。
  实测 kit 状态栏右缘 12px，与旧版像素口径一致。

- **ui**：禁用输入控件退回「下沉」口径——`:disabled` 底色从 `SurfaceAlt`
  改回画布档 `Bg`、描边退到弱档 `Border`。上一笔深色层次重排把
  `SurfaceAlt` 升格成悬浮工具栏的亮档后，禁用字段比可用的还亮，方向
  反了；浅色同步受益（白字段上灰一档）。新增像素守卫
  `TestDisabledFieldSinks`：禁用底色 = Bg 且不撞悬浮档，描边 = 弱档
  Border（深色 `Field==Bg`，中心点区分不了规则失效，靠描边色兜底）。

- **backend**：渲染路径**默认改为内存渲染**——建窗直接用 SDL 软件渲染器
  （像素全部落在系统内存的窗口表面，跨驱动/远程桌面表现一致，回读截图
  不受 GPU 纹理上限约束）。走 GPU 加速（accelerated + vsync）需显式
  `GHROMEX_GPU=1`；`GHROMEX_SOFTWARE=1` 与 `SDL_VIDEO_DRIVER=dummy` 两个
  强制口优先级不变（`GHROMEX_GPU=1` 也压不过）。渲染器选择与归因描述收成
  可测试的纯函数 `planRenderer` / `rendererDescFor`（描述四态：显式加速 /
  强制软件 / GPU 创建失败回退 / 默认内存渲染），并补护栏测试。诊断输出
  口径随之更新：默认打印 `renderer=software (driver=software; 默认内存渲染)`。

- **ui**：深色主题空间层次重排——新增 `Field` 令牌（输入控件底色），
  input/select/textarea/中性按钮底色从 `Surface` 迁到 `Field`。深色下
  Field=#0F172A 沉到画布档（一眼看出哪里能打字）、SurfaceAlt 提到
  #24334A（工具栏/控件条比菜单栏更亮），层次变成「输入区 ≤ 画布 <
  栏面 < 悬浮条」逐级拉开；`BorderStrong` #3E5069 → #334155（输入框
  默认描边收暗，focus 才交给亮蓝）。浅色仅语义迁移、色值不变。
- **ui**：中性描边按钮统一走 Field 底 + BorderStrong 描边（原 Surface
  底 + Border 描边在深色栏底上边界不清）；`.system-ui-bar` 内的中性
  按钮从「透明底纯文字」补成与输入框同档的描边按钮（is-primary 与
  禁用态显式压回，不被条选择器后写覆盖）。顺带删掉 `.system-ui-tool
  :active` 的重复声明。

- **examples**：notepad 新增 `-dark` 开关（对齐 kit 惯例）——`ui.Dark()`
  整套令牌换装，菜单/工具栏/查找栏/编辑区纸面/状态栏一起变，方便核对
  深色主题下两档控件高度与文字居中的表现。

- **engine**：表单 `input` 文字垂直居中修复——绘制行盒不再受继承的
  声明 `line-height` 影响：`inputVisibleWindow` 返回的 `th` 取的是声明
  行高（工具栏场景下被继承撑到 28，大于盒高 26），旧代码
  `if th < height` 才居中，条件不成立、`ty` 钳到框顶，文字整体偏上约
  半行距。现在行盒无条件居中，文字另补 `(th − 自然行高)/2` 的
  half-leading（`DrawText` 契约是 y = 纹理顶、不含半行距，常规流由
  `flushLine` 加，调用方自己居中时必须自己加）。像素实测 browser 地址
  栏文字中心 13.5 → 17.5 = 输入框中线。
- **ui / examples**：browser 地址栏「打开」改为槽位描边按钮——
  `ui.Tool("打开")`（无边框命令按钮，看着像纯文本）改为
  `ui.Slot(ui.NewButton(...).SetVariant("default"))`，动作要一眼可点。
  定下工具栏口径：命令按钮保持无边框（图标命令），「一眼可点」的动作
  按钮与纯文本提示都走槽位放描边按钮 / Label，不做伪按钮。配套：
  `.system-ui-tool-slot .system-ui-label { display: inline; margin: 0 }`
  （表单标签的块级底边距嵌进工具栏要收掉）；`Slot` 文档写明口径。
  navigate_test 断言选择器随迁到 `.system-ui-tool-slot .system-ui-btn`。

- **ui**：按钮文字垂直居中修正——核心 CSS 的按钮 `line-height` 改为公式
  推导：`控件高 − 上下边框(2) − 光学修正(1)`（新增 `$btnLH$`/`$toolLH$`
  变量，CSS 不再写 `line-height: height` 同值）。根因：`box-sizing:
  border-box` + 1px 边框下内容盒比 height 矮 2px，line-height 若等于
  height，行盒比内容盒高 2px，引擎按钮行盒居中守卫（pctH > lineH）不
  生效、行盒贴内容盒顶，文字偏低约 2px；再收 1px 是雅黑升降部不对称的
  光学修正。图标 middle 补偿随基线联动 4 → 2。像素实测：kit 主按钮
  白字中心 = 盒中线（+2 → 0），browser「打开」回到条带中线 17.5，
  三示例图标/分隔线/输入框/下拉全部 = 条带中线 ±0.5px。
- **ui**：标准表单档对齐业界口径——`ButtonHeight` 30 → 32，且
  `.system-ui-input` / `.system-ui-select` 的高度不再写死 30px，改随
  `$btnH$` 推导：页面内容区里按钮/输入框/下拉同高 32，顶底天然齐平。
  尺寸阶梯定型为两档：工具栏档 `ToolControlHeight` 28（配 ToolHeight 36，
  上下各 4px 呼吸）、标准表单档 `ButtonHeight` 32；两档互不跟随，换主题
  各调各的。像素实测 kit 画廊：输入框边框 456..487（32px）、主按钮蓝底
  32px。菜单浮层（30px 菜单项）与 textarea 多行档不受影响。
- **ui**：工具栏控件统一高度与对齐（最终口径）——新增主题字段
  `ToolControlHeight`（默认 28）：工具栏内所有交互控件（图标/文字按钮、
  槽位里的输入框与下拉）同高 28px、同用 `vertical-align: middle`，盒中点
  都落到同一条参考线上，彼此顶底齐平。配套：条带 padding 改上下不对称
  （上 −3 / 下 +3）把 middle 参考线整体抬回条带几何中线；图标保持 20px，
  middle 对符号字体的固定下沉用 `position: relative; top: −4px` 一次性
  补偿。像素实测 notepad（图标×4 + 分隔线 + 禁用态×2）与 browser（图标 +
  分隔线 + 下拉 + 地址栏）全部控件中心 = 条带中线 ±0.5px。图标按钮方形盒
  与槽位控件高不再跟随 `ButtonHeight`（页面按钮 30px 不变）。
- **ui**：默认主题紧凑化——`ToolHeight` 40 → 36，工具栏图标 16px → 20px
  （`MenuHeight` 32 不变）。工具栏上下内边距不再写死 5px，改由条高减
  控件高推导，换条高不再裁控件；基础 `.system-ui-icon` 仍为 16px（菜单
  图标、iconsheet 共用），仅工具栏规则放大。对齐口径见上一条。
- **engine**：textarea 补覆盖式滚动条——内容溢出时在内容盒右缘叠画轨道+
  滑块（8px，不占布局宽度、折行口径不变），按下即跳到点击位置、拖动跟随
  指针、抬起结束；全程不动光标不改焦点。绘制与命中/拖拽共用
  `textareaScrollbar` 单一几何源（「看到的就是能点的」）。通用容器
  （overflow 滚动）仍无滚动条，滚轮是唯一驱动源。
- **engine**：修复 textarea 首次聚焦后点击落位错误——`OnDocumentMouseDown`
  原先先聚焦（首焦把光标设到值末尾并滚动跟随）再按点击坐标映射光标，
  映射发生在已跳走的视口上，点第 1 行会把光标落到末尾附近且视口停在
  底部；现在按点击前的视口先算好目标光标、聚焦后写回，点哪落哪。
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
- **examples/stylebook**：新增「样式全集」对照页——`index.html` + 同目录
  `stylebook.css` 由 `<link rel="stylesheet">` 外链加载（`docOptions` 提供
  BaseURL + `go:embed` 加载器，引擎不碰 I/O 只回调宿主），逐节摆出引擎
  当前支持的全部标签与 CSS：行内文本语义、块级结构、语义块（无 UA 规则的
  h4~h6 与 section 等 raw/补齐两行对照）、媒体空盒、表单（含各 type 边界
  与 optgroup/fieldset 语义缺口）、表格（含 colspan 错位演示）、盒模型与
  定位（display/box-sizing/margin/padding/position/z-index）、边框圆角、
  背景阴影、文字排版、溢出、颜色与单位、选择器（含特异度与 !important
  演示）、11 个伪类、末节「不支持清单」（float/flex/opacity/伪元素/
  :nth-child/em/rem/calc 等 27 行，钉明「Chrome 侧不同是预期」）；每节带
  支持级别徽标。同一 `index.html` 可直接 `start` 进 Chrome 对照——两边读
  同一份 HTML 与 CSS。flags：`-exit-after`/`-dump-png`/`-dump-svg`/`-shot`；
  测试覆盖加载器、外链端到端（DumpSVG 颜色证据）、15 节锚点、标签计数与
  语义块两行的布局形态（raw 并排 = 无 UA 规则的护栏）。
- **examples/stylebook**：`-shot` 改为**真实字体整页截图**——按末节底边
  估页高，超过单段上限时窗口按段拉起、逐段回读再纵向拼接（SDL 渲染目标
  纹理高上限 8192，整窗拉到 8681 时其上整段黑，实测而得，段高留裕量取
  8000）。段偏移用 `body{position:relative;top:-Npx}` 平移绘制；`fixed`
  元素不随 body 平移，非末段 `display:none` 藏掉、末段放回来（同一选择器
  后写规则级联取胜，顺序即正确性）。原单屏 `saveShot` 删除，
  `rawToRGBA` 抽出供两处共用。
- **examples**：截图工具新增 `-dump-capture path.png`——把抓屏得到的桌面
  图原样落盘再照常打开遮罩，用于排查「打开就是一片灰」这类现象：灰是抓
  到的内容本身（屏幕上盖着亮色窗口）还是遮罩压暗所致（`reset()` 的
  `rgba(10,16,30,0.46)` 全屏 overlay），对照原图即可定性；配合
  `-delay 3s`（抓屏前睡眠，期间 Win+D 清屏）与 `-source`（跳过抓屏用
  指定图片）可分别隔离抓屏时机与渲染管线。
- **examples**：记事本对话框类型下拉收敛为两项——「文本文件」一项打包
  常见文本扩展名（txt/markdown/json/xml/html/htm/css/js/log/ini/csv/
  yml/yaml，分号分隔多模式走 Win32 COMDLG_FILTERSPEC 语法），所有文件
  兜底；状态栏文件类型同步认识 xml/html/css。
- **examples**：记事本状态栏口径调整——文件段不再显示文件名、改显文件
  类型（按扩展名，如 `文本文件 (*.txt)`，未保存 `*` 缀尾），文件名归
  标题栏；新增选中字数段（如 `118 个字被选中`），随选区自动刷新——引擎
  `setSelection` 本就走文档变更通知，应用层零新接口。`-shot` 另配
  `-shot-select`（截图前全选，模拟 Ctrl+A）。
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
