package engine

import (
	"strconv"
	"strings"
)

// Stylesheet 是解析后的 CSS 规则集合。
type Stylesheet struct {
	rules []cssRule
}

type cssRule struct {
	sels  []*Selector
	decls []simpleDecl
	order int
}

type simpleDecl struct {
	prop  string
	value string
}

// Merge 将 other 的规则追加到 s（用于 <style> 多段合并）。
func (s *Stylesheet) Merge(other *Stylesheet) {
	if other == nil {
		return
	}
	for _, r := range other.rules {
		r.order = len(s.rules)
		s.rules = append(s.rules, r)
	}
}

// ParseCSS 解析一段 CSS 文本。不支持的语法（@media 等）会被跳过而不报错。
func ParseCSS(src string) *Stylesheet {
	src = stripCSSComments(src)
	sheet := &Stylesheet{}
	i := 0
	for i < len(src) {
		open := strings.IndexByte(src[i:], '{')
		if open < 0 {
			break
		}
		selText := strings.TrimSpace(src[i : i+open])
		bodyStart := i + open + 1
		close := strings.IndexByte(src[bodyStart:], '}')
		if close < 0 {
			break
		}
		body := src[bodyStart : bodyStart+close]
		i = bodyStart + close + 1

		if strings.HasPrefix(selText, "@") {
			continue
		}
		decls := parseDeclarations(body)
		if len(decls) == 0 {
			continue
		}
		for _, selStr := range strings.Split(selText, ",") {
			selStr = strings.TrimSpace(selStr)
			if selStr == "" {
				continue
			}
			sel := ParseSelector(selStr)
			if sel.isEmpty() {
				continue
			}
			sheet.rules = append(sheet.rules, cssRule{
				sels:  []*Selector{sel},
				decls: decls,
				order: len(sheet.rules),
			})
		}
	}
	return sheet
}

func stripCSSComments(src string) string {
	for {
		a := strings.Index(src, "/*")
		if a < 0 {
			return src
		}
		b := strings.Index(src[a+2:], "*/")
		if b < 0 {
			return src[:a]
		}
		src = src[:a] + src[a+2+b+2:]
	}
}

func parseDeclarations(body string) []simpleDecl {
	var out []simpleDecl
	for _, part := range strings.Split(body, ";") {
		colon := strings.IndexByte(part, ':')
		if colon < 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(part[:colon]))
		value := strings.TrimSpace(part[colon+1:])
		if prop == "" || value == "" {
			continue
		}
		out = append(out, simpleDecl{prop: prop, value: value})
	}
	return out
}

// ParseInlineStyle 解析 style="..." 属性值。
func ParseInlineStyle(text string) []simpleDecl {
	return parseDeclarations(text)
}

// expandBoxValues 按 CSS 简写规则将 1~4 个值映射为 top right bottom left。
func expandBoxValues(value string) (top, right, bottom, left string) {
	parts := strings.Fields(value)
	switch len(parts) {
	case 0:
		return "0px", "0px", "0px", "0px"
	case 1:
		return parts[0], parts[0], parts[0], parts[0]
	case 2:
		return parts[0], parts[1], parts[0], parts[1]
	case 3:
		return parts[0], parts[1], parts[2], parts[1]
	default:
		return parts[0], parts[1], parts[2], parts[3]
	}
}

func boxRect(top, right, bottom, left string) Rect {
	return NewRect(
		parseLength(left),
		parseLength(right),
		parseLength(top),
		parseLength(bottom),
	)
}

func parseLength(v string) Size {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "auto" {
		return NewAutoSize()
	}
	if v == "0" {
		return NewZeroSize()
	}
	if strings.HasSuffix(v, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 32)
		if err == nil {
			return NewSize(SIZE_PERCENT, 0, float32(f))
		}
		return nil
	}
	if strings.HasSuffix(v, "px") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "px"))
		if err == nil {
			return NewSize(SIZE_PIXEL, n, 0)
		}
		return nil
	}
	if strings.HasSuffix(v, "pt") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "pt"))
		if err == nil {
			return NewSize(SIZE_POINT, n, 0)
		}
		return nil
	}
	if strings.HasSuffix(v, "dp") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "dp"))
		if err == nil {
			return NewSize(SIZE_DISPLAYPORT, n, 0)
		}
		return nil
	}
	// 无单位数字按 px 处理
	if n, err := strconv.Atoi(v); err == nil {
		return NewSize(SIZE_PIXEL, n, 0)
	}
	return nil
}

var namedColors = map[string]Color{
	"black":       NewColor(0, 0, 0, 255),
	"white":       NewColor(255, 255, 255, 255),
	"red":         NewColor(220, 38, 38, 255),
	"green":       NewColor(22, 163, 74, 255),
	"blue":        NewColor(37, 99, 235, 255),
	"gray":        NewColor(128, 128, 128, 255),
	"grey":        NewColor(128, 128, 128, 255),
	"lightgray":   NewColor(200, 200, 200, 255),
	"lightgrey":   NewColor(200, 200, 200, 255),
	"darkgray":    NewColor(80, 80, 80, 255),
	"yellow":      NewColor(234, 179, 8, 255),
	"orange":      NewColor(249, 115, 22, 255),
	"purple":      NewColor(147, 51, 234, 255),
	"pink":        NewColor(236, 72, 153, 255),
	"brown":       NewColor(120, 81, 56, 255),
	"cyan":        NewColor(8, 179, 189, 255),
	"magenta":     NewColor(216, 55, 198, 255),
	"silver":      NewColor(192, 192, 192, 255),
	"transparent": NewColor(0, 0, 0, 0),
}

// parseColor 支持 #rgb / #rrggbb / #rrggbbaa / rgb() / rgba() / 常见颜色名。
func parseColor(v string) Color {
	v = strings.TrimSpace(strings.ToLower(v))
	if c, ok := namedColors[v]; ok {
		return c
	}
	if strings.HasPrefix(v, "#") {
		hex := v[1:]
		parseHex := func(s string) (uint8, bool) {
			n, err := strconv.ParseUint(s, 16, 8)
			return uint8(n), err == nil
		}
		switch len(hex) {
		case 3:
			r, ok1 := parseHex(string(hex[0]) + string(hex[0]))
			g, ok2 := parseHex(string(hex[1]) + string(hex[1]))
			b, ok3 := parseHex(string(hex[2]) + string(hex[2]))
			if ok1 && ok2 && ok3 {
				return NewColor(r, g, b, 255)
			}
		case 4:
			r, ok1 := parseHex(string(hex[0]) + string(hex[0]))
			g, ok2 := parseHex(string(hex[1]) + string(hex[1]))
			b, ok3 := parseHex(string(hex[2]) + string(hex[2]))
			a, ok4 := parseHex(string(hex[3]) + string(hex[3]))
			if ok1 && ok2 && ok3 && ok4 {
				return NewColor(r, g, b, a)
			}
		case 6, 8:
			var parts [4]uint8
			n := len(hex) / 2
			for k := 0; k < n; k++ {
				vv, err := strconv.ParseUint(hex[k*2:k*2+2], 16, 8)
				if err != nil {
					return nil
				}
				parts[k] = uint8(vv)
			}
			a := uint8(255)
			if n == 4 {
				a = parts[3]
				return NewColor(parts[0], parts[1], parts[2], a)
			}
			return NewColor(parts[0], parts[1], parts[2], a)
		}
		return nil
	}
	if strings.HasPrefix(v, "rgb") {
		open := strings.IndexByte(v, '(')
		closing := strings.IndexByte(v, ')')
		if open >= 0 && closing > open {
			inner := v[open+1 : closing]
			inner = strings.ReplaceAll(inner, ",", " ")
			fields := strings.Fields(inner)
			if len(fields) == 3 || len(fields) == 4 {
				var vals [4]uint8
				vals[3] = 255
				for k := 0; k < len(fields); k++ {
					f := fields[k]
					if k == 3 {
						alpha, err := strconv.ParseFloat(f, 64)
						if err == nil {
							vals[3] = uint8(alpha * 255)
						}
						continue
					}
					n, err := strconv.Atoi(f)
					if err != nil {
						return nil
					}
					if n < 0 {
						n = 0
					}
					if n > 255 {
						n = 255
					}
					vals[k] = uint8(n)
				}
				return NewColor(vals[0], vals[1], vals[2], vals[3])
			}
		}
	}
	return nil
}

// applyDecl 将单条声明写入样式对象。未识别的属性被忽略。
func applyDecl(comp CSSStyleDeclaration, prop, value string) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch prop {
	case "display":
		switch v {
		case "none":
			comp.SetDisplay(DisplayNone)
		case "inline":
			comp.SetDisplay(DisplayInline)
		case "inline-block":
			comp.SetDisplay(DisplayInlineBlock)
		case "block":
			comp.SetDisplay(DisplayBlock)
		}
	case "position":
		switch v {
		case "relative":
			comp.SetPosition(PositionRelative)
		case "absolute":
			comp.SetPosition(PositionAbsolute)
		case "fixed":
			comp.SetPosition(PositionFixed)
		default: // static / 非法值回退文档流
			comp.SetPosition(PositionStatic)
		}
	case "box-sizing":
		// 仅两种合法值；非法回退 CSS 默认 content-box
		if v == "border-box" {
			comp.SetBoxSizing(BoxSizingBorderBox)
		} else {
			comp.SetBoxSizing(BoxSizingContentBox)
		}
	case "top":
		comp.SetInset(edgeRect(comp.Inset(), edgeTop, parseLength(v)))
	case "right":
		comp.SetInset(edgeRect(comp.Inset(), edgeRight, parseLength(v)))
	case "bottom":
		comp.SetInset(edgeRect(comp.Inset(), edgeBottom, parseLength(v)))
	case "left":
		comp.SetInset(edgeRect(comp.Inset(), edgeLeft, parseLength(v)))
	case "z-index":
		// auto 及非法值按 0（定位层内与 z-index:0 同序，文档序决胜）
		if n, err := strconv.Atoi(v); err == nil {
			comp.SetZIndex(n)
		}
	case "width":
		if s := parseLength(v); s != nil {
			comp.SetWidth(s)
		}
	case "height":
		if s := parseLength(v); s != nil {
			comp.SetHeight(s)
		}
	case "margin":
		t, r, b, l := expandBoxValues(v)
		comp.SetMargin(boxRect(t, r, b, l))
	case "margin-top":
		comp.SetMargin(edgeRect(comp.Margin(), edgeTop, parseLength(v)))
	case "margin-right":
		comp.SetMargin(edgeRect(comp.Margin(), edgeRight, parseLength(v)))
	case "margin-bottom":
		comp.SetMargin(edgeRect(comp.Margin(), edgeBottom, parseLength(v)))
	case "margin-left":
		comp.SetMargin(edgeRect(comp.Margin(), edgeLeft, parseLength(v)))
	case "padding":
		t, r, b, l := expandBoxValues(v)
		comp.SetPadding(boxRect(t, r, b, l))
	case "padding-top":
		comp.SetPadding(edgeRect(comp.Padding(), edgeTop, parseLength(v)))
	case "padding-right":
		comp.SetPadding(edgeRect(comp.Padding(), edgeRight, parseLength(v)))
	case "padding-bottom":
		comp.SetPadding(edgeRect(comp.Padding(), edgeBottom, parseLength(v)))
	case "padding-left":
		comp.SetPadding(edgeRect(comp.Padding(), edgeLeft, parseLength(v)))
	case "border-width":
		t, r, b, l := expandBoxValues(v)
		comp.SetBorderStyleWidth(boxRect(t, r, b, l))
	case "border-color":
		if c := parseColor(v); c != nil {
			comp.SetBorderColor(c)
		}
	case "border-style":
		if v == "none" {
			comp.SetBorderStyle(BorderStyleNone)
		} else {
			comp.SetBorderStyle(BorderStyleSolid)
		}
	case "border":
		applyBorderShorthand(comp, value)
	case "background-color":
		if v == "transparent" || v == "none" {
			comp.SetBackgroundColor(nil)
		} else if c := parseColor(v); c != nil {
			comp.SetBackgroundColor(c)
		}
	case "background":
		for _, part := range strings.Split(value, " ") {
			if c := parseColor(part); c != nil {
				comp.SetBackgroundColor(c)
				break
			}
		}
	case "color":
		if c := parseColor(v); c != nil {
			comp.SetColor(c)
		}
	case "font-size":
		if s := parseLength(v); s != nil {
			comp.SetFontSize(s)
		}
	case "font-weight":
		if v == "bold" || v == "700" || v == "800" || v == "900" {
			comp.SetFontWeight(FontWeightBold)
		} else {
			comp.SetFontWeight(FontWeightNormal)
		}
	case "font-family":
		comp.SetFontFamily(strings.Trim(strings.TrimSpace(value), "\"'"))
	case "text-align":
		switch v {
		case "center":
			comp.SetTextAlign(TextAlignCenter)
		case "right", "end":
			comp.SetTextAlign(TextAlignRight)
		default: // left / start / justify（按左处）
			comp.SetTextAlign(TextAlignLeft)
		}
	case "border-radius":
		// 仅支持四角统一圆角：取斜杠前（水平半径）第一个长度值；百分比相对短边
		s := v
		if i := strings.IndexByte(s, '/'); i >= 0 {
			s = strings.TrimSpace(s[:i])
		}
		if f := strings.Fields(s); len(f) > 0 {
			s = f[0]
		}
		if sz := parseLength(s); sz != nil {
			comp.SetBorderRadius(sz)
		}
		// background-image 等暂未实现，解析容忍、渲染忽略
	}
}

func applyBorderShorthand(comp CSSStyleDeclaration, value string) {
	for _, part := range strings.Fields(value) {
		lower := strings.ToLower(part)
		switch lower {
		case "none", "hidden":
			comp.SetBorderStyle(BorderStyleNone)
		case "solid", "dashed", "dotted", "double", "groove", "ridge", "inset", "outset":
			comp.SetBorderStyle(BorderStyleSolid)
		default:
			if s := parseLength(lower); s != nil && s.Type() != SIZE_AUTO {
				comp.SetBorderStyleWidth(NewRect(s, s, s, s))
				continue
			}
			if c := parseColor(lower); c != nil {
				comp.SetBorderColor(c)
			}
		}
	}
}

type boxEdge int

const (
	edgeTop boxEdge = iota
	edgeRight
	edgeBottom
	edgeLeft
)

func setEdge(rect Rect, setter func(Size), v string) {
	if s := parseLength(v); s != nil {
		setter(s)
	}
}

// edgeRect 返回替换了指定边的 Rect 拷贝。
func edgeRect(r Rect, edge boxEdge, s Size) Rect {
	if s == nil {
		s = NewZeroSize()
	}
	var l, rt, t, b Size
	if rr, ok := r.(*rect); ok {
		l, rt, t, b = rr.left, rr.right, rr.top, rr.bottom
	} else if r != nil {
		l, rt, t, b = r.Left(), r.Right(), r.Top(), r.Bottom()
	} else {
		zero := NewZeroSize()
		l, rt, t, b = zero, zero, zero, zero
	}
	switch edge {
	case edgeTop:
		t = s
	case edgeRight:
		rt = s
	case edgeBottom:
		b = s
	case edgeLeft:
		l = s
	}
	return NewRect(l, rt, t, b)
}
