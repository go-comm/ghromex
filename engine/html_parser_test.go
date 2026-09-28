package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 回归测试：源码末尾为普通文本/空白（最后一个 '<' 之后再无 '<'）时，
// 解析器不应 panic（曾越界访问 src[i:len(src)]）。
func TestParserTrailingText(t *testing.T) {
	srcs := []string{
		"<html><body>hello",        // 未闭合标签 + 尾部文本
		"<html><body>x\n \n",       // 尾部纯空白
		"plain text only",          // 完全无标签
		"<b>bold</b>trailing text", // 闭合后尾随文本
	}
	for _, src := range srcs {
		vp := engine.NewHeadlessViewport(200, 100)
		if _, err := engine.OpenDocument(vp, src); err != nil {
			t.Fatalf("OpenDocument(%q): %v", src, err)
		}
	}
}

// ParseFragment 同样覆盖无文档模式的路径。
func TestParseFragment(t *testing.T) {
	root := engine.ParseFragment("<div>abc<b>def</b>ghi")
	if root == nil {
		t.Fatal("nil fragment root")
	}
	if len(root.Children()) == 0 {
		t.Fatal("fragment has no children")
	}
}
