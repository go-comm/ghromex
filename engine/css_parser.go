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
	// important 标记该声明带 !important：级联时重要声明整体压过普通声明
	// （且作者的重要声明压过 UA 的重要声明，见 style_cascade.go）。
	important bool
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

// ParseCSS 解析一段 CSS 文本。不支持的语法（@media 等）会被跳过而不报错；
// @import 只被识别（不加载）——需要加载外部样式表时走 parseCSSRules。
func ParseCSS(src string) *Stylesheet {
	rules, _ := parseCSSRules(src)
	return &Stylesheet{rules: rules}
}

// parseCSSRules 解析 CSS，返回规则与 @import 引用（原样，未解析成绝对地址；
// 解析与加载由文档层负责，见 htmlDocument.addStylesheetText）。
//
// at-rule 分两类：
//   - 没有块、以分号结束的语句（@import / @charset…）：读到分号为止，
//     @import 的 URL 记进 imports；
//   - 带块的 at-rule（@media / @supports…）：整块跳过（引擎不分媒体）。
func parseCSSRules(src string) (rules []cssRule, imports []string) {
	src = stripCSSComments(src)
	i := 0
	for i < len(src) {
		open := strings.IndexByte(src[i:], '{')
		semi := strings.IndexByte(src[i:], ';')
		// 先收掉「分号在花括号之前」的语句。不做这一步的话，@import 会和
		// 下一条规则拼成 "@import url(x.css); body" 一起被当成 at-rule 跳过
		// ——@import 之后的规则会全部不生效（页面看起来「样式只加载了一半」）。
		if semi >= 0 && (open < 0 || semi < open) {
			stmt := strings.TrimSpace(src[i : i+semi])
			i += semi + 1
			if ref, ok := parseImportRef(stmt); ok {
				imports = append(imports, ref)
			}
			continue
		}
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
			rules = append(rules, cssRule{
				sels:  []*Selector{sel},
				decls: decls,
				order: len(rules),
			})
		}
	}
	return rules, imports
}

// parseImportRef 从一条 @import 语句里取出样式表引用。
//
// 支持 @import "x.css" / @import url(x.css) / @import url('x.css')，以及
// 后面的媒体查询（忽略——引擎不分媒体，与 @media 的处理一致）。
func parseImportRef(stmt string) (string, bool) {
	const keyword = "@import"
	if len(stmt) < len(keyword) || !strings.EqualFold(stmt[:len(keyword)], keyword) {
		return "", false
	}
	s := strings.TrimSpace(stmt[len(keyword):])
	if strings.HasPrefix(strings.ToLower(s), "url(") {
		end := strings.IndexByte(s, ')')
		if end < 0 {
			return "", false
		}
		s = strings.TrimSpace(s[4:end])
	} else {
		if s == "" || (s[0] != '"' && s[0] != '\'') {
			return "", false
		}
		end := strings.IndexByte(s[1:], s[0])
		if end < 0 {
			return "", false
		}
		s = s[1 : 1+end]
	}
	// url("x.css") 与 @import 'x.css' 都会带引号（引号不是地址的一部分）。
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
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
		// !important 标记：剥离后单独记录——此前整条声明因值里带
		// "!important" 而解析失败被丢弃（作者写 color:#f00 !important 颜色
		// 根本不生效）。
		important := false
		if i := strings.LastIndex(strings.ToLower(value), "!important"); i >= 0 {
			important = true
			value = strings.TrimSpace(value[:i])
		}
		if prop == "" || value == "" {
			continue
		}
		out = append(out, simpleDecl{prop: prop, value: value, important: important})
	}
	return out
}

// ParseInlineStyle 解析 style="..." 属性值。
func ParseInlineStyle(text string) []simpleDecl {
	return parseDeclarations(text)
}

// splitTopLevel 按分隔符切分，但忽略括号内的分隔符。
//
// box-shadow 的 rgba(0, 0, 0, .2) 是最典型的例子：里面的逗号属于颜色函数，
// 不是多条阴影的分隔符。按 strings.Split 切会把一条阴影切成四段，整条声明
// 直接失效。
func splitTopLevel(s string, sep byte) []string {
	var out []string
	depth := 0
	start := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if c == sep && depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// splitValueTokens 按空白切分，但不切括号内的空白。
// 用于 border 简写与 box-shadow：rgb(1, 2, 3) 必须保持为一个 token。
func splitValueTokens(s string) []string {
	var out []string
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if (c == ' ' || c == '\t' || c == '\n' || c == '\r') && depth == 0 {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// expandBoxValues 按 CSS 简写规则将 1~4 个值映射为 top right bottom left。
func expandBoxValues(value string) (top, right, bottom, left string) {
	parts := splitValueTokens(value)
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

// boxRect 解析四边简写值。任一分量非法（单位不支持等）时返回 ok=false——
// CSS 语义是「整条声明非法 → 忽略该声明」，而不是把四边清零（清零会抹掉
// UA 样式给出的 padding/margin）；忽略后仍保留 UA 值，与长写属性同口径。
func boxRect(top, right, bottom, left string) (Rect, bool) {
	ts, rs, bs, ls := parseLength(top), parseLength(right), parseLength(bottom), parseLength(left)
	if ts == nil || rs == nil || bs == nil || ls == nil {
		return nil, false
	}
	return NewRect(ls, rs, ts, bs), true
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

// ParseColor 解析一个 CSS 颜色值：#rgb / #rgba / #rrggbb / #rrggbbaa /
// rgb() / rgba() / 常见颜色名；无法解析时返回 nil。
//
// 给应用把「主题里的颜色字符串」变成引擎 Color 用——绘制层 API（如
// SetTooltipColors）收的是 Color 而不是 CSS 文本。
func ParseColor(v string) Color { return parseColor(v) }

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

// parseOverflow 解析 overflow 关键字，非法值回退 visible（CSS 默认）。
func parseOverflow(v string) Overflow {
	switch v {
	case "hidden", "clip":
		return OverflowHidden
	case "scroll":
		return OverflowScroll
	case "auto":
		return OverflowAuto
	default: // visible / 非法值
		return OverflowVisible
	}
}

// parseLineHeight 解析 line-height：normal / 无单位数字（倍数）/ 百分比
// （倍数）/ px（绝对长度）。
//
// 百分比与无单位数字语义相同——都相对**元素自身**的字号解析，且继承时
// 继承的是倍数本身（父级 1.5 + 子级 font-size:32px → 子级行高 48px），
// 故两者归一为倍数形态。非法值（负数、未知单位）返回 ok=false：CSS 语义
// 是「整条声明非法 → 忽略该声明」，保留继承来的值。
func parseLineHeight(v string) (LineHeight, bool) {
	switch v {
	case "normal":
		return NewLineHeightNormal(), true
	case "":
		return LineHeight{}, false
	}
	if strings.HasSuffix(v, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 32)
		if err != nil || f < 0 {
			return LineHeight{}, false
		}
		return NewLineHeightFactor(float32(f / 100)), true
	}
	if strings.HasSuffix(v, "px") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 32)
		if err != nil || f < 0 {
			return LineHeight{}, false
		}
		return NewLineHeightLength(float32(f)), true
	}
	// 无单位数字：倍数（如 1.5）。带未知单位（em/rem/vh…）暂不支持，
	// 整条声明忽略。
	if f, err := strconv.ParseFloat(v, 32); err == nil {
		if f < 0 {
			return LineHeight{}, false
		}
		return NewLineHeightFactor(float32(f)), true
	}
	return LineHeight{}, false
}

// parseVerticalAlign 解析 vertical-align。长度/百分比形式归为
// VerticalAlignRaise（相对基线抬高；百分比相对元素自身的 line-height）。
func parseVerticalAlign(v string) (VerticalAlign, Size, bool) {
	switch v {
	case "baseline":
		return VerticalAlignBaseline, nil, true
	case "top":
		return VerticalAlignTop, nil, true
	case "middle":
		return VerticalAlignMiddle, nil, true
	case "bottom":
		return VerticalAlignBottom, nil, true
	case "text-top":
		return VerticalAlignTextTop, nil, true
	case "text-bottom":
		return VerticalAlignTextBottom, nil, true
	case "sub":
		return VerticalAlignSub, nil, true
	case "super":
		return VerticalAlignSuper, nil, true
	}
	if strings.HasSuffix(v, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 32)
		if err != nil {
			return VerticalAlignBaseline, nil, false
		}
		return VerticalAlignRaise, NewSize(SIZE_PERCENT, 0, float32(f)), true
	}
	if strings.HasSuffix(v, "px") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "px"))
		if err != nil {
			return VerticalAlignBaseline, nil, false
		}
		return VerticalAlignRaise, NewSize(SIZE_PIXEL, n, 0), true
	}
	return VerticalAlignBaseline, nil, false
}

// parseBorderSpacing 解析 border-spacing 的 1~2 值。合法分量必须是
// 非负、非百分比、非 auto 的长度（px/pt/dp/无单位）；非法返回 ok=false，
// 调用方整条声明忽略——CSS 语义是「一条声明要么全部生效要么全部无效」，
// 只写一半间距比不写更糟。
func parseBorderSpacing(v string) (h, vSp Size, ok bool) {
	fields := strings.Fields(v)
	if len(fields) == 0 || len(fields) > 2 {
		return nil, nil, false
	}
	h = parseLength(fields[0])
	if !validSpacingSize(h) {
		return nil, nil, false
	}
	vSp = h
	if len(fields) == 2 {
		vSp = parseLength(fields[1])
		if !validSpacingSize(vSp) {
			return nil, nil, false
		}
	}
	return h, vSp, true
}

// validSpacingSize 判断分量是否是合法的间距长度。
func validSpacingSize(s Size) bool {
	if s == nil {
		return false
	}
	switch s.Type() {
	case SIZE_PERCENT, SIZE_AUTO:
		return false
	}
	return resolveLen(s, 0) >= 0
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
		case "table":
			comp.SetDisplay(DisplayTable)
		case "inline-table":
			comp.SetDisplay(DisplayInlineTable)
		case "table-caption":
			comp.SetDisplay(DisplayTableCaption)
		case "table-column":
			comp.SetDisplay(DisplayTableColumn)
		case "table-column-group":
			comp.SetDisplay(DisplayTableColumnGroup)
		case "table-header-group":
			comp.SetDisplay(DisplayTableHeaderGroup)
		case "table-footer-group":
			comp.SetDisplay(DisplayTableFooterGroup)
		case "table-row-group":
			comp.SetDisplay(DisplayTableRowGroup)
		case "table-row":
			comp.SetDisplay(DisplayTableRow)
		case "table-cell":
			comp.SetDisplay(DisplayTableCell)
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
	case "overflow":
		// 单值作用两轴；两值为 overflow-x overflow-y。非法值回退 visible。
		fields := strings.Fields(v)
		if len(fields) == 0 {
			return
		}
		ox := parseOverflow(fields[0])
		comp.SetOverflowX(ox)
		if len(fields) > 1 {
			comp.SetOverflowY(parseOverflow(fields[1]))
		} else {
			comp.SetOverflowY(ox)
		}
	case "overflow-x":
		comp.SetOverflowX(parseOverflow(v))
	case "overflow-y":
		comp.SetOverflowY(parseOverflow(v))
	case "white-space":
		// 只区分 pre 与其余值（normal/nowrap/…）：对照页只需要 pre 的
		// 「保留空白 + 不软换行」，其余按 CSS 默认的 normal 处理。
		if v == "pre" {
			comp.SetWhiteSpace(WhiteSpacePre)
		} else {
			comp.SetWhiteSpace(WhiteSpaceNormal)
		}
	case "pointer-events":
		// 只区分 none 与其余值：SVG 那套 fill/stroke/visiblePainted 精细档
		// 引擎用不上，非 none 一律按 CSS 默认 auto（可命中）处理。
		if v == "none" {
			comp.SetPointerEvents(PointerEventsNone)
		} else {
			comp.SetPointerEvents(PointerEventsAuto)
		}
	case "border-collapse":
		// 只区分 collapse 与其余值（separate 为 CSS 默认）。语义见
		// BorderCollapse 的注释：collapse 时每个单元格只摊到半个边框。
		if v == "collapse" {
			comp.SetBorderCollapse(BorderCollapseCollapse)
		} else {
			comp.SetBorderCollapse(BorderCollapseSeparate)
		}
	case "border-spacing":
		// 1~2 个长度（单值同写两轴）。CSS 不接受负值与百分比，auto 非法：
		// 任一分量非法则整条声明忽略（保留继承前的默认 0 / UA 2px）。
		if h, vSp, ok := parseBorderSpacing(v); ok {
			comp.SetBorderSpacing(h, vSp)
		}
	case "empty-cells":
		// 只区分 hide 与其余值（show 为 CSS 默认）。语义见 EmptyCells。
		if v == "hide" {
			comp.SetEmptyCells(EmptyCellsHide)
		} else {
			comp.SetEmptyCells(EmptyCellsShow)
		}
	case "caption-side":
		// 只区分 bottom 与其余值（top 为 CSS 默认；left/right 属竖排表格，
		// 引擎按 top 处理）。
		if v == "bottom" {
			comp.SetCaptionSide(CaptionSideBottom)
		} else {
			comp.SetCaptionSide(CaptionSideTop)
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
		if rect, ok := boxRect(t, r, b, l); ok {
			comp.SetMargin(rect)
		}
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
		if rect, ok := boxRect(t, r, b, l); ok {
			comp.SetPadding(rect)
		}
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
		if rect, ok := boxRect(t, r, b, l); ok {
			comp.SetBorderStyleWidth(rect)
		}
	case "border-color":
		applyBorderColorShorthand(comp, v)
	case "border-style":
		applyBorderStyleShorthand(comp, v)
	case "border":
		applyBorderShorthand(comp, value)
	case "border-top":
		applyBorderSideShorthand(comp, BorderSideTop, value)
	case "border-right":
		applyBorderSideShorthand(comp, BorderSideRight, value)
	case "border-bottom":
		applyBorderSideShorthand(comp, BorderSideBottom, value)
	case "border-left":
		applyBorderSideShorthand(comp, BorderSideLeft, value)
	case "border-top-width":
		setBorderWidthEdge(comp, edgeTop, v)
	case "border-right-width":
		setBorderWidthEdge(comp, edgeRight, v)
	case "border-bottom-width":
		setBorderWidthEdge(comp, edgeBottom, v)
	case "border-left-width":
		setBorderWidthEdge(comp, edgeLeft, v)
	case "border-top-color":
		setBorderColorEdge(comp, BorderSideTop, v)
	case "border-right-color":
		setBorderColorEdge(comp, BorderSideRight, v)
	case "border-bottom-color":
		setBorderColorEdge(comp, BorderSideBottom, v)
	case "border-left-color":
		setBorderColorEdge(comp, BorderSideLeft, v)
	case "border-top-style":
		setBorderStyleEdge(comp, BorderSideTop, v)
	case "border-right-style":
		setBorderStyleEdge(comp, BorderSideRight, v)
	case "border-bottom-style":
		setBorderStyleEdge(comp, BorderSideBottom, v)
	case "border-left-style":
		setBorderStyleEdge(comp, BorderSideLeft, v)
	case "box-shadow":
		if shadows, ok := parseBoxShadow(v); ok {
			comp.SetBoxShadow(shadows)
		}
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
	case "line-height":
		// 继承的是原始形态（倍数/绝对长度），解析成像素留到布局期——
		// 倍数要相对元素**自身**的字号解析，级联期拿不到最终字号。
		if lh, ok := parseLineHeight(v); ok {
			comp.SetLineHeight(lh)
		}
	case "vertical-align":
		if mode, off, ok := parseVerticalAlign(v); ok {
			comp.SetVerticalAlign(mode)
			if off != nil {
				comp.SetVerticalAlignOffset(off)
			}
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
	case "text-decoration":
		// 取列表中第一个可识别关键字（underline / line-through / none）；
		// 其余装饰（overline 等）暂不绘制，容忍忽略。
		recognized := false
		d := TextDecorationNone
		for _, part := range strings.Fields(v) {
			switch part {
			case "underline":
				d, recognized = TextDecorationUnderline, true
			case "line-through":
				d, recognized = TextDecorationLineThrough, true
			case "none":
				d, recognized = TextDecorationNone, true
			}
			if recognized {
				break
			}
		}
		if recognized {
			comp.SetTextDecoration(d)
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

// applyBorderShorthand 处理 border：宽度/样式/颜色同时写四边。
// 没出现的分量保持原值——CSS 的 initial 值会让「border:1px solid」把颜色
// 重置成 currentColor，但引擎的 UA 表单控件依赖原有颜色；保留原值更接近
// 此前的视觉，也让 author 能只覆盖宽度。
func applyBorderShorthand(comp CSSStyleDeclaration, value string) {
	for side := BorderSideTop; side <= BorderSideLeft; side++ {
		applyBorderSideShorthand(comp, side, value)
	}
}

// applyBorderSideShorthand 处理 border-top/right/bottom/left。
// 与 border 同一套 token 规则，只是作用域收窄到一条边。
func applyBorderSideShorthand(comp CSSStyleDeclaration, side BorderSide, value string) {
	for _, part := range splitValueTokens(value) {
		lower := strings.ToLower(part)
		switch lower {
		case "none", "hidden":
			comp.SetBorderStyleFor(side, BorderStyleNone)
		case "solid", "dashed", "dotted", "double", "groove", "ridge", "inset", "outset":
			// 引擎目前只画实线：其余线型按 solid 处理（与旧版 border 简写
			// 一致），至少宽度/颜色/单边语义是对的。
			comp.SetBorderStyleFor(side, BorderStyleSolid)
		case "currentcolor":
			// 显式 currentColor 存 nil，绘制期回退到元素文字色。
			comp.SetBorderColorFor(side, nil)
		default:
			if s := parseLength(lower); s != nil && s.Type() != SIZE_AUTO {
				comp.SetBorderStyleWidth(edgeRect(comp.BorderStyleWidth(),
					borderSideEdge(side), s))
				continue
			}
			if c, ok := parseBorderColorToken(lower); ok {
				comp.SetBorderColorFor(side, c)
			}
		}
	}
}

// parseBorderColorToken 解析边框颜色；currentColor 合法但值是 nil，
// 由绘制层按元素文字色回退。第二个返回值区分「合法 currentColor」与
// 「非法 token」，避免把 currentColor 当成解析失败而丢掉整条声明。
func parseBorderColorToken(v string) (Color, bool) {
	if strings.EqualFold(strings.TrimSpace(v), "currentcolor") {
		return nil, true
	}
	c := parseColor(v)
	return c, c != nil
}

// applyBorderColorShorthand 处理 border-color 的 1~4 值展开。
// 任一分量非法时整条声明忽略（CSS 语义），避免只改一半边。
func applyBorderColorShorthand(comp CSSStyleDeclaration, value string) {
	t, r, b, l := expandBoxValues(value)
	var colors [4]Color
	for i, part := range []string{t, r, b, l} {
		c, ok := parseBorderColorToken(part)
		if !ok {
			return
		}
		colors[i] = c
	}
	comp.SetBorderColorFor(BorderSideTop, colors[0])
	comp.SetBorderColorFor(BorderSideRight, colors[1])
	comp.SetBorderColorFor(BorderSideBottom, colors[2])
	comp.SetBorderColorFor(BorderSideLeft, colors[3])
}

// applyBorderStyleShorthand 处理 border-style 的 1~4 值展开。
func applyBorderStyleShorthand(comp CSSStyleDeclaration, value string) {
	t, r, b, l := expandBoxValues(value)
	var styles [4]BorderStyle
	for i, part := range []string{t, r, b, l} {
		s, ok := parseBorderStyleKeyword(part)
		if !ok {
			return
		}
		styles[i] = s
	}
	comp.SetBorderStyleFor(BorderSideTop, styles[0])
	comp.SetBorderStyleFor(BorderSideRight, styles[1])
	comp.SetBorderStyleFor(BorderSideBottom, styles[2])
	comp.SetBorderStyleFor(BorderSideLeft, styles[3])
}

func parseBorderStyleKeyword(v string) (BorderStyle, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none", "hidden":
		return BorderStyleNone, true
	case "solid", "dashed", "dotted", "double", "groove", "ridge", "inset", "outset":
		return BorderStyleSolid, true
	}
	return BorderStyleNone, false
}

// borderSideEdge 把公开的边序号映射到内部盒模型下标。
// 两者顺序刻意保持一致（见 BorderSide 的注释），这里只做类型转换。
func borderSideEdge(side BorderSide) boxEdge {
	return boxEdge(side)
}

func setBorderWidthEdge(comp CSSStyleDeclaration, edge boxEdge, value string) {
	if s := parseLength(value); s != nil {
		comp.SetBorderStyleWidth(edgeRect(comp.BorderStyleWidth(), edge, s))
	}
}

func setBorderColorEdge(comp CSSStyleDeclaration, side BorderSide, value string) {
	if c, ok := parseBorderColorToken(value); ok {
		comp.SetBorderColorFor(side, c)
	}
}

func setBorderStyleEdge(comp CSSStyleDeclaration, side BorderSide, value string) {
	if s, ok := parseBorderStyleKeyword(value); ok {
		comp.SetBorderStyleFor(side, s)
	}
}

// parseShadowLength 只接受绝对长度（px/pt/dp）与 0；box-shadow 的模糊/扩散
// 半径不接受百分比，偏移百分比在引擎里也没有可用的解析基数，故整体拒绝。
func parseShadowLength(v string) (int, bool) {
	s := parseLength(v)
	if s == nil {
		return 0, false
	}
	switch s.Type() {
	case SIZE_PIXEL:
		return s.Pixel(), true
	case SIZE_POINT:
		return s.Point() * 96 / 72, true
	case SIZE_DISPLAYPORT:
		return s.Displayport() * 16, true
	}
	return 0, false
}

// parseBoxShadow 解析 box-shadow: none | <shadow>#。
//
// 每条阴影的 token 可以是：inset、2~4 个长度（x y [blur [spread]]）和一个
// 颜色（位置不限）。颜色里的逗号由 splitTopLevel 保护。返回 ok=false 时
// 整条声明忽略，保留级联中上一条合法值。
func parseBoxShadow(value string) ([]Shadow, bool) {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "none" {
		return nil, true
	}
	segs := splitTopLevel(v, ',')
	shadows := make([]Shadow, 0, len(segs))
	for _, seg := range segs {
		if seg == "" {
			return nil, false
		}
		tokens := splitValueTokens(seg)
		var sh Shadow
		var lens []int
		colorSet := false
		for _, tok := range tokens {
			switch tok {
			case "inset":
				sh.Inset = true
				continue
			}
			if n, ok := parseShadowLength(tok); ok {
				lens = append(lens, n)
				continue
			}
			if tok == "currentcolor" {
				if colorSet {
					return nil, false
				}
				// nil 即 currentColor：绘制期用元素文字色。
				colorSet = true
				continue
			}
			if c := parseColor(tok); c != nil && !colorSet {
				sh.Color = c
				colorSet = true
				continue
			}
			return nil, false
		}
		if len(lens) < 2 || len(lens) > 4 {
			return nil, false
		}
		sh.X, sh.Y = lens[0], lens[1]
		if len(lens) >= 3 {
			if lens[2] < 0 {
				return nil, false // 模糊半径不能为负
			}
			sh.Blur = lens[2]
		}
		if len(lens) == 4 {
			sh.Spread = lens[3]
		}
		shadows = append(shadows, sh)
	}
	return shadows, true
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
// s 为 nil（声明非法：单位不支持等）时原样返回 r——CSS 语义是「非法声明被
// 忽略」，不是把该边清零（清零会抹掉 UA 样式值，与 setEdge 的处理不一致）。
func edgeRect(r Rect, edge boxEdge, s Size) Rect {
	if s == nil {
		return r
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
