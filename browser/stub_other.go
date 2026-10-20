//go:build !windows

// Package browser 在非 Windows 平台上为空实现。
//
// 装配门面依赖 backend/sdl2（Windows 专用 SDL2 后端），见
// backend/sdl2/stub_other.go 的说明。保留本文件是为了让 `go build ./...`
// 在任意平台都能通过。
package browser
