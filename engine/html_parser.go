package engine

import (
	"strconv"
	"strings"
)

// ParseHTMLInto 将 HTML 源码解析进既有文档：
// <head>/<body> 内容路由到文档结构节点，<style> 编译进文档样式表，
// <title> 写入 head 的 title 元素，<script>/<link>/<meta> 被忽略。
func ParseHTMLInto(doc HTMLDocument, src string) {
	parseInto(doc, doc.Head(), doc.Body(), src, doc)
}

// ParseFragment 将 HTML 片段解析到一个匿名 body 节点下并返回该节点。
func ParseFragment(src string) HTMLElement {
	root := newElement("body")
	parseInto(nil, nil, root, src, nil)
	return root
}

var voidElements = map[string]bool{
	"input": true, "img": true, "br": true, "hr": true, "meta": true,
	"link": true, "area": true, "base": true, "col": true, "embed": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// parseInto 核心解析循环。doc 为 nil 时退化为片段模式（无 head/style 路由）。
func parseInto(htmlNode HTMLElement, headEl, bodyEl HTMLElement, src string, doc HTMLDocument) {
	stack := []HTMLElement{htmlNode}
	if htmlNode == nil {
		stack = []HTMLElement{bodyEl}
	}
	current := func() HTMLElement { return stack[len(stack)-1] }

	push := func(e HTMLElement) {
		stack = append(stack, e)
	}
	popTo := func(tag string) {
		for k := len(stack) - 1; k > 0; k-- {
			if stack[k].TagName() == tag {
				stack = stack[:k]
				return
			}
		}
	}

	// ensureBody 处理根节点下直接出现的内容：自动开启 body。
	ensureInsertTarget := func() HTMLElement {
		cur := current()
		if doc != nil && cur != nil && cur.TagName() == "html" {
			push(bodyEl)
			return bodyEl
		}
		return cur
	}

	i := 0
	for i < len(src) {
		if src[i] != '<' {
			next := strings.IndexByte(src[i:], '<')
			if next < 0 {
				next = len(src) - i // 已是相对剩余部分的长度
			}
			raw := src[i : i+next]
			i += next
			if strings.TrimSpace(raw) == "" {
				continue
			}
			target := ensureInsertTarget()
			if target != nil {
				target.AppendChild(NewTextNode(decodeEntities(raw)))
			}
			continue
		}

		// 注释
		if strings.HasPrefix(src[i:], "<!--") {
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
				break
			}
			i += 4 + end + 3
			continue
		}
		// DOCTYPE / 处理指令
		if strings.HasPrefix(src[i:], "<!") {
			end := strings.IndexByte(src[i:], '>')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}

		// 结束标签
		if i+1 < len(src) && src[i+1] == '/' {
			j := i + 2
			start := j
			for j < len(src) && isTagChar(src[j]) {
				j++
			}
			tag := strings.ToLower(src[start:j])
			end := strings.IndexByte(src[j:], '>')
			if end >= 0 {
				j += end + 1
			}
			i = j
			popTo(tag)
			continue
		}

		// 开始标签
		j := i + 1
		start := j
		for j < len(src) && isTagChar(src[j]) {
			j++
		}
		if j == start {
			// 孤立的 '<'，按文本处理
			target := ensureInsertTarget()
			if target != nil {
				target.AppendChild(NewTextNode("<"))
			}
			i++
			continue
		}
		tag := strings.ToLower(src[start:j])
		attrs, after, selfClose := parseAttributes(src, j)

		switch tag {
		case "script":
			close := strings.Index(strings.ToLower(src[after:]), "</script")
			if close < 0 {
				i = len(src)
			} else {
				i = after + close
			}
			continue
		case "style":
			close := strings.Index(strings.ToLower(src[after:]), "</style")
			if close < 0 {
				break
			}
			cssText := src[after : after+close]
			i = after + close
			// 跳过 </style>
			if end := strings.IndexByte(src[i:], '>'); end >= 0 {
				i += end + 1
			}
			if doc != nil && !selfClose {
				doc.AddStylesheetText(cssText)
			}
			continue
		case "title":
			close := strings.Index(strings.ToLower(src[after:]), "</title")
			var text string
			if close < 0 {
				text = src[after:]
				i = len(src)
			} else {
				text = src[after : after+close]
				i = after + close
				if end := strings.IndexByte(src[i:], '>'); end >= 0 {
					i += end + 1
				}
			}
			if doc != nil && headEl != nil {
				if head, ok := headEl.(HTMLHeadElement); ok {
					head.Title().SetText(strings.TrimSpace(decodeEntities(text)))
				}
			}
			continue
		}

		// 普通元素
		var e HTMLElement
		if doc != nil {
			e = doc.CreateElement(tag)
		} else {
			e = newElement(tag)
		}
		for _, kv := range attrs {
			e.SetAttribute(kv[0], decodeEntities(kv[1]))
		}

		switch tag {
		case "html":
			// <html> 即文档根，仅补充属性，不压栈重复
			for _, kv := range attrs {
				htmlNode.SetAttribute(kv[0], kv[1])
			}
			i = after
			continue
		case "head":
			if doc != nil && headEl != nil {
				stack = stack[:1]
				push(headEl)
			}
			i = after
			continue
		case "body":
			if doc != nil && bodyEl != nil {
				stack = stack[:1]
				push(bodyEl)
			}
			i = after
			continue
		case "meta", "link", "base":
			i = after
			continue
		}

		target := ensureInsertTarget()
		if target != nil {
			target.AppendChild(e)
		}
		if !voidElements[tag] && !selfClose {
			push(e)
		}
		i = after
	}
}

func isTagChar(ch byte) bool {
	return ch == '-' || ch == '_' || ch == ':' ||
		(ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// parseAttributes 从 pos（标签名结束处）解析属性，返回属性列表、'>' 之后的下标与自闭合标记。
func parseAttributes(src string, pos int) (attrs [][2]string, next int, selfClose bool) {
	i := pos
	for i < len(src) {
		// 跳过空白
		for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r' || src[i] == '/') {
			if src[i] == '/' && i+1 < len(src) && src[i+1] == '>' {
				selfClose = true
				return attrs, i + 2, true
			}
			i++
		}
		if i >= len(src) {
			break
		}
		if src[i] == '>' {
			return attrs, i + 1, selfClose
		}
		// 属性名
		start := i
		for i < len(src) && src[i] != '=' && src[i] != '>' && src[i] != ' ' && src[i] != '\t' && src[i] != '\n' && src[i] != '\r' {
			i++
		}
		name := strings.ToLower(src[start:i])
		if name == "" {
			i++
			continue
		}
		// 可选值
		for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
			i++
		}
		value := ""
		if i < len(src) && src[i] == '=' {
			i++
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
			if i < len(src) && (src[i] == '"' || src[i] == '\'') {
				quote := src[i]
				i++
				vs := i
				for i < len(src) && src[i] != quote {
					i++
				}
				value = src[vs:i]
				if i < len(src) {
					i++
				}
			} else {
				vs := i
				for i < len(src) && src[i] != '>' && src[i] != ' ' && src[i] != '\t' && src[i] != '\n' && src[i] != '\r' {
					i++
				}
				value = src[vs:i]
			}
		}
		attrs = append(attrs, [2]string{name, value})
	}
	return attrs, i, selfClose
}

var entityRefs = map[string]string{
	"amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'",
	"nbsp": "\u00a0",
}

// decodeEntities 处理命名实体与数字实体（十进制/十六进制）。
func decodeEntities(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		semi := strings.IndexByte(s[i:], ';')
		if semi < 0 || semi > 12 {
			b.WriteByte(s[i])
			i++
			continue
		}
		ref := s[i+1 : i+semi]
		handled := false
		switch {
		case strings.HasPrefix(ref, "#x") || strings.HasPrefix(ref, "#X"):
			if n, err := strconv.ParseInt(ref[2:], 16, 32); err == nil && utf8Valid(rune(n)) {
				b.WriteRune(rune(n))
				handled = true
			}
		case strings.HasPrefix(ref, "#"):
			if n, err := strconv.ParseInt(ref[1:], 10, 32); err == nil && utf8Valid(rune(n)) {
				b.WriteRune(rune(n))
				handled = true
			}
		default:
			if rep, ok := entityRefs[ref]; ok {
				b.WriteString(rep)
				handled = true
			}
		}
		if handled {
			i += semi + 1
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

func utf8Valid(r rune) bool {
	return r >= 0 && r <= 0x10FFFF
}
