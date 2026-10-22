package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// valignDoc 打开一个「文字 + 一个行内盒」的文档，返回文档、段落与行内盒。
//
// 行内盒用 inline-block + 定高，盒的基线由 inlineBoxBaseline 按
// 「盒底 − 字体 descent」给出（16px 字号 → descent 10）。
func valignDoc(t *testing.T, boxStyle string) (engine.HTMLDocument, engine.HTMLElement, engine.HTMLElement) {
	t.Helper()
	src := `<!doctype html><html><head></head><body style="margin:0">
		<p id="p" style="margin:0">x<span id="b" style="` + boxStyle + `"></span></p>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	return doc, mustEl(t, doc, "#p"), mustEl(t, doc, "#b")
}

const (
	// tallBox：40px 高的 inline-block。盒自身基线在「盒底 − 字体 descent(10)」
	// → asc=30 / desc=10。
	tallBox = "display:inline-block;width:20px;height:40px"
	// shortBox：20px 高（与一行文字同高）→ asc=10 / desc=10。
	shortBox = "display:inline-block;width:20px;height:20px"
)

// vertical-align:super —— 上标：盒的基线上抬「父字号/3+1」= 6px。
// 盒自身 asc=10 → 抬后 16，成为行盒的 ascent（文字的 10 不够高）；
// descent 取文字的 10 → 行盒高 16+10 = 26。
func TestVerticalAlignSuperRaisesBox(t *testing.T) {
	_, p, b := valignDoc(t, shortBox+";vertical-align:super")
	if p.Height() != 26 {
		t.Errorf("段落高 = %d, want 26（上标抬 6px，行盒 20 -> 26）", p.Height())
	}
	// 盒顶仍在行顶：行盒向上长高，盒自身不移动。
	if b.Y() != 0 {
		t.Errorf("盒顶 = %d, want 0（上标只抬基线，盒顶留在行顶）", b.Y())
	}
}

// vertical-align:sub —— 下标：盒的基线下沉「父字号/5+1」= 4px。
// 盒自身 desc=10 → 沉后 14，超过文字的 10 → 行盒高 10+14 = 24。
func TestVerticalAlignSubLowersBox(t *testing.T) {
	_, p, b := valignDoc(t, shortBox+";vertical-align:sub")
	if p.Height() != 24 {
		t.Errorf("段落高 = %d, want 24（下标沉 4px，行盒 20 -> 24）", p.Height())
	}
	if b.Y() != 4 {
		t.Errorf("盒顶 = %d, want 4（下沉 4px）", b.Y())
	}
}

// vertical-align:top —— 盒顶与行盒顶对齐。盒比文字高，行盒被撑到
// 盒高（40）；文字的基线仍按 strut 落在行盒上部。
func TestVerticalAlignTopAlignsBoxTopToLineTop(t *testing.T) {
	_, p, b := valignDoc(t, tallBox+";vertical-align:top")
	if p.Height() != 40 {
		t.Errorf("段落高 = %d, want 40（盒高 40，顶对齐把行盒撑到盒高）", p.Height())
	}
	if b.Y() != 0 {
		t.Errorf("盒顶 = %d, want 0（顶对齐：盒顶 = 行盒顶）", b.Y())
	}
}

// vertical-align:bottom —— 盒底与行盒底对齐。
func TestVerticalAlignBottomAlignsBoxBottomToLineBottom(t *testing.T) {
	_, p, b := valignDoc(t, tallBox+";vertical-align:bottom")
	if p.Height() != 40 {
		t.Errorf("段落高 = %d, want 40（盒高 40）", p.Height())
	}
	if got := b.Y() + b.Height(); got != 40 {
		t.Errorf("盒底 = %d, want 40（底对齐：盒底 = 行盒底）", got)
	}
}

// vertical-align:middle —— 盒的垂直中点与「基线 + 半个 x 高度」对齐。
// x 高度按字号 0.52 估（16px → 8）→ 中点在基线上方 4px。
// 盒高 40：基线 = 中点 + 20 = 24 → 行盒的 ascent 取 24。
func TestVerticalAlignMiddleCentersBoxOnXHeight(t *testing.T) {
	doc, p, b := valignDoc(t, tallBox+";vertical-align:middle")
	// 文字基线（SVG 导出）应落在 24：盒中点 20 在其上方 4px。
	base := svgTextBaselineY(t, doc.DumpSVG())
	if base != 24 {
		t.Errorf("文字基线 = %d, want 24（middle：盒中点对齐 x 高度中点）", base)
	}
	if p.Height() != 40 {
		t.Errorf("段落高 = %d, want 40（盒高 40）", p.Height())
	}
	if got := b.Y() + b.Height()/2; got != 20 {
		t.Errorf("盒中点 = %d, want 20（盒未按 middle 摆位）", got)
	}
}

// vertical-align:text-top —— 盒顶与父级内容区上沿（strut 的 ascent）对齐。
func TestVerticalAlignTextTop(t *testing.T) {
	_, p, b := valignDoc(t, tallBox+";vertical-align:text-top")
	// strut 的 ascent = 10 → 盒顶在基线上方 10 → 盒底在基线下 30
	// → 行盒 = 10 + 30 = 40
	if p.Height() != 40 {
		t.Errorf("段落高 = %d, want 40（text-top：盒顶对齐内容区上沿）", p.Height())
	}
	if b.Y() != 0 {
		t.Errorf("盒顶 = %d, want 0（text-top：盒顶 = 内容区上沿）", b.Y())
	}
}

// vertical-align:text-bottom —— 盒底与父级内容区下沿（strut 的 descent）对齐。
func TestVerticalAlignTextBottom(t *testing.T) {
	_, p, b := valignDoc(t, tallBox+";vertical-align:text-bottom")
	if p.Height() != 40 {
		t.Errorf("段落高 = %d, want 40（text-bottom）", p.Height())
	}
	if got := b.Y() + b.Height(); got != 40 {
		t.Errorf("盒底 = %d, want 40（text-bottom：盒底 = 内容区下沿）", got)
	}
}

// vertical-align:10px —— 长度形式：盒相对基线上抬 10px。
func TestVerticalAlignLengthRaisesBox(t *testing.T) {
	_, p, b := valignDoc(t, shortBox+";vertical-align:10px")
	// 上抬 10 → ascent 10+10=20 → 行盒 = 20 + 10 = 30
	if p.Height() != 30 {
		t.Errorf("段落高 = %d, want 30（上抬 10px）", p.Height())
	}
	if b.Y() != 0 {
		t.Errorf("盒顶 = %d, want 0（盒随基线上抬，盒顶留在行顶）", b.Y())
	}
}

// vertical-align:50% —— 百分比形式：相对元素自身的 line-height。
// 未声明 line-height → 取字体自然行高 20 → 抬高 10px（与 10px 同值）。
func TestVerticalAlignPercentUsesOwnLineHeight(t *testing.T) {
	_, p, _ := valignDoc(t, shortBox+";vertical-align:50%")
	if p.Height() != 30 {
		t.Errorf("段落高 = %d, want 30（50%% 乘自然行高 20 = 抬 10px）", p.Height())
	}
}

// vertical-align 的百分比基数是元素**自身**声明的 line-height：
// line-height:40px + vertical-align:50% → 抬 20px。
func TestVerticalAlignPercentUsesDeclaredLineHeight(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<p id="p" style="margin:0;line-height:40px">x<span id="b"
			style="display:inline-block;width:20px;height:20px;vertical-align:50%"></span></p>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	p := mustEl(t, doc, "#p")
	// 盒抬 20 → 盒的 ascent 10+20=30；strut（40px 行高）asc 20/descent 20
	// → 行盒 = max(30,20) + max(-10,20) = 30 + 20 = 50
	if p.Height() != 50 {
		t.Errorf("段落高 = %d, want 50（50%% 乘自身 line-height 40 = 抬 20px）", p.Height())
	}
}

// text-top/text-bottom 对齐的是父级**内容区**（字体的 ascent/descent），
// 不含 line-height 的 Leading（CSS 2.1 §10.1）：行高 40 时基线在行顶
// 下方 20，而内容区上沿仍在基线上方 10 —— 盒顶因此落在 10 而非 0。
func TestVerticalAlignTextTopExcludesLeading(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<p id="p" style="margin:0;line-height:40px">x<span id="b"
			style="display:inline-block;width:20px;height:40px;vertical-align:text-top"></span></p>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	b := mustEl(t, doc, "#b")
	// 基线 = 20（行盒 ascent = 字体 ascent 10 + half-leading 10）；
	// 内容区上沿 = 基线 − 字体 ascent 10 = 10 → 盒顶落在 10。
	if b.Y() != 10 {
		t.Errorf("盒顶 = %d, want 10（text-top 对齐内容区上沿，不含 Leading）", b.Y())
	}
}

// 未声明 vertical-align 时行为不变：盒按基线对齐，行盒高 = 盒高。
func TestVerticalAlignDefaultIsBaseline(t *testing.T) {
	_, p, b := valignDoc(t, tallBox)
	// 盒高 40，基线在盒底 − 10 → asc 30 → 行盒 = 30 + 10 = 40
	if p.Height() != 40 {
		t.Errorf("段落高 = %d, want 40（默认基线对齐，盒高 40）", p.Height())
	}
	if b.Y() != 0 {
		t.Errorf("盒顶 = %d, want 0（基线对齐时盒顶在行顶）", b.Y())
	}
}

// <sup>/<sub> 走 UA 样式表的 vertical-align:super/sub，位移量与
// Chrome 实测一致（父字号/3+1、父字号/5+1）。位移体现在**基线**上：
// 上标基线比文字基线高 6px，下标低 4px。
func TestSupSubUseVerticalAlign(t *testing.T) {
	// 上标：抬 6px（父字号/3+1 = 16/3+1）
	supDoc := openDoc(t, 300, 200, `<!doctype html><html><head></head><body style="margin:0">
		<p id="p" style="margin:0">x<sup id="sup">y</sup></p>
	</body></html>`)
	p1 := mustEl(t, supDoc, "#p")
	sup := mustEl(t, supDoc, "#sup")
	// sup 的字号是 UA 的 13px → 盒自身 ascent 8 → 盒基线 = 盒顶 + 8。
	supBase := sup.Y() - p1.Y() + 8
	textBase := svgTextBaselineY(t, supDoc.DumpSVG())
	if got := textBase - supBase; got != 6 {
		t.Errorf("上标基线比文字高 %d px, want 6（上标抬 6px）", got)
	}

	// 下标：沉 4px（父字号/5+1 = 16/5+1）
	subDoc := openDoc(t, 300, 200, `<!doctype html><html><head></head><body style="margin:0">
		<p id="p" style="margin:0">x<sub id="sub">y</sub></p>
	</body></html>`)
	p2 := mustEl(t, subDoc, "#p")
	sub := mustEl(t, subDoc, "#sub")
	subBase := sub.Y() - p2.Y() + 8
	textBase2 := svgTextBaselineY(t, subDoc.DumpSVG())
	if got := subBase - textBase2; got != 4 {
		t.Errorf("下标基线比文字低 %d px, want 4（下标沉 4px）", got)
	}
}

// vertical-align 不继承：父级声明不影响子级行内盒。
func TestVerticalAlignNotInherited(t *testing.T) {
	const src = `<!doctype html><html><head></head><body style="margin:0">
		<div style="vertical-align:super">
			<span id="s" style="display:inline-block;width:20px;height:40px"></span>
		</div>
	</body></html>`
	doc := openDoc(t, 300, 200, src)
	s := mustEl(t, doc, "#s")
	// 未继承 → 基线对齐 → 盒顶 = 0（行顶）
	if s.Y() != 0 {
		t.Errorf("盒顶 = %d, want 0（vertical-align 不继承）", s.Y())
	}
}
