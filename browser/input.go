package browser

import (
	"github.com/go-comm/ghromex/backend/sdl2"
)

// 本文件是「平台无关输入注入」的入口：调试服务、外部脚本、宿主 worker
// 想往窗口注入点击/按键/滚轮时，只面对 InputEvent，不用知道事件字节
// 布局，也不用 import backend/sdl2。
//
// 为什么放在 browser：输入注入的终点是 SDL_PushEvent（平台能力），但
// 「注入什么动作」是应用层概念。browser 本来就 import backend/sdl2，
// 由它把 InputEvent 翻译成 sdl2.Event 最合适——依赖方向保持单向，
// 上层（debug、示例、宿主）只依赖 browser。

// InputKind 是输入事件的种类。
type InputKind uint8

const (
	// InputMouseMove 鼠标移动（更新 :hover 与光标形状）。
	InputMouseMove InputKind = iota
	// InputMouseDown 鼠标按下。
	InputMouseDown
	// InputMouseUp 鼠标抬起。
	InputMouseUp
	// InputWheel 滚轮（WheelX/WheelY 为档位）。
	InputWheel
	// InputKeyDown 按键按下（Key + Mods）。
	InputKeyDown
	// InputTextInput 文本输入（IME 上屏，UTF-8）。
	InputTextInput
	// InputFilesDropped 文件拖入窗口（Files 为整批绝对路径）。
	InputFilesDropped
)

// InputEvent 是一个平台无关的输入事件（注入用）。
//
// 它只描述「发生了什么」，不携带任何平台特定编码（SDL 键码位布局、
// KMOD 掩码的原始值都不暴露）：这些细节由 browser 在 toSDL 里翻译。
// 注入是**异步入队**的——事件在主循环下一帧才被消费；需要「上一个动作
// 已落地」的确定性时，投一个空任务等它跑完（见 tasks.go 的同步栅栏
// 说明），不要靠 sleep 猜。
type InputEvent struct {
	// Kind 事件种类。
	Kind InputKind
	// X/Y 是视口像素坐标（1 CSS px = 1 物理像素）。鼠标/滚轮事件必填；
	// 键盘/文本/拖放事件无意义，给 0 即可。
	X, Y int
	// Button 是鼠标按键号（仅 down/up 有效）：MouseButtonLeft/Middle/Right。
	Button int
	// WheelX/WheelY 是滚轮**档位**（仅 wheel 有效），SDL 口径：正 = 向上/
	// 向右（滚动偏移减小、露出更靠上/更靠左的内容）；browser 主循环再按
	// 每档 100px 换算给引擎。
	WheelX, WheelY int
	// Key 是键码（仅 keydown 有效）：字母/数字/空格用 ASCII（如 'a'、' '），
	// 控制键用本文件导出的 KeyEnter/KeyTab/.../KeyF1~F12。
	Key int
	// Mods 是修饰键掩码（仅 keydown 有效）：ModShift/ModCtrl/ModAlt 位或。
	Mods uint16
	// Text 是文本输入内容（仅 textinput 有效），UTF-8（中文走 IME 组合）。
	Text string
	// Files 是本批拖入窗口的绝对路径（仅 filesdropped 有效），多选不拆批。
	Files []string
}

// 鼠标按键号（与 SDL 的 SDL_BUTTON_* 一致：1 左、2 中、3 右）。
const (
	MouseButtonLeft   = 1
	MouseButtonMiddle = 2
	MouseButtonRight  = 3
)

// 修饰键掩码（与引擎 text_input.go 的 ModShift/ModCtrl/ModAlt 同值：
// 这里再导一份是免得「调 browser 注入还得 import engine 拿掩码」——
// 两边都是平台掩码的透传，值必须一致，改的时候对一对引擎那份）。
const (
	ModShift = 0x0003 // 左右 Shift 合并的位
	ModCtrl  = 0x00C0 // 左右 Ctrl 合并的位
	ModAlt   = 0x0300 // 左右 Alt 合并的位
)

// 常用键码。ASCII 区与键盘字符一一对应（SDL 的 SDLK_a 就是 'a'），直接
// 写字面量即可（'a'、'5'、' '）；下面是扫描码区/特殊键，与引擎
// text_input.go 的 Key* 常量**同值**（同一套 SDL 编码，浏览器按键解码
// 认的也是它们）。
const (
	KeyEnter     = 0x0D // '\r'
	KeyTab       = 0x09 // '\t'
	KeyEscape    = 0x1B
	KeyBackspace = 0x08
	KeyDelete    = 0x7F       // SDLK_DELETE（ASCII DEL，不是扫描码区）
	KeyHome      = 0x4000004A // SDL_SCANCODE_HOME   (74)
	KeyEnd       = 0x4000004D // SDL_SCANCODE_END    (77)
	KeyLeft      = 0x40000050 // SDL_SCANCODE_LEFT   (80)
	KeyRight     = 0x4000004F // SDL_SCANCODE_RIGHT  (79)
	KeyDown      = 0x40000051 // SDL_SCANCODE_DOWN   (81)
	KeyUp        = 0x40000052 // SDL_SCANCODE_UP     (82)
	KeyPageUp    = 0x4000004B // SDL_SCANCODE_PAGEUP (75)
	KeyPageDown  = 0x4000004E // SDL_SCANCODE_PAGEDOWN (78)
	KeyInsert    = 0x40000049 // SDL_SCANCODE_INSERT (73)
	// F1~F12：键码 = 0x40000000 | SDL_SCANCODE_Fx（F1 = 58）。
	KeyF1  = 0x4000003A
	KeyF2  = 0x4000003B
	KeyF3  = 0x4000003C
	KeyF4  = 0x4000003D
	KeyF5  = 0x4000003E
	KeyF6  = 0x4000003F
	KeyF7  = 0x40000040
	KeyF8  = 0x40000041
	KeyF9  = 0x40000042
	KeyF10 = 0x40000043
	KeyF11 = 0x40000044
	KeyF12 = 0x40000045
)

// InjectInput 把一个平台无关的输入事件注入窗口。
//
// 内部翻译成 sdl2.Event 后经 SDL_PushEvent 推送，与真人操作走同一条
// 解码链路（坐标修正、焦点链、悬停档位这些真机行为都能被复现）。注入是
// **异步**的：事件在主循环下一帧才被消费——主循环没在跑时没有消费者，
// 返回 ErrLoopNotRunning（不假装成功）。
//
// 与 InjectEvent 的关系：那是平台事件（sdl2.Event）的直通口，后端测试
// 手搓原始事件时用；应用层/脚本请用本方法，不必碰平台事件模型。
func (win *Window) InjectInput(ev InputEvent) error {
	if win == nil || win.sdl == nil {
		return ErrLoopNotRunning
	}
	win.sdl.InjectEvent(ev.toSDL())
	return nil
}

// toSDL 把 InputEvent 翻译成 sdl2.Event（字段一一对应，无隐藏语义）。
func (ev InputEvent) toSDL() sdl2.Event {
	var kind sdl2.EventKind
	switch ev.Kind {
	case InputMouseMove:
		kind = sdl2.EventMouseMotion
	case InputMouseDown:
		kind = sdl2.EventMouseButtonDown
	case InputMouseUp:
		kind = sdl2.EventMouseButtonUp
	case InputWheel:
		kind = sdl2.EventMouseWheel
	case InputKeyDown:
		kind = sdl2.EventKeyDown
	case InputTextInput:
		kind = sdl2.EventTextInput
	case InputFilesDropped:
		kind = sdl2.EventFilesDropped
	}
	return sdl2.Event{
		Kind:   kind,
		X:      ev.X,
		Y:      ev.Y,
		Button: uint8(ev.Button),
		WheelX: ev.WheelX,
		WheelY: ev.WheelY,
		Key:    ev.Key,
		Mod:    ev.Mods,
		Text:   ev.Text,
		Files:  ev.Files,
	}
}
