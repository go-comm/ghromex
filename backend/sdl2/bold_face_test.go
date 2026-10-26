//go:build windows

package sdl2

import (
	"path/filepath"
	"testing"
)

// TestBoldUsesRealBoldFace 粗体文字必须用**真粗体字文件**（msyhbd.ttc /
// arialbd.ttf / segoeuib.ttf），而不是让 FreeType 对常规字形做合成加粗：
// 合成加粗明显更重，24px 中文标题上还会出现笔画粘连/重影，与浏览器（按
// 字体族选粗体字重）差别很大。
func TestBoldUsesRealBoldFace(t *testing.T) {
	if err := Load(); err != nil {
		t.Skip(err)
	}
	if ttfInit() != 0 {
		t.Skip("TTF_Init 失败")
	}
	defer ttfQuit()

	g := NewGraphics(0, "")
	defer g.CloseFonts()

	regular := filepath.Join(fontsDir, "msyh.ttc")
	boldPath := filepath.Join(fontsDir, "msyhbd.ttc")
	if fontPathOf("msyhbd.ttc") == "" {
		t.Skip("本机无 msyhbd.ttc（真粗体雅黑），跳过")
	}
	if g.face(regular, 24, tierBold, false) == nil {
		t.Fatal("粗体 face 打开失败")
	}
	found := false
	for k, f := range g.faces {
		if filepath.Base(k.path) != "msyhbd.ttc" {
			continue
		}
		found = true
		if !f.bold {
			t.Error("真粗体 face 的样式标记应为 bold（避免再叠加 TTF_STYLE_BOLD）")
		}
	}
	if !found {
		t.Fatalf("粗体请求未打开真粗体文件 %s（face keys: %v）", boldPath, g.faces)
	}
	// 常规请求仍用常规文件，且两者是不同的 face（不互相污染样式）
	if g.face(regular, 24, tierRegular, false) == nil {
		t.Fatal("常规 face 打开失败")
	}
	regularFound := false
	for k := range g.faces {
		if filepath.Base(k.path) == "msyh.ttc" {
			regularFound = true
		}
	}
	if !regularFound {
		t.Error("常规请求应打开 msyh.ttc（与粗体 face 分开）")
	}
}

// TestBoldVariantLookup 粗体变体表按文件名匹配，缺文件时安全回退。
func TestBoldVariantLookup(t *testing.T) {
	for file, want := range map[string]string{
		"msyh.ttc":    "msyhbd.ttc",
		"arial.ttf":   "arialbd.ttf",
		"segoeui.ttf": "segoeuib.ttf",
		"times.ttf":   "timesbd.ttf",
	} {
		if got := boldVariants[file]; got != want {
			t.Errorf("boldVariants[%q] = %q, want %q", file, got, want)
		}
	}
	// 无粗体变体的字体（黑体等）不进表 → 回退合成加粗（与浏览器一致）
	if _, ok := boldVariants["simhei.ttf"]; ok {
		t.Error("simhei.ttf 无真粗体文件，不应有变体映射")
	}
}
