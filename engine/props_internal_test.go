package engine

import (
	"math"
	"testing"
)

// font-style / visibility / float / clear 的解析：合法值落位，非法值保留
// 原值（与其它属性「非法声明忽略」的口径一致）。
func TestParseFontStyleVisibilityFloatClear(t *testing.T) {
	st := newStyle()
	applyDecl(st, "font-style", "italic")
	if st.FontStyle() != FontStyleItalic {
		t.Fatalf("font-style:italic = %v, want italic", st.FontStyle())
	}
	// oblique 走同一档（引擎用合成斜体，两者观感一致）。
	applyDecl(st, "font-style", "normal")
	applyDecl(st, "font-style", "oblique")
	if st.FontStyle() != FontStyleItalic {
		t.Fatalf("font-style:oblique = %v, want italic", st.FontStyle())
	}
	applyDecl(st, "font-style", "bogus")
	if st.FontStyle() != FontStyleItalic {
		t.Fatalf("非法 font-style 覆盖了有效值: %v", st.FontStyle())
	}

	applyDecl(st, "visibility", "hidden")
	if st.Visibility() != VisibilityHidden {
		t.Fatalf("visibility:hidden = %v", st.Visibility())
	}
	applyDecl(st, "visibility", "collapse")
	if st.Visibility() != VisibilityCollapse {
		t.Fatalf("visibility:collapse = %v", st.Visibility())
	}
	applyDecl(st, "visibility", "visible")
	if st.Visibility() != VisibilityVisible {
		t.Fatalf("visibility:visible = %v", st.Visibility())
	}

	applyDecl(st, "float", "left")
	if st.Float() != FloatLeft {
		t.Fatalf("float:left = %v", st.Float())
	}
	applyDecl(st, "float", "right")
	if st.Float() != FloatRight {
		t.Fatalf("float:right = %v", st.Float())
	}
	applyDecl(st, "float", "inline-start")
	if st.Float() != FloatRight {
		t.Fatalf("非法 float 覆盖了有效值: %v", st.Float())
	}
	applyDecl(st, "float", "none")
	if st.Float() != FloatNone {
		t.Fatalf("float:none = %v", st.Float())
	}

	applyDecl(st, "clear", "left")
	if st.Clear() != ClearLeft {
		t.Fatalf("clear:left = %v", st.Clear())
	}
	applyDecl(st, "clear", "both")
	if st.Clear() != ClearBoth {
		t.Fatalf("clear:both = %v", st.Clear())
	}
	applyDecl(st, "clear", "bogus")
	if st.Clear() != ClearBoth {
		t.Fatalf("非法 clear 覆盖了有效值: %v", st.Clear())
	}
}

// opacity 接受 0~1 数字与百分比，越界钳到 [0,1]；初始值为 1（不是零值 0）。
func TestParseOpacity(t *testing.T) {
	st := newStyle()
	if st.Opacity() != 1 {
		t.Fatalf("opacity 初始值 = %v, want 1", st.Opacity())
	}
	applyDecl(st, "opacity", "0.4")
	if math.Abs(float64(st.Opacity())-0.4) > 1e-6 {
		t.Fatalf("opacity:0.4 = %v", st.Opacity())
	}
	applyDecl(st, "opacity", "50%")
	if math.Abs(float64(st.Opacity())-0.5) > 1e-6 {
		t.Fatalf("opacity:50%% = %v", st.Opacity())
	}
	applyDecl(st, "opacity", "2")
	if st.Opacity() != 1 {
		t.Fatalf("opacity:2 未钳到 1: %v", st.Opacity())
	}
	applyDecl(st, "opacity", "-1")
	if st.Opacity() != 0 {
		t.Fatalf("opacity:-1 未钳到 0: %v", st.Opacity())
	}
	applyDecl(st, "opacity", "abc")
	if st.Opacity() != 0 {
		t.Fatalf("非法 opacity 覆盖了有效值: %v", st.Opacity())
	}
}

// cursor 关键字映射到系统指针形状；url() 自定义光标退回 auto（引擎只支持
// 系统形状）；逗号后的回退关键字优先。
func TestParseCursor(t *testing.T) {
	st := newStyle()
	cases := []struct {
		value string
		want  Cursor
	}{
		{"pointer", CursorPointer},
		{"hand", CursorPointer},
		{"text", CursorText},
		{"move", CursorMove},
		{"not-allowed", CursorNotAllowed},
		{"wait", CursorWait},
		{"crosshair", CursorCrosshair},
		{"grab", CursorGrab},
		{"ew-resize", CursorEWResize},
		{"ns-resize", CursorNSResize},
		{"nwse-resize", CursorNWSEResize},
		{"url(x.png) 4 4, move", CursorMove},
		{"url(x.png)", CursorAuto},
	}
	for _, c := range cases {
		applyDecl(st, "cursor", c.value)
		if st.Cursor() != c.want {
			t.Errorf("cursor:%s = %v, want %v", c.value, st.Cursor(), c.want)
		}
	}
	// 非法关键字保留前一次的有效值。
	applyDecl(st, "cursor", "url(x.png)")
	applyDecl(st, "cursor", "bogus")
	if st.Cursor() != CursorAuto {
		t.Errorf("非法 cursor 覆盖了有效值: %v", st.Cursor())
	}
}

// copyStyle 必须同步新增字段，否则伪元素/克隆盒会丢掉这些属性。
func TestCopyStyleCopiesNewProps(t *testing.T) {
	src := newStyle()
	src.SetFontStyle(FontStyleItalic)
	src.SetVisibility(VisibilityHidden)
	src.SetFloat(FloatRight)
	src.SetClear(ClearBoth)
	src.SetOpacity(0.25)
	src.SetCursor(CursorPointer)

	dst := copyStyle(src)
	if dst.FontStyle() != FontStyleItalic || dst.Visibility() != VisibilityHidden ||
		dst.Float() != FloatRight || dst.Clear() != ClearBoth ||
		dst.Cursor() != CursorPointer {
		t.Fatalf("copyStyle 丢了新增字段: %+v", dst)
	}
	if math.Abs(float64(dst.Opacity())-0.25) > 1e-6 {
		t.Fatalf("copyStyle opacity = %v, want 0.25", dst.Opacity())
	}
}
