# 项目结构

> 本文件是 [README](../README.md) 的分册；接口信息用 `go doc github.com/go-comm/ghromex/<包>` 查。

```
core/             引擎与后端之间的契约（Graphics/Viewport/Paint/Color/Sprite…），
                  叶子包：engine 与 backend 都只依赖它，依赖方向保持单向
engine/            渲染引擎（纯 Go，不依赖任何图形后端）
  html_parser.go     HTML 解析 → DOM 树
  css_parser.go      CSS / 内联样式解析（含 @import 引用提取）
  css_url.go         外部样式表引用解析：相对引用 → 绝对地址（纯字符串）
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
backend/sdl2/      SDL2 后端：窗口、事件解码、SDL_ttf 文本纹理缓存（LRU + 像素双上限，
                   页面文本段数超限时不再逐帧重渲染）、
                   圆角精灵纹理缓存（同样 LRU + 像素双上限；精灵像素不变，
                   纹理只上传一次）、
                   剪贴板读写（Ctrl+C 写 / Ctrl+V 读，引擎只管文本）、
                   原生窗口句柄（NativeHandle，供原生对话框当父窗口）。
                   平台层：只依赖 core，不认识引擎（主循环在 browser）
browser/           应用层：窗口 + 文档 + 主循环（平台事件 → 引擎动作 → 重排 → 重绘）
runtime/dialog/     原生对话框的通用调用入口（平台无关的类型 + 五个对话框：
                   打开文件 / 打开多个文件 / 选择文件夹 / 另存为 / 消息框）
runtime/clipboard/   系统剪贴板的通用调用入口（Text / SetText）
backend/dialog/     原生对话框的平台实现（Windows）：文件对话框走 Shell
                   IFileDialog（纯 Win32 COM，无 CGO），消息框走 user32
                   MessageBoxW（不需要 COM）。父窗口句柄由调用方传入
backend/clipboard/   剪贴板的平台实现（复用 SDL2 的剪贴板接口——引擎内部的
                   Ctrl+C / Ctrl+V 走的是同一条路径）
ui/                应用工作台组件库（在 browser 之上）：菜单栏/工具栏/状态栏、
                   下拉浮层、快捷键分发、主题令牌与页面组件样式；
                   `ui.App` 把「建窗 + 装工作台 + 换页重装」封装成十来行
examples/          可独立运行的示例（页面源码可能是内嵌 HTML、HTML 资产或由 Go 构造）：
  ua/                原生 UA 表单 + 常用标签（零作者 CSS，与 Chrome 对照）
  stylebook/         样式全集（作者样式表：index.html + stylebook.css 外链加载，
                     全标签/CSS 演示 + 支持级别徽标，与 Chrome 对照）
  kit/               组件库演示（菜单/工具栏/状态栏 + 卡片式页面，零作者 CSS）
  iconsheet/         内置图标表（全部名称/字形/码位，单击复制名称）
  browser/           迷你本地浏览器（常驻工具条 + 状态栏、历史/刷新、链接导航、错误提示页、
                     外部样式表加载：examples/browser/assets.go）
  portal/            购物门户（fixed 顶栏、层叠上下文、商品网格、秒杀、购物车交互）
  notepad/           仿 Windows 记事本（page.go 构造 DOM、菜单栏+浮层菜单/工具栏/查找替换栏/
                     编辑区/状态栏、文件读写、撤销重做栈、剪贴板）
libs/              SDL2 运行库（SDL2.dll / SDL2_ttf.dll / zlib1.dll，见 libs/README.md）
```

**包内文档**：每个包的入口说明写在各自的 `doc.go`（`go doc ./engine` 可读，
也进 pkg.go.dev）：engine 讲分层地图与「改代码前必读的不变量」、backend/sdl2
讲平台边界与线程约束、browser 讲三方分工与用法骨架、ui 讲命令模型（Action/
Spec）与工作台的三条约定（换文档重装、快捷键先行、只用手册内的 CSS 子集）；
`runtime/` 的分层约定（后端 vs 平台能力）见 `runtime/README.md`；`libs/`
里第三方 DLL 的版本、出处、许可与查找顺序见 `libs/README.md`。
