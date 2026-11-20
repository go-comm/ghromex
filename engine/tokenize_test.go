package engine

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// collectTokens 跑一遍 tokenizeText 并把发射的词收集起来。
func collectTokens(s string) []string {
	var out []string
	tokenizeText(s, func(tok string) {
		out = append(out, tok)
	})
	return out
}

// TestTokenizeText 口径锁：改写 tokenizeText（[]string → 回调字节游标）后，
// 分词结果必须与原实现完全一致：
//   - 空白折叠为单个 " "，且首尾不产生空格；
//   - ASCII 连续可见字符为一个整词；
//   - CJK/非 ASCII 每个 rune 单字成词；
//   - 空格作为独立 token 出现在两侧词之间。
func TestTokenizeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"空串", "", nil},
		{"ASCII 两词", "hello world", []string{"hello", " ", "world"}},
		{"首尾空白丢弃", "  hello  ", []string{"hello"}},
		{"纯空白", "   ", nil},
		{"制表换行折叠", "a\tb\n c", []string{"a", " ", "b", " ", "c"}},
		{"CJK 逐字", "中文字", []string{"中", "文", "字"}},
		{"ASCII 接 CJK", "a中b", []string{"a", "中", "b"}},
		{"CJK 接 ASCII", "中a", []string{"中", "a"}},
		{"CJK 间空格", "中 文", []string{"中", " ", "文"}},
		{"混合空格", "不 接收 message", []string{"不", " ", "接", "收", " ", "message"}},
		{"不间断空格", "a\u00a0b", []string{"a", " ", "b"}},
		{"回车折叠", "ab\rcd", []string{"ab", " ", "cd"}},
		{"词内标点", "hi,there", []string{"hi,there"}},
		{"多空白折叠", "a   b", []string{"a", " ", "b"}},
		{"首 CJK 无前导空格", "  你好", []string{"你", "好"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := collectTokens(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("tokenizeText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// sink 阻止编译器把被测调用优化掉。
var sink string

// tokenizeReference 是改写前原实现（[]string + []rune 累加 + string(cur)）
// 的逐行拷贝，只用于差分对照，不参与生产路径。
func tokenizeReference(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	pendingSpace := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\u00a0':
			flush()
			if len(out) > 0 {
				pendingSpace = true
			}
		case r < 0x80:
			if pendingSpace {
				out = append(out, " ")
				pendingSpace = false
			}
			cur = append(cur, r)
		default:
			if pendingSpace {
				out = append(out, " ")
				pendingSpace = false
			}
			flush()
			out = append(out, string(r))
		}
	}
	flush()
	return out
}

// TestTokenizeTextMatchesReference 用固定语料 + 随机 fuzz 把新实现钉死在旧口径
// 上。字节子串改写最容易在「空白相邻 CJK」「多种空白混排」「非法字节」这类
// 边角上悄悄漂移，表驱动用例覆盖不全，这里做等价性差分。
func TestTokenizeTextMatchesReference(t *testing.T) {
	corpus := []string{
		"", " ", "  ", "\t", "\n", "\r", "\f", "\u00a0",
		"hello world", "  hello  ", "hello   world",
		"中文字", "a中b", "中a", "中 文", "不 接收 message",
		"a\u00a0b", "ab\rcd", "hi,there!", "a\tb\n c",
		"Hello, world! This is a test of the tokenizer.",
		"第一段中文。第二段中文！第三段？",
		"混合 mixed 内容 content 与 more words。",
		"a\r\nb", "\r\n\r\n", "x\u00a0\u00a0y",
		"line1\nline2\n\nline4",
		"emoji 😀 test", "Ünïcödé wörd", "ß Straße",
		"tab\there", "  \t  \n  ", "end with space ",
		" leading space", "a  b   c    d",
		strings.Repeat("word ", 200),
		strings.Repeat("中", 100),
		"\xff\xfe bad bytes", "a\xffb", "\xed\xa0\x80 surrogate",
	}
	// 随机串：字符集覆盖空白 / ASCII / CJK / 多字节 / 非法字节。
	alphabet := []rune{
		'a', 'b', 'Z', '0', '9', ' ', ' ', '\t', '\n', '\r', '\f', '\u00a0',
		'中', '文', '字', '，', '。', '！', 'é', 'ü', '😀', 'ß', 'Ω',
	}
	rng := rand.New(rand.NewSource(20261121))
	for i := 0; i < 50000; i++ {
		var sb strings.Builder
		for j, n := 0, rng.Intn(24); j < n; j++ {
			sb.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		corpus = append(corpus, sb.String())
	}

	for _, s := range corpus {
		want := tokenizeReference(s)
		got := collectTokens(s)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("tokenizeText(%q) 与旧实现不一致:\n  old=%q\n  new=%q", s, want, got)
		}
	}
}

// TestTokenizeTextASCIIZeroAlloc 锁住本轮优化的目标本身：ASCII 词全部走
// s[i:j] 子串，整条分词链路不再为每个词新建字符串。
//
// 用 testing.AllocsPerRun 直接数分配，而不是去比 unsafe.StringData 指针：
// 前者从 Go 1.0 就有，跨版本都跑得起来（后者是 Go 1.20+ API，还会把测试
// 和字符串内部表示绑死）；而且「零分配」正是我们要的性质，无需再绕一层
// 「共享底层数组」去间接推断。
func TestTokenizeTextASCIIZeroAlloc(t *testing.T) {
	const words = 512
	s := ""
	for i := 0; i < words; i++ {
		s += "word "
	}
	// "word " 重复 words 次 → words 个词 + words-1 个词间空格 = 2*words-1 个 token
	const wantToks = words*2 - 1
	count := 0
	// 预留够用的定长数组承接 token，避免收集器自身的 append 干扰计数。
	store := make([]string, wantToks+8)
	n := testing.AllocsPerRun(50, func() {
		count = 0
		tokenizeText(s, func(tok string) {
			store[count%len(store)] = tok
			count++
		})
		if count != wantToks {
			t.Fatalf("词数 = %d, want %d", count, wantToks)
		}
	})
	if n != 0 {
		t.Fatalf("纯 ASCII 分词仍有 %.0f 次分配/轮，应降为 0", n)
	}
	sink = store[0]
}

// BenchmarkTokenizeText 与旧实现（[]string + string(cur)）对照用。
func BenchmarkTokenizeText(b *testing.B) {
	s := ""
	for i := 0; i < 20000; i++ {
		s += "word "
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizeText(s, func(string) {})
	}
}
