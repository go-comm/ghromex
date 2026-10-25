package sdl2

import (
	"fmt"
	"os"
	"strings"
)

// envBool 读环境变量并按布尔解析：1/true/yes/on 为真，0/false/no/off/空为假。
//
// 环境变量是"逃生口"式的开关：值写错时应当回到默认行为（默认内存渲染、
// 关调试），而不是悄悄改变渲染路径。早期实现只判"非空"，于是
// GHROMEX_SOFTWARE=0 也会强制软件渲染——与意图正好相反，而且从输出里只看
// 得到 renderer=software，看不出是谁强制的。
func envBool(name string) bool {
	return boolOf(os.Getenv(name))
}

// boolOf 是 envBool 的解析本体（值级入口，方便单测与按值取参的 planRenderer）。
func boolOf(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// planRenderer 按环境变量决定渲染器路径，返回 (是否尝试加速, 强制软件)。
//
// 默认**内存渲染**（SDL 软件渲染器，像素全部落在系统内存的窗口表面）：
// 想走 GPU 加速（accelerated + vsync）必须显式 GHROMEX_GPU=1。两个强制
// 软件的口子优先级最高、且互相独立：
//   - GHROMEX_SOFTWARE=1 逃生口——部分虚拟显示驱动能创建 accelerated 渲染器
//     却"窗口能开、像素不出"，自动回退不会触发，只能硬关；
//   - SDL_VIDEO_DRIVER=dummy 无头自检——要保持确定性回读行为，不尝试 GPU。
//
// 强制原因统一折成 "GHROMEX_SOFTWARE/dummy" 这个既有口径（诊断输出与文档
// 都认它）；wantGPU 在强制时恒 false（GHROMEX_GPU=1 也压不过强制口）。
func planRenderer(gpu, software, driver string) (wantGPU bool, forced bool) {
	forced = boolOf(software) || driver == "dummy"
	return boolOf(gpu) && !forced, forced
}

// rendererDescFor 组渲染器路径的描述（RendererInfo / GHROMEX_DEBUG / bench
// 归因用）。四种情形要能一眼分开：加速（显式开启）、强制软件（配置）、
// GPU 创建失败回退（环境）、默认内存渲染（无事发生）——排查方向各不相同。
func rendererDescFor(gpu, forced, gpuFailed bool, driver string) string {
	switch {
	case gpu:
		return fmt.Sprintf("accelerated(GPU, driver=%s; GHROMEX_GPU=1)", driver)
	case forced:
		return fmt.Sprintf("software (driver=%s; 强制软件：GHROMEX_SOFTWARE/dummy)", driver)
	case gpuFailed:
		return fmt.Sprintf("software (driver=%s; GPU 创建失败已回退)", driver)
	default:
		return fmt.Sprintf("software (driver=%s; 默认内存渲染)", driver)
	}
}
