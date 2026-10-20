//go:build !windows

package main

import (
	"fmt"
	"os"
)

// main 在非 Windows 平台给出明确提示：demo 依赖 Windows 的 SDL2 后端
// （见 browser/stub_other.go 与 backend/sdl2/stub_other.go）。
func main() {
	fmt.Fprintln(os.Stderr, "ghromex demo 仅支持 Windows（SDL2 后端）")
	os.Exit(1)
}
