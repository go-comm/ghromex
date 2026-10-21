package browser

import "github.com/go-comm/ghromex/backend/sdl2"

// ErrUnsupported 表示当前平台没有可用的渲染后端，等价于 sdl2.ErrUnsupported。
//
// 平台差异收在 backend/sdl2：非 Windows 下 New 会返回它，Windows 上不会
// 出现。这里保留别名，调用方不必为了判断平台而直接依赖后端包。
var ErrUnsupported = sdl2.ErrUnsupported
