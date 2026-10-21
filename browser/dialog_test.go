package browser

import (
	"errors"
	"testing"
)

// 没有窗口时不能弹对话框：返回 ErrUnsupported，而不是崩。
//
// 无头模式（-dump-svg）与非 Windows 平台都会走到这里：应用层据此
// 提示「当前环境没有原生对话框」，或者退回到自己实现的路径输入。
func TestDialogWithoutWindow(t *testing.T) {
	win := &Window{}
	if _, err := win.OpenFileDialog(DialogOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("OpenFileDialog 错误 = %v, want ErrUnsupported", err)
	}
	if _, err := win.OpenMultipleFilesDialog(DialogOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("OpenMultipleFilesDialog 错误 = %v, want ErrUnsupported", err)
	}
	if _, err := win.OpenDirectoryDialog(DialogOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("OpenDirectoryDialog 错误 = %v, want ErrUnsupported", err)
	}
	if _, err := win.SaveFileDialog(DialogOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("SaveFileDialog 错误 = %v, want ErrUnsupported", err)
	}
	if _, err := win.MessageDialog(MessageOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("MessageDialog 错误 = %v, want ErrUnsupported", err)
	}
}

// 过滤器要原样转发给对话框包（显示名与扩展名模式都不能丢）。
func TestFilterForward(t *testing.T) {
	got := toDialogFilters([]FileFilter{
		{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
		{Name: "所有文件 (*.*)", Pattern: "*.*"},
	})
	if len(got) != 2 {
		t.Fatalf("转发后的过滤器数 = %d, want 2", len(got))
	}
	if got[0].Name != "文本文件 (*.txt)" || got[0].Pattern != "*.txt" {
		t.Errorf("第 1 个过滤器 = %+v", got[0])
	}
	if got[1].Name != "所有文件 (*.*)" || got[1].Pattern != "*.*" {
		t.Errorf("第 2 个过滤器 = %+v", got[1])
	}
	if toDialogFilters(nil) != nil {
		t.Errorf("空过滤器应转发为 nil")
	}
}
