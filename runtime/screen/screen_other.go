//go:build !windows

package screen

import "image"

// capture 在非 Windows 平台没有实现。
func capture() (*image.RGBA, error) {
	return nil, ErrUnsupported
}
