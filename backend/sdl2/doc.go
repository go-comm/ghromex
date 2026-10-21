// Package sdl2 是 Ghromex 的 SDL2 渲染后端：窗口、事件循环、文本纹理
// 缓存、系统剪贴板，以及 Windows 原生文件对话框。纯动态加载 SDL2.dll /
// SDL2_ttf.dll，无需 CGO 与 C 工具链；非 Windows 平台由 stub_other.go
// 提供同签名的空实现（NewWindow 与文件对话框返回 ErrUnsupported）。
//
// # 平台与线程约束
//
//   - 仅 Windows 可用：实现文件都是 //go:build windows。
//   - 同一 OS 线程：SDL 要求窗口的创建、事件循环、销毁在同一线程，
//     因此 NewWindow → Run → Close 必须在同一个 goroutine（NewWindow 会
//     LockOSThread，Close 时解锁）。另开 goroutine 操作窗口会出错。
//   - 依赖 DLL 放在工程根的 libs/（见 libs/README.md）：findLibsDir 依次
//     探测 ./libs、可执行文件旁的 libs/、可执行文件目录，并在上溯目录
//     （./、../、../../）里再找一遍；命中后用 SetDllDirectory 把该目录
//     加进 DLL 搜索路径，SDL2_ttf 依赖的 zlib1.dll 才能被找到。
//
// # 原生文件对话框
//
// OpenFileDialog / OpenDirectoryDialog / SaveFileDialog 走 Windows Shell
// 的 IFileDialog（Vista 起的现代对话框），不经 SDL：纯 Win32 COM 调用，
// 同样无 CGO。实现见 dialog_windows.go——要点：
//
//   - IFileDialog 必须在 STA 上创建与 Show：入口处 CoInitializeEx
//     (COINIT_APARTMENTTHREADED)，并 LockOSThread 固定线程。
//   - 父窗口用 SDL_GetWindowWMInfo 取真 HWND（SDL_CreateWindow 返回的是
//     SDL_Window*，不是 HWND；拿错会让 Show 直接回 E_INVALIDARG）。
//   - 虚表下标以 Windows SDK 的 ShObjIdl_core.idl 为准；GetResults 的
//     下标在不同 Windows 版本上不同，实现里运行时探测（getResultsSlot）。
//
// # 动态加载约定
//
//   - 符号用 bindSdl / bindTtf 绑定（必需，缺失即启动失败），或
//     bindProcOpt / bindProbe 绑定（可选，缺失即按能力降级，如缺
//     SDL_GetClipboardText 则粘贴不可用）。
//   - nativeCall 统一为 func(...uintptr) uintptr（x64 Windows 参数均为
//     指针宽度值）。
//   - 读回 C 字符串用 goCString：SDL 返回的是 C 堆内存指针，调用方
//     必须用 SDL_free 释放（readClipboardText 已处理）。
//
// # 事件解码
//
// decodeEvent 按 SDL2 的结构体定义解析（SDL_Event 是 56 字节联合体）：
// 键盘事件取 keysym.sym/mod@20/24，文本输入取 text[32]@12，滚轮取精确
// 坐标@28/32。字段偏移以真实 SDL 产生的事件实测为准，不要凭记忆改。
//
// # 测试
//
// backend/sdl2 的测试不依赖真实显示器：
//
//   - SDL_VIDEO_DRIVER=dummy + 软件渲染器，Window.Screenshot() 可回读
//     像素做断言（用完记得恢复剪贴板等全局状态）。
//   - 事件链路用 SDL_PushEvent 注入真实事件（run_events_test.go 的
//     pushMouseButtonUp / pushKeyDownMods 等），端到端验证解码→消费→
//     重排；只调 engine 的公开函数会绕过 SDL 这一层，漏掉真实 bug。
//   - 灰度文字保真探针：go test ./backend/sdl2 -run TestGrayscalePipelineFidelity
//     （GHROMEX_REAL=1 走真机 GPU）。
package sdl2
