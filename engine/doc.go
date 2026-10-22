// Package engine 是 Ghromex 的渲染引擎：纯 Go、不依赖任何图形后端，
// 把 HTML/CSS 变成一串绘制指令交给后端执行。
//
// # 分层
//
//	解析   html_parser.go / css_parser.go / css_url.go / selector.go
//	级联   style_cascade.go（选择器匹配 + 特异度 + 继承）+ style_ua.go（UA 默认样式）
//	       pseudo.go（伪类状态：hover/focus/active 与结构伪类）
//	布局   measure.go（盒模型/行盒/换行/定位）+ line_box.go（line-height 的
//	       Leading 分配与 vertical-align 的基线位移）+ margin_collapse.go +
//	       table_layout.go
//	滚动   scroll.go（overflow 容器 + 文档级滚动 + wheel 分发）
//	绘制   render.go（背景/边框/文本/装饰/层序/裁剪栈）+ form_render.go（表单控件）
//	交互   event_dispatch.go（命中测试 + 冒泡派发）+ text_input.go / selection.go /
//	       select.go / textarea.go / anchor.go / check.go
//	输出   graphics.go（Graphics 接口 + Fake/Buffer 实现）+ svg_dump.go（SVG 导出）
//
// # 改代码前必读的不变量
//
//  1. 纯 Go 可移植：engine 不得 import 平台包或图形库。窗口、剪贴板、
//     字体文件路径等平台能力由 backend/sdl2 提供（browser 是门面）。
//  2. 1 CSS px = 1 物理像素：布局全程整数像素，不做小数舍入（与浏览器的
//     亚像素差异是已知取舍，见根 README「已知限制」）。
//  3. 布局流水线顺序固定（见 windows.go 的 LayoutDocument）：
//     resolveStylesTree → layoutBox → layoutPositioned → applyScrollOffsets
//     → layoutSelectPopups。新增定位/滚动/浮层特性必须挂到正确的遍次，
//     漏挂会表现为「改了不生效」或「重排死循环」。
//  4. 命中测试与绘制必须同口径：裁剪（overflow）、定位层、select 浮层在
//     ElementAt 与 renderNode 两侧都要一致，否则会出现「看得见点不到」。
//  5. 变更通知是唯一的重排触发器：DOM 改动（属性/文本/焦点/选区/视口
//     尺寸）都要走 markChanged，后端按 ChangeCount 决定重排；**布局期不得
//     触发 markChanged**（会形成重排↔变更死循环，见 style_cascade.go 的
//     syncSelectValue）。
//  6. 行盒只有一处口径：行盒高 = 各内联项**贡献**的最大 ascent + 最大
//     descent，基线在距行顶 lineAsc 处。贡献 = 字体度量 ± line-height 的
//     Leading（line_box.go 的 applyLeading）± vertical-align 的位移
//     （line_box.go 的 verticalAlignShift）。绘制、命中测试、SVG 导出都
//     从同一份文本片段与盒几何取数——三者任一改口径都会出现「画出来的
//     和点到的不是同一个」。
//
// # 事件模型
//
// 命中测试取最深元素，沿父链依次派发 click / change / wheel，随后执行
// 引擎默认动作（select 展开选中、勾选切换、链接导航）。焦点与光标先于
// click 处理器更新——处理器里读到的已是点击后的状态。
//
// 按键：keydown 派发给聚焦元素（元素上用 OnKeyDown 注册），处理器
// 返回 true 表示已消费，引擎的默认动作（退格/删除/回车换行/方向键移动光标）
// 不再执行——应用层靠它接"回车 = 打开"这类行为，不用改引擎。
//
// 可编辑输入（input text/password、textarea）共享一套编辑状态：光标 +
// 选区锚点，单位是 value 的 rune 下标。失焦只停画、**不丢状态**（重新
// 聚焦会恢复光标与选区），页内菜单（复制/剪切/粘贴）据此作用于编辑区。
//
// # 伪类状态
//
// :hover / :focus / :active 是**动态状态**，存在文档上（pseudo.go）：
// 状态一变，样式就得重算，故走「变更通知 → 重排重绘 → 级联重算」这条
// 与 DOM 变更同一条链路（markChanged）。悬停/按下/焦点都只在**变化时**
// 通知——鼠标移动是最频繁的事件。
//
// 应用层若自建主循环（非 browser 门面），需自己把鼠标事件喂进来：
// OnDocumentMouseMove（更新悬停）、OnDocumentMouseDown/Up（按下态）、
// SetFocusedElement（焦点）。browser 门面已自动接好。
//
// # 外部样式表
//
// <link rel="stylesheet" href> 与 CSS 的 @import（可嵌套）都在**解析阶段**
// 同步加载，规则并入作者样式表并按文档顺序参与级联；@import 的规则排在
// 导入方之前（同特异度时导入方胜出，与浏览器一致）。
//
// 引擎不碰 I/O：加载器由宿主经 DocumentOptions.LoadStylesheet 提供。相对
// 引用由引擎解析成绝对地址——<link> 相对 BaseURL，@import 相对**上一层
// 样式表**的地址（css_url.go），所以加载器拿到的地址可以直接用。加载失败
// 只跳过那一份样式表（页面照常打开）；@import 的循环引用在链上截断
// （深度上限 maxStylesheetDepth）。
//
// 加载发生在解析期，所以加载器只能在 OpenDocumentWith 时传入；文档建好
// 之后再 SetStylesheetLoader 不会补加载页面里已有的引用（只影响此后新增
// 的引用）。加载器跑在打开文档的 goroutine 上——要联网的宿主应自己缓存或
// 预取，引擎没有「样式到货后再重排」的链路。
//
// # 后端可选扩展
//
// Graphics 接口之外有两个**可选扩展**（后端实现了就用，没实现就走等价
// 但更慢的回退路径，画面结果一致）：
//
//   - Clipper：PushClip/PopClip 的裁剪栈（裁剪矩形逐层求交下推）；
//   - SpriteDrawer：DrawSprite 位图精灵（圆角抗锯齿的"角标"——整块圆角
//     一次 blit，替掉逐像素的半透明小方块）。绘制调用数是 GPU 路径上的
//     主要开销：逐像素画圆角边缘时 portal 页一帧要发 3.2 万次调用，改用
//     角标后降到 2500 次，单帧绘制 149ms → 8.7ms（软件渲染器实测），
//     滚动动画才不会被切成几帧（一卡一卡还闪）。
//
// # 测试
//
// engine 的测试全是纯 Go，不需要显示器：
//
//   - FakeGraphics：只计数，断言绘制指令的调用序列；
//   - BufferGraphics：软件光栅化到 RGBA，可 SavePNG 与 ColorAt 逐像素断言；
//     NewBufferedViewport / OpenBufferedDocument 一把建好「视口 + 缓冲」，
//     免得视口与缓冲的尺寸写岔；
//   - HeadlessViewport：无头视口，配合 DumpSVG 断言布局几何；
//   - 事件用 engine.OnDocumentClick / OnDocumentKeyDown / OnDocumentPaste
//     等公开入口驱动，不要直接调内部函数——白盒绕过命中与焦点链路会漏掉
//     真实 bug。
package engine
