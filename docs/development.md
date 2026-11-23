# 调试与开发

> 本文件是 [README](../README.md) 的分册；接口信息用 `go doc github.com/go-comm/ghromex/<包>` 查。

## 调试环境变量

| 变量 | 作用 |
|---|---|
| `GHROMEX_DEBUG=1` | 回显绘制失败原因与每次点击的命中/派发结果。开关按布尔解析：`1/true/yes/on` 为开 |
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
