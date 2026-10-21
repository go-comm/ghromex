# runtime —— 平台能力

本目录放**与操作系统平台相关的能力**：不属于渲染引擎、也不属于某个渲染
后端的东西——原生文件对话框、原生消息框，以及以后要加的其它系统能力
（通知、托盘、电源状态、系统目录……）。

## 为什么单独一棵树

仓库的分层：

| 目录 | 放什么 | 依赖 |
| --- | --- | --- |
| `core/` | 引擎与后端之间的**契约**（`Graphics`/`Viewport`/`Paint`/`Color`…） | 叶子包，不依赖仓库内任何包 |
| `engine/` | 渲染引擎（HTML/CSS 解析、布局、绘制指令） | 纯 Go，只依赖 `core` |
| `backend/` | 平台实现（`sdl2`：窗口、事件解码、文本纹理；`dialog`：原生对话框） | 平台实现（Windows），只依赖 `core` |
| `browser/` | 应用层：窗口 + 文档 + 主循环（把平台事件翻译成引擎动作） | `engine` + `backend` |
| `runtime/` | 平台能力的**通用调用入口**（`dialog`：原生对话框） | 平台无关的类型，转调 `backend/` |

依赖方向是单向的：`core` 在最底下，`engine` 与 `backend` 各自向上依赖它，
`browser` 再把两者缝起来。平台层（`backend`）**不认识引擎**——它只解码事件、
只按 `core` 的契约绘制；「事件 → 引擎动作 → 重排 → 重绘」这条主循环在
`browser`。反过来的依赖（后端 import 引擎）会让平台层反过来依赖它上面的层，
引擎一改就牵动后端，后端也没法被别的引擎复用；`core/arch_test.go` 把这条
规则钉死。

`backend/` 是**实现层**，`runtime/` 是**调用入口**：调用方只 import
`runtime/dialog`（平台无关的类型与函数），由它转调 `backend/dialog` 的
平台实现。两层都不依赖渲染后端——父窗口句柄由调用方传入
（`browser.Window.NativeHandle` 给出真实 HWND）。

`engine` 保持纯 Go、可移植：不认识 SDL，也不认识 Win32——平台差异一律收
在 `backend/` 与 `runtime/` 里。

## 现在的内容

| 包 | 做什么 | 平台 |
| --- | --- | --- |
| `runtime/dialog` | 原生对话框的通用入口：打开文件 / 打开多个文件 / 选择文件夹 / 另存为 / 消息框 | 平台无关的类型；Windows 走下面的实现，其它平台返回 `ErrUnsupported` |
| `runtime/clipboard` | 系统剪贴板的通用入口：Text / SetText | 平台无关的接口；Windows 走下面的实现，其它平台是空操作 |
| `backend/dialog` | 原生对话框的平台实现 | Windows：Shell `IFileDialog`（纯 Win32 COM，无 CGO）与 user32 `MessageBoxW` |
| `backend/clipboard` | 剪贴板的平台实现 | 复用 SDL2 的剪贴板接口（与引擎内部 Ctrl+C/Ctrl+V 同一条路径） |

## 约定

- **纯 Go、无 CGO**：平台 API 一律用 `golang.org/x/sys/windows` 动态加载
  系统 DLL 后按 vtable/调用约定调用，与 `backend/sdl2` 加载 SDL2.dll 的
  做法一致。
- **跨平台构建要过**：`go build ./...` / `go test ./...` 在任意平台都要能
  跑，所以平台实现用 `//go:build windows` 隔开，非 Windows 提供同签名的
  空实现（返回 `ErrUnsupported`）。
- **不 import backend/sdl2**：能力包不反向依赖渲染后端（父窗口句柄等由调用方
  传入），否则两者会缠在一起。（`runtime/dialog` import `backend/dialog` 是
  「调用入口 → 实现」，与渲染后端无关。）
- **模态的要在回调里调**：文件对话框与消息框都是模态的，调用期间阻塞当前
  线程（父窗口的事件循环随之暂停），这与原生应用的行为一致。
