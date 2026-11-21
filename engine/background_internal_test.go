package engine

import (
	"fmt"
	"testing"
)

// bgDecl 用 applyDecl 把一条背景声明写进全新的样式对象（走完整解析路径：
// 属性名分派 + 值解析），测试因此不必碰级联与布局。
func bgDecl(prop, value string) *cssStyleDeclaration {
	cs := newStyle().(*cssStyleDeclaration)
	applyDecl(cs, prop, value)
	return cs
}

// bgSrcs 取图层表的引用列表（顺序即 CSS 层序）。
func bgSrcs(cs *cssStyleDeclaration) []string {
	out := make([]string, 0, len(cs.background))
	for i := range cs.background {
		out = append(out, cs.background[i].src)
	}
	return out
}

// bgKeyNames 是 <position> 关键字的可读名（按下标即常量值）。
var bgKeyNames = [...]string{
	bgKeyNone:   "none",
	bgKeyLeft:   "left",
	bgKeyCenter: "center",
	bgKeyRight:  "right",
	bgKeyTop:    "top",
	bgKeyBottom: "bottom",
}

// bgLenStr 把 backgroundLen 打成可读串，断言失败时一眼看出差在哪个分量。
func bgLenStr(l backgroundLen) string {
	unit := "px"
	if l.unit == bgUnitPct {
		unit = "%"
	}
	switch l.kind {
	case bgLenAuto:
		return "auto"
	case bgLenKey:
		if int(l.key) < len(bgKeyNames) {
			return bgKeyNames[l.key]
		}
		return fmt.Sprintf("key?%d", l.key)
	case bgLenVal:
		return fmt.Sprintf("%g%s", l.val, unit)
	case bgLenEdge:
		return bgKeyNames[l.key] + " " + fmt.Sprintf("%g%s", l.val, unit)
	}
	return fmt.Sprintf("?kind=%d", l.kind)
}

// ---------------------------------------------------------------------------
// 解析：长写属性
// ---------------------------------------------------------------------------

// TestBackgroundImageDeclParsing 覆盖 background-image：url（含引号与空白）、
// none、多层、以及「非法就整条忽略」的几种形态。
//
// 引用**保留原始大小写**：url 的路径部分大小写敏感，data: URL 的 base64
// 载荷降一位就解不开（applyDecl 为此专门传 raw）。
func TestBackgroundImageDeclParsing(t *testing.T) {
	cases := []struct {
		value string
		want  []string // nil 表示整条声明应被忽略
	}{
		{"url(a.png)", []string{"a.png"}},
		{`url("a.png")`, []string{"a.png"}},
		{`url( 'b.PNG' )`, []string{"b.PNG"}},
		{"URL(a.png)", []string{"a.png"}}, // 函数名大小写不敏感
		{"url(data:image/png;base64,AAA)", []string{"data:image/png;base64,AAA"}},
		{"none", []string{""}},
		{"url(a.png), url(b.png)", []string{"a.png", "b.png"}},
		{"url(a.png), none", []string{"a.png", ""}},
		{"url(a.png), url(b.png), url(c.png)", []string{"a.png", "b.png", "c.png"}},
		// 非法形态：整条忽略（CSS 的声明是全有或全无）
		{"image(b.png)", nil},
		{"url()", nil},
		{"url(a.png", nil},
		{"linear-gradient(red, blue)", nil},
		{"url(a.png) url(b.png)", nil}, // 一层只能有一个 <image>
	}
	for _, c := range cases {
		cs := bgDecl("background-image", c.value)
		got := bgSrcs(cs)
		if c.want == nil {
			if len(got) != 0 {
				t.Errorf("background-image:%s → 图层 %v，期望整条忽略", c.value, got)
			}
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("background-image:%s → %d 层 %v，期望 %d 层", c.value, len(got), got, len(c.want))
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("background-image:%s 第 %d 层引用 = %q，期望 %q", c.value, i, got[i], c.want[i])
			}
		}
	}
}

// TestBackgroundImageLayerCount 检查图层数由 background-image 决定：新列表
// 更短就截断多余的层（含它们的几何口径），更长就补初始值层。
func TestBackgroundImageLayerCount(t *testing.T) {
	cs := bgDecl("background-image", "url(a.png), url(b.png)")
	applyDecl(cs, "background-repeat", "no-repeat, no-repeat")
	if len(cs.background) != 2 {
		t.Fatalf("层数 = %d，期望 2", len(cs.background))
	}
	applyDecl(cs, "background-image", "url(c.png)")
	if got := bgSrcs(cs); len(got) != 1 || got[0] != "c.png" {
		t.Fatalf("截断后 = %v，期望 [c.png]", got)
	}
	// 被截掉的第 2 层不该复活：剩下的这层口径也随图层表一起没了（repeat
	// 的值表还留着 no-repeat 吗？——不留：图层表是唯一的存储处，截断即丢）。
	if rep, _, _ := backgroundKeyOf(&cs.background[0]); rep != bgRepeatNone {
		t.Errorf("保留层的平铺 = %d，期望 no-repeat（值表按索引对齐）", rep)
	}
	// 变长：新层回到各属性初始值
	applyDecl(cs, "background-image", "url(c.png), url(d.png)")
	if len(cs.background) != 2 {
		t.Fatalf("补层后层数 = %d，期望 2", len(cs.background))
	}
	if rep, _, _ := backgroundKeyOf(&cs.background[1]); rep != bgRepeatRepeat {
		t.Errorf("新层平铺 = %d，期望初始值 repeat", rep)
	}
	if cs.background[1].key != nil {
		t.Errorf("初始值层不该占口径字典")
	}
}

// TestBackgroundRepeatDecl 覆盖 background-repeat 的单值、双值、退化关键字
// 与循环对齐。
func TestBackgroundRepeatDecl(t *testing.T) {
	cases := []struct {
		value string
		want  uint8
	}{
		{"repeat", bgRepeatRepeat},
		{"no-repeat", bgRepeatNone},
		{"repeat-x", bgRepeatX},
		{"repeat-y", bgRepeatY},
		{"NO-REPEAT", bgRepeatNone},     // 关键字大小写不敏感
		{"space", bgRepeatRepeat},       // 退化为普通平铺
		{"round", bgRepeatRepeat},       // 同上
		{"repeat no-repeat", bgRepeatX}, // 横铺纵不铺
		{"no-repeat repeat", bgRepeatY}, // 横不铺纵铺
		{"no-repeat no-repeat", bgRepeatNone},
		{"repeat repeat", bgRepeatRepeat},
	}
	for _, c := range cases {
		cs := bgDecl("background-repeat", c.value)
		if len(cs.background) != 1 {
			t.Errorf("background-repeat:%s → %d 层，期望 1 层", c.value, len(cs.background))
			continue
		}
		if got, _, _ := backgroundKeyOf(&cs.background[0]); got != c.want {
			t.Errorf("background-repeat:%s → %d，期望 %d", c.value, got, c.want)
		}
	}

	// 非法取值：整条忽略（图层表保持为空，也不创建层）。
	for _, v := range []string{"foo", "repeat repeat repeat", "", "no-repeat foo"} {
		if cs := bgDecl("background-repeat", v); len(cs.background) != 0 {
			t.Errorf("background-repeat:%q 非法，却留下 %d 层", v, len(cs.background))
		}
	}

	// 初始值不占字典：repeat 与「没写过」必须是同一个形态。
	if k := bgDecl("background-repeat", "repeat").background[0].key; k != nil {
		t.Errorf("repeat 是初始值，口径字典应为 nil")
	}

	// 值表循环：单值作用到所有层，多值按索引取模。
	cs := bgDecl("background-image", "url(a.png), url(b.png), url(c.png)")
	applyDecl(cs, "background-repeat", "no-repeat")
	for i := range cs.background {
		if rep, _, _ := backgroundKeyOf(&cs.background[i]); rep != bgRepeatNone {
			t.Errorf("循环后第 %d 层平铺 = %d，期望 no-repeat", i, rep)
		}
	}
	cs = bgDecl("background-image", "url(a.png), url(b.png), url(c.png)")
	applyDecl(cs, "background-repeat", "no-repeat, repeat-x")
	want := []uint8{bgRepeatNone, bgRepeatX, bgRepeatNone}
	for i, w := range want {
		if rep, _, _ := backgroundKeyOf(&cs.background[i]); rep != w {
			t.Errorf("第 %d 层平铺 = %d，期望 %d（值表循环）", i, rep, w)
		}
	}

	// 先写 background-repeat 再写 background-image 也要对上（同一轮级联里
	// 声明顺序本就不保证）。
	cs = bgDecl("background-repeat", "no-repeat")
	applyDecl(cs, "background-image", "url(a.png)")
	if got := bgSrcs(cs); len(got) != 1 || got[0] != "a.png" {
		t.Fatalf("图层 = %v，期望 [a.png]", got)
	}
	if rep, _, _ := backgroundKeyOf(&cs.background[0]); rep != bgRepeatNone {
		t.Errorf("先写的 repeat 丢了：%d", rep)
	}
}

// TestBackgroundPositionDecl 覆盖 <position> 的三种形态：关键字、长度/百分比、
// 边偏移，以及单值简写的「另一轴补 center」。
func TestBackgroundPositionDecl(t *testing.T) {
	cases := []struct {
		value string
		x, y  string
	}{
		{"50% 50%", "50%", "50%"},
		{"0% 0%", "0%", "0%"},
		{"10px", "10px", "center"}, // 单值：另一轴 center
		{"top", "center", "top"},   // top 属纵轴
		{"bottom", "center", "bottom"},
		{"left", "left", "center"}, // left 属横轴
		{"center", "center", "center"},
		{"left top", "left", "top"},
		{"top left", "left", "top"}, // 顺序可换
		{"bottom right", "right", "bottom"},
		{"right bottom", "right", "bottom"},
		{"center center", "center", "center"},
		{"10px 20px", "10px", "20px"},
		{"right 10px bottom 20px", "right 10px", "bottom 20px"},
		{"left 10%", "left 10%", "center"},
		{"0 0", "0px", "0px"},
	}
	for _, c := range cases {
		cs := bgDecl("background-position", c.value)
		if len(cs.background) != 1 {
			t.Errorf("background-position:%s → %d 层，期望 1 层", c.value, len(cs.background))
			continue
		}
		l := &cs.background[0]
		if got := bgLenStr(l.posX); got != c.x {
			t.Errorf("background-position:%s 横轴 = %s，期望 %s", c.value, got, c.x)
		}
		if got := bgLenStr(l.posY); got != c.y {
			t.Errorf("background-position:%s 纵轴 = %s，期望 %s", c.value, got, c.y)
		}
	}

	// 非法：auto 不是 <position> 的分量；同一轴写两次；超过四值。
	for _, v := range []string{"auto", "10px 20px 30px", "top left right", "", "center center center"} {
		cs := bgDecl("background-position", v)
		if len(cs.background) != 0 {
			t.Errorf("background-position:%q 非法，却留下 %d 层", v, len(cs.background))
		}
	}
}

// TestBackgroundSizeDecl 覆盖 background-size：cover/contain 记成模式，
// 长度/百分比/auto 记成两轴，尺寸为 0 与负数非法。
func TestBackgroundSizeDecl(t *testing.T) {
	cs := bgDecl("background-size", "cover")
	if _, mode, _ := backgroundKeyOf(&cs.background[0]); mode != bgSizeCover {
		t.Errorf("cover → mode %d", mode)
	}
	cs = bgDecl("background-size", "contain")
	if _, mode, _ := backgroundKeyOf(&cs.background[0]); mode != bgSizeContain {
		t.Errorf("contain → mode %d", mode)
	}
	cs = bgDecl("background-size", "auto")
	if _, mode, _ := backgroundKeyOf(&cs.background[0]); mode != bgSizeAuto {
		t.Errorf("auto → mode %d，期望 auto", mode)
	}
	if k := bgDecl("background-size", "auto").background[0].key; k != nil {
		t.Errorf("auto 是初始值，口径字典应为 nil")
	}

	cases := []struct{ value, w, h string }{
		{"100px 50px", "100px", "50px"},
		{"50%", "50%", "auto"}, // 单值：另一轴 auto
		{"auto 30px", "auto", "30px"},
		{"auto auto", "auto", "auto"},
		{"10px 20%", "10px", "20%"},
	}
	for _, c := range cases {
		cs := bgDecl("background-size", c.value)
		l := &cs.background[0]
		if got := bgLenStr(l.sizeW); got != c.w {
			t.Errorf("background-size:%s 横轴 = %s，期望 %s", c.value, got, c.w)
		}
		if got := bgLenStr(l.sizeH); got != c.h {
			t.Errorf("background-size:%s 纵轴 = %s，期望 %s", c.value, got, c.h)
		}
	}

	// 非法：0（CSS 明确拒绝）、负数、位置关键字、cover 当双值、超过两轴。
	for _, v := range []string{"0", "0px 10px", "-10px", "left", "cover contain", "1px 2px 3px", ""} {
		cs := bgDecl("background-size", v)
		if len(cs.background) != 0 {
			t.Errorf("background-size:%q 非法，却留下 %d 层", v, len(cs.background))
		}
	}

	// 值表循环 + cover 与长度互不串味。
	cs = bgDecl("background-image", "url(a.png), url(b.png)")
	applyDecl(cs, "background-size", "100px 50px, cover")
	if got := bgLenStr(cs.background[0].sizeW); got != "100px" {
		t.Errorf("第 1 层横轴 = %s", got)
	}
	if _, mode, _ := backgroundKeyOf(&cs.background[1]); mode != bgSizeCover {
		t.Errorf("第 2 层 mode = %d，期望 cover", mode)
	}
	cs = bgDecl("background-image", "url(a.png), url(b.png), url(c.png)")
	applyDecl(cs, "background-size", "contain")
	for i := range cs.background {
		if _, mode, _ := backgroundKeyOf(&cs.background[i]); mode != bgSizeContain {
			t.Errorf("第 %d 层 mode = %d，期望 contain（循环表）", i, mode)
		}
	}
}

// TestBackgroundBoxDecl 覆盖 background-origin / background-clip 两套常量，
// 以及「初始值不占字典」。
func TestBackgroundBoxDecl(t *testing.T) {
	originCases := []struct {
		value string
		want  uint8
	}{
		{"border-box", bgOriginBorder},
		{"padding-box", bgOriginPadding},
		{"content-box", bgOriginContent},
		{"CONTENT-BOX", bgOriginContent},
	}
	for _, c := range originCases {
		cs := bgDecl("background-origin", c.value)
		if got := cs.background[0].originBox(); got != c.want {
			t.Errorf("background-origin:%s → %d，期望 %d", c.value, got, c.want)
		}
	}
	// padding-box 是 origin 的初始值：单独写它不该造字典项。
	if k := bgDecl("background-origin", "padding-box").background[0].key; k != nil {
		t.Errorf("background-origin:padding-box 是初始值，口径字典应为 nil")
	}

	clipCases := []struct {
		value string
		want  uint8
	}{
		{"border-box", bgClipBorder},
		{"padding-box", bgClipPadding},
		{"content-box", bgClipContent},
	}
	for _, c := range clipCases {
		cs := bgDecl("background-clip", c.value)
		if _, _, got := backgroundKeyOf(&cs.background[0]); got != c.want {
			t.Errorf("background-clip:%s → %d，期望 %d", c.value, got, c.want)
		}
	}
	if k := bgDecl("background-clip", "border-box").background[0].key; k != nil {
		t.Errorf("background-clip:border-box 是初始值，口径字典应为 nil")
	}

	// 两套常量不能混：clip 的 content(2) 与 origin 的 border(1) 不同值。
	cs := bgDecl("background-clip", "content-box")
	if cs.background[0].originBox() != bgOriginPadding {
		t.Errorf("只写 clip 不该动 origin")
	}
	cs = bgDecl("background-origin", "border-box")
	if _, _, clip := backgroundKeyOf(&cs.background[0]); clip != bgClipBorder {
		t.Errorf("只写 origin 不该动 clip")
	}

	// 非法：每层只允许一个 box 关键字；两个值的写法属于简写。
	for _, v := range []string{"border", "padding-box border-box", "text", ""} {
		if cs := bgDecl("background-origin", v); len(cs.background) != 0 {
			t.Errorf("background-origin:%q 非法，却留下 %d 层", v, len(cs.background))
		}
		if cs := bgDecl("background-clip", v); len(cs.background) != 0 {
			t.Errorf("background-clip:%q 非法，却留下 %d 层", v, len(cs.background))
		}
	}
}

// ---------------------------------------------------------------------------
// 解析：background 简写
// ---------------------------------------------------------------------------

// TestBackgroundShorthand 覆盖简写的各分量归类与重置语义。
func TestBackgroundShorthand(t *testing.T) {
	// url + repeat
	cs := bgDecl("background", "url(x.png) no-repeat")
	if got := bgSrcs(cs); len(got) != 1 || got[0] != "x.png" {
		t.Fatalf("简写图层 = %v，期望 [x.png]", got)
	}
	if rep, _, _ := backgroundKeyOf(&cs.background[0]); rep != bgRepeatNone {
		t.Errorf("no-repeat 没解析出来：%d", rep)
	}

	// 颜色函数不能被当成未实现 <image> 整条杀掉（snip 的选区暗化层就是
	// background: rgba(10,16,30,0.30)）。
	for _, v := range []string{"rgba(10,16,30,0.30)", "#334155", "rgb(1, 2, 3)", "hsl(120, 50%, 50%)"} {
		cs := bgDecl("background-color", "#ff0000")
		applyDecl(cs, "background", v)
		if cs.backgroundColor == nil {
			t.Errorf("background:%s 没解析出背景色", v)
		}
	}

	// 渐变仍未实现：整条忽略（保留原色，而不是清成透明）。
	cs = bgDecl("background-color", "#ff0000")
	applyDecl(cs, "background", "linear-gradient(red, blue)")
	if cs.backgroundColor == nil {
		t.Errorf("未实现的渐变应当让整条声明失效，而不是清空背景色")
	}

	// 简写重置背景色：没写颜色就是透明。
	cs = bgDecl("background-color", "#ff0000")
	applyDecl(cs, "background", "url(x.png)")
	if cs.backgroundColor != nil {
		t.Errorf("background 简写该把没写出的 background-color 重置为透明")
	}
	if len(cs.background) != 1 || cs.background[0].src != "x.png" {
		t.Errorf("简写图层 = %v", bgSrcs(cs))
	}

	// 简写重置长写口径：position/size/repeat/origin/clip 全回初始值。
	cs = bgDecl("background-image", "url(a.png)")
	applyDecl(cs, "background-repeat", "no-repeat")
	applyDecl(cs, "background-position", "50% 50%")
	applyDecl(cs, "background-size", "cover")
	applyDecl(cs, "background", "url(b.png)")
	if rep, mode, _ := backgroundKeyOf(&cs.background[0]); rep != bgRepeatRepeat || mode != bgSizeAuto {
		t.Errorf("简写后口径 = %d/%d，期望初始值 repeat/auto", rep, mode)
	}
	if cs.background[0].posX.kind != bgLenAuto || cs.background[0].posY.kind != bgLenAuto {
		t.Errorf("简写后 position = %s/%s，期望回到未声明（初始 0%% 0%%）",
			bgLenStr(cs.background[0].posX), bgLenStr(cs.background[0].posY))
	}

	// position / size（斜杠两侧空白可选，两种写法同义）
	for _, v := range []string{"url(x.png) 50%/cover no-repeat", "url(x.png) 50% / cover no-repeat"} {
		cs := bgDecl("background", v)
		if got := bgLenStr(cs.background[0].posX); got != "50%" {
			t.Errorf("%s → posX = %s，期望 50%%", v, got)
		}
		if got := bgLenStr(cs.background[0].posY); got != "center" {
			t.Errorf("%s → posY = %s，期望 center", v, got)
		}
		if _, mode, _ := backgroundKeyOf(&cs.background[0]); mode != bgSizeCover {
			t.Errorf("%s → size mode = %d，期望 cover", v, mode)
		}
		if rep, _, _ := backgroundKeyOf(&cs.background[0]); rep != bgRepeatNone {
			t.Errorf("%s → repeat = %d，期望 no-repeat", v, rep)
		}
	}

	// box：一个值同时设 origin 与 clip；两个值前者 origin、后者 clip。
	cs = bgDecl("background", "url(x.png) content-box")
	if cs.background[0].originBox() != bgOriginContent {
		t.Errorf("单值 box 没设 origin")
	}
	if _, _, clip := backgroundKeyOf(&cs.background[0]); clip != bgClipContent {
		t.Errorf("单值 box 没设 clip")
	}
	cs = bgDecl("background", "url(x.png) padding-box border-box")
	if cs.background[0].originBox() != bgOriginPadding {
		t.Errorf("双值 box 的第一值应当是 origin")
	}
	if _, _, clip := backgroundKeyOf(&cs.background[0]); clip != bgClipBorder {
		t.Errorf("双值 box 的第二值应当是 clip")
	}
	if cs.background[0].key != nil {
		t.Errorf("padding-box border-box 全是初始值，不该占字典")
	}

	// attachment：只实现 scroll，fixed/local 收下但不改口径。
	for _, v := range []string{"url(x.png) scroll", "url(x.png) fixed", "url(x.png) local"} {
		cs := bgDecl("background", v)
		if len(cs.background) != 1 || cs.background[0].src != "x.png" {
			t.Errorf("%s → 图层 = %v", v, bgSrcs(cs))
		}
		if k := cs.background[0].key; k != nil {
			t.Errorf("%s → 不该产生非初始口径：%+v", v, *k)
		}
	}

	// 认不出的 token：整条声明非法，原状态一点不动。
	cs = bgDecl("background-image", "url(old.png)")
	before := cs.background
	applyDecl(cs, "background", "url(x.png) bogus")
	if len(cs.background) != 1 || cs.background[0].src != "old.png" {
		t.Errorf("非法简写改动了图层表：%v", bgSrcs(cs))
	}
	if &cs.background[0] != &before[0] {
		t.Errorf("整条忽略时底层数组不该换")
	}

	// 多层：每层各自的口径互不串味。
	cs = bgDecl("background", "url(a.png), url(b.png) no-repeat")
	if len(cs.background) != 2 {
		t.Fatalf("层数 = %d，期望 2", len(cs.background))
	}
	if rep, _, _ := backgroundKeyOf(&cs.background[0]); rep != bgRepeatRepeat {
		t.Errorf("第 1 层平铺 = %d，期望初始值 repeat", rep)
	}
	if rep, _, _ := backgroundKeyOf(&cs.background[1]); rep != bgRepeatNone {
		t.Errorf("第 2 层平铺 = %d，期望 no-repeat", rep)
	}

	// 颜色属于最后一层（CSS Backgrounds 3 §3.9）：后出现的颜色覆盖前面的。
	cs = bgDecl("background", "url(a.png) #ff0000, url(b.png) #00ff00")
	if r, g, _, _ := cs.backgroundColor.RGBA(); r != 0 || g != 255 {
		t.Errorf("背景色 = %v，期望最后一层的 #00ff00", cs.backgroundColor)
	}
	// transparent 与 background-color 长写同口径：透明（不是 alpha=0 的黑）。
	cs = bgDecl("background", "transparent")
	if cs.backgroundColor != nil {
		t.Errorf("background:transparent → %v，期望透明（nil）", cs.backgroundColor)
	}

	// none / inherit 之类的关键字走「非法」通道：CSS 里 background: inherit
	// 需要继承值，引擎不支持关键字型取值，整条忽略。
	for _, v := range []string{"inherit", "initial", "unset", "revert", ""} {
		cs = bgDecl("background-color", "#ff0000")
		applyDecl(cs, "background", v)
		if cs.backgroundColor == nil || len(cs.background) != 0 {
			t.Errorf("background:%q 应整条忽略（色 %v / 层 %d）", v, cs.backgroundColor, len(cs.background))
		}
	}
}

// TestBackgroundShorthandKeepsURLCase 检查简写里的 url 引用保留原始大小写
// （data: 的 base64 载荷降位就废了）。
func TestBackgroundShorthandKeepsURLCase(t *testing.T) {
	const ref = "data:image/png;base64,iVBORw0KGgoABCDEF"
	cs := bgDecl("background", "url("+ref+") no-repeat")
	if len(cs.background) != 1 || cs.background[0].src != ref {
		t.Fatalf("引用 = %v，期望原样保留", bgSrcs(cs))
	}
	if rep, _, _ := backgroundKeyOf(&cs.background[0]); rep != bgRepeatNone {
		t.Errorf("同一条里的 no-repeat 应当按小写归类，实际 %d", rep)
	}
}

// ---------------------------------------------------------------------------
// 口径字典
// ---------------------------------------------------------------------------

// TestBackgroundKeyIntern 检查口径字典共享、以及「改口径必须换指针」——
// 字典可能被多个元素共用，就地改会把别人的背景口径一起改掉。
func TestBackgroundKeyIntern(t *testing.T) {
	a := bgDecl("background-repeat", "no-repeat").background[0].key
	b := bgDecl("background-repeat", "no-repeat").background[0].key
	if a == nil || b == nil {
		t.Fatal("非初始值的图层应当有字典项")
	}
	if a != b {
		t.Errorf("同样的口径没有共享字典项")
	}

	cs := bgDecl("background-repeat", "no-repeat")
	before := cs.background[0].key
	setBackgroundRepeat(&cs.background[0], bgRepeatX)
	after := cs.background[0].key
	if after == before {
		t.Errorf("改口径应当换一份字典项")
	}
	if before.repeat != bgRepeatNone {
		t.Errorf("共享字典被就地改坏了：%+v", *before)
	}
	if after.repeat != bgRepeatX {
		t.Errorf("新口径不对：%+v", *after)
	}

	// 全初始值不占分配。
	if k := internBackgroundKey(backgroundKey{
		repeat: bgRepeatRepeat, sizeMode: bgSizeAuto,
		boxPair: (bgOriginPadding << 2) | bgClipBorder,
	}); k != nil {
		t.Errorf("全初始值不该占字典项：%+v", *k)
	}
	// 四个口径位各自都能与初始值区分开。
	for _, k := range []backgroundKey{
		{repeat: bgRepeatNone},
		{sizeMode: bgSizeCover},
		{boxPair: bgClipPadding},
		{boxPair: bgOriginBorder << 2},
	} {
		if internBackgroundKey(k) == nil {
			t.Errorf("非初始值 %+v 被判成了初始值", k)
		}
	}
}

// TestNewBackgroundLayer 覆盖公开 API SetBackgroundImage 用的兼容口径：
// 铺满边框盒、不平铺。
func TestNewBackgroundLayer(t *testing.T) {
	bmp := &Bitmap{W: 4, H: 2, Pix: make([]byte, 4*2*4)}
	l := newBackgroundLayer(bmp, 100, 100)
	if l.bitmap != bmp {
		t.Errorf("位图没绑上")
	}
	if rep, _, clip := backgroundKeyOf(&l); rep != bgRepeatNone || clip != bgClipBorder {
		t.Errorf("口径 = %d/%d，期望不平铺 + 边框裁剪", rep, clip)
	}
	if l.originBox() != bgOriginBorder {
		t.Errorf("定位盒 = %d，期望边框盒（兼容旧行为）", l.originBox())
	}
	if got := bgLenStr(l.sizeW); got != "100%" {
		t.Errorf("sizeW = %s，期望 100%%", got)
	}
	// 尺寸给 0 表示该轴不写（回 auto，按固有尺寸）。
	l = newBackgroundLayer(bmp, 100, 0)
	if got := bgLenStr(l.sizeH); got != "auto" {
		t.Errorf("sizeH = %s，期望 auto", got)
	}
}

// TestCopyBackgroundLayers 检查拷贝出来的图层各持有自己的底层数组：
// copyStyle 之后一方改图层不能串到另一方（位图与字典仍共享）。
func TestCopyBackgroundLayers(t *testing.T) {
	src := bgDecl("background", "url(a.png) no-repeat").background
	dst := copyBackgroundLayers(src)
	if len(dst) != len(src) {
		t.Fatalf("层数 = %d，期望 %d", len(dst), len(src))
	}
	if &dst[0] == &src[0] {
		t.Errorf("底层数组没有被拷贝（共享元素存储）")
	}
	if dst[0].bitmap != src[0].bitmap || dst[0].key != src[0].key {
		t.Errorf("位图与口径字典应当共享")
	}
	setBackgroundRepeat(&dst[0], bgRepeatY)
	if rep, _, _ := backgroundKeyOf(&src[0]); rep != bgRepeatNone {
		t.Errorf("改拷贝层串到了原表：%d", rep)
	}
	if copyBackgroundLayers(nil) != nil {
		t.Errorf("空表要还回 nil")
	}
}

// ---------------------------------------------------------------------------
// 几何
// ---------------------------------------------------------------------------

// TestBackgroundTileStarts 覆盖平铺网格：起点对齐、越界瓦片保留（靠裁剪切边）、
// 不重复时只有一块。
func TestBackgroundTileStarts(t *testing.T) {
	cases := []struct {
		start, size, clipX, clipW int
		repeat                    bool
		want                      []int
	}{
		{0, 10, 0, 25, true, []int{0, 10, 20}},
		{0, 10, 0, 20, true, []int{0, 10}},           // 恰好整数块：边缘那块不算
		{5, 10, 20, 30, true, []int{15, 25, 35, 45}}, // 起点在裁剪区左侧
		{-5, 10, 0, 5, true, []int{-5}},              // 裁剪区窄于一块
		{0, 10, 0, 25, false, []int{0}},              // 不平铺：一块，即使越界
		{-40, 10, 0, 25, false, []int{-40}},
		{0, 0, 0, 25, true, nil}, // 尺寸非法
		{0, 10, 0, 0, true, nil}, // 裁剪区宽度非正
	}
	for _, c := range cases {
		got := tileStarts(c.start, c.size, c.clipX, c.clipW, c.repeat)
		if len(got) != len(c.want) {
			t.Errorf("tileStarts(%d,%d,%d,%d,%v) = %v，期望 %v",
				c.start, c.size, c.clipX, c.clipW, c.repeat, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("tileStarts(%d,%d,%d,%d,%v)[%d] = %d，期望 %d",
					c.start, c.size, c.clipX, c.clipW, c.repeat, i, got[i], c.want[i])
			}
		}
	}

	// 瓦片数上限：1px 的图铺满超宽裁剪区时不能把一帧吃满。
	if n := len(tileStarts(0, 1, 0, 1<<20, true)); n != maxBackgroundTiles {
		t.Errorf("瓦片数 = %d，期望封顶 %d", n, maxBackgroundTiles)
	}
}

// TestBackgroundImageSize 覆盖 background-size 的尺寸计算：固有尺寸、
// 单轴补另一轴、cover/contain。
func TestBackgroundImageSize(t *testing.T) {
	bmp := &Bitmap{W: 20, H: 10, Pix: make([]byte, 20*10*4)}
	cases := []struct {
		name  string
		layer backgroundLayer
		mode  uint8
		areaW int
		areaH int
		wantW int
		wantH int
	}{
		{"auto（固有尺寸）", backgroundLayer{}, bgSizeAuto, 100, 30, 20, 10},
		{"两轴长度", backgroundLayer{
			sizeW: backgroundLen{kind: bgLenVal, unit: bgUnitPx, val: 50},
			sizeH: backgroundLen{kind: bgLenVal, unit: bgUnitPx, val: 40},
		}, bgSizeAuto, 100, 30, 50, 40},
		{"单轴按纵横比补", backgroundLayer{
			sizeW: backgroundLen{kind: bgLenVal, unit: bgUnitPx, val: 50},
		}, bgSizeAuto, 100, 30, 50, 25},
		{"另一轴同理", backgroundLayer{
			sizeH: backgroundLen{kind: bgLenVal, unit: bgUnitPx, val: 20},
		}, bgSizeAuto, 100, 30, 40, 20},
		{"百分比相对定位盒", backgroundLayer{
			sizeW: backgroundLen{kind: bgLenVal, unit: bgUnitPct, val: 50},
			sizeH: backgroundLen{kind: bgLenVal, unit: bgUnitPct, val: 50},
		}, bgSizeAuto, 100, 30, 50, 15},
		{"cover", backgroundLayer{}, bgSizeCover, 100, 30, 100, 50},
		{"contain", backgroundLayer{}, bgSizeContain, 100, 30, 60, 30},
		{"cover 纵横同尺度", backgroundLayer{}, bgSizeCover, 40, 20, 40, 20},
	}
	for _, c := range cases {
		l := c.layer
		w, h := l.imageSize(bmp, c.areaW, c.areaH, c.mode)
		if w != c.wantW || h != c.wantH {
			t.Errorf("%s：%dx%d，期望 %dx%d", c.name, w, h, c.wantW, c.wantH)
		}
	}
	// 固有尺寸非法（0×0）→ 什么都不画。
	empty := backgroundLayer{}
	if w, h := empty.imageSize(&Bitmap{}, 100, 100, bgSizeAuto); w != 0 || h != 0 {
		t.Errorf("空位图 → %dx%d，期望 0x0", w, h)
	}
}

// TestBackgroundAxisOffset 覆盖 background-position 的偏移计算：百分比与
// 关键字的基数是「定位盒 − 图片尺寸」（所以 50% 就是居中），边偏移从
// 指定的那条边往里缩。
func TestBackgroundAxisOffset(t *testing.T) {
	cases := []struct {
		name string
		v    backgroundLen
		// area=100, img=20 → free=80
		want int
	}{
		{"未声明（初始 0%）", backgroundLen{}, 0},
		{"auto", backgroundLen{kind: bgLenAuto}, 0},
		{"0%", backgroundLen{kind: bgLenVal, unit: bgUnitPct}, 0},
		{"50%", backgroundLen{kind: bgLenVal, unit: bgUnitPct, val: 50}, 40},
		{"100%", backgroundLen{kind: bgLenVal, unit: bgUnitPct, val: 100}, 80},
		{"10px", backgroundLen{kind: bgLenVal, unit: bgUnitPx, val: 10}, 10},
		{"-10px", backgroundLen{kind: bgLenVal, unit: bgUnitPx, val: -10}, -10},
		{"center", backgroundLen{kind: bgLenKey, key: bgKeyCenter}, 40},
		{"right", backgroundLen{kind: bgLenKey, key: bgKeyRight}, 80},
		{"left", backgroundLen{kind: bgLenKey, key: bgKeyLeft}, 0},
		{"right 10px", backgroundLen{kind: bgLenEdge, key: bgKeyRight, val: 10}, 70},
		{"left 10px", backgroundLen{kind: bgLenEdge, key: bgKeyLeft, val: 10}, 10},
		{"bottom 10px", backgroundLen{kind: bgLenEdge, key: bgKeyBottom, val: 10}, 70},
		{"right 50%", backgroundLen{kind: bgLenEdge, key: bgKeyRight, unit: bgUnitPct, val: 50}, 40},
	}
	for _, c := range cases {
		if got := backgroundAxisOffset(c.v, 100, 20); got != c.want {
			t.Errorf("%s：偏移 = %d，期望 %d", c.name, got, c.want)
		}
	}
	// 图片比定位盒大：free 为负，50% 是负的居中偏移（图往左溢出）。
	if got := backgroundAxisOffset(backgroundLen{kind: bgLenVal, unit: bgUnitPct, val: 50}, 20, 100); got != -40 {
		t.Errorf("超宽图的 50%% 偏移 = %d，期望 -40", got)
	}
}

// TestBackgroundBoxRect 覆盖三种盒口径的缩进：边框盒不动，padding 盒只缩
// 边框，content 盒再缩 padding。
func TestBackgroundBoxRect(t *testing.T) {
	pd := edges{6, 5, 8, 7} // 上 右 下 左
	bd := edges{2, 1, 3, 4}
	bx, by, bw, bh := 10, 20, 100, 60
	cases := []struct {
		box        uint8
		x, y, w, h int
	}{
		{bgOriginBorder, 10, 20, 100, 60},
		{bgOriginPadding, 14, 22, 95, 55},
		{bgOriginContent, 21, 28, 83, 41},
	}
	for _, c := range cases {
		x, y, w, h := backgroundBoxRect(c.box, bx, by, bw, bh, pd, bd)
		if x != c.x || y != c.y || w != c.w || h != c.h {
			t.Errorf("盒 %d → %d,%d %dx%d，期望 %d,%d %dx%d",
				c.box, x, y, w, h, c.x, c.y, c.w, c.h)
		}
	}

	// clip 常量组要换算到 origin 常量组才能进 backgroundBoxRect。
	if clipBoxToOrigin(bgClipBorder) != bgOriginBorder ||
		clipBoxToOrigin(bgClipPadding) != bgOriginPadding ||
		clipBoxToOrigin(bgClipContent) != bgOriginContent {
		t.Errorf("clip → origin 换算错位")
	}

	// 盒子缩到非正（边框比盒宽）：宽高回 0 或负，绘制层据此不落墨。
	x, y, w, h := backgroundBoxRect(bgOriginContent, 0, 0, 4, 4, edges{10, 10, 10, 10}, edges{10, 10, 10, 10})
	if w > 0 || h > 0 {
		t.Errorf("过度缩进后 = %d,%d %dx%d，期望非正的宽高", x, y, w, h)
	}
}
