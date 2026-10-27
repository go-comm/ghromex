package engine

import (
	"strconv"
	"strings"
)

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
	// pseudos 是伪类列表（:hover/:first-child，及带参数的 :not()/:nth-child()）。
	// 伪类与标签/类/属性写在同一个片段里（a:hover、li:first-child）。
	pseudos []pseudoClass
	// pseudoElement 是该片段尾部的伪元素名（小写、不含冒号）：
	// "before"/"after" 表示引擎支持的生成盒，其他值（如 "first-line"）
	// 是未支持的伪元素。空串表示没有伪元素。
	//
	// 含伪元素的片段不参与**元素自身**的匹配——不能把 "p::before" 退化成
	// "p"（那会让所有 p 都吃到伪元素的声明）；它只由伪元素级联按宿主
	// 元素匹配（见 matchPseudoElementChain）。
	pseudoElement string
}

// pseudoClass 是片段里的一个伪类。无参伪类（:hover/:first-child…）只有
// 名字；**函数型伪类**（:not()/:nth-child()）在 ParseSelector 期就把参数
// 预解析成结构化形态（级联要对每个元素×每条规则跑匹配，不能每元素重解
// 一次串）。bad=true 表示参数非法（如 :not() 空参、:nth-child(2x)）——
// CSS 语义是整个选择器无效，等价于永不匹配。
type pseudoClass struct {
	name string
	// sels 是 :not() 参数解析出的选择器列表（Selectors 4 允许逗号表，
	// 全部不匹配才算匹配 :not）。
	sels []*Selector
	// nthA/nthB 是 :nth-child() 参数解析出的 An+B。
	nthA, nthB int
	bad        bool
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
// 伪类按 class 档计特异度（CSS 2.1 §6.4.3：伪类与类选择器同级）；
// 伪元素按类型选择器档计（Selectors 3 §9：伪元素与标签同级）。
// 函数型伪类里只有 :not() 把**参数**的特异度计入（Selectors 4 §10：取
// 参数列表最高者），:nth-child() 等仍按一个伪类计档（参数不计）。
func (s *Selector) Specificity() int {
	var a, b, c, extra int
	for _, cp := range s.compounds {
		if cp.id != "" {
			a++
		}
		b += len(cp.classes) + len(cp.attrs)
		for _, ps := range cp.pseudos {
			if ps.name == pseudoNot {
				for _, sel := range ps.sels {
					if spec := sel.Specificity(); spec > extra {
						extra = spec
					}
				}
				continue
			}
			b++
		}
		if cp.tag != "" {
			c++
		}
		if cp.pseudoElement != "" {
			c++
		}
	}
	return a*1000000 + b*1000 + c + extra
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
				// 伪类（:hover/:focus…）或伪元素（::before/::after）。
				// 单冒号的 :before/:after 是 CSS 2.1 的旧写法，按伪元素处理
				// （Chrome 同样兼容）。
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
				if !element && (name == pseudoElementBefore || name == pseudoElementAfter) {
					element = true
				}
				if element {
					if name == "" {
						// "::" 后没有名字：按未支持的伪元素处理（永不匹配）。
						name = "?"
					}
					cur.pseudoElement = name
					i = j
				} else if name != "" {
					// 函数型伪类（:not() / :nth-child() …）：若后跟 "("，把配对的括号
					// 内容作为参数整体吞掉（支持嵌套，如 :not(:nth-child(2))）。
					// 未识别的函数型伪类也要吞括号，避免里面的 ".class" 被误当成
					// 当前片段的类名、":" 再被解析成新伪类。
					arg := ""
					next := j
					if j < len(q) && q[j] == '(' {
						if end := matchParen(q, j); end > 0 {
							arg = q[j+1 : end]
							next = end + 1
						} else {
							arg = q[j+1:]
							next = len(q)
						}
					}
					pc := newPseudoClass(name, arg)
					cur.pseudos = append(cur.pseudos, pc)
					i = next
				} else {
					i = j
				}
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
//
// 带伪元素的片段不匹配元素自身：伪元素是生成盒，不是宿主元素的一部分，
// 它由伪元素级联单独匹配（matchPseudoElementChain）。
func matchCompound(doc HTMLDocument, c *compoundSelector, e HTMLElement) bool {
	if c == nil || c.pseudoElement != "" {
		return false
	}
	return matchCompoundBase(doc, c, e)
}

// matchCompoundBase 匹配复合选择器片段里**除伪元素之外**的部分。伪元素
// 级联用宿主元素调它：p:hover::before 的 :hover 仍然按宿主状态判定。
func matchCompoundBase(doc HTMLDocument, c *compoundSelector, e HTMLElement) bool {
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
	for _, ps := range c.pseudos {
		if !matchPseudo(doc, ps, e) {
			return false
		}
	}
	return true
}

// matchSelectorChain 从右向左沿父链匹配元素自身（尾部片段不得带伪元素）。
func matchSelectorChain(doc HTMLDocument, s *Selector, e HTMLElement) bool {
	return matchChain(doc, s, e, "")
}

// matchPseudoElementChain 判断元素是否匹配「宿主 + 伪元素」形式的规则：
// 选择器尾部片段必须带同名伪元素（before/after），其余部分按宿主元素
// 与父链匹配。祖先片段带伪元素（如 "p::before span"）不支持——伪元素
// 生成盒不参与选择器匹配，这种选择器永不命中。
func matchPseudoElementChain(doc HTMLDocument, s *Selector, e HTMLElement, kind string) bool {
	return matchChain(doc, s, e, kind)
}

// matchChain 是两条匹配路径的公共实现。pseudo 为空时匹配元素自身；
// 非空时要求尾部片段带该伪元素、按宿主元素匹配。
func matchChain(doc HTMLDocument, s *Selector, e HTMLElement, pseudo string) bool {
	if s.isEmpty() || e == nil {
		return false
	}
	last := len(s.compounds) - 1
	tail := s.compounds[last]
	if pseudo == "" {
		if !matchCompound(doc, tail, e) {
			return false
		}
	} else {
		if tail.pseudoElement != pseudo || !matchCompoundBase(doc, tail, e) {
			return false
		}
	}
	cur := e.ParentElement()
	for j := last - 1; j >= 0; j-- {
		if s.compounds[j].pseudoElement != "" {
			return false
		}
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

// matchParen 从 q[open]=='(' 开始向后扫描，返回配对的 ')' 的索引；未闭合返回 -1。
// 用于函数型伪类（:not() / :nth-child()）参数的边界识别（支持嵌套括号）。
func matchParen(q string, open int) int {
	depth := 0
	for i := open; i < len(q); i++ {
		switch q[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// newPseudoClass 根据名字与原始参数串构造 pseudoClass。
// 已知函数型伪类（:not / :nth-child）在解析期预解析参数——级联要对每个
// 元素×每条规则跑匹配，参数解析不能每次都重来；未识别的函数型伪类
// （:is() / :lang() 等）标记 bad=true（整条选择器无效，同 CSS 语义），
// 但括号内容照吞——否则里面的 ".class" 会被当成当前片段的类名。
func newPseudoClass(name, arg string) pseudoClass {
	pc := pseudoClass{name: name}
	switch name {
	case pseudoNot:
		// Selectors 4：:not() 接受逗号分隔的选择器列表；任一匹配即令 :not 失效。
		// 空参或解析不出选择器 → bad（CSS：非法参数使整条选择器无效）。
		for _, p := range splitTopLevel(arg, ',') {
			p = strings.TrimSpace(p)
			if p == "" {
				pc.bad = true
				return pc
			}
			sel := ParseSelector(p)
			if sel.isEmpty() {
				pc.bad = true
				return pc
			}
			pc.sels = append(pc.sels, sel)
		}
	case pseudoNthChild:
		a, b, ok := parseNth(arg)
		if !ok {
			pc.bad = true
			return pc
		}
		pc.nthA, pc.nthB = a, b
	default:
		// 无参伪类带上了括号（:hover(2)）或未知函数型伪类：一律 bad。
		if arg != "" {
			pc.bad = true
		}
	}
	return pc
}

// parseNth 解析 :nth-child() 的 An+B 参数（不区分大小写、忽略所有空白）。
// 支持：odd / even / 整数（3） / 2n+1 / -n+3 / n / +3n-2 / 3n 等标准写法。
// ok=false 表示非法参数（如 2x、n++、空串）。
func parseNth(raw string) (a, b int, ok bool) {
	s := strings.ToLower(strings.Join(strings.Fields(raw), ""))
	switch s {
	case "":
		return 0, 0, false
	case "odd":
		return 2, 1, true
	case "even":
		return 2, 0, true
	}
	n := strings.IndexByte(s, 'n')
	if n < 0 {
		// 纯整数：等价 0n+b（只对第 b 个元素匹配）。
		v, err := strconv.Atoi(s)
		if err != nil {
			return 0, 0, false
		}
		return 0, v, true
	}
	// aPart：n 之前的系数（空/"+"/"-" 按 ±1）；bPart：n 之后的常数项（可缺省）。
	aPart, bPart := s[:n], s[n+1:]
	switch aPart {
	case "", "+":
		a = 1
	case "-":
		a = -1
	default:
		v, err := strconv.Atoi(aPart)
		if err != nil {
			return 0, 0, false
		}
		a = v
	}
	if bPart == "" {
		return a, 0, true
	}
	v, err := strconv.Atoi(bPart)
	if err != nil {
		return 0, 0, false
	}
	return a, v, true
}
