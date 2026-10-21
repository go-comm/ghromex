//go:build !windows

package browser

import (
	"errors"
	"testing"
)

func TestNewReturnsErrUnsupported(t *testing.T) {
	win, err := New(800, 600, "test")
	if win != nil {
		t.Fatalf("New 返回了非 nil 窗口: %v", win)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("New 错误 = %v, want ErrUnsupported", err)
	}
}
