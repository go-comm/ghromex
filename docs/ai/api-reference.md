# API 参考（自动生成）

> 本文件由 `tools/gen-ai-context.ps1` 从 `go doc -all` 自动生成，请勿手工编辑；接口变化后重新运行脚本即可。
> 生成时间: 2026-10-25 00:11

---

# package backend/clipboard
````
package clipboard // import "github.com/go-comm/ghromex/backend/clipboard"

Package clipboard 是系统剪贴板的平台实现。

目前的实现复用 SDL2 的剪贴板接口（SDL_GetClipboardText / SDL_SetClipboardText）——它们操作的正是
Windows 系统剪贴板 （CF_UNICODETEXT），引擎内部处理 Ctrl+C / Ctrl+V 时走的也是同一条路径，
所以应用层读写的与引擎粘贴进来的是同一份内容。

非 Windows 平台没有剪贴板实现：Text 返回空串、SetText 是空操作。

调用方用 runtime/clipboard（通用入口），别直接依赖本包。

FUNCTIONS

func SetText(text string)
    SetText 把 UTF-8 文本写入系统剪贴板。

    没有剪贴板实现时是空操作。写空串等于不动剪贴板（SDL 的接口如此）。

func Text() string
    Text 读取系统剪贴板中的文本（UTF-8）。

    剪贴板为空、里面不是文本，或没有剪贴板实现时返回空串。

````

---

# package backend/dialog
````
package dialog // import "github.com/go-comm/ghromex/backend/dialog"

Package dialog 是原生对话框的**平台实现**（Windows）。

文件对话框接 Shell 的 IFileDialog（Vista 起的现代对话框），纯 Win32 COM 调用、无 CGO；消息框接 user32 的
MessageBoxW（不需要 COM）。 其它平台由 dialog_other.go 提供同签名的空实现（返回 ErrUnsupported）。

调用方用 runtime/dialog（通用入口）：那里定义了 Parent（父窗口句柄的 包装）与平台无关的类型别名，本包只认 uintptr
形式的原生句柄。

它与渲染后端（backend/sdl2）是两件事：对话框是操作系统的能力，不依赖 SDL。父窗口句柄由调用方传入（backend/sdl2 的
Window.NativeHandle 给出 真实 HWND）——SDL_CreateWindow 返回的是 SDL_Window* 而非 HWND，原生对话框
要拿真正的 HWND 当父窗口（否则 Show 会回 E_INVALIDARG）。

线程约束：IFileDialog 必须在 STA（单线程套间）上创建并 Show，本包内部 会 LockOSThread
并用 CoInitializeEx(COINIT_APARTMENTTHREADED) 把当前线程 初始化成 STA；Show
期间阻塞该线程（对话框自带消息循环，父窗口的 SDL 事件循环随之暂停，这与原生应用「模态对话框期间父窗口不响应」一致）。

本包提供五个对话框：打开文件、打开多个文件、选择文件夹、另存为，以及 消息框（提示/警告/询问）。前四个走 Shell 的 IFileDialog，消息框走
user32 的 MessageBoxW（不需要 COM、不需要 STA）。

对话框是**模态**的：应在按钮/菜单的回调里直接调用。

VARIABLES

var ErrUnsupported = errors.New("ghromex 原生文件对话框仅支持 Windows")
    ErrUnsupported 表示当前平台没有原生文件对话框实现。


FUNCTIONS

func OpenDirectoryDialog(parent uintptr, opts Options) (string, error)
    OpenDirectoryDialog 弹出「选择文件夹」对话框，返回用户选中的目录。 用户取消返回 ("", nil)。

func OpenFileDialog(parent uintptr, opts Options) ([]string, error)
    OpenFileDialog 弹出「打开文件」对话框（单选）。

    parent 是父窗口的原生 HWND（0 = 无父窗口，对话框照样能弹）。 返回用户选中的文件路径。用户取消返回 (nil,
    nil)——取消不是错误。

func OpenMultipleFilesDialog(parent uintptr, opts Options) ([]string, error)
    OpenMultipleFilesDialog 弹出「打开文件」对话框（允许多选）。

    返回用户选中的文件路径（多个，顺序同对话框里的选择顺序）。用户取消 返回 (nil, nil)——取消不是错误。

func SaveFileDialog(parent uintptr, opts Options) (string, error)
    SaveFileDialog 弹出「另存为」对话框，返回用户输入的保存路径。 用户取消返回 ("", nil)。


TYPES

type FileFilter struct {
	// Name 是显示名，如 "文本文件 (*.txt)"。
	Name string
	// Pattern 是扩展名模式，如 "*.txt"；多个模式用 ";" 分隔
	// （"*.txt;*.md"）。
	Pattern string
}
    FileFilter 描述文件对话框里的一个类型过滤器（对应下拉框里的一项）。

type MessageButtons int
    MessageButtons 是消息框的按钮组合。

const (
	// ButtonsOK 只有「确定」。
	ButtonsOK MessageButtons = iota
	// ButtonsOKCancel 是「确定」「取消」。
	ButtonsOKCancel
	// ButtonsYesNo 是「是」「否」。
	ButtonsYesNo
	// ButtonsYesNoCancel 是「是」「否」「取消」。
	ButtonsYesNoCancel
)
type MessageIcon int
    MessageIcon 是消息框的图标。

const (
	// IconInfo 是信息图标（字母 i）。
	IconInfo MessageIcon = iota
	// IconWarning 是警告图标（感叹号）。
	IconWarning
	// IconError 是错误图标（红叉）。
	IconError
	// IconQuestion 是询问图标（问号）。
	IconQuestion
)
type MessageOptions struct {
	// Title 是标题栏文字。
	Title string
	// Message 是正文。
	Message string
	// Icon 是图标；零值是 IconInfo。
	Icon MessageIcon
	// Buttons 是按钮组合；零值是 ButtonsOK。
	Buttons MessageButtons
}
    MessageOptions 是消息框的选项。

type MessageResult int
    MessageResult 是用户在消息框里点击的按钮。

const (
	// ResultOK 是「确定」。
	ResultOK MessageResult = iota
	// ResultCancel 是「取消」。
	ResultCancel
	// ResultYes 是「是」。
	ResultYes
	// ResultNo 是「否」。
	ResultNo
)
func MessageDialog(parent uintptr, opts MessageOptions) (MessageResult, error)
    MessageDialog 弹一个模态消息框，返回用户点击的按钮。

    parent 是父窗口的原生 HWND（0 = 无父窗口，消息框照样能弹）。 用户点「取消」返回 ResultCancel；平台不支持时返回
    ErrUnsupported。

    消息框是模态的：调用期间阻塞当前线程（父窗口的 SDL 事件循环随之 暂停），因此应在按钮/菜单的回调里直接调用。

type Options struct {
	// Title 是对话框标题；留空则用系统默认标题（打开/另存为）。
	Title string
	// DefaultDir 是初始目录；留空则用系统上次使用过的目录。
	DefaultDir string
	// DefaultName 是初始文件名（另存为用）。
	DefaultName string
	// Filters 是文件类型过滤器；留空则不过滤（显示全部文件）。
	Filters []FileFilter
}
    Options 是文件对话框的公共选项。

````

---

# package backend/sdl2
````
package sdl2 // import "github.com/go-comm/ghromex/backend/sdl2"

Package sdl2 是 Ghromex 的 SDL2 渲染后端：窗口、事件解码、文本纹理 缓存与系统剪贴板。纯动态加载 SDL2.dll /
SDL2_ttf.dll，无需 CGO 与 C 工具链；非 Windows 平台由 stub_other.go 提供同签名的空实现 （NewWindow 返回
ErrUnsupported）。

# 分层：后端不认识引擎

本包是**平台层**：只做窗口/渲染器/字体纹理/事件解码/绘制，向上的契约 只有 core 包的（core.Graphics /
core.Viewport）。它不 import 引擎， 也不持有文档：

  - 平台事件由 PollEvent() 交出去（Event 类型在本包），怎么解释成引擎 动作是上一层（browser）的事；
  - 绘制走 core.Graphics 的方法集，实现与引擎用的是同一份接口 （core.Graphics 与 engine.Graphics
    是别名），所以两边都满足。

文档与主循环（事件 → 引擎动作 → 重排 → 重绘）在 browser 层。

原生文件对话框不在本包：它是 Windows Shell 的能力，与渲染后端无关。 调用入口在 runtime/dialog（通用 API），平台实现在
backend/dialog。

# 平台与线程约束

  - 仅 Windows 可用：实现文件都是 //go:build windows。
  - 同一 OS 线程：SDL 要求窗口的创建、事件循环、销毁在同一线程， 因此 NewWindow → Run → Close 必须在同一个
    goroutine（NewWindow 会 LockOSThread，Close 时解锁）。另开 goroutine 操作窗口会出错。
  - 依赖 DLL 放在工程根的 libs/（见 libs/README.md）：findLibsDir 依次 探测 ./libs、可执行文件旁的
    libs/、可执行文件目录，并在上溯目录 （./、../、../../）里再找一遍；命中后用 SetDllDirectory 把该目录 加进 DLL
    搜索路径，SDL2_ttf 依赖的 zlib1.dll 才能被找到。

# 原生窗口句柄

Window.NativeHandle 返回窗口的原生 HWND，供原生对话框当父窗口： SDL_CreateWindow 返回的是
SDL_Window*，不是 HWND——原生对话框要拿真 HWND 当父窗口（拿错会让 Show 直接回 E_INVALIDARG），只能问
SDL_GetWindowWMInfo。取不到时返回 0（对话框照样能弹，只是不挂主窗口）。

# 动态加载约定

  - 符号用 bindSdl / bindTtf 绑定（必需，缺失即启动失败），或 bindProcOpt / bindProbe
    绑定（可选，缺失即按能力降级，如缺 SDL_GetClipboardText 则粘贴不可用）。
  - nativeCall 统一为 func(...uintptr) uintptr（x64 Windows 参数均为 指针宽度值）。
  - 读回 C 字符串用 goCString：SDL 返回的是 C 堆内存指针，调用方 必须用 SDL_free 释放（readClipboardText
    已处理）。

# 事件解码

decodeEvent 按 SDL2 的结构体定义解析（SDL_Event 是 56 字节联合体）： 键盘事件取
keysym.sym/mod@20/24，文本输入取 text[32]@12，滚轮取精确 坐标@28/32。字段偏移以真实 SDL
产生的事件实测为准，不要凭记忆改。

# 测试

backend/sdl2 的测试不依赖真实显示器：

  - SDL_VIDEO_DRIVER=dummy + 软件渲染器，Window.Screenshot() 可回读
    像素做断言（用完记得恢复剪贴板等全局状态）。
  - 事件链路用 SDL_PushEvent 注入真实事件（run_events_test.go 的 pushMouseButtonUp /
    pushKeyDownMods 等），端到端验证解码→消费→ 重排；只调 engine 的公开函数会绕过 SDL 这一层，漏掉真实 bug。
  - 灰度文字保真探针：go test ./backend/sdl2 -run TestGrayscalePipelineFidelity
    （GHROMEX_REAL=1 走真机 GPU）。

CONSTANTS

const (
	KmodShift = 0x0003 // 左右 Shift 合并的位
	KmodCtrl  = 0x00C0 // 左右 Ctrl 合并的位
	KmodAlt   = 0x0300 // 左右 Alt 合并的位
)
    修饰键位掩码（与 SDL 的 KMOD_* 一致）。

    事件的 Mod 字段是平台掩码，含义由平台层定义：上层要靠它区分 「Ctrl+V 组合键」与「真在打字」。

const (
	MouseButtonLeft   = 1
	MouseButtonMiddle = 2
	MouseButtonRight  = 3
)
    鼠标按键号（与 SDL 的 SDL_BUTTON_* 一致）。


VARIABLES

var ErrUnsupported = errors.New("ghromex SDL2 后端仅支持 Windows")
    ErrUnsupported 表示当前平台没有可用的 SDL2 后端。

    只有非 Windows 平台会返回它（NewWindow/OpenDocument/Run/Screenshot）； Windows 上加载
    SDL2.dll 成功与否会给出各自的错误。放在无构建标签的 文件里，跨平台代码才能引用它并用 errors.Is 判断。


FUNCTIONS

func CJKFontPath() string
    CJKFontPath 返回 CJK per-script fallback 字体，对齐 Chrome（zh Windows）：微软雅黑。 可用
    GHROMEX_CJK_FONT 覆盖。

func ClipboardText() string
    ClipboardText 读取系统剪贴板文本（UTF-8）。剪贴板为空或 SDL 未导出 相关符号时返回空串。

func DefaultFontPath() string
    DefaultFontPath 返回 standard 字体（未指定 family 时使用）。 默认 sans-serif（Arial）：桌面 UI
    习惯，有意区别于 Chrome 的 standard=Times。 可用 GHROMEX_FONT 覆盖。

func LCDTextEnabled() bool
    lcdText 报告 LCD 子像素渲染是否启用（TTF_RenderUTF8_LCD + LIGHT_SUBPIXEL
    hinting）。默认关闭：文本默认灰度 AA（hinting 档位见 fontHinting，默认 NORMAL；GHROMEX_HINT
    可切 light|mono|none），无彩边、对落点背景色不敏感。 GHROMEX_LCD=1 开启：竖笔画边缘横向锐度与浏览器
    ClearType 同级，代价是彩底上竖笔边缘轻微彩边（与浏览器子像素渲染同性质），且需按 落点背景色合成（见 DrawText）。
    惰性函数而非包级变量：hasTTFLCD 在 Load()（DLL 符号解析）后才有值， 包初始化时求值恒为 false。 LCDTextEnabled
    报告当前是否走 LCD（子像素）文本路径：既要环境变量 GHROMEX_LCD=1 打开，也要 DLL 真的导出 TTF_RenderUTF8_LCD。

    供上层（browser 层）判断「这条渲染路径在本机可用吗」——LCD 相关的 断言要用真实字形贴图，路径不可用时只能跳过。

func Load() error
    Load 加载 SDL2 与 SDL2_ttf 动态库并解析符号，重复调用安全。

func RendererDriver(renderer uintptr) string
    RendererDriver 返回渲染后端的驱动名（"direct3d11" / "opengl" / "software" / "llvmpipe"
    …），取自 SDL_GetRendererInfo。

    用于诊断：软件路径的瓶颈是**填充率**（每像素纳秒数），GPU 路径的瓶颈 通常在别处（纹理上传次数、draw call 数、swap
    间隔）——两者的排查方向 完全不同，先确认走的是哪条路再量。

func SetClipboardText(text string)
    SetClipboardText 把 UTF-8 文本写入系统剪贴板。

    引擎只负责选区文本，剪贴板读写属平台能力：应用经 runtime/clipboard （通用入口）→
    backend/clipboard（平台实现）落盘，engine 保持纯 Go、 无平台依赖。


TYPES

type Event struct {
	Kind EventKind

	// X/Y 是鼠标位置（像素，视口坐标）。
	X, Y int
	// Button 是鼠标按键号（MouseButtonLeft 等）。
	Button uint8
	// WheelX/WheelY 是滚轮滚动量（向上为正，与 SDL 一致）。
	WheelX, WheelY int

	// Key 是键码（与 SDL 的 SDLK_* 一致；字母键即 ASCII 码），Mod 是
	// 修饰键位掩码（KmodShift/KmodCtrl/KmodAlt）。
	Key int
	Mod uint16

	// Text 是文本输入内容（UTF-8）。
	Text string

	// Width/Height 是窗口的新尺寸（仅 EventWindowResized 有意义）。
	Width, Height int
}
    Event 是一个平台事件。

type EventKind uint8
    EventKind 是平台事件的种类。

    事件是「平台层的事实」，不是「引擎的动作」：鼠标移动、按键、窗口 尺寸变化由后端解码，具体怎么解释成引擎行为（点击、选区、滚动）由
    上一层做。这样后端不认识引擎，依赖方向保持单向。

const (
	// EventUnknown 是零值：表示「不关心的事件」（窗口移动、鼠标进入/
	// 离开等）。它必须是零值——解码不出种类的事件会落到这里，上层按
	// 未知事件忽略，不能被误当成退出。
	EventUnknown EventKind = iota
	// EventQuit 是退出请求（用户关窗 / 应用请求关闭）。
	EventQuit
	// EventWindowClose 是窗口被关闭（标题栏的关闭按钮）。
	EventWindowClose
	// EventWindowResized 是窗口尺寸变化（Width/Height 为新尺寸）。
	EventWindowResized
	// EventWindowExposed 是窗口重新露出（内容可能已丢，需要重绘）。
	EventWindowExposed
	// EventMouseMotion 是鼠标移动（X/Y 为当前位置）。
	EventMouseMotion
	// EventMouseLeave 是鼠标离开窗口：上层据此清掉悬停态（:hover）。
	EventMouseLeave
	// EventMouseButtonDown 是鼠标按下（Button 为按键号，X/Y 为位置）。
	EventMouseButtonDown
	// EventMouseButtonUp 是鼠标抬起。
	EventMouseButtonUp
	// EventMouseWheel 是滚轮（WheelX/WheelY 为滚动量）。
	EventMouseWheel
	// EventKeyDown 是按键按下（Key 为键码，Mod 为修饰键位掩码）。
	EventKeyDown
	// EventTextInput 是文本输入（UTF-8；中文 IME 的组合串也走这里）。
	EventTextInput
)
    平台事件种类。

type Graphics struct {
	// Has unexported fields.
}
    Graphics 基于 SDL2 Renderer + SDL_ttf 实现 core.Graphics。 每个字体文件只打开一个
    FT_Face（动态切字号/粗体），文本纹理按 （解析后的文件+子段+字号+粗体+颜色）缓存；含 CJK 字符的段用 fallback 字体。

func NewGraphics(renderer uintptr, fontPath string) *Graphics
    NewGraphics 创建渲染上下文。renderer 为 SDL_Renderer 句柄； fontPath 非空时作为 standard
    字体（测试/定制用）。

func (g *Graphics) Clear(r, gr, b, a uint8)
    Clear 清屏。同时把裁剪栈清空并关闭 SDL 裁剪（SDL_RenderClear 本身无视 裁剪区，这里仅为让 Go 侧栈与 SDL
    实际状态保持一致，防上一帧配对失衡）。 也是每帧的起点：在这里刷新文本渲染的环境变量取值（见 refreshEnv）。

func (g *Graphics) CloseFonts()
    CloseFonts 释放全部 face（由 Window.Close 调用）。

func (g *Graphics) DrawColor(x, y, w, h int, color core.Color)
    DrawColor 实现 core.Graphics。

func (g *Graphics) DrawImage(x, y, w, h int, image core.Image)
    DrawImage 暂不支持（预留接口）。

func (g *Graphics) DrawSprite(x, y int, s *core.Sprite)
    DrawSprite 实现 core.SpriteDrawer：把位图精灵上传成纹理并 blit 一次。

    精灵按指针缓存（引擎侧的角标精灵按「半径+颜色」缓存，同一份像素只上传 一次）。整块圆角一次 blit，替掉逐像素的半透明小方块——后者是绘制调用
    的主要来源（实测 portal 页一帧 1.8 万次 1×1 绘制），GPU 路径上每次都 是驱动调用，帧时间被拖到几十毫秒，滚动动画被切成两三帧。

func (g *Graphics) DrawText(x, y, w, h int, paint core.Paint, text string)
    DrawText 实现 core.Graphics：按脚本段绘制纹理，x 依次推进各段测量宽度， 各段以基线（TTF_FontAscent）对齐。

func (g *Graphics) FontMetrics(text string, fontSize int, bold bool, family string) (int, int)
    FontMetrics 实现 core.Graphics：返回行度量（基线以上 ascent、基线以下 descent）。各脚本段取最大值：行盒高度
    = 各内联盒 ascent 的最大者 + descent 的最大者（同基线对齐）。度量来自 SDL_ttf 的 TTF_FontAscent/
    TTF_FontDescent，行高与 Chrome/DirectWrite 可能有 1~4px 差异。 text 为空时只取主字体（不触发 CJK
    逐字回退），对应行盒的 strut。

func (g *Graphics) MeasureText(text string, fontSize int, bold bool, family string) (int, int)
    MeasureText 实现 core.Graphics。与 DrawText 同一套脚本分段与字体解析， 宽度为各段之和，行高取各段最大值（近似
    Chrome 的行盒语义）。

func (g *Graphics) PopClip()
    PopClip 实现 core.Clipper：弹出栈顶并应用剩余栈顶（空栈=关闭裁剪）。

func (g *Graphics) Present()
    Present 提交帧。

func (g *Graphics) PushClip(x, y, w, h int)
    PushClip 实现 core.Clipper：与当前裁剪区求交后下推并应用。 裁剪区由引擎在 padding 盒/滚动容器上给出，坐标即渲染坐标。

type Window struct {
	// Has unexported fields.
}
    Window 是 SDL2 窗口：平台层，只负责窗口/渲染器/字体纹理/事件解码/绘制。

    这里**不认识引擎**：文档与主循环（事件 → 引擎输入处理 → 重排 → 重绘） 在上一层（browser）做。后端只提供三样东西：

      - 视口契约（core.Viewport / core.Graphics）：引擎靠它量尺寸、落笔；
      - 事件（Event）：平台事件由上层解释成引擎动作；
      - 呈现（Present）：把这一帧提交到窗口。

    这样依赖方向是单向的：core ← engine，core ← backend/sdl2。

func NewWindow(w, h int, title string) (*Window, error)
    NewWindow 创建带渲染器的窗口。坐标体系 1:1（1 CSS px = 1 物理像素）。

    注意：SDL（Direct3D 后端）要求窗口与渲染器的创建、事件循环、销毁 发生在同一个 OS 线程上，否则退出释放资源时会死锁。这里在创建时锁定
    当前 goroutine 到 OS 线程，Close 时解锁；调用方应在同一个 goroutine 中依次使用 NewWindow → Run →
    Close（典型即 main goroutine）。

func NewWindowWithOptions(o WindowOptions) (*Window, error)
    NewWindowWithOptions 同 NewWindow，可指定位置与窗口类型。

    无论哪种窗口都先隐藏，等首帧 Present 后再显示（见 Present 的说明）： 截图遮罩如果建窗即显示，会先闪一帧黑底再出现桌面冻结图。

func (win *Window) Close()
    Close 释放窗口资源。幂等：重复调用直接返回——重复 SDL_Destroy* 会对已
    销毁对象二次释放（use-after-free），句柄不置零也会让后续绘制打到野指针。

func (win *Window) Closed() bool
    Closed 报告窗口是否已关闭（Close 之后为 true）。

func (win *Window) Delay(ms int)
    Delay 等待若干毫秒（主循环空闲时用来省 CPU，不空转）。

func (win *Window) Graphics() core.Graphics
    Graphics 实现 core.Viewport。

func (win *Window) InjectEvent(ev Event)
    InjectEvent 往事件队列注入一个平台事件（测试用）。

    真实事件由 SDL 产生；测试要用「按下鼠标」「敲键盘」这类事件驱动窗口， 就靠它——比在测试里手搓 SDL_Event 字节布局稳得多。

    字节布局按 SDL2 的事件结构填（字段偏移见 sdl2.go 的 decodeEvent 注释）： 类型码用 SDL
    的值，事件体内各字段的位置与真实事件一致，decodeEvent 才能照常解出。

func (win *Window) ModState() uint16
    ModState 返回当前修饰键位掩码（KmodShift/KmodCtrl/KmodAlt）。

    文本输入通道会混进 Ctrl+V 之类的控制码，要靠它把「组合键」和「打字」 分开——所以掩码的含义属于平台层，由后端定义。

func (win *Window) MoveTo(x, y int)
    MoveTo 运行期移动窗口（拖动悬浮预览）。坐标是虚拟桌面的全局坐标。

func (win *Window) NativeHandle() (uintptr, error)
    NativeHandle 返回窗口的原生 HWND（Windows），供原生对话框（runtime/dialog 的 IFileDialog）当父窗口。

    SDL_CreateWindow 返回的是 SDL_Window*，不是 HWND——原生对话框要拿真正的 HWND 当父窗口（否则 Show 会回
    E_INVALIDARG），只能问 SDL_GetWindowWMInfo。 取不到时返回 0：对话框照样能弹，只是不再挂在主窗口下面。

func (win *Window) PollEvent() (Event, bool)
    PollEvent 取下一个平台事件；队列为空时返回 false。

    事件怎么解释成引擎动作（点击、按键、滚轮）是上一层的事：后端只解码 SDL 事件，不认识文档与引擎。

func (win *Window) Present()
    Present 提交当前帧；首帧提交后才真正显示窗口。

    时序原因见 NewWindow 中 windowHidden 的注释：黑底出现在“建窗可见”与
    “首帧就绪”之间的空窗期，因此建窗隐藏，等这一帧内容进了交换链 （Present 已返回）再 ShowWindow——窗口一出现就是完整页面，没有黑闪。
    RaiseWindow 复原建窗即可见时的初始前台/焦点（否则可能落在别的窗口后面）。

func (win *Window) RendererInfo() string
    RendererInfo 返回渲染路径的描述（"accelerated(GPU, driver=direct3d11)" / "software
    (driver=software; …)"）。诊断与基准工具据此归因——数字必须 能说明「是哪条路径、哪个驱动量出来的」，否则无法解读。

func (win *Window) RequestClose()
    RequestClose 请求关闭窗口：向事件队列压入一个 SDL_QUIT 事件，事件
    循环收到后按「用户点关闭按钮」的同一条路径退出（应用层的「退出」 菜单项用）。

func (win *Window) Screenshot() ([]byte, uint32, int, int, error)
    Screenshot 回读渲染目标当前内容（32bpp 原生字节，行主序）及其像素格式与尺寸。 优先使用窗口的原生像素格式（SDL2 的
    SDL_GetWindowPixelFormat）；查不到 或非 32bpp 时固定请求 XBGR8888（0x16562004，小端下内存字节序
    R,G,B,X）， 由 SDL 负责转换。注意渲染目标无 alpha 通道时第 4 字节恒为 0（不是 alpha）， 颜色只能按前三个通道读。 配合
    SDL_VIDEO_DRIVER=dummy 可在无显示器环境验证渲染结果。

func (win *Window) SetSize(w, h int)
    SetSize 运行期修改窗口尺寸（截图预览从整屏缩成选区大小）。

    立即更新视口尺寸：SDL_SetWindowSize 的 WINDOWEVENT 要到下一轮事件
    才到，而调用方通常在同一轮里还要打开新文档/重排——等事件会让第一帧 按旧尺寸布局。事件到达后再写一次同样的值，没有副作用。

func (win *Window) SetTitle(title string)
    SetTitle 设置窗口标题（应用层可随时更新，如编辑器显示当前文件名）。

func (win *Window) SetVSync(on bool)
    SetVSync 开/关垂直同步（诊断用）。

    加速路径的 Present **含等 vsync 的阻塞**（不阻塞就抢不到交换链）， 所以「Present
    的耗时」既含拷贝也含等待，两个数混在一起没法归因。 关掉 vsync 后再量一次 Present，得到的就是纯拷贝耗时，二者相减即 vsync
    等待。软件路径没有 vsync，此调用无副作用。

func (win *Window) Shown() bool
    Shown 报告窗口是否已经显示（首次 Present 之后为 true）。

    窗口建出来时是隐藏的：交换链在首次 Present 前没有有效画面，可见就会 先闪一段黑底。

func (win *Window) Ticks() uint32
    Ticks 返回自 SDL 初始化以来的毫秒数（主循环用来算自动退出时刻）。

func (win *Window) ViewportHeight() int
    ViewportHeight 实现 core.Viewport。

func (win *Window) ViewportWidth() int
    ViewportWidth 实现 core.Viewport。

type WindowOptions struct {
	Width, Height int
	Title         string

	// X/Y 是窗口左上角的位置；Centered 为 true 时忽略 X/Y。
	//
	// 允许负数：多显示器时副屏可能在主屏左边（虚拟桌面原点为负）。
	// SDL 的位置参数是 int，按低 32 位解释，负值会正确传过去。
	X, Y     int
	Centered bool

	Borderless  bool
	AlwaysOnTop bool
	SkipTaskbar bool
	Resizable   bool
}
    WindowOptions 是建窗参数。零值表示位于 (0,0) 的固定尺寸普通窗口（带边框）， 与 NewWindow
    的默认值不同；截图工具的悬浮窗显式关掉边框/缩放并置顶。

    放在无构建标签的文件里：Windows 实现与非 Windows 空实现共用同一份 字段定义，新增字段时两边不会漂移。

````

---

# package browser
````
package browser // import "github.com/go-comm/ghromex/browser"

本文件是 GHROMEX_BENCH 分段基准工具：把主循环的各段分开计时， 累积足够样本后报 p50/p95/p99。

# 为什么要分段

「整帧多少毫秒」这个数字几乎没有诊断价值：
  - 加速路径的整帧恒为 ~16.6ms（vsync 间隔），**与实际工作量无关**；
  - dummy 驱动下 Present 是空操作，纯绘制与含 present 差一个数量级 （2560×1440：纯绘制 ~1.5ms vs 含
    present ~4ms）。

两种口径的数字并排会得出「快了 2.7 倍」之类的错误结论。分段计时 才能说清「时间花在哪儿」，也能把 vsync 等待从真实工作里剥出来。

# 不进 CI

计时受机器负载、驱动、电源策略、温度影响，数值不可复现——把它写成 断言只会得到一个 flaky 的测试。它是本地诊断工具，与 GHROMEX_DEBUG
同一层：手动跑、看数字、不做断言。

# 口径：软件路径的「重绘」是入队，不是光栅化

SDL 的软件光栅器把 DrawColor/DrawText **记进命令表**，真正填像素发生在 Present 时。所以软件路径上：

    重绘段  = 命令入队 + 遍历（与面积几乎无关，与调用数相关）
    呈现段  = 真正的像素填充 + 拷贝到窗口（与面积相关）

加速路径相反：命令提交给驱动就返回，填充在 GPU 上，Present 里等 vsync。

这也解释了「纯绘制 1.5ms」与「整帧 4ms」为什么能对上：后者是前两者之和。

# 用法

    GHROMEX_BENCH=1 go run ./examples/portal
    GHROMEX_BENCH=1 GHROMEX_BENCH_FRAMES=600 go run ./examples/portal

输出示例（软件路径，dummy 驱动，1280×820）：

    ghromex bench: renderer=software (driver=software; 强制软件：GHROMEX_SOFTWARE/dummy)
      窗口=1280x820  样本=400  帧周期 p50=3.04ms
      布局 p50=0.00ms p95=0.00ms p99=0.00ms
      清屏 p50=0.00ms p95=0.00ms p99=0.52ms
      重绘 p50=1.19ms p95=2.23ms p99=2.75ms  ← 软件光栅器：此处只是命令入队，像素在「呈现」段才填
      呈现 p50=1.87ms p95=3.34ms p99=3.90ms  ← 软件路径：只是拷贝，无 vsync
      空闲 p50=0.00ms p95=0.00ms p99=0.00ms  ← 基准模式不进入休眠（模拟持续动画，量最大吞吐）
      工作合计（布局+清屏+重绘）p50=1.20ms p95=2.28ms p99=2.85ms
      每帧分配 246KB（Go 侧累计分配）
      绘制调用 p50=1194 p95=1194 p99=1194（每帧）
      堆常驻 3.1MB（GC 后；开跑基线 3.0MB）

同一页面同一尺寸走加速路径时：

    ghromex bench: renderer=accelerated(GPU, driver=direct3d)
      窗口=1280x820  样本=400  帧周期 p50=16.47ms
      重绘 p50=1.32ms p95=2.43ms p99=3.08ms
      呈现 p50=14.62ms p95=15.84ms p99=16.19ms  ← 含等 vsync≈14.05ms（关 vsync 实测纯拷贝 0.57ms）

注意两边的「帧周期」不可横比：软件路径按工作量跑（3ms），加速路径被 vsync 锁在 16.6ms，其中 14ms 是纯等待。要比就比真实工作（重绘 +
拷贝）。

本文件是基准模式用的绘制调用计数器：套在真实 Graphics 外面数调用。

为什么要数：光看「重绘多少毫秒」不知道该优化什么。把调用数与耗时 并排看，才能判断时间随**调用数**涨还是随**面积**涨——两者的优化方向 完全不同（减调用
vs 减填充）。

Package browser 是面向应用层的装配门面：把渲染后端（当前为 backend/sdl2）的选择与直接依赖挡在应用之外，应用只需 import
browser 与 engine，不必接触底层类型。

# 分工

  - core：引擎与后端之间的契约（Graphics / Viewport / Paint / Color…）， 叶子包，engine 与 backend
    都只依赖它；
  - engine：渲染引擎，纯 Go，不认识窗口与剪贴板；
  - backend/sdl2：平台实现层（Windows），提供窗口、事件解码、绘制、 剪贴板——**不认识引擎**，只按 core 的契约干活；
  - browser：应用层，持有文档并跑主循环（平台事件 → 引擎动作 → 重排 → 重绘），把引擎与后端缝在一起。

主循环放在这一层而不是后端：后端只该做平台的事。事件怎么解释成引擎 动作（点击、选区、滚轮）、什么时候重排重绘，都是应用层的事；后端若
直接依赖引擎，平台层就反过来依赖了它上面的层（core/arch_test.go 把 这条规则钉死）。

平台差异收在 backend/sdl2：非 Windows 下 NewWindow 返回 ErrUnsupported，
本层原样透传，应用可据此提示「当前平台不支持」。

API 按「可多次 New」塑形（每次返回独立窗口）；多窗口共存的底层基础 （SDL 子系统引用计数、windowID 事件路由、face
缓存归属）待真实需求出现 时在 backend 内解决，届时本层对外签名不变。

# 线程约定

Window 的生命周期（New → OpenDocument → Run → Close）必须在同一个 goroutine：SDL
要求窗口的创建、事件循环与销毁在同一 OS 线程。

# 常用片段

    win, err := browser.New(900, 620, "记事本")
    if err != nil {
    	log.Fatal(err)
    }
    defer win.Close()
    doc, err := win.OpenDocument(string(page))
    if err != nil {
    	log.Fatal(err)
    }
    // ...绑定事件、注册 doc.SetOnChanged...
    err = win.Run()

需要外部样式表（页面里的 <link rel="stylesheet"> 与 CSS 的 @import）时 改用 OpenDocumentWith
并传入加载器——引用于解析期就加载，事后再装加载器 补不上：

    doc, err := win.OpenDocumentWith(string(page), engine.DocumentOptions{
    	BaseURL:        "file:///C:/pages/index.html",
    	LoadStylesheet: loadFromDisk, // func(href string) (string, error)
    })

窗口标题、关窗等平台能力也由本层转发（SetTitle / RequestClose）——
引擎只管文本，剪贴板读写留在平台侧（runtime/clipboard，engine 保持 纯 Go、可在无显示环境测试）。

# 原生对话框

原生对话框不在本包：调用入口是 runtime/dialog（通用 API），平台实现 在 backend/dialog。本包只提供
NativeHandle——原生对话框要挂在应用 窗口下面，需要平台的原生窗口句柄：

    hwnd, err := win.NativeHandle()
    paths, err := dialog.OpenFileDialog(dialog.Parent(hwnd), dialog.Options{
    	Title: "打开",
    	Filters: []dialog.FileFilter{
    		{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
    	},
    })

对话框是模态的：调用期间阻塞当前线程（父窗口的 SDL 事件循环随之 暂停），因此在按钮/菜单的回调里直接调用即可。非 Windows 平台返回
dialog.ErrUnsupported。

# 出图口径（别混）

分析走无头两路，产物不同、用途不同；截图是另一件事：

  - doc.DumpSVG()（-dump-svg）：级联+布局序列化成 SVG，文字是真实
    <text>、可在浏览器里看真字体；不经过光栅绘制器，反映的是「布局与 样式」，不是最终像素（圆角精灵、抗锯齿看不到）。核对几何/层级/配色。
  - -dump-png（示例 flag，完整流程）：装外壳 → Refresh 重排 → RenderNode 光栅化进缓冲 → 最后
    buf.DumpPNG 落盘。产物是「当前文档 在视口的完整一帧」，软件光栅后端文字是占位方块——CI 逐像素断言的 首选。
  - BufferGraphics.DumpPNG(path)（仅缓冲快照）：只是把缓冲里**已有**的
    像素编码成文件，不布局、不绘制——没渲染过是空图，没重排就渲染则是 旧帧。它是上一条流程的最后一步（示例的 -dump-png 都终结于此），单独
    调用只适合「我刚 RenderNode 完，把现场存下来」这类场景，别当出图入口。
  - Window.Screenshot()：回读真实窗口渲染目标的当前内容（原生字节 + 格式 + 尺寸），是「用户实际看到的样子」；要存 PNG
    由应用自己编码 （见 examples/snip 的 saveWindowPNG）。

一句话：分析用 -dump-svg / -dump-png，快照存档用 DumpPNG（先自己 Refresh + RenderNode），截图用
Screenshot。

VARIABLES

var ErrUnsupported = sdl2.ErrUnsupported
    ErrUnsupported 表示当前平台没有可用的渲染后端，等价于 sdl2.ErrUnsupported。

    平台差异收在 backend/sdl2：非 Windows 下 New 会返回它，Windows 上不会
    出现。这里保留别名，调用方不必为了判断平台而直接依赖后端包。


FUNCTIONS

func ClipboardText() string
    ClipboardText 读取系统剪贴板文本（UTF-8）。剪贴板属平台能力：引擎只 给出选区文本，读写由后端完成。非 Windows 平台返回空串。

func SetClipboardText(text string)
    SetClipboardText 把 UTF-8 文本写入系统剪贴板。非 Windows 平台为空操作。


TYPES

type MouseEvent struct {
	Kind   MouseEventKind
	X, Y   int
	Button int
}
    MouseEvent 是窗口级原始鼠标事件。Button 用 SDL 的编号（1 左、2 中、3 右）； Move 事件的 Button 没有意义。

type MouseEventKind uint8
    MouseEventKind 是原始鼠标事件的类型。

const (
	MouseDown MouseEventKind = iota
	MouseMove
	MouseUp
)
type Options struct {
	Width, Height int
	Title         string
	// X/Y 是窗口左上角；Centered 为 true 时忽略。
	X, Y     int
	Centered bool

	Borderless  bool
	AlwaysOnTop bool
	SkipTaskbar bool
	Resizable   bool
}
    Options 是建窗参数。零值表示位于 (0,0) 的固定尺寸普通窗口（带边框）； 截图工具的悬浮窗显式设置
    Borderless/AlwaysOnTop/SkipTaskbar。

type Window struct {
	// Has unexported fields.
}
    Window 是一个可承载 HTML 文档的窗口，实现 engine.Viewport。

    生命周期约定：New → OpenDocument → Run → Close，且须在同一 goroutine （SDL 要求窗口/渲染器/事件循环同
    OS 线程）。

    主循环在这一层：后端（backend/sdl2）只提供窗口、绘制与解码后的平台 事件，「事件 → 引擎动作 → 重排 → 重绘」由这里串起来。分层理由见
    core 包的说明。

func New(w, h int, title string) (*Window, error)
    New 创建一个窗口。坐标体系 1:1（1 CSS px = 1 物理像素）。

func NewWithOptions(o Options) (*Window, error)
    NewWithOptions 同 New，可指定位置与窗口类型（无边框/置顶/不占任务栏）。

func (win *Window) Close()
    Close 释放窗口资源。

func (win *Window) Graphics() engine.Graphics
    Graphics 实现 engine.Viewport。

func (win *Window) InjectEvent(ev sdl2.Event)
    InjectEvent 往事件队列注入一个平台事件（测试用）。

    真实事件由 SDL 产生；测试要用「按下鼠标」「敲键盘」这类事件驱动窗口， 就靠它——比在测试里手搓平台事件字节布局稳得多。

func (win *Window) MoveTo(x, y int)
    MoveTo 移动窗口（拖动截图预览）。坐标是虚拟桌面的全局坐标。

func (win *Window) NativeHandle() (uintptr, error)
    NativeHandle 返回窗口的原生句柄（Windows 上是 HWND），供 runtime/dialog 的原生对话框当父窗口。

    取不到时返回 ErrUnsupported（例如无头模式或非 Windows 平台）。

func (win *Window) OpenDocument(src string) (engine.HTMLDocument, error)
    OpenDocument 解析并布局 HTML 源码，绑定到本窗口。

func (win *Window) OpenDocumentWith(src string, opts engine.DocumentOptions) (engine.HTMLDocument, error)
    OpenDocumentWith 同 OpenDocument，可传文档选项（外部样式表加载器、地址 基准）。

    选项只能在这里给：<link rel="stylesheet"> 与 @import 在解析阶段就加载，
    文档返回时已经处理完，事后再装加载器不会补加载页面里原有的引用。

func (win *Window) RenderFrame()
    RenderFrame 手动绘制一帧（清屏+渲染+提交），供无头渲染测试/截图使用； 常规用法交给 Run 的帧循环。

func (win *Window) RequestClose()
    RequestClose 请求关闭窗口（应用层「退出」菜单项用）：向事件队列压入 一个退出事件，事件循环收到后正常返回，资源仍由 Close 释放。

func (win *Window) Resize(w, h int)
    Resize 改变窗口尺寸并强制下一帧重排。

    只改尺寸不换文档时也要重排（文档变更计数不会动），所以这里把 lastChange 置 -1：主循环的“计数变了才布局”判断因此必然成立。

func (win *Window) Run() error
    Run 进入事件循环：处理关闭/尺寸变化/暴露/点击，并按内容变化重排、逐帧重绘。

func (win *Window) Screenshot() ([]byte, uint32, int, int, error)
    Screenshot 回读渲染目标当前内容（原生字节+格式+尺寸）。

func (win *Window) SetAutoExit(d time.Duration)
    SetAutoExit 让 Run 在窗口打开指定时长后自动返回（调试/CI 用）。

func (win *Window) SetBackdrop(s *core.Sprite)
    SetBackdrop 设置窗口背景图（nil 清除）。清屏之后、文档之前贴在 (0,0)。

    背景图不参与布局：宽高应与窗口视口一致，像素格式为 RGBA8888 且 alpha=255（见 backdrop 字段的说明）。传新精灵后下一帧生效。

func (win *Window) SetKeyHandler(f func(key int, mods uint16) bool)
    SetKeyHandler 注册应用级按键处理器：key 为引擎键码（字母是 rune）， mods
    为引擎修饰位（engine.ModCtrl/ModShift/ModAlt）。返回 true 表示 事件已消费，不再交给 Ctrl+C/V 与文档。

    放在这一层而不是 engine：谁先看到按键是**事件循环**的策略——应用命令 优先于页面，页面优先于默认动作。

func (win *Window) SetMouseHandler(f func(ev MouseEvent) bool)
    SetMouseHandler 注册窗口级原始鼠标钩子：在引擎的点击/文本选择之前调用， 返回 true 表示事件已消费。传 nil 取消。

    放在这一层而不是引擎：选区/拖动悬浮窗这类交互属于应用，不需要页面元素 参与；引擎也不该为每个工具加一套专用事件。

func (win *Window) SetOnDocument(f func(engine.HTMLDocument))
    SetOnDocument 注册文档生命周期回调：每次 OpenDocument/OpenDocumentWith 成功后调用一次。传 nil 取消。

    回调里可以改 DOM（外壳注入、占位块、样式表）：变更会让文档计数 +1， 主循环下一帧自然重排重绘。

func (win *Window) SetTitle(title string)
    SetTitle 更新窗口标题（如编辑器显示当前文件名）。

func (win *Window) ViewportHeight() int
    ViewportHeight 实现 engine.Viewport。

func (win *Window) ViewportWidth() int
    ViewportWidth 实现 engine.Viewport。

````

---

# package core
````
package core // import "github.com/go-comm/ghromex/core"

Package core 是渲染引擎与平台后端之间的契约层。

它是叶子包：不依赖仓库内任何其它包。引擎与后端都只依赖它，于是 依赖方向是单向的：

    core  ←  engine（实现契约，纯 Go）
    core  ←  backend/sdl2（实现契约，平台相关）

没有它时，后端要「实现引擎的接口」就得 import 引擎，依赖方向就成了 backend → engine：平台层反过来依赖它上面的层。Go 的接口是结构化的
（只要方法集一致就满足），所以把接口与跨层用到的值类型搬到 core， 引擎用类型别名（type Color = core.Color）保持对外 API 不变，
后端则只 import core——两边实现的是同一个类型，接口照样满足。

这里只放**跨层契约**：接口，以及接口签名里用到的值类型。引擎内部的 类型（文档、DOM、样式）与平台内部的类型（纹理、窗口句柄）都不在此。

TYPES

type Clipper interface {
	PushClip(x, y, w, h int)
	PopClip()
}
    Clipper 是 Graphics 的可选扩展：矩形裁剪栈。overflow 非 visible 的 元素绘制其内容前 PushClip、画完
    PopClip，超出裁剪区的像素不落笔。 实现方可自行选择不支持（不实现本接口时渲染层退化为不裁剪，内容 可能画出盒外——README
    已知限制）。栈语义：每层与上一层求交集。

type Color interface {
	RGBA() (r, g, b, a uint8)
}
    Color 表示 RGBA 颜色。

func NewColor(r, g, b, a uint8) Color
    NewColor 造一个 RGBA 颜色。

type Graphics interface {
	DrawText(x, y, w, h int, paint Paint, text string)
	DrawColor(x, y, w, h int, color Color)
	DrawImage(x, y, w, h int, image Image)
	// MeasureText 返回文本在给定字号/粗体/字族下的像素宽高。
	// family 传 CSS font-family 原值（可含逗号列表）；度量必须与 DrawText
	// 使用同一字体解析规则，否则混排宽度会错位。
	MeasureText(text string, fontSize int, bold bool, family string) (w, h int)
	// FontMetrics 返回文本在给定字号/粗体/字族下的**行度量**：ascent 为
	// 基线以上的像素高度，descent 为基线以下的像素高度，二者之和即行盒
	// 高度。口径对齐浏览器 line-height:normal（Chrome 取字体 hhea 的
	// ascender+descender+lineGap），行盒按基线对齐各内联盒。
	// text 为空时返回主字体的度量（不含逐字回退）——用于行盒的 strut
	// （块级自身的字体）。
	FontMetrics(text string, fontSize int, bold bool, family string) (ascent, descent int)
}
    Graphics 是渲染后端的抽象。布局引擎依赖 MeasureText 获取文本度量， 渲染阶段通过
    DrawText/DrawColor/DrawImage 落笔。

type Image interface {
}
    Image 是绘制目标里的一张位图。

type Paint interface {
	Size() Size
	SetSize(size Size)
	Color() Color
	SetColor(color Color)
	FontFamily() string
	SetFontFamily(family string)
	Bold() bool
	SetBold(bold bool)
	Background() Color
	SetBackground(color Color)
}
    Paint 是一次绘制动作的属性集合（类比 Android Paint / Skia Paint）。 Background
    为文字落点的背景色（沿父链最近实底，可能 nil）：供 LCD 子像素合成使用——子像素渲染必须知道底色才能合成出正确过渡色。

func NewPaint() Paint
    NewPaint 造一份默认绘制属性（16px 常规字重、黑色）。

type Size interface {
	Type() SizeType
	Pixel() int
	SetPixel(v int)
	Point() int
	SetPoint(v int)
	Displayport() int
	SetDisplayport(v int)
	Percent() float32
	SetPercent(v float32)
	Disc(int) int
}
    Size 表示一个带单位的尺寸值。

func NewAutoSize() Size
    NewAutoSize 表示 CSS 中的 auto。

func NewSize(tp SizeType, intVal int, float32Val float32) Size
    NewSize 造一个带单位的尺寸。

func NewZeroSize() Size
    NewZeroSize 造一个 0 像素的尺寸。

type SizeType uint8
    SizeType 是带单位尺寸的种类。

const (
	SIZE_PIXEL SizeType = iota
	SIZE_POINT
	SIZE_DISPLAYPORT
	SIZE_PERCENT
	SIZE_AUTO
)
    尺寸单位。

type Sprite struct {
	W, H int
	Pix  []byte
}
    Sprite 是一块可 blit 的位图（RGBA8888，每像素 4 字节，行主序）。

    目前只用于圆角抗锯齿的“角标精灵”：整块圆角一次 blit，代替逐像素的 半透明小方块。逐像素画圆角边缘的绘制调用极多——实测 portal
    页一帧 18524 次 1×1 绘制，占全部绘制调用的 3/4；GPU 路径上每次都是驱动 调用，帧时间被拖到几十毫秒，120ms
    的滚动动画被切成两三帧，看起来就是 “一卡一卡、还会闪”。整角一次 blit 把这部分降到 4 次/盒。

    alpha 为 0 的像素表示“不着色”（blit 时不覆盖底色），与逐像素路径里 scaledAlpha 返回 ok=false 的跳过语义一致。

type SpriteDrawer interface {
	DrawSprite(x, y int, s *Sprite)
}
    SpriteDrawer 是 Graphics 的可选扩展：支持位图 blit 的后端实现它。 不实现的后端回落到逐像素绘制（结果一致，只是慢）。

type Viewport interface {
	ViewportWidth() int
	ViewportHeight() int
	Graphics() Graphics
}
    Viewport 是渲染输出目标的抽象：一个有尺寸、可绘图的窗口/画布。 具体后端（如 SDL2）实现该接口；文档通过 OpenDocument
    绑定到视口。

````

---

# package engine
````
package engine // import "github.com/go-comm/ghromex/engine"

Package engine 是 Ghromex 的渲染引擎：纯 Go、不依赖任何图形后端， 把 HTML/CSS 变成一串绘制指令交给后端执行。

# 分层

    解析   html_parser.go / css_parser.go / css_url.go / selector.go
    级联   style_cascade.go（选择器匹配 + 特异度 + 继承）+ style_ua.go（UA 默认样式）
           pseudo.go（伪类状态：hover/focus/active 与结构伪类）
    布局   measure.go（盒模型/行盒/换行/定位）+ line_box.go（line-height 的
           Leading 分配与 vertical-align 的基线位移）+ margin_collapse.go +
           table_layout.go
    滚动   scroll.go（overflow 容器 + 文档级滚动 + wheel 分发）
    绘制   render.go（背景/边框/文本/装饰/层序/裁剪栈）+ form_render.go（表单控件）
    交互   event_dispatch.go（命中测试 + 冒泡派发）+ text_input.go / selection.go /
           select.go / textarea.go / anchor.go / check.go
    输出   graphics.go（Graphics 接口 + Fake/Buffer 实现）+ svg_dump.go（SVG 导出）

# 改代码前必读的不变量

 1. 纯 Go 可移植：engine 不得 import 平台包或图形库。窗口、剪贴板、 字体文件路径等平台能力由 backend/sdl2
    提供（browser 是门面）。
 2. 1 CSS px = 1 物理像素：布局全程整数像素，不做小数舍入（与浏览器的 亚像素差异是已知取舍，见根 README「已知限制」）。
 3. 布局流水线顺序固定（见 windows.go 的 LayoutDocument）： resolveStylesTree
    → layoutBox → layoutPositioned → applyScrollOffsets →
    layoutSelectPopups。新增定位/滚动/浮层特性必须挂到正确的遍次， 漏挂会表现为「改了不生效」或「重排死循环」。
 4. 命中测试与绘制必须同口径：裁剪（overflow）、定位层、select 浮层在 ElementAt 与 renderNode
    两侧都要一致，否则会出现「看得见点不到」。
 5. 变更通知是唯一的重排触发器：DOM 改动（属性/文本/焦点/选区/视口 尺寸）都要走 markChanged，后端按 ChangeCount
    决定重排；**布局期不得 触发 markChanged**（会形成重排↔变更死循环，见 style_cascade.go 的
    syncSelectValue）。
 6. 行盒只有一处口径：行盒高 = 各内联项**贡献**的最大 ascent + 最大 descent，基线在距行顶 lineAsc
    处。贡献 = 字体度量 ± line-height 的 Leading（line_box.go 的 applyLeading）±
    vertical-align 的位移 （line_box.go 的 verticalAlignShift）。绘制、命中测试、SVG 导出都
    从同一份文本片段与盒几何取数——三者任一改口径都会出现「画出来的 和点到的不是同一个」。

# 事件模型

命中测试取最深元素，沿父链依次派发 click / change / wheel，随后执行 引擎默认动作（select
展开选中、勾选切换、链接导航）。焦点与光标先于 click 处理器更新——处理器里读到的已是点击后的状态。

按键：keydown 派发给聚焦元素（元素上用 OnKeyDown 注册），处理器 返回 true
表示已消费，引擎的默认动作（退格/删除/回车换行/方向键移动光标） 不再执行——应用层靠它接"回车 = 打开"这类行为，不用改引擎。

可编辑输入（input text/password、textarea）共享一套编辑状态：光标 + 选区锚点，单位是 value 的 rune
下标。失焦只停画、**不丢状态**（重新 聚焦会恢复光标与选区），页内菜单（复制/剪切/粘贴）据此作用于编辑区。

# 伪类状态

:hover / :focus / :active 是**动态状态**，存在文档上（pseudo.go）： 状态一变，样式就得重算，故走「变更通知 → 重排重绘
→ 级联重算」这条 与 DOM 变更同一条链路（markChanged）。悬停/按下/焦点都只在**变化时** 通知——鼠标移动是最频繁的事件。

应用层若自建主循环（非 browser 门面），需自己把鼠标事件喂进来：
OnDocumentMouseMove（更新悬停）、OnDocumentMouseDown/Up（按下态）、
SetFocusedElement（焦点）。browser 门面已自动接好。

# 外部样式表

<link rel="stylesheet" href> 与 CSS 的 @import（可嵌套）都在**解析阶段**
同步加载，规则并入作者样式表并按文档顺序参与级联；@import 的规则排在 导入方之前（同特异度时导入方胜出，与浏览器一致）。

引擎不碰 I/O：加载器由宿主经 DocumentOptions.LoadStylesheet 提供。相对 引用由引擎解析成绝对地址——<link>
相对 BaseURL，@import 相对**上一层 样式表**的地址（css_url.go），所以加载器拿到的地址可以直接用。加载失败
只跳过那一份样式表（页面照常打开）；@import 的循环引用在链上截断 （深度上限 maxStylesheetDepth）。

加载发生在解析期，所以加载器只能在 OpenDocumentWith 时传入；文档建好 之后再 SetStylesheetLoader
不会补加载页面里已有的引用（只影响此后新增 的引用）。加载器跑在打开文档的 goroutine 上——要联网的宿主应自己缓存或
预取，引擎没有「样式到货后再重排」的链路。

# 后端可选扩展

Graphics 接口之外有两个**可选扩展**（后端实现了就用，没实现就走等价 但更慢的回退路径，画面结果一致）：

  - Clipper：PushClip/PopClip 的裁剪栈（裁剪矩形逐层求交下推）；
  - SpriteDrawer：DrawSprite 位图精灵（圆角抗锯齿的"角标"——整块圆角 一次 blit，替掉逐像素的半透明小方块）。绘制调用数是
    GPU 路径上的 主要开销：逐像素画圆角边缘时 portal 页一帧要发 3.2 万次调用，改用 角标后降到 2500 次，单帧绘制 149ms →
    8.7ms（软件渲染器实测）， 滚动动画才不会被切成几帧（一卡一卡还闪）。

# 测试

engine 的测试全是纯 Go，不需要显示器：

  - FakeGraphics：只计数，断言绘制指令的调用序列；
  - BufferGraphics：软件光栅化到 RGBA，可 DumpPNG 与 ColorAt 逐像素断言； NewBufferedViewport /
    OpenBufferedDocument 一把建好「视口 + 缓冲」， 免得视口与缓冲的尺寸写岔；
  - HeadlessViewport：无头视口，配合 DumpSVG 断言布局几何；
  - 事件用 engine.OnDocumentClick / OnDocumentKeyDown / OnDocumentPaste
    等公开入口驱动，不要直接调内部函数——白盒绕过命中与焦点链路会漏掉 真实 bug。

CONSTANTS

const (
	SIZE_PIXEL       = core.SIZE_PIXEL
	SIZE_POINT       = core.SIZE_POINT
	SIZE_DISPLAYPORT = core.SIZE_DISPLAYPORT
	SIZE_PERCENT     = core.SIZE_PERCENT
	SIZE_AUTO        = core.SIZE_AUTO
)
    尺寸单位（与 core 同名常量，类型一致）。

const (
	EventClick   = "click"
	EventChange  = "change"
	EventWheel   = "wheel"
	EventKeyDown = "keydown"
)
    事件类型常量

const (
	KeyBackspace = 0x08
	KeyTab       = 0x09
	KeyEnter     = 0x0D
	KeyEscape    = 0x1B
	KeyHome      = 0x4000004A // SDL_SCANCODE_HOME  (74)
	KeyDelete    = 0x7F       // SDLK_DELETE（SDL_keycode.h 直接定义为 ASCII DEL，不是扫描码区！）
	KeyEnd       = 0x4000004D // SDL_SCANCODE_END   (77)
	KeyRight     = 0x4000004F // SDL_SCANCODE_RIGHT (79)
	KeyLeft      = 0x40000050 // SDL_SCANCODE_LEFT  (80)
	KeyDown      = 0x40000051 // SDL_SCANCODE_DOWN  (81)
	KeyUp        = 0x40000052 // SDL_SCANCODE_UP    (82)
)
    输入编辑用到的控制键码。ASCII 区取值对齐 SDL2 SDL_keysym.sym （SDL_keyboard.h 的 SDLK_* 定义），SDL
    后端可直接透传； 方向/Home/End 属扫描码区：SDLK = 0x40000000 | SDL_SCANCODE_*； Delete
    是例外：SDL_keycode.h 直接把它定义为 ASCII DEL(0x7F)。

const (
	ModShift = 0x0003
	ModCtrl  = 0x00C0
	ModAlt   = 0x0300
	ModGUI   = 0x0C00
)
    修饰键位掩码，取值对齐 SDL2 SDL_Keymod（SDL_keycode.h）： KMOD_LSHIFT/RESHIFT =
    0x1/0x2、LCTRL/RCTRL = 0x40/0x80、 LALT/RALT = 0x100/0x200、LGUIM/RGUIM =
    0x400/0x800。 后端把 KeyDown 的 keysym.mod 原样传入，引擎据此识别 Ctrl+A / Ctrl+C。


FUNCTIONS

func BaselineAdjust() int
    BaselineAdjust 返回当前全局文本基线补偿（像素）。

func CaretPosition(el HTMLElement) (line, col int, ok bool)
    CaretPosition 返回可编辑控件的光标位置（行、列，均从 1 起）。 textarea 按折行结果分行（软折行也算新行），单行 input
    恒为第 1 行。 返回 false 表示该元素不是可编辑控件（或没有光标状态）。

    失焦后仍返回最后的光标位置（编辑状态不随失焦丢弃，见 SetFocusedElement），因此状态栏之类的 UI 可持续显示光标位置。

func DefineCustomElement(tag string, prototype HTMLElement) error
    DefineCustomElement 注册自定义标签原型（类比 Web Components 的 customElements.define）。
    原型须由 newElement(tag) 起步构建；重复 Define 同一标签会直接返回错误而不覆盖。

func GetValue(el HTMLElement) string
    GetValue 返回控件当前值：
      - select：选中项的值（渲染同口径）；
      - input/textarea：value 属性；
      - 其他元素：value 属性（无则空串）。

func IsChecked(el HTMLElement) bool
    IsChecked 返回 radio/checkbox 的当前选中状态；非勾选控件恒为 false。

func LayoutDocument(doc HTMLDocument)
    LayoutDocument 对文档执行一次完整的级联 + 布局。

func MeasureDocumentWithStylesheet(g Graphics, doc HTMLDocument, mw, mh int)
    MeasureDocumentWithStylesheet 完整流水线入口：级联 + 布局，不依赖 Viewport。

func MeasureNode(g Graphics, node HTMLElement, mw, mh int)
    MeasureNode 布局入口：对根节点在 (mw, mh) 可用空间内执行一次布局。 若样式尚未级联（computed 为空），先以空样式表级联。

func NewBufferedViewport(w, h int) (*HeadlessViewport, *BufferGraphics)
    NewBufferedViewport 创建无显示视口 + 软件光栅化后端（BufferGraphics）： 视口与缓冲同尺寸，绘制指令落进缓冲区，可
    ColorAt 逐像素断言、DumpPNG 出图。

    与 NewHeadlessViewport 只差绘制后端：两者的文本度量同口径 （BufferGraphics 的度量与 FakeGraphics
    相同），换后端不改布局结果。

func OnDocumentClick(doc HTMLDocument, x, y int) bool
    OnDocumentClick 在文档上派发一次点击事件：从最深命中元素向上冒泡， 依次调用各元素注册的 click 处理器；随后执行引擎默认动作
    （select 展开/收起/选中、radio/checkbox 勾选切换、<a> 导航）， value/勾选状态变化补发
    change。返回是否有处理器被触发。

func OnDocumentKeyDown(doc HTMLDocument, key int) bool
    OnDocumentKeyDown 向聚焦输入派发一个控制键（key 为 SDL keysym 值）。
    消费：Backspace、Delete、Enter（仅 textarea 换行）、方向键、Home/End、Tab。
    这是无修饰键入口（保持既有调用方兼容）；带 Shift/Ctrl 的按键走 OnDocumentKeyDownMods。

func OnDocumentKeyDownMods(doc HTMLDocument, key int, mods uint16) bool
    OnDocumentKeyDownMods 处理带修饰键的按键：Ctrl+A 全选、Ctrl+C 表示
    「有选区可复制」（真正的剪贴板写入由后端完成——引擎不依赖 SDL）、 Shift+方向键/Home/End 扩展选区。其余按键行为与
    OnDocumentKeyDown 相同。

func OnDocumentMouseDown(doc HTMLDocument, x, y int) bool
    OnDocumentMouseDown 在鼠标按下时准备潜在拖选：命中可编辑输入则聚焦、
    按按下点落光标，并把锚点定在同一位置。返回是否落在可编辑输入上—— 后端据此决定后续 MouseMotion 是否扩展选区，以及抬起时是否跳过
    click （拖选不是点击，不应触发链接/按钮的默认动作）。

    真正的选区在 MouseMotion 中才展开：按下即产生选区会让"单击定位光标" 也走选区路径，和浏览器行为不一致。

func OnDocumentMouseDrag(doc HTMLDocument, x, y int) bool
    OnDocumentMouseDrag 把拖选活动端移到指针位置，锚点保持不变。指针移出
    控件后仍按原控件布局反推光标（并自然夹到行首/行尾/首末行），因此 可以拖到输入框外继续扩选。按住 textarea 覆盖滚动条时改为滚动。

func OnDocumentMouseMove(doc HTMLDocument, x, y int) bool
    OnDocumentMouseMove 按坐标更新悬停态：命中测试后把命中的元素记为 悬停元素（命中空白处则清除）。返回是否命中了元素。

    应用层在每次鼠标移动时调用它，:hover 样式因此跟手。

func OnDocumentMouseUp(doc HTMLDocument)
    OnDocumentMouseUp 结束拖选。选区本身保留（浏览器同语义：松开鼠标后 高亮仍在，直到再次点击或按键）。

    同时清掉按下态（:active）——鼠标已抬起。

func OnDocumentPaste(doc HTMLDocument, text string) bool
    OnDocumentPaste 向聚焦输入粘贴文本（Ctrl+V 的引擎侧入口）。

    剪贴板读取属平台能力，由后端（SDL2）读出后调用本函数——引擎不碰 系统剪贴板，保持纯 Go、无显示环境可测（与 Ctrl+C 的分工一致：
    引擎给文本，后端写/读剪贴板）。

    语义对齐浏览器：
      - 在光标处插入；若有选区则先删除选区（粘贴替换选区）；
      - 单行 input 粘贴多行文本时折成单行（换行折成空格，连续换行只留 一个空格）；
      - textarea 保留换行（多行粘贴即多行文本）。

    返回是否被消费（无焦点 / 无可粘贴内容时不消费）。

func OnDocumentTextInput(doc HTMLDocument, s string) bool
    OnDocumentTextInput 向聚焦输入插入一段可打印文本（1 个或多个字符； 中文等 IME 组合完成的字符串走 SDL_TEXTINPUT
    到这里）。插入点为当前 光标位置（默认值末尾），随后光标后移；若有选区则先删除选区再插入 （浏览器同语义）。返回是否被消费。

    控制字符（<0x20 与 0x7F）被丢弃：Windows 会把 Ctrl+C / Ctrl+V 这类 组合键以控制码（0x03 /
    0x16）的形式送进文本输入通道，不过滤的话 剪贴板操作会在输入框里留下一个看不见的控制字符。

func OnDocumentWheel(doc HTMLDocument, x, y, dx, dy int) bool
    OnDocumentWheel 在文档上派发一次滚轮事件：坐标为视口坐标， dx/dy 为像素位移（dy > 0 =
    向下滚，内容上移露出更下方内容，与浏览器 WheelEvent.deltaY 同号）。返回是否有元素实际滚动（含 wheel 处理器被 触发时的
    handled 由调用方另计——这里只回报滚动是否发生）。

    流程：先沿祖先链冒泡 wheel 处理器（先于滚动，读到的是滚动前状态）， 再从命中元素向上找第一个能消费位移的滚动目标（textarea /
    overflow 容器 / 文档），某轴滚到底继续尝试其祖先，最终落到根元素。

func OpenBufferedDocument(w, h int, src string) (*BufferGraphics, HTMLDocument, error)
    OpenBufferedDocument 在软件光栅化后端上打开文档，并完成首次级联、布局 与绘制，返回缓冲区与文档，供逐像素断言。

    它把「建视口 → OpenDocument → RenderNode」三步收成一步：视口与缓冲的尺寸
    由同一处给定，不会写岔（尺寸不一致时，像素断言会读到空白处）。

func ParseHTMLInto(doc HTMLDocument, src string)
    ParseHTMLInto 将 HTML 源码解析进既有文档： <head>/<body> 内容路由到文档结构节点，<style> 编译进文档样式表，
    <link rel="stylesheet" href> 交给文档的样式表加载器（见 DocumentOptions）， <title> 写入 head
    的 title 元素，<script>/<meta> 与其余 <link> 被忽略。

func RenderNode(g Graphics, node HTMLElement)
    RenderNode 从根节点开始绘制整棵树。g 为渲染后端。

    层叠模型（对齐浏览器主案例）：常规流内容按文档序先绘；定位元素盖在 静态内容之上，按 (z-index，文档序) 排序。z-index 非 0
    的定位元素与 fixed 元素会建立层叠上下文，其内部定位子树在该上下文内单独排序后再 整体落层，避免「祖先背景盖住自己 z:auto/absolute
    子元素」。 定位子树由本 pass 绘制：主 pass 递归遇到 positioned 子节点直接跳过。 未实现 opacity/transform
    成组、负 z 压到背景下等更细规则，需要时再扩展。

    视口裁剪：绘制时跳过完全落在视口外的盒子（滚动时页面下半部分不再白 画一遍）。这是 Chrome「只合成可见瓦片」的等价物——滚动动画每帧都要
    重画整页，不裁剪的话一帧上千次绘制调用，帧时间顶到 vsync 之外就掉帧。

func ScrollAnimating(doc HTMLDocument) bool
    ScrollAnimating 报告文档里是否有滚动动画在途（调用方据此不休眠， 否则动画会被 10ms 的空闲休眠切成一段一段的台阶）。

func SelectValue(sel HTMLElement) string
    SelectValue 返回 select 当前选中项的值（渲染同口径）。

func SelectedText(doc HTMLDocument) (string, bool)
    SelectedText 返回聚焦输入当前选中的文本。第二个返回值为 false 表示
    没有选区（或没有聚焦输入），后端据此决定是否写剪贴板——空选区不应 清空用户原有剪贴板。

    password 返回真实 value（浏览器允许选中并复制密码框内容），显示层 的 ● 掩码只在绘制/SVG 导出时生效。

func SetActiveElement(doc HTMLDocument, el HTMLElement)
    SetActiveElement 设置按下态元素（nil 表示没有按下）。 状态变化触发变更通知（:active 规则要改样式）。

func SetBaselineAdjust(px int) int
    SetBaselineAdjust 设置全局文本基线补偿（像素，正数=下移），返回设置前的 值。用于与 Chrome 对照时的微调；0 =
    不补偿（默认）。

func SetChecked(el HTMLElement, on bool)
    SetChecked 设置勾选状态并触发重排重绘：radio 置位时同组其余项自动取消， 关闭 radio 只改自身。程序化赋值不派发
    change（与浏览器一致）。

func SetFocusedElement(doc HTMLDocument, el HTMLElement)
    SetFocusedElement 设置聚焦元素；不可聚焦的元素等效清除（nil）。 首次聚焦某控件时光标移到值末尾（与浏览器点选落位一致；精确落点由
    点击坐标修正）；再次聚焦保留该控件原有的光标与选区。 焦点变化触发文档变更通知——聚焦框/光标重绘与文本变化走同一条
    changed→重排重绘链路；:focus 样式也因此更新。

    失焦不丢弃编辑状态：选区与光标原样留在控件上，只是停止绘制（渲染 路径按 focused 决定是否画选区）。页内菜单（记事本的「复制/剪切/
    粘贴」）点击时编辑区会先失焦，若失焦即清空选区，菜单动作就无选中 可用；保留状态后，应用层把焦点交还给编辑区即可继续操作。

func SetHoveredElement(doc HTMLDocument, el HTMLElement)
    SetHoveredElement 设置悬停元素（nil 表示鼠标不在文档上）。

    悬停态变化会改样式（:hover 规则），故触发变更通知 → 重排重绘。 悬停元素**没变**时不通知：鼠标移动是最频繁的事件，每次都重排会把
    主循环拖垮（浏览器同样只在悬停目标变化时重算样式）。

func SetSelectValue(sel HTMLElement, value string) bool
    SetSelectValue 按值选中选项并触发变更通知；未找到匹配项返回 false。 程序化赋值不改变展开态（与浏览器一致）。

func SetTooltipColors(doc HTMLDocument, c TooltipColors)
    SetTooltipColors 设置文档的提示气泡配色，让气泡跟随应用主题而不是引擎 默认值。空字段保持默认。

    纯绘制层状态、不触发变更通知：调用方自己保证会有一次重绘（换主题通常 伴随样式表追加，重排重绘自然发生）。

func SetValue(el HTMLElement, value string)
    SetValue 设置控件当前值并触发重排重绘：
      - select：按值选中对应 option（未匹配则忽略）；
      - input/textarea：写 value 属性，光标移到值末尾。

func TickScroll(doc HTMLDocument) bool
    TickScroll 推进文档级滚动动画一帧，并把视觉位置的增量平移到已布局的 盒子上。返回是否有内容移动（调用方需要重绘，但**不必重排**）。

    动画按真实时间推进（见 scrollAnimDuration），故帧率波动不会拖慢滚动
    速度；每帧只做一次平移，代价与页面元素数线性相关、与样式表规模无关。

    引擎不自带帧循环：后端每帧调一次本函数即可（backend/sdl2 的事件循环 已接上）。不调则动画停在起点（滚动目标仍然生效，只是不做补间）。

func TickTooltip(doc HTMLDocument) bool
    TickTooltip 推进提示气泡的计时，后端每帧调一次（browser 主循环已接上， 与 TickScroll
    并排）。返回是否需要重绘（气泡弹出或收起——纯绘制层变化， 不必重排）。

    可见性变化全部集中在这里：目标或文案变化就重新计时（并把旧气泡收掉）； 目标没变但曾被按下/点击/滚动收掉过，就不追回，直到鼠标换目标；弹出后
    原地不动，等目标变化。

func TooltipPending(doc HTMLDocument) bool
    TooltipPending 报告当前悬停是否指向一个还没弹出的气泡（正悬停在带提示的
    元素上、未到弹出点、也没被按下/滚动收掉）。调用方据此跳过空闲休眠——与 ScrollAnimating 同理：休眠会让气泡比延时晚一拍才弹出。

    按 hover 实时解析而不是读上一帧登记的状态：鼠标刚进入带提示的元素就该 不休眠，登记发生在下一次 TickTooltip，读状态会晚一帧。

func TooltipText(doc HTMLDocument) string
    TooltipText 返回当前弹出中的气泡文案（未弹出时为空串）。

func CustomElements() *elementRegistry
func SystemElements() *elementRegistry
func ParseInlineStyle(text string) []simpleDecl
    ParseInlineStyle 解析 style="..." 属性值。


TYPES

type BorderCollapse uint8
    BorderCollapse 决定表格相邻单元格的边框是否合并（border-collapse）， 只被表格布局读取。separate 为
    CSS 默认（单元格各自画四边边框，相邻两条各 自 1px）；collapse 时相邻单元格共享一条线——浏览器把它实现成每个单元格只
    摊到"半个边框"（实测 Chrome：`border: 1px` + `padding: 4px 8px` 的单元格 外盒 = 内容 + 16 +
    1），外缘的半个边框归表格自己。

const (
	BorderCollapseSeparate BorderCollapse = iota // 默认
	BorderCollapseCollapse
)
type BorderSide uint8
    BorderSide 是边框四边的下标。顺序与渲染内部的 eTop/eRight/eBottom/eLeft
    一致：这样样式层、解析层与绘制层可以共用一份 [4] 数组，不必在中间反复 做名字到索引的映射。

const (
	BorderSideTop BorderSide = iota
	BorderSideRight
	BorderSideBottom
	BorderSideLeft
)
type BorderStyle uint8

const (
	BorderStyleNone BorderStyle = iota
	BorderStyleSolid
)
type BoxSizing uint8

const (
	BoxSizingContentBox BoxSizing = iota // width/height 仅为内容盒（CSS 默认）
	BoxSizingBorderBox                   // width/height 含 padding+border（不含 margin）
)
type BufferGraphics struct {
	W, H int

	DrawCalls int
	TextCalls int
	Texts     []string // 按绘制顺序记录的文本内容
	// Has unexported fields.
}
    BufferGraphics 把绘制指令落在一块二维 RGBA 缓冲区上： DrawColor 填充矩形、DrawText
    把每个非空白字符画成一个色块（近似字形占位）、 DrawImage 填充品红占位块。它不依赖任何图形后端，适合单元测试断言像素， 也能 DumpPNG
    出图供人工查看。缓冲区初始为全透明。

    混合规则做了简化：半透明填充按 src*a + dst*(255-a) 直通混合（不预乘）， 仅用于测试与可视化，不追求色彩学精确。

func NewBufferGraphics(w, h int) *BufferGraphics

func (g *BufferGraphics) ColorAt(x, y int) (r, gg, b, a uint8, ok bool)
    ColorAt 读取像素点，越界时 ok=false。

func (g *BufferGraphics) DrawColor(x, y, w, h int, color Color)
    DrawColor 实现 Graphics：颜色填充矩形。

func (g *BufferGraphics) DrawImage(x, y, w, h int, image Image)
    DrawImage 实现 Graphics：记录调用并填充品红占位块。

func (g *BufferGraphics) DrawSprite(x, y int, s *Sprite)
    DrawSprite 实现 engine.SpriteDrawer：按 RGBA8888 逐像素混合精灵 （alpha 0
    的像素跳过，与逐像素路径的跳过语义一致）。

func (g *BufferGraphics) DrawText(x, y, w, h int, paint Paint, text string)
    DrawText 实现 Graphics：整段空白只记录不绘制；其余每个字符画一个内缩 色块作为字形近似，保证"该有字的地方有颜色可断言"。

func (g *BufferGraphics) DumpPNG(path string) error
    DumpPNG 把缓冲区当前内容编码为 PNG 文件落盘。

    这是「仅缓冲快照」：不布局、不绘制，缓冲里画过什么就存什么——没渲染过 是空图，没重排就渲染则是旧帧。要出「应用一帧」得先走完整流程（Refresh
    重排 + RenderNode 光栅化，示例的 -dump-png 即此），本方法只是最后一步。

func (g *BufferGraphics) FillRect(x, y, w, h int, r, gg, b, a uint8)
    FillRect 填充一个整数矩形（自动裁剪到缓冲区与当前裁剪栈）。

func (g *BufferGraphics) FontMetrics(text string, fontSize int, bold bool, family string) (int, int)
    FontMetrics 实现 Graphics：与 FakeGraphics 相同的估算口径。

func (g *BufferGraphics) MeasureText(text string, fontSize int, bold bool, family string) (w, h int)
    MeasureText 实现 Graphics：与 FakeGraphics 相同的粗略估算， 保证布局结果可与断言表对应。

func (g *BufferGraphics) PopClip()
    PopClip 实现 Clipper：弹出栈顶；栈空时为无害的多余调用。

func (g *BufferGraphics) PushClip(x, y, w, h int)
    PushClip 实现 Clipper：压入一个与当前栈顶求交集的裁剪矩形。

type CSSStyleDeclaration interface {
	Display() Display
	SetDisplay(display Display) CSSStyleDeclaration
	Width() Size
	SetWidth(w Size) CSSStyleDeclaration
	Height() Size
	SetHeight(h Size) CSSStyleDeclaration
	BackgroundColor() Color
	SetBackgroundColor(color Color) CSSStyleDeclaration
	BackgroundImage() Image
	SetBackgroundImage(image Image) CSSStyleDeclaration
	Padding() Rect
	SetPadding(r Rect) CSSStyleDeclaration
	Margin() Rect
	SetMargin(r Rect) CSSStyleDeclaration
	BorderStyleWidth() Rect
	SetBorderStyleWidth(r Rect) CSSStyleDeclaration
	// BorderColor/SetBorderColor 是四边同色的兼容入口（读顶边、写四边）；
	// 单边颜色用 BorderColorFor/SetBorderColorFor。
	BorderColor() Color
	SetBorderColor(color Color) CSSStyleDeclaration
	BorderColorFor(side BorderSide) Color
	SetBorderColorFor(side BorderSide, color Color) CSSStyleDeclaration
	BorderImage() Image
	SetBorderImage(image Image) CSSStyleDeclaration
	// BorderStyle/SetBorderStyle 同样保留「读顶边、写四边」的旧语义。
	BorderStyle() BorderStyle
	SetBorderStyle(style BorderStyle) CSSStyleDeclaration
	BorderStyleFor(side BorderSide) BorderStyle
	SetBorderStyleFor(side BorderSide, style BorderStyle) CSSStyleDeclaration
	// BoxShadow 是 box-shadow 声明（逗号分隔的多条阴影），默认 nil。
	BoxShadow() []Shadow
	SetBoxShadow(shadows []Shadow) CSSStyleDeclaration
	Color() Color
	SetColor(color Color) CSSStyleDeclaration
	FontSize() Size
	SetFontSize(size Size) CSSStyleDeclaration
	FontFamily() string
	SetFontFamily(family string) CSSStyleDeclaration
	FontWeight() FontWeight
	SetFontWeight(weight FontWeight) CSSStyleDeclaration
	TextAlign() TextAlign
	SetTextAlign(align TextAlign) CSSStyleDeclaration
	BorderRadius() Size
	SetBorderRadius(radius Size) CSSStyleDeclaration
	Position() Position
	SetPosition(pos Position) CSSStyleDeclaration
	// Inset 是 top/right/bottom/left 四边偏移（默认全 auto）：
	// relative 为偏移量，absolute/fixed 为相对包含块的锚距。
	Inset() Rect
	SetInset(r Rect) CSSStyleDeclaration
	ZIndex() int
	SetZIndex(z int) CSSStyleDeclaration
	// BoxSizing 决定 width/height 是否含 padding+border。Chrome UA 样式表将
	// 表单控件默认置为 border-box（见 style_ua.go），引擎按此对齐。
	BoxSizing() BoxSizing
	SetBoxSizing(b BoxSizing) CSSStyleDeclaration
	// TextDecoration 为文本装饰（underline/line-through/none），随父链继承。
	TextDecoration() TextDecoration
	SetTextDecoration(d TextDecoration) CSSStyleDeclaration
	// OverflowX/OverflowY 为水平/纵向溢出处理（overflow-x / overflow-y，
	// 简写 overflow 两值同写）。默认 visible。
	OverflowX() Overflow
	SetOverflowX(o Overflow) CSSStyleDeclaration
	OverflowY() Overflow
	SetOverflowY(o Overflow) CSSStyleDeclaration
	// WhiteSpace 决定空白处理（normal/pre），沿父链继承。
	WhiteSpace() WhiteSpace
	SetWhiteSpace(w WhiteSpace) CSSStyleDeclaration
	// PointerEvents 决定元素是否参与命中测试（auto/none），沿父链继承；
	// 只有命中测试读它，渲染与布局不受影响。
	PointerEvents() PointerEvents
	SetPointerEvents(p PointerEvents) CSSStyleDeclaration
	// BorderCollapse 决定表格相邻单元格边框是否合并（border-collapse），
	// 不继承、仅表格布局读取。默认 separate。
	BorderCollapse() BorderCollapse
	SetBorderCollapse(b BorderCollapse) CSSStyleDeclaration
	// LineHeight 是 line-height 的值（normal/倍数/绝对长度），沿父链继承
	// 的是**原始形态**（倍数相对子元素自己的字号解析）。
	LineHeight() LineHeight
	SetLineHeight(lh LineHeight) CSSStyleDeclaration
	// VerticalAlign 是 vertical-align 的值（不继承），只作用于行内级盒。
	VerticalAlign() VerticalAlign
	SetVerticalAlign(v VerticalAlign) CSSStyleDeclaration
	// VerticalAlignOffset 是 vertical-align 为长度/百分比时的抬升值
	// （百分比相对元素自身的 line-height）。
	VerticalAlignOffset() Size
	SetVerticalAlignOffset(s Size) CSSStyleDeclaration
}
    CSSStyleDeclaration 是可级联应用的样式对象。 Width/Height 默认为 auto；盒模型四边默认 0px。

type Clipper = core.Clipper
    Clipper 是 Graphics 的可选扩展：矩形裁剪栈。

type Color = core.Color
    Color 表示 RGBA 颜色。

func CanvasBackground(doc HTMLDocument) Color
    CanvasBackground 实现 CSS 画布背景传播（canvas background propagation）：
    画布底色取根元素（html）的背景，根无背景时退到 body。返回 nil 表示两者 都没有（调用方用默认白底）。真实浏览器正是靠这条规则让 body
    的浅色背景 铺满整个窗口，而不是只铺到内容高度。

func NewColor(r, g, b, a uint8) Color
    NewColor 造一个 RGBA 颜色。

func ParseColor(v string) Color
    ParseColor 解析一个 CSS 颜色值：#rgb / #rgba / #rrggbb / #rrggbbaa / rgb() / rgba()
    / 常见颜色名；无法解析时返回 nil。

    给应用把「主题里的颜色字符串」变成引擎 Color 用——绘制层 API（如 SetTooltipColors）收的是 Color 而不是 CSS
    文本。

type Display uint8

const (
	DisplayNone Display = iota
	DisplayInline
	DisplayInlineBlock
	DisplayBlock
)
type DocumentOptions struct {
	// BaseURL 是文档地址：<link rel="stylesheet" href> 的相对引用按它解析，
	// 也是第一层 @import 的引用方。按 URL 口径用 `/` 分隔；本地文件先转成
	// file:// 地址（见 examples/browser 的 fileURL），空串表示没有基准
	// （此时相对引用原样交给加载器）。
	BaseURL string
	// LoadStylesheet 非 nil 时启用外部样式表：<link rel="stylesheet" href>
	// 与 CSS 的 @import 都会经它加载。
	LoadStylesheet StylesheetLoader
}
    DocumentOptions 是 OpenDocumentWith 的可选能力。零值等同于 OpenDocument： 不加载外部样式表。

type ELement interface {
	SetAttribute(key string, value string)
	// RemoveAttribute 删除属性；不存在时是空操作。
	//
	// 布尔属性（disabled/checked 等）按「属性是否存在」判定，启用控件时
	// 不能只把值改成空串，必须真正删掉属性。
	RemoveAttribute(key string)
	GetAttribute(key string) string
	ID() string
	SetID(id string)
}

type FakeGraphics struct {
	DrawCalls int
	TextCalls int
}
    FakeGraphics 供测试与无显卡环境使用的空后端。

func (g *FakeGraphics) DrawColor(x, y, w, h int, color Color)

func (g *FakeGraphics) DrawImage(x, y, w, h int, image Image)

func (g *FakeGraphics) DrawText(x, y, w, h int, paint Paint, s string)

func (g *FakeGraphics) FontMetrics(text string, fontSize int, bold bool, family string) (int, int)
    FontMetrics 实现 Graphics：给出估算的行度量（与 MeasureText 的估算
    行高同口径拆成基线上下两半），供布局按基线对齐行内盒。

func (g *FakeGraphics) MeasureText(text string, fontSize int, bold bool, family string) (w, h int)
    MeasureText 给出粗略估算：CJK 按全宽、拉丁按半宽，行高 1.3 倍字号。

func (g *FakeGraphics) PopClip()

func (g *FakeGraphics) PushClip(x, y, w, h int)
    PushClip/PopClip 实现 Clipper（空后端无需真裁剪，但保持接口一致， 让"后端是否支持裁剪"不因测试替身而改变）。

type FontWeight uint8

const (
	FontWeightNormal FontWeight = iota
	FontWeightBold
)
type Graphics = core.Graphics
    Graphics 是渲染后端的抽象。

func NewFakeGraphics() Graphics

type HTMLBodyElement interface {
	HTMLElement

	SetOnLoad(f func())
}

type HTMLCollection interface {
	Item(i int) HTMLElement
	NamedItem(name string) HTMLElement
	Length() int
}

type HTMLDivElement struct {
	HTMLElement
}

func (e *HTMLDivElement) AppendChild(child HTMLElement, es ...HTMLElement)

func (e *HTMLDivElement) Children() []HTMLElement

func (e *HTMLDivElement) Clone() HTMLElement

func (e *HTMLDivElement) ParentElement() HTMLElement

func (e *HTMLDivElement) RemoveChild(child HTMLElement, es ...HTMLElement)

type HTMLDocument interface {
	HTMLElement

	CreateElement(tag string) HTMLElement
	GetElementById(id string) HTMLElement
	GetElementsByName(name string) []HTMLElement

	Body() HTMLBodyElement
	Head() HTMLHeadElement

	// Stylesheet 返回文档 <style> 编译出的样式表。
	Stylesheet() *Stylesheet
	// AddStylesheetText 解析并合并一段 CSS（其中的 @import 会被加载）。
	AddStylesheetText(css string)
	// AddStylesheetLink 解析并合并 <link rel="stylesheet" href> 引用的样式表。
	// 解析器遇到样式表链接时调用；运行期想动态挂样式表也可直接调它。
	AddStylesheetLink(href string)
	SetStylesheet(sheet *Stylesheet)
	// SetStylesheetLoader 安装外部样式表加载器；nil 关闭外部样式表
	// （<link rel="stylesheet"> 与 @import 都被忽略，其余照常）。
	SetStylesheetLoader(f StylesheetLoader)
	// SetBaseURL 设置文档地址：<link> 与第一层 @import 的相对引用按它解析。
	// 嵌套 @import 的基准是上一层样式表的地址，由引擎自动接续。
	SetBaseURL(base string)

	// Title 返回 head > title 的文本。
	Title() string

	Viewport() Viewport
	// Refresh 基于当前视口重新执行级联与布局。
	Refresh()
	// DumpSVG 将当前布局导出为 SVG 文本（调试/无头验证用）。
	DumpSVG() string

	// SetOnChanged 注册文档内容变化回调（布局失效通知，供后端重排）。
	SetOnChanged(f func())
	// SetOnNavigate 注册链接导航回调：点击带 href 的 <a> 时以 href 调用。
	// 引擎不内置多文档/外跳，跳转策略由应用决定；f 为 nil 时移除。
	SetOnNavigate(f func(href string))
	// ChangeCount 返回内容变化计数，后端据此判断是否需要重排。
	ChangeCount() int64

	Close()
}
    HTMLDocument 是文档根节点，持有样式表与所属视口。

func NewDocument() HTMLDocument

func OpenDocument(v Viewport, src string) (HTMLDocument, error)
    OpenDocument 解析 HTML/CSS 源码并完成首次级联与布局。

func OpenDocumentWith(v Viewport, src string, opts DocumentOptions) (HTMLDocument, error)
    OpenDocumentWith 同 OpenDocument，额外传入文档选项（外部样式表加载器与 地址基准）。

    选项必须在这里给、而不是等文档建好再设置：<link rel="stylesheet"> 与 CSS 的 @import
    都在**解析阶段**就解析并加载，文档返回时引用已经处理完了 ——事后 SetStylesheetLoader
    只影响此后新增的链接，页面里原有的引用不会 补加载。

type HTMLElement interface {
	ELement

	TagName() string
	String() string

	Style() CSSStyleDeclaration
	SetStyle(style interface{})

	AppendChild(e HTMLElement, es ...HTMLElement)
	// InsertBefore 把 e 插到 before 之前（before 为 nil 或不是直接子节点时
	// 追加到末尾）。UI 外壳要在页面内容之前插占位块，没有它就只能在
	// 「摘掉全部子节点再按序接回」里打转。
	InsertBefore(e HTMLElement, before HTMLElement)
	RemoveChild(e HTMLElement, es ...HTMLElement)
	Children() []HTMLElement
	ParentElement() HTMLElement

	Clone() HTMLElement

	GetElementsByClassName(name string) []HTMLElement
	GetElementsByTagName(name string) []HTMLElement

	QuerySelector(query string) HTMLElement
	QuerySelectorAll(query string) []HTMLElement

	// GetBoundingClientRect 返回布局完成后的边框盒（不含 margin）。
	GetBoundingClientRect() Rect

	X() int

	Y() int

	Width() int

	Height() int

	Text() string
	SetText(text string)
	InnerText() string
	SetInnerText(text string)
	InnerHTML() string
	SetInnerHTML(html string)

	// OnClick 注册点击事件处理器，f 为 nil 时移除。
	OnClick(f func(ev *MouseEvent))
	// OnChange 注册值变更事件处理器（select 选中项变化等），f 为 nil 时移除。
	OnChange(f func(ev *MouseEvent))
	// OnWheel 注册滚轮事件处理器（先于滚动发生派发），f 为 nil 时移除。
	OnWheel(f func(ev *MouseEvent))
	// OnKeyDown 注册按键事件处理器，f 为 nil 时移除。处理器返回 true 表示
	// 已消费：引擎的默认动作不再执行（等价于网页的 preventDefault）。
	OnKeyDown(f func(ev *KeyEvent) bool)

	EncodeJSON(w io.StringWriter, prefix string, indent bool)
	FormatJSON(prefix string, indent bool) string
	// Has unexported methods.
}
    HTMLElement 是 DOM 元素节点的基础接口。 扩展新标签时嵌入
    HTMLElement，并用 newElement(tag) 初始化内部节点， 同时覆盖
    AppendChild/RemoveChild/Children/ParentElement/setParentElement
    以保证包装器正确维护父子链（参考 html_elements.go 中的 div/span）。

func ActiveElement(doc HTMLDocument) HTMLElement
    ActiveElement 返回当前被按下的元素（鼠标按下到抬起之间）。

func CloneElement(e HTMLElement) HTMLElement
    CloneElement 克隆注册表中的原型，返回全新的独立实例。 原型是 typed nil（接口非 nil、指针为 nil）时返回 nil：调
    Clone 会空指针崩溃。

func ElementAt(root HTMLElement, x, y int) HTMLElement
    ElementAt 命中测试：返回包含 (x, y) 的最深层元素，未命中返回 nil。 文本节点不可命中，事件目标为其父元素；零尺寸盒不参与命中。
    展开的 select 选项浮层绘制在最上层，优先于常规树遍历判定。 命中受祖先 overflow 裁剪约束：点落在裁剪区外时整个子树不可命中
    （视觉上已被裁掉的内容不应可点，与渲染一致）。

    与渲染同口径：display:none 的子树不参与命中（渲染层也直接跳过）。 遍历不按"点是否落在子盒内"剪枝——子内容可以露在父盒之外（绝对定位、
    负边距），overflow 可见时它照样画出来，也就照样能点到；否则就是 「看得见点不到」。

func FocusedElement(doc HTMLDocument) HTMLElement
    FocusedElement 返回文档当前聚焦元素，无则 nil。

func HoveredElement(doc HTMLDocument) HTMLElement
    HoveredElement 返回当前被鼠标悬停的元素（无则 nil）。

func NewTextNode(text string) HTMLElement
    NewTextNode 创建一个文本节点。

func ParseFragment(src string) HTMLElement
    ParseFragment 将 HTML 片段解析到一个匿名 body 节点下并返回该节点。

func SelectOptions(sel HTMLElement) []HTMLElement
    SelectOptions 是 selectOptions 的导出版本。

type HTMLHeadElement interface {
	HTMLElement

	Title() HTMLElement
}

type HTMLSpanElement struct {
	HTMLElement
}

func (e *HTMLSpanElement) AppendChild(child HTMLElement, es ...HTMLElement)

func (e *HTMLSpanElement) Children() []HTMLElement

func (e *HTMLSpanElement) Clone() HTMLElement

func (e *HTMLSpanElement) ParentElement() HTMLElement

func (e *HTMLSpanElement) RemoveChild(child HTMLElement, es ...HTMLElement)

type HTMLUnknownElement struct {
	HTMLElement
}
    HTMLUnknownElement 未知标签的占位类型（行内原子盒）。

type HeadlessViewport struct {
	W, H int
	G    Graphics
}
    HeadlessViewport 提供无显示环境下的测试视口（FakeGraphics）。

func NewHeadlessViewport(w, h int) *HeadlessViewport
    NewHeadlessViewport 创建无显示视口：w×h 视口 + FakeGraphics 绘制后端， 供无显示器环境做级联/布局与 SVG
    导出（不产生任何窗口）。

    要出图/逐像素断言像素，用 NewBufferedViewport（软件光栅化后端）。

func (v *HeadlessViewport) Graphics() Graphics
    Graphics 返回该视口使用的绘制后端（默认 FakeGraphics）。

func (v *HeadlessViewport) ViewportHeight() int
    ViewportHeight 返回视口高度（px）。

func (v *HeadlessViewport) ViewportWidth() int
    ViewportWidth 返回视口宽度（px）。

type Image = core.Image
    Image 是绘制目标里的一张位图。

type KeyEvent struct {
	Type string
	Key  int
	Mods uint16
}
    KeyEvent 描述一次按键事件。Key 为键码：ASCII 区对齐 SDL2 的 SDL_keysym.sym（方向键/Home/End
    等扫描码区见 text_input.go 的常量表）， Mods 为修饰键位掩码（ModCtrl/ModShift，见 text_input.go）。

    keydown 派发给聚焦元素；处理器返回 true 表示"已消费"，引擎的默认动作 （退格/删除/回车换行/方向键移动光标等）不再执行——等价于网页的
    preventDefault。应用层靠它把"回车 = 打开"这类行为接进来（浏览器地址栏、 记事本查找框都用得上），不必去改引擎的默认动作。

type LineHeight struct {
	// Has unexported fields.
}
    LineHeight 是 line-height 的值：normal / 相对字号的倍数 / 绝对长度。

    倍数与绝对长度都**继承原始形态**而非解析后的像素值——倍数要相对子元素 自己的字号解析（父级 line-height:1.5 + 子级
    font-size:32px → 子级行高 48px），继承成像素就全错了。

func NewLineHeightFactor(f float32) LineHeight
    NewLineHeightFactor 造相对字号的倍数行高（1.5 表示 1.5em）。

func NewLineHeightLength(px float32) LineHeight
    NewLineHeightLength 造绝对行高（px）。

func NewLineHeightNormal() LineHeight
    NewLineHeightNormal 造 line-height: normal。

func (l LineHeight) Factor() float32
    Factor 返回倍数（仅 LineHeightFactor 有效）。

func (l LineHeight) Kind() LineHeightKind
    Kind 返回取值形态。

func (l LineHeight) Length() float32
    Length 返回绝对行高（仅 LineHeightLength 有效）。

type LineHeightKind uint8
    LineHeightKind 是 line-height 的取值形态。

const (
	// LineHeightNormal 是 line-height: normal——行高取字体度量
	// （ascent + descent），不做额外行距调整。
	LineHeightNormal LineHeightKind = iota
	// LineHeightFactor 是相对**元素自身字号**的倍数：line-height:1.5 与
	// line-height:150% 同义（两者都按继承下来的倍数、相对各自元素的字号
	// 解析），故归为同一形态。
	LineHeightFactor
	// LineHeightLength 是绝对行高（px）。
	LineHeightLength
)
type MouseEvent struct {
	Type    string
	X       int
	Y       int
	DeltaX  int
	DeltaY  int
	Element HTMLElement
}
    MouseEvent 描述一次鼠标事件，坐标为文档视口坐标。 DeltaX/DeltaY 仅在 wheel 事件上有意义（像素位移，正向 =
    内容向左/向上滚， 即视口向下/向右移，与浏览器 WheelEvent.deltaX/Y 同号）。

type OnCSSStyleDeclarationChanged interface {
	OnChanged()
}

type Overflow uint8
    Overflow 控制盒子内容溢出时的处理。visible 不裁剪；hidden 裁剪但不可滚； scroll/auto 裁剪且可滚（auto
    仅在内容确实溢出时才可滚）。

const (
	OverflowVisible Overflow = iota // 默认：溢出可见（不裁剪不滚动）
	OverflowHidden                  // 裁剪，不可滚
	OverflowScroll                  // 裁剪，恒可滚（滚动条与否不影响滚轮）
	OverflowAuto                    // 裁剪，溢出时可滚
)
type Paint = core.Paint
    Paint 是一次绘制动作的属性集合。

func NewPaint() Paint
    NewPaint 造一份默认绘制属性（16px 常规字重、黑色）。

type PointerEvents uint8
    PointerEvents 决定元素是否参与命中测试（pointer-events）。只实现两档： auto 是 CSS 默认（正常接收点击）；none
    让元素自己不再是被点中的目标。

    为什么要有这一档：外壳与页面里存在「只为排版而存在」的盒子——ui 用来给 fixed 菜单栏/状态栏让位的占位块、1px 分隔线。它们按 DOM
    序排在页面内容 之后，于是盒子里层的内容虽然照样画得出来（渲染层按 DOM 序叠放），点击却 被这些透明盒子吃掉，表现为「看得见点不到」。真实 CSS
    里这类装饰元素正是 用 pointer-events:none 交还交互的。

    与 CSS 一致，该属性沿父链继承：父级 none 时子树默认也不可点，子级自己 声明 auto 可以恢复。

const (
	PointerEventsAuto PointerEvents = iota
	PointerEventsNone
)
type Position uint8

const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
)
type Rect interface {
	Left() Size
	Right() Size
	Top() Size
	Bottom() Size
}
    Rect 在这里表示盒模型四边的尺寸集合（left/right/top/bottom）。

func NewRect(left, right, top, botoom Size) Rect

func NewZeroRect() Rect

type Selector struct {
	// Has unexported fields.
}
    Selector 是从左到右排列的复合选择器序列，combs[i] 位于 compounds[i] 与 compounds[i+1] 之间。

func ParseSelector(q string) *Selector
    ParseSelector 解析 "div.card > .title" 形式的选择器串。

func (s *Selector) Specificity() int
    Specificity 返回选择器特异度，按 CSS 的 (a,b,c) 字典序编码为单个整数： a=id
    数、b=class/attr/伪类数、c=类型选择器数，编码为 a*1e6 + b*1e3 + c。 编码保证整数比较与 CSS 优先级一致——旧实现用
    id*100 + class*10 + tag， 11 个类（110）会压过 1 个 id（100），与 CSS 相反。

    伪类按 class 档计特异度（CSS 2.1 §6.4.3：伪类与类选择器同级）。

type Shadow struct {
	X, Y   int // 偏移
	Blur   int // 模糊半径
	Spread int // 扩散半径（可为负）
	Color  Color
	// Inset 只解析、暂不绘制：外阴影覆盖卡片/弹层等主场景，内阴影需要在
	// 边框盒内单独做遮罩（并叠在背景之上），留待需要时再补。
	Inset bool
}
    Shadow 是一个 box-shadow 值。

    长度在解析期就落成像素（box-shadow 的模糊/扩散半径不接受百分比；偏移 虽允许百分比，但 UI 与对照页只用
    px，本轮不引入带单位的 Shadow，避免 在绘制期还要拿元素盒宽度做二次解析）。Color 为 nil 表示 currentColor：
    级联期不知道元素最终文字色，留到绘制期回退。

type Size = core.Size
    Size 表示一个带单位的尺寸值。

func NewAutoSize() Size
    NewAutoSize 表示 CSS 中的 auto。

func NewSize(tp SizeType, intVal int, float32Val float32) Size
    NewSize 造一个带单位的尺寸。

func NewZeroSize() Size
    构造器转发到 core（值类型与实现同处 core，跨层共用同一份）。

type SizeType = core.SizeType
    SizeType 是带单位尺寸的种类。

type Sprite = core.Sprite
    Sprite 是一块可 blit 的位图。

type SpriteDrawer = core.SpriteDrawer
    SpriteDrawer 是 Graphics 的可选扩展：支持位图 blit。

type Stylesheet struct {
	// Has unexported fields.
}
    Stylesheet 是解析后的 CSS 规则集合。

func ParseCSS(src string) *Stylesheet
    ParseCSS 解析一段 CSS 文本。不支持的语法（@media 等）会被跳过而不报错； @import
    只被识别（不加载）——需要加载外部样式表时走 parseCSSRules。

func (s *Stylesheet) Merge(other *Stylesheet)
    Merge 将 other 的规则追加到 s（用于 <style> 多段合并）。

type StylesheetLoader func(href string) (string, error)
    StylesheetLoader 加载一份外部样式表，返回 CSS 文本；失败返回 error。

    href 是**已解析成绝对地址**的引用（见 resolveURL）：<link> 按文档的 BaseURL 解析，@import
    按上一层样式表的地址解析，所以加载器拿到的永远 是可以直接用的地址，不用自己再拼相对路径。

    加载是**同步**的：在解析 <link>/@import 时当场调用，跑在打开文档的那个 goroutine（浏览器的 UI
    线程）。要联网的宿主应在加载器里做缓存或预取， 别在这里等一个慢请求——引擎没有「样式到货后再重排」的链路。

    返回 error 时该样式表被忽略（页面继续渲染，其余样式照常生效），错误 由宿主自己上报——引擎不打印、不弹窗：缺一份 CSS 不该让页面打不开。

type TextAlign uint8

const (
	TextAlignLeft TextAlign = iota // 默认（等同 CSS start）
	TextAlignCenter
	TextAlignRight
)
type TextDecoration uint8
    TextDecoration 文本装饰（下划线/删除线）。CSS 该属性沿父链继承， <a> 的 UA 下划线正是靠继承落到其文本子节点上。

const (
	TextDecorationNone TextDecoration = iota
	TextDecorationUnderline
	TextDecorationLineThrough
)
type TooltipColors struct {
	Background Color // 底色（默认深板岩蓝）
	Border     Color // 1px 描边（默认中间色）
	Text       Color // 文字（默认浅色）
}
    TooltipColors 是提示气泡的配色（浮在文档之上的绘制层，不是 DOM 元素， 所以拿不到级联样式，只能由应用显式下发）。空字段按引擎默认。

func TooltipColorsOf(doc HTMLDocument) TooltipColors
    TooltipColorsOf 返回文档当前的气泡配色（未设置的字段为 nil，表示用引擎 默认配色）。

type VerticalAlign uint8
    VerticalAlign 是 vertical-align 的取值（**不继承**）。只作用于行内级盒 （引擎里行内元素都是"原子盒"，见
    measure.go 的 inlineBoxBaseline）。

const (
	// VerticalAlignBaseline 是默认值：盒的基线与父级基线对齐。
	VerticalAlignBaseline VerticalAlign = iota
	// VerticalAlignTop：盒的顶边与行盒顶边对齐。
	VerticalAlignTop
	// VerticalAlignMiddle：盒的垂直中点与「父级基线 + 半个 x 高度」对齐。
	VerticalAlignMiddle
	// VerticalAlignBottom：盒的底边与行盒底边对齐。
	VerticalAlignBottom
	// VerticalAlignTextTop：盒的顶边与父级内容区上沿（strut 的 ascent）对齐。
	VerticalAlignTextTop
	// VerticalAlignTextBottom：盒的底边与父级内容区下沿（strut 的 descent）对齐。
	VerticalAlignTextBottom
	// VerticalAlignSub：相对基线下移（父级字号/5+1，与 Chrome 的 <sub> 同口径）。
	VerticalAlignSub
	// VerticalAlignSuper：相对基线上移（父级字号/3+1，与 Chrome 的 <sup> 同口径）。
	VerticalAlignSuper
	// VerticalAlignRaise：长度/百分比，相对基线上移指定距离。
	VerticalAlignRaise
)
type Viewport = core.Viewport
    Viewport 是渲染输出目标的抽象：一个有尺寸、可绘图的窗口/画布。 具体后端（如 SDL2）实现该接口；文档通过 OpenDocument
    绑定到视口。

    接口本体在 core 包（见 core.Viewport）：平台后端只依赖 core，不反向 依赖 engine。这里的别名保证对外 API
    不变、两边实现的是同一类型。

type WhiteSpace uint8
    WhiteSpace 控制空白折叠与换行（white-space）。只实现对照需要的两档： normal 为 CSS
    默认（连续空白折叠为一个空格、可软换行）；pre 保留空白与 换行、不做软换行（<pre> 的 UA 值）。缺这一档时 <pre> 里的换行与缩进会
    被当成普通文本折叠掉，与浏览器视觉差一整行。

const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpacePre
)
````

---

# package runtime/clipboard
````
package clipboard // import "github.com/go-comm/ghromex/runtime/clipboard"

Package clipboard 是系统剪贴板的通用调用入口。

调用方只 import 这一个包：平台差异由它内部收着——Windows 走 backend/clipboard 的实现，其它平台没有剪贴板（Text
返回空串、 SetText 是空操作）。

剪贴板里只放文本：引擎的输入框只管选区文本，取选区、读写剪贴板是 应用层的事（engine 保持纯 Go、可移植）。

典型用法（记事本示例）：

    clipboard.SetText(selected)   // 复制
    txt := clipboard.Text()       // 粘贴

注意：剪贴板是进程间共享的，写入会覆盖用户剪贴板里的原内容。

FUNCTIONS

func SetText(text string)
    SetText 把 UTF-8 文本写入系统剪贴板。

    当前平台没有剪贴板实现时是空操作。写空串等于不动剪贴板。

func Text() string
    Text 读取系统剪贴板中的文本（UTF-8）。

    剪贴板为空、里面不是文本，或当前平台没有剪贴板实现时返回空串。

````

---

# package runtime/dialog
````
package dialog // import "github.com/go-comm/ghromex/runtime/dialog"

Package dialog 是原生对话框的通用调用入口。

调用方只 import 这一个包：平台差异由它内部收着——Windows 走 backend/dialog 的 Win32 实现（Shell 的
IFileDialog 与 user32 的 MessageBoxW，纯 Win32 调用、无 CGO），其它平台返回 ErrUnsupported。

五个对话框：打开文件、打开多个文件、选择文件夹、另存为、消息框。

# 父窗口

原生对话框要挂在应用窗口下面，需要平台的原生窗口句柄—— browser.Window.NativeHandle() 给出（Windows 上是真实
HWND）。用 Parent 包装它传给对话框：

    paths, err := dialog.OpenFileDialog(dialog.Parent(hwnd), dialog.Options{
    	Title:      "打开",
    	DefaultDir: `C:\Users\me\Documents`,
    	Filters: []dialog.FileFilter{
    		{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
    		{Name: "所有文件 (*.*)", Pattern: "*.*"},
    	},
    })

Parent 为 0 表示没有父窗口：对话框照样能弹，只是不挂在主窗口下面。

# 模态

对话框都是模态的：调用期间阻塞当前线程（父窗口的事件循环随之暂停）， 这与原生应用里模态对话框的表现一致，应在按钮/菜单的回调里直接调用。

VARIABLES

var ErrUnsupported = backenddialog.ErrUnsupported
    ErrUnsupported 表示当前平台没有原生对话框实现。


FUNCTIONS

func OpenDirectoryDialog(parent Parent, opts Options) (string, error)
    OpenDirectoryDialog 弹出「选择文件夹」对话框，返回用户选中的目录。

    用户取消返回 ("", nil)；平台不支持时返回 ErrUnsupported。

func OpenFileDialog(parent Parent, opts Options) ([]string, error)
    OpenFileDialog 弹出「打开文件」对话框（单选），返回用户选中的文件路径。

    用户取消返回 (nil, nil)——取消不是错误，调用方按「用户没选」处理即可。 平台不支持时返回 ErrUnsupported。

func OpenMultipleFilesDialog(parent Parent, opts Options) ([]string, error)
    OpenMultipleFilesDialog 弹出「打开文件」对话框（允许多选），返回用户 选中的多个文件路径。

    用户取消返回 (nil, nil)；平台不支持时返回 ErrUnsupported。

func SaveFileDialog(parent Parent, opts Options) (string, error)
    SaveFileDialog 弹出「另存为」对话框，返回用户输入的保存路径。

    用户取消返回 ("", nil)；平台不支持时返回 ErrUnsupported。


TYPES

type FileFilter = backenddialog.FileFilter
    FileFilter 描述文件对话框里的一个类型过滤器（对应下拉框里的一项）。

type MessageButtons = backenddialog.MessageButtons
    MessageButtons 是消息框的按钮组合。

type MessageIcon = backenddialog.MessageIcon
    MessageIcon 是消息框的图标。

type MessageOptions = backenddialog.MessageOptions
    MessageOptions 是消息框的选项。

type MessageResult = backenddialog.MessageResult
    MessageResult 是用户在消息框里点击的按钮。

func MessageDialog(parent Parent, opts MessageOptions) (MessageResult, error)
    MessageDialog 弹一个模态消息框，返回用户点击的按钮。

    例如「是否覆盖已存在的文件？」：

        res, err := dialog.MessageDialog(dialog.Parent(hwnd), dialog.MessageOptions{
        	Title:   "确认",
        	Message: "文件已存在，要覆盖吗？",
        	Icon:    dialog.IconQuestion,
        	Buttons: dialog.ButtonsYesNo,
        })
        if err == nil && res == dialog.ResultYes {
        	// 用户点了「是」
        }

    平台不支持时返回 ErrUnsupported。

type Options = backenddialog.Options
    Options 是文件对话框的选项。

type Parent uintptr
    Parent 是对话框的父窗口（平台相关的原生窗口句柄）。

    取法：browser.Window.NativeHandle()。0 表示没有父窗口。

````

---

# package runtime/screen
````
package screen // import "github.com/go-comm/ghromex/runtime/screen"

Package screen 是桌面抓屏的平台能力。

渲染后端（SDL）只能回读**自己窗口**的内容；截图工具要把整个桌面冻结 下来给用户拖选，必须走操作系统接口。Windows 上用 GDI 的
BitBlt，纯 Win32 调用、无 CGO；其它平台返回 ErrUnsupported。

与 runtime/clipboard、runtime/dialog 的定位一致：平台差异收在内部， 调用方只看到一个返回 image.RGBA 的函数。

VARIABLES

var ErrUnsupported = errors.New("ghromex 桌面抓屏仅支持 Windows")
    ErrUnsupported 表示当前平台没有桌面抓屏实现。


FUNCTIONS

func Capture() (*image.RGBA, error)
    Capture 抓取主屏，返回 1:1 的 RGBA 图像，原点为 (0,0)。

    返回的像素 alpha 恒为 255。原因：GDI 的 BitBlt 不写 alpha 字节（32bpp 下通常是 0），而渲染层的
    DrawSprite 走 alpha 混合——不补 255 的话整张 截图会被当成“全透明”画不出来。

    只抓主屏：多显示器虚拟桌面（负坐标、每显示器不同缩放）需要另做坐标 映射，属于后续能力。

````

---

# package ui
````
package ui // import "github.com/go-comm/ghromex/ui"

本文件是把组件库接到窗口上的便利层（App）；包文档见 doc.go。

本文件是页面基础控件：标签、文本视图、按钮、勾选、下拉与输入框。

控件是引擎原生元素上的薄包装：应用仍然用 AppendChild/OnClick 那一套， 只是少写类名、取值/设值/禁用态和事件适配。样式全部来自
theme.go。

Package ui 是应用工作台与页面组件库。

它提供三样东西：

  - 工作台：菜单栏（含下拉浮层、快捷键、禁用态）、工具栏、状态栏（左侧分段 + 右侧消息）；
  - 页面组件：卡片、标题、副标题、徽标、提示条、列表、操作行、输入框、下拉、勾选、只读文本视图、按钮；
  - 主题：浅色/深色两套视觉令牌 + 生成的样式表，应用一行 CSS 都不用写。

包内分两层，各管各的生命周期：

  - Workbench（工作台，入口 Install）：装在一份文档上的周边构件——内容四周的 菜单栏/工具栏/状态栏、主题样式与快捷键表。只依赖
    engine，无头也能装； 换文档要重装（工作台活在文档 DOM 里）。
  - App（应用，入口 New）：窗口的生命周期封装——建 browser.Window、换文档时
    重装工作台、按键路由到当前工作台、Run/Close。App 只是 Install 的一个 固定用法，需要自管窗口（或不要窗口）时直接用
    Install。

# 最小用法

    spec := ui.Spec{
    	Title: "记事本", Width: 900, Height: 620, Status: "就绪",
    	Menus: []ui.Menu{{Title: "文件", Items: []ui.Action{
    		ui.Item("保存", "Ctrl+S", save).WithIcon("save").When(editor.Dirty),
    		ui.Sep(),
    		ui.Item("退出", "Alt+Q", ui.Quit),
    	}}},
    	Tools: []ui.Action{
    		ui.Tool("保存", save).WithIcon("save").When(editor.Dirty),
    		ui.Sep(),
    		ui.Tool("复制", copy).WithIcon("copy"),
    	},
    }
    app, err := ui.New(spec)
    if err != nil {
    	return err
    }
    defer app.Close()
    if _, err := app.LoadHTML(page); err != nil {
    	return err
    }
    return app.Run()

页面内容只写类名（<div class="system-ui-page">、<div class="system-ui-card">…），
样式全部来自 Install 注入的主题样式表。组件类统一带 system-ui- 前缀，并 只在 Install 加到文档根上的 system-ui
作用域下生效；根作用域把组件规则 的特异度抬高一档，第三方 `.panel div` 之类的后代选择器压不过。

body 的复位同样归工作台：装了工作台的文档，body 无外边距、颜色字体跟随 主题，页面不必自写 margin:0 / padding:0 / color
/ font-family 这些样板； 自定义排版请挂在内容容器上（根作用域下 body 规则特异度更高，页面的 裸 body 规则盖不过它）。确实要保留
body 原样的页面（演示 UA 样式、靠 body padding 给固定栏让位）用 Spec.NoBodyReset 跳过复位。

# 状态栏

左侧可以分段，段有 Key、固定/自适应宽度、段内对齐和隐藏态；右侧消息独立 更新，非空时按 Theme.StatusRightWidth
预留宽度。高频更新只改文字：

    spec.StatusSegments = []ui.StatusSegment{
    	{Key: "file", Text: "未命名", Width: 220},
    	{Key: "pos", Text: "行 1，列 1", Width: 140},
    	{Key: "chars", Text: "0 个字符"},
    }
    sh.SetStatusSegment("pos", "行 2，列 3")
    sh.SetStatusRight("已保存")

Spec.Status / SetStatus 是单段兼容写法。

# 页面控件

页面可以只写类名，也可以用 Go 侧包装构造原生控件；NewXxx 返回的 View 都 实现 Element()，可直接 AppendChild。注意
NewButton 是内容区按钮，工具栏 的 ui.Tool 仍构造 Action：

    name := ui.NewTextInput(doc, "未命名.txt")
    field := ui.NewField(doc, "文件名称", name)
    field.SetHint("最多 255 个字符")
    page.AppendChild(field.Element())

    auto := ui.NewCheckbox(doc, "自动保存", true)
    auto.OnChange(func(on bool) { /* ... */ })
    page.AppendChild(auto.Element())

横着一排控件（查找替换栏、地址栏这类）用 NewBar 拼：条内控件排成一行、 按 Theme.BarHeight 垂直居中，条自己不管摆哪儿——定位声明交给
SetInline， 收起用 SetVisible（它只在应用那份声明上补 display:none，不会冲掉 top）：

    bar := ui.NewBar(doc, keyword, find).SetInline("position:absolute;left:0;top:72px")
    bar.SetVisible(false)
    page.AppendChild(bar.Element())

工具栏里也要放输入框/下拉时，用 Slot 把现成控件嵌进去（槽位不是命令， 没有快捷键与可用态；控件句柄仍归应用）：

    addr := ui.NewTextInput(doc, "")
    ...
    Tools: []ui.Action{ui.Tool("刷新", refresh), ui.Slot(addr)},

输入框目前没有实时 input/change 事件，实时联动用 OnKeyDown 或在命令执行 时读取 Value；只读多行文本用
TextView，可编辑多行用 TextAreaView——要它 贴着铺满整块内容区（纸面而不是表单字段）就切平铺变体：

    editor := ui.NewTextArea(doc, "")
    editor.SetVariant("plain") // 去掉描边与圆角

# 悬停提示

引擎内置系统气泡式提示（原生 title 属性）：在带非空 title 的元素（含其 子孙）上停留片刻，鼠标右下弹出气泡；离开、按下、点击或滚动收起，换目标
重新计时。工具栏按钮的提示已按 Label + Shortcut 自动生成，图标按钮悬停 就能看到文字；页面元素要提示就直接写 title 属性：

    btn.Element().SetAttribute("title", "保存到当前文件（Ctrl+S）")

气泡配色跟随主题（Install / SetTheme 自动下发：浮层底色 + 描边 + 主文字）； 不用 ui 的裸引擎应用可用
engine.SetTooltipColors 自行指定，默认深底浅字。

# 图标

命令可以用 WithIcon 挂图标：

    ui.Tool("保存", save).WithIcon("save")
    ui.Item("新建", "Ctrl+N", newFile).WithIcon("new")

内置图标名见 iconGlyphs，按用途分组：

    文件/编辑  new open save save-as copy cut paste undo redo search replace
              print delete refresh folder page document code edit rename select-all
    方向/导航  up down left right chevron-up chevron-down chevron-left chevron-right
              page-left page-right back forward home go-to-start menu history
    对齐/排版  align-left align-center align-right bulleted-list list check-list
              bold italic underline font-size font-color highlight
    播放/媒体  play pause stop record previous next rewind fast-forward shuffle
              repeat repeat-one volume mute fullscreen back-to-window
    视图/窗口  zoom-in zoom-out fit-page view view-all preview dock-left dock-right
              dock-bottom close-pane open-pane minimize maximize restore
    通用       add remove check close more filter settings info help warning error
              favorite share link attach send folder calendar

也可以直接给一个字符。字形来自 Windows 10/11 自带的 Segoe MDL2 Assets （segmdl2.ttf），按
16px 网格设计、颜色跟随文字，悬停/禁用/换主题都不需要 额外资源。工具栏按钮有图标时画成方形图标按钮，没有图标时退回文字按钮；
菜单里只要有一项带图标，整张菜单就会预留图标列。

需要枚举全部图标（做选择器、文档或图标表）时调用 Icons()：返回按名称 排序的 Name/Glyph
副本，别名也会作为独立条目出现。examples/iconsheet 是一个可直接运行的图标表。

图标字体是系统字体：跨平台部署时要么在 Theme.IconFont 里给一个同编码 的图标字体，要么只写文字（不调 WithIcon）。

# 主题

内置浅色 Light() 与深色 Dark()；Spec.Theme 留空时用浅色：

    dark := ui.Dark()
    spec.Theme = &dark

两套主题的尺寸与字体相同，只换颜色。想微调时先取完整的一套再改字段：

    th := ui.Dark()
    th.Accent = "#22C55E"
    spec.Theme = &th

不能只写 ui.Theme{Bg: "#0F172A"} 就指望得到深色：零值字段统一按浅色 补齐（零值 Theme
的默认语义是浅色），其余颜色会留在浅色上。OnAccent 是强调色上的文字/图标颜色，深色主题用它把亮蓝底上的字压成深色。

运行时切换用 App.SetTheme（或 Workbench.SetTheme）：新主题样式追加在旧样式 之后，页面内容不重建，适合浅色/深色切换。

# 语义色

主题提供六组语义色，页面只写类名，不用记十六进制：

    Primary/Accent  强调：主按钮、选中、链接、焦点
    Default         默认/中性：次按钮、取消
    Success         成功；Warning 警告；Danger 危险；Info 信息

每组令牌分实色与淡底两套：Accent/Success/Warning/Danger/Info 是实底，
On* 是实底上的文字，*Soft/*Border/*Text 是淡底、描边与淡底文字。 Default
不单独存令牌，复用 Surface/Border/Text/SurfaceAlt/Press 这组 中性色。按钮用
is-primary/is-default/is-success/is-warning/is-danger/
is-info；徽标、提示条与文字色类同样接受这些后缀：

    <div class="system-ui-btn is-success">通过</div>
    <span class="system-ui-badge is-warning">待处理</span>
    <div class="system-ui-alert is-danger">删除后无法恢复。</div>
    <span class="system-ui-text-info">共 12 项</span>

对比度由主题负责：Warning 的实底用深色 On，其它实底在浅色主题用白字、 深色主题用同色系深字。

# 为什么是「文档内自绘」而不是原生控件

引擎只渲染文档，没有独立的窗口 chrome 层。组件库的做法是把工作台作为 文档的一部分：position:fixed 压在页面上、body
首尾插占位块让出空间、 换文档时重装。好处是纯 Go、跨平台、可测试（无头就能导出 PNG/SVG 看 效果）；代价是页面自己 z-index
更高的元素会盖住工作台，且工作台不参与 原生窗口菜单的键盘语义。

# 三个必须知道的约定

  - 工作台是文档的一部分：换文档后要重新 Install（App 已通过 browser.Window.SetOnDocument 自动完成）。
  - 应用快捷键先于页面：App 用 browser.Window.SetKeyHandler 注册， 输入框聚焦时 Ctrl+S 也先到应用。
  - 主题只使用引擎支持的 CSS 子集：没有 flex/grid/过渡，主题保持扁平 （不用阴影），横向排列靠 inline-block，分隔线是 1px
    的实体元素。

本文件是组件库内部的 DOM 与按键小工具；包文档见 doc.go。

本文件是图标表：命令的图标名 → 字形；包文档见 doc.go。

本文件是菜单栏与下拉浮层；包文档见 doc.go。

本文件是状态栏；包文档见 doc.go。

本文件是组件库的视觉令牌与样式表生成；包文档见 doc.go。

本文件是工具栏；包文档见 doc.go。

本文件是组件库的装配与命令执行：Spec/Action 模型、Install、快捷键与 刷新；菜单/工具栏/状态栏的实现分别在
menu.go/toolbar.go/status.go。

VARIABLES

var ErrQuit = errors.New("ui: quit")
    ErrQuit 是「退出」命令的返回值：App 收到后请求关窗。

    用返回值而不是让命令直接调 Window.Close：命令表可以在无头环境 （ui.Install，没有窗口）里跑，退出语义必须由宿主决定。


FUNCTIONS

func Quit() error
    Quit 是标准的退出命令，可直接放进菜单：ui.Item("退出", "Alt+Q", ui.Quit)。


TYPES

type Action struct {
	Label    string // 显示文字（图标按钮上作为无图标时的兜底文字）
	Shortcut string // 展示与匹配用："Ctrl+S"、"Alt+Left"；空表示没有
	Icon     string // 图标名（见 iconGlyphs）或直接给字符；空表示没有
	Sep      bool   // 是否是分隔线（此时其余字段忽略）
	Run      func() error
	// Enable 返回命令当前是否可用；nil 表示恒可用。菜单打开时、按钮
	// 刷新时求值——不做每帧轮询。
	Enable func() bool

	// Has unexported fields.
}
    Action 是一条应用命令，菜单项与工具栏按钮共用。

    命令与呈现分离：同一份 Action 既能在菜单里出现，也能作为工具栏按钮， 快捷键只写一次。

func Item(label, shortcut string, run func() error) Action
    Item 构造一条菜单命令。run 为 nil 时命令只关闭菜单。

func Sep() Action
    Sep 构造一条分隔线（菜单里是横线，工具栏里是竖线）。

func Slot(c Control) Action
    Slot 把一个现成控件（输入框、下拉等）嵌进工具栏，与命令按钮同排：

        addr := ui.NewTextInput(doc, "")
        ...
        Tools: []ui.Action{
        	ui.Tool("刷新", refresh).WithIcon("refresh"),
        	ui.Slot(addr),
        }

    控件句柄仍归应用：取值、设值、禁用直接调控件自己的方法，不必按 id 回查 元素。槽位不是命令：没有快捷键与可用态，禁用请用控件自己的
    SetEnabled。

func Tool(label string, run func() error) Action
    Tool 构造一个工具栏按钮（无快捷键），进 Spec.Tools、渲染成 .system-ui-tool；与内容区控件的 NewButton
    不是一回事。

func (a Action) When(f func() bool) Action
    When 给命令挂可用性判断，返回新值便于链式书写：

        ui.Item("保存", "Ctrl+S", save).When(editor.Dirty)

func (a Action) WithIcon(name string) Action
    WithIcon 给命令挂图标，返回新值便于链式书写：

        ui.Tool("保存", save).WithIcon("save")

    图标名见包内 iconGlyphs（文件/编辑、方向/导航、对齐/排版、播放/媒体、 视图/窗口和通用命令），常用如
    up/down/left/right、align-left/align-center/
    align-right、play/pause/stop、zoom-in/zoom-out；也可以直接写字符。同一份 Action
    放进菜单与工具栏时，两处都用这个图标。

type App struct {
	// Has unexported fields.
}
    App 是「窗口 + 工作台」的便利封装：建窗、装菜单/工具栏/状态栏、换页 重装、快捷键路由一手包办。

    只想要文档层（无头导出、嵌入已有窗口）时用 Install，不必用 App： App 只是 Install 的一个固定用法。

func New(spec Spec) (*App, error)
    New 按 Spec 建窗口并注册文档/按键钩子。

    此时还没有文档：菜单栏等要等 LoadHTML 之后、文档存在时装上去 （SetOnDocument 回调负责每次换文档后重装）。

func (a *App) Close()
    Close 释放窗口资源。

func (a *App) LoadHTML(src string) (engine.HTMLDocument, error)
    LoadHTML 解析页面源码并装好工作台，返回文档。

func (a *App) LoadHTMLWith(src string, opts engine.DocumentOptions) (engine.HTMLDocument, error)
    LoadHTMLWith 同 LoadHTML，可传文档选项（外部样式表加载器、地址基准）。

func (a *App) RequestClose()
    RequestClose 请求退出：投递关闭事件，Run 会正常返回。

func (a *App) Run() error
    Run 进入事件循环，直到窗口关闭。

func (a *App) SetStatus(text string)
    SetStatus 更新状态栏左侧文本（工作台还没建好时忽略，便于在命令闭包 里无脑调用）。

func (a *App) SetTheme(theme Theme)
    SetTheme 切换当前窗口的主题。

    已经装好的工作台会追加一套新主题样式，页面内容与滚动位置保持不变；之后 如果换文档，新文档也沿用这套主题。

func (a *App) Window() *browser.Window
    Window 暴露底层窗口（原生对话框父句柄、标题、截图等）。

func (a *App) Workbench() *Workbench
    Workbench 是装在当前文档上的工作台；还没加载文档时为 nil。

type BarView struct {
	// Has unexported fields.
}
    BarView 是页面里的横向控件条：一组控件排成一行（查找替换栏、地址栏这类）。

    它和工作台的工具栏不是一回事：工具栏是 fixed 在视口上的应用工作台，控件条属于 页面内容，摆在哪里由应用决定，所以 NewBar
    不写定位。条内控件按 inline-block 排开（引擎没有 flex，ui 的主题排工具栏也是这个办法），窗口太窄放不下时会 从行尾掉到下一行——要
    nowrap 的话在主题里加 white-space。

    定位声明与可见性分开管：应用用 SetInline 写自己那份内联声明（例如
    "position:absolute;left:0;top:72px"），SetVisible 只在其上补 display， 两者不会互相冲掉。

func NewBar(doc engine.HTMLDocument, controls ...Control) *BarView
    NewBar 创建控件条，controls 依次从左到右排入（nil 跳过）。

func (v *BarView) Append(controls ...Control) *BarView
    Append 追加控件，顺序即从左到右。

func (v *BarView) Element() engine.HTMLElement

func (v *BarView) SetInline(decls string) *BarView
    SetInline 设置应用自己那份内联声明（定位、宽度等）；传空串清掉。 显隐请用 SetVisible，直接往元素写 style 会被
    SetVisible 覆盖。

func (v *BarView) SetVisible(on bool) *BarView
    SetVisible 显示/隐藏控件条。隐藏写的是 display:none：引擎会连布局盒一起 清掉，条里的控件既看不见也点不到（与渲染层同口径）。

func (v *BarView) Visible() bool
    Visible 报告控件条当前是否可见。

type ButtonView struct {
	// Has unexported fields.
}
    ButtonView 是内容区按钮，底层是 <button type="button">。

    工具栏的 ui.Tool 是 Action 构造器；这里用 NewButton 区分。

func NewButton(doc engine.HTMLDocument, label string, onClick func()) *ButtonView
    NewButton 创建按钮。onClick 可以为 nil。

func (v *ButtonView) Element() engine.HTMLElement

func (v *ButtonView) SetEnabled(on bool)
    SetEnabled 设置按钮可用态。禁用时同时写 disabled 属性和 is-disabled 类，
    点击处理器也会再判一次，避免类名尚未刷新时被点到。

func (v *ButtonView) SetLabel(label string)
    SetLabel 更新按钮文字。

func (v *ButtonView) SetVariant(name string) *ButtonView
    SetVariant 设置语义变体，如 "primary"、"danger"；也可以直接传 "is-primary"。

type CheckboxView struct {
	// Has unexported fields.
}
    CheckboxView 是原生 checkbox 加包裹式 label：点文字也能切换。

func NewCheckbox(doc engine.HTMLDocument, label string, checked bool) *CheckboxView
    NewCheckbox 创建勾选框。label 为空时只保留控件本身。

func (v *CheckboxView) Checked() bool
    Checked 返回当前勾选状态。

func (v *CheckboxView) Element() engine.HTMLElement

func (v *CheckboxView) OnChange(f func(bool))
    OnChange 注册勾选变化回调；传 nil 移除。

func (v *CheckboxView) SetChecked(on bool)
    SetChecked 设置勾选状态；程序化赋值不触发 change。

func (v *CheckboxView) SetEnabled(on bool)
    SetEnabled 设置可用态。

func (v *CheckboxView) SetLabel(label string)
    SetLabel 更新勾选框文字。

type Control interface {
	Element() engine.HTMLElement
}
    Control 是页面控件的最小公共接口。

    控件本身不是新的布局系统：Element() 返回的仍是普通 engine.HTMLElement， 可以放进任何页面容器。

type FieldView struct {
	// Has unexported fields.
}
    FieldView 是「标签 + 控件 + 提示/错误」的纵向表单项。

func NewField(doc engine.HTMLDocument, label string, control Control) *FieldView
    NewField 创建表单项。control 为 nil 时只生成标签与提示区。

func (v *FieldView) Element() engine.HTMLElement

func (v *FieldView) SetError(text string)
    SetError 更新错误文字；空串隐藏错误行并去掉表单项的错误描边。

func (v *FieldView) SetHint(text string)
    SetHint 更新提示文字；空串隐藏提示行。

type IconEntry struct {
	Name  string
	Glyph string
}
    IconEntry 是内置图标表里的一项：图标名与对应的 Segoe MDL2 字形。

func Icons() []IconEntry
    Icons 返回全部内置图标，按名称排序。

    返回的是副本，修改切片不会影响内置表；别名也会作为独立条目返回。 图标名可以直接传给 Action.WithIcon。

type LabelView struct {
	// Has unexported fields.
}
    LabelView 是单行静态标签，底层是 <label>。

func NewLabel(doc engine.HTMLDocument, text string) *LabelView
    NewLabel 创建标签。需要关联输入控件时调用 For。

func (v *LabelView) Element() engine.HTMLElement

func (v *LabelView) For(target engine.HTMLElement) *LabelView
    For 把标签关联到目标控件（设置 label 的 for 属性）。

func (v *LabelView) SetText(text string)
    SetText 更新标签文字。

type Menu struct {
	Title string
	Items []Action
}
    Menu 是一个顶级菜单。

type SelectOption struct {
	Value string
	Label string
}
    SelectOption 是下拉框的一项。

type SelectView struct {
	// Has unexported fields.
}
    SelectView 是原生 <select> 的薄包装。

func NewSelect(doc engine.HTMLDocument, opts []SelectOption) *SelectView
    NewSelect 创建下拉框；初始选中项由浏览器/引擎默认（通常第一项）， 需要指定时调用 SetValue。

func (v *SelectView) Element() engine.HTMLElement

func (v *SelectView) OnChange(f func(string))
    OnChange 注册选中项变化回调；传 nil 移除。

func (v *SelectView) SetEnabled(on bool)
    SetEnabled 设置可用态。

func (v *SelectView) SetOptions(opts []SelectOption)
    SetOptions 重建选项，尽量保留当前选中值。

func (v *SelectView) SetValue(value string) bool
    SetValue 按值选中一项；找不到时返回 false 并保持原值。

func (v *SelectView) Value() string
    Value 返回当前选中项的值。

type Spec struct {
	// Title/Width/Height 只给 App 用（建窗口）；ui.Install 忽略。
	Title  string
	Width  int
	Height int

	// Theme 为空时用 Light()。
	Theme *Theme

	// NoBodyReset 跳过对 body 的样式复位（margin/padding/color/font）；
	// 画布底色（html background-color）不受影响，始终注入。
	//
	// 默认 Install 会复位 body：UA 的 8px 外边距与默认字体不该让每个页面
	// 重复写一遍。两类页面需要关掉它：
	//   - 演示 UA 样式的页面——body 的 margin:8px 本身就是演示内容；
	//   - 靠 body padding 给固定栏让位的页面——复位会把排版顶歪。
	//
	// 关掉之后 body 的外观由页面自己负责。
	NoBodyReset bool

	Menus []Menu
	Tools []Action

	// Status/StatusRight 是状态栏的初始左右文本。
	//
	// StatusSegments 非空时优先使用分段；Status 只在没有分段时作为
	// 单段状态栏的快捷写法（等价于一个 Key 为 "status" 的段）。
	Status         string
	StatusRight    string
	StatusSegments []StatusSegment

	// Focus 返回「命令执行后该把焦点还给谁」。菜单/工具栏是普通 div，
	// 点它们会让页面里的输入框失焦；命令跑完把焦点还回去，用户才能
	// 接着打字（记事本点「保存」后光标应该还在原处）。
	Focus func() engine.HTMLElement
}
    Spec 描述一套工作台。

type StatusSegment struct {
	Key    string           // 更新用；空时按位置生成，调用方应保证唯一
	Text   string           // 初始文字
	Width  int              // 0 = 自适应；>0 = 固定宽度（含 padding，单位 px）
	Align  engine.TextAlign // 段内对齐，零值即左对齐
	Hidden bool             // 为 true 时不生成该段，也不生成它前面的分隔线
}
    StatusSegment 是状态栏左侧的一段文字。

    段按切片顺序从左到右排列，段与段之间自动加 1px 分隔线。固定 Width 的 段适合「文件名 / 行列 /
    字符数」这类需要稳定列宽的内容；Width 为 0 的 段按内容自适应，文本变化会推动后面的段。

type TextAreaView struct {
	// Has unexported fields.
}
    TextAreaView 是可编辑多行输入框。

func NewTextArea(doc engine.HTMLDocument, value string) *TextAreaView
    NewTextArea 创建可编辑多行输入框。

func (v *TextAreaView) Element() engine.HTMLElement

func (v *TextAreaView) OnKeyDown(f func(*engine.KeyEvent) bool)
    OnKeyDown 注册按键处理器；返回 true 表示消费。

func (v *TextAreaView) SetEnabled(on bool)
    SetEnabled 设置可用态。

func (v *TextAreaView) SetValue(value string)
    SetValue 设置值，光标移到末尾。

func (v *TextAreaView) SetVariant(name string) *TextAreaView
    SetVariant 设置外观变体，如 "plain"（贴边铺满：无描边、无圆角）； 也可以直接传 "is-plain"。传空串回到默认描边。

func (v *TextAreaView) Value() string
    Value 返回当前值。

type TextInputView struct {
	// Has unexported fields.
}
    TextInputView 是单行输入框。

    引擎目前没有 input/change 事件，实时联动请用 OnKeyDown，或在命令执行时 读取 Value。

func NewTextInput(doc engine.HTMLDocument, value string) *TextInputView
    NewTextInput 创建单行输入框。

func (v *TextInputView) Element() engine.HTMLElement

func (v *TextInputView) OnKeyDown(f func(*engine.KeyEvent) bool)
    OnKeyDown 注册按键处理器；返回 true 表示消费。

func (v *TextInputView) SetEnabled(on bool)
    SetEnabled 设置可用态。

func (v *TextInputView) SetValue(value string)
    SetValue 设置值，光标移到末尾。

func (v *TextInputView) Value() string
    Value 返回当前值。

type TextView struct {
	// Has unexported fields.
}
    TextView 是只读多行文本视图，底层是 div。

    它不参与焦点和编辑；需要可编辑多行输入时用 TextAreaView。TextView 用 white-space:pre
    保留换行，高度由主题给默认值，应用可再改内联样式。

func NewTextView(doc engine.HTMLDocument, text string) *TextView
    NewTextView 创建只读多行文本视图。

func (v *TextView) Element() engine.HTMLElement

func (v *TextView) SetText(text string)
    SetText 更新文本内容。

type Theme struct {
	Font     string // font-family 原值
	IconFont string // 图标字体（默认 Segoe MDL2 Assets，见 icon.go）

	Bg           string // 画布底色（卡片之外的区域）
	Surface      string // 面板/栏/浮层底色
	SurfaceAlt   string // 工具栏底色
	Hover        string // 悬停底色
	Press        string // 按下底色
	Border       string // 分隔线与描边
	BorderStrong string // 需要更清晰边界的地方（浮层、输入框、工具栏分隔线）
	Text         string // 主文字
	Muted        string // 次要文字/禁用态

	// 强调色（Primary）：主按钮、选中态、链接、焦点。字段名沿用 Accent
	// （旧 API），语义上它就是 Primary；Default（默认/中性）不单独存
	// 令牌，复用 Surface/Border/Text/SurfaceAlt/Press——同一组中性色
	// 只留一份，避免两处值漂移。
	Accent       string // 强调色实底（主按钮、选中态）
	AccentHover  string // 强调色悬停/按下
	OnAccent     string // 强调色实底上的文字/图标（对比度由主题负责）
	AccentSoft   string // 强调色淡底（悬停/徽标/提示）
	AccentBorder string // 强调色淡底描边
	AccentText   string // 强调色淡底上的文字

	// 语义色：每组都是「实底 + 悬停 + On（实底文字）+ 淡底 + 描边 +
	// 淡底文字」。按钮用实底/On，徽标与提示用淡底/描边/文字。
	// 浅色主题里 Warning 的 On 用深色（琥珀底压白字不够清楚）；
	// 深色主题的实底更亮，On 统一用同色系深色。
	Success       string
	SuccessHover  string
	OnSuccess     string
	SuccessSoft   string
	SuccessBorder string
	SuccessText   string

	Warning       string
	WarningHover  string
	OnWarning     string
	WarningSoft   string
	WarningBorder string
	WarningText   string

	Danger       string
	DangerHover  string
	OnDanger     string
	DangerSoft   string
	DangerBorder string
	DangerText   string

	Info       string
	InfoHover  string
	OnInfo     string
	InfoSoft   string
	InfoBorder string
	InfoText   string

	Radius           int // 面板/浮层圆角
	MenuHeight       int // 菜单栏高度
	ToolHeight       int // 工具栏高度
	StatusHeight     int // 状态栏高度
	StatusRightWidth int // 状态栏右侧消息预留宽度
	ButtonHeight     int // 按钮高度
	// ToolControlHeight 是工具栏内交互控件（图标/文字按钮、槽位里的输入框
	// 与下拉）的统一高度：条内所有控件同高、同用 vertical-align: middle
	// 对齐，才能彼此顶底齐平（默认 28，配 ToolHeight 36）。
	ToolControlHeight int
	// BarHeight 是控件条（NewBar）的高度。控件条是页面内容里的一行工具，
	// 应用要按它给自己的内容让位，所以尺寸得能从主题读到，不能只写在 CSS 里。
	BarHeight int
}
    Theme 是组件库的视觉令牌：一套配色 + 一组尺寸。

    为什么是「令牌 + 生成样式表」而不是让应用写 CSS：
      - 引擎的 CSS 是子集（没有 flex/grid），工作台要用的技巧（占位块、 1px 分隔线、绝对定位浮层）容易写错；
      - 主题是一处改、全部一致的：应用只写结构，不碰颜色与间距；
      - 样式表由 Install 追加在作者样式之后，同特异度时工作台胜出，页面 自带的 body/div 规则不会把菜单栏挤歪。

    零值字段由 withDefaults 用 Light() 补齐，应用只需改想改的几项。

func Dark() Theme
    Dark 返回默认深色主题。

    从 Light() 拷贝后只覆盖颜色：尺寸/字体不重复写，以后新增令牌也不会 出现「浅色补了、深色忘了」。配色取蓝灰系（slate），与浅色主题同一套
    色相，切换时只有明暗变化，不会有另一种风格。

    强调色比浅色主题更亮（#60A5FA）：深底上 #2563EB 的蓝字会发闷，菜单 高亮和图标都看不清。代价是白字压不住它，所以 OnAccent
    反过来用深色 ——主按钮的对比度由主题负责，而不是在 CSS 里写死白色。

    其余语义色沿用同一规律：实底取更亮的一档（400/500），On 用同色系的 深色；淡底取深色画布上的低饱和色，淡底文字取 300 档。浅色主题则相反：
    实底取 600/700，On 用白色（Warning 例外，深色字更清楚）。

func Light() Theme
    Light 返回默认浅色主题。

    其它浅色配色可以在这套令牌上改字段；深色见 Dark。两套主题的尺寸、 字体完全一致，只换颜色——工作台几何不会因为换皮而分叉。

type Workbench struct {
	// Has unexported fields.
}
    Workbench（工作台）是一套装在某个文档上的周边构件：内容四周的菜单栏、 工具栏、状态栏，加上主题样式与快捷键表。

    生命周期与文档绑定：换文档要重新 Install（App 通过 browser.Window.SetOnDocument
    自动做这件事）。Workbench 不持有窗口， 所以无头环境（ui.Install + DumpSVG/PNG）也能完整测试。

func Install(doc engine.HTMLDocument, spec Spec) *Workbench
    Install 把工作台装进一个已打开的文档，返回可继续操作的 Workbench。

    做四件事：
     1. 追加主题样式表（在作者样式之后，同特异度时工作台胜出）；
     2. 在 body 首尾插占位块——fixed 的栏不占文档流，页面内容要让出空间；
     3. 用当前 Spec 生成菜单栏/工具栏/状态栏；
     4. 绑定快捷键与「点别处关菜单」。

    doc 为 nil 时返回 nil（调用方不必判空文档）。

func (s *Workbench) CloseMenus()
    CloseMenus 收起所有菜单。

func (s *Workbench) Doc() engine.HTMLDocument
    Doc 返回工作台所在的文档。

func (s *Workbench) HandleKey(key int, mods uint16) bool
    HandleKey 处理应用级按键：返回 true 表示已消费。

    由 App 挂在 browser.Window.SetKeyHandler 上，位置在 Ctrl+C/V 与页面 之前——输入框聚焦时 Ctrl+S
    也必须先到应用。

func (s *Workbench) OpenMenu(i int)
    OpenMenu 打开第 i 个菜单（越界忽略）。

    每次打开都按当前状态重建浮层：菜单项数量少，重建比「先建好再逐个 更新禁用态」简单，也保证可用性是打开那一刻的最新值。

func (s *Workbench) Refresh()
    Refresh 重算工具栏按钮的可用态。

    只在命令执行后（以及安装时）调用：可用性不是每帧都变的量，轮询 只会让主循环每帧重排。菜单项在打开菜单时现算，不需要刷。

func (s *Workbench) SetStatus(text string)
    SetStatus 更新左侧文本。

    兼容旧 API：有 Key 为 "status" 的段时更新它；没有分段时创建一个； 已有自定义分段但没有 "status"
    时更新最左一段。新代码建议直接用 SetStatusSegment。

func (s *Workbench) SetStatusRight(text string)
    SetStatusRight 更新状态栏右侧文本（提示/计数/快捷键）。

    右侧消息固定宽度、右对齐；有消息时左侧轨道自动预留 Theme.StatusRightWidth，没有消息时收起预留。

func (s *Workbench) SetStatusSegment(key, text string) bool
    SetStatusSegment 按 Key 更新一段文字；找不到 Key 时返回 false。

    只改文本，不重建段结构：适合光标位置、字符数这类高频更新。

func (s *Workbench) SetStatusSegments(segs []StatusSegment)
    SetStatusSegments 重建左侧全部分段。

    结构变化（增删段、改宽度/对齐/隐藏）走这里；只改文字用 SetStatusSegment，避免每次光标移动都重建 DOM。

func (s *Workbench) SetStatusVisible(visible bool)
    SetStatusVisible 显示或隐藏状态栏，同时收起/恢复底部占位。

    应用可以把它接到「视图 → 显示状态栏」这类命令上；隐藏时底部占位一起 收起，页面内容可以使用腾出的高度，不会被固定栏挡住。

func (s *Workbench) SetTheme(theme Theme)
    SetTheme 在不重建菜单/工具栏/页面的前提下换一套主题。

    新样式表追加在旧样式之后，同特异度时后写胜出；Workbench 的尺寸跟着新主题 重算，浅色/深色（尺寸相同）切换时只有配色变化。旧样式表不会被移除，
    反复切换会让样式表增长，适合交互式切换而不是高频轮换。

func (s *Workbench) StatusSegment(key string) (string, bool)
    StatusSegment 返回一段当前文字；没有该 Key 时返回 "", false。

````
