package dialog

import (
	"testing"

	backenddialog "github.com/go-comm/ghromex/backend/dialog"
)

// 通用层的类型就是平台层的类型（别名）：调用方拿平台层的值传进来也能对上，
// 平台层换实现时通用层的名字不变。
func TestTypeAliasesMatchPlatform(t *testing.T) {
	var (
		_ Options        = backenddialog.Options{}
		_ FileFilter     = backenddialog.FileFilter{}
		_ MessageOptions = backenddialog.MessageOptions{}
		_ MessageIcon    = backenddialog.IconInfo
		_ MessageButtons = backenddialog.ButtonsOK
		_ MessageResult  = backenddialog.ResultOK
	)
	if ErrUnsupported != backenddialog.ErrUnsupported {
		t.Errorf("通用层的 ErrUnsupported 与平台层不是同一个错误")
	}
}

// Parent 是原生窗口句柄的包装：零值表示「没有父窗口」（对话框照样能弹，
// 只是不挂在主窗口下面）。
func TestParentZeroMeansNoParent(t *testing.T) {
	var p Parent
	if uintptr(p) != 0 {
		t.Errorf("Parent 零值 = %d, want 0", uintptr(p))
	}
}
