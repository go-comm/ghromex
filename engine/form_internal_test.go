package engine

import "testing"

// 白盒测试：折行、取值、选择器声明与命中规则等纯逻辑分支。

func TestWrapTextareaRules(t *testing.T) {
	g := NewFakeGraphics() // 13px 口径：ASCII 7px、空格 6px、CJK 10px
	const px = 10          // → ASCII 6px/字、空格 5px

	cases := []struct {
		name   string
		value  string
		maxW   int
		wants  []string
		starts []int
		ends   []int
	}{
		{name: "空值也给一行", value: "", maxW: 30,
			wants: []string{""}, starts: []int{0}, ends: []int{0}},
		{name: "不超宽单行", value: "abc", maxW: 30,
			wants: []string{"abc"}, starts: []int{0}, ends: []int{3}},
		{name: "无空格硬切", value: "abcdefg", maxW: 30,
			// 5 字 = 30px 满，第 6 字触发断行 → ["abcde","fg"]
			wants:  []string{"abcde", "fg"},
			starts: []int{0, 5},
			ends:   []int{5, 7}},
		{name: "空格回退断行", value: "aaaa bbbb", maxW: 30,
			// 第 5 字超出 → 回退到空格：["aaaa","bbbb"]，空格不落行首
			wants:  []string{"aaaa", "bbbb"},
			starts: []int{0, 5},
			ends:   []int{4, 9}},
		{name: "显式换行", value: "a\nb", maxW: 1000,
			// end 指向 '\n' 所在下标，下一行从其后开始
			wants:  []string{"a", "b"},
			starts: []int{0, 2},
			ends:   []int{1, 3}},
		{name: "值以换行结尾保留空行", value: "x\n", maxW: 1000,
			wants:  []string{"x", ""},
			starts: []int{0, 2},
			ends:   []int{1, 2}},
		{name: "单字超宽也不死循环", value: "汉字汉字", maxW: 3,
			// 每个 CJK 10px > 3px：len(cur)==0 时不截断，逐字成行
			wants:  []string{"汉", "字", "汉", "字"},
			starts: []int{0, 1, 2, 3},
			ends:   []int{1, 2, 3, 4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := wrapTextarea(g, c.value, c.maxW, px, false, "")
			if len(lines) != len(c.wants) {
				t.Fatalf("行数 = %d, want %d: %#v", len(lines), len(c.wants), lines)
			}
			for i, ln := range lines {
				if ln.text != c.wants[i] {
					t.Errorf("行[%d].text = %q, want %q", i, ln.text, c.wants[i])
				}
				if ln.start != c.starts[i] || ln.end != c.ends[i] {
					t.Errorf("行[%d] 区间 = [%d,%d), want [%d,%d)",
						i, ln.start, ln.end, c.starts[i], c.ends[i])
				}
			}
			// 区间必须连续无重叠（光标映射依赖该不变式）
			for i := 1; i < len(lines); i++ {
				if lines[i].start != lines[i-1].end+1 && lines[i].start != lines[i-1].end {
					// 同一逻辑行被折成两段时 start == 上一段 end（无 \n 消费）
					t.Errorf("行[%d].start=%d 与上一行 end=%d 不连续",
						i, lines[i].start, lines[i-1].end)
				}
			}
		})
	}
}

func TestCaretRowEdges(t *testing.T) {
	g := NewFakeGraphics()
	lines := wrapTextarea(g, "ab\ncd", 1000, 10, false, "")
	if len(lines) != 2 {
		t.Fatalf("行数 = %d, want 2", len(lines))
	}
	cases := []struct{ caret, want int }{
		{0, 0},  // 行首
		{1, 0},  // 行中
		{2, 0},  // '\n' 之前 → 仍在本行
		{3, 1},  // 换行后行首
		{5, 1},  // 末行末尾
		{99, 1}, // 越界 → 贴末行
		{-5, 0}, // 负值 → 首行
	}
	for _, c := range cases {
		if got := caretRow(lines, c.caret); got != c.want {
			t.Errorf("caretRow(%d) = %d, want %d", c.caret, got, c.want)
		}
	}
	if got := caretRow(nil, 0); got != 0 {
		t.Errorf("caretRow(nil) = %d, want 0", got)
	}
}

func TestOptionValueFallbacks(t *testing.T) {
	root := ParseFragment(`<select id="s" value="纯文本">
		<option value="b">Beta 文本</option>
		<option>纯文本</option>
		<option selected>选中文本</option>
	</select>`)
	sel := findTag(root, "select")
	if sel == nil {
		t.Fatal("select not found")
	}
	opts := selectOptions(sel)
	if len(opts) != 3 {
		t.Fatalf("option 数 = %d, want 3", len(opts))
	}
	// value 属性优先于文本
	if got := optionValue(opts[0]); got != "b" {
		t.Errorf("optionValue(value 属性) = %q, want b", got)
	}
	// 无 value 属性 → 文本（去空白）
	if got := optionValue(opts[1]); got != "纯文本" {
		t.Errorf("optionValue(纯文本 option) = %q, want 纯文本", got)
	}

	// value 属性匹配 → selected 属性 → 首个
	base := inner(sel)
	if got := selectedOptionBase(base); got != inner(opts[1]) {
		t.Errorf("value=纯文本 未匹配到「纯文本」项")
	}
	base.attrs["value"] = "b"
	if got := selectedOptionBase(base); got != inner(opts[0]) {
		t.Errorf("value=b 未匹配到 value 属性项")
	}
	delete(base.attrs, "value")
	if got := selectedOptionBase(base); got != inner(opts[2]) {
		t.Errorf("无 value 时未回落到 selected 属性项")
	}
	delete(inner(opts[2]).attrs, "selected")
	if got := selectedOptionBase(base); got != inner(opts[0]) {
		t.Errorf("无 value/selected 时未回落到首个 option")
	}

	// 空 select → nil
	if got := selectedOptionBase(inner(ParseFragment(`<select id="e"></select>`))); got != nil {
		t.Errorf("无 option 的 select 应返回 nil, got %v", got)
	}
}

func TestApplyDeclTextDecoration(t *testing.T) {
	c := newStyle()
	applyDecl(c, "text-decoration", "underline")
	if c.TextDecoration() != TextDecorationUnderline {
		t.Errorf("underline 解析 = %v, want underline", c.TextDecoration())
	}
	applyDecl(c, "text-decoration", "Line-Through") // 大小写不敏感
	if c.TextDecoration() != TextDecorationLineThrough {
		t.Errorf("line-through 解析 = %v", c.TextDecoration())
	}
	applyDecl(c, "text-decoration", "underline line-through") // 取首个可识别
	if c.TextDecoration() != TextDecorationUnderline {
		t.Errorf("多值取首个 = %v, want underline", c.TextDecoration())
	}
	applyDecl(c, "text-decoration", "none")
	if c.TextDecoration() != TextDecorationNone {
		t.Errorf("none 解析 = %v, want none", c.TextDecoration())
	}
	// 未知值（overline）应忽略，保持上一个值
	applyDecl(c, "text-decoration", "underline")
	applyDecl(c, "text-decoration", "overline")
	if c.TextDecoration() != TextDecorationUnderline {
		t.Errorf("未知值未被忽略，装饰 = %v, want underline", c.TextDecoration())
	}
}

func TestBoxContainsHitRules(t *testing.T) {
	b := inner(newElement("div"))
	b.x, b.y, b.width, b.height = 10, 20, 30, 40
	cases := []struct {
		x, y int
		want bool
		why  string
	}{
		{10, 20, true, "左上角闭区间"},
		{39, 59, true, "右下角内侧（半开）"},
		{40, 60, false, "右下角越界"},
		{9, 20, false, "左侧越界"},
		{10, 60, false, "下侧越界"},
	}
	for _, c := range cases {
		if got := boxContains(b, c.x, c.y); got != c.want {
			t.Errorf("boxContains(%d,%d) = %v (%s), want %v",
				c.x, c.y, got, c.why, c.want)
		}
	}
	// 零尺寸盒不参与命中（关闭的 option 即此形态）
	b.width = 0
	if boxContains(b, 10, 20) {
		t.Error("零宽盒不应命中")
	}
	if boxContains(nil, 0, 0) {
		t.Error("nil 盒不应命中")
	}
}

// findTag 在子树里按标签名找第一个元素（白盒遍历，不依赖查询选择器）。
func findTag(e HTMLElement, tag string) HTMLElement {
	if tagIs(e, tag) {
		return e
	}
	for _, c := range e.Children() {
		if got := findTag(c, tag); got != nil {
			return got
		}
	}
	return nil
}
