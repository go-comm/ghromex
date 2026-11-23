//go:build windows

// 文件拖放（拖入窗口）端到端链路：平台事件注入 → 主循环 → 应用回调。
//
// 真实拖放（Explorer → 窗口）要真人操作，无法自动化；这里验证的是
// backend → browser 这一段：一批路径必须原样、按序、一次到达回调。
package browser

import (
	"testing"
	"time"

	"github.com/go-comm/ghromex/backend/sdl2"
)

func TestFileDropHandlerReceivesDroppedFiles(t *testing.T) {
	win := newTestWindow(t, 320, 200, "filedrop")
	if _, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">drop</body></html>`); err != nil {
		t.Fatal(err)
	}

	var got []string
	win.SetFileDropHandler(func(files []string) {
		got = append(got, files...)
	})

	win.InjectEvent(sdl2.Event{Kind: sdl2.EventFilesDropped, Files: []string{`C:\pics\a.png`}})
	win.InjectEvent(sdl2.Event{Kind: sdl2.EventFilesDropped, Files: []string{`C:\pics\b.png`, `C:\pics\c.jpg`}})

	win.SetAutoExit(100 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	want := []string{`C:\pics\a.png`, `C:\pics\b.png`, `C:\pics\c.jpg`}
	if len(got) != len(want) {
		t.Fatalf("回调收到 %d 个路径 (%q)，期望 %d 个", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 个路径 = %q, want %q（顺序被改动）", i, got[i], want[i])
		}
	}
}

// 没注册回调时不等于报错：拖放被忽略，主循环照常运行。
func TestFileDropWithoutHandlerIsIgnored(t *testing.T) {
	win := newTestWindow(t, 320, 200, "filedrop-none")
	if _, err := win.OpenDocument(`<!doctype html><html><body style="margin:0">drop</body></html>`); err != nil {
		t.Fatal(err)
	}

	win.InjectEvent(sdl2.Event{Kind: sdl2.EventFilesDropped, Files: []string{`C:\pics\a.png`}})
	win.SetAutoExit(100 * time.Millisecond)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}
}

// 回调里换文档必须当场生效：拖放回调与按键/鼠标钩子同层，在事件阶段
// 执行，可以开新文档、改 DOM。
func TestFileDropHandlerCanOpenDocument(t *testing.T) {
	win := newTestWindow(t, 320, 200, "filedrop-open")
	if _, err := win.OpenDocument(`<!doctype html><html><body style="margin:0"><div id="old">old</div></body></html>`); err != nil {
		t.Fatal(err)
	}

	var opened string
	win.SetFileDropHandler(func(files []string) {
		if len(files) == 0 {
			return
		}
		opened = files[0]
		src := `<!doctype html><html><body style="margin:0"><div id="new">` + files[0] + `</div></body></html>`
		if _, err := win.OpenDocument(src); err != nil {
			t.Errorf("回调里打开文档失败: %v", err)
		}
	})

	win.InjectEvent(sdl2.Event{Kind: sdl2.EventFilesDropped, Files: []string{`C:\pics\a.png`}})
	win.SetFrameHandler(func() bool {
		win.RequestClose()
		return false
	})
	win.SetAutoExit(time.Second)
	if err := win.Run(); err != nil {
		t.Fatal(err)
	}

	if opened != `C:\pics\a.png` {
		t.Fatalf("回调没有收到路径：%q", opened)
	}
	if win.doc == nil || win.doc.GetElementById("new") == nil {
		t.Fatal("拖放回调里打开的文档没有生效（回调不在事件阶段？）")
	}
}
