# 已知限制

> 本文件是 [README](../README.md) 的分册；接口信息用 `go doc github.com/go-comm/ghromex/<包>` 查。

- **绑定层的结构体大小必须与运行库一致**：SDL 的结构体按值跨越 DLL 边界（Go 侧申请内存、SDL 按自己的大小写），估小了就是**越界写坏调用者的栈**。实例：`SDL_RendererInfo` 曾按「name + flags + num_texture_formats + 指针」估成 32/40 字节，而运行库（SDL 2.28.4）写的是 **88 字节**（`texture_formats` 是**内联数组** `Uint32[16]`，不是指针）——越界 48 字节，调用之后渲染器句柄变成 0、随后空指针崩溃。现在按真实布局定义，并有测试逐个量（缓冲区填 0xAA，看最后一个被改写的字节）断言「运行库写入长度 ≤ Go 侧结构体大小」：`SDL_RendererInfo` 88/88、`SDL_Event` 56/56、`SDL_SysWMinfo` 32/32。同类问题也可能出现在别的跨界结构体上（`SDL_Surface`/`SDL_Texture`/`SDL_AudioSpec`/`GUITHREADINFO` 等——`GUITHREADINFO` 就有：末字段 `rcCaret` 是 RECT 16 字节，写成 int32 会让结构体少 8 字节，运行库越界写），新增按值跨界时要**先量再写**。另外，C 侧地址（uintptr）转可解引用的指针统一走 `cMemPointer`（`backend/sdl2/sdl2.go`）：把位模式原样搬过去，**不做任何指针算术**；取地址的局部变量按 Go 的地址性规则一定在内存里，实测热循环一千万次读回一致。这个工具**只用于 C 侧内存**（C 内存不受 Go GC 管），并且**不提供任何 GC 保活**——Go 堆指针必须直接以 `unsafe.Pointer` 传递（`uintptr` 不是指针，`runtime.KeepAlive` 也救不了「已经被当成指针用的 uintptr」）。它是绕过 `go vet` 的 `unsafeptr` 检查的**唯一**入口，注释里已注明这是针对 C 内存的已知误报
  > 注：`b8b4817` 的 commit message 把上面这件事归因为「DLL 越界写」，**那是错的**——越界的是我们 Go 侧的结构体定义，DLL 写的就是它自己的结构体。以 `27ce6ea` 为准。

  **跨界结构体清单**（按值传给 C 函数的必须与头文件逐字段一致；不透明指针只
  传指针 ✓ 不受布局影响）：

  | 结构体 | 跨界的形式 | 大小 | 关键偏移（64 位） | 状态 |
  |---|---|---|---|---|
  | `SDL_RendererInfo` | 运行库写回 | 88 | name@0 / flags@8 / num_texture_formats@12 / texture_formats@16（**内联数组 Uint32[16]**，不是指针）/ max_texture_width@80 / max_texture_height@84 | 已核对 ✓ |
  | `SDL_Event` | 运行库写回 | 56 | type@0 / timestamp@4 / windowID@8 / 各事件私有字段（解码按偏移，真实事件验证过 ✓） | 已核对 ✓ |
  | `SDL_SysWMinfo` | 运行库写回 | 32 | version@0（SDL_version 3 字节 + 1 填充）/ subsystem@4 / info.win{8,16,24} | 已核对 ✓ |
  | `SDL_Rect` | **按值传参** | 16 | x@0 / y@4 / w@8 / h@12 | 已核对 ✓ |
  | `GUITHREADINFO` | 运行库写回 | 72 | cbSize@0 / flags@4 / hwndFocus@16 / rcCaret@56（**RECT 16 字节**，写成 int32 会少 8 字节 → 越界写） | 已修 ✓ |
  | `GUID` | 按值传参 | 16 | Data1@0 / Data2@4 / Data3@6 / Data4@8 | 已核对 ✓ |
  | `COMDLG_FILTERSPEC` | 按指针传 | 16 | 两个 LPCWSTR @0/@8 | 已核对 ✓ |
  | `SDL_Surface` / `SDL_Texture` / `SDL_Color` / `SDL_Point` / `SDL_AudioSpec` | 不透明指针 / 不涉及 | — | 库内只传指针 ✓ 不受布局影响 | 无风险 ✓ |

  新增按值跨界的东西前，先照着这张表**量一遍**（填 0xAA 看运行库写了多少字
  节）+ 抄头文件对偏移 ✓
- **`-race` 跑不了（已知缺口）**：race 检测需要 cgo（C 工具链），本仓库的 Windows 纯 Go 环境没有 gcc。替代路径（谁有条件谁跑）：`CGO_ENABLED=1 CC=tcc go test -race ./...`（tcc 约 1MB）或 `docker run --rm -v $PWD:/src -w /src golang:1.24 sh -c "apt update && apt install -y gcc && CGO_ENABLED=1 go test -race ./..."`。注意那只覆盖**纯 Go 包**（engine/core/runtime/browser 的非 Windows 标签部分）；`backend/sdl2` 与 browser 的窗口类测试是 `//go:build windows`，需要真实窗口，只能在 Windows 上跑。引擎是单线程设计（事件派发与每帧 `TickScroll` 同一 goroutine），本地按 goroutine 逐处核对过，但「核对过」不等于「跑过 race」——这个缺口记在这里，别当它不存在
- 布局为块级 + 行内/inline-block 流式布局 + 定位子集；无 flex/grid；**textarea 有覆盖式滚动条**（内容溢出即显示，轨道点击跳转 + 滑块拖拽，见 engine/textarea.go），**文档级滚动同样有覆盖式滚动条**（视口右缘/下缘溢出即显示：滑块位置跟随视觉偏移、轨道点击跳转、拖拽立即到位不缓动，层序在定位层之上 select 浮层之下，见 engine/viewport_scrollbar.go；hover/光标仍按底下内容响应；宿主有固定栏时可用 `SetViewportScrollbarInsets` 把纵条收进两栏之间、`SetViewportHorizontalScrollbar` 关掉横条——ui 工作台两者都开了，滚动条只服务页面内容区），通用容器滚动仍仅滚轮驱动（无滚动条 UI、无 PageUp/PageDown 翻页键、无触控板惯性），wheel 步长 100px/档（与 Chrome/Windows 一致）；文档级滚动为**动画式**：**基于时间的缓动 + 目标位置累加**（Chrome 同款）——滚轮只把目标（`scrollTop`/`scrollLeft`）累加一档，每帧按「已推进的动画时间 / 150ms」求进度 t∈[0,1]，位置 = 起点 + (目标 − 起点) × easeOutQuad(t)；时长用**墙钟时间**算而非帧计数（高刷屏不会滚太快、掉帧不会变速），不用「固定帧数每帧加 11px」（那样在高刷/掉帧时会变速）；连续滚动时新的一档目标继续累加、缓动段从**当前视觉位置**重新起算（位置连续、不跳）；60Hz 下一档 100px 的累计位移是 21.0、39.5、55.6、69.1、80.2、88.9、95.1、98.8、100（每帧增量 21、19、16、13、11、9、6、4、1，单调不增——可见位移按「每帧位移取整」而非「累积位置取整」，后者会在 floor/ceil 之间来回跳，看着像抖）；**两轴各自计时**（frame clock 与缓动进度都分轴：只滚纵向不会把横向的动画重新计时，也不会截断横向的帧间隔——触控板 momentum 时 wheel 比帧还密，共用时钟会让另一轴几乎停住）；单帧推进量有上限（20ms 动画进度：卡顿的一帧——新内容进视口要现建纹理/字模，可能上百毫秒——不会让内容跳过一整段距离；代价是**低于 50Hz 的屏上动画会被拉长**，45Hz 约 165ms、30Hz 约 250ms）；滚动只平移子树——**动画帧不重排**（`engine.TickScroll` 按增量平移已布局的盒子，级联与布局只在内容变化时跑）；绘制时按**视口裁剪**（完全落在视口外的盒子不画，滚动时页面下半部分不再白画一遍：portal 页每帧绘制调用 2624 → 1014）；字体路径解析与文本度量已缓存（文本度量按「文本+字号+粗细+family」），examples/ua 单次重排约 1~2ms。但滚动偏移无对外 DOM API（`scrollTop`/`scrollLeft` 未导出，仅由 wheel 与 textarea 光标跟随内部驱动），亦无 `scroll`/`wheel` 之外的滚动事件；引擎是**单线程**的（事件派发与每帧的 `TickScroll` 必须在同一 goroutine，否则要自行加锁）
- position v1 边界：absolute 双锚（left+right 同给）时 auto 宽按内容收缩而非拉伸；未实现嵌套层叠上下文（负 z-index 压至祖先背景之下；`opacity` 不做图层合成，见下条）；命中测试未按 z-index 取最上层元素（select 选项浮层例外，按浮层层序优先）
- `opacity` 是**近似**：引擎没有离屏图层/分组合成，实现为「子树里每次绘制动作的颜色 alpha 乘 opacity」——纯色背景、边框、文字、阴影的观感与浏览器基本一致，但**重叠内容会各自透出下层**（真实 CSS 是整个子树先合成成一张图再按 opacity 混合，重叠处不该叠出更深的颜色），半透明子元素叠加时偏差更明显。SVG 导出走 `<g opacity>`，语义与浏览器一致（导出比屏幕更准）
- `float`/`clear` 的作用域是**每个父元素自己的块格式化上下文**：子元素内部产生的浮动只影响该子元素自己的行盒，不向父级的兄弟传播（真实 CSS 里子块未建立新 BFC 时浮动会外溢、影响后续兄弟与祖先高度）。另外浮动前的待排内容会先结束当前行（浮动总从新行开始），`float` 与 `display:inline-block` 同款 shrink-to-fit 宽度；浮动不参与外边距折叠
- 行内盒的基线：inline-block 的基线取「盒底 − 字体 descent」（内容高度 = 行高时与 Chrome 的「最后一行基线」同值）；**定高盒装不下内容**（文字折行溢出盒外）时，Chrome 的基线是**最后一行**的基线（CSS 2.1 §10.8.1），引擎仍取盒底 → 同行的后续盒不会被推下去（差一整个行高）
- 伪类实现了常用的一组加函数型（`:hover`/`:focus`/`:active`/`:checked`/`:disabled`/`:enabled`/`:first-child`/`:last-child`/`:only-child`/`:root`/`:empty`，以及 `:not(选择器列表)`/`:nth-child(An+B)`——参数在选择器解析期预解析，`:is()`/`:nth-of-type()` 等其余函数型伪类未实现，参数非法（`:not()` 空参、`:nth-child(2x)`）按 CSS 语义使整条选择器无效、永不匹配），伪元素实现了 `::before`/`::after`（`content` 支持字符串与 `attr()`；`counter()`/`url()` 等未实现，未知伪元素一律不匹配）；`:hover`/`:active` 沿父链继承但**不跨 shadow DOM/iframe 边界**（引擎无这些概念）；焦点扩展到 button/链接后，**Tab 仍只在可编辑输入之间循环**（浏览器会把按钮也纳入 Tab 序——引擎保留原行为以免打断记事本的「Tab 在文本框间移动」习惯）；悬停态变化会触发**整页重排**（级联是全量重算，见下条），大页面 + 高频鼠标移动时开销明显；`:focus` 只改样式，**按钮/链接不画焦点框**（引擎只给可编辑控件画蓝框）
- `<input>` 可编辑仅支持 text/password 类型；radio/checkbox 无键盘 Space 切换、无 disabled / indeterminate / 表单重置；无剪切（Ctrl+X）、无按词扩展选区（Ctrl+Shift+方向键）
- `<select>` 仅鼠标交互（展开/收起/选中），无键盘上下键选择、无 multiple、无 optgroup；选项浮层不随视口裁剪（超出视口的选项画到窗口外）；收起态按内容收缩（文字 + 箭头位 16px，与 Chrome 同口径）
- `<textarea>` 无剪切（Ctrl+X）、无按词扩展选区（Ctrl+Shift+方向键）；滚轮可滚动（超盒高的行随滚动偏移平移、半截行按裁剪截断；后端未实现 `Clipper` 时退化为整行丢弃）
- 页面文本选区：无双击选词 / 三击选行，拖选越出视口不会自动滚动；复制按「最近块级祖先变化即换行」处理，`<br>` 不产生文本节点，跨 `<br>` 复制不会插入换行；`input` 内的文本仍走控件自己的选区模型（两者互斥）
- 行高（`line-height: normal`）由 SDL_ttf 的 `TTF_FontAscent`/`TTF_FontDescent`
  计算（两者都是 **int32 有符号**返回值，`TTF_FontDescent` 为负——不按 int32 重解释
  会被当成巨大正数再钳成 0，所有字体的行盒就会短 2~3px）
  计算，与 Chrome/DirectWrite 可能有 1~4px 差异，不再是逐像素同口径；且布局
  全程为整数像素，浏览器的小数边距/行盒（如 h3 的 `margin: 1em` = 18.72px、
  h2 的 0.83em = 19.92px）只能取整，逐段累积后整页可漂移数像素；
  空块自折叠未实现（父-子穿越 margin 折叠已实现）
- `<img>`：支持 PNG / JPEG / GIF（只解**第一帧**，无动画）；未加载或加载失败时是
  0×0 的空盒（Chrome 会画 alt 文字 / 裂图图标，引擎没有 alt 回退），也没有
  `srcset`/`sizes`、`exif` 旋转与 `image-rendering`；位图只按目标矩形拉伸（缩放算法由
  后端决定：SDL2 交给 `SDL_RenderCopy` 的纹理过滤，`BufferGraphics` 是最近邻），
  不做九宫格/裁剪填充
- 窗口图标：只吃**已解码的位图**（`*core.Bitmap`），不认 `.ico` 这类多帧容器
  （一个 `.ico` 里各档尺寸 + key 色透明，各平台规矩不一；宿主想用自己的图标文件，
  先取出其中一档像素、`engine.DecodeImage` 解出来再传）；也不做 exe 资源图标（打包期
  的事，需外部资源工具链）。建议给 32×32 或 64×64：系统按 `SM_CXICON`/`SM_CXSMICON`
  两档缩放，太小放糊、太大后端多一道缩放
- 背景图（`background-image` + 配套长写）：只做 `url()` 与 `none`，渐变 `<image>`、
  `image-set()`、`element()` 等整条声明忽略（按 CSS 语义「非法即丢弃」）；不做
  `background-attachment: fixed`（视口固定）；`background-repeat` 的 `space`/`round`
  退化为普通平铺（不做等距铺开与整倍数缩放）；圆角元素上的背景图仍按「角标补盖」
  近似（引擎的裁剪栈只有矩形），不是真正的圆角遮罩
- 圆角裁剪（`overflow:hidden` + `border-radius`）的角区用「盒后底色」补盖实现：背后不是纯色（渐变/图片）时是近似；后端不支持 `engine.SpriteDrawer` 时不补盖（退化为矩形裁剪）
- 裁剪仅由实现 `engine.Clipper`（`PushClip`/`PopClip`）的后端生效（当前 `BufferGraphics`、SDL2 均已实现；自定义后端未实现时静默跳过裁剪、仅不裁剪不报错）
- 边框只画**实线**：`inset/outset/groove/ridge/dashed/dotted/double` 都退化为
  solid。四边的宽度/颜色/样式已独立支持；直角边框的角按外角到内角的斜线分色
  （miter，两邻边宽度不同时斜线也按宽度比例走），圆角处两邻边的颜色按 45°
  硬分界近似（Chrome 是渐变过渡），因此 Chrome 的 `input`（2px inset）、`button`
  （2px outset #000）、`hr`（1px inset，上暗 #808080 下浅）仍只做到外盒尺寸与
  单边颜色一致，线型明暗差异保留（对照页 examples/ua 的 hr 引擎画成两行同色）
- `box-shadow` 支持外阴影与内阴影（`inset`）。偏移/模糊/扩散只接受 px/pt/dp
  （偏移百分比不支持），模糊用三次盒式模糊近似高斯。外阴影按 CSS 规则裁在
  元素边框盒之外（背景透明时盒内也不会透出阴影）；内阴影以 **padding 盒**为
  边界：阴影区是「偏移并按 spread 收缩后的形状之外」，模糊后按 padding 盒
  （含圆角）裁切——边缘最深、向内容淡出。大盒子（精灵超过约 70 万像素）或
  后端不实现 `engine.SpriteDrawer` 时退化：外阴影用圆角矩形多层近似（后端
  实现 `engine.Clipper` 时按四条裁剪带把边框盒挖掉；两者都不支持时盒内可能
  透出阴影）；内阴影退化为 padding 盒内形状之外的实色带（忽略模糊与形状
  圆角），后端连 `Clipper` 都没有时不画内阴影
- 字体：generic/具名映射基于 Windows 自带字体文件，缺失时逐级回退 standard；
  无 webfont 加载；斜体（`font-style: italic/oblique`）用 SDL_ttf 的合成斜体
  （`TTF_SetFontStyle(TTF_STYLE_ITALIC)`）——中文等无真斜体字形的字体靠倾斜
  变换得到，观感比浏览器（同样合成）略硬，且斜体**不改变字形前进宽度**（度量
  与正体一致）；`monospace`/`cursive` 映射到 simsun.ttc 的 face 0（SimSun），Chrome 用的是
  同一文件里的新宋体（NSimSun）——行高与整串宽度一致，但单字 ink 高度与
  短串宽度有微差（对照页 `<code>元素</code>` 等宽串 61 vs Chrome 63、ink 高 15 vs 13）
- 表格是**完整模型**（CSS 2.1 §17），角色只由 `display` 判定：HTML 标签靠 UA
  样式表落位到表格家族，普通元素写 `display: table` 同样进网格布局。已支持：
  - 网格与匿名盒：colspan/rowspan（HTML5 放置算法；`rowspan=0` 延伸到网格末行，
    超出末行按剩余行数钳制、**不生成幽灵行**，colspan 上限 1000）；表格/行组里的
    块级或行内内容按 CSS 生成匿名行/匿名格，行内非格内容按连续段聚成匿名格；
  - 列宽：col/colgroup 的 `width`（样式或遗留属性、含 `span`）作列下限，单列格
    天然宽定列、跨列格按比例补差；格的**百分比声明宽按列外宽解析**（含格自身
    padding/border，同 Chrome 表格语义——声明宽合计 100% 不会被 padding 撑破
    容器），像素声明宽按外宽作硬下限（作者声明的格宽不压下去）；表格显式宽按
    比例拉伸/压缩列（不低于列下限，列宽和更大时按和撑宽），`auto` 宽收缩到可用宽；
  - 行高与对齐：行等高（取行内最高格），`tr` 显式高/遗留 `height` 作下限，跨行格
    按比例补差；格盒拉伸填满行带，内容按 `vertical-align` 落位（格上的 `baseline`
    按 `middle` 处理，§17.5.2.1；遗留 `valign` 属性作回退档）；
  - `border-spacing`：1~2 值，网格**四边都留**（含外缘，Chrome 同款）；UA 给
    `<table>` 的默认是 2px，CSS 初始值是 0（`display:table` 的普通元素不带这 2px）；
  - 题注：`caption-side(top/bottom)`，题注在**表格盒之外**（§17.4，表格盒只含网格，
    宽度随表格边框盒）；`empty-cells: hide` 只隐绘制（几何/命中照旧）；
  - 行组顺序：表头组 → 行组（DOM 序）→ 表尾组，与源码顺序无关（§17.5.1.2）；
  - `border-collapse: collapse` 的半边框模型保持：单元格只画上/左、外缘右/下由表格
    补画（实测紧贴无间隙、每行只摊 1px 线，对照页表格 181×112 vs Chrome
    181.75×112，单元格外盒 90×30 vs 90.38×30）。
  已知边界：collapse 下不做相邻边框的「宽者胜」仲裁（取先声明格）；rowspan 跨行组
  （thead→tbody）不合并；`table-layout` 无布局语义（始终自动表格布局）；列宽取
  **内容最大值**（不做浏览器的自动表格布局分配，Chrome 带小数像素）
- 列表：缩进/外边距/标记已实现并与 Chrome 对齐（ul 的 •、ol 的序号；标记位置按内容左缘 − 间隙近似，符号类 0.875em / 编号类 0.6em）。`list-style-type`（disc/circle/square/decimal/decimal-leading-zero/lower|upper-alpha/lower|upper-roman/none）与 `list-style-position`（outside/inside）及 `list-style` 简写已支持；仍无 `display:list-item`（标记只对 ul/ol 的 li 子元素生成）与 `list-style-image`（`url()` 只解析、不绘制）；`inside` 只缩进 li 自身的首行，li 首个内容子节点是块级时标记退回 outside 绘制（Chrome 会把块首行整体右移）
- `white-space` 五值已齐（`normal/nowrap/pre/pre-wrap/pre-line`）；`break-spaces` 按
  `pre-wrap` 近似（区别只在行尾空格细节）、`tab-size` 固定 8 个空格宽；`line-height` 不支持 `em`/`rem`/`vh` 等单位
  （只认无单位数字、百分比与 px；其余整条声明忽略，保留继承来的值）；
  `vertical-align` 的 `top`/`bottom` 与行盒最终高度互相依赖，引擎迭代两轮收敛，
  极端组合（同一行里多个 top 盒且高度各异）可能与浏览器差 1~2px
- 级联为每次重排全量重算：逐元素重跑规则匹配，按尾部复合选择器主键索引查候选桶、再按完整特异度排序（命中候选仍是元素数 × 规则桶数 × 祖先深度）——大样式表 + 大文档时每次输入都会触发一次全量级联，尚未做失效缓存（字体路径与文本度量已有缓存，见「滚动」条；类名匹配已改为零分配扫描）
- 灰度 AA（默认）无横向子像素分解，竖笔边缘锐度低于 LCD/ClearType（物理极限）；
  软硬由 hinting/tone 曲线档位决定（`GHROMEX_HINT` 默认 normal、`GHROMEX_CONTRAST`
  默认关，可联调取点，见渲染段），保真度探针证实管线忠实（见
  [css-support.md](css-support.md) 的渲染段；
  需要横向锐度可 `GHROMEX_LCD=1` 开 LCD 子像素）
