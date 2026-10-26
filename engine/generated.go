package engine

import (
	"strconv"
	"strings"
)

// 伪元素 ::before / ::after（CSS 2.1 §12.1 生成内容）。
//
// 实现口径：生成盒是**宿主的真实子元素**，插在 children 的首/尾参与
// 常规布局、绘制、命中与 SVG 导出；但它不是 DOM 节点——Children() 会
// 过滤掉，选择器（:first-child/:empty 等）、InnerText、查询 API 都看不到
// 它。样式不走进普通级联：由 style_cascade.go 按「宿主元素 + 伪元素」
// 单独匹配（p:hover::before 的 :hover 仍按宿主状态判定）。
//
// 支持范围：
//   - content: "文本" / '文本' / attr(name) / 多个分量拼接；
//   - content: none / normal → 不生成（CSS 默认值同样是「不生成」）；
//   - 其他值（counter()/url()/open-quote…）忽略，整条 content 若无有效
//     分量则不生成；
//   - 生成盒默认 display:inline，作者可用 display/width/height/padding/
//     border/background 等常规属性把它做成徽标、圆点、箭头之类。

const (
	pseudoElementBefore = "before"
	pseudoElementAfter  = "after"
)

// syncGeneratedContent 同步宿主的两个生成盒（样式已算好，base.computed 可用）。
func syncGeneratedContent(doc HTMLDocument, host *htmlElement, author *Stylesheet) {
	if !author.hasPseudoRules() {
		// 常见路径：整份样式表没有伪元素规则。只有曾经生成过盒子的宿主
		// 需要清理（样式表被替换/移除的情况），其余直接返回。
		if host.hasGenerated {
			dropGenerated(host, pseudoElementBefore)
			dropGenerated(host, pseudoElementAfter)
		}
		return
	}
	syncGenerated(doc, host, author, pseudoElementBefore)
	syncGenerated(doc, host, author, pseudoElementAfter)
}

// syncGenerated 处理单个伪元素：无 content / 不可生成时移除旧盒，否则
// 复用（或新建）生成盒并写入计算样式与文本。
func syncGenerated(doc HTMLDocument, host *htmlElement, author *Stylesheet, kind string) {
	if !canGenerateContent(host) {
		dropGenerated(host, kind)
		return
	}
	decls := collectPseudoDecls(doc, author, host.self, kind, originAuthor)
	comp, content, ok := generatedStyle(host, decls)
	if !ok {
		dropGenerated(host, kind)
		return
	}
	gen := host.genBefore
	if kind == pseudoElementAfter {
		gen = host.genAfter
	}
	if gen == nil {
		gen = newGeneratedElement(host, kind)
		if kind == pseudoElementBefore {
			host.genBefore = gen
		} else {
			host.genAfter = gen
		}
	}
	gen.computed = comp
	setGeneratedText(gen, content)
	placeGenerated(host, gen, kind)
}

// canGenerateContent 判断宿主元素是否支持 ::before/::after。
//
// 替换元素与表单控件（input/textarea/select/img…）没有伪元素；表格的
// 结构盒（table/row/row-group/column）由表格布局按标签角色解析 children，
// 插入生成盒会被误当成行/列——这些盒子不支持。单元格与题注支持
// （它们的内容走常规流布局）。
func canGenerateContent(host *htmlElement) bool {
	if host == nil || host.computed == nil {
		return false
	}
	switch host.computed.Display() {
	case DisplayNone,
		DisplayTable, DisplayInlineTable,
		DisplayTableRow, DisplayTableRowGroup,
		DisplayTableHeaderGroup, DisplayTableFooterGroup,
		DisplayTableColumn, DisplayTableColumnGroup:
		return false
	}
	switch strings.ToLower(host.tagName) {
	case "#text", // 文本节点不是元素，没有伪元素
		"input", "textarea", "select", "option", "optgroup",
		"img", "br", "hr", "iframe", "video", "audio", "canvas", "svg":
		return false
	}
	return true
}

// generatedStyle 用伪元素规则算出生成盒的计算样式与 content 文本。
//
// 继承从宿主开始（与普通子元素同口径），display 初始值为 inline
// （CSS 伪元素的初始 display，不是引擎 newStyle 的 inline-block）。
// 返回 ok=false 表示没有有效 content，不应生成盒子。
func generatedStyle(host *htmlElement, decls []declCandidate) (CSSStyleDeclaration, string, bool) {
	comp := newStyle()
	applyInheritedStyle(comp, host.computed)
	comp.SetDisplay(DisplayInline)

	content := ""
	hasContent := false
	apply := func(list []declCandidate) {
		for _, c := range list {
			if c.decl.prop == "content" {
				// content 不走 applyDecl：它带引号字符串，不能被 ToLower
				// 破坏大小写，也不是真正的样式字段。
				content, hasContent = parseContentValue(c.decl.value, host.self)
				continue
			}
			applyDecl(comp, c.decl.prop, c.decl.value)
		}
	}
	apply(filterByImportant(decls, false))
	apply(filterByImportant(decls, true))
	if !hasContent {
		return nil, "", false
	}
	return comp, content, true
}

// newGeneratedElement 创建生成盒。标签名带伪元素名，调试与 JSON/SVG
// 输出时一眼可辨；它不会出现在公开 DOM 里。
func newGeneratedElement(host *htmlElement, kind string) *htmlElement {
	el := newElement("::" + kind).(*htmlElement)
	el.generated = true
	el.genPseudo = kind
	el.genHost = host
	el.parentElement = host.self
	return el
}

// setGeneratedText 写入生成盒的文本内容。复用已有文本节点、直接改 text
// 字段（不走 SetText）：级联发生在布局内部，触发变更通知会造成「每轮
// 布局都自增 ChangeCount」的死循环。
func setGeneratedText(gen *htmlElement, content string) {
	gen.text = content
	if content == "" {
		gen.children = nil
		gen.genText = nil
		return
	}
	if gen.genText == nil {
		tn := NewTextNode(content).(*textNode)
		tn.setParentElement(gen.self)
		gen.genText = tn
		gen.children = []HTMLElement{tn}
	} else {
		gen.genText.node.text = content
	}
	// 文本节点没有独立级联，渲染时读它的 computed 取文字装饰等属性；
	// 与生成盒共用同一份样式即可（都是继承属性，不会互相污染）。
	gen.genText.node.computed = gen.computed
}

// placeGenerated 把生成盒放到宿主的首（before）或尾（after）位。
func placeGenerated(host *htmlElement, gen *htmlElement, kind string) {
	idx := -1
	for i, c := range host.children {
		if inner(c) == gen {
			idx = i
			break
		}
	}
	if kind == pseudoElementBefore {
		if idx == 0 {
			host.hasGenerated = true
			return
		}
		if idx > 0 {
			host.children = append(host.children[:idx], host.children[idx+1:]...)
		}
		host.children = append([]HTMLElement{gen}, host.children...)
	} else {
		if idx >= 0 && idx == len(host.children)-1 {
			host.hasGenerated = true
			return
		}
		if idx >= 0 {
			host.children = append(host.children[:idx], host.children[idx+1:]...)
		}
		host.children = append(host.children, gen)
	}
	gen.parentElement = host.self
	host.hasGenerated = true
}

// dropGenerated 移除宿主缓存的生成盒（content 消失、display:none、
// 元素变成不可生成的类型时）。
func dropGenerated(host *htmlElement, kind string) {
	gen := host.genBefore
	if kind == pseudoElementAfter {
		gen = host.genAfter
	}
	if gen == nil {
		return
	}
	for i, c := range host.children {
		if inner(c) == gen {
			host.children = append(host.children[:i], host.children[i+1:]...)
			break
		}
	}
	if kind == pseudoElementBefore {
		host.genBefore = nil
	} else {
		host.genAfter = nil
	}
	host.hasGenerated = host.genBefore != nil || host.genAfter != nil
}

// firstNonGeneratedChild 返回第一个真实子节点（跳过 ::before/::after
// 生成盒）。<pre> 的首行换行裁剪等 DOM 形状判定要用它。
func firstNonGeneratedChild(el *htmlElement) HTMLElement {
	if el == nil {
		return nil
	}
	for _, c := range el.children {
		if cb := inner(c); cb != nil && cb.generated {
			continue
		}
		return c
	}
	return nil
}

// parseContentValue 解析 content 的值，返回生成文本与是否生成盒子。
//
// 支持带引号的字符串（含 CSS 转义）与 attr(属性名)，多个分量按顺序拼接：
//
//	content: "[" attr(data-label) "]"
//
// none/normal 以及「没有任何可识别分量」的取值（counter()/url() 等）
// 返回 ok=false。
func parseContentValue(raw string, host HTMLElement) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	switch strings.ToLower(s) {
	case "none", "normal":
		return "", false
	}

	var b strings.Builder
	recognized := false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case c == '"' || c == '\'':
			str, n, closed := parseCSSString(s[i:])
			b.WriteString(str)
			if closed {
				recognized = true
			}
			i += n
		case hasPrefixFold(s[i:], "attr("):
			end := strings.IndexByte(s[i:], ')')
			if end < 0 {
				i = len(s)
				continue
			}
			name := strings.TrimSpace(s[i+len("attr(") : i+end])
			if host != nil {
				b.WriteString(host.GetAttribute(name))
			}
			recognized = true
			i += end + 1
		default:
			// 跳过不支持的函数/关键字：整段（含括号内）视为一个分量。
			j := i
			depth := 0
			for j < len(s) {
				ch := s[j]
				if ch == '(' {
					depth++
				} else if ch == ')' {
					if depth > 0 {
						depth--
					}
					if depth == 0 {
						j++
						break
					}
				} else if depth == 0 && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f') {
					break
				}
				j++
			}
			if j == i {
				j++ // 防御：非法字节也要前进
			}
			i = j
		}
	}
	if !recognized {
		return "", false
	}
	return b.String(), true
}

// parseCSSString 解析开头的 CSS 字符串字面量，返回内容、消耗字节数与
// 是否正常闭合（未闭合时按到结尾处理，调用方不把该分量算作有效内容）。
func parseCSSString(s string) (string, int, bool) {
	if s == "" || (s[0] != '"' && s[0] != '\'') {
		return "", 0, false
	}
	quote := s[0]
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			next := s[i+1]
			if isHexDigit(next) {
				// 十六进制转义：1~6 位，后面可跟一个空白分隔符。
				j := i + 1
				for j < len(s) && j-(i+1) < 6 && isHexDigit(s[j]) {
					j++
				}
				if v, err := strconv.ParseInt(s[i+1:j], 16, 32); err == nil {
					b.WriteRune(rune(v))
				}
				if j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n') {
					j++
				}
				i = j - 1
				continue
			}
			if next == '\n' || next == '\r' {
				i++ // 续行反斜杠：换行不进入内容
				continue
			}
			b.WriteByte(next)
			i++
			continue
		}
		if c == quote {
			return b.String(), i + 1, true
		}
		b.WriteByte(c)
	}
	return b.String(), len(s), false
}

// hasPrefixFold 判断 s 是否以 prefix 开头（ASCII 大小写不敏感）。
func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// isHexDigit 判断是否为十六进制数字。
func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
