# 调试与开发

> 本文件是 [README](../README.md) 的分册；接口信息用 `go doc github.com/go-comm/ghromex/<包>` 查。

## 调试环境变量

| 变量 | 作用 |
|---|---|
| `GHROMEX_DEBUG=1` | 后端诊断输出：绘制调用失败原因、渲染路径（driver 名）、窗口图标设置失败。开关按布尔解析：`1/true/yes/on` 为开。**不回显点击命中/派发**——要看「点到了谁」用下节的调试服务 |
| `GHROMEX_DEBUG_PORT=端口` | 起本地调试服务（HTTP/JSON 驱动与观测窗口），见下节。值可以是端口号（`9223`）、`host:port`（`127.0.0.1:9223`）或 `0`（随机端口）；未设置/空值 = 不起服务，非回环地址直接拒绝启动 |
| `GHROMEX_LCD=1` | 开启 LCD 子像素文本渲染（默认关闭：文本默认灰度 AA；开启后竖笔更锐，彩底边缘有轻微彩边，见 [limitations.md](limitations.md)） |
| `GHROMEX_HINT=档位` | 灰度文本 hinting：`normal`（默认，与 Chrome 灰度轮廓最同构）/`light`（更柔）/`mono`/`none`；LCD 打开时强制 LIGHT_SUBPIXEL，本变量仅影响灰度路径；运行中切换亦生效（face 与纹理按档位重建）；需 SDL2_ttf 导出 `TTF_SetFontHinting`，缺失时固定为 FreeType 默认 hinting |
| `GHROMEX_CONTRAST=k` | 灰度字形 tone 曲线强度（>1 生效、上限 3，默认关）：以 0.5 为枢轴把覆盖度推两端，边缘更实、毛边更净；k 越大墨色越重、竖边台阶感越明显，可与 `GHROMEX_HINT` 联调（如 `light+1.3`、`none+1.5`），仅影响 blended（灰度）路径 |
| `GHROMEX_BASELINE_ADJ=px` | 全局文本基线补偿（正数=下移），与 Chrome/DirectWrite 对照时的微调：只挪基线、不排行盒高度，故不影响行距与盒尺寸；默认 0。引擎行盒高度取自字体 hhea/OS2 度量（与 Chrome 同源），残差只来自两侧的取整（同一字号约 0.5~1px）。运行时改需重排一次（环境变量在首帧即生效） |
| `GHROMEX_GPU=1` | 开启 GPU 加速渲染器（accelerated + vsync）。**默认是内存渲染（SDL 软件渲染器，像素全在系统内存）**，不设此变量就不创建加速路径。开关按布尔解析：`1/true/yes/on` 为开，`0/false/no/off` 与未设置都表示“不开” |
| `GHROMEX_SOFTWARE=1` | 强制软件渲染器（优先级最高，`GHROMEX_GPU=1` 也压不过；默认即内存渲染，本开关的用途是钉死路径、排除加速干扰，如 GPU 可创建但不出像素的虚拟显示环境）。开关按布尔解析：`1/true/yes/on` 为开，`0/false/no/off` 与未设置都表示“不强制” |
| `GHROMEX_FONT=路径` | 指定 standard 字体（默认 Arial，即 sans-serif） |
| `GHROMEX_CJK_FONT=路径` | 指定 CJK fallback 字体（默认微软雅黑） |
| `SDL_VIDEO_DRIVER=dummy` | 无显示器运行（配合 `Window.Screenshot()` 做回读断言） |
| `GHROMEX_BENCH=1` | 分段基准：布局/清屏/重绘/呈现/空闲分开计时，≥300 帧后报 p50/p95/p99/max，并打印渲染路径（driver 名、是否 dummy）。只做本地诊断，**不进 CI**（计时不可复现，断言它只会得到 flaky 测试）。`GHROMEX_BENCH_FRAMES` 调每批样本数 |
| `GHROMEX_SCROLLPROBE=1` | 滚动专项探针（隐含 `GHROMEX_BENCH` 的分段采集）：帧首注入真实滚轮走 `onWheel → 滚动动画 → TickScroll` 那条链，另计 `TickScroll` 耗时、绘制调用构成（文字段/色块）、布局帧数与尖峰帧号、缩放与鼠标移动事件帧号、有位移帧比例。带「到界自动掉头」（连续无位移即反向），避免长页面滚到底后样本变成静止页。只做本地诊断，不进 CI |
| `GHROMEX_SCROLLPROBE_NOTCH=n` | 每次注入滚 n 格（一格 = 100px，默认 1） |
| `GHROMEX_SCROLLPROBE_EVERY=n` | 每 n 帧注入一次（默认 1＝持续滚）。设 1 会让滚动动画每帧被重定向（`startScrollAnim` 清零动画时间、该帧无位移），量到的是注入频率失真；接近真实设备的节奏是 ≥6 |
| `GHROMEX_SCROLLPROBE_STALL=n` | 连续 n 次「注入了但内容没位移」判为碰到滚动边界并掉头（默认 4） |

## 本地调试服务（debug 包）

`debug` 包把「驱动一个正在跑的窗口 + 读它的内部状态」做成 HTTP/JSON 接口，
给外部脚本（PowerShell / Python / CI）用。它是**开发期外挂**：默认不起、
只绑回环、不开 CORS，发布路径里不启用。

起服务（两种等价写法）：

```go
// 1. 显式起
srv, err := debug.Serve(win, debug.Options{Addr: "127.0.0.1:9223"})
if err == nil {
    defer srv.Close()
    fmt.Println(srv.URL("/debug/"))
}

// 2. 读环境变量 GHROMEX_DEBUG_PORT（未设置则返回 (nil, nil)，即不起）
srv, err := debug.ServeFromEnv(win)
```

`examples/browser` 已经接好：`go run ./examples/browser -debug-port 9223`
（`-debug-port` 优先于环境变量）。

### 端点

| 方法与路径 | 作用 |
|---|---|
| `GET /debug/state` | 窗口与文档状态：视口尺寸、标题、变更/重绘计数、滚动偏移、悬停/焦点/按下元素、选区 |
| `GET /debug/hit?x=&y=` | 命中测试：点上的元素链（自下而上）+ 伪类状态 + 光标形状 |
| `GET /debug/dom?selector=&depth=&limit=` | DOM 树快照（盒 + 文本 + 属性白名单 + 元素滚动偏移） |
| `GET /debug/text?selector=` | 元素的可见文本（InnerText）与表单取值 |
| `GET /debug/attr?selector=&name=` | 元素的单个属性值（任意属性名） |
| `GET /debug/shot.png` | 当前画面 PNG |
| `GET /debug/layout.svg` | 当前布局 SVG（与 `-dump-svg` 同源） |
| `POST /debug/doc` | 换文档：`{"html":"..."}` 或 `{"path":"..."}` |
| `POST /debug/input` | 执行一串输入动作（见下） |
| `GET /debug/help` | 端点与动作清单（JSON，脚本可自描述） |

`GET /debug/dom` 默认 `depth=6`、`limit=2000`：整页几十万节点一次全量序列化
能把连接撑爆，读的时候用 `selector` 收窄。

### 输入动作

请求体是动作数组（也接受 `{"actions":[...]}`）：

```json
[{"op":"click","x":320,"y":240},
 {"op":"wait","ms":120},
 {"op":"wheel","x":320,"y":240,"dy":-1},
 {"op":"key","key":"ctrl+s"},
 {"op":"text","s":"你好"},
 {"op":"drag","x":10,"y":20,"x2":200,"y2":20,"steps":8},
 {"op":"drop","files":["C:\\a.png"]}]
```

- 坐标是**视口像素**；滚轮档位用 SDL 口径（`dy` 正 = 滚轮向上）。
- 动作按顺序执行，**每个动作后自动插一道同步栅栏**（一个空的主循环任务），
  所以下一个动作看到的是上一个动作已经落地后的世界——不必自己插 `wait`
  去等事件派发。`wait` 是真的睡眠，用来让滚动动画跨帧推进。
- 输入走 `browser.Window.InjectInput`（内部翻译成 SDL 事件 → `SDL_PushEvent`），
  与真人操作同一条解码链路，所以坐标修正、焦点链、悬停档位这些真实 bug
  都能被复现。键码/修饰键/按键号用 `browser` 包导出的中性常量
  （`KeyEnter`/`ModCtrl`/`MouseButtonLeft` 等），脚本不碰平台事件模型。
- 动作 `state`/`hit`/`text`/`dom` 是观测：结果进响应的 `results[]`。
- `close` 之后不再插栅栏（窗口正在退出，等来的只会是「主循环已退出」）。

### PowerShell 例子

```powershell
Invoke-RestMethod http://127.0.0.1:9223/debug/state | ConvertTo-Json -Depth 6

Invoke-RestMethod -Method Post -Uri http://127.0.0.1:9223/debug/input `
  -ContentType "application/json" `
  -Body '[{"op":"click","x":320,"y":240},{"op":"key","key":"ctrl+a"}]'

Invoke-WebRequest http://127.0.0.1:9223/debug/shot.png -OutFile shot.png
```

### 线程纪律

SDL 的窗口与渲染器绑在 `NewWindow→Run→Close` 所在的那个 OS 线程，HTTP handler
跑在别的 goroutine 上。因此包内**读文档/截图/改 DOM 一律经 `Window.Do` 交回
主循环执行**，注入输入才走 `SDL_PushEvent`。主循环没跑时请求会明确报
「主循环未在运行」——不假装成功，这是排查「脚本跑了但窗口没反应」的关键信息。

### 不在发布路径里

`debug` 与 `ui` 同层，但 `engine`/`browser`/`ui` 都不许 import 它
（`core/arch_test.go` 的 `TestDebugIsNotADependency` 钉死）。它自己也只依赖
`engine` + `browser`，**不 import `backend/sdl2`**：注入走
`browser.Window.InjectInput`（中性 `browser.InputEvent`，`core/arch_test.go`
的分层表钉死了 `debug` 只能依赖 `engine`/`browser`）。别把端口暴露到
局域网，也别在发行版里默认开启。

## 开发与测试

```powershell
go vet ./...
go test ./...
```

- `engine` 包测试全为纯 Go（FakeGraphics / BufferGraphics / SVG 断言），无显示器即可跑
- `backend/sdl2` 的事件解码回归测试不加载 DLL；事件字段偏移以真实 SDL 产生的事件实测为准（见 `decodeEvent` 注释）
- 示例退出约定：`go run ./examples/ua -exit-after 3s` 可用于冒烟验证
- 灰度文字保真度探针：`go test ./backend/sdl2 -run TestGrayscalePipelineFidelity`（默认 dummy 驱动软件渲染器，
  `GHROMEX_REAL=1` 走真机 GPU）——同源直出面与落屏截图逐像素比对，供回归「灰度管线忠实」结论
- 文字锐度/字体对比条：`GHROMEX_CMP=1 go test ./backend/sdl2 -run 'TestFontCompareStrip|TestHintingValueScan'`
  输出 `.temp/fontcmp.png`（NORMAL/LIGHT/NONE/SS2x/LCD 五行并排，肉眼与量化均可比对）
