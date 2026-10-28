# RFC 0001：滚动偏移改「绘制/命中期现算」（offset 传播）

- 状态：**封存（Deferred，不排期）**
- 决定日期：2026-10-28
- 结论先行：设计成立、边界想清楚了，但**投入产出比不够**——它消除的两项开销
  （平移 + 快路径布局）只占滚动帧成本的 43%，真正的大头（重绘遍历 198ms）
  分文未动。在重绘遍历被别的手段（display list 缓存 / 瓦片化 / 子树剪枝）
  解决之前，本方案单独落地改变不了「大页面滚动卡」的体感。重启前请先读
  「为什么不做的」一节。

## 1. 问题与现状（2026-10 实测，压力页）

滚动是**物理平移**：布局后 `applyScrollOffsets` 对整树 `shiftBox`（搬
`base.x/y` 与每个 textRun 的 x/y）；滚动动画每帧 `TickScroll →
applyScrollDeltas` 再整树平移一次；渲染与命中直接读搬过的坐标。

压力页（14.9MB / 约 42 万元素，1024×720）每档滚轮实测：

| 段 | 耗时 | 本方案能否消除 |
|---|---|---|
| TickScroll 整树平移 | 107ms | ✅ 变 O(滚动盒数) |
| 布局快路径（applyScrollOffsets） | 40ms | ✅ 变 O(滚动盒数) 夹紧检查 |
| **RenderNode 重绘遍历** | **198ms** | ❌ 分文未动 |

`position:fixed` 靠 `shiftBox` 递归时跳过实现；`position:sticky` 完全没有。

## 2. 设计（讨论通过的版本）

### 2.1 新模型

盒坐标**永远停在布局坐标系**（文档空间，不平移）。渲染与命中在树遍历时
携带累积偏移 `(offX, offY)`，绘制/比较时现算 `有效坐标 = 布局坐标 + off`。

### 2.2 偏移传播规则（四个边界）

| 边界 | 规则 |
|---|---|
| 嵌套 `overflow:scroll` 累加 | 进入滚动容器的**子树**时 off 扣掉容器的 scrollVisualOffset（负方向）；容器**自身盒子**用进入前的 off（它随父滚动走）。多层嵌套自然叠加 |
| `position:fixed` 截断 | 走到 fixed 元素时 `off := (0,0)`，子树继承；子树内再有滚动容器照常扣自己的。等价于今天 shiftBox 跳过 fixed 的语义，且 fixed 的裁剪盒/视口剔除第一次真正正确（今天依赖布局坐标恰好等于视口坐标） |
| `position:sticky` 动态计算 | 新增支持。布局期只缓存最近滚动容器指针与包含块边界；渲染/命中期调**同一个** helper 把自然位置夹在「容器可视边 + top/left 偏移」与「包含块对边 − 元素尺寸」之间。按每帧当前偏移现算，动画天然平滑 |
| `PushClip` 套合 + 视口外不发 DrawCall | 裁剪盒 rect 按「容器盒 + off」现算后 push；裁剪盒（overflow 非 visible）**整体落在视口外 → 整棵子树不走不画**（裁剪盒内的子内容逃不出去，剪枝安全）；overflow:visible 维持今天逐盒判断口径（子内容可能溢出可见） |

### 2.3 RenderNode / ElementAt 同口径

- `renderNodeTextColor` 加 `offX, offY` 参数，节点内所有绘制点（背景/边框/
  textRun/光标/列表标记/表单控件分支）统一用 `base.x+offX, base.y+offY`，
  不再有第二个坐标来源。
- `ElementAt` 的 walk 同样带 off；`clips` 矩形与 `PushClip` 用同一计算函数；
  命中比较用同一套有效坐标。
- 两个入口共享三个内部 helper：`containerOffset`（滚动容器扣多少）、
  `stickyAdjust`（吸顶修正）、`clipRectOf`（裁剪盒现算）。渲染画的与命中
  判的必然同一点——「看得见就点得到」从两边各自维护平移量变成结构性保证。

### 2.4 改造清单

| 消费方 | 改法 |
|---|---|
| render 主 pass / paintPositioned 定位 pass | 定位 pass 的 `pushAncestorClips` 已走父链收裁剪盒，同一条链顺带累加 off（遇 fixed 截断），起点 off 不再是 0 |
| ElementAt / CursorAt / 拖选 / tooltip 命中 | 全走新 walk（同一入口，自动一致） |
| `GetBoundingClientRect` | 语义保持「视口空间」：自身布局坐标 + 父链累加 off（O(深度)） |
| select 浮层 / 视口滚动条 / tooltip | 按 fixed 同款处理（视口空间锚定，off=0），浮层位置锚定 select 的现算位置 |
| 滚动夹紧（内容变小回弹） | 全量布局时顺手收集 `scrollables` 清单，夹紧走清单 O(滚动盒数) |
| TickScroll / LayoutDocument 快路径 | TickScroll 只推动画状态 + O(滚动盒) 夹紧，零整树遍历 |
| 删除项 | `shiftBox`、`applyScrollOffsets`、`applyScrollDeltas`、每元素 `scrollShiftTop/Left`（−8B/节点；`field_packing` 测试若钉了尺寸需同步） |

### 2.5 验证方案（当时规划）

- 现有 position/hit/scroll/form/SVG 测试网全绿为底线；
- 新增回归（判据为绘制调用数/坐标断言，不依赖计时）：嵌套两层滚动偏移累加、
  fixed 滚动前后不动、sticky 钉住/释放临界、裁剪盒滚出视口 DrawCall 计数为 0、
  `GetBoundingClientRect` 滚动前后与今天一致；
- 动手前在当时 HEAD 上用 BufferGraphics 对多页 × 多滚动位置存基线 PNG，
  重构后像素 diff（本地流程不进仓库）；
- `GHROMEX_SCROLLPROBE` 同口径复测。

## 3. 为什么不做的（封存理由）

1. **投入产出比不够（治标不治本）**：滚动帧 345ms = 平移 107 + 快路径 40 +
   重绘 198。本方案满打满算只消除前 147ms，剩下的 198ms 依然让大页面卡顿
   ——它才是终极 BOSS，而本方案对它分文未动。收益封顶 43% 的重构，不值。
2. **核心概念的兼容性风险高**：`GetBoundingClientRect` 被 ui 层、事件、拖拽、
   宿主应用大量调用，坐标语义改动极易引发焦点错位、点击漂移、弹窗错位的
   连锁回归；`sticky` 还要为它从零补一套动态 clamp，战线拉得太长。
3. **阶段成果已守住体验底线**：滚动快路径（40ms 纯平移）+ hover/active/focus
   变更分级（1.72s → 零重排）落地后，「鼠标动一下卡 1.7 秒」「开页后第一下
   滚轮卡住」这类致命缺陷已消除。在没有「重绘遍历」的解药之前，先不动底层
   坐标系，避免为优化而优化引入不可控回归。

## 4. 重启条件与替代路径

**重启条件**（满足任一再回来读本 RFC）：

- 重绘遍历有了新的解法（display list 缓存 / 瓦片化 / GPU 提交路径），198ms
  这个大头已解决，平移 147ms 上升为主要矛盾；
- 引擎要正式支持 `position:sticky`（那时本方案 2.2 的 sticky helper 顺带复活）。

**更低风险的替代路径**（真要再剃那 198ms，按顺序尝试）：

1. **裁剪子树剪枝（最小改动，不改坐标系）**：在 `RenderNode` 递归里，遇
   `overflow: hidden/scroll` 容器且其盒完全在视口外 → 直接 return 不递归。
   现有 `offscreenBox` 已逐盒判断，只差「剪枝到子树」这一步，几十行、零
   语义变化。注意 overflow:visible 不能剪（子内容可溢出可见）。
2. **脏区域重绘（Dirty Rect）**：滚动只重绘新露出的条带。
3. **display list 缓存 / 瓦片化**：独立 PR，另开 RFC。
