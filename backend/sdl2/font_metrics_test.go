//go:build windows

package sdl2

import (
	"os"
	"testing"
)

// 字体度量的 descent 必须为正。
//
// TTF_FontDescent 返回的是**负数**（int32），而 nativeCall 统一返回 uintptr：
// 不按 int32 重解释，-3 会变成 4294967293，取负后仍为负 → 被钳成 0。
// 后果是所有字体的 descent 恒为 0、行盒比 Chrome 短 2~3px（整页行距偏紧，
// 定高按钮里居中的文字也偏低）。这里锁住符号处理。
func TestFontDescentIsPositive(t *testing.T) {
	os.Setenv("SDL_VIDEO_DRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEO_DRIVER")

	win, err := NewWindow(64, 64, "metrics")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer win.Close()
	g := win.g

	for _, c := range []struct {
		family string
		size   int
		bold   bool
	}{
		{"Arial", 13, false},
		{"system-ui", 13, true},
	} {
		asc, desc := g.FontMetrics("M", c.size, c.bold, c.family)
		if asc <= 0 {
			t.Errorf("%s %dpx: ascent = %d，want > 0", c.family, c.size, asc)
		}
		if desc <= 0 {
			t.Errorf("%s %dpx: descent = %d，want > 0（TTF_FontDescent 的负值没按 int32 重解释）",
				c.family, c.size, desc)
		}
	}
}
