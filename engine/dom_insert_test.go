package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// ids 取出元素的 id 序列，方便断言顺序。
func ids(es []engine.HTMLElement) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.ID())
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// InsertBefore 是 UI 外壳「在页面内容之前插占位块」的基础：必须真的插到
// 参照节点之前。没有它就只能「摘掉全部子节点再按序接回」，会把应用自己
// 挂在子节点上的顺序与引用全部打乱。
func TestInsertBeforeInsertsAtPosition(t *testing.T) {
	doc := engine.NewDocument()
	body := doc.Body()

	a := doc.CreateElement("div")
	a.SetID("a")
	b := doc.CreateElement("div")
	b.SetID("b")
	body.AppendChild(a, b)

	spacer := doc.CreateElement("div")
	spacer.SetID("spacer")
	body.InsertBefore(spacer, a)

	if got, want := ids(body.Children()), []string{"spacer", "a", "b"}; !equalIDs(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	// 父指针必须接上：与兄弟节点指向同一个父实体。
	//
	// 这里不直接和 doc.Body() 比：Body() 返回的是 htmlBodyElement 包装器，
	// 而父链里存的是它内部嵌的 *htmlElement（引擎既有实现），两者不是
	// 同一个接口值。用「和兄弟的父指针一致」断言既避开这层包装，也正好
	// 覆盖「插进来的节点要能被事件/变更通知上溯」这个真正的要求。
	if p := spacer.ParentElement(); p == nil || p != a.ParentElement() {
		t.Fatal("InsertBefore 没有把新节点接到父链上")
	}

	// 插到中间：新节点必须在 b 之前。
	mid := doc.CreateElement("div")
	mid.SetID("mid")
	body.InsertBefore(mid, b)
	if got, want := ids(body.Children()), []string{"spacer", "a", "mid", "b"}; !equalIDs(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
}

// 参照节点为 nil 或不属于本元素时退回追加：调用方拿不到 children[0]
// （空容器）或参照节点已被移除时，不应丢节点。
func TestInsertBeforeFallsBackToAppend(t *testing.T) {
	doc := engine.NewDocument()
	body := doc.Body()

	a := doc.CreateElement("div")
	a.SetID("a")
	body.AppendChild(a)

	spacer := doc.CreateElement("div")
	spacer.SetID("spacer")
	body.InsertBefore(spacer, nil)
	if got, want := ids(body.Children()), []string{"a", "spacer"}; !equalIDs(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}

	// 外来节点（不属于 body）当参照：退回追加。
	stranger := doc.CreateElement("div")
	stranger.SetID("stranger")
	b := doc.CreateElement("div")
	b.SetID("b")
	body.InsertBefore(b, stranger)
	if got, want := ids(body.Children()), []string{"a", "spacer", "b"}; !equalIDs(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
}
