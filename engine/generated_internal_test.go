package engine

import "testing"

// 生成盒是宿主的真实子元素（首/尾），但 Children() 不暴露它。
func TestGeneratedNodesPlacement(t *testing.T) {
	doc, err := OpenDocument(NewHeadlessViewport(300, 100),
		`<!doctype html><html><head><style>
			#d::before { content: "<<"; }
			#d::after { content: ">>"; }
		</style></head><body style="margin:0"><div id="d">X</div></body></html>`)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	d := inner(doc.QuerySelector("#d"))
	if d == nil || d.genBefore == nil || d.genAfter == nil {
		t.Fatalf("生成盒未创建: %+v", d)
	}
	if len(d.children) != 3 {
		t.Fatalf("children 数 = %d, want 3（before + 文本 + after）", len(d.children))
	}
	if inner(d.children[0]) != d.genBefore {
		t.Errorf("::before 不在首位")
	}
	if inner(d.children[len(d.children)-1]) != d.genAfter {
		t.Errorf("::after 不在末位")
	}
	if !d.hasGenerated {
		t.Errorf("hasGenerated 未置位（Children 过滤会失效）")
	}
	if got := len(d.Children()); got != 1 {
		t.Errorf("Children() = %d, want 1（生成盒应被过滤）", got)
	}
	if tn := d.genBefore.genText; tn == nil || tn.Text() != "<<" {
		t.Errorf("::before 文本 = %v, want <<", tn)
	}
	if d.genBefore.computed == nil || d.genBefore.computed.Display() != DisplayInline {
		t.Errorf("生成盒默认 display 应为 inline")
	}
	if d.genBefore.genHost != d || d.genBefore.genPseudo != pseudoElementBefore {
		t.Errorf("生成盒的宿主/伪元素名不对: host=%p pseudo=%q", d.genBefore.genHost, d.genBefore.genPseudo)
	}
}

// content 变回 none 时生成盒从 children 移除并清空缓存。
func TestGeneratedNodesRemovedWhenContentGone(t *testing.T) {
	doc, err := OpenDocument(NewHeadlessViewport(300, 100),
		`<!doctype html><html><head><style>
			#d::before { content: "a"; }
			#d:hover::before { content: none; }
		</style></head><body style="margin:0"><div id="d">X</div></body></html>`)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	d := inner(doc.QuerySelector("#d"))
	if d.genBefore == nil {
		t.Fatalf("初始生成盒未创建")
	}
	SetHoveredElement(doc, d.self)
	doc.Refresh()
	if d.genBefore != nil {
		t.Errorf("content:none 后生成盒缓存未清空")
	}
	if d.hasGenerated {
		t.Errorf("content:none 后 hasGenerated 仍为真")
	}
	for _, c := range d.children {
		if cb := inner(c); cb != nil && cb.generated {
			t.Errorf("content:none 后生成盒仍在 children 里")
		}
	}
}

// content: "" 生成盒子但没有文本子节点（宽高由样式决定）。
func TestGeneratedEmptyContent(t *testing.T) {
	doc, err := OpenDocument(NewHeadlessViewport(300, 100),
		`<!doctype html><html><head><style>
			#d::before { content: ""; display: inline-block; width: 24px; height: 8px; }
		</style></head><body style="margin:0"><div id="d">X</div></body></html>`)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	d := inner(doc.QuerySelector("#d"))
	if d.genBefore == nil {
		t.Fatalf("空 content 也应生成盒子")
	}
	if d.genBefore.genText != nil || len(d.genBefore.children) != 0 {
		t.Errorf("空 content 不该创建文本节点")
	}
	if got := d.genBefore.width; got != 24 {
		t.Errorf("生成盒宽 = %d, want 24", got)
	}
	if got := d.genBefore.height; got != 8 {
		t.Errorf("生成盒高 = %d, want 8", got)
	}
}
