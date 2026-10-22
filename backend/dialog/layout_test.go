//go:build windows

package dialog

import (
	"testing"
	"unsafe"
)

// 跨 DLL 边界的结构体大小必须与 Windows 头文件一致。
//
// 与 backend/sdl2 里 SDL_RendererInfo 那条同类：Go 侧估小了，运行库按真实
// 结构体大小写，越界写坏调用者的栈。GUITHREADINFO 的末字段 rcCaret 是
// RECT（4×int32 = 16 字节），写成 int32 会让结构体只有 64 字节，而真实是
// 72 字节——GetGUIThreadInfo 越界写 8 字节。
func TestGUIThreadInfoLayoutMatchesHeader(t *testing.T) {
	if got := unsafe.Sizeof(guiThreadInfo{}); got != 72 {
		t.Errorf("GUITHREADINFO 结构体 = %d 字节, want 72（winuser.h, 64 位）", got)
	}
	if got := unsafe.Offsetof(guiThreadInfo{}.hwndFocus); got != 16 {
		t.Errorf("GUITHREADINFO.hwndFocus 偏移 = %d, want 16", got)
	}
	if got := unsafe.Offsetof(guiThreadInfo{}.rcCaret); got != 56 {
		t.Errorf("GUITHREADINFO.rcCaret 偏移 = %d, want 56", got)
	}
	if got := unsafe.Sizeof(guiRect{}); got != 16 {
		t.Errorf("RECT 大小 = %d, want 16", got)
	}
}

// GUID 与 COMDLG_FILTERSPEC 同样是按值/按指针交给系统 API 的结构体。
func TestComStructLayouts(t *testing.T) {
	if got := unsafe.Sizeof(guid{}); got != 16 {
		t.Errorf("GUID 大小 = %d, want 16", got)
	}
	if got := unsafe.Sizeof(comDlgFilterSpec{}); got != 16 {
		t.Errorf("COMDLG_FILTERSPEC 大小 = %d, want 16（两个 LPCWSTR）", got)
	}
}
