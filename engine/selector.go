package engine

import "strings"

type combinator uint8

const (
	combDescendant combinator = iota // 空格：后代
	combChild                        // >：直接子元素
)

// compoundSelector 是不含组合器的选择器片段，如 button.btn-primary#ok:hover。
type compoundSelector struct {
	tag     string
	id      string
	classes []string
	attrs   [][2]string
	// pseudos 是伪类名列表（:hover/:focus/:first-child…，小写、不含冒号）。
	// 伪类与标签/类/属性写在同一个片段里（a:hover、li:first-child）。
	pseudos []string
	// pseudoElement 标记该片段含伪元素（::before/::after）。引擎不支持
	// 伪元素，含伪元素的片段**永不匹配**——不能把 "p::before" 退化成
	// "p"（那会让所有 p 都吃到伪元素的声明）。
	pseudoElement bool
}

// Selector 是从左到右排列的复合选择器序列，combs[i] 位于 compounds[i] 与 compounds[i+1] 之间。
type Selector struct {
	compounds []*compoundSelector
	combs     []combinator
}

func (s *Selector) isEmpty() bool {
	return s == nil || len(s.compounds) == 0
}

// Specificity 返回选择器特异度，按 CSS 的 (a,b,c) 字典序编码为单个整数：
// a=id 数、b=class/attr/伪类数、c=类型选择器数，编码为 a*1e6 + b*1e3 + c。
// 编码保证整数比较与 CSS 优先级一致——旧实现用 id*100 + class*10 + tag，
// 11 个类（110）会压过 1 个 id（100），与 CSS 相反。
//
// 伪类按 class 档计特异度（CSS 2.1 §6.4.3：伪类与类选择器同级）。
func (s *Selector) Specificity() int {
	var a, b, c int
	for _, cp := range s.compounds {
		if cp.id != "" {
			a++
		}
		b += len(cp.classes) + len(cp.attrs) + len(cp.pseudos)
		if cp.tag != "" {
			c++
		}
	}
	return a*1000000 + b*1000 + c
}

// ParseSelector 解析 "div.card > .title" 形式的选择器串。
func ParseSelector(q string) *Selector {
	sel := &Selector{}
	q = strings.TrimSpace(q)
	var cur *compoundSelector
	pendingComb := combDescendant
	hasPending := false

	flush := func() {
		if cur != nil {
			if len(sel.compounds) > 0 {
				sel.combs = append(sel.combs, pendingComb)
			}
			sel.compounds = append(sel.compounds, cur)
			hasPending = false
		}
		cur = nil
	}

	i := 0
	for i < len(q) {
		ch := q[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			flush()
		case ch == '>':
			if cur == nil {
				pendingComb = combChild
				hasPending = true
			} else {
				// "a>b"：结束当前复合并标记子代
				c := cur
				cur = nil
				flushCompoundInto(sel, c, combChild)
			}
			i++
			continue
		default:
			if cur == nil {
				if hasPending {
					// 已由 '>' 设置
				} else {
					pendingComb = combDescendant
				}
				start := i
				for i < len(q) && q[i] != ' ' && q[i] != '>' && q[i] != '#' && q[i] != '.' && q[i] != '[' && q[i] != ':' {
					i++
				}
				if start < i {
					cur = &compoundSelector{tag: strings.ToLower(q[start:i])}
				} else {
					cur = &compoundSelector{}
				}
				continue // 不递增，重新处理当前字符
			}
			switch ch {
			case '#':
				j := i + 1
				for j < len(q) && isSelectorChar(q[j]) {
					j++
				}
				cur.id = q[i+1 : j]
				i = j
			case '.':
				j := i + 1
				for j < len(q) && isSelectorChar(q[j]) {
					j++
				}
				cur.classes = append(cur.classes, q[i+1:j])
				i = j
			case ':':
				// 伪类（:hover/:focus…）或伪元素（::before）。伪元素引擎
				// 暂无支持，解析出来但不参与匹配（整条选择器不会命中）。
				j := i + 1
				element := false
				if j < len(q) && q[j] == ':' {
					element = true
					j++
				}
				nameStart := j
				for j < len(q) && isPseudoChar(q[j]) {
					j++
				}
				name := strings.ToLower(q[nameStart:j])
				if element {
					cur.pseudoElement = true
				} else if name != "" {
					cur.pseudos = append(cur.pseudos, name)
				}
				i = j
			case '[':
				j := strings.IndexByte(q[i:], ']')
				if j < 0 {
					i = len(q)
					continue
				}
				inner := q[i+1 : i+j]
				if eq := strings.IndexByte(inner, '='); eq >= 0 {
					key := strings.TrimSpace(inner[:eq])
					val := strings.Trim(strings.TrimSpace(inner[eq+1:]), "\"'")
					cur.attrs = append(cur.attrs, [2]string{key, val})
				} else {
					cur.attrs = append(cur.attrs, [2]string{strings.TrimSpace(inner), ""})
				}
				i = i + j + 1
			default:
				i++
			}
			continue
		}
		flush()
		i++
	}
	flush()
	return sel
}

func flushCompoundInto(sel *Selector, c *compoundSelector, comb combinator) {
	if c == nil {
		return
	}
	if len(sel.compounds) > 0 {
		sel.combs = append(sel.combs, comb)
	}
	sel.compounds = append(sel.compounds, c)
}

func isSelectorChar(ch byte) bool {
	return ch == '-' || ch == '_' ||
		(ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// isPseudoChar 是伪类名可用的字符：字母、数字、'-'、'_'（不含 ':'，
// 否则 "a:hover" 的类名会把伪类名一起吞掉）。
func isPseudoChar(ch byte) bool {
	return ch == '-' || ch == '_' ||
		(ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// matchCompound 判断元素是否匹配一个复合选择器片段。伪类按当前状态
// 匹配（hover/focus/active 来自文档状态，first-child 等看 DOM 形状）。
func matchCompound(doc HTMLDocument, c *compoundSelector, e HTMLElement) bool {
	if e == nil {
		return false
	}
	if c.pseudoElement {
		// 伪元素未实现：整条选择器不匹配（不能退化成只匹配标签/类）。
		return false
	}
	if c.tag != "" && !strings.EqualFold(e.TagName(), c.tag) {
		return false
	}
	if c.id != "" && e.GetAttribute("id") != c.id {
		return false
	}
	for _, cls := range c.classes {
		if !containsWord(e.GetAttribute("class"), cls) {
			return false
		}
	}
	for _, kv := range c.attrs {
		v := e.GetAttribute(kv[0])
		if kv[1] == "" {
			if v == "" {
				return false
			}
		} else if v != kv[1] {
			return false
		}
	}
	for _, ps := range c.pseudos {
		if !matchPseudo(doc, ps, e) {
			return false
		}
	}
	return true
}

// matchSelectorChain 从右向左沿父链匹配。
func matchSelectorChain(doc HTMLDocument, s *Selector, e HTMLElement) bool {
	if s.isEmpty() {
		return false
	}
	last := len(s.compounds) - 1
	if !matchCompound(doc, s.compounds[last], e) {
		return false
	}
	cur := e.ParentElement()
	for j := last - 1; j >= 0; j-- {
		comb := s.combs[j]
		if comb == combChild {
			if cur == nil || !matchCompound(doc, s.compounds[j], cur) {
				return false
			}
			cur = cur.ParentElement()
			continue
		}
		found := false
		for cur != nil {
			if matchCompound(doc, s.compounds[j], cur) {
				found = true
				cur = cur.ParentElement()
				break
			}
			cur = cur.ParentElement()
		}
		if !found {
			return false
		}
	}
	return true
}
