# 已支持的 CSS 子集

> 本文件是 [README](../README.md) 的分册；接口信息用 `go doc github.com/go-comm/ghromex/<包>` 查。

`display(none/inline/inline-block/block，以及完整表格家族：table/inline-table/
table-caption/table-column(-group)/table-header-group/table-row-group/
table-footer-group/table-row/table-cell)`、`width/height`、
`box-sizing(content-box/border-box，表单控件 UA 默认 border-box 对齐 Chrome)`、
`margin*`（相邻块级兄弟折叠：取较大者；父-子穿越/空块折叠未实现）、`padding*`、
`border`/`border-top/right/bottom/left` 简写、`border-width/color/style`（1~4 值展开）
与 12 个单边长写（四边可独立宽度/颜色/样式）、`border-radius`(四角统一，仅水平半径)、
`border-collapse(collapse/separate)`、`border-spacing(1~2 值，px/pt/dp/%)`、
`caption-side(top/bottom)`、`empty-cells(show/hide)`、
`list-style-type(disc/circle/square/decimal/decimal-leading-zero/lower|upper-alpha/
lower|upper-roman/none)`、`list-style-position(outside/inside)`、`list-style` 简写
（`list-style-image` 接受但忽略，标记仍按 type 画）、
`box-shadow`（外阴影与内阴影 `inset`；逗号分隔多条、px/pt/dp 偏移/模糊/扩散、
颜色，第一条在最上层；内阴影按 padding 盒裁切并支持 `border-radius`）、
`background-color`、`background-image`（`url()`/`none`，逗号分隔**多层**，列表首层画在
最上面；引用按文档 BaseURL 解析，字节仍走宿主 `LoadImage`，`data:` 内建解码）、
`background-repeat(repeat/repeat-x/repeat-y/no-repeat，双值写法横纵分轴；space/round
退化为普通平铺)`、`background-position(轴向关键字/长度/百分比/「贴边再偏移」如
`right 10px bottom 20px`，单值另一轴补 center)`、`background-size(auto/cover/contain/
长度/百分比，百分比相对定位盒)`、`background-origin`/`background-clip`（border/padding/
content-box 两套独立口径：前者定定位盒、后者定裁剪盒）、`background` 简写（颜色/图/
position/斜杠 size/repeat/box/attachment 自动归类，未写出的分量重置回初始值）；各长写
值表按 CSS 的**循环表**口径对齐到层。背景图到货只涨重绘计数（不重排）；渐变 `<image>`
与 `background-attachment: fixed` 未实现，这类声明按 CSS 语义整条忽略；`space`/`round`
两种铺法退化为普通平铺；圆角裁切下背景图仍按「角标补盖」近似，不是真正的圆角遮罩）、
`color`、`font-size`、`font-weight(bold)`、`font-family`、
`text-align(left/center/right)`、`text-decoration(underline/line-through/none，随父链继承)`、
`white-space(normal/nowrap/pre/pre-wrap/pre-line)`（CSS Text §4.1 的三条正交行为：
`pre` 保留空白与制表符、按源换行强制断行、不软换行（长行溢出）；`pre-wrap` 同上但
在空格处软换行（不拆长词）；`pre-line` 折叠空白（含行首空白）但保留源换行；
`nowrap` 折叠空白且不换行；`break-spaces` 按 `pre-wrap` 近似；UA 的 `pre` 用 `pre`，
行内 `code`/`kbd`/`samp`/`tt` 仍是 `normal`）、
`line-height(normal/倍数/百分比/px)`（行距一半加在文字之上、一半加在之下，行盒高度
严格等于声明值；倍数与百分比相对**元素自身**字号解析，随父链继承的是**原始形态**
——父级 `1.5` + 子级 `font-size:32px` → 子级行高 48px）、
`vertical-align(baseline/top/middle/bottom/text-top/text-bottom/sub/super/长度/百分比)`
（不继承；只作用于行内级盒——引擎里行内元素都是「原子盒」，`middle` 的半个 x 高度
按字号 0.52 估，SDL_ttf 无 x 高度接口；`super`/`sub` 的位移量取 Chrome 实测的
「父字号/3+1」「父字号/5+1」，`<sup>`/`<sub>` 即走这条 UA 规则）、
**伪类**（`:hover`/`:focus`/`:active`/`:checked`/`:disabled`/`:enabled`/
`:first-child`/`:last-child`/`:only-child`/`:root`/`:empty`；伪类按 class 档计
特异度。`:hover`/`:active` 沿父链继承——鼠标停在子元素上时祖先同样匹配；
`:focus` 只落在焦点元素上。动态伪类的状态存在文档上，状态变化走
「变更通知 → 重排重绘 → 级联重算」，与浏览器的链路一致；
函数型伪类 `:not(选择器列表)`（参数列表任一命中即排除，特异度取参数
最高者）与 `:nth-child(An+B)`（odd/even/整数/2n+1 全语法，只数元素兄弟）
已实现，参数在解析期预解析；`:is()` 等其余函数型伪类与非法参数按
CSS 语义整条选择器无效，永不匹配）、
**伪元素** `::before`/`::after`（单冒号旧写法 `:before`/`:after` 等价；
`content` 支持带引号字符串、`attr(属性名)` 与多分量拼接，`none`/`normal`
不生成盒子；生成盒默认 `display:inline`，可写 `width/height/padding/
border/background/position` 等常规属性，参与布局、绘制、命中与 SVG 导出，
但不出现在 `Children()`/`InnerText`/查询结果里——不是 DOM 节点；
`counter()`/`url()`/`open-quote` 等取值未实现）、
`position(relative/absolute/fixed)`、`top/right/bottom/left`(px 与 %，支持负值；
absolute/fixed 相对包含块锚定，right/bottom 回推)、`z-index`（非 0 的定位元素与 fixed 元素建立层叠上下文；z:auto 与 z:0 暂不区分）、
`overflow(visible/hidden/clip/scroll/auto：单值作用两轴、两值为 overflow-x overflow-y、
overflow-x/overflow-y 分轴；hidden 只裁不滚，scroll 恒可滚，auto 溢出时可滚，
文档级滚动不受 html overflow 关键字约束)；
`float(left/right/none)`/`clear(left/right/both/none)`（浮动盒脱流、auto 宽按
shrink-to-fit 收缩，行盒按浮动边缘收缩、块盒按 `clear` 下推到浮动下缘之下；
浮动不撑开父级 auto 高度——与 Chrome 同口径，作用域简化见 [limitations.md](limitations.md)）、
`opacity(0~1 数字/百分比，越界钳到 [0,1])`（不做图层合成，按「子树每次绘制
动作的颜色 alpha 乘 opacity」近似，见 [limitations.md](limitations.md)）、
`visibility(visible/hidden/collapse)`（hidden/collapse 自身不绘制、不可命中，
但照常占位；子级显式 `visible` 可恢复显示与命中）、
`cursor`（系统指针子集：`default/auto/pointer/text/move/wait/crosshair/
not-allowed/grab/grabbing/ew|ns|nwse|nesw-resize`；`url()` 自定义光标退回
`auto`，逗号后的回退关键字优先。SDL2 后端落到系统指针，无头/软件光栅化忽略）、
`font-style(normal/italic/oblique)`（italic/oblique 都走 SDL_ttf 的合成斜体，
不改变字形前进宽度——布局度量与正体一致，只有绘制与 SVG 导出体现倾斜）、
inline-block 的百分比宽度按包含块内容宽解析（而非行内剩余宽）；
UA 默认样式表覆盖浏览器 Windows 侧的常用规则：body 外边距 8px、
h1-h3 字号/外边距、p 段距、a 蓝色下划线、strong/b 加粗、
em/i/cite/var/dfn 给 `font-style: italic`、`a` 给 `cursor: pointer`、表单控件给
`cursor: text`/`font-style: normal`；em/i/code/small/sub/sup 等行内语义标签为 inline（sub/sup 另按 Chrome 实测做
基线位移：上标 `父字号/3+1`、下标 `父字号/5+1`，字号取 smaller 的 13px）、
dt **不加粗**（Chrome UA 实测计算权重 400；HTML4 的 UA 才加粗 dt）、
dd 缩进 40px、ul/ol 缩进 40px、blockquote/pre/figure 外边距、
`pre`/`code`/`kbd`/`samp`/`tt` 只给 `font-family: monospace`（**不写死字号**：继承父级，
父级从未声明字号时才落到通用族默认 13px；`tt` 另补 `display: inline`）、
hr 边框、form/input/select/textarea/button 的尺寸与外观（多行控件的基线按
CSS 2.1 §10.8.1 取底外边距边——Chrome 的 textarea 计算 overflow 非 visible）；
**交互反馈（悬停/按下）**：按钮型控件（`<button>` 与 `<input type=submit/reset/button>`
同款）底色与边框一起加深（悬停 `#EFEFEF`→`#E3E3E3`、按下→`#D4D4D4`，边框
`#767676`→`#6A6A6A`/`#4F4F4F`），文本类控件（text/password/select/textarea）
只加深边框，勾选控件（radio/checkbox）不参与——**这是与 Chrome UA 样式表的
有意分歧**：Chrome 没有 hover 规则（浏览器里按钮悬停不变色），但作为桌面 UI
框架，按钮悬停毫无反馈会显得「死了」。作者 CSS 仍可覆盖（作者来源优先于 UA
来源，与 CSS 级联一致）；
表格完整模型（display 家族 + colspan/rowspan + border-spacing + 题注），
列表标记为引擎自绘（支持 list-style-type / list-style-position），几何仍是近似模型，见 [limitations.md](limitations.md)；
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
逐字回退；Noto CJK 在系统上通常是 VF 单文件且 fvar 默认字重是 Thin(100)，
SDL2_ttf 读不了 VF 命名实例（Chrome 经 DirectWrite 取 400/700 实例），引擎
因此优先用 `tools/gen-cjk-instances.ps1` 生成的静态 Regular/Bold 面
（落 `libs/fonts/`，gitignore）——未生成时退回 VF 本体（正文偏细）；
行盒（line-height: normal）直接用 SDL_ttf 的 `TTF_FontAscent`/
`TTF_FontDescent`：行盒高 = 各内联盒「基线以上高度」的最大值 +
「基线以下高度」的最大值，无内容时不加 strut。该口径是 FreeType 的像素
取整，不再逐像素对齐 Chrome/DirectWrite（同一字体行高可能差 1~4px），
换取不解析字体表的实现简化；声明了 `line-height` 时改按声明值分 Leading
（一半在文字之上、一半在之下，half-leading 取 floor(leading/2+0.5)，
与 Chrome 的基线落点一致），行盒高度严格等于声明值；
粗体按字体族选**真粗体字文件**（雅黑 → msyhbd.ttc、Arial → arialbd.ttf、
Segoe UI → segoeuib.ttf、Noto Sans/Serif SC → 生成的 NotoSansSC-Bold.ttf），
不依赖 FreeType 对常规字形的合成加粗——合成加粗
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

**外部样式表**：`<link rel="stylesheet" href>` 与 CSS 的 `@import`（可嵌套）都支持。
相对引用由引擎解析成绝对地址——`<link>` 相对文档地址，`@import` 相对**上一层样式表**
的地址（主题文件放在子目录里，`../` 也不会被解析到页面目录去）。引擎不碰 I/O，
加载器由宿主提供：

```go
doc, err := engine.OpenDocumentWith(vp, src, engine.DocumentOptions{
	BaseURL: "file:///C:/pages/index.html", // 本地文件先转成 file:// 地址
	LoadStylesheet: func(href string) (string, error) {
		// href 已是可直接使用的绝对地址（相对引用早在引擎里拼好了）
		return os.ReadFile(pathFromFileURL(href))
	},
})
```

`OpenDocument` 等价于不传加载器（`<link>`/`@import` 被忽略，与旧行为一致）。加载是
**同步**的、发生在解析阶段，所以选项必须在打开文档时给——事后再装加载器不会补加载
页面里已有的引用。加载失败只跳过那一份样式表（页面照常打开、其余样式生效），错误由
加载器自己上报；`@import` 的循环引用在链上截断（深度上限 32）。`media` 属性不参与
筛选（引擎不分媒体，与 `@media` 被跳过同口径）。`examples/browser` 把它接到了本地
文件系统（`assets.go`，只认 `file://`），演示页是 `external.html`。

**图片 `<img>`**：`<img src>` 走替换元素链路——布局按图片的**固有尺寸**定盒（CSS
`width`/`height` 优先，其次是 HTML `width`/`height` 属性，再按已知一边 + 固有纵横比
推另一边），解码后的位图画进内容盒（缩放由后端完成，`opacity` 与父级 `overflow`
裁剪照常生效）。和样式表一样，字节来源由宿主提供，引擎不读文件、不联网：

```go
doc, err := engine.OpenDocumentWith(vp, src, engine.DocumentOptions{
	BaseURL: "file:///C:/pages/index.html",
	LoadImage: func(src string) ([]byte, error) {
		// src 已是绝对地址；PNG/JPEG/GIF 的识别与解码由引擎负责
		return os.ReadFile(pathFromFileURL(src))
	},
})
```

与样式表不同，图片加载是**异步**的：布局时登记引用，worker goroutine 调加载器并解码，
完成后把位图写回文档缓存、给就绪计数 +1；主循环的 `PendingChange`/`LayoutDocument`
收口（`ChangeCount` 增长一次）后自动重排，图片就出现在首帧之后的那一帧。因此：加载器**可能在多个
goroutine 里被并发调用**（实现必须线程安全）；加载失败按「空图」处理（盒子
0×0、不落墨、不 panic，错误由加载器自己上报）；同一 `src` 只加载一次（引擎按解析后的
地址缓存，多个 `<img>` 共享同一份位图）。`data:` URL（base64 或百分号转义）由引擎
**同步**解码，不经过加载器——自包含页面与测试用它就能首帧出图。`examples/browser`
的 `loadLocalImage` 是只读本地文件的范例。
