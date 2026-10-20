package engine_test

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// ---------------------------------------------------------------------------
// 级联与行内格式上下文的语义回归
// ---------------------------------------------------------------------------

// TestImportantDeclarationWins 带 !important 的声明压过特异度更高的普通
// 声明——此前 !important 整条声明被解析丢弃（值里带标记 → 解析失败）。
func TestImportantDeclarationWins(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#x { color: #0000FF; }
	div { color: #FF0000 !important; }
	</style></head><body><div id="x">text</div></body></html>`
	doc := openDoc(t, 200, 100, src)
	svg := doc.DumpSVG()
	if !strings.Contains(svg, `fill="#ff0000"`) {
		t.Fatalf("!important 声明未生效（应取 #ff0000）：\n%s", svg)
	}
	if strings.Contains(svg, `fill="#0000ff"`) {
		t.Fatalf("高特异度普通声明压过了 !important：\n%s", svg)
	}
}

// TestSpecificityIdBeatsManyClasses 特异度按 (id, class, tag) 字典序比较：
// 1 个 id 压过任意多个 class——旧实现压成单个整数（id*100+class*10+tag），
// 11 个类（110）会压过 1 个 id（100）。
func TestSpecificityIdBeatsManyClasses(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#only { width: 100px; }
	.a.b.c.d.e.f.g.h.i.j.k { width: 300px; }
	</style></head><body>
	<div id="only" class="a b c d e f g h i j k"></div>
	</body></html>`
	doc := openDoc(t, 400, 300, src)
	r := mustEl(t, doc, "#only").GetBoundingClientRect()
	if w := r.Right().Pixel() - r.Left().Pixel(); w != 100 {
		t.Errorf("盒宽 = %d, want 100（id 选择器应压过 11 个 class）", w)
	}
}

// TestFixedElementDoesNotScroll position:fixed 以视口为包含块，文档滚动
// 时必须留在原位（Chrome 语义）。
func TestFixedElementDoesNotScroll(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	#tall { height: 2000px; }
	#fx { position: fixed; top: 10px; left: 0; width: 50px; height: 20px; background-color: #FF0000; }
	</style></head><body>
	<div id="tall"></div><div id="fx"></div>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	fx := mustEl(t, doc, "#fx")
	if y := fx.GetBoundingClientRect().Top().Pixel(); y != 10 {
		t.Fatalf("滚动前 fixed 顶 = %d, want 10", y)
	}
	if !engine.OnDocumentWheel(doc, 100, 100, 0, 100) {
		t.Fatal("滚轮应被文档消费")
	}
	if y := fx.GetBoundingClientRect().Top().Pixel(); y != 10 {
		t.Errorf("滚动后 fixed 顶 = %d, want 10（fixed 不随文档滚动）", y)
	}
}

// TestInlineWhitespacePreserved 行内元素之间的空格必须保留（CSS 折叠为
// 单个空格）——纯空白文本节点此前整体消失，两段文字直接相邻。
func TestInlineWhitespacePreserved(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	span { font-size: 16px; }
	</style></head><body>
	<span id="a">A</span> <span id="b">B</span>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	a := mustEl(t, doc, "#a")
	b := mustEl(t, doc, "#b")
	gap := b.X() - (a.X() + a.Width())
	if gap <= 0 {
		t.Errorf("两段行内文字间隙 = %d, want > 0（行内空格被吞）", gap)
	}
}

// TestTrailingWhitespaceRemoved 行尾空格应被移除（CSS 折叠规则）：
// 行内内容后的空白不应在块级子元素前留下间隙。
func TestTrailingWhitespaceRemoved(t *testing.T) {
	src := `<!doctype html><html><head><style>
	body { margin: 0; }
	</style></head><body>
	<span id="a">A</span> <div id="blk">B</div>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	a := mustEl(t, doc, "#a")
	blk := mustEl(t, doc, "#blk")
	if blk.Y() == a.Y() {
		t.Error("块级子元素应另起一行")
	}
}
