package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 无头导出的冒烟测试：示例的接线（页面 + 命令表 + Install）必须能跑通
// 并产出可解码的图。示例最容易坏的地方是改了页面/命令表忘了同步尺寸，
// 所以这里连尺寸一起断言。
func TestDumpProducesPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kit.png")
	if err := dump(buildSpec(func(string) {}), path, ""); err != nil {
		t.Fatalf("无头导出 PNG 失败: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 PNG: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("PNG 无法解码: %v", err)
	}
	if cfg.Width != winW || cfg.Height != winH {
		t.Fatalf("PNG 尺寸 = %dx%d，期望 %dx%d", cfg.Width, cfg.Height, winW, winH)
	}
}

// SVG 导出：尺寸来自视口，且文字真的画出来了（每字一个 <text>）。
func TestDumpProducesSVG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kit.svg")
	if err := dump(buildSpec(func(string) {}), "", path); err != nil {
		t.Fatalf("无头导出 SVG 失败: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 SVG: %v", err)
	}
	svg := string(data)
	if !strings.HasPrefix(svg, "<svg") {
		t.Fatalf("输出不像 SVG: %.40q", svg)
	}
	if !strings.Contains(svg, `viewBox="0 0 900 760"`) {
		t.Fatalf("SVG 尺寸不对，缺少 viewBox：%.80q", svg)
	}
	// 菜单栏、工具栏、状态栏、页面内容都要有字，否则就是外壳没装上。
	if n := strings.Count(svg, "<text"); n < 20 {
		t.Fatalf("SVG 只有 %d 个文本节点，外壳或页面内容可能没渲染", n)
	}
}
