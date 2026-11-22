package sdl2

import "github.com/go-comm/ghromex/core"

// WindowOptions 是建窗参数。零值表示位于 (0,0) 的固定尺寸普通窗口（带边框），
// 与 NewWindow 的默认值不同；截图工具的悬浮窗显式关掉边框/缩放并置顶。
//
// 放在无构建标签的文件里：Windows 实现与非 Windows 空实现共用同一份
// 字段定义，新增字段时两边不会漂移。
type WindowOptions struct {
	Width, Height int
	Title         string

	// Icon 是窗口图标（标题栏 + 任务栏 + Alt+Tab），非 nil 时在窗口建好、
	// 首帧显示之前设上——否则会先看到一眼 SDL 默认图标。非 Windows 平台
	// 忽略（NewWindowWithOptions 本来就返回 ErrUnsupported）。
	//
	// 运行期想换图标用 Window.SetIcon，同一份 core.Bitmap 口径。
	Icon *core.Bitmap

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
