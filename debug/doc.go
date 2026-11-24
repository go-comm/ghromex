// Package debug 是 ghromex 的本地调试服务：用 HTTP/JSON 驱动与观测一个
// 正在运行的窗口。
//
// # 为什么需要它
//
// 「点一下看看命中了谁」「滚一屏再看布局」「截个图对比」这类事，在没有
// JS 的桌面 GUI 里只能靠人肉或系统层脚本（AutoHotkey / PowerShell 模拟
// 鼠标）驱动——后者绕过了平台事件解码，**看不见真机 bug**（坐标修正、
// 焦点链、悬停档位那些）。本包把驱动能力收进引擎内部：注入走
// `browser.Window.InjectInput`（内部翻译成 SDL 事件再推送 → 后端解码 →
// 浏览器派发），与真人操作同一条链路。
//
// # 线程纪律（本包最重要的一条）
//
// SDL 的窗口与渲染器绑在 `NewWindow→Run→Close` 所在的那个 OS 线程，引擎
// 文档树也不接受跨线程访问。HTTP handler 跑在 net/http 的 goroutine 上，
// 因此：
//
//   - **读文档/截图/改 DOM 一律经 `Window.Do` 交回主循环执行**；
//   - **注入输入**走 `Window.InjectInput`（`SDL_PushEvent` 本身线程安全），
//     事件在主循环下一帧被消费；
//   - 等待（`wait`）是 handler goroutine 上的真实睡眠，好让滚动动画跨帧推进。
//
// # 安全边界
//
//   - **默认不开**：只有显式调用 `Serve`/`ServeFromEnv` 才有服务；
//   - **只绑 loopback**：非 127.0.0.1/localhost/::1 的地址一律拒绝启动；
//   - **不开 CORS**：网页不能跨源调它（也省掉「任意网页 POST 到本机端口」
//     这条攻击面）。要跨源请自己在宿主里加白名单。
//
// 它是开发调试设施，不是产品功能：不要把端口暴露到局域网，也不要在发布
// 路径里默认启用。
//
// # 端点
//
//	GET  /debug/state      窗口与文档状态（视口尺寸、文档标题、变更计数、悬停/焦点、选区）
//	GET  /debug/hit?x&y    命中测试：点上的元素链、矩形、光标形状
//	GET  /debug/dom        DOM 树快照（selector/depth/limit 可选）
//	GET  /debug/text       选择器对应元素的文本与取值（selector 必填）
//	GET  /debug/attr       选择器对应元素的单个属性值（selector+name 必填）
//	GET  /debug/shot.png   当前画面（PNG）
//	GET  /debug/layout.svg 当前布局（SVG）
//	POST /debug/doc        换文档（{"html":"..."} 或 {"path":"..."}）
//	POST /debug/input      执行一串输入动作（见下）
//	GET  /debug/help       端点与动作清单（JSON）
//
// # 输入动作
//
// 请求体是动作数组（也接受 {"actions":[...]} 的对象形式）：
//
//	[{"op":"click","x":320,"y":240},
//	 {"op":"wait","ms":120},
//	 {"op":"wheel","x":320,"y":240,"dy":-1},
//	 {"op":"key","key":"ctrl+s"},
//	 {"op":"text","s":"你好"},
//	 {"op":"drag","x":10,"y":20,"x2":200,"y2":20,"steps":8},
//	 {"op":"drop","files":["C:\\a.png"]}]
//
// 坐标是视口像素；滚轮档位「正 = 向上/向右」（`InputEvent` 的口径）；按键名
// 里 ctrl/shift/alt 与 enter/tab/F5/方向键都认（见 debug/input.go 的解析表）。
// 每个动作之后
// 服务会做一次「同步栅栏」（一个空的主循环任务），因此**下一个动作看到的是
// 上一个动作已经落地后的世界**——不用自己插 wait 去等事件派发。
//
// # 用法
//
//	if srv, err := debug.Serve(win, debug.Options{Addr: "127.0.0.1:0"}); err == nil {
//	    defer srv.Close()
//	    fmt.Println(srv.URL("/debug/"))
//	}
//
// 或用环境变量（见 ServeFromEnv）：`GHROMEX_DEBUG_PORT=9223`。
package debug
