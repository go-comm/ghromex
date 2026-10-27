# libs —— SDL2 运行库

本目录放 Ghromex 在 Windows 上运行所需的三个第三方 DLL。`backend/sdl2`
通过 `golang.org/x/sys/windows` **动态加载**它们（不链接、不需要 CGO 与
C 工具链），所以这里既没有 .a/.lib，也没有头文件——只有运行时用的 DLL。

| 文件 | 版本 | 作用 |
| --- | --- | --- |
| `SDL2.dll` | 2.28.4 | 窗口、渲染器、事件循环、系统剪贴板 |
| `SDL2_ttf.dll` | 2.20.1 | TrueType 字体渲染（行盒高度取自 `TTF_FontAscent/TTF_FontDescent`） |
| `zlib1.dll` | 1.2.11 | `SDL2_ttf.dll` 的依赖（FreeType 链接的 zlib） |

## 来源与许可

- SDL2 / SDL2_ttf：<https://www.libsdl.org/> — **zlib 许可**，允许再分发
  （随包附带时保留原始版权声明即可）。
- zlib：<https://zlib.net/> — zlib 许可。

换成别的版本通常也能跑：绑定的是**符号名**而非版本号，只要导出
`SDL_Init` / `SDL_CreateWindow` / … 与 `TTF_Init` / `TTF_OpenFont` / …
即可。个别符号按「可选能力」处理，缺失时对应功能降级而不影响启动：

| 缺失的符号 | 后果 |
| --- | --- |
| `SDL_GetClipboardText` / `SDL_free` | 粘贴不可用（Ctrl+V 无内容可粘） |
| `TTF_SetFontHinting` | 固定用 FreeType 默认 hinting（`GHROMEX_HINT` 失效） |
| `TTF_SetFontSize` / `TTF_FontAscent` / `TTF_FontDescent` | 退回旧的「每字号开一个 face」路径，行度量口径略有差异 |
| `TTF_RenderUTF8_LCD` | LCD 子像素渲染不可用（`GHROMEX_LCD=1` 失效） |

## 查找顺序

`backend/sdl2` 的 `findLibsDir()` 依次探测（先外层目录、再内层候选）：

1. `./libs`（从仓库根 `go run ./examples/...` 时命中）
2. 可执行文件旁的 `libs/`
3. 可执行文件所在目录

上面三项在 `.`、`..`、`../..` 三个层级各找一遍。命中后用
`SetDllDirectory` 把该目录加进 DLL 搜索路径——`SDL2_ttf.dll` 依赖的
`zlib1.dll` 才能被找到。

## 缺失时

启动会报错：

- `解析 SDL2.dll 符号失败: ...` —— DLL 没找到或不是预期的 SDL2；
- `NewWindow: SDL_Init: ...` —— 找到了但初始化失败（缺依赖 DLL）。

把上表三个文件放进本目录即可；也可以放到系统 PATH 能搜到的地方。

## libs/fonts —— 补充字体（按需生成，不入库）

Google 的 `NotoSansSC-VF.ttf` / `NotoSerifSC-VF.ttf` 的 fvar 默认字重是
Thin(100)，而 SDL2_ttf 读不了 VF 命名实例（Chrome 经 DirectWrite 取
400/700 实例）——直接打开会渲染出细三档的正文。本目录放
`tools/gen-cjk-instances.ps1` 从系统 VF 实例化出的静态字重面：

| 文件 | 字重 | 作用 |
| --- | --- | --- |
| `NotoSansSC-Regular.ttf` | 400 | CJK sans 回退的正体面（替代 VF 的 Thin 默认面） |
| `NotoSansSC-Bold.ttf` | 700 | 粗体/半粗档真字面（Chrome 对 600 同样就近选 700） |
| `NotoSerifSC-Regular.ttf` / `NotoSerifSC-Bold.ttf` | 400/700 | serif 回退同理 |

生成（需 python + `pip install fonttools`）：

```
powershell -ExecutionPolicy Bypass -File tools\gen-cjk-instances.ps1
```

目录被 `.gitignore` 排除；`backend/sdl2` 的 `fontPathOf` 在系统字体目录
未命中时来此查找，**仅当系统装了同名 VF 才选用**（Chrome 没选 Noto 的
机器上引擎也不选，两边族选择一致）。缺文件时退回旧行为（VF Thin 面 +
合成加粗）。字体为 SIL OFL 许可，允许再分发（生成物随应用打包时附
许可文本即可）。
