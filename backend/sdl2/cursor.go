package sdl2

import "github.com/go-comm/ghromex/core"

// 系统鼠标指针（CSS cursor 的可用子集）。
//
// SDL2 的 SDL_CreateSystemCursor 按 SDL_SystemCursor 枚举创建平台光标，
// SDL_SetCursor 把它设为当前指针。引擎侧（browser 层）按命中结果给出
// core.CursorShape，这里只负责映射到 SDL 枚举并缓存句柄——每次鼠标移动都
// 创建光标会泄漏，故按枚举值复用。
//
// SDL2 没有 help/grab/grabbing 的专用形状：help 退回箭头，grab/grabbing
// 用手型（与大多数桌面程序一致）。符号缺失时 SetCursor 返回 false，
// 指针形状纯属交互装饰，不影响渲染与命中。

// systemCursorID 把引擎的光标形状映射为 SDL_SystemCursor 枚举值。
func systemCursorID(shape core.CursorShape) uintptr {
	switch shape {
	case core.CursorShapeText:
		return sysCursorIBeam
	case core.CursorShapeWait:
		return sysCursorWait
	case core.CursorShapeCrosshair:
		return sysCursorCrosshair
	case core.CursorShapeNWSEResize:
		return sysCursorSizeNWSE
	case core.CursorShapeNESWResize:
		return sysCursorSizeNESW
	case core.CursorShapeEWResize:
		return sysCursorSizeWE
	case core.CursorShapeNSResize:
		return sysCursorSizeNS
	case core.CursorShapeMove:
		return sysCursorSizeAll
	case core.CursorShapeNotAllowed:
		return sysCursorNo
	case core.CursorShapePointer, core.CursorShapeGrab, core.CursorShapeGrabbing:
		return sysCursorHand
	default:
		// auto/default/help 与未知形状：系统箭头。
		return sysCursorArrow
	}
}

// SetCursor 实现 core.CursorSetter：把系统指针切换为指定形状。
func (g *Graphics) SetCursor(shape core.CursorShape) bool {
	if g == nil || !hasCreateSystemCursor || !hasSetCursor {
		return false
	}
	id := systemCursorID(shape)
	h, ok := g.cursors[id]
	if !ok {
		h = sdlCreateSystemCursor(id)
		if h == 0 {
			return false
		}
		if g.cursors == nil {
			g.cursors = make(map[uintptr]uintptr)
		}
		g.cursors[id] = h
	}
	sdlSetCursor(h)
	return true
}

// 编译期断言：SDL 后端必须实现 core.CursorSetter，否则 cursor 样式静默失效。
var _ core.CursorSetter = (*Graphics)(nil)
