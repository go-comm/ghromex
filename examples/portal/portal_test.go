package main

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 门户页的布局回归：只断言「页面打开后关键区块确实有非零盒子」，
// 不锁具体像素值。这样调整视觉细节不会误报，但删掉整块结构、宽度
// 计算退化成 0、或侧栏基线错位时能立即失败。
func TestPortalSectionsLayout(t *testing.T) {
	doc, err := engine.OpenDocument(engine.NewHeadlessViewport(1280, 820), string(page))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	defer doc.Close()

	if got := doc.Title(); got != "GHROMEX MARKET · 今日上新" {
		t.Errorf("title = %q, want %q", got, "GHROMEX MARKET · 今日上新")
	}

	selectors := []string{
		".chrome", ".hero", ".hero-side", ".cat", ".deal",
		".main-col", ".side-col", ".product", ".side-panel",
		".footer",
	}
	for _, sel := range selectors {
		el := doc.QuerySelector(sel)
		if el == nil {
			t.Fatalf("%s 不存在", sel)
		}
		r := el.GetBoundingClientRect()
		if r.Right().Pixel() <= r.Left().Pixel() {
			t.Errorf("%s 宽度 = %d, want > 0", sel, r.Right().Pixel()-r.Left().Pixel())
		}
		if r.Bottom().Pixel() <= r.Top().Pixel() {
			t.Errorf("%s 高度 = %d, want > 0", sel, r.Bottom().Pixel()-r.Top().Pixel())
		}
	}

	if got := len(doc.QuerySelectorAll(".cat")); got != 6 {
		t.Errorf("分类数 = %d, want 6", got)
	}
	if got := len(doc.QuerySelectorAll(".product")); got != 8 {
		t.Errorf("商品数 = %d, want 8", got)
	}
	if got := len(doc.QuerySelectorAll(".add")); got != 12 {
		t.Errorf("加入购物车按钮数 = %d, want 12（秒杀 3 + 商品 8 + Hero 1）", got)
	}

	chrome := doc.QuerySelector(".chrome").GetBoundingClientRect()
	if top := chrome.Top().Pixel(); top != 0 {
		t.Errorf("固定顶栏 top = %d, want 0", top)
	}

	hero := doc.QuerySelector(".hero").GetBoundingClientRect()
	side := doc.QuerySelector(".hero-side").GetBoundingClientRect()
	if side.Top().Pixel() != hero.Top().Pixel() {
		t.Errorf("Hero 侧栏 top = %d, want %d（绝对定位应与主视觉顶边对齐）", side.Top().Pixel(), hero.Top().Pixel())
	}

	mainCol := doc.QuerySelector(".main-col").GetBoundingClientRect()
	sideCol := doc.QuerySelector(".side-col").GetBoundingClientRect()
	if sideCol.Top().Pixel() != mainCol.Top().Pixel() {
		t.Errorf("侧栏 top = %d, want %d（应与主内容顶边对齐）", sideCol.Top().Pixel(), mainCol.Top().Pixel())
	}
}
