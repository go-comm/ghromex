// Package screen 是桌面抓屏的平台能力。
//
// 渲染后端（SDL）只能回读**自己窗口**的内容；截图工具要把整个桌面冻结
// 下来给用户拖选，必须走操作系统接口。Windows 上用 GDI 的 BitBlt，纯
// Win32 调用、无 CGO；其它平台返回 ErrUnsupported。
//
// 与 runtime/clipboard、runtime/dialog 的定位一致：平台差异收在内部，
// 调用方只看到一个返回 image.RGBA 的函数。
package screen

import (
	"errors"
	"image"
)

// ErrUnsupported 表示当前平台没有桌面抓屏实现。
var ErrUnsupported = errors.New("ghromex 桌面抓屏仅支持 Windows")

// Capture 抓取主屏，返回 1:1 的 RGBA 图像，原点为 (0,0)。
//
// 返回的像素 alpha 恒为 255。原因：GDI 的 BitBlt 不写 alpha 字节（32bpp
// 下通常是 0），而渲染层的 DrawSprite 走 alpha 混合——不补 255 的话整张
// 截图会被当成“全透明”画不出来。
//
// 只抓主屏：多显示器虚拟桌面（负坐标、每显示器不同缩放）需要另做坐标
// 映射，属于后续能力。
func Capture() (*image.RGBA, error) {
	return capture()
}
