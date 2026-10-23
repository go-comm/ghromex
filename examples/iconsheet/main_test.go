package main

import (
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/ui"
)

// 分组表要覆盖 ui.Icons() 的每一项，且不能重复；改名/新增图标时会直接失败。
func TestIconGroupsCoverAllIcons(t *testing.T) {
	all := ui.Icons()
	known := make(map[string]bool, len(all))
	for _, e := range all {
		known[e.Name] = true
	}

	seen := make(map[string]bool, len(all))
	for _, g := range iconGroups {
		for _, name := range g.Names {
			if !known[name] {
				t.Errorf("分组 %q 里的图标不存在: %s", g.Title, name)
			}
			if seen[name] {
				t.Errorf("图标重复分组: %s", name)
			}
			seen[name] = true
		}
	}
	for _, e := range all {
		if !seen[e.Name] {
			t.Errorf("图标未分组: %s", e.Name)
		}
	}
}

// 生成出来的页面必须包含每个图标名，避免分组逻辑漏项。
func TestIconSheetContainsEveryIcon(t *testing.T) {
	entries := ui.Icons()
	page := buildPage(entries)
	for _, e := range entries {
		if !strings.Contains(page, `data-icon="`+e.Name+`"`) {
			t.Errorf("页面缺少图标 %q", e.Name)
		}
	}
}

// 卡片尺寸要压过主题的 system-ui-btn（根作用域后是 0,2,1）：这里真正
// Install 外壳再量布局，防止以后又把 176x136 退回 30px 高的工具栏按钮。
func TestIconCardSize(t *testing.T) {
	doc := newInstalledSheet(t)
	card := iconCard(t, doc, "up")
	b := card.GetBoundingClientRect()
	if w := b.Right().Pixel() - b.Left().Pixel(); w != 176 {
		t.Errorf("卡片宽 = %d，期望 176", w)
	}
	if h := b.Bottom().Pixel() - b.Top().Pixel(); h != 136 {
		t.Errorf("卡片高 = %d，期望 136", h)
	}

	glyph := card.QuerySelector(".icon-glyph")
	if glyph == nil {
		t.Fatal("卡片缺少图标字形")
	}
	gb := glyph.GetBoundingClientRect()
	if h := gb.Bottom().Pixel() - gb.Top().Pixel(); h < 42 {
		t.Errorf("图标字形高 = %d，期望至少 42（字号 42px）", h)
	}
}

func newInstalledSheet(t *testing.T) engine.HTMLDocument {
	t.Helper()
	vp, _ := engine.NewBufferedViewport(winW, winH)
	doc, err := engine.OpenDocument(vp, buildPage(ui.Icons()))
	if err != nil {
		t.Fatal(err)
	}
	ui.Install(doc, buildSpec(ui.Icons(), func(string) {}, ui.Item("切换主题", "", nil)))
	doc.Refresh()
	return doc
}

// 搜索按名称子串过滤，支持大小写不敏感和多关键词；分组/空状态/结果计数
// 都跟着更新。这里用真实文档验证输入框 value 变化到页面可见性的整条链。
func TestSearchFiltersByName(t *testing.T) {
	doc, input := newSearchSheet(t)

	if isHidden(iconCard(t, doc, "play")) {
		t.Fatal("初始状态不应隐藏图标")
	}

	input.SetAttribute("value", "align")
	for _, name := range []string{"align-left", "align-center", "align-right"} {
		if isHidden(iconCard(t, doc, name)) {
			t.Errorf("%s 应匹配 align", name)
		}
	}
	for _, name := range []string{"play", "volume", "trash"} {
		if !isHidden(iconCard(t, doc, name)) {
			t.Errorf("%s 不应匹配 align", name)
		}
	}
	if !isHidden(iconGroup(t, doc, "play")) {
		t.Error("没有匹配项的分组应隐藏")
	}
	if isHidden(iconGroup(t, doc, "align-left")) {
		t.Error("有匹配项的分组不应隐藏")
	}
	if got := searchResult(t, doc); got != "找到 3 个名字" {
		t.Errorf("结果 = %q，期望 %q", got, "找到 3 个名字")
	}
	if got := groupCount(t, doc, "align-left"); got != "3/14" {
		t.Errorf("对齐组计数 = %q，期望 %q", got, "3/14")
	}
	if !isHidden(doc.QuerySelector("#icon-empty")) {
		t.Error("有结果时空状态应隐藏")
	}

	// 多关键词是「都包含」而不是整串匹配。
	input.SetAttribute("value", "align left")
	if isHidden(iconCard(t, doc, "align-left")) {
		t.Error("align-left 应匹配 align left")
	}
	if !isHidden(iconCard(t, doc, "align-center")) {
		t.Error("align-center 不应匹配 align left")
	}
	if got := searchResult(t, doc); got != "找到 1 个名字" {
		t.Errorf("多关键词结果 = %q，期望 %q", got, "找到 1 个名字")
	}

	// 大小写不敏感。
	input.SetAttribute("value", "ALIGN")
	if isHidden(iconCard(t, doc, "align-left")) {
		t.Error("ALIGN 应匹配 align-left")
	}
	if got := searchResult(t, doc); got != "找到 3 个名字" {
		t.Errorf("大写查询结果 = %q，期望 %q", got, "找到 3 个名字")
	}

	// volume 一族都要留下，mute 没有 volume 子串。
	input.SetAttribute("value", "volume")
	if got := searchResult(t, doc); got != "找到 5 个名字" {
		t.Errorf("volume 结果 = %q，期望 %q", got, "找到 5 个名字")
	}
	if !isHidden(iconCard(t, doc, "mute")) {
		t.Error("mute 不应匹配 volume")
	}

	// 无结果：所有卡片隐藏，空状态出现。
	input.SetAttribute("value", "zzz")
	if !isHidden(iconCard(t, doc, "play")) {
		t.Error("无结果时卡片应隐藏")
	}
	if isHidden(doc.QuerySelector("#icon-empty")) {
		t.Error("无结果时应显示空状态")
	}
	if got := searchResult(t, doc); got != "找到 0 个名字" {
		t.Errorf("无结果计数 = %q，期望 %q", got, "找到 0 个名字")
	}

	// 清空恢复全部。
	input.SetAttribute("value", "")
	if isHidden(iconCard(t, doc, "play")) {
		t.Error("清空后应恢复全部图标")
	}
	if !isHidden(doc.QuerySelector("#icon-empty")) {
		t.Error("清空后空状态应隐藏")
	}
	if got := searchResult(t, doc); got != "共 160 个名字" {
		t.Errorf("清空后结果 = %q，期望 %q", got, "共 160 个名字")
	}
}

// 真实输入链路：聚焦搜索框 → TextInput 插入 → 过滤；退格删除同样生效。
func TestSearchTypingAndBackspace(t *testing.T) {
	doc, input := newSearchSheet(t)
	engine.SetFocusedElement(doc, input)

	if !engine.OnDocumentTextInput(doc, "align") {
		t.Fatal("搜索框聚焦后 TextInput 应被消费")
	}
	if got := input.GetAttribute("value"); got != "align" {
		t.Fatalf("输入后 value = %q，期望 %q", got, "align")
	}
	if isHidden(iconCard(t, doc, "align-left")) {
		t.Error("输入 align 后 align-left 应可见")
	}
	if !isHidden(iconCard(t, doc, "play")) {
		t.Error("输入 align 后 play 应隐藏")
	}

	// 退格回到空串，全部图标恢复。
	for i := 0; i < len("align"); i++ {
		engine.OnDocumentKeyDown(doc, engine.KeyBackspace)
	}
	if got := input.GetAttribute("value"); got != "" {
		t.Fatalf("退格后 value = %q，期望空串", got)
	}
	if isHidden(iconCard(t, doc, "play")) {
		t.Error("退格清空后应恢复全部图标")
	}
}

// Esc 在输入框聚焦时清空查询。
func TestSearchEscapeClears(t *testing.T) {
	doc, input := newSearchSheet(t)
	input.SetAttribute("value", "volume")
	if !isHidden(iconCard(t, doc, "play")) {
		t.Fatal("volume 查询应过滤掉 play")
	}

	engine.SetFocusedElement(doc, input)
	engine.OnDocumentKeyDown(doc, engine.KeyEscape)
	if got := input.GetAttribute("value"); got != "" {
		t.Fatalf("Esc 后 value = %q，期望空串", got)
	}
	if isHidden(iconCard(t, doc, "play")) {
		t.Error("Esc 清空后应恢复全部图标")
	}
}

func newSearchSheet(t *testing.T) (engine.HTMLDocument, engine.HTMLElement) {
	t.Helper()
	vp, _ := engine.NewBufferedViewport(winW, winH)
	doc, err := engine.OpenDocument(vp, buildPage(ui.Icons()))
	if err != nil {
		t.Fatal(err)
	}
	wireSearch(doc, nil)
	input := doc.QuerySelector("#icon-search")
	if input == nil {
		t.Fatal("页面缺少搜索框")
	}
	return doc, input
}

func iconCard(t *testing.T, doc engine.HTMLDocument, name string) engine.HTMLElement {
	t.Helper()
	el := doc.QuerySelector(`.icon-card[data-icon="` + name + `"]`)
	if el == nil {
		t.Fatalf("找不到图标卡片 %s", name)
	}
	return el
}

func iconGroup(t *testing.T, doc engine.HTMLDocument, name string) engine.HTMLElement {
	t.Helper()
	card := iconCard(t, doc, name)
	grid := card.ParentElement()
	if grid == nil || grid.ParentElement() == nil {
		t.Fatalf("图标卡片 %s 缺少分组容器", name)
	}
	return grid.ParentElement()
}

func groupCount(t *testing.T, doc engine.HTMLDocument, name string) string {
	t.Helper()
	el := iconGroup(t, doc, name).QuerySelector(".icon-count")
	if el == nil {
		t.Fatalf("分组 %s 缺少计数", name)
	}
	return el.Text()
}

func searchResult(t *testing.T, doc engine.HTMLDocument) string {
	t.Helper()
	el := doc.QuerySelector("#icon-search-result")
	if el == nil {
		t.Fatal("缺少搜索结果计数")
	}
	return el.Text()
}

func isHidden(el engine.HTMLElement) bool {
	return el != nil && strings.Contains(el.GetAttribute("class"), "is-hidden")
}
