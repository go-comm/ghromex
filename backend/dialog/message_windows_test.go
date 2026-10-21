//go:build windows

package dialog

import "testing"

// 消息框的位标志映射：图标与按钮组合要翻译成 MessageBoxW 的 uType。
//
// 这层不弹框（弹框要真人操作），但映射错了用户看到的就是错的图标或
// 错的按钮，所以单独测。
func TestMessageBoxFlags(t *testing.T) {
	cases := []struct {
		name    string
		icon    MessageIcon
		buttons MessageButtons
		want    uint32
	}{
		{"信息+确定", IconInfo, ButtonsOK, mbOK | mbIconInfo},
		{"警告+确定取消", IconWarning, ButtonsOKCancel, mbOKCancel | mbIconWarning},
		{"错误+是否", IconError, ButtonsYesNo, mbYesNo | mbIconError},
		{"询问+是否取消", IconQuestion, ButtonsYesNoCancel, mbYesNoCancel | mbIconQuestion},
		{"零值默认信息框", 0, 0, mbOK | mbIconInfo},
	}
	for _, c := range cases {
		if got := messageBoxFlags(c.icon, c.buttons); got != c.want {
			t.Errorf("%s: messageBoxFlags = 0x%X, want 0x%X", c.name, got, c.want)
		}
	}
}

// MessageBoxW 的返回值（被点击按钮的 ID）要翻译成 MessageResult。
func TestMessageResult(t *testing.T) {
	cases := []struct {
		id   uintptr
		want MessageResult
		ok   bool
	}{
		{msgIDOK, ResultOK, true},
		{msgIDCancel, ResultCancel, true},
		{msgIDYes, ResultYes, true},
		{msgIDNo, ResultNo, true},
		{0, ResultOK, false},  // MessageBoxW 失败时返回 0
		{99, ResultOK, false}, // 未知按钮
	}
	for _, c := range cases {
		got, ok := messageResult(c.id)
		if got != c.want || ok != c.ok {
			t.Errorf("messageResult(%d) = (%v, %v), want (%v, %v)", c.id, got, ok, c.want, c.ok)
		}
	}
}
