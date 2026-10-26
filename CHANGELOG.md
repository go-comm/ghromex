# 更新日志

版本号即 git tag（Go module 语义：`v0.3.0` 可从
`github.com/go-comm/ghromex@v0.3.0` 直接取用）。

## 未发布

**font-weight 数字权重归档修复**：
`font-weight` 解析原先只把 `bold`/700/800/900 判为粗体，`600` 等其余值一律
落成 normal——GitHub markdown 的 h1~h6 与表格 th 全部是 `font-weight: 600`，
作者的 600 覆盖 UA 的 bold 后整页标题画成细体（会议纪要页「标题那么细」的
根因）。现按 Chrome 就近选面归档：550~900 取粗体面、100~500 取正体面
（引擎只有正/粗两档字面）；`bolder`/`lighter` 按二档模型等价归粗/正；
非法值（var() 未展开等）按 CSS 语义丢弃，不再误覆盖成 normal。
回归：`TestFontWeightMapping`。

**@media 多规则块泄漏修复**：
带块 at-rule 的跳过原先只收到块内**第一个** `}`，`@media (prefers-color-scheme: dark)`
等多规则块从第二条起被提升成顶层规则应用（会议纪要页亮色背景漏出暗色表格）。
现在按大括号深度配平整块跳过（`skipCSSBlock`），块后规则照常解析。
回归：`TestParseCSSRulesSkipsMultiRuleMediaBlock`。

**表格列宽压缩的最小内容下限**：
`mins` 原先只看 col/colgroup 约束（无 col 时下限 1px），比例压缩可把列压到
内容最小宽以下——不可断 ASCII 片段被裁出格外、CJK 列压成一字宽竖排。
新增每格 1px 试排量取最小内容外宽（最宽不可断片段 + 内边距）作列下限，
跨列格按 `spreadExtra` 分摊，作者显式声明的格宽直接作下限。
会议纪要页列宽 45/360/130/77/263 对齐 Chrome 62/376/133/78/240。
回归：`TestTableColumnShrinkRespectsMinContent`。

**滚动快速路径**：
仅触发 `markScrollChanged` 时（滚轮、textarea 内滚、编程滚动），
`LayoutDocument` 跳过级联与重排，仅对子树盒子做增量平移（`applyScrollDeltas`）并重定位
`::before/::after` 生成盒。4.9 MB / 149k 节点页面滚动帧从 4.13 s 降至 **~100 ms**。

**选择器桶索引**：
CSS 级联按尾部复合选择器主键（id / class[0] / tag / 无）分桶，
`collectMatchingDecls` / `collectPseudoDecls` 同用一套索引，
把逐元素全表扫描（O(元素数×规则数)）缩为候选遍历（O(元素数×候选数)）。
同页面首轮布局从 4.2 s 降至 **~780 ms**（-80%）。

**内存优化**：
- `attrs` map 与 `element.style`（CSSStyleDeclaration）在 `newElement` 中改为惰性分配；
- `Clone` 仅在源元素真建过 style 时才复制；
- `computed` 级联对象跨轮复用，用结构体副本（`*defaultStyleTemplate`）重置而非 `newStyle`。
堆内存从 650 MB 降至 **310 MB**，sys 从 1.1 GB 降至 **550 MB**；单元素占用 4.3 KB → **2.1 KB**。

**input 长文本窗口跟随光标（scroll-follows-caret，Chrome 同口径）**：
此前单行 input 值超宽时窗口固定「舍头保尾」，光标索引能回到 0 但画面永远
滚不回首字（examples/browser 反馈"光标移动不到首字"）。改造三点：
1. `htmlElement` 新增 `textScrollX` / `textScrollSet` 记录横向窗口偏移与
   是否已被光标定位；未定位保持旧「保尾」口径（静态展示与地址栏兼容），
   已定位则纯窗口跟随。
2. `inputVisibleWindow` 按 `textScrollX` 在 rune 边界舍头；保尾模式
   （`target==maxScroll`）向上取整，保证末字完整可见；保头模式（`target=0`，
   Home/左移回首字）不再强制舍满，首字必须可见；中间位置由绘制层按内盒
   裁剪兜底（单字溢出 ≤ 1 字符宽）。
3. `ensureCaretVisible` 扩展到 input：光标越出左/右缘时把 `textScrollX`
   推回 `pre` / `pre-avail`，并在首次定位时标记 `textScrollSet=true`；
   绘制层（`render.go`）对 `text/password` input 的文本/光标/选区统一按
   padding 盒 `pushClip` 裁剪，保证右溢出字符不外漏（与旧口径 `tw≤avail`
   等价，无感知差异）。
`svg_dump.go` 同步尾部按可用宽截断，保持导出与屏幕一致。
回归：`TestInputCaretInVisibleWindow`（旧点击右缘键入落尾）、
`TestInputCaretScrollsWindow`（Home/End/聚焦三态）、
`TestInputCaretCharByCharLeftScroll`（逐字左移全程窗口跟随）。

**表格修复：UA 块级语义标签缺失导致第二份 `display:table` 塌陷**：
真实故障报告（vm02 宿主机文件系统）暴露——同一文档里第一个
`display:table` 正常、之后每个都列宽等分、行高虚胖（尾页 66 → 514）。
根因两层：

1. `section/article/aside/header/footer/main/nav/address/h4~h6` 等未收录进
   UA 样式表，按 CSS 初始值 `inline` 参与行内原子布局；第二个块与兄弟挤到
   游标右缘后吃到 `remain=0` 试排，内部表格在 1px 容器里测天然宽。补齐
   HTML5 渲染规范的 block 落位（`address` 同时补上注释早已声明却缺失的
   italic；`h4~h6` 按 Chrome 口径 16/13/11px）。
2. `layoutTableBox` 的二、三遍会把格的 computed 宽/高/盒模型/display 覆写成
   槽位值且不还原，泄漏到下一轮布局的「天然宽测量」——试排污染后无法自愈。
   现在入口快照、退出还原（pin/restore），任何中途试排过的表格重排都从
   作者声明重新量起。

回归测试：`TestDisplayTablePerSectionInstances`（同构双表格逐格一致）、
`TestDisplayTableAfterInlineAtomicTrial`（inline 试排后重排按内容量列）、
`TestUASemanticBlockDefaults`（UA 落位与字号）。

**`<pre>` / `<code>` 对齐 Chrome + `white-space` 五值**：核对并修复四组与浏览器
的口径偏差，补齐 CSS Text 的空白处理族。

- UA 不再给 `pre`/`code`/`kbd`/`samp` 写死 13px 字号：只给
  `font-family: monospace`，**字号继承父级**（Chrome 实测父级 20px → 它们就是
  20px；父级从未显式声明字号时才落到通用族默认 13px，见 `style_cascade.go`
  的 quirk）。此前写死 13px，改过字号的容器里代码会比浏览器小一档。
- 补 `tt { display: inline }`：HTML4 遗留的等宽语义标签 Chrome 仍认，引擎此前
  按「未知标签 → inline-block」渲染，段落里被原子化还会断行。
- 首换行剥离只对 `<pre>`/`<textarea>`/`<listing>` 生效（新增 `isPreStripTag`）：
  之前任何 `white-space: pre` 的元素都剥首换行，`<div style="white-space:pre">`
  比 Chrome 少一行。
- `white-space(normal/nowrap/pre/pre-wrap/pre-line)` 按 CSS Text §4.1 的三条
  正交行为实现全五值（`break-spaces` 按 `pre-wrap` 近似）：`pre` 保留空白与
  源换行、不软换行（长行溢出）；`pre-wrap` 同上但在空格处软换行、**不拆长词**；
  `pre-line` 折叠空白（含行首空白）但保留源换行、连续换行保留空行；
  `nowrap` 折叠且不换行。软换行守卫（`addItem`、原子盒、行首空格丢弃）统一
  受 `softWrap` 控制。
- 回归测试：`engine/whitespace_pre_test.go`（五值解析、字号继承、tt、空 pre
  高 0、tab 停靠 8 空格宽、首换行按标签剥离、pre 不软换行/pre-wrap 折行且不拆
  词/pre-line 折行折叠、nowrap 单行溢出、行内 code 折叠空白、pre 内 `<br>`）。
- stylebook 新增 white-space 五值演示与行内等宽语义行；README 同步。

**布局与交互补齐：`float`/`clear`、`opacity`、`visibility`、`cursor`、`font-style: italic`**：
五项此前只解析不生效（或完全没解析）的 CSS 能力，现在都有实际行为。

- `float(left/right/none)`：浮动盒脱流（不推进游标、不撑父级 auto 高度）、
  auto 宽按 shrink-to-fit 收缩，落点从当前游标向下找第一个能容纳外盒宽度的
  位置（放不下就下移到挡路浮动的最低底边）。行盒按浮动边缘收缩（左浮动抬高
  左界、右浮动压低右界），`text-align` 也按收缩后的可用行宽计算；行内元素
  内部的行盒继续避让外层浮动，inline-block / inline-table 建立新 BFC、内部
  不避让。
- `clear(left/right/both/none)`：块盒顶边下推到相关浮动下缘之下；产生 clearance
  时相邻外边距不再折叠。
- `opacity(0~1/百分比)`：无图层合成，按「子树每次绘制动作的颜色 alpha 乘
  opacity」近似（`engine/opacity.go` 的 `withOpacity` 包装 Graphics）；定位子树
  不在主 pass 递归里，按父链乘积补包（`ancestorOpacity`）。
- `visibility(visible/hidden/collapse)`：hidden/collapse 自身不绘制、不可命中，
  但照常占位；子级显式 `visible` 可恢复显示与命中。命中测试与渲染同口径。
- `cursor`：新增 `core.CursorShape` 与可选接口 `core.CursorSetter`；引擎
  `engine.CursorAt` 按命中结果给形状，browser 层在鼠标移动时调用，SDL2 后端用
  `SDL_CreateSystemCursor`/`SDL_SetCursor` 落到系统指针（按枚举值缓存句柄）。
  支持 `default/auto/pointer/text/move/wait/crosshair/not-allowed/grab/
  grabbing/ew|ns|nwse|nesw-resize`，`url()` 退回 `auto`。UA 给 `a` 设 pointer、
  表单控件设 text。
- `font-style(normal/italic/oblique)`：新增可选接口 `core.StyledPaint`
  （`Italic`/`SetItalic`），SDL2 后端用 `TTF_SetFontStyle(TTF_STYLE_ITALIC)`
  合成斜体（无真斜体字形的中文字体同样生效）。合成斜体不改变字形前进宽度，
  布局度量与正体一致；UA 给 `i/cite/em/var/dfn` 设 italic、表单控件设 normal。
- SVG 导出与屏幕渲染同构：斜体输出 `font-style="italic"`、hidden 跳过自身绘制、
  opacity 包 `<g opacity>` 组（定位层补祖先 opacity 乘积）。
- 顺带修掉一个行盒换行缺陷：间隙空格把游标顶到右边界后，换行判断
  （`right > it.x`）永久失效，整段文字粘成一行溢出容器（stylebook 的说明段落
  曾整行冲出边框）。现在间隙空格放不下就丢弃（行首空格按 CSS 折叠），超宽
  单词/原子盒之后的项也会回到新行。
- 回归测试：`engine/props_internal_test.go`（解析/简写/初始值/copyStyle）、
  `engine/float_visibility_test.go`（float 绕排与高度、clear、visibility 占位
  与命中、cursor 跟随命中、opacity 像素、满行/超宽后的换行、SVG 同构）。
- 文档：README 支持范围与已知限制、stylebook 新增对应演示节同步更新。

**每帧回调 `browser.Window.SetFrameHandler`（拖动窗口跟手）**：新增每帧回调
（事件队列排空后、布局之前调用一次，返回 `true` 表示还有每帧任务）。主循环
据此把空闲休眠从 10ms 降到 1ms（实测 `SDL_Delay(1)` 约 1.06ms），供「拖动
无边框窗口」这类高频交互把一批鼠标事件合并成一次 `MoveTo`——逐事件重定位
窗口会让它在一毫秒内跳好几次，配合 10ms 休眠就是拖动一顿一顿。`examples/snip`
的预览窗口已改用这条路径：鼠标事件只记目标位置，帧末落成一次窗口移动，拖动
期间保持 1ms 轮询；顺带把选区遮罩的不透明度从 0.46 调到 0.30（此前整屏压得
太暗，底层内容看不清）。

**列表标记样式 `list-style-type` / `list-style-position` / `list-style`**：
此前标记固定为 ul → `•`、ol → 序号，作者无法改。现在标记形态由
`list-style-type` 决定，位置由 `list-style-position` 决定：

- `list-style-type`：`disc` / `circle` / `square` / `decimal` /
  `decimal-leading-zero` / `lower-alpha` / `upper-alpha`（`lower-latin` /
  `upper-latin` 为别名）/ `lower-roman` / `upper-roman` / `none`；字母按
  双射 26 进制（27 → `aa`）、罗马数字超出 1~3999 回退十进制。
- `list-style-position`：`outside`（默认，标记右对齐到内容左缘 − 间隙）与
  `inside`（标记画在内容盒内首行行首，首行文字整体右移标记宽 + 0.5em）。
- `list-style` 简写：type / position / image 任意顺序，省略分量重置为初始值
  （type=disc、position=outside）。`list-style-image` 接受 `url()` 但不绘制
  （引擎无图片解码/加载）。
- 两者都是继承属性：作者写在 `ul`/`ol` 上即可作用于其 `li`；UA 样式表把
  `ul` 置为 disc、`ol` 置为 decimal（作者未声明时行为不变）。标记类型取
  `li` 自身的计算值，因此 `li { list-style-type: … }` 也能覆盖。
- 几何：符号类间隙 0.875em、编号类 0.6em（沿用 Chrome 实测值）；inside
  只缩进 li 自身首行——首个内容子节点是块级时标记退回 outside 绘制，
  避免压住块内容（Chrome 会把块首行整体右移，引擎不做这层传播）。
- SVG 导出同步：标记文本、位置与屏幕渲染同口径。
- 回归测试：`engine/list_style_internal_test.go`（解析/简写、标记文本、
  布局与 inside 缩进、块级子节点回退）与 `engine/list_style_test.go`
  （none 不落墨、square 画在盒外、inside 画在盒内）。
- 文档：README 支持范围与已知限制、stylebook 列表节演示同步更新。

**内阴影 `box-shadow: inset`**：此前只解析不绘制，现在与 Chrome 同口径渲染——
内阴影以 **padding 盒**为边界，阴影区是「偏移并按 spread 收缩后的形状之外」，
模糊后按 padding 盒（含圆角）裁切，因此边缘最深、向内容方向淡出。

- 几何：形状 = padding 盒收缩 `2×spread`、偏移 `(x, y)`，内圆角 = 外圆角 −
  最宽边框（负数取 0）；spread 收缩到空时整块 padding 盒都是阴影，负 spread
  把形状扩出 padding 盒时阴影不可见。
- 绘制顺序：内阴影画在背景/边框之后、内容之前（外阴影仍在背景之前）；
  多条声明按 CSS 规则「第一条在最上层」倒序绘制，内/外阴影可同时存在。
- 实现：新增 `insetShadowSprite`（掩码「形状外为 1、形状内为 0」→ 三次盒式
  模糊 → 按 padding 盒覆盖率裁切），精灵进同一张 LRU 缓存（键新增 `inset`）。
  退化路径 `paintInsetShadowFallback`：后端无 `SpriteDrawer` 时裁到 padding
  盒画形状外的实色带（忽略模糊与形状圆角）；连 `Clipper` 都没有则不画。
- SVG 导出：`mask` 挖出形状 + `feGaussianBlur` + `clipPath` 裁 padding 盒，
  与渲染路径同序（背景之后、边框之前）。
- 回归测试：`engine/inset_shadow_test.go`（spread/偏移/模糊软边/padding 盒
  裁切/多条顺序/内外共存/SVG）与 `engine/inset_shadow_internal_test.go`
  （形状几何、精灵掩码、退化路径裁剪）。
- 文档：README 支持范围与已知限制、stylebook 演示（新增内描边示例）同步更新。

**伪元素 `::before` / `::after`**：支持用 `content` 生成装饰盒（圆点、箭头、
徽标、引号等）。生成盒是宿主的真实子元素，插在 `children` 首/尾参与常规
布局、绘制、命中与 SVG 导出；但它不是 DOM 节点——`Children()` / `InnerText` /
查询 API 都看不到它，`:first-child` / `:empty` 等结构伪类也不把它算进子节点。

- 选择器：`::before` / `::after`，兼容单冒号旧写法 `:before` / `:after`；
  伪元素按类型选择器档计特异度。`p:hover::before` 的 `:hover` 按宿主状态
  匹配（悬停切换 `content` 会随重排更新）。未知伪元素（`::first-line` 等）
  仍永不匹配。
- `content`：带引号字符串（含 CSS 转义）、`attr(属性名)`、多分量拼接
  （`content: "[" attr(data-k) "]"`）；`none` / `normal` 不生成盒子；
  `content: ""` 仍生成空盒（可配 `width/height/background` 画圆点）。
  `counter()` / `url()` / `open-quote` 等取值未实现。
- 生成盒默认 `display:inline`（不是引擎 `newStyle` 的 `inline-block`），
  其余继承口径与普通子元素一致；作者可写 `display/width/height/padding/
  border/background/position` 等常规属性。
- 边界：替换元素与表单控件（`input`/`textarea`/`select`/`img`/`br` 等）
  不生成；表格结构盒（`table`/`tr`/行组/列）不生成（避免生成盒被表格布局
  误当成行/列），单元格与题注支持。样式表没有伪元素规则时整条路径跳过，
  不给常规布局增加扫描开销。
- 回归测试：`engine/generated_test.go`（源序/颜色、DOM 过滤、`attr()`、
  `content:none`、空内容盒、单冒号、`:hover::before`、未知伪元素）与
  `engine/generated_internal_test.go`（首尾挂载、content 消失后清理、空内容）。

**完整表格模型（CSS 2.1 §17）**：表格布局入口从「按标签名」改为「按 `display`
判定」——HTML 标签仍靠 UA 样式表落位，任意元素写 `display: table` 同样成网格
（Chrome 同款）。

- 新增 `display` 值：`DisplayTable` / `DisplayInlineTable` / `DisplayTableCaption` /
  `DisplayTableColumn` / `DisplayTableColumnGroup` / `DisplayTableHeaderGroup` /
  `DisplayTableFooterGroup` / `DisplayTableRowGroup` / `DisplayTableRow` /
  `DisplayTableCell`（`Display` 是公开类型，值追加不破坏既有代码）。
- 网格：colspan/rowspan 按 HTML5 放置算法落位（`rowspan=0` 延伸到网格末行，
  跨度超出末行按剩余行数钳制、**不生成幽灵行**；colspan 上限 1000）；行组顺序
  固定为「表头组 → 行组 → 表尾组」（§17.5.1.2）。
- 匿名盒：表格/行组直属单元格进隐式行；表格级块级或行内内容生成占满整行的
  匿名行；行内非格内容按连续段聚成匿名格。匿名载体不进 DOM，绘制/命中没有
  它的盒。
- 列宽：col/colgroup 的 `width`（样式或遗留属性、含 `span`）作列下限；单列格
  天然外宽定列、跨列格按比例补差；表格显式宽按比例拉伸/压缩列（不低于列下限），
  `auto` 宽收缩到可用宽。
- 行高与对齐：行等高（取行内最高格），`tr` 显式高/遗留 `height` 作下限，跨行格
  按比例补差；格盒拉伸填满行带，内容按 `vertical-align` 落位——格上的
  `baseline` 按 `middle` 处理（§17.5.2.1），遗留 `valign` 属性作回退档。
- `border-spacing`：新增 `BorderSpacing()` / `SetBorderSpacing(h, v Size)`
  （1~2 值，px/pt/dp/%）；网格四边都留间距（含外缘，Chrome 同款）。UA 给
  `<table>` 的默认是 2px，CSS 初始值是 0——`display:table` 的普通元素不带这份 2px。
  collapse 模式间距恒 0（半边框模型不变）。
- 题注：新增 `CaptionSide()` / `SetCaptionSide`（top/bottom，继承）。题注在
  **表格盒之外**（§17.4：表格盒只含网格，题注占包装盒的额外高度），宽度随表格
  边框盒——顺带修掉了 collapse 外缘边框从题注带穿过的绘制缺陷。
- `empty-cells`：新增 `EmptyCells()` / `SetEmptyCells`（show/hide，继承）；
  `hide` 只跳过背景/边框绘制（`render.go`、`svg_dump.go` 同口径），几何与命中不变。
- **解析修复**：表格标签的隐含闭合改为逐层收栈——`<tr>` 现在会同时收掉未闭合的
  `<td>` 和上一行（`<td>a<td>b<tr>...` 是常态写法，旧实现会把新行嵌进旧行）。
- 回归测试：新增 `engine/table_display_internal_test.go`（11 条：display 网格、
  UA 2px vs div 0、colspan/rowspan、rowspan=0/9999 钳制、6px 与 4px 6px 间距、
  vertical-align 三档、caption-side 上下、empty-cells 像素+标记、col 宽度、
  隐式闭合、声明宽拉伸）；既有 collapse/列对齐契约保持全绿。
- 已知边界：collapse 下不做相邻边框的「宽者胜」仲裁（取先声明格）；rowspan 跨
  行组不合并；`table-layout` 无布局语义（始终自动表格布局）。
- 文档与对照页：`examples/stylebook` 表格章节升为「支持」并补 `display:table`
  对照节、colspan/rowspan、border-spacing 6px、vertical-align；README 同步。

**修复**：外壳对页面 CSS 免疫。外壳三栏（菜单栏/工具栏/状态栏）与控件条装在
页面文档里，页面作者样式与组件样式同源，页面规则会直接漏进外壳：

- 继承属性：页面 `body` 写 `text-align:center`（提示页、落地页很常见）时整排
  工具按钮跟着居中——示例浏览器打开外部链接（提示页）即触发「顶部变居中」；
  `font-size` / `font-weight` / `color` / `line-height` / `text-decoration` /
  `white-space` / `pointer-events:none`（最后一条会让整个外壳点不动）同理。
- 全局元素规则：页面 `div{margin:8px;padding:20px;border:2px}` 会把 fixed 栏
  撑大、按钮顶偏、分隔线描边；`div{width/height}` 会改菜单/占位块尺寸；
  `div,span{display:none}` 会直接吞掉菜单项/分隔线/图标/占位块；
  `div`/`span` 的字号/字重/颜色/下划线/居中/`pre` 会改外壳文字观感（引擎宽度
  测量忽略字重，几何看不出来，只有像素能抓）。

修法：`ui/theme.go` 给外壳/控件类名加两组复位——几何（`margin/padding/
border-width/width/height`）与文字口径（`font-weight/text-align/text-decoration/
white-space`），三栏容器另复位全部继承属性。复位块写在组件规则之前，组件
自己需要的边距/字号（按钮 padding、状态轨道 `padding-right`、图标按钮居中）
由后面的同名规则覆盖，正常观感不变；1px 分隔线的 `height` 被复位块归零，
所以分隔线规则移到复位块之后。另有 7 个组件原本靠默认 `display`（菜单项/
菜单分隔线/菜单图标/快捷键/图标/上下占位块），页面 `div,span{display:none}`
会直接吞掉它们，按类名钉回 `block`/`inline`。新增
`TestChromeIgnoresHostilePageCSS`：六类敌意页面（继承属性 / div 盒规则 /
div 文字规则 / span 文字规则 / div 尺寸 / div+span 隐藏）与干净页面比三栏
几何 + **三栏像素带**逐像素一致，并断言 `pointer-events:none` 下工具栏仍可
命中；去掉复位后该测试报 `menubar 框 = {0 0 684 76}`、`菜单 框 = {0 0 144 32}`
等。`TestInstallBuildsChrome` 补三条分隔线 1px 守卫，浏览器导航测试补提示页
上的按钮位置断言。

已知边界（引擎没有 shadow DOM，无法完全隔离）：页面直接写 `!important`、
或写与组件同名的类规则（`.system-ui-btn{...}`）时仍会覆盖；页面用元素规则
给外壳内**原生控件**（`input`/`select`）设样式时，组件已声明大部分属性，
但未声明的（如 `line-height`）仍可能漏入。

**修复**：textarea 里的 `\r` 不再被当成字形画出来。Windows 文本文件与剪贴板
用 CRLF，`\r` 混进 value 后折行逻辑只认 `\n`，于是每行末尾多画一个方块，
光标列数、选区宽度也各多 1：

- 写入路径归一（与 HTML 预处理同口径）：解析 `<textarea>` 原始文本、`SetValue`、
  粘贴三条入口都先把 CRLF / 孤立 CR 折成 `\n`（`\r\n` 与 `\r` 都算一个换行），
  紧跟开标签的首个换行仍按规范忽略。
- 折行兜底：`SetAttribute("value", …)` 是底层逃生口，值原样保留，但
  `wrapTextarea` 不再把 `\r` 当字符——CRLF 的 `\r` 跳过（交给后面的 `\n`
  断行），孤立 CR 与 `\n` 同义断行。
- 回归测试：`TestTextareaNormalizesNewlines`（解析 `a\r\nb\rc` → `a\nb\nc`
  且绘制为三行、`SetValue("x\r\ny")` 的值与光标 2,2、直接写属性 `p\r\nq`
  绘制不出 `\r`）；`TestPasteLineBreakHandling` 补 textarea 粘贴 CRLF 断言。
- 示例 notepad 补文件层换行保持：打开时检测原文件风格（CRLF/LF/孤立 CR，取
  多数派，平票取先出现的），编辑区内部统一 LF，保存时按检测结果写回；新建
  文档用平台默认（Windows CRLF，其它 LF）。「打开 CRLF 文件 → 编辑 → 保存」
  不再把用户文件悄悄改成 LF。状态栏末段实时显示当前风格（`CRLF` / `LF` /
  `CR`，VS Code 同款），并收窄了文件/位置段宽度给它让位
  （`examples/notepad/eol.go`，新增 `TestSavePreservesLineEndings` /
  `TestDetectEOL`）。

已知边界：单行 `input` 的 `\r`/`\n` 未做归一（Chrome 直接 strip；粘贴路径
已折成空格）；直接 `SetAttribute` 写入的值仍原样保留 `\r`（只是不画出来），
`GetValue` 如实返回；notepad 对混排文件按多数派统一，不做逐行保留。

## v0.3.0 — 2026-10-25

ui 组件库成型，notepad 与 browser 两个示例改到全程只用 `ui` 控件，新增
stylebook（样式全集对照）与 iconsheet（图标表）两个示例。

**破坏性变更**

- 命名：工具栏构造器 `ui.Button` → `ui.Tool`；`ui.Shell` → `ui.Workbench`
  （`App.Shell()` → `App.Workbench()`）；组件类名 `ui-*` → `system-ui-*`，
  样式自动挂到 `html.system-ui` 根作用域。`Install` 语义不变，替换名字即可。
- 尺寸口径定型为两档：工具栏档新增 `Theme.ToolControlHeight` 28（`ToolHeight`
  40 → 36、工具栏图标 16 → 20），标准表单档 `ButtonHeight` 30 → 32
  （input/select 随其同高）；按钮 `line-height` 改由公式推导，文字真正垂直居中。
- 渲染默认值：窗口**默认走内存渲染**，GPU 加速需显式 `GHROMEX_GPU=1`。
- 接口与 flag：`BufferGraphics.SavePNG` 更名 `DumpPNG`；移除示例
  `-dump-buffer-png`，无头出图统一 `-dump-svg`（几何）/ `-dump-png`（像素），
  真实窗口截图走 `-shot`（`Window.Screenshot`）。

**引擎**：原生 `title` 提示气泡（悬停 600ms 弹出、`TickTooltip` 推进、配色经
`SetTooltipColors` 按文档下发）、`pointer-events`、`RemoveAttribute`、textarea
覆盖式滚动条与首次聚焦点击落位修复、表单 `input` 文字垂直居中、`display:none`
矩形彻底空掉、未布局 textarea 的 `CaretPosition` 退回按硬换行计算。

**ui**：页面控件全套（`NewLabel` / `NewTextView` / `NewButton` / `NewCheckbox` /
`NewSelect` / `NewTextInput` / `NewTextArea` / `NewField`）、控件条 `NewBar`、
工具栏槽位 `ui.Slot`、`Spec.NoBodyReset`、运行时换肤 `SetTheme`、
`SetStatusVisible`；状态栏收敛到统一段模型（左侧分段 + `AnchorRight` 右钉段 +
可点段 `Action` + 多右项按 Σ 外扩预留）；新增 `Field` 令牌并重排深色主题层次。

**示例**：browser 外壳从手写工具条（绝对坐标 + 换页重排）改为 `ui.Install` 装配；
notepad 页面改由 Go 构造（删 `index.html` 与 `go:embed`）并给全部命令配图标；
kit 新增「基础控件」卡片；删除 form / login / scroll（回归测试改用内联夹具）。

以下条目为 v0.3.0 的逐条明细，保留以便升级对照。

- **ui**：状态栏段可带事件（VSCode 式可点项）——`StatusSegment.Action.Run`
  非空即挂 `has-action` 类（悬停/按下换主题 `$hover$/$press$ 中性底色，与
  工具栏同款令牌）并绑定点击：走与菜单项、工具栏按钮完全相同的命令分发
  （关菜单、失焦恢复、ErrQuit 退出、失败原因写进状态栏）。新增 `Title`
  悬停提示（原生 title 气泡，定宽段文字被裁时尤其有用）。纯文本段不挂类
  不绑事件，零交互成本。kit 右侧两个右钉段接 `Action` 演示；新增
  `TestStatusSegmentAction`（类名契约/title/坐标命中恰好触发一次/纯文本段
  不误触）、`TestStatusSegmentActionError`（失败进状态栏）、
  `TestStatusSegActionHover`（像素守卫：悬停=Hover 令牌、移开回 Surface）。

- **ui**：状态栏支持多个右项——右钉段全部显式定宽时，右区宽度与左轨道
  垫宽按「Σ段宽+分隔线」自动外扩（原先装不下的部分会被 `overflow:hidden`
  裁掉）；含自适应段则回退 `Theme.StatusRightWidth` 基准预留。贴边口径
  补齐：糖消息缺位时最右段带 `is-last` 收掉右内边距贴 `right:12px` 盒缘
  （与左区 `is-first`、糖消息同为 12px），糖消息在场时让位留 10px 间隙；
  右钉段零值对齐按右区口径写实为右对齐。kit 状态栏改为两个定宽右钉段
  演示（Σ=351 外扩），新增 `TestStatusRightZoneWidth` 守卫（外扩/回退/
  内联清理/盒几何/is-last），`TestStatusRightSegment` 补贴边位让位回收断言。

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
