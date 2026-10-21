//go:build windows

package browser

import (
	"testing"
)

// 真窗口模式下 NativeHandle 要给出真实 HWND——原生对话框（runtime/dialog）
// 靠它当父窗口（拿错会让 IFileDialog::Show 直接回 E_INVALIDARG）。
//
// 无显示环境（CI）建不出窗口，此时跳过。
func TestNativeHandleReturnsHWND(t *testing.T) {
	win, err := New(320, 200, "native-handle-test")
	if err != nil {
		t.Skipf("创建窗口失败（无显示环境？）: %v", err)
	}
	defer win.Close()

	hwnd, err := win.NativeHandle()
	if err != nil {
		t.Fatalf("取原生句柄失败: %v", err)
	}
	if hwnd == 0 {
		t.Fatal("原生句柄为 0")
	}
}
