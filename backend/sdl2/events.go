// 本文件是平台事件的公开模型（无构建标签：类型本身与平台无关，
// 各平台的解码在 window.go / stub_other.go 里）。

package sdl2

// EventKind 是平台事件的种类。
//
// 事件是「平台层的事实」，不是「引擎的动作」：鼠标移动、按键、窗口
// 尺寸变化由后端解码，具体怎么解释成引擎行为（点击、选区、滚动）由
// 上一层做。这样后端不认识引擎，依赖方向保持单向。
type EventKind uint8

// 平台事件种类。
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

// Event 是一个平台事件。
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

// 修饰键位掩码（与 SDL 的 KMOD_* 一致）。
//
// 事件的 Mod 字段是平台掩码，含义由平台层定义：上层要靠它区分
// 「Ctrl+V 组合键」与「真在打字」。
const (
	KmodShift = 0x0003 // 左右 Shift 合并的位
	KmodCtrl  = 0x00C0 // 左右 Ctrl 合并的位
	KmodAlt   = 0x0300 // 左右 Alt 合并的位
)

// 鼠标按键号（与 SDL 的 SDL_BUTTON_* 一致）。
const (
	MouseButtonLeft   = 1
	MouseButtonMiddle = 2
	MouseButtonRight  = 3
)
