package engine

import "strings"

type combinator uint8

const (
	combDescendant combinator = iota // 空格：后代
	combChild                        // >：直接子元素
)

// compoundSelector 是不含组合器的选择器片段，如 button.btn-primary#ok。
type compoundSelector struct {
	tag     string
	id      string
	classes []string
	attrs   [][2]string
}

// Selector 是从左到右排列的复合选择器序列，combs[i] 位于 compounds[i] 与 compounds[i+1] 之间。
type Selector struct {
	compounds []*compoundSelector
	combs     []combinator
}

func (s *Selector) isEmpty() bool {
	return s == nil || len(s.compounds) == 0
}

// Specificity 返回 (id*100 + class/attr*10 + tag) 形式的特异度。
func (s *Selector) Specificity() int {
	score := 0
	for _, c := range s.compounds {
		if c.id != "" {
			score += 100
		}
		score += 10 * len(c.classes)
		score += 10 * len(c.attrs)
		if c.tag != "" {
			score++
		}
	}
	return score
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
				for i < len(q) && q[i] != ' ' && q[i] != '>' && q[i] != '#' && q[i] != '.' && q[i] != '[' {
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
	return ch == '-' || ch == '_' || ch == ':' ||
		(ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

func matchCompound(c *compoundSelector, e HTMLElement) bool {
	if e == nil {
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
	return true
}

// matchSelectorChain 从右向左沿父链匹配。
func matchSelectorChain(s *Selector, e HTMLElement) bool {
	if s.isEmpty() {
		return false
	}
	last := len(s.compounds) - 1
	if !matchCompound(s.compounds[last], e) {
		return false
	}
	cur := e.ParentElement()
	for j := last - 1; j >= 0; j-- {
		comb := s.combs[j]
		if comb == combChild {
			if cur == nil || !matchCompound(s.compounds[j], cur) {
				return false
			}
			cur = cur.ParentElement()
			continue
		}
		found := false
		for cur != nil {
			if matchCompound(s.compounds[j], cur) {
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
