# 更新日志

版本号即 git tag（Go module 语义：`v0.2.0` 可从
`github.com/go-comm/ghromex@v0.2.0` 直接取用）。

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
