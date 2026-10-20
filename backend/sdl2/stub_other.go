//go:build !windows

// Package sdl2 在非 Windows 平台上为空实现。
//
// 本后端依赖 Windows 的 SDL2.dll / SDL2_ttf.dll 与系统字体目录
// （backend/sdl2 下的实现文件均为 //go:build windows）。保留本文件是为了让
// `go build ./...` / `go vet ./...` / `go test ./...` 在任意平台都能通过：
// engine 包本身是纯 Go、可移植的，其它平台可自行实现 engine.Graphics 后端。
package sdl2
