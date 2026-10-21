# runtime —— 平台能力

本目录放**与操作系统平台相关的能力**：不属于渲染引擎、也不属于某个渲染
后端的东西——原生文件对话框、原生消息框，以及以后要加的其它系统能力
（通知、托盘、电源状态、系统目录……）。

## 为什么单独一棵树

仓库的三层分工：

| 目录 | 放什么 | 依赖 |
| --- | --- | --- |
| `engine/` | 渲染引擎（HTML/CSS 解析、布局、绘制指令） | 纯 Go，无平台依赖 |
| `backend/` | 渲染后端（`sdl2`：窗口、事件循环、文本纹理） | 平台实现（Windows） |
| `runtime/` | 平台能力（`dialog`：原生对话框、消息框） | 平台实现（Windows） |

`backend/` 与 `runtime/` 的区别是**后端 vs 能力**：后端负责「把页面画出来」
（渲染器、字体、事件循环），能力负责「操作系统能替应用做的事」。两者都
是平台相关的，但互不依赖——`runtime/dialog` 不 import `backend/sdl2`，
父窗口句柄由调用方传入（`backend/sdl2` 的 `Window.NativeHandle` 给出真实
HWND）。

`engine` 保持纯 Go、可移植：不认识 SDL，也不认识 Win32——平台差异一律收
在 `backend/` 与 `runtime/` 里。

## 现在的内容

| 包 | 做什么 | 平台 |
| --- | --- | --- |
| `runtime/dialog` | 原生文件对话框（打开文件 / 打开多个文件 / 选择文件夹 / 另存为）与消息框 | Windows：Shell `IFileDialog`（纯 Win32 COM，无 CGO）与 user32 `MessageBoxW`；其它平台返回 `ErrUnsupported` |

## 约定

- **纯 Go、无 CGO**：平台 API 一律用 `golang.org/x/sys/windows` 动态加载
  系统 DLL 后按 vtable/调用约定调用，与 `backend/sdl2` 加载 SDL2.dll 的
  做法一致。
- **跨平台构建要过**：`go build ./...` / `go test ./...` 在任意平台都要能
  跑，所以平台实现用 `//go:build windows` 隔开，非 Windows 提供同签名的
  空实现（返回 `ErrUnsupported`）。
- **不 import backend**：能力包不反向依赖渲染后端（父窗口句柄等由调用方
  传入），否则两者会缠在一起。
- **模态的要在回调里调**：文件对话框与消息框都是模态的，调用期间阻塞当前
  线程（父窗口的事件循环随之暂停），这与原生应用的行为一致。
