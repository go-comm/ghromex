package sdl2

import "errors"

// ErrUnsupported 表示当前平台没有可用的 SDL2 后端。
//
// 只有非 Windows 平台会返回它（NewWindow/OpenDocument/Run/Screenshot）；
// Windows 上加载 SDL2.dll 成功与否会给出各自的错误。放在无构建标签的
// 文件里，跨平台代码才能引用它并用 errors.Is 判断。
var ErrUnsupported = errors.New("ghromex SDL2 后端仅支持 Windows")
