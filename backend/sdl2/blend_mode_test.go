package sdl2

import (
	"testing"

	"github.com/go-comm/ghromex/core"
)

// 混合模式契约（确定性断言：不碰像素、不碰时间）：
//
//   - 不透明填充**期间**是 NONE —— 直接写像素，软件光栅器快 10 倍
//     （2.1ns/像素 → 0.2ns/像素；门户页一帧填 1700 万像素，2560×1440
//     单帧 37ms → 4.4ms）。
//   - 函数返回后**复原 BLEND** —— 混合模式是渲染器的全局状态，贴图拷贝
//     （精灵、文字的抗锯齿边缘）要靠混合；SDL 各后端对「拷贝用纹理的还是
//     渲染器的混合模式」处理不一致，全局保持 BLEND 最稳。
//   - 半透明填充**全程 BLEND** —— 逐像素混合，不能省。
//
// 价值在于：将来有人把 defer 删了、或新增一条不透明路径忘了切，测试会红，
// 而不是等到某个平台的精灵变半透明才发现。
func TestDrawColorBlendModeContract(t *testing.T) {
	origBlend := sdlSetRenderDrawBlendMode
	origColor := sdlSetRenderDrawColor
	origFill := sdlRenderFillRect
	defer func() {
		sdlSetRenderDrawBlendMode = origBlend
		sdlSetRenderDrawColor = origColor
		sdlRenderFillRect = origFill
	}()

	var mode uintptr = blendModeBlend // 渲染器当前混合模式
	var modeAtFill uintptr            // 填充发生那一刻的模式
	fills := 0
	sdlSetRenderDrawBlendMode = func(args ...uintptr) uintptr {
		mode = args[1]
		return 0
	}
	sdlSetRenderDrawColor = func(args ...uintptr) uintptr { return 0 }
	sdlRenderFillRect = func(args ...uintptr) uintptr {
		fills++
		modeAtFill = mode
		return 0
	}

	g := NewGraphics(0, "")

	// 不透明填充：期间 NONE，返回后 BLEND。
	mode, fills, modeAtFill = blendModeBlend, 0, 0
	g.DrawColor(0, 0, 10, 10, core.NewColor(255, 0, 0, 255))
	if fills != 1 {
		t.Fatalf("不透明填充的填充次数 = %d, want 1", fills)
	}
	if modeAtFill != blendModeNone {
		t.Errorf("不透明填充**期间**的混合模式 = %d, want %d（NONE：直接写像素）",
			modeAtFill, blendModeNone)
	}
	if mode != blendModeBlend {
		t.Errorf("不透明填充返回后的混合模式 = %d, want %d（BLEND：贴图拷贝要混合）",
			mode, blendModeBlend)
	}

	// 半透明填充：全程 BLEND。
	mode, fills, modeAtFill = blendModeBlend, 0, 0
	g.DrawColor(0, 0, 10, 10, core.NewColor(255, 0, 0, 128))
	if fills != 1 {
		t.Fatalf("半透明填充的填充次数 = %d, want 1", fills)
	}
	if modeAtFill != blendModeBlend {
		t.Errorf("半透明填充期间的混合模式 = %d, want %d（BLEND：逐像素混合）",
			modeAtFill, blendModeBlend)
	}
	if mode != blendModeBlend {
		t.Errorf("半透明填充返回后的混合模式 = %d, want %d", mode, blendModeBlend)
	}
}
