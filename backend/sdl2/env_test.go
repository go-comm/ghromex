package sdl2

import "testing"

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
