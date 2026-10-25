package sdl2

import (
	"testing"
)

// 环境变量按布尔解析：只有明确为"真"的值才开；写错、0、空一律回默认。
//
// 曾经的坑：GHROMEX_SOFTWARE 只判"非空"，于是 GHROMEX_SOFTWARE=0 也强制
// 软件渲染——用户明明要求走 GPU，却拿到软件路径且输出里看不出原因。
func TestEnvBool(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
		{"off", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{" 1 ", true},
		{"maybe", false}, // 无法识别 → 回默认（假）
	}
	for _, c := range cases {
		t.Setenv("GHROMEX_TEST_FLAG", c.val)
		if got := envBool("GHROMEX_TEST_FLAG"); got != c.want {
			t.Errorf("envBool(%q) = %v, want %v", c.val, got, c.want)
		}
	}
}

// 渲染器路径选择：默认内存渲染（软件），GHROMEX_GPU=1 才尝试加速，
// GHROMEX_SOFTWARE=1 / dummy 驱动两个强制口优先级最高（GHROMEX_GPU=1 也
// 压不过）——这是行为护栏，改默认或改强制顺序都会在这里显形。
func TestPlanRenderer(t *testing.T) {
	cases := []struct {
		name                  string
		gpu, software, driver string
		wantGPU, wantForced   bool
	}{
		{"默认（全空）", "", "", "", false, false},
		{"GPU=0 按关", "0", "", "", false, false},
		{"GPU=1 显式开", "1", "", "", true, false},
		{"GPU=1 + SOFTWARE=1 强制压过", "1", "1", "", false, true},
		{"GPU=1 + dummy 强制压过", "1", "", "dummy", false, true},
		{"SOFTWARE=1（与旧口径一致）", "", "1", "", false, true},
		{"dummy 无头", "", "", "dummy", false, true},
		{"非 dummy 的其它驱动", "", "", "windows", false, false},
	}
	for _, c := range cases {
		gotGPU, gotForced := planRenderer(c.gpu, c.software, c.driver)
		if gotGPU != c.wantGPU || gotForced != c.wantForced {
			t.Errorf("%s: planRenderer(%q,%q,%q) = (%v,%v), want (%v,%v)",
				c.name, c.gpu, c.software, c.driver,
				gotGPU, gotForced, c.wantGPU, c.wantForced)
		}
	}
}

// 渲染路径描述的四种归因必须互不混淆：bench/GHROMEX_DEBUG 全靠它定位
// 「数字是哪条路径量出来的」。
func TestRendererDescFor(t *testing.T) {
	cases := []struct {
		gpu, forced, failed bool
		driver              string
		want                string
	}{
		{false, false, false, "software",
			"software (driver=software; 默认内存渲染)"},
		{true, false, false, "direct3d11",
			"accelerated(GPU, driver=direct3d11; GHROMEX_GPU=1)"},
		{false, true, false, "software",
			"software (driver=software; 强制软件：GHROMEX_SOFTWARE/dummy)"},
		{false, false, true, "llvmpipe",
			"software (driver=llvmpipe; GPU 创建失败已回退)"},
	}
	for _, c := range cases {
		got := rendererDescFor(c.gpu, c.forced, c.failed, c.driver)
		if got != c.want {
			t.Errorf("rendererDescFor(%v,%v,%v,%q) = %q, want %q",
				c.gpu, c.forced, c.failed, c.driver, got, c.want)
		}
	}
}
