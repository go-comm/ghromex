package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// 元素级 keydown：聚焦元素的处理器先于引擎默认动作执行；处理器返回 true
// 表示已消费，引擎的默认动作（textarea 的回车换行等）不再执行。
func TestElementKeyDownHandler(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, `<!doctype html><html><body>
		<input id="a" type="text" value="xy">
		<textarea id="t"></textarea>
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	in := doc.QuerySelector("#a")
	ta := doc.QuerySelector("#t")
	if in == nil || ta == nil {
		t.Fatal("元素缺失")
	}

	// 没注册处理器时：单行 input 上的回车不消费（引擎默认行为不变）。
	engine.SetFocusedElement(doc, in)
	if engine.OnDocumentKeyDown(doc, engine.KeyEnter) {
		t.Error("单行 input 的回车不该被引擎消费")
	}

	// 注册后：处理器拿到键码与修饰键，返回 true 表示已消费。
	var gotKey int
	var gotMods uint16
	in.OnKeyDown(func(ev *engine.KeyEvent) bool {
		gotKey, gotMods = ev.Key, ev.Mods
		return true
	})
	if !engine.OnDocumentKeyDown(doc, engine.KeyEnter) {
		t.Error("处理器消费了回车，引擎应返回已消费")
	}
	if gotKey != engine.KeyEnter {
		t.Errorf("处理器收到的键码 = %d, want %d", gotKey, engine.KeyEnter)
	}
	if gotMods != 0 {
		t.Errorf("处理器收到的修饰键 = %d, want 0", gotMods)
	}

	// 修饰键要透传给处理器（应用层据此区分 Ctrl+Enter 之类）。
	gotKey, gotMods = 0, 0
	engine.OnDocumentKeyDownMods(doc, engine.KeyEnter, engine.ModCtrl)
	if gotMods&engine.ModCtrl == 0 {
		t.Errorf("处理器收到的修饰键 = %d, want 含 Ctrl", gotMods)
	}

	// textarea：处理器消费回车 → 引擎不再插入换行；不消费 → 照旧换行。
	engine.SetFocusedElement(doc, ta)
	ta.OnKeyDown(func(ev *engine.KeyEvent) bool { return ev.Key == engine.KeyEnter })
	engine.OnDocumentKeyDown(doc, engine.KeyEnter)
	if got := ta.GetAttribute("value"); got != "" {
		t.Errorf("textarea 值 = %q, want 空（回车被处理器消费）", got)
	}
	ta.OnKeyDown(func(ev *engine.KeyEvent) bool { return false })
	engine.OnDocumentKeyDown(doc, engine.KeyEnter)
	if got := ta.GetAttribute("value"); got != "\n" {
		t.Errorf("textarea 值 = %q, want 换行（处理器不消费时引擎默认动作照旧）", got)
	}
}

// 处理器只在聚焦元素上生效：焦点不在时按键走不到它。
func TestElementKeyDownOnlyForFocusedElement(t *testing.T) {
	vp := engine.NewHeadlessViewport(400, 300)
	doc, err := engine.OpenDocument(vp, `<!doctype html><html><body>
		<input id="a" type="text">
		<input id="b" type="text">
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	a := doc.QuerySelector("#a")
	b := doc.QuerySelector("#b")
	hit := 0
	a.OnKeyDown(func(*engine.KeyEvent) bool {
		hit++
		return true
	})
	engine.SetFocusedElement(doc, b)
	engine.OnDocumentKeyDown(doc, engine.KeyEnter)
	if hit != 0 {
		t.Fatalf("未聚焦元素的处理器被调用了 %d 次", hit)
	}
	engine.SetFocusedElement(doc, a)
	engine.OnDocumentKeyDown(doc, engine.KeyEnter)
	if hit != 1 {
		t.Fatalf("聚焦元素的处理器被调用了 %d 次, want 1", hit)
	}
}
