package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 深色主题必须是一套完整令牌：漏填的颜色会被 withDefaults 补成浅色，
// 在深底上表现为某块组件突然发白。这里逐个颜色字段检查非空，并验证
// 关键语义：深底、浅字、亮强调色配深色文字。
func TestDarkThemeIsComplete(t *testing.T) {
	d := Dark()
	colors := map[string]string{
		"Bg":           d.Bg,
		"Surface":      d.Surface,
		"SurfaceAlt":   d.SurfaceAlt,
		"Field":        d.Field,
		"Hover":        d.Hover,
		"Press":        d.Press,
		"Border":       d.Border,
		"BorderStrong": d.BorderStrong,
		"Text":         d.Text,
		"Muted":        d.Muted,
		"Accent":       d.Accent,
		"AccentHover":  d.AccentHover,
		"OnAccent":     d.OnAccent,
		"AccentSoft":   d.AccentSoft,
		"AccentBorder": d.AccentBorder,
		"AccentText":   d.AccentText,

		"Success":       d.Success,
		"SuccessHover":  d.SuccessHover,
		"OnSuccess":     d.OnSuccess,
		"SuccessSoft":   d.SuccessSoft,
		"SuccessBorder": d.SuccessBorder,
		"SuccessText":   d.SuccessText,

		"Warning":       d.Warning,
		"WarningHover":  d.WarningHover,
		"OnWarning":     d.OnWarning,
		"WarningSoft":   d.WarningSoft,
		"WarningBorder": d.WarningBorder,
		"WarningText":   d.WarningText,

		"Danger":       d.Danger,
		"DangerHover":  d.DangerHover,
		"OnDanger":     d.OnDanger,
		"DangerSoft":   d.DangerSoft,
		"DangerBorder": d.DangerBorder,
		"DangerText":   d.DangerText,

		"Info":       d.Info,
		"InfoHover":  d.InfoHover,
		"OnInfo":     d.OnInfo,
		"InfoSoft":   d.InfoSoft,
		"InfoBorder": d.InfoBorder,
		"InfoText":   d.InfoText,
	}
	for name, c := range colors {
		if c == "" {
			t.Fatalf("%s 为空：深色主题必须是完整的一套", name)
		}
	}

	l := Light()
	if d.Bg == l.Bg || d.Surface == l.Surface || d.Text == l.Text {
		t.Fatal("深色主题仍在用浅色的底/面/文字色")
	}
	if d.OnAccent == l.OnAccent {
		t.Fatal("深色主题的强调色更亮，OnAccent 不能沿用白色")
	}
	if d.OnSuccess == l.OnSuccess || d.OnDanger == l.OnDanger || d.OnInfo == l.OnInfo {
		t.Fatal("深色主题的语义实底更亮，OnSuccess/OnDanger/OnInfo 不能沿用浅色")
	}
	if d.Font != l.Font || d.Radius != l.Radius ||
		d.MenuHeight != l.MenuHeight || d.ToolHeight != l.ToolHeight ||
		d.StatusHeight != l.StatusHeight || d.StatusRightWidth != l.StatusRightWidth ||
		d.ButtonHeight != l.ButtonHeight {
		t.Fatal("两套主题的字体/尺寸应保持一致，只换颜色")
	}
	if d.Field == l.Field {
		t.Fatal("深色主题的输入区底色不能沿用浅色（会和栏底糊在一起）")
	}
	// 空间层次：输入区/画布最深 → 栏面 → 工具栏/控件条逐级提亮。
	// 深色主题最怕「糊成一团」，三层亮度必须拉开可辨的档位。
	lum := func(hex string) int {
		n, _ := strconv.ParseInt(hex[1:], 16, 32)
		r, g, b := int(n>>16)&0xFF, int(n>>8)&0xFF, int(n)&0xFF
		return (299*r + 587*g + 114*b) / 1000
	}
	if !(lum(d.Field) <= lum(d.Bg) && lum(d.Bg) < lum(d.Surface) && lum(d.Surface) < lum(d.SurfaceAlt)) {
		t.Fatalf("深色层次不成立：Field=%s Bg=%s Surface=%s SurfaceAlt=%s",
			d.Field, d.Bg, d.Surface, d.SurfaceAlt)
	}
}

// 生成 CSS 时主按钮必须用 OnAccent，不能在样式表里写死白色：否则深色
// 主题的亮蓝底上压白字会看不清。
func TestDarkThemeCSS(t *testing.T) {
	d := Dark()
	css := d.css(32, 72, 24, true)
	if strings.Contains(css, "$") {
		t.Fatalf("CSS 里残留未替换的占位符:\n%s", css)
	}
	for _, want := range []string{d.Bg, d.Surface, d.Text, d.Accent, d.OnAccent} {
		if !strings.Contains(css, want) {
			t.Fatalf("CSS 缺少颜色 %s", want)
		}
	}
	if strings.Contains(css, "#FFFFFF") {
		t.Fatal("CSS 里仍有写死的白色（主按钮应使用 OnAccent）")
	}
	if !strings.Contains(css,
		"background-color: "+d.Accent+"; border-color: "+d.Accent+"; color: "+d.OnAccent) {
		t.Fatal("主按钮没有同时使用 Accent 与 OnAccent")
	}
	// 页面根节点要带文字色，否则深色背景上的裸文字仍是默认黑色。
	if !strings.Contains(css, "background-color: "+d.Bg+"; color: "+d.Text) {
		t.Fatal(".system-ui-page 没有设置主题文字色")
	}
}

// 语义色必须真的进入样式表：实底按钮用 On*，徽标/提示条用淡底 + 描边 +
// 淡底文字；Default 复用中性令牌。否则 Theme 字段只是摆设。
func TestSemanticColorsCSS(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
	}{
		{"light", Light()},
		{"dark", Dark()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := tc.th
			css := th.css(32, 72, 24, true)
			if strings.Contains(css, "$") {
				t.Fatalf("CSS 里残留未替换的占位符:\n%s", css)
			}
			checks := []struct {
				name  string
				class string
				decl  string
			}{
				{"默认按钮", ".system-ui-btn.is-default",
					"background-color: " + th.Field + "; border-color: " + th.BorderStrong + "; color: " + th.Text},
				{"主按钮", ".system-ui-btn.is-primary",
					"background-color: " + th.Accent + "; border-color: " + th.Accent + "; color: " + th.OnAccent},
				{"成功按钮", ".system-ui-btn.is-success",
					"background-color: " + th.Success + "; border-color: " + th.Success + "; color: " + th.OnSuccess},
				{"警告按钮", ".system-ui-btn.is-warning",
					"background-color: " + th.Warning + "; border-color: " + th.Warning + "; color: " + th.OnWarning},
				{"危险按钮", ".system-ui-btn.is-danger",
					"background-color: " + th.Danger + "; border-color: " + th.Danger + "; color: " + th.OnDanger},
				{"信息按钮", ".system-ui-btn.is-info",
					"background-color: " + th.Info + "; border-color: " + th.Info + "; color: " + th.OnInfo},

				{"主徽标", ".system-ui-badge.is-primary",
					"background-color: " + th.AccentSoft + "; border-color: " + th.AccentBorder + "; color: " + th.AccentText},
				{"默认徽标", ".system-ui-badge.is-default",
					"background-color: " + th.SurfaceAlt + "; border-color: " + th.Border + "; color: " + th.Text},
				{"成功徽标", ".system-ui-badge.is-success",
					"background-color: " + th.SuccessSoft + "; border-color: " + th.SuccessBorder + "; color: " + th.SuccessText},
				{"警告徽标", ".system-ui-badge.is-warning",
					"background-color: " + th.WarningSoft + "; border-color: " + th.WarningBorder + "; color: " + th.WarningText},
				{"危险徽标", ".system-ui-badge.is-danger",
					"background-color: " + th.DangerSoft + "; border-color: " + th.DangerBorder + "; color: " + th.DangerText},
				{"信息徽标", ".system-ui-badge.is-info",
					"background-color: " + th.InfoSoft + "; border-color: " + th.InfoBorder + "; color: " + th.InfoText},

				{"主提示", ".system-ui-alert.is-primary",
					"background-color: " + th.AccentSoft + "; border-color: " + th.AccentBorder + "; color: " + th.AccentText},
				{"默认提示", ".system-ui-alert.is-default",
					"background-color: " + th.SurfaceAlt + "; border-color: " + th.Border + "; color: " + th.Text},
				{"成功提示", ".system-ui-alert.is-success",
					"background-color: " + th.SuccessSoft + "; border-color: " + th.SuccessBorder + "; color: " + th.SuccessText},
				{"警告提示", ".system-ui-alert.is-warning",
					"background-color: " + th.WarningSoft + "; border-color: " + th.WarningBorder + "; color: " + th.WarningText},
				{"危险提示", ".system-ui-alert.is-danger",
					"background-color: " + th.DangerSoft + "; border-color: " + th.DangerBorder + "; color: " + th.DangerText},
				{"信息提示", ".system-ui-alert.is-info",
					"background-color: " + th.InfoSoft + "; border-color: " + th.InfoBorder + "; color: " + th.InfoText},
			}
			for _, c := range checks {
				if !strings.Contains(css, c.class) {
					t.Errorf("%s：CSS 缺少类 %s", c.name, c.class)
				}
				if !strings.Contains(css, c.decl) {
					t.Errorf("%s：CSS 缺少声明 %q", c.name, c.decl)
				}
			}
			for _, class := range []string{
				".system-ui-text-primary", ".system-ui-text-default", ".system-ui-text-success",
				".system-ui-text-warning", ".system-ui-text-danger", ".system-ui-text-info",
			} {
				if !strings.Contains(css, class) {
					t.Errorf("CSS 缺少语义文字类 %s", class)
				}
			}
		})
	}
}

// 字符串级测试看不出级联顺序：内容区的 .system-ui-actions .system-ui-btn 会覆盖基础
// 按钮底色。这里真渲染一颗语义按钮，从边框盒中心取色确认最终颜色。
// scopeCSS 必须给每条顶层选择器加根作用域，同时保留注释与 html 特例。
func TestScopeCSS(t *testing.T) {
	got := scopeCSS("/* top { } */\nhtml { a: b; }\n.a, .b { c: d; }\n/* keep { */\n.a .b { e: f; }", uiRootClass)
	want := "/* top { } */\nhtml." + uiRootClass + " { a: b; }\nhtml." + uiRootClass + " .a, html." +
		uiRootClass + " .b { c: d; }\n/* keep { */\nhtml." + uiRootClass + " .a .b { e: f; }"
	if got != want {
		t.Fatalf("scopeCSS 输出 =\n%s\n期望:\n%s", got, want)
	}
}

// 生成的样式必须全部挂在 html.system-ui 下；只改类名前缀不够，
// 根作用域把组件规则的特异度抬到 0,2,1，第三方 .panel div 压不过。
func TestThemeCSSScoped(t *testing.T) {
	css := Light().css(24, 56, 24, true)
	for _, want := range []string{
		"html." + uiRootClass + " { background-color:",
		"html." + uiRootClass + " .system-ui-btn {",
		"html." + uiRootClass + " .system-ui-actions .system-ui-btn {",
		"html." + uiRootClass + " .system-ui-page {",
		"html." + uiRootClass + " .system-ui-status-track {",
		"html." + uiRootClass + " .system-ui-status-seg {",
		"html." + uiRootClass + " .system-ui-status-right {",
		"html." + uiRootClass + " .system-ui-textview {",
		"html." + uiRootClass + " .system-ui-select {",
		"html." + uiRootClass + " .system-ui-check {",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS 缺少带根作用域的选择器: %s", want)
		}
	}
	if !strings.Contains(css, "padding-right: 280px") {
		t.Error("状态栏没有按 Theme.StatusRightWidth 预留右侧消息宽度")
	}
	if strings.Contains(css, ".ui-") {
		t.Error("CSS 里仍有旧 ui- 类名")
	}
}

// 根作用域要真的挡住第三方后代选择器：.panel div（0,1,1）本来能压过
// .system-ui-btn（0,1,0），挂到 html.system-ui 下之后组件规则（0,2,1）胜出。
func TestRootScopeBeatsThirdPartyDescendant(t *testing.T) {
	vp, buf := engine.NewBufferedViewport(160, 80)
	doc, err := engine.OpenDocument(vp, `<style>
		.panel div { background-color: #FF0000; }
	</style>
	<div class="panel"><div class="system-ui-btn" style="width:80px;height:24px"></div></div>`)
	if err != nil {
		t.Fatalf("打开文档失败: %v", err)
	}
	Install(doc, Spec{})
	doc.Refresh()
	engine.RenderNode(buf, doc)

	el := doc.QuerySelector(".system-ui-btn")
	if el == nil {
		t.Fatal("找不到按钮")
	}
	b := el.GetBoundingClientRect()
	x := (b.Left().Pixel() + b.Right().Pixel()) / 2
	y := (b.Top().Pixel() + b.Bottom().Pixel()) / 2
	r, g, bl, _, ok := buf.ColorAt(x, y)
	if !ok {
		t.Fatalf("取色失败: (%d,%d)", x, y)
	}
	if r == 0xFF && g == 0 && bl == 0 {
		t.Fatal("第三方 .panel div 覆盖了组件底色：根作用域没有生效")
	}
}

func TestSemanticButtonPaint(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
	}{
		{"light", Light()},
		{"dark", Dark()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			variants := []struct {
				name  string
				class string
				want  string
			}{
				{"primary", "is-primary", tc.th.Accent},
				{"default", "is-default", tc.th.Field},
				{"success", "is-success", tc.th.Success},
				{"warning", "is-warning", tc.th.Warning},
				{"danger", "is-danger", tc.th.Danger},
				{"info", "is-info", tc.th.Info},
			}
			for _, v := range variants {
				t.Run(v.name, func(t *testing.T) {
					th := tc.th
					vp, buf := engine.NewBufferedViewport(200, 120)
					doc, err := engine.OpenDocument(vp,
						`<div class="system-ui-actions"><div class="system-ui-btn `+v.class+`" style="width:80px;height:24px"></div></div>`)
					if err != nil {
						t.Fatalf("打开文档失败: %v", err)
					}
					Install(doc, Spec{Theme: &th})
					doc.Refresh()
					engine.RenderNode(buf, doc)

					el := doc.QuerySelector(".system-ui-btn")
					if el == nil {
						t.Fatal("找不到按钮")
					}
					box := el.GetBoundingClientRect()
					x := (box.Left().Pixel() + box.Right().Pixel()) / 2
					y := (box.Top().Pixel() + box.Bottom().Pixel()) / 2
					r, g, b, a, ok := buf.ColorAt(x, y)
					if !ok {
						t.Fatalf("取色失败: (%d,%d)", x, y)
					}
					wr, wg, wb := mustRGB(t, v.want)
					if r != wr || g != wg || b != wb || a != 255 {
						t.Fatalf("按钮底色 = #%02X%02X%02X（alpha %d），期望 %s",
							r, g, b, a, v.want)
					}
				})
			}
		})
	}
}

// 禁用输入控件必须「沉」到画布档：深色主题里 SurfaceAlt 是悬浮工具栏
// 的亮档，禁用字段若还走它，会比可用的更亮（旧 bug）。像素守卫：
// 禁用 input 底色 = Bg，且不等于 SurfaceAlt。
func TestDisabledFieldSinks(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
	}{
		{"light", Light()},
		{"dark", Dark()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := tc.th
			vp, buf := engine.NewBufferedViewport(200, 80)
			doc, err := engine.OpenDocument(vp,
				`<div style="background-color:`+th.Surface+`"><input class="system-ui-input" disabled style="width:120px"></div>`)
			if err != nil {
				t.Fatalf("打开文档失败: %v", err)
			}
			Install(doc, Spec{Theme: &th})
			doc.Refresh()
			engine.RenderNode(buf, doc)

			el := doc.QuerySelector(".system-ui-input")
			if el == nil {
				t.Fatal("找不到输入框")
			}
			box := el.GetBoundingClientRect()
			x := (box.Left().Pixel() + box.Right().Pixel()) / 2
			y := (box.Top().Pixel() + box.Bottom().Pixel()) / 2
			r, g, b, _, ok := buf.ColorAt(x, y)
			if !ok {
				t.Fatalf("取色失败: (%d,%d)", x, y)
			}
			wr, wg, wb := mustRGB(t, th.Bg)
			if r != wr || g != wg || b != wb {
				t.Fatalf("禁用输入底色 = #%02X%02X%02X，期望画布档 %s", r, g, b, th.Bg)
			}
			sr, sg, sb := mustRGB(t, th.SurfaceAlt)
			if r == sr && g == sg && b == sb {
				t.Fatalf("禁用输入底色撞上悬浮档 %s：禁用要沉不要浮", th.SurfaceAlt)
			}
			// 描边退回弱档：深色里 Field==Bg，中心点区分不了「规则没生效」，
			// 但描边 Border(#2B3A4F) ≠ BorderStrong(#334155) 能区分。左缘
			// 竖直中段是 1px 直边（避开圆角），扫描取第一个非底色像素。
			br, bg2, bb := mustRGB(t, th.Border)
			hit := false
			for x := box.Left().Pixel(); x <= box.Left().Pixel()+4 && !hit; x++ {
				rr, gg, bl, _, ok := buf.ColorAt(x, y)
				if ok && rr == br && gg == bg2 && bl == bb {
					hit = true
				}
			}
			if !hit {
				t.Fatalf("禁用输入描边不是弱档 %s：:disabled 规则可能没生效", th.Border)
			}
		})
	}
}

// 可点状态段的悬停底色是主题令牌：像素区分「has-action 规则真生效」
// 与「只挂了类名没样式」。
func TestStatusSegActionHover(t *testing.T) {
	th := Light()
	vp, buf := engine.NewBufferedViewport(240, 90)
	doc, err := engine.OpenDocument(vp, `<div>内容</div>`)
	if err != nil {
		t.Fatalf("打开文档失败: %v", err)
	}
	Install(doc, Spec{Theme: &th, StatusSegments: []StatusSegment{
		{Key: "go", Text: "点我", Width: 100,
			Action: Action{Run: func() error { return nil }}},
	}})
	doc.Refresh()
	segs := doc.QuerySelectorAll(".system-ui-status-seg")
	if len(segs) != 1 {
		t.Fatalf("状态段数 = %d, want 1", len(segs))
	}
	box := segs[0].GetBoundingClientRect()
	// 取段盒右侧空白区（文字靠左且首段收内边距），避开字形。
	x := (box.Left().Pixel()+box.Right().Pixel())/2 + 30
	y := (box.Top().Pixel() + box.Bottom().Pixel()) / 2
	sr, sg, sb := mustRGB(t, th.Surface)

	engine.RenderNode(buf, doc)
	if r, g, b, _, ok := buf.ColorAt(x, y); !ok || r != sr || g != sg || b != sb {
		t.Fatalf("悬停前底色 = #%02X%02X%02X，期望 Surface %s", r, g, b, th.Surface)
	}
	// 悬停：has-action 段换 $hover$ 底色。
	engine.OnDocumentMouseMove(doc, x, y)
	doc.Refresh()
	engine.RenderNode(buf, doc)
	hr, hg, hb := mustRGB(t, th.Hover)
	if r, g, b, _, ok := buf.ColorAt(x, y); !ok || r != hr || g != hg || b != hb {
		t.Fatalf("悬停底色 = #%02X%02X%02X，期望 Hover %s", r, g, b, th.Hover)
	}
	// 移开：回 Surface。
	engine.OnDocumentMouseMove(doc, x, 5)
	doc.Refresh()
	engine.RenderNode(buf, doc)
	if r, g, b, _, ok := buf.ColorAt(x, y); !ok || r != sr || g != sg || b != sb {
		t.Fatalf("移开后底色 = #%02X%02X%02X，期望 Surface %s", r, g, b, th.Surface)
	}
}

func mustRGB(t *testing.T, s string) (uint8, uint8, uint8) {
	t.Helper()
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		t.Fatalf("颜色格式错误: %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		t.Fatalf("颜色格式错误: %q: %v", s, err)
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v)
}

// 语义色的对比度由主题负责：实底上的 On*、淡底上的 *Text 都要达到
// WCAG AA 的 4.5:1（组件字号多为 12–14px，不能按大字号放宽）。
func TestSemanticContrast(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
	}{
		{"light", Light()},
		{"dark", Dark()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := tc.th
			pairs := []struct {
				name string
				fg   string
				bg   string
			}{
				{"主按钮", th.OnAccent, th.Accent},
				{"成功按钮", th.OnSuccess, th.Success},
				{"警告按钮", th.OnWarning, th.Warning},
				{"危险按钮", th.OnDanger, th.Danger},
				{"信息按钮", th.OnInfo, th.Info},
				{"主按钮悬停", th.OnAccent, th.AccentHover},
				{"成功按钮悬停", th.OnSuccess, th.SuccessHover},
				{"警告按钮悬停", th.OnWarning, th.WarningHover},
				{"危险按钮悬停", th.OnDanger, th.DangerHover},
				{"信息按钮悬停", th.OnInfo, th.InfoHover},
				{"默认按钮悬停", th.Text, th.SurfaceAlt},
				{"默认按钮按下", th.Text, th.Press},
				{"主徽标", th.AccentText, th.AccentSoft},
				{"成功徽标", th.SuccessText, th.SuccessSoft},
				{"警告徽标", th.WarningText, th.WarningSoft},
				{"危险徽标", th.DangerText, th.DangerSoft},
				{"信息徽标", th.InfoText, th.InfoSoft},
			}
			for _, p := range pairs {
				if ratio := contrastRatio(t, p.fg, p.bg); ratio < 4.5 {
					t.Errorf("%s：%s 在 %s 上对比度 %.2f，低于 4.5", p.name, p.fg, p.bg, ratio)
				}
			}
		})
	}
}

func contrastRatio(t *testing.T, fg, bg string) float64 {
	t.Helper()
	a, b := hexLuminance(t, fg), hexLuminance(t, bg)
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

func hexLuminance(t *testing.T, s string) float64 {
	t.Helper()
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		t.Fatalf("颜色格式错误: %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		t.Fatalf("颜色格式错误: %q: %v", s, err)
	}
	channel := func(shift uint) float64 {
		c := float64(v>>shift&0xFF) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(16) + 0.7152*channel(8) + 0.0722*channel(0)
}

// 主题要真的经 Install 进到文档里，而不只是 css() 能拼出字符串。
// 取一块干净的画布（内容 div 右侧、状态栏上方——body 复位后外边距区没了，
// 得自己挑不压元素的点）：那是 html 的底色，正好验证样式表注入并参与级联。
func TestInstallAppliesThemeBackground(t *testing.T) {
	cases := []struct {
		name  string
		theme Theme
		want  [3]uint8
	}{
		{"light", Light(), [3]uint8{0xF1, 0xF5, 0xF9}},
		{"dark", Dark(), [3]uint8{0x0F, 0x17, 0x2A}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vp, buf := engine.NewBufferedViewport(160, 120)
			doc, err := engine.OpenDocument(vp,
				`<div style="width:40px;height:40px">x</div>`)
			if err != nil {
				t.Fatalf("打开文档失败: %v", err)
			}
			theme := tc.theme
			Install(doc, Spec{Theme: &theme})
			doc.Refresh()
			engine.RenderNode(buf, doc)

			r, g, b, a, ok := buf.ColorAt(100, 60)
			if !ok {
				t.Fatal("取色失败")
			}
			if r != tc.want[0] || g != tc.want[1] || b != tc.want[2] || a != 255 {
				t.Fatalf("画布底色 = #%02X%02X%02X（alpha %d），期望 #%02X%02X%02X",
					r, g, b, a, tc.want[0], tc.want[1], tc.want[2])
			}
		})
	}
}

// SetTheme 要在同一份文档上追加新主题，不能重建页面。连续切回浅色时，
// 后追加的浅色规则也要压过前面深色规则（同特异度后写胜出）。
func TestSetThemeSwitchesBackground(t *testing.T) {
	vp, buf := engine.NewBufferedViewport(160, 120)
	doc, err := engine.OpenDocument(vp, `<div style="width:40px;height:40px">x</div>`)
	if err != nil {
		t.Fatalf("打开文档失败: %v", err)
	}
	sh := Install(doc, Spec{})
	if sh == nil {
		t.Fatal("Install 返回 nil")
	}

	check := func(name string, want [3]uint8) {
		t.Helper()
		doc.Refresh()
		engine.RenderNode(buf, doc)
		r, g, b, a, ok := buf.ColorAt(100, 60)
		if !ok {
			t.Fatalf("%s: 取色失败", name)
		}
		if r != want[0] || g != want[1] || b != want[2] || a != 255 {
			t.Fatalf("%s 画布底色 = #%02X%02X%02X（alpha %d），期望 #%02X%02X%02X",
				name, r, g, b, a, want[0], want[1], want[2])
		}
	}

	check("初始浅色", [3]uint8{0xF1, 0xF5, 0xF9})
	sh.SetTheme(Dark())
	check("切换深色", [3]uint8{0x0F, 0x17, 0x2A})
	sh.SetTheme(Light())
	check("切回浅色", [3]uint8{0xF1, 0xF5, 0xF9})
}

// colorEq 断言两个 Color 的 RGBA 一致（want 为 nil 时断言 got 也为 nil）。
func colorEq(t *testing.T, name string, got, want engine.Color) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s = 非 nil，期望未设置", name)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s 未设置，期望 %v", name, want)
	}
	gr, gg, gb, ga := got.RGBA()
	wr, wg, wb, wa := want.RGBA()
	if gr != wr || gg != wg || gb != wb || ga != wa {
		t.Fatalf("%s = %02X%02X%02X%02X，期望 %02X%02X%02X%02X", name, gr, gg, gb, ga, wr, wg, wb, wa)
	}
}

// 提示气泡是绘制层、拿不到级联样式，配色由 Install / SetTheme 按主题令牌
// 下发（浮层底色 + 浮层描边 + 主文字）——浅色主题上是浅底深字，深色相反。
func TestTooltipColorsFollowTheme(t *testing.T) {
	doc := newTestDoc(t, 640, 480, `<div>内容</div>`)
	dk := Dark()
	sh := Install(doc, Spec{Theme: &dk})
	c := engine.TooltipColorsOf(doc)
	colorEq(t, "深色底色", c.Background, engine.ParseColor(dk.Surface))
	colorEq(t, "深色描边", c.Border, engine.ParseColor(dk.BorderStrong))
	colorEq(t, "深色文字", c.Text, engine.ParseColor(dk.Text))

	lt := Light()
	sh.SetTheme(lt)
	c = engine.TooltipColorsOf(doc)
	colorEq(t, "浅色底色", c.Background, engine.ParseColor(lt.Surface))
	colorEq(t, "浅色文字", c.Text, engine.ParseColor(lt.Text))
}

// 控件条（NewBar）的样式必须跟着令牌走：条高取 Theme.BarHeight，条内控件
// 靠推导出的上下内边距垂直居中（写死数字的话换主题就散了），并且带上主题
// 底色与分隔线；textarea 的平铺变体（贴边铺满的编辑区）不带描边与圆角。
func TestBarThemeCSS(t *testing.T) {
	th := Light()
	css := th.css(32, 72, 24, true)
	i := strings.Index(css, " .system-ui-bar {")
	if i < 0 {
		t.Fatal("CSS 里没有 .system-ui-bar 规则")
	}
	end := i + 320
	if end > len(css) {
		end = len(css)
	}
	rule := css[i:end]
	down := (th.BarHeight - th.ButtonHeight) / 2
	for _, want := range []string{
		"height: " + strconv.Itoa(th.BarHeight) + "px",
		"padding: " + strconv.Itoa(down) + "px",
		"line-height: " + strconv.Itoa(th.ButtonHeight) + "px",
		"white-space: nowrap",
	} {
		if !strings.Contains(rule, want) {
			t.Errorf("控件条规则缺少 %s，规则体：\n%s", want, rule)
		}
	}
	if !strings.Contains(css, " .system-ui-textarea.is-plain {") {
		t.Error("CSS 里没有 textarea 的平铺变体 .system-ui-textarea.is-plain")
	}

	// 换一套条高，内边距要跟着变（否则控件不在条里居中）。
	custom := th
	custom.BarHeight = 64
	if got := custom.css(32, 72, 24, true); !strings.Contains(got, "height: 64px; padding: "+strconv.Itoa((64-th.ButtonHeight)/2)+"px") {
		t.Error("BarHeight 改 64 后条内边距没跟着推导")
	}
	// 零值走 withDefaults 补齐（漏填不该生成 height: 0px 的条）。
	filled := Theme{}.withDefaults()
	if filled.BarHeight != th.BarHeight {
		t.Fatalf("BarHeight 默认值 = %d, want %d", filled.BarHeight, th.BarHeight)
	}
	if got := filled.css(32, 72, 24, true); !strings.Contains(got, "height: "+strconv.Itoa(th.BarHeight)+"px") {
		t.Error("补齐默认值后控件条高度不对")
	}
}

// NoBodyReset 只跳过 body 复位块：画布底色（html background-color）没有
// 与页面排版冲突的语义，无论开关都注入。
func TestCSSWithoutBodyReset(t *testing.T) {
	css := Light().css(32, 72, 24, false)
	if strings.Contains(css, "html.system-ui body {") {
		t.Fatal("resetBody=false 时不该注入 body 复位规则")
	}
	if !strings.Contains(css, "html.system-ui { background-color: "+Light().Bg) {
		t.Fatal("跳过 body 复位时画布底色仍要注入")
	}
	// 开关打开时复位规则照常出现（0,2,1 特异度，页面裸 body 规则盖不过）。
	if on := Light().css(32, 72, 24, true); !strings.Contains(on, "html.system-ui body {") {
		t.Fatal("resetBody=true 时要注入 body 复位规则")
	}
}

// 装饰元素不该吃掉点击：body 首尾的占位块（DOM 序在页面内容之后）与栏内
// 1px 分隔线都要 pointer-events:none——否则浮在内容上的控件条看得见点不到。
// 反过来，栏与控件条本身必须保持可命中。
func TestDecorativeCSSPointerEvents(t *testing.T) {
	css := Light().css(32, 72, 24, true)
	// ruleBody 返回选择器后第一条规则的正文（到第一个 } 为止）。
	ruleBody := func(sel string) string {
		t.Helper()
		i := strings.Index(css, sel)
		if i < 0 {
			t.Fatalf("CSS 里没有 %s", sel)
		}
		j := strings.Index(css[i:], "}")
		if j < 0 {
			t.Fatalf("%s 的规则没有结尾", sel)
		}
		return css[i : i+j]
	}
	for _, sel := range []string{
		".system-ui-spacer-top {", ".system-ui-spacer-bottom {", ".system-ui-hairline {",
	} {
		if !strings.Contains(ruleBody(sel), "pointer-events: none") {
			t.Errorf("%s 缺 pointer-events: none（会吃掉下面内容的点击）", sel)
		}
	}
	for _, sel := range []string{" .system-ui-toolbar {", " .system-ui-menu {", " .system-ui-bar {"} {
		if strings.Contains(ruleBody(sel), "pointer-events: none") {
			t.Errorf("%s 不该整条不可点", sel)
		}
	}
}
