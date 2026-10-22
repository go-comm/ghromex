//go:build windows

// 窗口钩子测试：SetOnDocument（换文档重装外壳）与 SetKeyHandler（应用
// 快捷键先于文档）。
package browser

import (
	"testing"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/engine"
)

// SetOnDocument 在每次 OpenDocument/OpenDocumentWith 成功后各触发一次，
// 且触发时 win.doc 已经指向新文档、回调里的 DOM 改动会被主循环看到。
func TestOnDocumentFiresPerOpen(t *testing.T) {
	win := newTestWindow(t, 320, 240, "hooks")

	var got []engine.HTMLDocument
	win.SetOnDocument(func(doc engine.HTMLDocument) {
		got = append(got, doc)
		if win.doc != doc {
			t.Errorf("回调时 win.doc 尚未指向新文档")
		}
		// 模拟组件库装外壳：回调里改 DOM 必须真的落到文档上。
		marker := doc.CreateElement("div")
		marker.SetAttribute("id", "hook-marker")
		doc.Body().AppendChild(marker)
	})

	first, err := win.OpenDocument(`<div>一</div>`)
	if err != nil {
		t.Fatalf("打开第一页: %v", err)
	}
	second, err := win.OpenDocumentWith(`<div>二</div>`, engine.DocumentOptions{})
	if err != nil {
		t.Fatalf("打开第二页: %v", err)
	}

	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("回调收到 %d 个文档，期望两页各一次且顺序一致", len(got))
	}
	if second.GetElementById("hook-marker") == nil {
		t.Fatal("回调里的 DOM 改动没有落到文档上")
	}

	// 取消注册后不再触发。
	win.SetOnDocument(nil)
	if _, err := win.OpenDocument(`<div>三</div>`); err != nil {
		t.Fatalf("打开第三页: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("取消注册后仍触发了回调（%d 次）", len(got))
	}
}

// 应用按键处理器先于文档执行：返回 true 时 Ctrl+A 不该落到文档。
func TestKeyHandlerRunsBeforeDocument(t *testing.T) {
	win := newTestWindow(t, 320, 240, "hooks")
	doc, err := win.OpenDocument(`<input id="q" type="text" value="hello">`)
	if err != nil {
		t.Fatalf("打开文档: %v", err)
	}
	input := doc.GetElementById("q")
	engine.SetFocusedElement(doc, input)
	if engine.FocusedElement(doc) != input {
		t.Fatal("输入框没能聚焦")
	}

	var seenKey int
	var seenMods uint16
	win.SetKeyHandler(func(key int, mods uint16) bool {
		seenKey, seenMods = key, mods
		// 只吃 Ctrl+A：验证「命中就拦下」，其它键放行给文档。
		return (mods&sdl2.KmodCtrl) != 0 && (key == 'a' || key == 'A')
	})

	win.onKeyDown(sdl2.Event{Key: 'a', Mod: sdl2.KmodCtrl})
	if seenKey != 'a' || seenMods != sdl2.KmodCtrl {
		t.Fatalf("处理器收到 key=%d mods=0x%x，期望 a/Ctrl", seenKey, seenMods)
	}
	if text, ok := engine.SelectedText(doc); ok {
		t.Fatalf("被应用拦截的 Ctrl+A 落到了文档（选中 %q）", text)
	}

	// 摘掉处理器后同样的按键应该由文档处理（全选）。
	win.SetKeyHandler(nil)
	win.onKeyDown(sdl2.Event{Key: 'a', Mod: sdl2.KmodCtrl})
	if text, ok := engine.SelectedText(doc); !ok || text != "hello" {
		t.Fatalf("文档没有处理 Ctrl+A：选中 = %q, ok=%v", text, ok)
	}
}
