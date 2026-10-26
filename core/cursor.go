package core

// CursorShape 是系统鼠标指针的形状（CSS cursor 的可用子集）。
//
// 放在 core 而不是 engine：形状由浏览器层按命中结果决定、由平台后端
// （SDL2）落到真实指针上，两边都只依赖 core，不互相依赖。
type CursorShape uint8

// 指针形状。默认值（0）是系统箭头，对应 CSS 的 auto/default。
const (
	CursorShapeDefault    CursorShape = iota
	CursorShapeText                   // I 形文本光标（CSS text）
	CursorShapePointer                // 手型（CSS pointer）
	CursorShapeMove                   // 四向移动（CSS move）
	CursorShapeNotAllowed             // 禁止（CSS not-allowed）
	CursorShapeWait                   // 等待（CSS wait）
	CursorShapeHelp                   // 帮助（CSS help）
	CursorShapeCrosshair              // 十字（CSS crosshair）
	CursorShapeGrab                   // 抓取（CSS grab）
	CursorShapeGrabbing               // 抓取中（CSS grabbing）
	CursorShapeEWResize               // 水平缩放（CSS e-resize/w-resize/ew-resize）
	CursorShapeNSResize               // 垂直缩放（CSS n-resize/s-resize/ns-resize）
	CursorShapeNESWResize             // 左下-右上缩放（CSS ne-resize/sw-resize）
	CursorShapeNWSEResize             // 左上-右下缩放（CSS nw-resize/se-resize）
)

// CursorSetter 是 Graphics 的可选扩展：把系统鼠标指针切换为指定形状。
//
// 不实现该接口的后端（无头、软件光栅化、非 Windows 平台）静默忽略——
// 指针形状纯属交互装饰，缺它不影响渲染与命中。返回 false 表示该形状
// 在本平台不可用（调用方无需重试）。
type CursorSetter interface {
	SetCursor(shape CursorShape) bool
}
