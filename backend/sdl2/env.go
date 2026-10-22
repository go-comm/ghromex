package sdl2

import (
	"os"
	"strings"
)

// envBool 读环境变量并按布尔解析：1/true/yes/on 为真，0/false/no/off/空为假。
//
// 环境变量是"逃生口"式的开关：值写错时应当回到默认行为（自动选渲染器、
// 关调试），而不是悄悄改变渲染路径。早期实现只判"非空"，于是
// GHROMEX_SOFTWARE=0 也会强制软件渲染——与意图正好相反，而且从输出里只看
// 得到 renderer=software，看不出是谁强制的。
func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
