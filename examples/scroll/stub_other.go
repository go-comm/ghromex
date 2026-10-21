//go:build !windows

// 本示例依赖 Windows 的 SDL2 后端；其它平台保留一个可编译的入口，
// 让 `go build ./...` 在任意平台都能通过（运行时给出明确提示）。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "ghromex 示例仅支持 Windows（SDL2 后端）")
	os.Exit(1)
}
